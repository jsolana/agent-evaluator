package httpagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
	"github.com/jsolana/agent-evaluator/backend/agentkit/agenttest"
)

const statelessYAML = `
baseUrl: ${AGENT_URL}
headers:
  Authorization: "Bearer ${AGENT_KEY}"
turn:
  path: /chat
  body: '{"messages": {{ json .Messages }}, "options": {{ json .Config }}}'
response:
  text: answer
  tokens: usage.total
`

func env(m map[string]string) agentkit.Env { return func(k string) string { return m[k] } }

// api imitates a chat API; status != 0 makes it fail.
func api(t *testing.T, status int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		var in struct {
			Messages []message      `json:"messages"`
			Options  map[string]any `json:"options"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		last := in.Messages[len(in.Messages)-1].Content
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answer": strings.ToUpper(last), "usage": map[string]any{"total": len(in.Messages)}, "options": in.Options,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newFrom(t *testing.T, yaml string, srv *httptest.Server) *Agent {
	t.Helper()
	s, err := load([]byte(yaml), env(map[string]string{"AGENT_URL": srv.URL, "AGENT_KEY": "k"}))
	if err != nil {
		t.Fatal(err)
	}
	return newAgent(s, nil)
}

func send(t *testing.T, a *Agent, cfg string, in agentkit.TurnInput) (agentkit.TurnOutput, error) {
	t.Helper()
	c, err := a.NewConversation(context.Background(), agentkit.ConversationSpec{Config: json.RawMessage(cfg)})
	if err != nil {
		t.Fatal(err)
	}
	return c.Send(context.Background(), in)
}

func TestConformsToTheAgentContract(t *testing.T) {
	agenttest.Run(t, func(t *testing.T) agentkit.Agent { return newFrom(t, statelessYAML, api(t, 0)) })
}

func TestStatelessResendsHistoryAndReadsTheResponse(t *testing.T) {
	a := newFrom(t, statelessYAML, api(t, 0))
	out, err := send(t, a, `{"model":"m1"}`, agentkit.TurnInput{
		Message: "b", History: []agentkit.Exchange{{Question: "a", Answer: "A"}}})
	if err != nil || out.Text != "B" || out.Tokens != 3 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if !strings.Contains(string(out.Raw), `"model":"m1"`) {
		t.Fatalf("agentConfig must reach the template: %s", out.Raw)
	}
}

func TestQuotesInTheQuestionAreEscaped(t *testing.T) {
	a := newFrom(t, statelessYAML, api(t, 0))
	if out, err := send(t, a, "", agentkit.TurnInput{Message: `say "hi"` + "\n"}); err != nil || !strings.HasPrefix(out.Text, `SAY "HI"`) {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestErrorsAreClassifiedFromTheConfiguration(t *testing.T) {
	yaml := statelessYAML + "errors:\n  retryable: [503, 429]\n  conflict: [409]\n"
	for status, want := range map[int]error{
		503: agentkit.ErrRetryable, 429: agentkit.ErrRetryable, 409: agentkit.ErrConflict, 500: agentkit.ErrFatal, 400: agentkit.ErrFatal,
	} {
		if _, err := send(t, newFrom(t, yaml, api(t, status)), "", agentkit.TurnInput{Message: "a"}); !errors.Is(err, want) {
			t.Errorf("status %d: got %v, want %v", status, err, want)
		}
	}
	if _, err := send(t, newFrom(t, statelessYAML, api(t, 502)), "", agentkit.TurnInput{Message: "a"}); !errors.Is(err, agentkit.ErrRetryable) {
		t.Errorf("5xx is retryable by default, got %v", err)
	}
}

func TestMissingAnswerFieldIsFatalAndExplained(t *testing.T) {
	a := newFrom(t, strings.Replace(statelessYAML, "text: answer", "text: nope", 1), api(t, 0))
	if _, err := send(t, a, "", agentkit.TurnInput{Message: "a"}); !errors.Is(err, agentkit.ErrFatal) || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("got %v", err)
	}
}

func TestStatefulKeepsTheConversationIdAndResumesIt(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Q, ID string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		got = append(got, in.ID)
		_ = json.NewEncoder(w).Encode(map[string]any{"reply": "ok", "conv": map[string]any{"id": "c-1"}})
	}))
	defer srv.Close()
	yaml := `
baseUrl: ` + srv.URL + `
conversation: {mode: stateful}
turn:
  path: /ask
  body: '{"q": {{ json .Message }}, "id": {{ json .Ref }}}'
response: {text: reply, ref: conv.id}
`
	a := newFrom(t, yaml, srv)
	c, _ := a.NewConversation(context.Background(), agentkit.ConversationSpec{})
	for i := 1; i <= 2; i++ {
		if _, err := c.Send(context.Background(), agentkit.TurnInput{Index: i, Message: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if c.Ref() != "c-1" || got[0] != "" || got[1] != "c-1" {
		t.Fatalf("ref=%q ids sent=%q", c.Ref(), got)
	}
	if r, _ := a.NewConversation(context.Background(), agentkit.ConversationSpec{Ref: "c-9"}); r.Ref() != "c-9" {
		t.Fatalf("resume lost the ref: %q", r.Ref())
	}
}

func TestIdempotencyKeyIsSentAndDeclared(t *testing.T) {
	var header string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("Idempotency-Key")
		_, _ = w.Write([]byte(`{"answer":"ok"}`))
	}))
	defer srv.Close()
	yaml := "baseUrl: " + srv.URL + "\nturn: {path: /chat, idempotencyHeader: Idempotency-Key}\nresponse: {text: answer}\n"
	a := newFrom(t, yaml, srv)
	if _, err := send(t, a, "", agentkit.TurnInput{Message: "a", IdempotencyKey: "k-7"}); err != nil || header != "k-7" || !a.Idempotent() {
		t.Fatalf("header=%q idempotent=%v err=%v", header, a.Idempotent(), err)
	}
	if newFrom(t, statelessYAML, srv).Idempotent() {
		t.Fatal("without idempotencyHeader the agent must not claim idempotency")
	}
}

func TestCheckReportsEveryMissingVariable(t *testing.T) {
	s, err := load([]byte(statelessYAML), env(map[string]string{"AGENT_URL": "http://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if err := newAgent(s, nil).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "AGENT_KEY") {
		t.Fatalf("got %v", err)
	}
}

func TestInvalidConfigurationsFailAtLoad(t *testing.T) {
	cases := map[string]string{
		"unknown field":        "baseUrl: http://x\nturn: {path: /c}\nresponse: {text: a}\nbogus: 1\n",
		"no text":              "baseUrl: http://x\nturn: {path: /c}\n",
		"bad mode":             "baseUrl: http://x\nconversation: {mode: weird}\nturn: {path: /c}\nresponse: {text: a}\n",
		"stateful without ref": "baseUrl: http://x\nconversation: {mode: stateful}\nturn: {path: /c}\nresponse: {text: a}\n",
		"bad template":         "baseUrl: http://x\nturn: {path: /c, body: '{{ .Nope '}\nresponse: {text: a}\n",
		"bad status":           "baseUrl: http://x\nturn: {path: /c}\nresponse: {text: a}\nerrors: {retryable: [abc]}\n",
		"traces incomplete":    "baseUrl: http://x\nturn: {path: /c}\nresponse: {text: a}\ntraces: {from: t}\n",
	}
	for name, yaml := range cases {
		if _, err := load([]byte(yaml), env(nil)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestBodyThatIsNotJSONIsFatal(t *testing.T) {
	srv := api(t, 0)
	yaml := "baseUrl: " + srv.URL + "\nturn: {path: /chat, body: 'not json {{ .Message }}'}\nresponse: {text: answer}\n"
	if _, err := send(t, newFrom(t, yaml, srv), "", agentkit.TurnInput{Message: "a"}); !errors.Is(err, agentkit.ErrFatal) {
		t.Fatalf("got %v", err)
	}
}

func TestFactoryNeedsTheConfigFile(t *testing.T) {
	if _, err := agentkit.Default.Build("httpagent", env(nil)); err == nil || !strings.Contains(err.Error(), "HTTPAGENT_CONFIG") {
		t.Fatalf("got %v", err)
	}
	path := filepath.Join(t.TempDir(), "a.yaml")
	if err := os.WriteFile(path, []byte(statelessYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := agentkit.Default.Build("httpagent", env(map[string]string{"HTTPAGENT_CONFIG": path}))
	if err != nil || p.Agent == nil {
		t.Fatalf("plugin=%+v err=%v", p, err)
	}
	if err := p.Agent.(agentkit.HealthChecker).Check(context.Background()); err == nil {
		t.Fatal("unset ${AGENT_URL} must make the health check fail, not the start-up")
	}
}

// The shipped example must stay valid and behave like the coded httpchat adapter.
func TestShippedExampleIsValid(t *testing.T) {
	src, err := os.ReadFile("../../examples/httpagent/httpchat.agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Messages []message }
		_ = json.NewDecoder(r.Body).Decode(&in)
		_ = json.NewEncoder(w).Encode(map[string]any{"answer": in.Messages[len(in.Messages)-1].Content, "tokens": 5})
	}))
	defer srv.Close()
	s, err := load(src, env(map[string]string{"HTTPCHAT_URL": srv.URL, "HTTPCHAT_KEY": "k"}))
	if err != nil {
		t.Fatal(err)
	}
	out, err := send(t, newAgent(s, nil), "", agentkit.TurnInput{Message: "hello"})
	if err != nil || out.Text != "hello" || out.Tokens != 5 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
