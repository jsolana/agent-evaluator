package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/jsolana/agent-evaluator/backend/internal/domain"
)

// TestFeatures runs the acceptance criteria in features/*.feature against the
// real HTTP API (fake agent, in-memory store). Strict: an undefined or pending
// step fails the test.
func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("acceptance scenarios failed")
	}
}

// world is the state shared by the steps of one scenario.
type world struct {
	srv       *httptest.Server
	stop      func()
	status    int
	body      []byte
	suite     domain.Suite     // last suite returned by an import
	source    string           // YAML of the last import
	evals     map[string]int64 // evaluation label -> id
	analysis  domain.Analysis
	cmpBase   int64
	cmpTarget int64
}

func initializeScenario(sc *godog.ScenarioContext) {
	w := &world{evals: map[string]int64{}}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.stop != nil {
			w.stop()
		}
		return ctx, nil // the step error is already reported by godog
	})

	sc.Step(`^the evaluator is running with a fake agent$`, w.evaluatorRunning)
	sc.Step(`^I import the suite:$`, w.importSuite)
	sc.Step(`^I import the same suite again$`, w.importSameSuiteAgain)
	sc.Step(`^the import is rejected with the errors:$`, w.importRejected)
	sc.Step(`^no suite exists$`, w.noSuiteExists)
	sc.Step(`^suite "([^"]*)" version (\d+) is created with (\d+) scenarios$`, w.suiteVersionCreated)
	sc.Step(`^the existing version (\d+) is returned$`, w.existingVersionReturned)
	sc.Step(`^scenario "([^"]*)" of the suite has (\d+) turns$`, w.scenarioHasTurns)
	sc.Step(`^I evaluate the latest version of "([^"]*)" as "([^"]*)"$`, w.evaluateLatest)
	sc.Step(`^evaluation "([^"]*)" completes with (\d+) passed and (\d+) failed scenarios$`, w.evaluationCompletes)
	sc.Step(`^in "([^"]*)" scenario "([^"]*)" fails first at turn (\d+)$`, w.failsFirstAt)
	sc.Step(`^in "([^"]*)" the turns of scenario "([^"]*)" are:$`, w.turnsAre)
	sc.Step(`^in "([^"]*)" the trace of scenario "([^"]*)" is:$`, w.traceIs)
	sc.Step(`^in "([^"]*)" turn (\d+) of "([^"]*)" fails criterion "([^"]*)" explaining "([^"]*)"$`, w.failsCriterion)
	sc.Step(`^I annotate turn (\d+) of "([^"]*)" in "([^"]*)" as "([^"]*)":$`, w.annotate)
	sc.Step(`^turn (\d+) of "([^"]*)" in "([^"]*)" has (\d+) annotations? tagged "([^"]*)"$`, w.hasAnnotations)
	sc.Step(`^I analyze turn (\d+) of "([^"]*)" in "([^"]*)"$`, w.analyzeTurn)
	sc.Step(`^the analysis says "([^"]*)" because of the "([^"]*)"$`, w.analysisSays)
	sc.Step(`^turn (\d+) of "([^"]*)" in "([^"]*)" shows that latest analysis$`, w.showsLatestAnalysis)
	sc.Step(`^I analyze the failed turns of "([^"]*)"$`, w.analyzeFailed)
	sc.Step(`^(\d+) turns are queued for analysis$`, w.turnsQueued)
	sc.Step(`^turns 2, 3 of "([^"]*)" in "([^"]*)" get the analysis "([^"]*)" while turn 1 has none$`, w.batchAnalyzed)
	sc.Step(`^I create a new version of "([^"]*)" replacing "([^"]*)" with "([^"]*)"$`, w.newVersionReplacing)
	sc.Step(`^suite "([^"]*)" lists (\d+) evaluations across its versions$`, w.listsEvaluations)
	sc.Step(`^I compare "([^"]*)" with "([^"]*)"$`, w.compare)
	sc.Step(`^the scenario pass rate changes by ([-0-9.]+)$`, w.passRateChanges)
	sc.Step(`^the scenarios compare as:$`, w.scenariosCompareAs)
	sc.Step(`^the turns of scenario "([^"]*)" compare as:$`, w.turnsCompareAs)
	sc.Step(`^turn (\d+) of "([^"]*)" shows the baseline annotation and its tool calls next to the passing target$`, w.turnComparisonDetail)
}

