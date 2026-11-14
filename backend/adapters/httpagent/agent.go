// Package httpagent is a generic adapter for REST agents, driven by a YAML
// description instead of Go code: where to call, how to build the request from
// the conversation, and where to find the answer in the response. Agents that do
// not fit this shape (streaming, token refresh, several calls per turn) need an
// adapter written against agentkit.
package httpagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// Agent talks to a REST agent as described by its configuration.
type Agent struct {
	spec *spec
	http *http.Client
}

func newAgent(s *spec, c *http.Client) *Agent {
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Agent{spec: s, http: c}
}

// Check implements agentkit.HealthChecker: every ${NAME} the configuration
// references must be set.
func (a *Agent) Check(ctx context.Context) error {
	return agentkit.Require(a.spec.env, a.spec.required...)(ctx)
}

// Idempotent implements agentkit.Idempotent: the agent deduplicates turns when
// the configuration names the header that carries the idempotency key.
func (a *Agent) Idempotent() bool { return a.spec.Turn.IdempotencyHeader != "" }

func (a *Agent) NewConversation(_ context.Context, cs agentkit.ConversationSpec) (agentkit.Conversation, error) {
	var config any
	if len(cs.Config) > 0 && string(cs.Config) != "null" {
		if err := json.Unmarshal(cs.Config, &config); err != nil {
			return nil, fmt.Errorf("%w: agentConfig: %v", agentkit.ErrFatal, err)
		}
	}
	c := &conversation{agent: a, config: config}
	if a.spec.stateful {
		c.ref = cs.Ref
	}
	return c, nil
}

type conversation struct {
	agent  *Agent
	config any
	ref    string
}

func (c *conversation) Ref() string { return c.ref }

func (c *conversation) Send(ctx context.Context, in agentkit.TurnInput) (agentkit.TurnOutput, error) {
	s := c.agent.spec
	body, err := s.renderBody(newTemplateData(in, c.ref, c.config))
	if err != nil {
		return agentkit.TurnOutput{}, fmt.Errorf("%w: %v", agentkit.ErrFatal, err)
	}
	req, err := http.NewRequestWithContext(ctx, s.Turn.Method, s.BaseURL+s.Turn.Path, bytes.NewReader(body))
	if err != nil {
		return agentkit.TurnOutput{}, fmt.Errorf("%w: %v", agentkit.ErrFatal, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range s.Headers {
		req.Header.Set(k, v)
	}
	if h := s.Turn.IdempotencyHeader; h != "" {
		req.Header.Set(h, in.IdempotencyKey)
	}

	start := time.Now()
	resp, err := c.agent.http.Do(req)
	if err != nil { // the request may not have been processed
		return agentkit.TurnOutput{}, fmt.Errorf("%w: %v", agentkit.ErrRetryable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return agentkit.TurnOutput{}, classify(s, resp.StatusCode, raw)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return agentkit.TurnOutput{}, fmt.Errorf("%w: the response is not JSON: %v", agentkit.ErrFatal, err)
	}
	text, ok := lookupString(decoded, s.Response.Text)
	if !ok {
		return agentkit.TurnOutput{}, fmt.Errorf("%w: response.text %q is not a string in %s", agentkit.ErrFatal, s.Response.Text, raw)
	}
	if s.stateful {
		if ref, ok := lookupString(decoded, s.Response.Ref); ok && c.ref == "" {
			c.ref = ref
		}
	}
	return agentkit.TurnOutput{
		Text: text, Raw: raw, Tokens: lookupInt(decoded, s.Response.Tokens), DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// classify turns a non-2xx status into the error class the runner understands.
func classify(s *spec, status int, body []byte) error {
	switch {
	case s.conflict.has(status):
		return fmt.Errorf("%w: status %d: %s", agentkit.ErrConflict, status, body)
	case s.retryable.has(status):
		return fmt.Errorf("%w: status %d", agentkit.ErrRetryable, status)
	default:
		return fmt.Errorf("%w: status %d: %s", agentkit.ErrFatal, status, body)
	}
}
