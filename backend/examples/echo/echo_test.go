package echo

import (
	"testing"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
	"github.com/jsolana/agent-evaluator/backend/agentkit/agenttest"
)

func TestEchoConformsToTheAgentContract(t *testing.T) {
	agenttest.Run(t, func(*testing.T) agentkit.Agent { return &Agent{} })
}

func TestFactoryIsRegistered(t *testing.T) {
	p, err := agentkit.Default.Build("echo", func(string) string { return "" })
	if err != nil || p.Agent == nil {
		t.Fatalf("plugin=%+v err=%v", p, err)
	}
}
