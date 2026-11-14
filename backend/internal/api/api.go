// Package api exposes the evaluator over HTTP. Resources mirror the domain:
// suites (definition) and evaluations (execution); a scenario and a turn of an
// evaluation are addressed with the suite's scenario key and turn index.
// Comparisons are read-only views at each level (?with=<other evaluation>).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/analysis"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/compare"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/report"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/store"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/suite"
)

type Store interface {
	report.Loader
	ListAnalyses(ctx context.Context, turnResultID int64) ([]domain.Analysis, error)
	CreateSuite(ctx context.Context, s domain.Suite) (domain.Suite, bool, error)
	ListSuites(ctx context.Context) ([]store.SuiteSummary, error)
	CreateEvaluation(ctx context.Context, e domain.Evaluation) (domain.Evaluation, error)
	ListEvaluations(ctx context.Context, suiteName string) ([]store.EvaluationSummary, error)
	ListSpans(ctx context.Context, turnResultID int64) ([]domain.Span, error)
	AddAnnotation(ctx context.Context, turnResultID int64, a domain.Annotation) (domain.Annotation, error)
}

// Submitter enqueues a persisted evaluation for execution.
type Submitter interface {
	Submit(ctx context.Context, evaluationID int64) error
}

// Analyst gives an LLM second opinion on turn evaluations.
type Analyst interface {
	AnalyzeTurn(ctx context.Context, evaluationID int64, key string, index int) (domain.Analysis, error)
	AnalyzeFailed(ctx context.Context, evaluationID int64) (int, error)
}

// HealthCheck reports the status of a dependency (nil = ok).
type HealthCheck func(ctx context.Context) error

// Check is a named dependency. An optional one (e.g. the LLM judge, which
// deterministic-only suites do not need) only produces a warning.
type Check struct {
	Fn       HealthCheck
	Optional bool
}

type Server struct {
	store   Store
	runner  Submitter
	suites  suite.Options
	health  map[string]Check
	log     *slog.Logger
	maxBody int64
	origin  string // allowed CORS origin; empty = same origin only
	analyst Analyst
}

// WithAnalysis enables the analysis endpoints.
func (s *Server) WithAnalysis(a Analyst) *Server {
	s.analyst = a
	return s
}

func New(st Store, r Submitter, suites suite.Options, health map[string]Check, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{store: st, runner: r, suites: suites, health: health, log: log, maxBody: 5 << 20}
}

// WithCORS allows one browser origin (e.g. the Vite dev server) to call the API.
func (s *Server) WithCORS(origin string) *Server {
	s.origin = origin
	return s
}

func (s *Server) Handler() http.Handler { return s.cors(s.routes()) }

func (s *Server) cors(next http.Handler) http.Handler {
	if s.origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == s.origin {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", s.origin)
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type")
			h.Set("Access-Control-Expose-Headers", "Location")
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/health", s.getHealth)

	m.HandleFunc("POST /api/suites", s.createSuite)
	m.HandleFunc("GET /api/suites", s.listSuites)
	m.HandleFunc("GET /api/suites/{suiteId}", s.getSuite)
	m.HandleFunc("GET /api/suites/{suiteId}/source", s.getSuiteSource)
	m.HandleFunc("GET /api/suites/{suiteId}/scenarios/{scenarioKey}", s.getSuiteScenario)

	m.HandleFunc("POST /api/evaluations", s.createEvaluation)
	m.HandleFunc("GET /api/evaluations", s.listEvaluations)
	m.HandleFunc("GET /api/evaluations/{evaluationId}", s.getEvaluation)
	m.HandleFunc("GET /api/evaluations/{evaluationId}/scenarios", s.listScenarioResults)
	m.HandleFunc("GET /api/evaluations/{evaluationId}/scenarios/{scenarioKey}", s.getScenarioResult)
	m.HandleFunc("GET /api/evaluations/{evaluationId}/scenarios/{scenarioKey}/turns/{turnIndex}", s.getTurnResult)
	m.HandleFunc("GET /api/evaluations/{evaluationId}/scenarios/{scenarioKey}/turns/{turnIndex}/annotations", s.listAnnotations)
	m.HandleFunc("POST /api/evaluations/{evaluationId}/scenarios/{scenarioKey}/turns/{turnIndex}/annotations", s.createAnnotation)

	m.HandleFunc("POST /api/evaluations/{evaluationId}/analysis", s.analyzeFailed)
	m.HandleFunc("GET /api/evaluations/{evaluationId}/scenarios/{scenarioKey}/turns/{turnIndex}/analysis", s.listAnalyses)
	m.HandleFunc("POST /api/evaluations/{evaluationId}/scenarios/{scenarioKey}/turns/{turnIndex}/analysis", s.analyzeTurn)

	m.HandleFunc("GET /api/evaluations/{evaluationId}/comparison", s.compareEvaluations)
	m.HandleFunc("GET /api/evaluations/{evaluationId}/scenarios/{scenarioKey}/comparison", s.compareScenarios)
	m.HandleFunc("GET /api/evaluations/{evaluationId}/scenarios/{scenarioKey}/turns/{turnIndex}/comparison", s.compareTurns)
	return m
}

// ---------------------------------------------------------------- errors and helpers

type apiError struct {
	status  int
	msg     string
	details []string
}

func (e *apiError) Error() string { return e.msg }

func badRequest(msg string, details ...string) error {
	return &apiError{http.StatusBadRequest, msg, details}
}
func notFound(what string) error { return &apiError{http.StatusNotFound, what + " not found", nil} }

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apiError
	switch {
	case errors.As(err, &ae):
	case errors.Is(err, store.ErrNotFound):
		ae = &apiError{status: http.StatusNotFound, msg: "not found"}
	default:
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		ae = &apiError{status: http.StatusInternalServerError, msg: "internal error"}
	}
	writeJSON(w, ae.status, map[string]any{"error": ae.msg, "details": ae.details})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, badRequest("invalid " + name)
	}
	return id, nil
}

