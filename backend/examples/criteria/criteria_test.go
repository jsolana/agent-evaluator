package criteria

import (
	"testing"

	"github.com/jsolana/agent-evaluator/backend/criterionkit"
	"github.com/jsolana/agent-evaluator/backend/criterionkit/criteriontest"
)

func TestMaxWords(t *testing.T) {
	criteriontest.Run(t, maxWordsFactory, []criteriontest.Case{
		{Name: "within", Params: `{"limit":3}`, Input: criterionkit.Input{Answer: "one two three"}, WantPassed: true},
		{Name: "over", Params: `{"limit":2}`, Input: criterionkit.Input{Answer: "one two three"}, WantPassed: false},
	})
	criteriontest.Invalid(t, maxWordsFactory, `{}`, `{"limit":0}`, `{"limt":3}`)
}

func TestLatencyUnder(t *testing.T) {
	criteriontest.Run(t, latencyFactory, []criteriontest.Case{
		{Name: "fast", Params: `{"ms":500}`, Input: criterionkit.Input{DurationMs: 120}, WantPassed: true},
		{Name: "slow", Params: `{"ms":500}`, Input: criterionkit.Input{DurationMs: 900}, WantPassed: false},
	})
	criteriontest.Invalid(t, latencyFactory, `{"ms":-1}`, `{"seconds":2}`)
}

func TestRegisteredInTheDefaultRegistry(t *testing.T) {
	for _, name := range []string{"max_words", "latency_under"} {
		if _, ok := criterionkit.Default.Lookup(name); !ok {
			t.Errorf("%s is not registered", name)
		}
	}
}
