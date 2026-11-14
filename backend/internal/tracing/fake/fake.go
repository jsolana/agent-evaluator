// Package fake reads the spans the fake agent embeds in its raw response.
package fake

import (
	"context"
	"encoding/json"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/tracing"
)

type Source struct{}

func (Source) Spans(_ context.Context, q tracing.Query) ([]domain.Span, error) {
	var r struct {
		Spans []domain.Span `json:"spans"`
	}
	if err := json.Unmarshal(q.Raw, &r); err != nil {
		return nil, err
	}
	for i := range r.Spans {
		r.Spans[i].Seq = i + 1
	}
	return r.Spans, nil
}
