package criteria

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jsolana/agent-evaluator/backend/criterionkit"
	"github.com/jsolana/agent-evaluator/backend/internal/domain"
)

const externalDefaultVersion = "1"

// Adopt makes the criteria registered through criterionkit available to suites.
// A type that is already built in is an error, so a package can never silently
// replace a core check.
func (r *Registry) Adopt(ext *criterionkit.Registry) error {
	for _, typ := range ext.Names() {
		if _, builtIn := r.factories[typ]; builtIn {
			return fmt.Errorf("criterion %q is built in; register your criterion under another name", typ)
		}
		f, _ := ext.Lookup(typ)
		r.Register(typ, externalFactory(typ, f))
	}
	return nil
}

func externalFactory(typ string, f criterionkit.Factory) Factory {
	return func(params json.RawMessage) (Criterion, error) {
		c, err := f(params)
		if err != nil {
			return nil, err
		}
		return external{typ: typ, c: c}, nil
	}
}

// external adapts a criterionkit.Criterion to the core's. It owns what a third
// party must not have to know: the label, the type, a default version and
// isolation from panics.
type external struct {
	typ string
	c   criterionkit.Criterion
}

func (e external) Evaluate(ctx context.Context, in Input) (res domain.CriterionResult, err error) {
	defer func() {
		if p := recover(); p != nil {
			res, err = domain.CriterionResult{}, fmt.Errorf("criterion %s panicked: %v", e.typ, p)
		}
	}()
	got, err := e.c.Evaluate(ctx, criterionkit.Input{
		History: in.History, Question: in.Question, Answer: in.Answer, Spans: in.Spans,
		DurationMs: in.DurationMs, Tokens: in.Tokens,
	})
	if err != nil {
		return domain.CriterionResult{}, err
	}
	version := got.Version
	if version == "" {
		version = externalDefaultVersion
	}
	out := result(e.typ, version, got.Passed, strings.TrimSpace(got.Explanation), got.Evidence...)
	out.Score, out.CostTokens = got.Score, got.Tokens
	return out, nil
}
