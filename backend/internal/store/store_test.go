package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
)

var ctx = context.Background()

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func suiteFixture(hash string) domain.Suite {
	turn := func(i int, q string) domain.Turn {
		return domain.Turn{Index: i, Question: q, ContentHash: q + hash,
			Criteria: []domain.Criterion{{Type: "regex", Params: json.RawMessage(`{"pattern":"x"}`)}}}
	}
	return domain.Suite{
		Name: "Synthetic", ContentHash: hash, MinTurns: 3, Source: "name: Synthetic",
		Scenarios: []domain.Scenario{
			{Key: "a", Name: "A", Tags: []string{"t"}, Metadata: map[string]any{"k": "v"}, Turns: []domain.Turn{turn(1, "q1"), turn(2, "q2"), turn(3, "q3")}},
			{Key: "b", Name: "B", Turns: []domain.Turn{turn(1, "p1"), turn(2, "p2"), turn(3, "p3")}},
		},
	}
}

func TestSuiteVersioning(t *testing.T) {
	s := open(t)
	v1, created, err := s.CreateSuite(ctx, suiteFixture("h1"))
	if err != nil || !created || v1.Version != 1 || v1.ID == 0 {
		t.Fatalf("v1: %+v created=%v err=%v", v1, created, err)
	}
	same, created, _ := s.CreateSuite(ctx, suiteFixture("h1"))
	if created || same.ID != v1.ID {
		t.Fatal("re-importing identical content must return the existing version")
	}
	v2, created, _ := s.CreateSuite(ctx, suiteFixture("h2"))
	if !created || v2.Version != 2 {
		t.Fatalf("changed content must create version 2, got %+v", v2)
	}

	got, err := s.GetSuite(ctx, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	sc := got.Scenarios[0]
	if got.Source != "name: Synthetic" || len(got.Scenarios) != 2 || sc.Key != "a" || sc.Metadata["k"] != "v" ||
		len(sc.Turns) != 3 || sc.Turns[2].Index != 3 || sc.Turns[0].Criteria[0].ID == 0 || string(sc.Turns[0].Criteria[0].Params) != `{"pattern":"x"}` {
		t.Fatalf("suite tree not persisted: %+v", got)
	}
	list, _ := s.ListSuites(ctx)
	if len(list) != 2 || list[0].ScenarioCount != 2 {
		t.Fatalf("ListSuites = %+v", list)
	}
	if _, err := s.GetSuite(ctx, 999); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestEvaluationLifecycleAndResults(t *testing.T) {
	s := open(t)
	suite, _, _ := s.CreateSuite(ctx, suiteFixture("h1"))
	ev, err := s.CreateEvaluation(ctx, domain.Evaluation{SuiteID: suite.ID, Label: "baseline", AgentConfig: json.RawMessage(`{"promptId":"p"}`), Concurrency: 2})
	if err != nil || ev.Status != domain.EvaluationQueued {
		t.Fatalf("CreateEvaluation: %+v %v", ev, err)
	}
	srs, _ := s.ListScenarioResults(ctx, ev.ID)
	if len(srs) != 2 || srs[0].Status != domain.ScenarioPending {
		t.Fatalf("scenario results must be created pending: %+v", srs)
	}
	ids, _ := s.UnfinishedEvaluations(ctx)
	if len(ids) != 1 || ids[0] != ev.ID {
		t.Fatalf("UnfinishedEvaluations = %v", ids)
	}

	// Turn result with spans, criteria and annotations.
	sr := srs[0]
	tr := domain.TurnResult{ScenarioResultID: sr.ID, TurnID: suite.Scenarios[0].Turns[0].ID, Index: 1, IdempotencyKey: "k1", Status: domain.TurnRunning}
	tr.ID, err = s.InsertTurnResult(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	tr.Status, tr.ResponseText, tr.Passed, tr.Tokens = domain.TurnCompleted, "hola", true, 5
	tr.ResponseRaw = json.RawMessage(`{"answer":"hola"}`)
	if err := s.UpdateTurnResult(ctx, tr); err != nil {
		t.Fatal(err)
	}
	spans := []domain.Span{{Seq: 1, Kind: "tool", Name: "ragSearch", Documents: []domain.Document{{Rank: 1, URL: "u"}}, Attributes: json.RawMessage(`{"arguments":{}}`)}}
	if err := s.SaveSpans(ctx, tr.ID, spans); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSpans(ctx, tr.ID, spans); err != nil { // replace on retry, no duplicates
		t.Fatal(err)
	}
	score := 0.5
	crit := []domain.CriterionResult{{CriterionID: suite.Scenarios[0].Turns[0].Criteria[0].ID, CriterionType: "regex", CriterionVersion: "1", Passed: true, Label: "pass", Score: &score, Evidence: []string{"x"}}}
	if err := s.SaveCriterionResults(ctx, tr.ID, crit); err != nil {
		t.Fatal(err)
	}
	ann, err := s.AddAnnotation(ctx, tr.ID, domain.Annotation{Author: "javi", Body: "rubric too strict", Tags: []string{"rubric_too_strict"}})
	if err != nil || ann.ID == 0 || ann.CreatedAt.IsZero() {
		t.Fatalf("AddAnnotation: %+v %v", ann, err)
	}

	if _, err := s.AddAnalysis(ctx, tr.ID, domain.Analysis{Model: "m", Verdict: "real_failure", Confidence: 0.4, RootCause: "agent"}); err != nil {
		t.Fatal(err)
	}
	second, _ := s.AddAnalysis(ctx, tr.ID, domain.Analysis{Model: "m", Verdict: "false_positive", Confidence: 0.9, RootCause: "rubric", Recommendation: "relax"})
	history, _ := s.ListAnalyses(ctx, tr.ID)
	latest, _ := s.LatestAnalysesByEvaluation(ctx, ev.ID)
	if len(history) != 2 || history[0].Verdict != "real_failure" || latest[tr.ID].ID != second.ID || latest[tr.ID].Recommendation != "relax" || second.CreatedAt.IsZero() {
		t.Fatalf("analyses: history=%+v latest=%+v", history, latest)
	}

	turns, _ := s.ListTurnResults(ctx, sr.ID)
	if len(turns) != 1 || turns[0].ResponseText != "hola" || !turns[0].Passed || string(turns[0].ResponseRaw) != `{"answer":"hola"}` {
		t.Fatalf("ListTurnResults = %+v", turns)
	}
	full, _ := s.ListSpans(ctx, tr.ID)
	if len(full) != 1 || full[0].Documents[0].URL != "u" {
		t.Fatalf("ListSpans = %+v", full)
	}
	light, _ := s.SpansByEvaluation(ctx, ev.ID)
	if len(light[tr.ID]) != 1 || light[tr.ID][0].Name != "ragSearch" || light[tr.ID][0].Documents != nil {
		t.Fatalf("SpansByEvaluation must be light: %+v", light)
	}
	crits, _ := s.CriterionResultsByEvaluation(ctx, ev.ID)
	if c := crits[tr.ID]; len(c) != 1 || *c[0].Score != 0.5 || c[0].Evidence[0] != "x" {
		t.Fatalf("CriterionResultsByEvaluation = %+v", crits)
	}
	anns, _ := s.AnnotationsByEvaluation(ctx, ev.ID)
	if len(anns[tr.ID]) != 1 || anns[tr.ID][0].Tags[0] != "rubric_too_strict" {
		t.Fatalf("AnnotationsByEvaluation = %+v", anns)
	}
	all, _ := s.TurnResultsByEvaluation(ctx, ev.ID)
	if len(all[sr.ID]) != 1 {
		t.Fatalf("TurnResultsByEvaluation = %+v", all)
	}

	// Scenario and evaluation updates.
	sr.Status, sr.ConversationRef, sr.FirstFailedTurn = domain.ScenarioFailed, "conv-1", 2
	if err := s.UpdateScenarioResult(ctx, sr); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEffectiveConfig(ctx, ev.ID, json.RawMessage(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	_ = s.SetEffectiveConfig(ctx, ev.ID, json.RawMessage(`{"a":2}`)) // first one wins
	if err := s.UpdateEvaluationStatus(ctx, ev.ID, domain.EvaluationRunning, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateEvaluationStatus(ctx, ev.ID, domain.EvaluationCompleted, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetEvaluation(ctx, ev.ID)
	if got.Status != domain.EvaluationCompleted || got.StartedAt == nil || got.FinishedAt == nil || string(got.EffectiveConfig) != `{"a":1}` {
		t.Fatalf("GetEvaluation = %+v", got)
	}
	srs, _ = s.ListScenarioResults(ctx, ev.ID)
	if srs[0].ConversationRef != "conv-1" || srs[0].FirstFailedTurn != 2 {
		t.Fatalf("scenario result not updated: %+v", srs[0])
	}
	if ids, _ := s.UnfinishedEvaluations(ctx); len(ids) != 0 {
		t.Fatalf("completed evaluation must not be unfinished: %v", ids)
	}
}

func TestListEvaluationsAcrossSuiteVersions(t *testing.T) {
	s := open(t)
	v1, _, _ := s.CreateSuite(ctx, suiteFixture("h1"))
	v2, _, _ := s.CreateSuite(ctx, suiteFixture("h2"))
	e1, _ := s.CreateEvaluation(ctx, domain.Evaluation{SuiteID: v1.ID, Label: "a", Concurrency: 1})
	e2, _ := s.CreateEvaluation(ctx, domain.Evaluation{SuiteID: v2.ID, Label: "b", Concurrency: 1})
	list, err := s.ListEvaluations(ctx, "Synthetic")
	if err != nil || len(list) != 2 || list[0].ID != e1.ID || list[1].ID != e2.ID || list[1].SuiteVersion != 2 {
		t.Fatalf("ListEvaluations = %+v %v", list, err)
	}
	if all, _ := s.ListEvaluations(ctx, ""); len(all) != 2 {
		t.Fatal("empty suite filter must list all")
	}
}