// ---------------------------------------------------------------- health

func (s *Server) getHealth(w http.ResponseWriter, r *http.Request) {
	status, checks := http.StatusOK, map[string]string{}
	for name, check := range s.health {
		switch err := check.Fn(r.Context()); {
		case err == nil:
			checks[name] = "ok"
		case check.Optional:
			checks[name] = "warning: " + err.Error()
		default:
			checks[name], status = err.Error(), http.StatusServiceUnavailable
		}
	}
	writeJSON(w, status, map[string]any{"status": http.StatusText(status), "checks": checks})
}

// ---------------------------------------------------------------- suites

func (s *Server) createSuite(w http.ResponseWriter, r *http.Request) {
	src, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxBody))
	if err != nil {
		s.fail(w, r, badRequest("request body too large or unreadable"))
		return
	}
	parsed, err := suite.Parse(src, s.suites)
	if err != nil {
		var verr suite.ValidationError
		if errors.As(err, &verr) {
			s.fail(w, r, badRequest("invalid suite", verr...))
		} else {
			s.fail(w, r, badRequest(err.Error()))
		}
		return
	}
	created, isNew, err := s.store.CreateSuite(r.Context(), parsed)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	created.Source = ""
	status := http.StatusOK
	if isNew {
		status = http.StatusCreated
	}
	w.Header().Set("Location", fmt.Sprintf("/api/suites/%d", created.ID))
	writeJSON(w, status, created)
}

