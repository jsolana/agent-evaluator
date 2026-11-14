// Package store persists suites, evaluations and results in SQLite
// (database/sql + pure-Go driver). JSON columns hold lists and opaque objects.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

var ErrNotFound = errors.New("not found")

type Store struct{ db *sql.DB }

// Open opens (or creates) the database. ":memory:" is used in tests.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	// One connection: serializes writes (SQLite has a single writer) and keeps
	// a ":memory:" database shared. Enough for this PoC's load.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db}, nil
}

func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// ---------------------------------------------------------------- suites

type SuiteSummary struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Version         int       `json:"version"`
	MinTurns        int       `json:"minTurns"`
	ScenarioCount   int       `json:"scenarioCount"`
	EvaluationCount int       `json:"evaluationCount"`
	CreatedAt       time.Time `json:"createdAt"`
}

// CreateSuite stores a new version of the suite named s.Name. If the latest
// version has the same content it is returned instead (created=false).
func (s *Store) CreateSuite(ctx context.Context, su domain.Suite) (domain.Suite, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return su, false, err
	}
	defer tx.Rollback()

	var latestID int64
	var latestVersion int
	var latestHash string
	err = tx.QueryRowContext(ctx, `SELECT id, version, content_hash FROM scenario_suite WHERE name = ? ORDER BY version DESC LIMIT 1`, su.Name).
		Scan(&latestID, &latestVersion, &latestHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return su, false, err
	}
	if latestHash == su.ContentHash && latestID != 0 {
		tx.Rollback()
		existing, err := s.GetSuite(ctx, latestID)
		return existing, false, err
	}

	su.Version = latestVersion + 1
	su.CreatedAt = time.Now().UTC()
	res, err := tx.ExecContext(ctx, `INSERT INTO scenario_suite (name, version, content_hash, min_turns, source_yaml, created_at) VALUES (?,?,?,?,?,?)`,
		su.Name, su.Version, su.ContentHash, su.MinTurns, su.Source, ts(su.CreatedAt))
	if err != nil {
		return su, false, err
	}
	su.ID, _ = res.LastInsertId()
	for i := range su.Scenarios {
		sc := &su.Scenarios[i]
		res, err := tx.ExecContext(ctx, `INSERT INTO scenario (suite_id, key, name, tags, metadata, position) VALUES (?,?,?,?,?,?)`,
			su.ID, sc.Key, sc.Name, js(sc.Tags), js(sc.Metadata), i)
		if err != nil {
			return su, false, err
		}
		sc.ID, _ = res.LastInsertId()
		for j := range sc.Turns {
			t := &sc.Turns[j]
			res, err := tx.ExecContext(ctx, `INSERT INTO turn (scenario_id, idx, question, content_hash) VALUES (?,?,?,?)`, sc.ID, t.Index, t.Question, t.ContentHash)
			if err != nil {
				return su, false, err
			}
			t.ID, _ = res.LastInsertId()
			for k := range t.Criteria {
				c := &t.Criteria[k]
				res, err := tx.ExecContext(ctx, `INSERT INTO criterion (turn_id, position, type, params) VALUES (?,?,?,?)`, t.ID, k, c.Type, string(c.Params))
				if err != nil {
					return su, false, err
				}
				c.ID, _ = res.LastInsertId()
			}
		}
	}
	return su, true, tx.Commit()
}

func (s *Store) ListSuites(ctx context.Context) ([]SuiteSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.name, s.version, s.min_turns, s.created_at,
		       (SELECT COUNT(*) FROM scenario WHERE suite_id = s.id),
		       (SELECT COUNT(*) FROM evaluation WHERE suite_id = s.id)
		FROM scenario_suite s ORDER BY s.name, s.version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SuiteSummary
	for rows.Next() {
		var x SuiteSummary
		var created string
		if err := rows.Scan(&x.ID, &x.Name, &x.Version, &x.MinTurns, &created, &x.ScenarioCount, &x.EvaluationCount); err != nil {
			return nil, err
		}
		x.CreatedAt = parseTS(created)
		out = append(out, x)
	}
	return out, rows.Err()
}

