package fake

import (
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

func init() { agentkit.Register("fake", factory) }

// factory builds the offline demo agent: deterministic, with a small delay so
// the UI shows progress.
func factory(agentkit.Env) (agentkit.Plugin, error) {
	a := New(nil)
	a.Delay = 300 * time.Millisecond
	return agentkit.Plugin{Agent: a, Traces: Source{}}, nil
}
