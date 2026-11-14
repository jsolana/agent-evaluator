package hiringlab

import (
	"testing"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
	"github.com/jsolana/agent-evaluator/backend/agentkit/agenttest"
)

func TestHiringLabConformsToTheAgentContract(t *testing.T) {
	agenttest.Run(t, func(t *testing.T) agentkit.Agent {
		a, stop := newAgent(t, &fakeAsk{})
		t.Cleanup(stop)
		return a
	})
}

func TestCheckReportsMissingSettings(t *testing.T) {
	if err := New(Config{}).Check(nil); err == nil {
		t.Fatal("an unconfigured agent must report it")
	}
	if err := New(Config{BaseURL: "http://x"}).Check(nil); err == nil {
		t.Fatal("a missing key must be reported")
	}
}
