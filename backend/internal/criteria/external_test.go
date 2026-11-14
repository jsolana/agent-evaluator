package criteria

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/criterionkit"
	"github.com/jsolana/agent-evaluator/backend/internal/domain"
)

type stubCriterion struct {
	res  criterionkit.Result
	err  error
	fn   func(criterionkit.Input)
	boom bool
}

func (s stubCriterion) Evaluate(_ context.Context, in criterionkit.Input) (criterionkit.Result, error) {
	if s.boom {
		panic("kaboom")
	}
	if s.fn != nil {
		s.fn(in)
	}
	return s.res, s.err
}

func adopt(t *testing.T, c stubCriterion) *Registry {
	t.Helper()
	ext := criterionkit.NewRegistry()
	ext.Register("custom", func(p json.RawMessage) (criterionkit.Criterion, error) {
		var v struct{ N int }
		if err := criterionkit.DecodeParams(p, &v); err != nil {
			return nil, err
		}
		return c, nil
	})
	r := NewRegistry(nil)
	if err := r.Adopt(ext); err != nil {
		t.Fatal(err)
	}
	return r
}

func eval(t *testing.T, r *Registry, in Input) (domain.CriterionResult, error) {
	t.Helper()
	c, err := r.Build(domain.Criterion{Type: "custom", Params: json.RawMessage(`{"n":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	return c.Evaluate(context.Background(), in)
}

func TestAdoptedCriterionBecomesACoreResult(t *testing.T) {
	score := 0.5
	var seen criterionkit.Input
	r := adopt(t, stubCriterion{
		res: criterionkit.Result{Passed: false, Score: &score, Explanation: " too long ", Evidence: []string{"x"}, Tokens: 9},
		fn:  func(in criterionkit.Input) { seen = in },
	})
	res, err := eval(t, r, Input{Question: "q", Answer: "a", History: []Exchange{{Question: "p", Answer: "r"}}, DurationMs: 120, Tokens: 30})
	if err != nil {
		t.Fatal(err)
	}
	if res.CriterionType != "custom" || res.CriterionVersion != "1" || res.Passed || res.Label != "fail" ||
		res.Explanation != "too long" || res.CostTokens != 9 || res.Score == nil || *res.Score != 0.5 || len(res.Evidence) != 1 {
		t.Fatalf("result: %+v", res)
	}
	if seen.Question != "q" || seen.Answer != "a" || len(seen.History) != 1 || seen.DurationMs != 120 || seen.Tokens != 30 {
		t.Fatalf("the criterion must receive the turn: %+v", seen)
	}
}

func TestAdoptedCriterionKeepsItsVersionAndParamValidation(t *testing.T) {
	r := adopt(t, stubCriterion{res: criterionkit.Result{Passed: true, Version: "7", Explanation: "ok"}})
	if res, _ := eval(t, r, Input{}); res.CriterionVersion != "7" || res.Label != "pass" {
		t.Fatalf("result: %+v", res)
	}
	if err := r.ValidateCriterion("custom", json.RawMessage(`{"typo":1}`)); err == nil {
		t.Fatal("params must be validated at import time by the criterion's own factory")
	}
}

func TestAdoptedCriterionErrorsAndPanicsAreReportedNotPropagated(t *testing.T) {
	r := adopt(t, stubCriterion{err: errors.New("service down")})
	if _, err := eval(t, r, Input{}); err == nil || !strings.Contains(err.Error(), "service down") {
		t.Fatalf("got %v", err)
	}
	r = adopt(t, stubCriterion{boom: true})
	if _, err := eval(t, r, Input{}); err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("a panic must become an error, got %v", err)
	}
}

func TestAdoptRejectsNamesOfBuiltInCriteria(t *testing.T) {
	ext := criterionkit.NewRegistry()
	ext.Register("regex", func(json.RawMessage) (criterionkit.Criterion, error) { return stubCriterion{}, nil })
	if err := NewRegistry(nil).Adopt(ext); err == nil || !strings.Contains(err.Error(), `"regex" is built in`) {
		t.Fatalf("got %v", err)
	}
}
