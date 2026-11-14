package httpchat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
	"github.com/jsolana/agent-evaluator/backend/agentkit/agenttest"
)

// server imitates the chat API: it answers with the number of messages it received.
func server(t *testing.T, status int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var in struct{ Messages []message }
		_ = json.NewDecoder(r.Body).Decode(&in)
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answer": "got " + string(rune('0'+len(in.Messages))) + " messages", "tokens": 7})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPChatConformsToTheAgentContract(t *testing.T) {
	agenttest.Run(t, func(t *testing.T) agentkit.Agent {
		return &Agent{URL: server(t, 0).URL, Key: "k", HTTP: http.DefaultClient}
	})
}

func TestHistoryIsResentBecauseTheAPIIsStateless(t *testing.T) {
	a := &Agent{URL: server(t, 0).URL, Key: "k", HTTP: http.DefaultClient}
	c, _ := a.NewConversation(context.Background(), agentkit.ConversationSpec{})
	out, err := c.Send(context.Background(), agentkit.TurnInput{Message: "b", History: []agentkit.Exchange{{Question: "a", Answer: "x"}}})
	if err != nil || out.Text != "got 3 messages" || out.Tokens != 7 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestErrorsAreClassifiedForTheRunner(t *testing.T) {
	for status, want := range map[int]error{500: agentkit.ErrRetryable, 429: agentkit.ErrRetryable, 400: agentkit.ErrFatal} {
		a := &Agent{URL: server(t, status).URL, Key: "k", HTTP: http.DefaultClient}
		c, _ := a.NewConversation(context.Background(), agentkit.ConversationSpec{})
		if _, err := c.Send(context.Background(), agentkit.TurnInput{Message: "a"}); !errors.Is(err, want) {
			t.Errorf("status %d: got %v, want %v", status, err, want)
		}
	}
}

func TestFactoryReportsMissingSettingsThroughCheck(t *testing.T) {
	p, err := agentkit.Default.Build("httpchat", func(string) string { return "" })
	if err != nil {
		t.Fatalf("a missing setting must not stop the server: %v", err)
	}
	if err := p.Agent.(agentkit.HealthChecker).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTPCHAT_URL") {
		t.Fatalf("got %v", err)
	}
}
