// Package domain holds the entities of the evaluator and the verdict rules.
// It has no dependencies on storage, transport or the evaluated agent.
package domain

import (
	"encoding/json"
	"time"
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

// Exchange is one completed user/agent turn of a conversation.
type Exchange struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Attribute keys of Span.Attributes shared by every trace source, so the core
// can read them without knowing which adapter produced the span.
const (
	SpanAttrArguments = "arguments" // input of a tool call
	SpanAttrResult    = "result"    // output of a tool call not already mapped to Documents
)

// Span is one step of a turn's trace (a tool call today, an LLM call in other systems).
type Span struct {
	SpanID     string          `json:"spanId"`
	ParentID   string          `json:"parentId"`
	Seq        int             `json:"seq"`
	Kind       string          `json:"kind"` // tool | llm | retrieval | other
	Name       string          `json:"name"`
	Start      time.Time       `json:"start"`
	DurationMs int64           `json:"durationMs"`
	Attributes json.RawMessage `json:"attributes,omitempty"`
	Documents  []Document      `json:"documents,omitempty"`
	Status     string          `json:"status"`
	Error      string          `json:"error,omitempty"`
}

// Arguments returns the tool-call input stored under SpanAttrArguments, or nil.
func (s Span) Arguments() json.RawMessage {
	var attrs map[string]json.RawMessage
	if json.Unmarshal(s.Attributes, &attrs) != nil {
		return nil
	}
	return attrs[SpanAttrArguments]
}

type Document struct {
	Rank    int    `json:"rank"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

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
