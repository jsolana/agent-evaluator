// Package analysis asks an LLM for a second opinion on a turn's evaluation:
// is the failure real, a false positive (rubric or judge too strict), or a
// pass the judge should not have given? It gathers the conversation, traces,
// rubric, results and human annotations, and stores the advisory verdict.
package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jsolana/agent-evaluator/backend/internal/criteria"
	"github.com/jsolana/agent-evaluator/backend/internal/domain"
	"github.com/jsolana/agent-evaluator/backend/internal/report"
)

var (
	ErrNotFound    = errors.New("scenario or turn not found")
	ErrNotExecuted = errors.New("turn was not executed in this evaluation")
)

type CriterionOutcome struct {
	Type        string
	Params      json.RawMessage
	Passed      bool
	Explanation string
	Evidence    []string
	Error       string
}

type ToolCall struct {
	Name       string
	Arguments  json.RawMessage
	DurationMs int64
	Error      string
}

// Request is everything the analyzer sees about one turn.
type Request struct {
	History     []criteria.Exchange
	Question    string
	Answer      string
	TurnPassed  bool
	Criteria    []CriterionOutcome
	Tools       []ToolCall
	Documents   []domain.Document
	Annotations []domain.Annotation
}

type Analyzer interface {
	Analyze(ctx context.Context, r Request) (domain.Analysis, error)
}

type Store interface {
	report.Loader
	ListSpans(ctx context.Context, turnResultID int64) ([]domain.Span, error)
	AddAnalysis(ctx context.Context, turnResultID int64, a domain.Analysis) (domain.Analysis, error)
}

type Service struct {
	store    Store
	analyzer Analyzer
	log      *slog.Logger
	limit    chan struct{} // bounds concurrent background analyses
}

func New(st Store, a Analyzer, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: st, analyzer: a, log: log, limit: make(chan struct{}, 2)}
}

// AnalyzeTurn analyzes one executed turn synchronously and stores the result.
func (s *Service) AnalyzeTurn(ctx context.Context, evaluationID int64, key string, index int) (domain.Analysis, error) {
	rep, err := report.Load(ctx, s.store, evaluationID)
	if err != nil {
		return domain.Analysis{}, err
	}
	sc := rep.Scenario(key)
	if sc.Turn(index) == nil {
		return domain.Analysis{}, ErrNotFound
	}
	return s.analyze(ctx, sc, index)
}

// AnalyzeFailed queues an analysis of every failed turn without one yet and
// returns how many were queued. Work continues in the background.
func (s *Service) AnalyzeFailed(ctx context.Context, evaluationID int64) (int, error) {
	rep, err := report.Load(ctx, s.store, evaluationID)
	if err != nil {
		return 0, err
	}
	type job struct {
		sc    *report.ScenarioReport
		index int
	}
	var jobs []job
	for i := range rep.Scenarios {
		sc := &rep.Scenarios[i]
		for _, t := range sc.Turns {
			if t.Status == domain.TurnCompleted && !t.Passed && t.LatestAnalysis == nil {
				jobs = append(jobs, job{sc, t.Index})
			}
		}
	}
	for _, j := range jobs {
		go func(j job) {
			s.limit <- struct{}{}
			defer func() { <-s.limit }()
			if _, err := s.analyze(context.Background(), j.sc, j.index); err != nil {
				s.log.Warn("analysis failed", "evaluation", evaluationID, "scenario", j.sc.Key, "turn", j.index, "err", err)
			}
		}(j)
	}
	return len(jobs), nil
}

func (s *Service) analyze(ctx context.Context, sc *report.ScenarioReport, index int) (domain.Analysis, error) {
	t := sc.Turn(index)
	if t.TurnResultID == 0 || t.Status != domain.TurnCompleted {
		return domain.Analysis{}, ErrNotExecuted
	}
	spans, err := s.store.ListSpans(ctx, t.TurnResultID)
	if err != nil {
		return domain.Analysis{}, err
	}
	req := Request{Question: t.Question, Answer: t.Answer, TurnPassed: t.Passed, Annotations: t.Annotations}
	for _, prev := range sc.Turns[:index-1] {
		req.History = append(req.History, criteria.Exchange{Question: prev.Question, Answer: prev.Answer})
	}
	params := map[int64]json.RawMessage{}
	for _, c := range t.Criteria {
		params[c.ID] = c.Params
	}
	for _, r := range t.CriterionResults {
		req.Criteria = append(req.Criteria, CriterionOutcome{Type: r.CriterionType, Params: params[r.CriterionID],
			Passed: r.Passed, Explanation: r.Explanation, Evidence: r.Evidence, Error: r.Error})
	}
	for _, sp := range spans {
		req.Tools = append(req.Tools, ToolCall{Name: sp.Name, Arguments: sp.Arguments(), DurationMs: sp.DurationMs, Error: sp.Error})
		req.Documents = append(req.Documents, sp.Documents...)
	}
	a, err := s.analyzer.Analyze(ctx, req)
	if err != nil {
		return domain.Analysis{}, fmt.Errorf("analysis: %w", err)
	}
	return s.store.AddAnalysis(ctx, t.TurnResultID, a)
}