// GetSuite returns the full definition tree: scenarios, turns and criteria.
func (s *Store) GetSuite(ctx context.Context, id int64) (domain.Suite, error) {
	var su domain.Suite
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, name, version, content_hash, min_turns, source_yaml, created_at FROM scenario_suite WHERE id = ?`, id).
		Scan(&su.ID, &su.Name, &su.Version, &su.ContentHash, &su.MinTurns, &su.Source, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return su, ErrNotFound
	}
	if err != nil {
		return su, err
	}
	su.CreatedAt = parseTS(created)

	rows, err := s.db.QueryContext(ctx, `
		SELECT sc.id, sc.key, sc.name, sc.tags, sc.metadata, t.id, t.idx, t.question, t.content_hash, c.id, c.type, c.params
		FROM scenario sc JOIN turn t ON t.scenario_id = sc.id JOIN criterion c ON c.turn_id = t.id
		WHERE sc.suite_id = ? ORDER BY sc.position, t.idx, c.position`, id)
	if err != nil {
		return su, err
	}
	defer rows.Close()
	for rows.Next() {
		var sc domain.Scenario
		var t domain.Turn
		var c domain.Criterion
		var tags, meta, params string
		if err := rows.Scan(&sc.ID, &sc.Key, &sc.Name, &tags, &meta, &t.ID, &t.Index, &t.Question, &t.ContentHash, &c.ID, &c.Type, &params); err != nil {
			return su, err
		}
		c.Params = json.RawMessage(params)
		if n := len(su.Scenarios); n == 0 || su.Scenarios[n-1].ID != sc.ID {
			_ = json.Unmarshal([]byte(tags), &sc.Tags)
			_ = json.Unmarshal([]byte(meta), &sc.Metadata)
			su.Scenarios = append(su.Scenarios, sc)
		}
		cur := &su.Scenarios[len(su.Scenarios)-1]
		if n := len(cur.Turns); n == 0 || cur.Turns[n-1].ID != t.ID {
			cur.Turns = append(cur.Turns, t)
		}
		turn := &cur.Turns[len(cur.Turns)-1]
		turn.Criteria = append(turn.Criteria, c)
	}
	return su, rows.Err()
}

// ---------------------------------------------------------------- evaluations

type EvaluationSummary struct {
	domain.Evaluation
	SuiteName    string `json:"suiteName"`
	SuiteVersion int    `json:"suiteVersion"`
}

// CreateEvaluation stores a queued evaluation and one pending result per scenario.
func (s *Store) CreateEvaluation(ctx context.Context, e domain.Evaluation) (domain.Evaluation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return e, err
	}
	defer tx.Rollback()
	e.Status, e.CreatedAt = domain.EvaluationQueued, time.Now().UTC()
	res, err := tx.ExecContext(ctx, `INSERT INTO evaluation (suite_id, label, agent_config, concurrency, status, created_at) VALUES (?,?,?,?,?,?)`,
		e.SuiteID, e.Label, nullJSON(e.AgentConfig), e.Concurrency, e.Status, ts(e.CreatedAt))
	if err != nil {
		return e, err
	}
	e.ID, _ = res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `INSERT INTO scenario_result (evaluation_id, scenario_id, status)
		SELECT ?, id, ? FROM scenario WHERE suite_id = ? ORDER BY position`, e.ID, domain.ScenarioPending, e.SuiteID); err != nil {
		return e, err
	}
	return e, tx.Commit()
}

const evaluationCols = `e.id, e.suite_id, e.label, e.agent_config, e.effective_config, e.concurrency, e.status, e.error, e.created_at, e.started_at, e.finished_at, s.name, s.version`

func scanEvaluation(row interface{ Scan(...any) error }) (EvaluationSummary, error) {
	var x EvaluationSummary
	var agentCfg, effCfg, started, finished sql.NullString
	var created string
	err := row.Scan(&x.ID, &x.SuiteID, &x.Label, &agentCfg, &effCfg, &x.Concurrency, &x.Status, &x.Error, &created, &started, &finished, &x.SuiteName, &x.SuiteVersion)
	if err != nil {
		return x, err
	}
	x.CreatedAt = parseTS(created)
	if agentCfg.Valid {
		x.AgentConfig = json.RawMessage(agentCfg.String)
	}
	if effCfg.Valid {
		x.EffectiveConfig = json.RawMessage(effCfg.String)
	}
	if started.Valid {
		t := parseTS(started.String)
		x.StartedAt = &t
	}
	if finished.Valid {
		t := parseTS(finished.String)
		x.FinishedAt = &t
	}
	return x, nil
}

func (s *Store) GetEvaluation(ctx context.Context, id int64) (EvaluationSummary, error) {
	x, err := scanEvaluation(s.db.QueryRowContext(ctx, `SELECT `+evaluationCols+` FROM evaluation e JOIN scenario_suite s ON s.id = e.suite_id WHERE e.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrNotFound
	}
	return x, err
}

