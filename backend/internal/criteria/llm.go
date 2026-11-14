package criteria

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jsolana/agent-evaluator/backend/internal/domain"
)

// Judge grades a turn against a rubric. It is implemented by an LLM client
// (internal/judge) and faked in tests.
type Judge interface {
	Judge(ctx context.Context, r JudgeRequest) (JudgeVerdict, error)
}

type JudgeRequest struct {
	Kind      string // llm_judge | context_retention: selects the rubric
	History   []Exchange
	Question  string
	Answer    string
	Expected  string
	Documents []domain.Document // retrieved evidence, to ground the verdict
}

type JudgeVerdict struct {
	Passed      bool
	Score       float64
	Explanation string
	Evidence    []string
	Tokens      int
	Version     string // judge model + rubric version
}

type llmCriterion struct {
	kind     string
	expected string
	judge    Judge
}

func llmFactory(kind string, judge Judge) Factory {
	return func(params json.RawMessage) (Criterion, error) {
		var p struct {
			Expected string `json:"expected"`
		}
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		if strings.TrimSpace(p.Expected) == "" {
			return nil, errors.New("expected is required")
		}
		return llmCriterion{kind, p.Expected, judge}, nil
	}
}

func (c llmCriterion) Evaluate(ctx context.Context, in Input) (domain.CriterionResult, error) {
	var docs []domain.Document
	for _, s := range in.Spans {
		docs = append(docs, s.Documents...)
	}
	v, err := c.judge.Judge(ctx, JudgeRequest{
		Kind: c.kind, History: in.History, Question: in.Question,
		Answer: in.Answer, Expected: c.expected, Documents: docs,
	})
	if err != nil {
		return domain.CriterionResult{}, err
	}
	version := v.Version
	if version == "" {
		version = "1"
	}
	r := result(c.kind, version, v.Passed, v.Explanation, v.Evidence...)
	score := v.Score
	r.Score = &score
	r.CostTokens = v.Tokens
	return r, nil
}
