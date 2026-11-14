// Package runner executes evaluations asynchronously.
//
// Flow: Submit publishes the evaluation id on the queue. A consumer starts one
// coordinator per evaluation; the coordinator feeds scenarios to a global pool
// of workers, never keeping more than the evaluation's concurrency in flight
// (fair sharing, no starvation). A worker runs one scenario: its turns strictly
// in order within one conversation, persisting each turn before sending it so
// a restart resumes with the same idempotency key.
package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
	"github.com/jsolana/agent-evaluator/backend/internal/criteria"
	"github.com/jsolana/agent-evaluator/backend/internal/domain"
	"github.com/jsolana/agent-evaluator/backend/internal/queue"
	"github.com/jsolana/agent-evaluator/backend/internal/store"
)

const Topic = "evaluations"

// Store is the subset of persistence the runner needs.
type Store interface {
	GetEvaluation(ctx context.Context, id int64) (store.EvaluationSummary, error)
	GetSuite(ctx context.Context, id int64) (domain.Suite, error)
	UnfinishedEvaluations(ctx context.Context) ([]int64, error)
	UpdateEvaluationStatus(ctx context.Context, id int64, status domain.EvaluationStatus, errMsg string) error
	SetEffectiveConfig(ctx context.Context, id int64, cfg json.RawMessage) error
	ListScenarioResults(ctx context.Context, evaluationID int64) ([]domain.ScenarioResult, error)
	UpdateScenarioResult(ctx context.Context, r domain.ScenarioResult) error
	ListTurnResults(ctx context.Context, scenarioResultID int64) ([]domain.TurnResult, error)
	InsertTurnResult(ctx context.Context, t domain.TurnResult) (int64, error)
	UpdateTurnResult(ctx context.Context, t domain.TurnResult) error
	SaveSpans(ctx context.Context, turnResultID int64, spans []domain.Span) error
	SaveCriterionResults(ctx context.Context, turnResultID int64, results []domain.CriterionResult) error
}

// CriteriaBuilder turns a declared criterion into an executable one.
type CriteriaBuilder interface {
	Build(c domain.Criterion) (criteria.Criterion, error)
}

type Config struct {
	Workers            int           // global pool size (default 4)
	DefaultConcurrency int           // per-evaluation in-flight scenarios when not set (default 2)
	MaxAttempts        int           // per turn, for retryable agent errors (default 3)
	Backoff            time.Duration // base backoff between attempts (default 1s)
	// ConflictRetries bounds how often a 409 is retried when an earlier attempt
	// with the same idempotency key exists (the turn may still be running on the
	// platform). A 409 on the first attempt is never retried. Default 5.
	ConflictRetries int
	Logger          *slog.Logger
}

type Runner struct {
	store    Store
	agent    agentkit.Agent
	traces   agentkit.TraceSource
	criteria CriteriaBuilder
	queue    queue.Queue
	cfg      Config
	tasks    chan task
}

type task struct {
	evaluation store.EvaluationSummary
	scenario   domain.Scenario
	result     domain.ScenarioResult
	done       func()
}

func New(st Store, ag agentkit.Agent, tr agentkit.TraceSource, cb CriteriaBuilder, q queue.Queue, cfg Config) *Runner {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.DefaultConcurrency <= 0 {
		cfg.DefaultConcurrency = 2
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = time.Second
	}
	if cfg.ConflictRetries <= 0 {
		cfg.ConflictRetries = 5
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Runner{store: st, agent: ag, traces: tr, criteria: cb, queue: q, cfg: cfg, tasks: make(chan task)}
}

// Start launches the worker pool and the queue consumer. They stop with ctx.
func (r *Runner) Start(ctx context.Context) error {
	deliveries, err := r.queue.Consume(ctx, Topic)
	if err != nil {
		return err
	}
	for i := 0; i < r.cfg.Workers; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case t := <-r.tasks:
					r.runScenario(ctx, t)
					t.done()
				}
			}
		}()
	}
	go func() {
		for d := range deliveries {
			go r.coordinate(ctx, d)
		}
	}()
	return nil
}

// Submit enqueues an evaluation that is already persisted as queued.
func (r *Runner) Submit(ctx context.Context, evaluationID int64) error {
	return r.queue.Publish(ctx, queue.Message{Topic: Topic, ID: strconv.FormatInt(evaluationID, 10)})
}