// ListEvaluations lists evaluations of every version of a suite (all if name is empty), oldest first.
func (s *Store) ListEvaluations(ctx context.Context, suiteName string) ([]EvaluationSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+evaluationCols+` FROM evaluation e JOIN scenario_suite s ON s.id = e.suite_id
		WHERE ? = '' OR s.name = ? ORDER BY e.id`, suiteName, suiteName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EvaluationSummary
	for rows.Next() {
		x, err := scanEvaluation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) UnfinishedEvaluations(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM evaluation WHERE status IN (?, ?) ORDER BY id`, domain.EvaluationQueued, domain.EvaluationRunning)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpdateEvaluationStatus also stamps started_at (first time running) and finished_at.
func (s *Store) UpdateEvaluationStatus(ctx context.Context, id int64, status domain.EvaluationStatus, errMsg string) error {
	now := ts(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `UPDATE evaluation SET status = ?, error = ?,
		started_at  = CASE WHEN ? = 'running' AND started_at IS NULL THEN ? ELSE started_at END,
		finished_at = CASE WHEN ? IN ('completed','failed') THEN ? ELSE finished_at END
		WHERE id = ?`, status, errMsg, status, now, status, now, id)
	return err
}

// SetEffectiveConfig records the configuration reported by the agent; the first one wins.
func (s *Store) SetEffectiveConfig(ctx context.Context, id int64, cfg json.RawMessage) error {
	_, err := s.db.ExecContext(ctx, `UPDATE evaluation SET effective_config = ? WHERE id = ? AND effective_config IS NULL`, nullJSON(cfg), id)
	return err
}

// ---------------------------------------------------------------- scenario and turn results

func (s *Store) ListScenarioResults(ctx context.Context, evaluationID int64) ([]domain.ScenarioResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.evaluation_id, r.scenario_id, r.conversation_ref, r.status, r.first_failed_turn, r.total_tokens, r.total_ms, r.error
		FROM scenario_result r JOIN scenario sc ON sc.id = r.scenario_id WHERE r.evaluation_id = ? ORDER BY sc.position`, evaluationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ScenarioResult
	for rows.Next() {
		var r domain.ScenarioResult
		if err := rows.Scan(&r.ID, &r.EvaluationID, &r.ScenarioID, &r.ConversationRef, &r.Status, &r.FirstFailedTurn, &r.TotalTokens, &r.TotalMs, &r.Error); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateScenarioResult(ctx context.Context, r domain.ScenarioResult) error {
	_, err := s.db.ExecContext(ctx, `UPDATE scenario_result SET conversation_ref = ?, status = ?, first_failed_turn = ?, total_tokens = ?, total_ms = ?, error = ? WHERE id = ?`,
		r.ConversationRef, r.Status, r.FirstFailedTurn, r.TotalTokens, r.TotalMs, r.Error, r.ID)
	return err
}

func (s *Store) InsertTurnResult(ctx context.Context, t domain.TurnResult) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO turn_result (scenario_result_id, turn_id, idx, idempotency_key, status, attempts) VALUES (?,?,?,?,?,?)`,
		t.ScenarioResultID, t.TurnID, t.Index, t.IdempotencyKey, t.Status, t.Attempts)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateTurnResult(ctx context.Context, t domain.TurnResult) error {
	_, err := s.db.ExecContext(ctx, `UPDATE turn_result SET status = ?, attempts = ?, response_text = ?, response_raw = ?, tokens = ?, duration_ms = ?, passed = ?, error = ? WHERE id = ?`,
		t.Status, t.Attempts, t.ResponseText, nullJSON(t.ResponseRaw), t.Tokens, t.DurationMs, t.Passed, t.Error, t.ID)
	return err
}

const turnCols = `t.id, t.scenario_result_id, t.turn_id, t.idx, t.idempotency_key, t.status, t.attempts, t.response_text, t.response_raw, t.tokens, t.duration_ms, t.passed, t.error`

func scanTurn(rows *sql.Rows) (domain.TurnResult, error) {
	var t domain.TurnResult
	var raw sql.NullString
	err := rows.Scan(&t.ID, &t.ScenarioResultID, &t.TurnID, &t.Index, &t.IdempotencyKey, &t.Status, &t.Attempts, &t.ResponseText, &raw, &t.Tokens, &t.DurationMs, &t.Passed, &t.Error)
	if raw.Valid {
		t.ResponseRaw = json.RawMessage(raw.String)
	}
	return t, err
}

func (s *Store) ListTurnResults(ctx context.Context, scenarioResultID int64) ([]domain.TurnResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+turnCols+` FROM turn_result t WHERE t.scenario_result_id = ? ORDER BY t.idx`, scenarioResultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TurnResult
	for rows.Next() {
		t, err := scanTurn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TurnResultsByEvaluation groups turn results by scenario result id (raw responses excluded).
func (s *Store) TurnResultsByEvaluation(ctx context.Context, evaluationID int64) (map[int64][]domain.TurnResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.scenario_result_id, t.turn_id, t.idx, t.idempotency_key, t.status, t.attempts, t.response_text, NULL, t.tokens, t.duration_ms, t.passed, t.error
		FROM turn_result t JOIN scenario_result r ON r.id = t.scenario_result_id WHERE r.evaluation_id = ? ORDER BY t.scenario_result_id, t.idx`, evaluationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]domain.TurnResult{}
	for rows.Next() {
		t, err := scanTurn(rows)
		if err != nil {
			return nil, err
		}
		out[t.ScenarioResultID] = append(out[t.ScenarioResultID], t)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- spans, criteria, annotations

// SaveSpans replaces the spans of a turn (a resumed turn may be evaluated twice).
func (s *Store) SaveSpans(ctx context.Context, turnResultID int64, spans []domain.Span) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM span WHERE turn_result_id = ?`, turnResultID); err != nil {
		return err
	}
	for _, sp := range spans {
		var started any
		if !sp.Start.IsZero() {
			started = ts(sp.Start)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO span (turn_result_id, span_id, parent_span_id, seq, kind, name, started_at, duration_ms, attributes, documents, status, error)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, turnResultID, sp.SpanID, sp.ParentID, sp.Seq, sp.Kind, sp.Name, started, sp.DurationMs,
			nullJSON(sp.Attributes), js(sp.Documents), sp.Status, sp.Error); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListSpans returns the full spans of a turn, documents included.
func (s *Store) ListSpans(ctx context.Context, turnResultID int64) ([]domain.Span, error) {
	m, err := s.spans(ctx, `SELECT turn_result_id, span_id, parent_span_id, seq, kind, name, started_at, duration_ms, attributes, documents, status, error
		FROM span WHERE turn_result_id = ? ORDER BY seq`, turnResultID)
	return m[turnResultID], err
}

// SpansByEvaluation returns light spans (no attributes, no documents) keyed by turn result.
func (s *Store) SpansByEvaluation(ctx context.Context, evaluationID int64) (map[int64][]domain.Span, error) {
	return s.spans(ctx, `SELECT sp.turn_result_id, sp.span_id, sp.parent_span_id, sp.seq, sp.kind, sp.name, sp.started_at, sp.duration_ms, NULL, NULL, sp.status, sp.error
		FROM span sp JOIN turn_result t ON t.id = sp.turn_result_id JOIN scenario_result r ON r.id = t.scenario_result_id
		WHERE r.evaluation_id = ? ORDER BY sp.turn_result_id, sp.seq`, evaluationID)
}

func (s *Store) spans(ctx context.Context, q string, arg int64) (map[int64][]domain.Span, error) {
	rows, err := s.db.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]domain.Span{}
	for rows.Next() {
		var id int64
		var sp domain.Span
		var started, attrs, docs sql.NullString
		if err := rows.Scan(&id, &sp.SpanID, &sp.ParentID, &sp.Seq, &sp.Kind, &sp.Name, &started, &sp.DurationMs, &attrs, &docs, &sp.Status, &sp.Error); err != nil {
			return nil, err
		}
		if started.Valid {
			sp.Start = parseTS(started.String)
		}
		if attrs.Valid {
			sp.Attributes = json.RawMessage(attrs.String)
		}
		if docs.Valid {
			_ = json.Unmarshal([]byte(docs.String), &sp.Documents)
		}
		out[id] = append(out[id], sp)
	}
	return out, rows.Err()
}

// SaveCriterionResults replaces the criterion results of a turn.
func (s *Store) SaveCriterionResults(ctx context.Context, turnResultID int64, results []domain.CriterionResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM criterion_result WHERE turn_result_id = ?`, turnResultID); err != nil {
		return err
	}
	for _, r := range results {
		if _, err := tx.ExecContext(ctx, `INSERT INTO criterion_result (turn_result_id, criterion_id, criterion_type, criterion_version, passed, score, label, explanation, evidence, cost_tokens, latency_ms, error)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, turnResultID, r.CriterionID, r.CriterionType, r.CriterionVersion, r.Passed, r.Score, r.Label, r.Explanation,
			js(r.Evidence), r.CostTokens, r.LatencyMs, r.Error); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CriterionResultsByEvaluation(ctx context.Context, evaluationID int64) (map[int64][]domain.CriterionResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.turn_result_id, c.id, c.criterion_id, c.criterion_type, c.criterion_version, c.passed, c.score, c.label, c.explanation, c.evidence, c.cost_tokens, c.latency_ms, c.error
		FROM criterion_result c JOIN turn_result t ON t.id = c.turn_result_id JOIN scenario_result r ON r.id = t.scenario_result_id
		JOIN criterion cr ON cr.id = c.criterion_id
		WHERE r.evaluation_id = ? ORDER BY c.turn_result_id, cr.position`, evaluationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]domain.CriterionResult{}
	for rows.Next() {
		var id int64
		var r domain.CriterionResult
		var score sql.NullFloat64
		var evidence string
		if err := rows.Scan(&id, &r.ID, &r.CriterionID, &r.CriterionType, &r.CriterionVersion, &r.Passed, &score, &r.Label, &r.Explanation, &evidence, &r.CostTokens, &r.LatencyMs, &r.Error); err != nil {
			return nil, err
		}
		if score.Valid {
			v := score.Float64
			r.Score = &v
		}
		_ = json.Unmarshal([]byte(evidence), &r.Evidence)
		out[id] = append(out[id], r)
	}
	return out, rows.Err()
}

func (s *Store) AddAnnotation(ctx context.Context, turnResultID int64, a domain.Annotation) (domain.Annotation, error) {
	a.CreatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO annotation (turn_result_id, author, body, tags, created_at) VALUES (?,?,?,?,?)`,
		turnResultID, a.Author, a.Body, js(a.Tags), ts(a.CreatedAt))
	if err != nil {
		return a, err
	}
	a.ID, _ = res.LastInsertId()
	return a, nil
}

func (s *Store) AnnotationsByEvaluation(ctx context.Context, evaluationID int64) (map[int64][]domain.Annotation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.turn_result_id, a.id, a.author, a.body, a.tags, a.created_at
		FROM annotation a JOIN turn_result t ON t.id = a.turn_result_id JOIN scenario_result r ON r.id = t.scenario_result_id
		WHERE r.evaluation_id = ? ORDER BY a.id`, evaluationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]domain.Annotation{}
	for rows.Next() {
		var id int64
		var a domain.Annotation
		var tags, created string
		if err := rows.Scan(&id, &a.ID, &a.Author, &a.Body, &tags, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tags), &a.Tags)
		a.CreatedAt = parseTS(created)
		out[id] = append(out[id], a)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- helpers

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// js encodes lists and maps, storing nil as an empty JSON value.
func js(v any) string {
	b, _ := json.Marshal(v)
	if s := string(b); s != "null" {
		return s
	}
	return "[]"
}

func nullJSON(r json.RawMessage) any {
	if len(r) == 0 || strings.TrimSpace(string(r)) == "null" {
		return nil
	}
	return string(r)
}

// ---------------------------------------------------------------- analyses

func (s *Store) AddAnalysis(ctx context.Context, turnResultID int64, a domain.Analysis) (domain.Analysis, error) {
	a.CreatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO analysis (turn_result_id, model, verdict, confidence, rationale, root_cause, recommendation, tokens, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`, turnResultID, a.Model, a.Verdict, a.Confidence, a.Rationale, a.RootCause, a.Recommendation, a.Tokens, ts(a.CreatedAt))
	if err != nil {
		return a, err
	}
	a.ID, _ = res.LastInsertId()
	return a, nil
}

const analysisCols = `a.id, a.model, a.verdict, a.confidence, a.rationale, a.root_cause, a.recommendation, a.tokens, a.created_at`

func (s *Store) analyses(ctx context.Context, q string, arg int64) (map[int64][]domain.Analysis, error) {
	rows, err := s.db.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]domain.Analysis{}
	for rows.Next() {
		var id int64
		var a domain.Analysis
		var created string
		if err := rows.Scan(&id, &a.ID, &a.Model, &a.Verdict, &a.Confidence, &a.Rationale, &a.RootCause, &a.Recommendation, &a.Tokens, &created); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTS(created)
		out[id] = append(out[id], a)
	}
	return out, rows.Err()
}

// ListAnalyses returns every analysis of a turn, oldest first.
func (s *Store) ListAnalyses(ctx context.Context, turnResultID int64) ([]domain.Analysis, error) {
	m, err := s.analyses(ctx, `SELECT a.turn_result_id, `+analysisCols+` FROM analysis a WHERE a.turn_result_id = ? ORDER BY a.id`, turnResultID)
	return m[turnResultID], err
}

// LatestAnalysesByEvaluation returns the most recent analysis of each turn.
func (s *Store) LatestAnalysesByEvaluation(ctx context.Context, evaluationID int64) (map[int64]domain.Analysis, error) {
	m, err := s.analyses(ctx, `SELECT a.turn_result_id, `+analysisCols+` FROM analysis a
		JOIN turn_result t ON t.id = a.turn_result_id JOIN scenario_result r ON r.id = t.scenario_result_id
		WHERE r.evaluation_id = ? AND a.id = (SELECT MAX(id) FROM analysis WHERE turn_result_id = a.turn_result_id)`, evaluationID)
	out := map[int64]domain.Analysis{}
	for id, list := range m {
		out[id] = list[len(list)-1]
	}
	return out, err
}
