// Package fake is a deterministic in-memory agent for tests and local demos.
// It mimics the semantics of a stateful, idempotent agent:
// server-side conversation history, idempotency keys and 409 on concurrent turns.
package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// Reply decides the outcome of a turn. history holds the previous user
// messages of the same conversation, so tests can assert context is kept.
type Reply func(ref string, history []string, in agentkit.TurnInput) (text string, spans []agentkit.Span, err error)

type Agent struct {
	Reply Reply
	Delay time.Duration

	mu        sync.Mutex
	seq       int
	histories map[string][]string
	byKey     map[string]agentkit.TurnOutput
	inFlight  map[string]bool
	// Observations for assertions.
	Calls               int
	ConcurrentViolation bool
	MaxParallel, active int
}

func New(r Reply) *Agent {
	if r == nil {
		r = Echo
	}
	return &Agent{Reply: r, histories: map[string][]string{}, byKey: map[string]agentkit.TurnOutput{}, inFlight: map[string]bool{}}
}

// Echo answers "answer N: <message>" and reports one search span.
func Echo(_ string, history []string, in agentkit.TurnInput) (string, []agentkit.Span, error) {
	return fmt.Sprintf("answer %d: %s", len(history)+1, in.Message),
		[]agentkit.Span{{Kind: "tool", Name: "search", DurationMs: 120, Status: "ok", Documents: []agentkit.Document{{Rank: 1, URL: "https://example.com/doc"}}}}, nil
}

// Idempotent reports that the fake replays the saved output of a reused key.
func (a *Agent) Idempotent() bool { return true }

func (a *Agent) NewConversation(_ context.Context, spec agentkit.ConversationSpec) (agentkit.Conversation, error) {
	return &conversation{agent: a, ref: spec.Ref}, nil
}

type conversation struct {
	agent *Agent
	ref   string
}

func (c *conversation) Ref() string { return c.ref }

func (c *conversation) Send(ctx context.Context, in agentkit.TurnInput) (agentkit.TurnOutput, error) {
	a := c.agent
	a.mu.Lock()
	a.Calls++
	if out, ok := a.byKey[in.IdempotencyKey]; ok { // idempotent replay
		a.mu.Unlock()
		return out, nil
	}
	if c.ref == "" {
		a.seq++
		c.ref = fmt.Sprintf("fake-%d", a.seq)
	}
	if a.inFlight[c.ref] {
		a.ConcurrentViolation = true
		a.mu.Unlock()
		return agentkit.TurnOutput{}, fmt.Errorf("%w: turn already running", agentkit.ErrConflict)
	}
	a.inFlight[c.ref] = true
	a.active++
	if a.active > a.MaxParallel {
		a.MaxParallel = a.active
	}
	history := append([]string(nil), a.histories[c.ref]...)
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.inFlight[c.ref] = false
		a.active--
		a.mu.Unlock()
	}()

	select {
	case <-ctx.Done():
		return agentkit.TurnOutput{}, ctx.Err()
	case <-time.After(a.Delay):
	}

	text, spans, err := a.Reply(c.ref, history, in)
	if err != nil {
		return agentkit.TurnOutput{}, err
	}
	raw, _ := json.Marshal(map[string]any{"answer": text, "spans": spans})
	out := agentkit.TurnOutput{
		Text: text, Raw: raw, Tokens: 10, DurationMs: a.Delay.Milliseconds(),
		EffectiveConfig: json.RawMessage(`{"agent":"fake"}`),
	}
	a.mu.Lock()
	a.histories[c.ref] = append(a.histories[c.ref], in.Message)
	a.byKey[in.IdempotencyKey] = out
	a.mu.Unlock()
	return out, nil
}