func (s *Server) listSuites(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListSuites(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if list == nil {
		list = []store.SuiteSummary{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) loadSuite(r *http.Request) (domain.Suite, error) {
	id, err := pathID(r, "suiteId")
	if err != nil {
		return domain.Suite{}, err
	}
	su, err := s.store.GetSuite(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		return su, notFound("suite")
	}
	return su, err
}

func (s *Server) getSuite(w http.ResponseWriter, r *http.Request) {
	su, err := s.loadSuite(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	su.Source = ""
	writeJSON(w, http.StatusOK, su)
}

func (s *Server) getSuiteSource(w http.ResponseWriter, r *http.Request) {
	su, err := s.loadSuite(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = io.WriteString(w, su.Source)
}

func (s *Server) getSuiteScenario(w http.ResponseWriter, r *http.Request) {
	su, err := s.loadSuite(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, sc := range su.Scenarios {
		if sc.Key == r.PathValue("scenarioKey") {
			writeJSON(w, http.StatusOK, sc)
			return
		}
	}
	s.fail(w, r, notFound("scenario"))
}

// ---------------------------------------------------------------- evaluations

type createEvaluationRequest struct {
	SuiteID     int64           `json:"suiteId"`
	Label       string          `json:"label"`
	AgentConfig json.RawMessage `json:"agentConfig"`
	Concurrency int             `json:"concurrency"`
}

func (s *Server) createEvaluation(w http.ResponseWriter, r *http.Request) {
	var req createEvaluationRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil {
		s.fail(w, r, badRequest("invalid body: "+err.Error()))
		return
	}
	if req.Concurrency == 0 {
		req.Concurrency = 2
	}
	if req.Concurrency < 1 || req.Concurrency > 20 {
		s.fail(w, r, badRequest("concurrency must be between 1 and 20"))
		return
	}
	if len(req.AgentConfig) > 0 && !strings.HasPrefix(strings.TrimSpace(string(req.AgentConfig)), "{") && strings.TrimSpace(string(req.AgentConfig)) != "null" {
		s.fail(w, r, badRequest("agentConfig must be a JSON object"))
		return
	}
	if _, err := s.store.GetSuite(r.Context(), req.SuiteID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			err = badRequest("suite not found")
		}
		s.fail(w, r, err)
		return
	}
	ev, err := s.store.CreateEvaluation(r.Context(), domain.Evaluation{SuiteID: req.SuiteID, Label: req.Label, AgentConfig: req.AgentConfig, Concurrency: req.Concurrency})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.runner.Submit(r.Context(), ev.ID); err != nil {
		s.fail(w, r, err) // stays queued in the database; recovered on restart
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/evaluations/%d", ev.ID))
	writeJSON(w, http.StatusAccepted, ev)
}

type evaluationView struct {
	Evaluation store.EvaluationSummary `json:"evaluation"`
	Summary    report.Summary          `json:"summary"`
}

func (s *Server) listEvaluations(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListEvaluations(r.Context(), r.URL.Query().Get("suite"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]evaluationView, 0, len(list))
	for _, ev := range list {
		rep, err := report.Load(r.Context(), s.store, ev.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out = append(out, evaluationView{rep.Evaluation, rep.Summary})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) loadReport(r *http.Request, param string) (report.EvaluationReport, error) {
	var id int64
	var err error
	if param == "with" {
		id, err = strconv.ParseInt(r.URL.Query().Get("with"), 10, 64)
		if err != nil {
			return report.EvaluationReport{}, badRequest("query parameter 'with' must be an evaluation id")
		}
	} else if id, err = pathID(r, param); err != nil {
		return report.EvaluationReport{}, err
	}
	rep, err := report.Load(r.Context(), s.store, id)
	if errors.Is(err, store.ErrNotFound) {
		return rep, notFound(fmt.Sprintf("evaluation %d", id))
	}
	return rep, err
}

func (s *Server) loadScenario(r *http.Request) (*report.ScenarioReport, error) {
	rep, err := s.loadReport(r, "evaluationId")
	if err != nil {
		return nil, err
	}
	sc := rep.Scenario(r.PathValue("scenarioKey"))
	if sc == nil {
		return nil, notFound("scenario")
	}
	return sc, nil
}

func (s *Server) loadTurn(r *http.Request) (*report.TurnReport, error) {
	sc, err := s.loadScenario(r)
	if err != nil {
		return nil, err
	}
	idx, err := strconv.Atoi(r.PathValue("turnIndex"))
	if err != nil {
		return nil, badRequest("invalid turnIndex")
	}
	t := sc.Turn(idx)
	if t == nil {
		return nil, notFound("turn")
	}
	return t, nil
}

func (s *Server) getEvaluation(w http.ResponseWriter, r *http.Request) {
	rep, err := s.loadReport(r, "evaluationId")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, evaluationView{rep.Evaluation, rep.Summary})
}

func (s *Server) listScenarioResults(w http.ResponseWriter, r *http.Request) {
	rep, err := s.loadReport(r, "evaluationId")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep.Scenarios)
}

func (s *Server) getScenarioResult(w http.ResponseWriter, r *http.Request) {
	sc, err := s.loadScenario(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

// withSpans adds the full spans (documents included) to a turn detail.
func (s *Server) withSpans(ctx context.Context, t *report.TurnReport) error {
	if t == nil || t.TurnResultID == 0 {
		return nil
	}
	spans, err := s.store.ListSpans(ctx, t.TurnResultID)
	t.Spans = spans
	return err
}

func (s *Server) getTurnResult(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadTurn(r)
	if err == nil {
		err = s.withSpans(r.Context(), t)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ---------------------------------------------------------------- annotations

func (s *Server) listAnnotations(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadTurn(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t.Annotations)
}

func (s *Server) createAnnotation(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadTurn(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if t.TurnResultID == 0 {
		s.fail(w, r, &apiError{status: http.StatusConflict, msg: "turn was not executed in this evaluation"})
		return
	}
	var a domain.Annotation
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(&a); err != nil {
		s.fail(w, r, badRequest("invalid body: "+err.Error()))
		return
	}
	if strings.TrimSpace(a.Body) == "" {
		s.fail(w, r, badRequest("body is required"))
		return
	}
	if strings.TrimSpace(a.Author) == "" {
		a.Author = "anonymous"
	}
	a.ID = 0
	created, err := s.store.AddAnnotation(r.Context(), t.TurnResultID, a)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// ---------------------------------------------------------------- analysis

func (s *Server) requireAnalyst(w http.ResponseWriter, r *http.Request) bool {
	if s.analyst == nil {
		s.fail(w, r, &apiError{status: http.StatusServiceUnavailable, msg: "analysis is not configured"})
		return false
	}
	return true
}

func (s *Server) analyzeTurn(w http.ResponseWriter, r *http.Request) {
	if !s.requireAnalyst(w, r) {
		return
	}
	id, err := pathID(r, "evaluationId")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	idx, err := strconv.Atoi(r.PathValue("turnIndex"))
	if err != nil {
		s.fail(w, r, badRequest("invalid turnIndex"))
		return
	}
	a, err := s.analyst.AnalyzeTurn(r.Context(), id, r.PathValue("scenarioKey"), idx)
	switch {
	case errors.Is(err, analysis.ErrNotFound), errors.Is(err, store.ErrNotFound):
		s.fail(w, r, notFound("turn"))
	case errors.Is(err, analysis.ErrNotExecuted):
		s.fail(w, r, &apiError{status: http.StatusConflict, msg: err.Error()})
	case err != nil:
		s.log.Warn("analysis failed", "err", err)
		s.fail(w, r, &apiError{status: http.StatusBadGateway, msg: err.Error()})
	default:
		writeJSON(w, http.StatusCreated, a)
	}
}

func (s *Server) listAnalyses(w http.ResponseWriter, r *http.Request) {
	t, err := s.loadTurn(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list := []domain.Analysis{}
	if t.TurnResultID != 0 {
		if list, err = s.store.ListAnalyses(r.Context(), t.TurnResultID); err != nil {
			s.fail(w, r, err)
			return
		}
		if list == nil {
			list = []domain.Analysis{}
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) analyzeFailed(w http.ResponseWriter, r *http.Request) {
	if !s.requireAnalyst(w, r) {
		return
	}
	if r.URL.Query().Get("only") != "failed" {
		s.fail(w, r, badRequest("only=failed is the supported batch analysis"))
		return
	}
	id, err := pathID(r, "evaluationId")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	n, err := s.analyst.AnalyzeFailed(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		err = notFound("evaluation")
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": n})
}

// ---------------------------------------------------------------- comparisons

func (s *Server) compareEvaluations(w http.ResponseWriter, r *http.Request) {
	target, err := s.loadReport(r, "evaluationId")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	base, err := s.loadReport(r, "with")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, compare.Evaluations(base, target))
}

func (s *Server) compareScenarios(w http.ResponseWriter, r *http.Request) {
	target, err := s.loadReport(r, "evaluationId")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	base, err := s.loadReport(r, "with")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	key := r.PathValue("scenarioKey")
	b, t := base.Scenario(key), target.Scenario(key)
	if b == nil && t == nil {
		s.fail(w, r, notFound("scenario"))
		return
	}
	writeJSON(w, http.StatusOK, compare.Scenarios(b, t))
}

func (s *Server) compareTurns(w http.ResponseWriter, r *http.Request) {
	target, err := s.loadReport(r, "evaluationId")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	base, err := s.loadReport(r, "with")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	idx, err := strconv.Atoi(r.PathValue("turnIndex"))
	if err != nil {
		s.fail(w, r, badRequest("invalid turnIndex"))
		return
	}
	key := r.PathValue("scenarioKey")
	b, t := base.Scenario(key).Turn(idx), target.Scenario(key).Turn(idx)
	if b == nil && t == nil {
		s.fail(w, r, notFound("turn"))
		return
	}
	if err := s.withSpans(r.Context(), b); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.withSpans(r.Context(), t); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, compare.Turns(b, t))
}
