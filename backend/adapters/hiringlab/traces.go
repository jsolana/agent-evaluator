package hiringlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// Source extracts spans from the traces Hiring Lab returns inline in the /ask
// response. No extra calls: the stored raw response is the source.
type Source struct{}

var _ agentkit.TraceSource = Source{}

func (Source) Spans(_ context.Context, q agentkit.TraceQuery) ([]agentkit.Span, error) {
	var resp struct {
		Traces []struct {
			Tool       string          `json:"tool"`
			Arguments  json.RawMessage `json:"arguments"`
			Result     json.RawMessage `json:"result"`
			DurationMs int64           `json:"durationMs"`
			Error      string          `json:"error"`
		} `json:"traces"`
	}
	if err := json.Unmarshal(q.Raw, &resp); err != nil {
		return nil, fmt.Errorf("hiringlab traces: %w", err)
	}
	spans := make([]agentkit.Span, 0, len(resp.Traces))
	for i, t := range resp.Traces {
		s := agentkit.Span{
			SpanID: strconv.Itoa(i + 1), Seq: i + 1, Kind: "tool", Name: t.Tool,
			DurationMs: t.DurationMs, Status: "ok", Error: t.Error,
		}
		if t.Error != "" {
			s.Status = "error"
		}
		s.Documents = documents(t.Result)
		attrs := map[string]json.RawMessage{agentkit.SpanAttrArguments: t.Arguments}
		if len(s.Documents) == 0 && len(t.Result) > 0 {
			attrs[agentkit.SpanAttrResult] = t.Result // documents already hold search results; keep other results verbatim
		}
		s.Attributes, _ = json.Marshal(attrs)
		spans = append(spans, s)
	}
	return spans, nil
}

// documents maps a ragSearch result (array of {title,url,content}) to documents.
func documents(result json.RawMessage) []agentkit.Document {
	var items []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	}
	if json.Unmarshal(result, &items) != nil {
		return nil
	}
	var docs []agentkit.Document
	for i, it := range items {
		if it.URL == "" && it.Content == "" {
			continue
		}
		docs = append(docs, agentkit.Document{Rank: i + 1, Title: it.Title, URL: it.URL, Content: it.Content})
	}
	return docs
}