// ---------------------------------------------------------------- HTTP helpers

func (w *world) do(method, path, body string) error {
	req, err := http.NewRequest(method, w.srv.URL+path, bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	w.status = resp.StatusCode
	w.body, err = io.ReadAll(resp.Body)
	return err
}

// get calls GET path, expects 200 and decodes into out.
func (w *world) get(path string, out any) error {
	if err := w.do(http.MethodGet, path, ""); err != nil {
		return err
	}
	if w.status != http.StatusOK {
		return fmt.Errorf("GET %s: status %d: %s", path, w.status, w.body)
	}
	return json.Unmarshal(w.body, out)
}

func (w *world) eval(label string) (int64, error) {
	id, ok := w.evals[label]
	if !ok {
		return 0, fmt.Errorf("no evaluation labelled %q", label)
	}
	return id, nil
}

func (w *world) turnPath(label, key string, turn int) (string, error) {
	id, err := w.eval(label)
	return fmt.Sprintf("/api/evaluations/%d/scenarios/%s/turns/%d", id, key, turn), err
}

// ---------------------------------------------------------------- steps: suites

func (w *world) evaluatorRunning() error {
	srv, stop, err := startServer()
	w.srv, w.stop = srv, stop
	return err
}

func (w *world) importSuite(doc *godog.DocString) error {
	w.source = doc.Content
	if err := w.do(http.MethodPost, "/api/suites", doc.Content); err != nil {
		return err
	}
	if w.status == http.StatusCreated || w.status == http.StatusOK {
		return json.Unmarshal(w.body, &w.suite)
	}
	return nil // a rejection is asserted by the next step
}

func (w *world) importSameSuiteAgain() error {
	return w.importSuite(&godog.DocString{Content: w.source})
}

func (w *world) importRejected(table *godog.Table) error {
	if w.status != http.StatusBadRequest {
		return fmt.Errorf("want 400, got %d: %s", w.status, w.body)
	}
	var resp struct{ Details []string }
	if err := json.Unmarshal(w.body, &resp); err != nil {
		return err
	}
	var want []string
	for _, row := range table.Rows {
		want = append(want, row.Cells[0].Value)
	}
	if strings.Join(resp.Details, "\n") != strings.Join(want, "\n") {
		return fmt.Errorf("errors:\n%s\nwant:\n%s", strings.Join(resp.Details, "\n"), strings.Join(want, "\n"))
	}
	return nil
}

func (w *world) noSuiteExists() error {
	var list []any
	if err := w.get("/api/suites", &list); err != nil {
		return err
	}
	if len(list) != 0 {
		return fmt.Errorf("want no suites, got %d", len(list))
	}
	return nil
}

func (w *world) suiteVersionCreated(name string, version, scenarios int) error {
	if w.status != http.StatusCreated {
		return fmt.Errorf("want 201 Created, got %d: %s", w.status, w.body)
	}
	if w.suite.Name != name || w.suite.Version != version || len(w.suite.Scenarios) != scenarios {
		return fmt.Errorf("got suite %q v%d with %d scenarios", w.suite.Name, w.suite.Version, len(w.suite.Scenarios))
	}
	return nil
}

func (w *world) existingVersionReturned(version int) error {
	if w.status != http.StatusOK || w.suite.Version != version {
		return fmt.Errorf("want 200 with version %d, got %d with version %d", version, w.status, w.suite.Version)
	}
	return nil
}

func (w *world) scenarioHasTurns(key string, turns int) error {
	var sc domain.Scenario
	if err := w.get(fmt.Sprintf("/api/suites/%d/scenarios/%s", w.suite.ID, key), &sc); err != nil {
		return err
	}
	if len(sc.Turns) != turns {
		return fmt.Errorf("scenario %s has %d turns", key, len(sc.Turns))
	}
	for _, t := range sc.Turns {
		if len(t.Criteria) == 0 {
			return fmt.Errorf("turn %d has no criteria", t.Index)
		}
	}
	return nil
}

func (w *world) newVersionReplacing(name, old, replacement string) error {
	var source string
	if err := w.do(http.MethodGet, fmt.Sprintf("/api/suites/%d/source", w.suite.ID), ""); err != nil {
		return err
	}
	source = string(w.body)
	if w.suite.Name != name || !strings.Contains(source, old) {
		return fmt.Errorf("suite %q source does not contain %q", w.suite.Name, old)
	}
	return w.importSuite(&godog.DocString{Content: strings.Replace(source, old, replacement, 1)})
}

// ---------------------------------------------------------------- steps: evaluations

func (w *world) evaluateLatest(name, label string) error {
	if w.suite.Name != name {
		return fmt.Errorf("last imported suite is %q", w.suite.Name)
	}
	body := fmt.Sprintf(`{"suiteId":%d,"label":%q,"concurrency":2}`, w.suite.ID, label)
	if err := w.do(http.MethodPost, "/api/evaluations", body); err != nil {
		return err
	}
	if w.status != http.StatusAccepted {
		return fmt.Errorf("want 202 Accepted, got %d: %s", w.status, w.body)
	}
	var ev domain.Evaluation
	if err := json.Unmarshal(w.body, &ev); err != nil {
		return err
	}
	w.evals[label] = ev.ID
	return nil
}

func (w *world) evaluationCompletes(label string, passed, failed int) error {
	id, err := w.eval(label)
	if err != nil {
		return err
	}
	var v evaluationView
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if err := w.get(fmt.Sprintf("/api/evaluations/%d", id), &v); err != nil {
			return err
		}
		if v.Evaluation.Status == domain.EvaluationCompleted {
			if s := v.Summary.Scenarios; s.Passed != passed || s.Failed != failed || s.ExecutionError != 0 {
				return fmt.Errorf("summary: %+v", s)
			}
			return nil
		}
	}
	return fmt.Errorf("evaluation %q did not complete (status %s)", label, v.Evaluation.Status)
}

