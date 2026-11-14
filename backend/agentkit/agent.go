// Package agentkit is the public contract between the evaluator and the system
// under evaluation. To evaluate your own agent, implement Agent (and optionally
// TraceSource), register a Factory with Register from an init function and run
// the evaluator from your own main. It depends on the standard library only.
package agentkit

import (
	"context"
	"encoding/json"
	"errors"
)

// Error classes let the runner decide on retries without knowing the adapter.
var (
	ErrRetryable = errors.New("retryable agent error")     // transient and safe to resend: the agent did not process the turn, or it deduplicates by idempotency key
	ErrConflict  = errors.New("conversation conflict")     // turn already running/failed or key reused: inspect, do not retry blindly
	ErrFatal     = errors.New("non-retryable agent error") // invalid input or credentials
)

type ConversationSpec struct {
	Config   json.RawMessage   // per-evaluation agent configuration (opaque to the core)
	Metadata map[string]any    // per-scenario metadata from the suite (opaque)
	Ref      string            // existing conversation to resume after a restart
	Labels   map[string]string // evaluation/scenario ids, for traceability in the agent's own tooling
}

type Agent interface {
	NewConversation(ctx context.Context, spec ConversationSpec) (Conversation, error)
}

// HealthChecker is an optional capability: agents that can verify their own
// configuration or reachability implement it, and the evaluator reports the
// result in its health endpoint. A failing check does not stop the server.
type HealthChecker interface {
	Check(ctx context.Context) error
}

// Idempotent is an optional capability: agents that deduplicate turns by
// TurnInput.IdempotencyKey implement it and return true. For any other agent
// the runner never resends a turn that may already have reached it.
type Idempotent interface {
	Idempotent() bool
}

type Conversation interface {
	Ref() string // external conversation id; empty until the first turn starts it
	Send(ctx context.Context, in TurnInput) (TurnOutput, error)
}

type TurnInput struct {
	Index          int
	Message        string
	IdempotencyKey string
	// History holds the completed turns of this conversation, also after a
	// restart. Agents that keep the conversation server-side may ignore it;
	// stateless agents (e.g. a chat completions API) send it with the message.
	History []Exchange
}

type TurnOutput struct {
	Text            string
	Raw             json.RawMessage // full adapter response, stored for audit and trace extraction
	TraceRef        string          // correlation id to fetch traces (e.g. an OTel trace id)
	Tokens          int
	DurationMs      int64
	EffectiveConfig json.RawMessage
}
