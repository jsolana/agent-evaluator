// Package criteriontest helps to test a criterion in a few lines.
package criteriontest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/criterionkit"
)

// Case is one turn and the verdict expected for it.
type Case struct {
	Name       string
	Params     string // JSON params as the suite would declare them
	Input      criterionkit.Input
	WantPassed bool
}

// Run builds the criterion for every case and checks its verdict. It also checks
// that the verdict carries an explanation, which the report always shows.
func Run(t *testing.T, factory criterionkit.Factory, cases []Case) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			crit, err := factory(json.RawMessage(c.Params))
			if err != nil {
				t.Fatalf("factory(%s): %v", c.Params, err)
			}
			res, err := crit.Evaluate(context.Background(), c.Input)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if res.Passed != c.WantPassed {
				t.Fatalf("passed = %v, want %v (%s)", res.Passed, c.WantPassed, res.Explanation)
			}
			if res.Explanation == "" {
				t.Fatal("a verdict needs an explanation")
			}
		})
	}
}

// Invalid checks that the factory rejects the given params.
func Invalid(t *testing.T, factory criterionkit.Factory, params ...string) {
	t.Helper()
	for _, p := range params {
		if _, err := factory(json.RawMessage(p)); err == nil {
			t.Errorf("params %s must be rejected", p)
		}
	}
}
