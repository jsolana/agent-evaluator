package agentkit

import (
	"context"
	"encoding/json"
	"time"
)

// TraceSource tells the evaluator how to obtain the trace (spans) of a turn.
// Agents instrumented with OpenTelemetry would be queried from a collector by
// TraceRef; agents that return traces inline parse Raw. Implementing it is
// optional: without a source the evaluator records turns with no spans, and
// only criteria that do not need them (regex, llm_judge...) can be used.
type TraceSource interface {
	Spans(ctx context.Context, q TraceQuery) ([]Span, error)
}

// TraceQuery identifies the turn whose spans are requested.
type TraceQuery struct {
	TraceRef string          // TurnOutput.TraceRef
	Raw      json.RawMessage // TurnOutput.Raw
}

// NoTraces is the TraceSource of agents that expose no trace.
type NoTraces struct{}

func (NoTraces) Spans(context.Context, TraceQuery) ([]Span, error) { return nil, nil }

// Exchange is one completed user/agent turn of a conversation.
type Exchange struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Attribute keys of Span.Attributes shared by every trace source, so the core
// can read them without knowing which adapter produced the span.
const (
	SpanAttrArguments = "arguments" // input of a tool call
	SpanAttrResult    = "result"    // output of a tool call not already mapped to Documents
)

// Span is one step of a turn's trace (a tool call today, an LLM call in other systems).
type Span struct {
	SpanID     string          `json:"spanId"`
	ParentID   string          `json:"parentId"`
	Seq        int             `json:"seq"`
	Kind       string          `json:"kind"` // tool | llm | retrieval | other
	Name       string          `json:"name"`
	Start      time.Time       `json:"start"`
	DurationMs int64           `json:"durationMs"`
	Attributes json.RawMessage `json:"attributes,omitempty"`
	Documents  []Document      `json:"documents,omitempty"`
	Status     string          `json:"status"`
	Error      string          `json:"error,omitempty"`
}

// Arguments returns the tool-call input stored under SpanAttrArguments, or nil.
func (s Span) Arguments() json.RawMessage {
	var attrs map[string]json.RawMessage
	if json.Unmarshal(s.Attributes, &attrs) != nil {
		return nil
	}
	return attrs[SpanAttrArguments]
}

// Document is a source retrieved by a step of the trace.
type Document struct {
	Rank    int    `json:"rank"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}
