package domain

// TurnPassed reports whether every criterion of a turn passed.
// A criterion that could not be evaluated (Error set) counts as not passed.
func TurnPassed(results []CriterionResult) bool {
	for _, r := range results {
		if !r.Passed || r.Error != "" {
			return false
		}
	}
	return true
}

// ScenarioVerdict derives the scenario status from its turns, in order, and the
// 1-based index of the first turn the agent failed (0 if none).
// An execution error is a technical failure, not an agent failure, so it takes
// precedence over the pass/fail outcome.
func ScenarioVerdict(turns []TurnResult) (ScenarioStatus, int) {
	firstFailed := 0
	executionError := false
	for i, t := range turns {
		if t.Status == TurnExecutionError {
			executionError = true
			continue
		}
		if !t.Passed && firstFailed == 0 {
			firstFailed = i + 1
		}
	}
	switch {
	case executionError:
		return ScenarioExecutionError, firstFailed
	case firstFailed > 0:
		return ScenarioFailed, firstFailed
	default:
		return ScenarioPassed, 0
	}
}