type turnView struct {
	Index            int
	Answer           string
	Passed           bool
	CriterionResults []domain.CriterionResult
	Annotations      []domain.Annotation
	Spans            []domain.Span
	Tools            struct {
		Sequence []string
		Counts   map[string]int
	}
}

func (w *world) failsFirstAt(label, key string, turn int) error {
	id, err := w.eval(label)
	if err != nil {
		return err
	}
	var sc struct {
		Status          string
		FirstFailedTurn int
	}
	if err := w.get(fmt.Sprintf("/api/evaluations/%d/scenarios/%s", id, key), &sc); err != nil {
		return err
	}
	if sc.Status != "failed" || sc.FirstFailedTurn != turn {
		return fmt.Errorf("scenario %s: status %s, first failed turn %d", key, sc.Status, sc.FirstFailedTurn)
	}
	return nil
}

func (w *world) turnsAre(label, key string, table *godog.Table) error {
	id, err := w.eval(label)
	if err != nil {
		return err
	}
	var sc struct{ Turns []turnView }
	if err := w.get(fmt.Sprintf("/api/evaluations/%d/scenarios/%s", id, key), &sc); err != nil {
		return err
	}
	for _, row := range table.Rows[1:] {
		n, _ := strconv.Atoi(row.Cells[0].Value)
		if n < 1 || n > len(sc.Turns) {
			return fmt.Errorf("turn %d not found", n)
		}
		t := sc.Turns[n-1]
		wantPassed, wantAnswer, wantTools := row.Cells[1].Value == "yes", row.Cells[2].Value, row.Cells[3].Value
		if t.Passed != wantPassed || t.Answer != wantAnswer || strings.Join(t.Tools.Sequence, ", ") != wantTools {
			return fmt.Errorf("turn %d: passed=%v answer=%q tools=%q", n, t.Passed, t.Answer, strings.Join(t.Tools.Sequence, ", "))
		}
	}
	return nil
}

func (w *world) failsCriterion(label string, turn int, key, typ, explanation string) error {
	path, err := w.turnPath(label, key, turn)
	if err != nil {
		return err
	}
	var t turnView
	if err := w.get(path, &t); err != nil {
		return err
	}
	for _, c := range t.CriterionResults {
		if c.CriterionType == typ && !c.Passed && strings.Contains(c.Explanation, explanation) {
			return nil
		}
	}
	return fmt.Errorf("no failed %s criterion explaining %q in %+v", typ, explanation, t.CriterionResults)
}

