// Package suite parses and validates scenario suites written in YAML.
package suite

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jsolana/agent-evaluator/backend/internal/domain"
	"gopkg.in/yaml.v3"
)

// CriterionValidator checks that a criterion type exists and its params are valid.
// It is implemented by the criteria registry and injected here to keep this
// package independent of concrete criteria.
type CriterionValidator interface {
	ValidateCriterion(typ string, params json.RawMessage) error
}

type Options struct {
	MinTurns  int // minimum turns per scenario; a server policy (SUITE_MIN_TURNS), not overridable by suites
	Validator CriterionValidator
}

// ValidationError lists every problem found, each prefixed with its path.
type ValidationError []string

func (v ValidationError) Error() string { return "invalid suite:\n" + strings.Join(v, "\n") }

type fileSuite struct {
	Name      string         `yaml:"name"`
	MinTurns  *int           `yaml:"minTurns"` // only to reject it: the minimum is set by the server
	Scenarios []fileScenario `yaml:"scenarios"`
}

type fileScenario struct {
	Key      string         `yaml:"key"`
	Name     string         `yaml:"name"`
	Tags     []string       `yaml:"tags"`
	Metadata map[string]any `yaml:"metadata"`
	Turns    []fileTurn     `yaml:"turns"`
}

type fileTurn struct {
	Question string           `yaml:"question"`
	Criteria []map[string]any `yaml:"criteria"`
}

// Parse decodes and validates a suite. On validation failure it returns a
// ValidationError with all problems, so the user can fix them in one pass.
func Parse(src []byte, opts Options) (domain.Suite, error) {
	var f fileSuite
	if err := yaml.Unmarshal(src, &f); err != nil {
		return domain.Suite{}, fmt.Errorf("malformed YAML: %w", err)
	}
	if len(f.Scenarios) == 0 {
		return domain.Suite{}, ValidationError{"scenarios: at least one scenario is required"}
	}

	var errs ValidationError
	add := func(path, msg string, args ...any) { errs = append(errs, path+": "+fmt.Sprintf(msg, args...)) }

	if strings.TrimSpace(f.Name) == "" {
		add("name", "is required")
	}
	if f.MinTurns != nil {
		add("minTurns", "is set by the server (SUITE_MIN_TURNS=%d), remove it from the suite", opts.MinTurns)
	}
	minTurns := opts.MinTurns

	s := domain.Suite{Name: f.Name, MinTurns: minTurns, Source: string(src), ContentHash: hash(src)}
	seen := map[string]bool{}
	for i, fs := range f.Scenarios {
		p := fmt.Sprintf("scenarios[%d]", i)
		sc := domain.Scenario{Key: fs.Key, Name: fs.Name, Tags: fs.Tags, Metadata: fs.Metadata}
		if sc.Key == "" && len(fs.Turns) > 0 {
			sc.Key = hash([]byte(fs.Turns[0].Question))[:12]
		}
		if seen[sc.Key] {
			add(p+".key", "duplicate key %q", sc.Key)
		}
		seen[sc.Key] = true
		if len(fs.Turns) < minTurns {
			add(p+".turns", "has %d turns, minimum is %d", len(fs.Turns), minTurns)
		}
		for j, ft := range fs.Turns {
			tp := fmt.Sprintf("%s.turns[%d]", p, j)
			turn, turnErrs := parseTurn(ft, j+1, tp, opts.Validator)
			errs = append(errs, turnErrs...)
			sc.Turns = append(sc.Turns, turn)
		}
		s.Scenarios = append(s.Scenarios, sc)
	}
	if len(errs) > 0 {
		return domain.Suite{}, errs
	}
	return s, nil
}

func parseTurn(ft fileTurn, index int, path string, v CriterionValidator) (domain.Turn, ValidationError) {
	var errs ValidationError
	if strings.TrimSpace(ft.Question) == "" {
		errs = append(errs, path+".question: is required")
	}
	if len(ft.Criteria) == 0 {
		errs = append(errs, path+".criteria: at least one criterion is required")
	}
	t := domain.Turn{Index: index, Question: ft.Question}
	for k, raw := range ft.Criteria {
		cp := fmt.Sprintf("%s.criteria[%d]", path, k)
		c, err := parseCriterion(raw, v)
		if err != nil {
			errs = append(errs, cp+": "+err.Error())
			continue
		}
		t.Criteria = append(t.Criteria, c)
	}
	canonical, _ := json.Marshal(struct {
		Q string
		C []domain.Criterion
	}{t.Question, t.Criteria})
	t.ContentHash = hash(canonical)
	return t, errs
}

func parseCriterion(raw map[string]any, v CriterionValidator) (domain.Criterion, error) {
	typ, _ := raw["type"].(string)
	if typ == "" {
		return domain.Criterion{}, errors.New("type is required")
	}
	rest := make(map[string]any, len(raw))
	for k, val := range raw {
		if k != "type" {
			rest[k] = val
		}
	}
	params, err := json.Marshal(rest) // map keys are sorted: stable hashes
	if err != nil {
		return domain.Criterion{}, err
	}
	if err := v.ValidateCriterion(typ, params); err != nil {
		return domain.Criterion{}, err
	}
	return domain.Criterion{Type: typ, Params: params}, nil
}

func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
