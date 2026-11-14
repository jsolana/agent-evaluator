package report

import (
	"testing"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
)

func TestToolUsage(t *testing.T) {
	u := ToolUsageOf([]domain.Span{{Kind: "tool", Name: "ragSearch"}, {Kind: "llm", Name: "gpt"}, {Kind: "tool", Name: "ragGetSourceChunks"}, {Kind: "tool", Name: "ragSearch"}})
	if len(u.Sequence) != 3 || u.Sequence[2] != "ragSearch" || u.Counts["ragSearch"] != 2 || u.Counts["gpt"] != 0 {
		t.Fatalf("got %+v", u)
	}
	m := u.Merge(ToolUsage{Sequence: []string{"ragSearch"}, Counts: map[string]int{"ragSearch": 1}})
	if m.Counts["ragSearch"] != 3 || len(m.Sequence) != 4 || u.Counts["ragSearch"] != 2 {
		t.Fatalf("merge must not mutate: %+v / %+v", m, u)
	}
}

func TestSummarize(t *testing.T) {
	pass := TurnReport{Status: domain.TurnCompleted, Passed: true, Tokens: 10, DurationMs: 100,
		CriterionResults: []domain.CriterionResult{{CriterionType: "regex", Passed: true}}}
	fail := TurnReport{Status: domain.TurnCompleted, Passed: false, Tokens: 10, DurationMs: 100,
		CriterionResults: []domain.CriterionResult{{CriterionType: "regex", Passed: true}, {CriterionType: "llm_judge", Passed: false, CostTokens: 900}}}
	notRun := TurnReport{Status: StatusNotRun}
	scenarios := []ScenarioReport{
		{Status: domain.ScenarioPassed, Turns: []TurnReport{pass, pass, pass}, TotalTokens: 30, Tools: ToolUsage{Counts: map[string]int{"ragSearch": 3}}},
		{Status: domain.ScenarioFailed, FirstFailedTurn: 2, Turns: []TurnReport{pass, fail, pass}, TotalTokens: 30, Tools: ToolUsage{Counts: map[string]int{"ragSearch": 1}}},
		{Status: domain.ScenarioExecutionError, Turns: []TurnReport{pass, {Status: domain.TurnExecutionError}, notRun}},
		{Status: domain.ScenarioPending, Turns: []TurnReport{notRun, notRun, notRun}},
	}
	s := Summarize(scenarios)
	if s.Scenarios != (Counts{Total: 4, Passed: 1, Failed: 1, ExecutionError: 1, Pending: 1}) {
		t.Fatalf("scenario counts %+v", s.Scenarios)
	}
	if s.ScenarioPassRate != 0.5 { // execution errors and pending are excluded
		t.Fatalf("ScenarioPassRate = %v", s.ScenarioPassRate)
	}
	if s.Turns.Total != 12 || s.Turns.Evaluated != 7 || s.Turns.Passed != 6 {
		t.Fatalf("turn counts %+v", s.Turns)
	}
	if s.FirstFailedTurns[2] != 1 || s.Tokens != 60 || s.Tools["ragSearch"] != 4 || s.Progress != 0.75 {
		t.Fatalf("summary %+v", s)
	}
	if s.JudgeTokens != 900 {
		t.Fatalf("JudgeTokens = %d", s.JudgeTokens)
	}
	if c := s.Criteria["llm_judge"]; c.Passed != 0 || c.Total != 1 {
		t.Fatalf("criteria %+v", s.Criteria)
	}
}
