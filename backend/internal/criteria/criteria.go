// Package criteria defines the Criterion contract and the built-in criteria a
// scenario turn can declare. New criteria are added by implementing Criterion
// and registering a factory; nothing else in the system changes.
package criteria

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
)

// Exchange is one previous user/agent turn of the conversation.
type Exchange = domain.Exchange

// Input is everything a criterion may inspect for one turn.
type Input struct {
	History  []Exchange
	Question string
	Answer   string
	Spans    []domain.Span
}

type Criterion interface {
	Evaluate(ctx context.Context, in Input) (domain.CriterionResult, error)
}

// Factory builds a criterion from its params, validating them.
type Factory func(params json.RawMessage) (Criterion, error)

type Registry struct {
	factories map[string]Factory
}

// NewRegistry returns a registry with every built-in criterion.
func NewRegistry(judge Judge) *Registry {
	r := &Registry{factories: map[string]Factory{}}
	r.Register("tools_required", toolsFactory("tools_required", true))
	r.Register("tools_forbidden", toolsFactory("tools_forbidden", false))
	r.Register("sources_retrieved", sourcesFactory)
	r.Register("regex", regexFactory)
	r.Register("llm_judge", llmFactory("llm_judge", judge))
	r.Register("context_retention", llmFactory("context_retention", judge))
	return r
}

func (r *Registry) Register(typ string, f Factory) { r.factories[typ] = f }

func (r *Registry) Build(c domain.Criterion) (Criterion, error) {
	f, ok := r.factories[c.Type]
	if !ok {
		return nil, fmt.Errorf("unknown type %q", c.Type)
	}
	return f(c.Params)
}

// ValidateCriterion implements suite.CriterionValidator.
func (r *Registry) ValidateCriterion(typ string, params json.RawMessage) error {
	_, err := r.Build(domain.Criterion{Type: typ, Params: params})
	return err
}

// decode rejects unknown fields so a typo in a rubric fails at import time.
func decode(params json.RawMessage, v any) error {
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}
	d := json.NewDecoder(bytes.NewReader(params))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func result(typ, version string, passed bool, explanation string, evidence ...string) domain.CriterionResult {
	label := "fail"
	if passed {
		label = "pass"
	}
	return domain.CriterionResult{
		CriterionType: typ, CriterionVersion: version, Passed: passed,
		Label: label, Explanation: explanation, Evidence: evidence,
	}
}
