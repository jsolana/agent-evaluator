package compare

import (
	"encoding/json"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/internal/domain"
	"github.com/jsolana/agent-evaluator/backend/internal/report"
	"github.com/jsolana/agent-evaluator/backend/internal/store"
)

func turn(i int, hash string, passed bool, tools ...string) report.TurnReport {
	spans := make([]domain.Span, len(tools))
	for k, n := range tools {
		spans[k] = domain.Span{Kind: "tool", Name: n}
	}
	return report.TurnReport{Index: i, ContentHash: hash, Status: domain.TurnCompleted, Passed: passed, Tokens: 10, Tools: report.ToolUsageOf(spans)}
}

func scenario(key string, status domain.ScenarioStatus, turns ...report.TurnReport) report.ScenarioReport {
	s := report.ScenarioReport{Key: key, Status: status, Turns: turns, Tools: report.ToolUsageOf(nil)}
	for _, t := range turns {
		s.TotalTokens += t.Tokens
		s.Tools = s.Tools.Merge(t.Tools)
	}
	return s
}

func evaluation(id int64, cfg string, scenarios ...report.ScenarioReport) report.EvaluationReport {
	ev := store.EvaluationSummary{}
	ev.ID, ev.EffectiveConfig = id, json.RawMessage(cfg)
	return report.EvaluationReport{Evaluation: ev, Scenarios: scenarios, Summary: report.Summarize(scenarios)}
}

func TestEvaluationComparison(t *testing.T) {
	base := evaluation(1, `{"promptId":"p1","config":{"topK":6}}`,
		scenario("regressed", domain.ScenarioPassed, turn(1, "a", true), turn(2, "b", true)),
		scenario("improved", domain.ScenarioFailed, turn(1, "c", false, "search")),
		scenario("rubric", domain.ScenarioFailed, turn(1, "d", false)),
		scenario("gone", domain.ScenarioPassed, turn(1, "e", true)),
		scenario("broken", domain.ScenarioPassed, turn(1, "f", true)),
	)
	target := evaluation(2, `{"promptId":"p2","config":{"topK":6}}`,
		scenario("regressed", domain.ScenarioFailed, turn(1, "a", true), turn(2, "b", false)),
		scenario("improved", domain.ScenarioPassed, turn(1, "c", true, "search", "getSource")),
		scenario("rubric", domain.ScenarioPassed, turn(1, "d2", true)),
		scenario("new", domain.ScenarioPassed, turn(1, "g", true)),
		scenario("broken", domain.ScenarioExecutionError, report.TurnReport{Index: 1, ContentHash: "f", Status: domain.TurnExecutionError}),
	)
	c := Evaluations(base, target)

	want := map[string]struct {
		change  Change
		outcome Outcome
	}{
		"regressed": {Unchanged, Regression},
		"improved":  {Unchanged, Improvement},
		"rubric":    {Changed, Improvement},
		"gone":      {Removed, NotComparable},
		"new":       {Added, NotComparable},
		"broken":    {Unchanged, NotComparable},
	}
	if len(c.Scenarios) != len(want) {
		t.Fatalf("got %d scenarios", len(c.Scenarios))
	}
	for _, s := range c.Scenarios {
		w := want[s.Key]
		if s.Change != w.change || s.Outcome != w.outcome {
			t.Errorf("%s: got %s/%s, want %s/%s", s.Key, s.Change, s.Outcome, w.change, w.outcome)
		}
	}
	if c.Scenarios[0].Key != "regressed" || c.Scenarios[len(c.Scenarios)-1].Key != "gone" {
		t.Fatalf("order: target scenarios first, removed ones last: %s ... %s", c.Scenarios[0].Key, c.Scenarios[len(c.Scenarios)-1].Key)
	}
	if c.Outcomes[Regression] != 1 || c.Outcomes[Improvement] != 2 || c.Changes[Added] != 1 {
		t.Fatalf("counts %+v %+v", c.Outcomes, c.Changes)
	}
	reg := find(c, "regressed")
	if reg.Base.TurnVerdicts[1] != "pass" || reg.Target.TurnVerdicts[1] != "fail" || reg.Target.FirstFailedTurn != 0 {
		t.Fatalf("turn verdicts %+v %+v", reg.Base, reg.Target)
	}
	imp := find(c, "improved")
	if imp.ToolDelta["getSource"] != 1 || imp.ToolDelta["search"] != 0 {
		t.Fatalf("tool delta %+v", imp.ToolDelta)
	}
	if len(c.ConfigDiff) != 1 || c.ConfigDiff[0].Path != "promptId" {
		t.Fatalf("config diff %+v", c.ConfigDiff)
	}
}

func find(c EvaluationComparison, key string) ScenarioDiff {
	for _, s := range c.Scenarios {
		if s.Key == key {
			return s
		}
	}
	return ScenarioDiff{}
}

func TestScenarioAndTurnComparison(t *testing.T) {
	b := scenario("s", domain.ScenarioPassed, turn(1, "a", true, "search"), turn(2, "b", true), turn(3, "c", true))
	tg := scenario("s", domain.ScenarioFailed, turn(1, "a", true, "search", "search"), turn(2, "b2", false))
	sc := Scenarios(&b, &tg)
	if sc.Change != Changed || sc.Outcome != Regression || len(sc.Turns) != 3 {
		t.Fatalf("scenario diff %+v", sc)
	}
	t1, t2, t3 := sc.Turns[0], sc.Turns[1], sc.Turns[2]
	if t1.Change != Unchanged || t1.Outcome != StablePass || t1.ToolDelta["search"] != 1 {
		t.Fatalf("turn 1 %+v", t1)
	}
	if t2.Change != Changed || t2.Outcome != Regression {
		t.Fatalf("turn 2 %+v", t2)
	}
	if t3.Change != Removed || t3.Target != nil || t3.Base == nil {
		t.Fatalf("turn 3 %+v", t3)
	}
}

func TestConfigDiffFlattensNestedValues(t *testing.T) {
	d := ConfigDiff(json.RawMessage(`{"a":{"b":1,"c":[1,2]},"x":"same"}`), json.RawMessage(`{"a":{"b":2,"c":[1,2]},"x":"same","y":true}`))
	if len(d) != 2 || d[0].Path != "a.b" || d[1].Path != "y" || d[1].Base != nil {
		t.Fatalf("got %+v", d)
	}
}
