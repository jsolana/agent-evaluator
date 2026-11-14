package hiringlab

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/internal/tracing"
)

// Fixture: a real /ask response recorded from the assigned project (contents trimmed).
func TestSpansFromRecordedResponse(t *testing.T) {
	raw, err := os.ReadFile("testdata/ask_response.json")
	if err != nil {
		t.Fatal(err)
	}
	spans, err := Source{}.Spans(context.Background(), tracing.Query{Raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Kind != "tool" || s.Name != "ragSearch" || s.Seq != 1 || s.DurationMs <= 0 || s.Status != "ok" {
		t.Fatalf("unexpected span %+v", s)
	}
	if len(s.Documents) != 6 || s.Documents[0].Rank != 1 || s.Documents[0].URL != "https://www.viabcp.com/seguridad/enlaces-correo" || s.Documents[0].Title == "" {
		t.Fatalf("documents not mapped: %+v", s.Documents)
	}
	var attrs map[string]any
	_ = json.Unmarshal(s.Attributes, &attrs)
	args, _ := attrs["arguments"].(map[string]any)
	if !strings.Contains(args["query"].(string), "phishing") {
		t.Fatalf("arguments must be kept: %s", s.Attributes)
	}
}

func TestSpansKeepErrorsAndNonSearchTools(t *testing.T) {
	raw := `{"traces":[
	  {"tool":"ragGetSourceChunks","arguments":{"sourceId":"d1","limit":8,"offset":0},"durationMs":40,"result":{"chunks":["a","b"]}},
	  {"tool":"ragSearch","arguments":{"query":"x"},"durationMs":10,"result":null,"error":"timeout"}]}`
	spans, err := Source{}.Spans(context.Background(), tracing.Query{Raw: json.RawMessage(raw)})
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 || spans[0].Name != "ragGetSourceChunks" || len(spans[0].Documents) != 0 ||
		!strings.Contains(string(spans[0].Attributes), "chunks") {
		t.Fatalf("unexpected spans %+v", spans)
	}
	if spans[1].Status != "error" || spans[1].Error != "timeout" {
		t.Fatalf("error not mapped: %+v", spans[1])
	}
}

func TestSpansWithoutTraces(t *testing.T) {
	spans, err := Source{}.Spans(context.Background(), tracing.Query{Raw: json.RawMessage(`{"answer":"hi"}`)})
	if err != nil || len(spans) != 0 {
		t.Fatalf("got %v %v", spans, err)
	}
	if _, err := (Source{}).Spans(context.Background(), tracing.Query{Raw: json.RawMessage(`not json`)}); err == nil {
		t.Fatal("want error on malformed raw response")
	}
}
