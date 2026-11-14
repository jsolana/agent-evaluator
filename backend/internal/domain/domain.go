// Package domain holds the entities of the evaluator and the verdict rules.
// It has no dependencies on storage, transport or the evaluated agent.
package domain

import (
	"encoding/json"
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// Definition side: a versioned suite of multi-turn scenarios.

type Suite struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Version     int        `json:"version"`
	ContentHash string     `json:"contentHash"`
	MinTurns    int        `json:"minTurns"`
	Source      string     `json:"source,omitempty"`
	Scenarios   []Scenario `json:"scenarios,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type Scenario struct {
	ID       int64          `json:"id"`
	Key      string         `json:"key"` // stable across suite versions
	Name     string         `json:"name"`
	Tags     []string       `json:"tags,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"` // opaque, forwarded to the agent adapter
	Turns    []Turn         `json:"turns,omitempty"`
}

type Turn struct {
	ID          int64       `json:"id"`
	Index       int         `json:"index"` // 1-based
	Question    string      `json:"question"`
	ContentHash string      `json:"contentHash"` // detects rubric changes between suite versions
	Criteria    []Criterion `json:"criteria,omitempty"`
}

type Criterion struct {
	ID     int64           `json:"id"`
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Execution side: an evaluation of a suite and its results per level.

type EvaluationStatus string

const (
	EvaluationQueued    EvaluationStatus = "queued"
	EvaluationRunning   EvaluationStatus = "running"
	EvaluationCompleted EvaluationStatus = "completed"
	EvaluationFailed    EvaluationStatus = "failed"
)

type Evaluation struct {
	ID              int64            `json:"id"`
	SuiteID         int64            `json:"suiteId"`
	Label           string           `json:"label"`
	AgentConfig     json.RawMessage  `json:"agentConfig,omitempty"`
	EffectiveConfig json.RawMessage  `json:"effectiveConfig,omitempty"`
	Concurrency     int              `json:"concurrency"`
	Status          EvaluationStatus `json:"status"`
	Error           string           `json:"error,omitempty"`
	CreatedAt       time.Time        `json:"createdAt"`
	StartedAt       *time.Time       `json:"startedAt,omitempty"`
	FinishedAt      *time.Time       `json:"finishedAt,omitempty"`
}

type ScenarioStatus string

const (
	ScenarioPending        ScenarioStatus = "pending"
	ScenarioRunning        ScenarioStatus = "running"
	ScenarioPassed         ScenarioStatus = "passed"
	ScenarioFailed         ScenarioStatus = "failed"
	ScenarioExecutionError ScenarioStatus = "execution_error"
)

type ScenarioResult struct {
	ID              int64          `json:"id"`
	EvaluationID    int64          `json:"evaluationId"`
	ScenarioID      int64          `json:"scenarioId"`
	ConversationRef string         `json:"conversationRef"`
	Status          ScenarioStatus `json:"status"`
	FirstFailedTurn int            `json:"firstFailedTurn"` // 0 when no turn failed
	TotalTokens     int            `json:"totalTokens"`
	TotalMs         int64          `json:"totalMs"`
	Error           string         `json:"error,omitempty"`
}

type TurnStatus string

const (
	TurnPending        TurnStatus = "pending"
	TurnRunning        TurnStatus = "running"
	TurnCompleted      TurnStatus = "completed"
	TurnExecutionError TurnStatus = "execution_error"
)

type TurnResult struct {
	ID               int64           `json:"id"`
	ScenarioResultID int64           `json:"scenarioResultId"`
	TurnID           int64           `json:"turnId"`
	Index            int             `json:"index"`
	IdempotencyKey   string          `json:"idempotencyKey"`
	Status           TurnStatus      `json:"status"`
	Attempts         int             `json:"attempts"`
	ResponseText     string          `json:"responseText"`
	ResponseRaw      json.RawMessage `json:"responseRaw,omitempty"`
	Tokens           int             `json:"tokens"`
	DurationMs       int64           `json:"durationMs"`
	Passed           bool            `json:"passed"`
	Error            string          `json:"error,omitempty"`
}

// Exchange, Span and Document are part of the public agent contract; the
// domain re-exports them so the rest of the core keeps one vocabulary.
type (
	Exchange = agentkit.Exchange
	Span     = agentkit.Span
	Document = agentkit.Document
)

const (
	SpanAttrArguments = agentkit.SpanAttrArguments
	SpanAttrResult    = agentkit.SpanAttrResult
)

type CriterionResult struct {
	ID               int64    `json:"id"`
	CriterionID      int64    `json:"criterionId"`
	CriterionType    string   `json:"criterionType"`
	CriterionVersion string   `json:"criterionVersion"`
	Passed           bool     `json:"passed"`
	Score            *float64 `json:"score,omitempty"`
	Label            string   `json:"label"`
	Explanation      string   `json:"explanation"`
	Evidence         []string `json:"evidence,omitempty"`
	CostTokens       int      `json:"costTokens"`
	LatencyMs        int64    `json:"latencyMs"`
	Error            string   `json:"error,omitempty"`
}

type Annotation struct {
	ID        int64     `json:"id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Analysis is an LLM second opinion on a turn's evaluation. It is advisory:
// it never changes the verdict.
type Analysis struct {
	ID             int64     `json:"id"`
	Model          string    `json:"model"`
	Verdict        string    `json:"verdict"` // real_failure | false_positive | judge_too_lax | correct
	Confidence     float64   `json:"confidence"`
	Rationale      string    `json:"rationale"`
	RootCause      string    `json:"rootCause"` // agent | retrieval | rubric | judge | context | none
	Recommendation string    `json:"recommendation"`
	Tokens         int       `json:"tokens"`
	CreatedAt      time.Time `json:"createdAt"`
}
