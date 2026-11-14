package fake

import (
	"testing"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
	"github.com/jsolana/agent-evaluator/backend/agentkit/agenttest"
)

func TestFakeConformsToTheAgentContract(t *testing.T) {
	agenttest.Run(t, func(*testing.T) agentkit.Agent { return New(nil) })
}