// Recover re-enqueues evaluations left queued or running by a previous process.
func (r *Runner) Recover(ctx context.Context) error {
	ids, err := r.store.UnfinishedEvaluations(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.Submit(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) coordinate(ctx context.Context, d queue.Delivery) {
	log := r.cfg.Logger.With("evaluation", d.Message().ID)
	id, err := strconv.ParseInt(d.Message().ID, 10, 64)
	if err != nil {
		log.Error("invalid message", "err", err)
		_ = d.Ack()
		return
	}
	ev, err := r.store.GetEvaluation(ctx, id)
	if err != nil {
		log.Error("loading evaluation", "err", err)
		_ = d.Ack()
		return
	}
	if ev.Status == domain.EvaluationCompleted || ev.Status == domain.EvaluationFailed {
		_ = d.Ack() // duplicate delivery
		return
	}
	fail := func(err error) {
		log.Error("evaluation failed", "err", err)
		_ = r.store.UpdateEvaluationStatus(ctx, id, domain.EvaluationFailed, err.Error())
		_ = d.Ack()
	}
	su, err := r.store.GetSuite(ctx, ev.SuiteID)
	if err != nil {
		fail(err)
		return
	}
	results, err := r.store.ListScenarioResults(ctx, id)
	if err != nil {
		fail(err)
		return
	}
	if err := r.store.UpdateEvaluationStatus(ctx, id, domain.EvaluationRunning, ""); err != nil {
		fail(err)
		return
	}

	scenarios := map[int64]domain.Scenario{}
	for _, sc := range su.Scenarios {
		scenarios[sc.ID] = sc
	}
	limit := ev.Concurrency
	if limit <= 0 {
		limit = r.cfg.DefaultConcurrency
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, res := range results {
		if terminal(res.Status) {
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return // shutdown: the evaluation stays running and is recovered later
		}
		wg.Add(1)
		t := task{evaluation: ev, scenario: scenarios[res.ScenarioID], result: res, done: func() { <-sem; wg.Done() }}
		select {
		case r.tasks <- t:
		case <-ctx.Done():
			return
		}
	}
	wg.Wait()
	if ctx.Err() != nil {
		return
	}
	if err := r.store.UpdateEvaluationStatus(ctx, id, domain.EvaluationCompleted, ""); err != nil {
		log.Error("completing evaluation", "err", err)
	}
	_ = d.Ack()
	log.Info("evaluation completed")
}

func terminal(s domain.ScenarioStatus) bool {
	return s == domain.ScenarioPassed || s == domain.ScenarioFailed || s == domain.ScenarioExecutionError
}

// runScenario executes the remaining turns of one scenario in a single conversation.
func (r *Runner) runScenario(ctx context.Context, t task) {
	log := r.cfg.Logger.With("evaluation", t.evaluation.ID, "scenario", t.scenario.Key)
	sr := t.result
	sr.Status = domain.ScenarioRunning
	if err := r.store.UpdateScenarioResult(ctx, sr); err != nil {
		log.Error("updating scenario", "err", err)
		return
	}
	finish := func(results []domain.TurnResult, execErr error) {
		if ctx.Err() != nil {
			return // shutdown: leave it running so Recover resumes it
		}
		sr.Status, sr.FirstFailedTurn = domain.ScenarioVerdict(results)
		sr.TotalTokens, sr.TotalMs = 0, 0
		for _, tr := range results {
			sr.TotalTokens += tr.Tokens
			sr.TotalMs += tr.DurationMs
		}
		if execErr != nil {
			sr.Status, sr.Error = domain.ScenarioExecutionError, execErr.Error()
		}
		if err := r.store.UpdateScenarioResult(ctx, sr); err != nil {
			log.Error("saving scenario result", "err", err)
		}
	}

	existing, err := r.store.ListTurnResults(ctx, sr.ID)
	if err != nil {
		finish(nil, err)
		return
	}
	byIndex := map[int]domain.TurnResult{}
	for _, tr := range existing {
		byIndex[tr.Index] = tr
	}
	conv, err := r.agent.NewConversation(ctx, agentkit.ConversationSpec{
		Config: t.evaluation.AgentConfig, Metadata: t.scenario.Metadata, Ref: sr.ConversationRef,
		Labels: map[string]string{
			"evaluation": strconv.FormatInt(t.evaluation.ID, 10), "evaluationLabel": t.evaluation.Label,
			"suite": t.evaluation.SuiteName, "suiteVersion": strconv.Itoa(t.evaluation.SuiteVersion), "scenario": t.scenario.Key,
		},
	})
	if err != nil {
		finish(nil, err)
		return
	}

	var history []criteria.Exchange
	var results []domain.TurnResult
	for _, turn := range t.scenario.Turns {
		tr, seen := byIndex[turn.Index]
		if seen && tr.Status == domain.TurnCompleted {
			history = append(history, criteria.Exchange{Question: turn.Question, Answer: tr.ResponseText})
			results = append(results, tr)
			continue
		}
		if seen && tr.Attempts > 0 && !r.idempotent() {
			// The turn may have reached the agent before the restart; resending it
			// to an agent without idempotency could run it twice.
			tr.Status, tr.Error = domain.TurnExecutionError, "interrupted turn not resent: the agent is not idempotent"
			_ = r.store.UpdateTurnResult(ctx, tr)
			results = append(results, tr)
			finish(results, fmt.Errorf("turn %d: %s", turn.Index, tr.Error))
			return
		}
		if !seen {
			tr = domain.TurnResult{ScenarioResultID: sr.ID, TurnID: turn.ID, Index: turn.Index, IdempotencyKey: newKey(), Status: domain.TurnRunning}
			if tr.ID, err = r.store.InsertTurnResult(ctx, tr); err != nil {
				finish(results, err)
				return
			}
		}

		out, err := r.send(ctx, conv, &tr, turn, history)
		if ref := conv.Ref(); ref != "" && ref != sr.ConversationRef {
			sr.ConversationRef = ref
			_ = r.store.UpdateScenarioResult(ctx, sr)
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			tr.Status, tr.Error = domain.TurnExecutionError, err.Error()
			_ = r.store.UpdateTurnResult(ctx, tr)
			results = append(results, tr)
			log.Warn("turn failed to execute", "turn", turn.Index, "err", err)
			finish(results, fmt.Errorf("turn %d: %w", turn.Index, err))
			return
		}
		tr = r.evaluateTurn(ctx, t.evaluation.ID, tr, turn, out, history)
		history = append(history, criteria.Exchange{Question: turn.Question, Answer: tr.ResponseText})
		results = append(results, tr)
	}
	finish(results, nil)
}

// send delivers a turn, retrying transient errors with the same idempotency
// key. For an idempotent agent, a 409 after an earlier attempt with that key
// means it may still be processing it: wait and ask again with the same key,
// which returns the saved result once the turn completes.
func (r *Runner) send(ctx context.Context, conv agentkit.Conversation, tr *domain.TurnResult, turn domain.Turn, history []criteria.Exchange) (agentkit.TurnOutput, error) {
	in := agentkit.TurnInput{Index: turn.Index, Message: turn.Question, IdempotencyKey: tr.IdempotencyKey, History: history}
	retries, conflicts := 0, 0
	for {
		previous := tr.Attempts > 0
		tr.Attempts++
		_ = r.store.UpdateTurnResult(ctx, *tr)
		out, err := conv.Send(ctx, in)
		if err == nil {
			return out, nil
		}
		var wait time.Duration
		switch {
		case errors.Is(err, agentkit.ErrRetryable) && retries < r.cfg.MaxAttempts-1:
			wait = r.cfg.Backoff << retries
			retries++
		case errors.Is(err, agentkit.ErrConflict) && previous && r.idempotent() && conflicts < r.cfg.ConflictRetries:
			wait = r.cfg.Backoff << conflicts
			conflicts++
		default:
			return agentkit.TurnOutput{}, err
		}
		select {
		case <-ctx.Done():
			return agentkit.TurnOutput{}, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// evaluateTurn stores the response, its trace and every criterion result.
func (r *Runner) evaluateTurn(ctx context.Context, evaluationID int64, tr domain.TurnResult, turn domain.Turn, out agentkit.TurnOutput, history []criteria.Exchange) domain.TurnResult {
	tr.ResponseText, tr.ResponseRaw, tr.Tokens, tr.DurationMs = out.Text, out.Raw, out.Tokens, out.DurationMs
	if len(out.EffectiveConfig) > 0 {
		_ = r.store.SetEffectiveConfig(ctx, evaluationID, out.EffectiveConfig)
	}

	spans, err := r.traces.Spans(ctx, agentkit.TraceQuery{TraceRef: out.TraceRef, Raw: out.Raw})
	if err != nil {
		tr.Error = "trace extraction: " + err.Error()
	}
	_ = r.store.SaveSpans(ctx, tr.ID, spans)

	in := criteria.Input{History: history, Question: turn.Question, Answer: out.Text, Spans: spans}
	results := make([]domain.CriterionResult, 0, len(turn.Criteria))
	for _, c := range turn.Criteria {
		results = append(results, r.evaluateCriterion(ctx, c, in))
	}
	_ = r.store.SaveCriterionResults(ctx, tr.ID, results)

	tr.Passed = domain.TurnPassed(results)
	tr.Status = domain.TurnCompleted
	_ = r.store.UpdateTurnResult(ctx, tr)
	return tr
}

func (r *Runner) evaluateCriterion(ctx context.Context, c domain.Criterion, in criteria.Input) domain.CriterionResult {
	start := time.Now()
	res := domain.CriterionResult{CriterionType: c.Type, Label: "error"}
	impl, err := r.criteria.Build(c)
	if err == nil {
		var got domain.CriterionResult
		if got, err = impl.Evaluate(ctx, in); err == nil {
			res = got
		}
	}
	if err != nil {
		res.Error = err.Error()
		res.Explanation = "criterion could not be evaluated: " + err.Error()
	}
	res.CriterionID = c.ID
	res.LatencyMs = time.Since(start).Milliseconds()
	return res
}

// idempotent reports whether the agent deduplicates turns by idempotency key.
func (r *Runner) idempotent() bool {
	i, ok := r.agent.(agentkit.Idempotent)
	return ok && i.Idempotent()
}

func newKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "turn-" + hex.EncodeToString(b)
}
