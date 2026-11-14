package judge

import (
	"context"
	"strings"
	"testing"
)

// The evaluated agent is configurable: the system prompts name it, and a
// generic description is used when none is given.
func TestSystemPromptsNameTheEvaluatedAgent(t *testing.T) {
	for _, tc := range []struct{ configured, want string }{
		{"a banking assistant", "a banking assistant"},
		{"", DefaultAgentDescription},
	} {
		var judged, analyzed captured
		js := chatServer(t, &judged, nil, okVerdict, "stop")
		as := chatServer(t, &analyzed, nil, okAnalysis, "stop")
		cfg := Config{APIKey: "k", Model: "m", AgentDescription: tc.configured}

		cfg.BaseURL = js.URL + "/v1"
		if _, err := NewOpenAICompatible(cfg).Judge(context.Background(), request()); err != nil {
			t.Fatal(err)
		}
		cfg.BaseURL = as.URL + "/v1"
		if _, err := NewOpenAICompatible(cfg).Analyze(context.Background(), analysisRequest()); err != nil {
			t.Fatal(err)
		}
		js.Close()
		as.Close()

		for name, got := range map[string]captured{"judge": judged, "analysis": analyzed} {
			if system := systemMessage(t, got); !strings.Contains(system, tc.want) {
				t.Fatalf("%s system prompt for %q lacks %q: %s", name, tc.configured, tc.want, system)
			}
		}
	}
}

func systemMessage(t *testing.T, got captured) string {
	t.Helper()
	msgs, _ := got.body["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("no messages sent")
	}
	sys, _ := msgs[0].(map[string]any)
	content, _ := sys["content"].(string)
	return content
}
