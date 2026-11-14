// Package criterionkit is the public contract for adding your own criteria: the
// checks a scenario turn declares in a suite (`- {type: max_words, limit: 80}`).
// Implement Criterion, register a Factory from an init function and run the
// evaluator from your own main. It depends on the standard library and agentkit.
//
// Return an error from Evaluate only when the turn could not be evaluated (a
// dependency is down). A turn that was evaluated and does not comply returns
// Passed: false with an explanation.
package criterionkit

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// Input is everything a criterion may inspect for one turn.
type Input struct {
	History    []agentkit.Exchange // previous turns of the conversation
	Question   string
	Answer     string
	Spans      []agentkit.Span // trace of the turn; empty if the agent exposes none
	DurationMs int64           // time the agent took to answer
	Tokens     int             // tokens the agent reported for the turn
}

// Result is the verdict of a criterion for one turn.
type Result struct {
	Passed      bool
	Score       *float64 // optional, 0..1
	Explanation string   // why it passed or failed; shown in the report
	Evidence    []string // excerpts that support the verdict
	Version     string   // bump when the logic changes, so comparisons can tell; "1" if empty
	Tokens      int      // cost in tokens, if the criterion called an LLM
}

type Criterion interface {
	Evaluate(ctx context.Context, in Input) (Result, error)
}

// Factory builds a criterion from the params the suite declares, validating
// them: it runs when a suite is imported, so mistakes are reported with their
// path before anything is executed.
type Factory func(params json.RawMessage) (Criterion, error)

// DecodeParams decodes params into v and rejects unknown fields, so a typo in a
// suite fails at import time. Empty params decode as {}.
func DecodeParams(params json.RawMessage, v any) error {
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}
	d := json.NewDecoder(bytes.NewReader(params))
	d.DisallowUnknownFields()
	return d.Decode(v)
}
