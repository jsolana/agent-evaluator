// Package recipeagent connects the recipe agent (Python, traced with OpenInference)
// to the evaluator. The agent keeps no conversation, so the evaluator's history is
// resent each turn. Tokens and latency come from the agent's own telemetry, and
// the spans of a turn are fetched by trace id, as they would be from a collector.
//
//	POST {RECIPE_URL}/chat           {"messages":[...]} -> {"answer","usage","latency_ms","trace_id"}
//	GET  {RECIPE_URL}/traces/{id}    -> {"spans":[...]}   (OpenInference spans)
package recipeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

func init() { agentkit.Register("recipeagent", factory) }

func factory(env agentkit.Env) (agentkit.Plugin, error) {
	a := &Agent{
		URL:   strings.TrimRight(env("RECIPE_URL"), "/"),
		HTTP:  &http.Client{Timeout: 2 * time.Minute},
		check: agentkit.Require(env, "RECIPE_URL"),
	}
	return agentkit.Plugin{Agent: a, Traces: Traces{URL: a.URL, HTTP: a.HTTP}}, nil
}

// Agent implements agentkit.Agent and agentkit.HealthChecker.
type Agent struct {
	URL   string
	HTTP  *http.Client
	check func(context.Context) error
}

func (a *Agent) Check(ctx context.Context) error {
	if a.check != nil {
		if err := a.check(ctx); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent health: status %d", resp.StatusCode)
	}
	return nil
}

func (a *Agent) NewConversation(context.Context, agentkit.ConversationSpec) (agentkit.Conversation, error) {
	return &conversation{agent: a}, nil
}

type conversation struct{ agent *Agent }

// Ref is empty: stateless agent, nothing to resume.
func (*conversation) Ref() string { return "" }

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Answer string `json:"answer"`
	Usage  struct {
		Total int `json:"total_tokens"`
	} `json:"usage"`
	LatencyMs int64  `json:"latency_ms"`
	TraceID   string `json:"trace_id"`
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
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.agent.HTTP.Do(req)
	if err != nil {
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
	var r chatResponse
	if err := json.Unmarshal(raw, &r); err != nil || r.Answer == "" {
		return agentkit.TurnOutput{}, fmt.Errorf("%w: unexpected response: %s", agentkit.ErrFatal, raw)
	}
	return agentkit.TurnOutput{
		Text: r.Answer, Raw: raw, TraceRef: r.TraceID,
		Tokens: r.Usage.Total, DurationMs: r.LatencyMs,
	}, nil
}
