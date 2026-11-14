// Package report builds the read models the API serves: an evaluation with
// its scenarios and turns joined to their definitions, tool usage and a summary.
package report

import (
	"context"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/store"
)

// StatusNotRun marks a defined turn that was never sent (e.g. after an execution error).
const StatusNotRun domain.TurnStatus = "not_run"

type ToolUsage struct {
	Sequence []string       `json:"sequence"`
	Counts   map[string]int `json:"counts"`
}

func ToolUsageOf(spans []domain.Span) ToolUsage {
	u := ToolUsage{Sequence: []string{}, Counts: map[string]int{}}
	for _, s := range spans {
		if s.Kind == "tool" {
			u.Sequence = append(u.Sequence, s.Name)
			u.Counts[s.Name]++
		}
	}
	return u
}

// Merge returns the concatenation of two usages without mutating them.
func (u ToolUsage) Merge(o ToolUsage) ToolUsage {
	m := ToolUsage{Sequence: append(append([]string{}, u.Sequence...), o.Sequence...), Counts: map[string]int{}}
	for k, v := range u.Counts {
		m.Counts[k] += v
	}
	for k, v := range o.Counts {
		m.Counts[k] += v
	}
	return m
}

type TurnReport struct {
	Index            int                      `json:"index"`
	Question         string                   `json:"question"`
	ContentHash      string                   `json:"contentHash"`
	Criteria         []domain.Criterion       `json:"criteria"`
	TurnResultID     int64                    `json:"turnResultId,omitempty"`
	Status           domain.TurnStatus        `json:"status"`
	Passed           bool                     `json:"passed"`
	Answer           string                   `json:"answer"`
	Tokens           int                      `json:"tokens"`
	DurationMs       int64                    `json:"durationMs"`
	Attempts         int                      `json:"attempts"`
	Error            string                   `json:"error,omitempty"`
	CriterionResults []domain.CriterionResult `json:"criterionResults"`
	Annotations      []domain.Annotation      `json:"annotations"`
	LatestAnalysis   *domain.Analysis         `json:"latestAnalysis,omitempty"`
	Tools            ToolUsage                `json:"tools"`
	Spans            []domain.Span            `json:"spans,omitempty"` // light in scenario views; full (attributes, documents) in the turn detail
}

type ScenarioReport struct {
	Key             string                `json:"key"`
	Name            string                `json:"name"`
	Tags            []string              `json:"tags,omitempty"`
	Status          domain.ScenarioStatus `json:"status"`
	FirstFailedTurn int                   `json:"firstFailedTurn"`
	ConversationRef string                `json:"conversationRef"`
	TotalTokens     int                   `json:"totalTokens"`
	TotalMs         int64                 `json:"totalMs"`
	Error           string                `json:"error,omitempty"`
	Turns           []TurnReport          `json:"turns"`
	Tools           ToolUsage             `json:"tools"`
}

type Counts struct {
	Total          int `json:"total"`
	Passed         int `json:"passed"`
	Failed         int `json:"failed"`
	ExecutionError int `json:"executionError"`
	Pending        int `json:"pending"`
}

type TurnCounts struct {
	Total     int `json:"total"`
	Evaluated int `json:"evaluated"`
	Passed    int `json:"passed"`
}

type PassCount struct {
	Passed int `json:"passed"`
	Total  int `json:"total"`
}

type Summary struct {
	Scenarios        Counts               `json:"scenarios"`
	Turns            TurnCounts           `json:"turns"`
	ScenarioPassRate float64              `json:"scenarioPassRate"` // passed / (passed + failed)
	TurnPassRate     float64              `json:"turnPassRate"`     // passed / evaluated turns
	Progress         float64              `json:"progress"`         // finished scenarios / total
	Tokens           int                  `json:"tokens"`           // agent tokens
	JudgeTokens      int                  `json:"judgeTokens"`      // tokens spent by LLM criteria
	DurationMs       int64                `json:"durationMs"`
	Tools            map[string]int       `json:"tools"`
	FirstFailedTurns map[int]int          `json:"firstFailedTurns"` // turn index -> scenarios that first failed there
	Criteria         map[string]PassCount `json:"criteria"`         // per criterion type
}

type EvaluationReport struct {
	Evaluation store.EvaluationSummary `json:"evaluation"`
	Summary    Summary                 `json:"summary"`
	Scenarios  []ScenarioReport        `json:"scenarios"`
}

