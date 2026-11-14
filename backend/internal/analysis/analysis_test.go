package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/report"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/store"
)

// fakeStore serves a fixed report: scenario "s" with turn 1 passed and turn 2 failed.
type fakeStore struct {
	report.Loader
	mu    sync.Mutex
	saved map[int64][]domain.Analysis
}

func (f *fakeStore) GetEvaluation(context.Context, int64) (store.EvaluationSummary, error) {
	return store.EvaluationSummary{Evaluation: domain.Evaluation{ID: 1, SuiteID: 1}}, nil
}
func (f *fakeStore) GetSuite(context.Context, int64) (domain.Suite, error) {
	return domain.Suite{Scenarios: []domain.Scenario{{ID: 1, Key: "s", Turns: []domain.Turn{
		{ID: 1, Index: 1, Question: "q1", Criteria: []domain.Criterion{{ID: 1, Type: "regex", Params: json.RawMessage(`{"pattern":"a"}`)}}},
		{ID: 2, Index: 2, Question: "q2", Criteria: []domain.Criterion{{ID: 2, Type: "regex", Params: json.RawMessage(`{"pattern":"S/ 1,200"}`)}}},
		{ID: 3, Index: 3, Question: "q3", Criteria: []domain.Criterion{{ID: 3, Type: "regex", Params: json.RawMessage(`{"pattern":"c"}`)}}},
	}}}}, nil
}
func (f *fakeStore) ListScenarioResults(context.Context, int64) ([]domain.ScenarioResult, error) {
	return []domain.ScenarioResult{{ID: 1, ScenarioID: 1, Status: domain.ScenarioFailed, FirstFailedTurn: 2}}, nil
}
func (f *fakeStore) TurnResultsByEvaluation(context.Context, int64) (map[int64][]domain.TurnResult, error) {
	return map[int64][]domain.TurnResult{1: {
		{ID: 10, Index: 1, Status: domain.TurnCompleted, Passed: true, ResponseText: "a1"},
		{ID: 20, Index: 2, Status: domain.TurnCompleted, Passed: false, ResponseText: "S/ 1,500"},
	}}, nil
}
func (f *fakeStore) SpansByEvaluation(context.Context, int64) (map[int64][]domain.Span, error) {
	return nil, nil
}
func (f *fakeStore) CriterionResultsByEvaluation(context.Context, int64) (map[int64][]domain.CriterionResult, error) {
	return map[int64][]domain.CriterionResult{20: {{CriterionID: 2, CriterionType: "regex", Passed: false, Explanation: "pattern not found: S/ 1,200"}}}, nil
}
func (f *fakeStore) AnnotationsByEvaluation(context.Context, int64) (map[int64][]domain.Annotation, error) {
	return map[int64][]domain.Annotation{20: {{Body: "el corpus dice 1,500", Tags: []string{"rubric_too_strict"}}}}, nil
}
func (f *fakeStore) LatestAnalysesByEvaluation(context.Context, int64) (map[int64]domain.Analysis, error) {
	return nil, nil
}
func (f *fakeStore) ListSpans(_ context.Context, id int64) ([]domain.Span, error) {
	return []domain.Span{{Kind: "tool", Name: "ragSearch", DurationMs: 50, Attributes: json.RawMessage(`{"arguments":{"query":"cuota"}}`),
		Documents: []domain.Document{{Rank: 1, URL: "https://bcp/x"}}}}, nil
}
func (f *fakeStore) AddAnalysis(_ context.Context, id int64, a domain.Analysis) (domain.Analysis, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saved == nil {
		f.saved = map[int64][]domain.Analysis{}
	}
	a.ID = int64(len(f.saved) + 1)
	f.saved[id] = append(f.saved[id], a)
	return a, nil
}

type recordingAnalyzer struct {
	mu  sync.Mutex
	got []Request
	err error
}

func (r *recordingAnalyzer) Analyze(_ context.Context, req Request) (domain.Analysis, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, req)
	return domain.Analysis{Verdict: "false_positive", RootCause: "rubric", Confidence: 0.8}, r.err
}

func TestAnalyzeTurnSendsFullContextAndStoresTheResult(t *testing.T) {
	st, an := &fakeStore{}, &recordingAnalyzer{}
	s := New(st, an, nil)
	a, err := s.AnalyzeTurn(context.Background(), 1, "s", 2)
	if err != nil {
		t.Fatal(err)
	}
	if a.Verdict != "false_positive" || len(st.saved[20]) != 1 {
		t.Fatalf("analysis not stored: %+v %+v", a, st.saved)
	}
	r := an.got[0]
	if r.Question != "q2" || r.Answer != "S/ 1,500" || r.TurnPassed || len(r.History) != 1 || r.History[0].Answer != "a1" {
		t.Fatalf("conversation context: %+v", r)
	}
	if len(r.Criteria) != 1 || string(r.Criteria[0].Params) != `{"pattern":"S/ 1,200"}` || r.Criteria[0].Explanation == "" {
		t.Fatalf("rubric and results: %+v", r.Criteria)
	}
	if len(r.Tools) != 1 || r.Tools[0].Name != "ragSearch" || len(r.Documents) != 1 || len(r.Annotations) != 1 {
		t.Fatalf("traces and annotations: %+v", r)
	}
}

func TestAnalyzeTurnErrors(t *testing.T) {
	s := New(&fakeStore{}, &recordingAnalyzer{}, nil)
	if _, err := s.AnalyzeTurn(context.Background(), 1, "nope", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown scenario: %v", err)
	}
	if _, err := s.AnalyzeTurn(context.Background(), 1, "s", 3); !errors.Is(err, ErrNotExecuted) {
		t.Fatalf("turn 3 was not executed: %v", err)
	}
	bad := New(&fakeStore{}, &recordingAnalyzer{err: errors.New("llm down")}, nil)
	if _, err := bad.AnalyzeTurn(context.Background(), 1, "s", 2); err == nil {
		t.Fatal("analyzer errors must be returned")
	}
}

func TestAnalyzeFailedRunsInBackgroundOnlyOnFailedTurns(t *testing.T) {
	st, an := &fakeStore{}, &recordingAnalyzer{}
	n, err := New(st, an, nil).AnalyzeFailed(context.Background(), 1)
	if err != nil || n != 1 {
		t.Fatalf("queued %d, err %v", n, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		st.mu.Lock()
		done := len(st.saved[20]) == 1 && len(st.saved[10]) == 0
		st.mu.Unlock()
		if done {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("background analysis not stored: %+v", st.saved)
}
