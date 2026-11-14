package httpagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// source maps an array of the response to spans, as the traces section says.
type source struct{ cfg *TracesConfig }

func (t source) Spans(_ context.Context, q agentkit.TraceQuery) ([]agentkit.Span, error) {
	var resp any
	if err := json.Unmarshal(q.Raw, &resp); err != nil {
		return nil, fmt.Errorf("httpagent traces: %w", err)
	}
	node, ok := lookup(resp, t.cfg.From)
	if !ok {
		return nil, nil // the turn carried no trace
	}
	items, _ := node.([]any)
	spans := make([]agentkit.Span, 0, len(items))
	for i, item := range items {
		name, _ := lookupString(item, t.cfg.Name)
		sp := agentkit.Span{
			SpanID: strconv.Itoa(i + 1), Seq: i + 1, Kind: "tool", Name: name,
			DurationMs: int64(lookupInt(item, t.cfg.DurationMs)), Status: "ok",
		}
		if msg, _ := lookupString(item, t.cfg.Error); msg != "" {
			sp.Status, sp.Error = "error", msg
		}
		attrs := map[string]any{}
		if args, ok := lookup(item, t.cfg.Arguments); ok {
			attrs[agentkit.SpanAttrArguments] = args
		}
		if d := t.cfg.Documents; d != nil {
			sp.Documents = documents(item, d)
		}
		sp.Attributes, _ = json.Marshal(attrs)
		spans = append(spans, sp)
	}
	return spans, nil
}

func documents(item any, d *struct {
	From    string `yaml:"from"`
	Title   string `yaml:"title"`
	URL     string `yaml:"url"`
	Content string `yaml:"content"`
}) []agentkit.Document {
	node, _ := lookup(item, d.From)
	list, _ := node.([]any)
	var docs []agentkit.Document
	for _, e := range list {
		doc := agentkit.Document{Rank: len(docs) + 1}
		doc.Title, _ = lookupString(e, d.Title)
		doc.URL, _ = lookupString(e, d.URL)
		doc.Content, _ = lookupString(e, d.Content)
		docs = append(docs, doc)
	}
	return docs
}
