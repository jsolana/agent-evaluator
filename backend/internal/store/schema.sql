CREATE TABLE IF NOT EXISTS scenario_suite (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL,
  version      INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  min_turns    INTEGER NOT NULL,
  source_yaml  TEXT NOT NULL,
  created_at   TEXT NOT NULL,
  UNIQUE (name, version)
);
CREATE TABLE IF NOT EXISTS scenario (
  id       INTEGER PRIMARY KEY,
  suite_id INTEGER NOT NULL REFERENCES scenario_suite(id),
  key      TEXT NOT NULL,
  name     TEXT NOT NULL,
  tags     TEXT NOT NULL,
  metadata TEXT NOT NULL,
  position INTEGER NOT NULL,
  UNIQUE (suite_id, key)
);
CREATE TABLE IF NOT EXISTS turn (
  id           INTEGER PRIMARY KEY,
  scenario_id  INTEGER NOT NULL REFERENCES scenario(id),
  idx          INTEGER NOT NULL,
  question     TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  UNIQUE (scenario_id, idx)
);
CREATE TABLE IF NOT EXISTS criterion (
  id       INTEGER PRIMARY KEY,
  turn_id  INTEGER NOT NULL REFERENCES turn(id),
  position INTEGER NOT NULL,
  type     TEXT NOT NULL,
  params   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS evaluation (
  id               INTEGER PRIMARY KEY,
  suite_id         INTEGER NOT NULL REFERENCES scenario_suite(id),
  label            TEXT NOT NULL,
  agent_config     TEXT,
  effective_config TEXT,
  concurrency      INTEGER NOT NULL,
  status           TEXT NOT NULL,
  error            TEXT NOT NULL DEFAULT '',
  created_at       TEXT NOT NULL,
  started_at       TEXT,
  finished_at      TEXT
);
CREATE TABLE IF NOT EXISTS scenario_result (
  id                INTEGER PRIMARY KEY,
  evaluation_id     INTEGER NOT NULL REFERENCES evaluation(id),
  scenario_id       INTEGER NOT NULL REFERENCES scenario(id),
  conversation_ref  TEXT NOT NULL DEFAULT '',
  status            TEXT NOT NULL,
  first_failed_turn INTEGER NOT NULL DEFAULT 0,
  total_tokens      INTEGER NOT NULL DEFAULT 0,
  total_ms          INTEGER NOT NULL DEFAULT 0,
  error             TEXT NOT NULL DEFAULT '',
  UNIQUE (evaluation_id, scenario_id)
);
CREATE TABLE IF NOT EXISTS turn_result (
  id                 INTEGER PRIMARY KEY,
  scenario_result_id INTEGER NOT NULL REFERENCES scenario_result(id),
  turn_id            INTEGER NOT NULL REFERENCES turn(id),
  idx                INTEGER NOT NULL,
  idempotency_key    TEXT NOT NULL UNIQUE,
  status             TEXT NOT NULL,
  attempts           INTEGER NOT NULL DEFAULT 0,
  response_text      TEXT NOT NULL DEFAULT '',
  response_raw       TEXT,
  tokens             INTEGER NOT NULL DEFAULT 0,
  duration_ms        INTEGER NOT NULL DEFAULT 0,
  passed             INTEGER NOT NULL DEFAULT 0,
  error              TEXT NOT NULL DEFAULT '',
  UNIQUE (scenario_result_id, idx)
);
CREATE TABLE IF NOT EXISTS span (
  id             INTEGER PRIMARY KEY,
  turn_result_id INTEGER NOT NULL REFERENCES turn_result(id),
  span_id        TEXT NOT NULL,
  parent_span_id TEXT NOT NULL,
  seq            INTEGER NOT NULL,
  kind           TEXT NOT NULL,
  name           TEXT NOT NULL,
  started_at     TEXT,
  duration_ms    INTEGER NOT NULL,
  attributes     TEXT,
  documents      TEXT,
  status         TEXT NOT NULL,
  error          TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS criterion_result (
  id                INTEGER PRIMARY KEY,
  turn_result_id    INTEGER NOT NULL REFERENCES turn_result(id),
  criterion_id      INTEGER NOT NULL REFERENCES criterion(id),
  criterion_type    TEXT NOT NULL,
  criterion_version TEXT NOT NULL,
  passed            INTEGER NOT NULL,
  score             REAL,
  label             TEXT NOT NULL,
  explanation       TEXT NOT NULL,
  evidence          TEXT NOT NULL,
  cost_tokens       INTEGER NOT NULL,
  latency_ms        INTEGER NOT NULL,
  error             TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS annotation (
  id             INTEGER PRIMARY KEY,
  turn_result_id INTEGER NOT NULL REFERENCES turn_result(id),
  author         TEXT NOT NULL,
  body           TEXT NOT NULL,
  tags           TEXT NOT NULL,
  created_at     TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS analysis (
  id             INTEGER PRIMARY KEY,
  turn_result_id INTEGER NOT NULL REFERENCES turn_result(id),
  model          TEXT NOT NULL,
  verdict        TEXT NOT NULL,
  confidence     REAL NOT NULL,
  rationale      TEXT NOT NULL,
  root_cause     TEXT NOT NULL,
  recommendation TEXT NOT NULL,
  tokens         INTEGER NOT NULL,
  created_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_analysis_turn ON analysis(turn_result_id);
CREATE INDEX IF NOT EXISTS idx_span_turn ON span(turn_result_id);
CREATE INDEX IF NOT EXISTS idx_crit_turn ON criterion_result(turn_result_id);
CREATE INDEX IF NOT EXISTS idx_ann_turn ON annotation(turn_result_id);
CREATE INDEX IF NOT EXISTS idx_turnres_sr ON turn_result(scenario_result_id);
