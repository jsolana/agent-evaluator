package main

import (
	"strings"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/internal/criteria"
	"github.com/jsolana/agent-evaluator/backend/internal/suite"
)

const legacy = `name: "Synthetic"
scenarios:
  - id: "0b97"
    name: "card | ambiguous"
    tags: [cards]
    turns:
      - question: "¿cómo lo activo?"
        expectedAnswer: "Pide aclaración"
        expectedTools: []
        forbiddenTools: [ragSearch, ragGetSourceChunks]
        sources: []
      - question: "la tarjeta"
        expectedAnswer: "Pasos de activación"
        expectedTools: [ragSearch]
        sources: ["https://www.viabcp.com/x"]
`

func TestConvertLegacyScenarios(t *testing.T) {
	out, err := convert([]byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	s, err := suite.Parse(out, suite.Options{MinTurns: 2, Validator: criteria.NewRegistry(nil)})
	if err != nil {
		t.Fatalf("converted suite must be valid: %v\n%s", err, out)
	}
	sc := s.Scenarios[0]
	if strings.Contains(string(out), "minTurns") || sc.Key != "0b97" || sc.Tags[0] != "cards" {
		t.Fatalf("got %+v", s)
	}
	types := func(i int) string {
		var ts []string
		for _, c := range sc.Turns[i].Criteria {
			ts = append(ts, c.Type)
		}
		return strings.Join(ts, ",")
	}
	if types(0) != "llm_judge,tools_forbidden" || types(1) != "llm_judge,tools_required,sources_retrieved" {
		t.Fatalf("criteria: %q / %q", types(0), types(1))
	}
}