func (w *world) annotate(turn int, key, label, tag string, doc *godog.DocString) error {
	path, err := w.turnPath(label, key, turn)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"author": "reviewer", "body": strings.TrimSpace(doc.Content), "tags": []string{tag}})
	if err := w.do(http.MethodPost, path+"/annotations", string(body)); err != nil {
		return err
	}
	if w.status != http.StatusCreated {
		return fmt.Errorf("want 201, got %d: %s", w.status, w.body)
	}
	return nil
}

func (w *world) hasAnnotations(turn int, key, label string, count int, tag string) error {
	path, err := w.turnPath(label, key, turn)
	if err != nil {
		return err
	}
	var anns []domain.Annotation
	if err := w.get(path+"/annotations", &anns); err != nil {
		return err
	}
	if len(anns) != count {
		return fmt.Errorf("want %d annotations, got %d", count, len(anns))
	}
	for _, a := range anns {
		if len(a.Tags) == 0 || a.Tags[0] != tag || a.Body == "" {
			return fmt.Errorf("annotation %+v", a)
		}
	}
	return nil
}

func (w *world) listsEvaluations(name string, count int) error {
	var list []evaluationView
	if err := w.get("/api/evaluations?suite="+name, &list); err != nil {
		return err
	}
	if len(list) != count {
		return fmt.Errorf("want %d evaluations, got %d", count, len(list))
	}
	if list[0].Evaluation.SuiteVersion != 1 || list[len(list)-1].Evaluation.SuiteVersion != 2 {
		return fmt.Errorf("evaluations must span versions 1 and 2: %+v", list)
	}
	return nil
}

// ---------------------------------------------------------------- steps: comparison

func (w *world) compare(target, base string) error {
	var err error
	if w.cmpTarget, err = w.eval(target); err != nil {
		return err
	}
	w.cmpBase, err = w.eval(base)
	return err
}

func (w *world) comparison(path string, out any) error {
	return w.get(fmt.Sprintf("/api/evaluations/%d%s/comparison?with=%d", w.cmpTarget, path, w.cmpBase), out)
}

func (w *world) passRateChanges(delta float64) error {
	var c struct{ DeltaScenarioPassRate float64 }
	if err := w.comparison("", &c); err != nil {
		return err
	}
	if c.DeltaScenarioPassRate != delta {
		return fmt.Errorf("delta = %v", c.DeltaScenarioPassRate)
	}
	return nil
}

func (w *world) scenariosCompareAs(table *godog.Table) error {
	var c struct {
		Scenarios []struct{ Key, Change, Outcome string }
	}
	if err := w.comparison("", &c); err != nil {
		return err
	}
	got := map[string]string{}
	for _, s := range c.Scenarios {
		got[s.Key] = s.Change + "/" + s.Outcome
	}
	for _, row := range table.Rows[1:] {
		key, want := row.Cells[0].Value, row.Cells[1].Value+"/"+row.Cells[2].Value
		if got[key] != want {
			return fmt.Errorf("scenario %s: got %s, want %s", key, got[key], want)
		}
	}
	return nil
}

func (w *world) turnsCompareAs(key string, table *godog.Table) error {
	var c struct {
		Turns []struct {
			Index           int
			Change, Outcome string
		}
	}
	if err := w.comparison("/scenarios/"+key, &c); err != nil {
		return err
	}
	for _, row := range table.Rows[1:] {
		n, _ := strconv.Atoi(row.Cells[0].Value)
		if n < 1 || n > len(c.Turns) {
			return fmt.Errorf("turn %d not compared", n)
		}
		got, want := c.Turns[n-1].Change+"/"+c.Turns[n-1].Outcome, row.Cells[1].Value+"/"+row.Cells[2].Value
		if got != want {
			return fmt.Errorf("turn %d: got %s, want %s", n, got, want)
		}
	}
	return nil
}

func (w *world) turnComparisonDetail(turn int, key string) error {
	var c struct{ Base, Target turnView }
	if err := w.comparison(fmt.Sprintf("/scenarios/%s/turns/%d", key, turn), &c); err != nil {
		return err
	}
	if len(c.Base.Annotations) != 1 || len(c.Base.Spans) != 2 || c.Base.Passed || !c.Target.Passed {
		return fmt.Errorf("base: %d annotations, %d spans, passed=%v; target passed=%v",
			len(c.Base.Annotations), len(c.Base.Spans), c.Base.Passed, c.Target.Passed)
	}
	return nil
}

