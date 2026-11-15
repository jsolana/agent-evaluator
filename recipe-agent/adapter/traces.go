package recipeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// Traces implements agentkit.TraceSource over the agent's OpenTelemetry spans.
// To read from Jaeger or Tempo instead, only the fetch and the field mapping change.
type Traces struct {
	URL  string
	HTTP *http.Client
}

var _ agentkit.TraceSource = Traces{}

type otelSpan struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id"`
	Context  struct {
		SpanID string `json:"span_id"`
	} `json:"context"`
	Start      time.Time      `json:"start_time"`
	End        time.Time      `json:"end_time"`
	Attributes map[string]any `json:"attributes"`
	Status     struct {
		Code        string `json:"status_code"`
		Description string `json:"description"`
	} `json:"status"`
}

func (t Traces) Spans(ctx context.Context, q agentkit.TraceQuery) ([]agentkit.Span, error) {
	if q.TraceRef == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL+"/traces/"+q.TraceRef, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traces: status %d", resp.StatusCode)
	}
	var body struct {
		Spans []otelSpan `json:"spans"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("traces: %w", err)
	}
	return toSpans(body.Spans), nil
}

// kinds maps OpenInference span kinds to agentkit kinds; anything else is "other".
var kinds = map[string]string{"LLM": "llm", "TOOL": "tool", "RETRIEVER": "retrieval"}

// toSpans maps OpenInference spans to agentkit spans. Spans keep their OpenInference
// attributes (messages, token counts, input/output) so the evaluator can show them.
func toSpans(in []otelSpan) []agentkit.Span {
	sort.Slice(in, func(i, j int) bool { return in[i].Start.Before(in[j].Start) })
	out := make([]agentkit.Span, 0, len(in))
	for i, s := range in {
		sp := agentkit.Span{
			SpanID: s.Context.SpanID, Seq: i + 1, Kind: "other", Name: s.Name,
			Start: s.Start, DurationMs: s.End.Sub(s.Start).Milliseconds(), Status: "ok",
		}
		if s.ParentID != nil {
			sp.ParentID = *s.ParentID
		}
		if k, ok := kinds[fmt.Sprint(s.Attributes["openinference.span.kind"])]; ok {
			sp.Kind = k
		}
		if m, ok := s.Attributes["llm.model_name"].(string); ok {
			sp.Name = m
		}
		if s.Status.Code == "ERROR" {
			sp.Status, sp.Error = "error", s.Status.Description
		}
		sp.Attributes, _ = json.Marshal(openInference(s.Attributes))
		out = append(out, sp)
	}
	return out
}

// openInference keeps the attributes defined by the OpenInference conventions.
func openInference(attrs map[string]any) map[string]any {
	kept := map[string]any{}
	for k, v := range attrs {
		for _, prefix := range []string{"llm.", "input.", "output.", "openinference."} {
			if strings.HasPrefix(k, prefix) {
				kept[k] = v
			}
		}
	}
	return kept
}
