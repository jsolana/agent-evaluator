package domain

import "testing"

func TestTurnPassed(t *testing.T) {
	tests := []struct {
		name     string
		criteria []CriterionResult
		want     bool
	}{
		{"all criteria pass", []CriterionResult{{Passed: true}, {Passed: true}}, true},
		{"one criterion fails", []CriterionResult{{Passed: true}, {Passed: false}}, false},
		{"criterion errored counts as failed", []CriterionResult{{Passed: true, Error: "judge timeout"}}, false},
		{"no criteria passes vacuously", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TurnPassed(tt.criteria); got != tt.want {
				t.Fatalf("TurnPassed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScenarioVerdict(t *testing.T) {
	pass := TurnResult{Status: TurnCompleted, Passed: true}
	fail := TurnResult{Status: TurnCompleted, Passed: false}
	broken := TurnResult{Status: TurnExecutionError}

	tests := []struct {
		name            string
		turns           []TurnResult
		wantStatus      ScenarioStatus
		wantFirstFailed int
	}{
		{"all turns pass", []TurnResult{pass, pass, pass}, ScenarioPassed, 0},
		{"first failing turn is reported 1-based", []TurnResult{pass, fail, fail}, ScenarioFailed, 2},
		{"execution error wins over agent failure", []TurnResult{fail, broken}, ScenarioExecutionError, 1},
		{"execution error alone is not an agent failure", []TurnResult{pass, broken}, ScenarioExecutionError, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, first := ScenarioVerdict(tt.turns)
			if status != tt.wantStatus || first != tt.wantFirstFailed {
				t.Fatalf("ScenarioVerdict() = (%s, %d), want (%s, %d)", status, first, tt.wantStatus, tt.wantFirstFailed)
			}
		})
	}
}