// traceIs checks the conversation trace (light spans per turn) served by the
// scenario endpoint in a single call.
func (w *world) traceIs(label, key string, table *godog.Table) error {
	id, err := w.eval(label)
	if err != nil {
		return err
	}
	var sc struct {
		Turns []struct {
			DurationMs int64
			Spans      []domain.Span
		}
	}
	if err := w.get(fmt.Sprintf("/api/evaluations/%d/scenarios/%s", id, key), &sc); err != nil {
		return err
	}
	for _, row := range table.Rows[1:] {
		n, _ := strconv.Atoi(row.Cells[0].Value)
		t := sc.Turns[n-1]
		var names []string
		var toolMs int64
		for _, s := range t.Spans {
			names = append(names, s.Name)
			toolMs += s.DurationMs
		}
		got := fmt.Sprintf("%s|%d|%d", strings.Join(names, ", "), toolMs, t.DurationMs)
		want := row.Cells[1].Value + "|" + row.Cells[2].Value + "|" + row.Cells[3].Value
		if got != want {
			return fmt.Errorf("turn %d trace: got %s, want %s", n, got, want)
		}
	}
	return nil
}

// ---------------------------------------------------------------- steps: analysis

func (w *world) analyzeTurn(turn int, key, label string) error {
	path, err := w.turnPath(label, key, turn)
	if err != nil {
		return err
	}
	if err := w.do(http.MethodPost, path+"/analysis", ""); err != nil {
		return err
	}
	if w.status != http.StatusCreated {
		return fmt.Errorf("want 201, got %d: %s", w.status, w.body)
	}
	return json.Unmarshal(w.body, &w.analysis)
}

func (w *world) analysisSays(verdict, cause string) error {
	if w.analysis.Verdict != verdict || w.analysis.RootCause != cause || w.analysis.Rationale == "" || w.analysis.ID == 0 {
		return fmt.Errorf("analysis = %+v", w.analysis)
	}
	return nil
}

func (w *world) showsLatestAnalysis(turn int, key, label string) error {
	id, err := w.eval(label)
	if err != nil {
		return err
	}
	var sc struct {
		Turns []struct{ LatestAnalysis *domain.Analysis }
	}
	if err := w.get(fmt.Sprintf("/api/evaluations/%d/scenarios/%s", id, key), &sc); err != nil {
		return err
	}
	if a := sc.Turns[turn-1].LatestAnalysis; a == nil || a.ID != w.analysis.ID {
		return fmt.Errorf("latest analysis = %+v, want id %d", a, w.analysis.ID)
	}
	path, _ := w.turnPath(label, key, turn)
	var history []domain.Analysis
	if err := w.get(path+"/analysis", &history); err != nil {
		return err
	}
	if len(history) != 1 {
		return fmt.Errorf("history has %d analyses", len(history))
	}
	return nil
}

func (w *world) analyzeFailed(label string) error {
	id, err := w.eval(label)
	if err != nil {
		return err
	}
	if err := w.do(http.MethodPost, fmt.Sprintf("/api/evaluations/%d/analysis?only=failed", id), ""); err != nil {
		return err
	}
	if w.status != http.StatusAccepted {
		return fmt.Errorf("want 202, got %d: %s", w.status, w.body)
	}
	return nil
}

func (w *world) turnsQueued(n int) error {
	var resp struct{ Queued int }
	if err := json.Unmarshal(w.body, &resp); err != nil || resp.Queued != n {
		return fmt.Errorf("queued = %s", w.body)
	}
	return nil
}

func (w *world) batchAnalyzed(key, label, verdict string) error {
	id, err := w.eval(label)
	if err != nil {
		return err
	}
	var sc struct {
		Turns []struct{ LatestAnalysis *domain.Analysis }
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if err := w.get(fmt.Sprintf("/api/evaluations/%d/scenarios/%s", id, key), &sc); err != nil {
			return err
		}
		t := sc.Turns
		if t[1].LatestAnalysis != nil && t[2].LatestAnalysis != nil {
			if t[0].LatestAnalysis != nil || t[1].LatestAnalysis.Verdict != verdict || t[2].LatestAnalysis.Verdict != verdict {
				return fmt.Errorf("analyses: %+v", t)
			}
			return nil
		}
	}
	return fmt.Errorf("background analyses did not arrive")
}
