// Package tracing defines how the core obtains the trace (spans) of a turn.
// Agents instrumented with OpenTelemetry would be queried from a collector by
// TraceRef; agents that return traces inline (e.g. Hiring Lab) parse Raw.
package tracing

import (
	"context"
	"encoding/json"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
)

type Query struct {
	TraceRef string
	Raw      json.RawMessage
}

type Source interface {
	Spans(ctx context.Context, q Query) ([]domain.Span, error)
}
