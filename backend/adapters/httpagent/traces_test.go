package httpagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

const tracesYAML = `
baseUrl: http://x
turn: {path: /c}
response: {text: answer}
traces:
  from: calls
  name: tool
  arguments: args
  durationMs: ms
  error: err
  documents: {from: found, url: link, title: name, content: body}
`

func TestSpansAreMappedFromTheResponse(t *testing.T) {
	s, err := load([]byte(tracesYAML), env(nil))
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"calls":[
	  {"tool":"search","args":{"q":"fees"},"ms":40,"found":[{"link":"https://e.com/a","name":"A","body":"text"},{"link":"https://e.com/b"}]},
	  {"tool":"fetch","ms":10,"err":"timeout"}]}`)
	spans, err := source{cfg: s.Traces}.Spans(context.Background(), agentkit.TraceQuery{Raw: raw})
	if err != nil || len(spans) != 2 {
		t.Fatalf("spans=%+v err=%v", spans, err)
	}
	first, second := spans[0], spans[1]
	if first.Name != "search" || first.DurationMs != 40 || first.Status != "ok" || first.Seq != 1 || len(first.Documents) != 2 ||
		first.Documents[0].URL != "https://e.com/a" || first.Documents[1].Rank != 2 || string(first.Arguments()) != `{"q":"fees"}` {
		t.Fatalf("first span: %+v", first)
	}
	if second.Status != "error" || second.Error != "timeout" {
		t.Fatalf("second span: %+v", second)
	}
}

func TestTurnsWithoutTraceYieldNoSpans(t *testing.T) {
	s, _ := load([]byte(tracesYAML), env(nil))
	spans, err := source{cfg: s.Traces}.Spans(context.Background(), agentkit.TraceQuery{Raw: json.RawMessage(`{"answer":"x"}`)})
	if err != nil || len(spans) != 0 {
		t.Fatalf("spans=%+v err=%v", spans, err)
	}
}
