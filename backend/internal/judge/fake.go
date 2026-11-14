package judge

import (
	"context"
	"strings"

	"github.com/jsolana/agent-evaluator/backend/internal/analysis"
	"github.com/jsolana/agent-evaluator/backend/internal/criteria"
	"github.com/jsolana/agent-evaluator/backend/internal/domain"
)

// Fake is a deterministic judge and analyzer for offline demos and UI tests
// (JUDGE_PROVIDER=fake). It never calls an LLM.
type Fake struct{}

func (Fake) Judge(context.Context, criteria.JudgeRequest) (criteria.JudgeVerdict, error) {
	return criteria.JudgeVerdict{Passed: true, Score: 1, Explanation: "fake judge: always passes", Version: "fake/" + rubricVersion}, nil
}

// Analyze: a passed turn is "correct"; a failed one is a "false_positive" if a
// reviewer tagged the rubric as too strict, otherwise a "real_failure".
func (Fake) Analyze(_ context.Context, r analysis.Request) (domain.Analysis, error) {
	a := domain.Analysis{Model: "fake/analysis-v1", Confidence: 0.7}
	switch {
	case r.TurnPassed:
		a.Verdict, a.RootCause, a.Rationale = "correct", "none", "The turn passed and the answer matches the rubric."
	case strictRubric(r.Annotations):
		a.Verdict, a.RootCause = "false_positive", "rubric"
		a.Rationale = "A reviewer flagged the rubric as too strict and the answer is consistent with the retrieved documents."
		a.Recommendation = "Update the failing criterion to accept the answer."
	default:
		a.Verdict, a.RootCause, a.Rationale = "real_failure", "agent", "The answer does not satisfy the failing criterion."
	}
	return a, nil
}

func strictRubric(anns []domain.Annotation) bool {
	for _, a := range anns {
		for _, t := range a.Tags {
			if t == "rubric_too_strict" {
				return true
			}
		}
		if strings.Contains(strings.ToLower(a.Body), "rubric") || strings.Contains(strings.ToLower(a.Body), "rúbrica") {
			return true
		}
	}
	return false
}
