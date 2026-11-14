// Package hiringlab adapts the Hiring Lab bank assistant (POST /ask) to agent.Agent.
// Every Hiring Lab particularity lives here: the URL layout, idempotency header,
// the JSON-wrapped answer and the error codes.
package hiringlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/agent"
)

type Config struct {
	BaseURL    string // HIRING_LAB_BASE_URL (project base URL)
	Token      string // HIRING_LAB_PROJECT_KEY (bearer)
	HTTPClient *http.Client
}

type Agent struct {
	base, token string
	http        *http.Client
}

func New(cfg Config) *Agent {
	c := cfg.HTTPClient
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Agent{
		base:  strings.TrimRight(cfg.BaseURL, "/"),
		token: cfg.Token, http: c,
	}
}

// Idempotent reports that Hiring Lab deduplicates turns by Idempotency-Key.
func (a *Agent) Idempotent() bool { return true }

// options are the per-evaluation settings Hiring Lab accepts on /ask.
type options struct {
	PromptID           string `json:"promptId,omitempty"`
	KnowledgeVersionID string `json:"knowledgeVersionId,omitempty"`
}

func (a *Agent) NewConversation(ctx context.Context, spec agent.ConversationSpec) (agent.Conversation, error) {
	var opts options
	if len(spec.Config) > 0 && string(spec.Config) != "null" {
		d := json.NewDecoder(bytes.NewReader(spec.Config))
		d.DisallowUnknownFields()
		if err := d.Decode(&opts); err != nil {
			return nil, fmt.Errorf("%w: agentConfig: %v", agent.ErrFatal, err)
		}
	}
	ref := spec.Ref
	if ref == "" && len(spec.Labels) > 0 {
		ref = a.createTagged(ctx, spec.Labels)
	}
	return &conversation{agent: a, opts: opts, ref: ref}, nil
}

// createTagged creates a conversation with title and metadata before the first
// /ask, so runs can be found in the platform's Conversations and Activity.
// It is best effort: on failure the first /ask starts an untagged conversation.
func (a *Agent) createTagged(ctx context.Context, labels map[string]string) string {
	title := "evaluator"
	if e, s := labels["evaluation"], labels["scenario"]; e != "" || s != "" {
		title = fmt.Sprintf("evaluator · evaluation %s · %s", e, s)
	}
	body, _ := json.Marshal(map[string]any{"data": map[string]any{"title": title, "metadata": labels}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/conversations", bytes.NewReader(body))
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var created struct {
		ID string `json:"id"`
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return ""
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)
	return created.ID
}

type conversation struct {
	agent *Agent
	opts  options
	ref   string
}

func (c *conversation) Ref() string { return c.ref }

func (c *conversation) Send(ctx context.Context, in agent.TurnInput) (agent.TurnOutput, error) {
	body, _ := json.Marshal(struct {
		Question       string `json:"question"`
		ConversationID string `json:"conversationId,omitempty"`
		options
	}{in.Message, c.ref, c.opts})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.agent.base+"/ask", bytes.NewReader(body))
	if err != nil {
		return agent.TurnOutput{}, fmt.Errorf("%w: %v", agent.ErrFatal, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.agent.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", in.IdempotencyKey)

	resp, err := c.agent.http.Do(req)
	if err != nil {
		return agent.TurnOutput{}, fmt.Errorf("%w: %v", agent.ErrRetryable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var r struct {
		Answer         string          `json:"answer"`
		ConversationID string          `json:"conversationId"`
		Tokens         int             `json:"tokens"`
		DurationMs     int64           `json:"durationMs"`
		Snapshot       json.RawMessage `json:"snapshot"`
	}
	_ = json.Unmarshal(raw, &r)
	if r.ConversationID != "" && c.ref == "" {
		c.ref = r.ConversationID // errors of a reserved turn also carry it
	}

	switch {
	case resp.StatusCode == http.StatusConflict:
		return agent.TurnOutput{}, fmt.Errorf("%w: %s", agent.ErrConflict, raw)
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return agent.TurnOutput{}, fmt.Errorf("%w: status %d: %s", agent.ErrRetryable, resp.StatusCode, raw)
	case resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated:
		return agent.TurnOutput{}, fmt.Errorf("%w: status %d: %s", agent.ErrFatal, resp.StatusCode, raw)
	}
	return agent.TurnOutput{
		Text: answerText(r.Answer), Raw: raw, Tokens: r.Tokens,
		DurationMs: r.DurationMs, EffectiveConfig: compact(r.Snapshot),
	}, nil
}

// answerText unwraps {"text_response": "..."}; any other answer is returned as is.
func answerText(answer string) string {
	var wrapped struct {
		TextResponse *string `json:"text_response"`
	}
	if json.Unmarshal([]byte(answer), &wrapped) == nil && wrapped.TextResponse != nil {
		return *wrapped.TextResponse
	}
	return answer
}

// compact re-encodes JSON with sorted keys so equal configs compare equal.
func compact(raw json.RawMessage) json.RawMessage {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return raw
	}
	out, _ := json.Marshal(v)
	return out
}
