// Package agenttest is a conformance suite for agentkit.Agent implementations.
// Call Run from the adapter's own tests to check that it honours the contract
// the evaluator relies on.
package agenttest

import (
	"context"
	"testing"
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// Run exercises the agent returned by newAgent with a short conversation. The
// agent must answer any question; it needs no particular content.
func Run(t *testing.T, newAgent func(t *testing.T) agentkit.Agent) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	turn := func(t *testing.T, c agentkit.Conversation, i int, msg, key string, history []agentkit.Exchange) agentkit.TurnOutput {
		t.Helper()
		out, err := c.Send(ctx, agentkit.TurnInput{Index: i, Message: msg, IdempotencyKey: key, History: history})
		if err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
		if out.Text == "" {
			t.Fatalf("turn %d: empty answer text", i)
		}
		return out
	}

	t.Run("answers turns of one conversation", func(t *testing.T) {
		c, err := newAgent(t).NewConversation(ctx, agentkit.ConversationSpec{})
		if err != nil {
			t.Fatal(err)
		}
		first := turn(t, c, 1, "Hello", "key-1", nil)
		ref := c.Ref()
		turn(t, c, 2, "And now?", "key-2", []agentkit.Exchange{{Question: "Hello", Answer: first.Text}})
		if c.Ref() != ref {
			t.Fatalf("Ref changed between turns: %q -> %q", ref, c.Ref())
		}
	})

	t.Run("resumes a conversation by Ref", func(t *testing.T) {
		a := newAgent(t)
		c, err := a.NewConversation(ctx, agentkit.ConversationSpec{})
		if err != nil {
			t.Fatal(err)
		}
		turn(t, c, 1, "Hello", "key-1", nil)
		ref := c.Ref()
		if ref == "" {
			t.Skip("stateless agent: nothing to resume")
		}
		resumed, err := a.NewConversation(ctx, agentkit.ConversationSpec{Ref: ref})
		if err != nil {
			t.Fatal(err)
		}
		if resumed.Ref() != ref {
			t.Fatalf("resumed Ref = %q, want %q", resumed.Ref(), ref)
		}
	})

	t.Run("replays a turn sent twice with the same key", func(t *testing.T) {
		a := newAgent(t)
		if i, ok := a.(agentkit.Idempotent); !ok || !i.Idempotent() {
			t.Skip("agent does not declare itself idempotent")
		}
		c, err := a.NewConversation(ctx, agentkit.ConversationSpec{})
		if err != nil {
			t.Fatal(err)
		}
		one := turn(t, c, 1, "Hello", "same-key", nil)
		two := turn(t, c, 1, "Hello", "same-key", nil)
		if one.Text != two.Text {
			t.Fatalf("a repeated key must replay the first answer: %q vs %q", one.Text, two.Text)
		}
	})

	t.Run("reports health when it can", func(t *testing.T) {
		h, ok := newAgent(t).(agentkit.HealthChecker)
		if !ok {
			t.Skip("agent has no health check")
		}
		if err := h.Check(ctx); err != nil {
			t.Fatalf("a configured agent must be healthy: %v", err)
		}
	})
}
