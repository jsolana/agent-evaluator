package suite

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// knownTypes is a test double for the criteria registry.
type knownTypes map[string]bool

func (k knownTypes) ValidateCriterion(typ string, params json.RawMessage) error {
	if !k[typ] {
		return fmt.Errorf("unknown type %q", typ)
	}
	var p map[string]any
	_ = json.Unmarshal(params, &p)
	if typ == "llm_judge" && p["expected"] == nil {
		return errors.New("expected is required")
	}
	return nil
}

var validator = knownTypes{"llm_judge": true, "tools_required": true}

const validYAML = `
name: Synthetic
scenarios:
  - key: mortgage
    name: "mortgage | complex"
    tags: [mortgage]
    metadata: {channel: web}
    turns:
      - question: "¿MiVivienda o tradicional?"
        criteria:
          - type: llm_judge
            expected: "Describe ambos"
          - type: tools_required
            tools: [search]
      - question: "¿Y la cuota?"
        criteria: [{type: llm_judge, expected: "Sigue con el hipotecario"}]
      - question: "Resume"
        criteria: [{type: llm_judge, expected: "Resume ambos turnos"}]
`

func TestParseValidSuite(t *testing.T) {
	s, err := Parse([]byte(validYAML), Options{MinTurns: 3, Validator: validator})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if s.Name != "Synthetic" || s.MinTurns != 3 || len(s.Scenarios) != 1 {
		t.Fatalf("unexpected suite: %+v", s)
	}
	sc := s.Scenarios[0]
	if sc.Key != "mortgage" || sc.Metadata["channel"] != "web" || len(sc.Turns) != 3 {
		t.Fatalf("unexpected scenario: %+v", sc)
	}
	if sc.Turns[0].Index != 1 || sc.Turns[2].Index != 3 {
		t.Fatalf("turn indexes must be 1-based: %+v", sc.Turns)
	}
	c := sc.Turns[0].Criteria[1]
	if c.Type != "tools_required" || string(c.Params) != `{"tools":["search"]}` {
		t.Fatalf("criterion params must exclude type: %s %s", c.Type, c.Params)
	}
	if s.ContentHash == "" || sc.Turns[0].ContentHash == "" || s.Source != validYAML {
		t.Fatal("hashes and source must be set")
	}
}

// The minimum number of turns is a server policy: a suite cannot lower it.
func TestMinTurnsIsControlledByTheServer(t *testing.T) {
	_, err := Parse([]byte("minTurns: 1\n"+validYAML), Options{MinTurns: 3, Validator: validator})
	var verr ValidationError
	if !errors.As(err, &verr) || !strings.Contains(verr.Error(), "minTurns: is set by the server") {
		t.Fatalf("a suite-level minTurns must be rejected, got %v", err)
	}
	s, err := Parse([]byte(validYAML), Options{MinTurns: 4, Validator: validator})
	if err == nil || s.MinTurns != 0 {
		t.Fatal("the server minimum (4) must apply to a 3-turn scenario")
	}
}

func TestTurnHashChangesOnlyWhenTheTurnChanges(t *testing.T) {
	a, _ := Parse([]byte(validYAML), Options{MinTurns: 3, Validator: validator})
	b, _ := Parse([]byte(strings.Replace(validYAML, "Describe ambos", "Describe ambos sin recomendar", 1)), Options{MinTurns: 3, Validator: validator})
	ta, tb := a.Scenarios[0].Turns, b.Scenarios[0].Turns
	if ta[0].ContentHash == tb[0].ContentHash {
		t.Fatal("turn 1 hash must change when its rubric changes")
	}
	if ta[1].ContentHash != tb[1].ContentHash {
		t.Fatal("turn 2 hash must not change")
	}
}

func TestParseReportsAllErrorsWithPaths(t *testing.T) {
	src := `
scenarios:
  - key: a
    turns:
      - question: ""
        criteria: [{type: llm_jugde}]
      - question: "q2"
        criteria: []
  - key: a
    turns:
      - question: "q"
        criteria: [{type: llm_judge}]
`
	_, err := Parse([]byte(src), Options{MinTurns: 3, Validator: validator})
	var verr ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	want := []string{
		"name: is required",
		"scenarios[0].turns: has 2 turns, minimum is 3",
		"scenarios[0].turns[0].question: is required",
		`scenarios[0].turns[0].criteria[0]: unknown type "llm_jugde"`,
		"scenarios[0].turns[1].criteria: at least one criterion is required",
		`scenarios[1].key: duplicate key "a"`,
		"scenarios[1].turns: has 1 turns, minimum is 3",
		"scenarios[1].turns[0].criteria[0]: expected is required",
	}
	if strings.Join(verr, "\n") != strings.Join(want, "\n") {
		t.Fatalf("errors:\n%s\nwant:\n%s", strings.Join(verr, "\n"), strings.Join(want, "\n"))
	}
}

func TestParseRejectsMalformedYAMLAndEmptySuites(t *testing.T) {
	for name, src := range map[string]string{
		"malformed":    "name: [unclosed",
		"no scenarios": "name: x\nscenarios: []",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(src), Options{MinTurns: 1, Validator: validator}); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestMissingKeyIsDerivedFromFirstQuestion(t *testing.T) {
	src := strings.Replace(validYAML, "key: mortgage\n    name:", "name:", 1)
	a, err := Parse([]byte(src), Options{MinTurns: 3, Validator: validator})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	b, _ := Parse([]byte(src), Options{MinTurns: 3, Validator: validator})
	if a.Scenarios[0].Key == "" || a.Scenarios[0].Key != b.Scenarios[0].Key {
		t.Fatalf("derived key must be stable, got %q and %q", a.Scenarios[0].Key, b.Scenarios[0].Key)
	}
}
