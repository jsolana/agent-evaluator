// Package tracing defines how the core obtains the trace (spans) of a turn.
// Agents instrumented with OpenTelemetry would be queried from a collector by
// TraceRef; agents that return traces inline parse Raw.
package tracing

import (
	"context"
	"encoding/json"

	"github.com/jsolana/agent-evaluator/backend/internal/domain"
)

type Query struct {
	TraceRef string
	Raw      json.RawMessage
}

type Source interface {
	Spans(ctx context.Context, q Query) ([]domain.Span, error)
}
