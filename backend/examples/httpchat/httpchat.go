// Package httpchat is a complete adapter for a stateless chat HTTP API, and the
// reference for the tutorial in the README. The agent it talks to is assumed to
// expose:
//
//	POST {HTTPCHAT_URL}/chat   Authorization: Bearer {HTTPCHAT_KEY}
//	{"messages":[{"role":"user|assistant","content":"..."}]}  ->  {"answer":"...","tokens":123}
//
// Because the API keeps no conversation, the adapter resends the history that
// the evaluator provides in TurnInput.History. It imports only agentkit.
package httpchat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

func init() { agentkit.Register("httpchat", factory) }

// factory reads the settings. Missing ones are reported by Check, not here.
func factory(env agentkit.Env) (agentkit.Plugin, error) {
	a := &Agent{
		URL: strings.TrimRight(env("HTTPCHAT_URL"), "/"), Key: env("HTTPCHAT_KEY"),
		HTTP:  &http.Client{Timeout: 2 * time.Minute},
		check: agentkit.Require(env, "HTTPCHAT_URL", "HTTPCHAT_KEY"),
	}
	return agentkit.Plugin{Agent: a}, nil // the API exposes no trace: no TraceSource
}

type Agent struct {
	URL, Key string
	HTTP     *http.Client
	check    func(context.Context) error
}

// Check implements agentkit.HealthChecker.
func (a *Agent) Check(ctx context.Context) error {
	if a.check == nil {
		return nil
	}
	return a.check(ctx)
}

func (a *Agent) NewConversation(context.Context, agentkit.ConversationSpec) (agentkit.Conversation, error) {
	return &conversation{agent: a}, nil
}

type conversation struct{ agent *Agent }

// Ref is empty: the API keeps no conversation, so there is nothing to resume.
func (*conversation) Ref() string { return "" }

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *conversation) Send(ctx context.Context, in agentkit.TurnInput) (agentkit.TurnOutput, error) {
	var msgs []message
	for _, h := range in.History {
		msgs = append(msgs, message{"user", h.Question}, message{"assistant", h.Answer})
	}
	msgs = append(msgs, message{"user", in.Message})
	body, _ := json.Marshal(map[string]any{"messages": msgs})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.agent.URL+"/chat", bytes.NewReader(body))
	if err != nil {
		return agentkit.TurnOutput{}, fmt.Errorf("%w: %v", agentkit.ErrFatal, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.agent.Key)
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.agent.HTTP.Do(req)
	if err != nil { // network failure: the request may not have been processed
		return agentkit.TurnOutput{}, fmt.Errorf("%w: %v", agentkit.ErrRetryable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return agentkit.TurnOutput{}, fmt.Errorf("%w: status %d", agentkit.ErrRetryable, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return agentkit.TurnOutput{}, fmt.Errorf("%w: status %d: %s", agentkit.ErrFatal, resp.StatusCode, raw)
	}
	var r struct {
		Answer string `json:"answer"`
		Tokens int    `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.Answer == "" {
		return agentkit.TurnOutput{}, fmt.Errorf("%w: unexpected response: %s", agentkit.ErrFatal, errors.Join(err, errors.New(string(raw))))
	}
	return agentkit.TurnOutput{Text: r.Answer, Raw: raw, Tokens: r.Tokens, DurationMs: time.Since(start).Milliseconds()}, nil
}
