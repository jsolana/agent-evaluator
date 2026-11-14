package fake

import (
	"context"
	"encoding/json"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// Source reads the spans the fake agent embeds in its raw response.
type Source struct{}

func (Source) Spans(_ context.Context, q agentkit.TraceQuery) ([]agentkit.Span, error) {
	var r struct {
		Spans []agentkit.Span `json:"spans"`
	}
	if err := json.Unmarshal(q.Raw, &r); err != nil {
		return nil, err
	}
	for i := range r.Spans {
		r.Spans[i].Seq = i + 1
	}
	return r.Spans, nil
}
