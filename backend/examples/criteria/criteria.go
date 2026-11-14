// Package criteria shows how to add your own checks. Each criterion is a small
// type plus a Factory registered from init; suites then declare it by name:
//
//   - {type: max_words, limit: 80}
//   - {type: latency_under, ms: 2000}
//
// It imports only criterionkit.
package criteria

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"encoding/json"

	"github.com/jsolana/agent-evaluator/backend/criterionkit"
)

func init() {
	criterionkit.Register("max_words", maxWordsFactory)
	criterionkit.Register("latency_under", latencyFactory)
}

// ---- max_words: the answer must not be longer than a limit.

type maxWords struct{ limit int }

func maxWordsFactory(params json.RawMessage) (criterionkit.Criterion, error) {
	var p struct {
		Limit int `json:"limit"`
	}
	if err := criterionkit.DecodeParams(params, &p); err != nil {
		return nil, err
	}
	if p.Limit <= 0 {
		return nil, errors.New("limit must be positive")
	}
	return maxWords{p.Limit}, nil
}

func (c maxWords) Evaluate(_ context.Context, in criterionkit.Input) (criterionkit.Result, error) {
	n := len(strings.Fields(in.Answer))
	return criterionkit.Result{Passed: n <= c.limit, Explanation: fmt.Sprintf("%d words (limit %d)", n, c.limit)}, nil
}

// ---- latency_under: the agent must answer within a time budget.

type latencyUnder struct{ ms int64 }

func latencyFactory(params json.RawMessage) (criterionkit.Criterion, error) {
	var p struct {
		Ms int64 `json:"ms"`
	}
	if err := criterionkit.DecodeParams(params, &p); err != nil {
		return nil, err
	}
	if p.Ms <= 0 {
		return nil, errors.New("ms must be positive")
	}
	return latencyUnder{p.Ms}, nil
}

func (c latencyUnder) Evaluate(_ context.Context, in criterionkit.Input) (criterionkit.Result, error) {
	return criterionkit.Result{
		Passed:      in.DurationMs <= c.ms,
		Explanation: fmt.Sprintf("answered in %d ms (budget %d ms)", in.DurationMs, c.ms),
	}, nil
}
