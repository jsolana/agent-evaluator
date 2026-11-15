package recipeagent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
	"github.com/jsolana/agent-evaluator/backend/agentkit/agenttest"
)

const spansJSON = `{"spans":[
 {"name":"ChatCompletion","parent_id":"0xaa","context":{"span_id":"0xbb"},"start_time":"2026-01-01T00:00:00.1Z","end_time":"2026-01-01T00:00:00.4Z",
  "attributes":{"openinference.span.kind":"LLM","llm.model_name":"m","llm.token_count.prompt":11,"other":1},"status":{"status_code":"ERROR","description":"boom"}},
 {"name":"recipe-agent","parent_id":null,"context":{"span_id":"0xaa"},"start_time":"2026-01-01T00:00:00Z","end_time":"2026-01-01T00:00:00.5Z",
  "attributes":{"openinference.span.kind":"AGENT","input.value":"huevos"},"status":{"status_code":"UNSET"}}]}`

func fake(t *testing.T, status int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case status != 0:
			w.WriteHeader(status)
		case r.URL.Path == "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/chat":
			_, _ = w.Write([]byte(`{"answer":"Tortilla","usage":{"total_tokens":16},"latency_ms":500,"trace_id":"abc"}`))
		case r.URL.Path == "/traces/abc":
			_, _ = w.Write([]byte(spansJSON))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newAgent(url string) *Agent { return &Agent{URL: url, HTTP: http.DefaultClient} }

func TestConformsToTheAgentContract(t *testing.T) {
	agenttest.Run(t, func(t *testing.T) agentkit.Agent { return newAgent(fake(t, 0).URL) })
}

func TestTurnCarriesTokensLatencyAndTraceRef(t *testing.T) {
	c, _ := newAgent(fake(t, 0).URL).NewConversation(context.Background(), agentkit.ConversationSpec{})
	out, err := c.Send(context.Background(), agentkit.TurnInput{Message: "huevos"})
	if err != nil || out.Tokens != 16 || out.DurationMs != 500 || out.TraceRef != "abc" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestErrorsAreClassifiedForTheRunner(t *testing.T) {
	for status, want := range map[int]error{429: agentkit.ErrRetryable, 502: agentkit.ErrRetryable, 400: agentkit.ErrFatal} {
		c, _ := newAgent(fake(t, status).URL).NewConversation(context.Background(), agentkit.ConversationSpec{})
		if _, err := c.Send(context.Background(), agentkit.TurnInput{Message: "x"}); !errors.Is(err, want) {
			t.Errorf("status %d: err=%v, want %v", status, err, want)
		}
	}
}

func TestSpansMapOpenInferenceToAgentkit(t *testing.T) {
	srv := fake(t, 0)
	spans, err := Traces{URL: srv.URL, HTTP: http.DefaultClient}.Spans(context.Background(), agentkit.TraceQuery{TraceRef: "abc"})
	if err != nil || len(spans) != 2 {
		t.Fatalf("spans=%+v err=%v", spans, err)
	}
	root, llm := spans[0], spans[1] // ordered by start time
	if root.Kind != "other" || root.DurationMs != 500 || root.ParentID != "" {
		t.Errorf("root=%+v", root)
	}
	if llm.Kind != "llm" || llm.DurationMs != 300 || llm.Status != "error" || llm.Error != "boom" || llm.ParentID != "0xaa" {
		t.Errorf("llm=%+v", llm)
	}
	if llm.Name != "m" || string(llm.Attributes) != `{"llm.model_name":"m","llm.token_count.prompt":11,"openinference.span.kind":"LLM"}` {
		t.Errorf("attributes=%s", llm.Attributes)
	}
}