func Summarize(scenarios []ScenarioReport) Summary {
	s := Summary{Tools: map[string]int{}, FirstFailedTurns: map[int]int{}, Criteria: map[string]PassCount{}}
	for _, sc := range scenarios {
		s.Scenarios.Total++
		switch sc.Status {
		case domain.ScenarioPassed:
			s.Scenarios.Passed++
		case domain.ScenarioFailed:
			s.Scenarios.Failed++
			s.FirstFailedTurns[sc.FirstFailedTurn]++
		case domain.ScenarioExecutionError:
			s.Scenarios.ExecutionError++
		default:
			s.Scenarios.Pending++
		}
		s.Tokens += sc.TotalTokens
		s.DurationMs += sc.TotalMs
		for k, v := range sc.Tools.Counts {
			s.Tools[k] += v
		}
		for _, t := range sc.Turns {
			s.Turns.Total++
			if t.Status != domain.TurnCompleted {
				continue
			}
			s.Turns.Evaluated++
			if t.Passed {
				s.Turns.Passed++
			}
			for _, c := range t.CriterionResults {
				s.JudgeTokens += c.CostTokens
				pc := s.Criteria[c.CriterionType]
				pc.Total++
				if c.Passed && c.Error == "" {
					pc.Passed++
				}
				s.Criteria[c.CriterionType] = pc
			}
		}
	}
	s.ScenarioPassRate = ratio(s.Scenarios.Passed, s.Scenarios.Passed+s.Scenarios.Failed)
	s.TurnPassRate = ratio(s.Turns.Passed, s.Turns.Evaluated)
	s.Progress = ratio(s.Scenarios.Total-s.Scenarios.Pending, s.Scenarios.Total)
	return s
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// Loader is the subset of the store needed to build reports.
type Loader interface {
	GetEvaluation(ctx context.Context, id int64) (store.EvaluationSummary, error)
	GetSuite(ctx context.Context, id int64) (domain.Suite, error)
	ListScenarioResults(ctx context.Context, evaluationID int64) ([]domain.ScenarioResult, error)
	TurnResultsByEvaluation(ctx context.Context, evaluationID int64) (map[int64][]domain.TurnResult, error)
	SpansByEvaluation(ctx context.Context, evaluationID int64) (map[int64][]domain.Span, error)
	CriterionResultsByEvaluation(ctx context.Context, evaluationID int64) (map[int64][]domain.CriterionResult, error)
	AnnotationsByEvaluation(ctx context.Context, evaluationID int64) (map[int64][]domain.Annotation, error)
	LatestAnalysesByEvaluation(ctx context.Context, evaluationID int64) (map[int64]domain.Analysis, error)
}

// Load joins an evaluation's results with its suite definition. Every defined
// scenario and turn appears, even if it was not executed.
func Load(ctx context.Context, l Loader, evaluationID int64) (EvaluationReport, error) {
	ev, err := l.GetEvaluation(ctx, evaluationID)
	if err != nil {
		return EvaluationReport{}, err
	}
	su, err := l.GetSuite(ctx, ev.SuiteID)
	if err != nil {
		return EvaluationReport{}, err
	}
	results, err := l.ListScenarioResults(ctx, evaluationID)
	if err != nil {
		return EvaluationReport{}, err
	}
	turns, err := l.TurnResultsByEvaluation(ctx, evaluationID)
	if err != nil {
		return EvaluationReport{}, err
	}
	spans, err := l.SpansByEvaluation(ctx, evaluationID)
	if err != nil {
		return EvaluationReport{}, err
	}
	crits, err := l.CriterionResultsByEvaluation(ctx, evaluationID)
	if err != nil {
		return EvaluationReport{}, err
	}
	anns, err := l.AnnotationsByEvaluation(ctx, evaluationID)
	if err != nil {
		return EvaluationReport{}, err
	}
	analyses, err := l.LatestAnalysesByEvaluation(ctx, evaluationID)
	if err != nil {
		return EvaluationReport{}, err
	}

	byScenario := map[int64]domain.ScenarioResult{}
	for _, r := range results {
		byScenario[r.ScenarioID] = r
	}
	rep := EvaluationReport{Evaluation: ev}
	for _, sc := range su.Scenarios {
		res := byScenario[sc.ID]
		sr := ScenarioReport{
			Key: sc.Key, Name: sc.Name, Tags: sc.Tags, Status: res.Status, FirstFailedTurn: res.FirstFailedTurn,
			ConversationRef: res.ConversationRef, TotalTokens: res.TotalTokens, TotalMs: res.TotalMs, Error: res.Error,
			Tools: ToolUsageOf(nil),
		}
		byIndex := map[int]domain.TurnResult{}
		for _, tr := range turns[res.ID] {
			byIndex[tr.Index] = tr
		}
		for _, t := range sc.Turns {
			tr, ok := byIndex[t.Index]
			turn := TurnReport{
				Index: t.Index, Question: t.Question, ContentHash: t.ContentHash, Criteria: t.Criteria,
				Status: StatusNotRun, CriterionResults: []domain.CriterionResult{}, Annotations: []domain.Annotation{}, Tools: ToolUsageOf(nil),
			}
			if ok {
				turn.TurnResultID, turn.Status, turn.Passed, turn.Answer = tr.ID, tr.Status, tr.Passed, tr.ResponseText
				turn.Tokens, turn.DurationMs, turn.Attempts, turn.Error = tr.Tokens, tr.DurationMs, tr.Attempts, tr.Error
				turn.Tools = ToolUsageOf(spans[tr.ID])
				turn.Spans = spans[tr.ID] // light spans (no attributes/documents): the conversation trace
				if c := crits[tr.ID]; c != nil {
					turn.CriterionResults = c
				}
				if a := anns[tr.ID]; a != nil {
					turn.Annotations = a
				}
				if a, ok := analyses[tr.ID]; ok {
					turn.LatestAnalysis = &a
				}
			}
			sr.Tools = sr.Tools.Merge(turn.Tools)
			sr.Turns = append(sr.Turns, turn)
		}
		rep.Scenarios = append(rep.Scenarios, sr)
	}
	rep.Summary = Summarize(rep.Scenarios)
	return rep, nil
}

// Scenario returns the scenario with the given key, or nil.
func (r EvaluationReport) Scenario(key string) *ScenarioReport {
	for i := range r.Scenarios {
		if r.Scenarios[i].Key == key {
			return &r.Scenarios[i]
		}
	}
	return nil
}

// Turn returns the turn with the given 1-based index, or nil.
func (s *ScenarioReport) Turn(index int) *TurnReport {
	if s == nil {
		return nil
	}
	for i := range s.Turns {
		if s.Turns[i].Index == index {
			return &s.Turns[i]
		}
	}
	return nil
}
