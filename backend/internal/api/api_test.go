package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jsolana/agent-evaluator/backend/internal/agent"
	"github.com/jsolana/agent-evaluator/backend/internal/agent/fake"
	"github.com/jsolana/agent-evaluator/backend/internal/analysis"
	"github.com/jsolana/agent-evaluator/backend/internal/criteria"
	"github.com/jsolana/agent-evaluator/backend/internal/domain"
	"github.com/jsolana/agent-evaluator/backend/internal/judge"
	"github.com/jsolana/agent-evaluator/backend/internal/queue/memory"
	"github.com/jsolana/agent-evaluator/backend/internal/runner"
	"github.com/jsolana/agent-evaluator/backend/internal/store"
	"github.com/jsolana/agent-evaluator/backend/internal/suite"
	tracefake "github.com/jsolana/agent-evaluator/backend/internal/tracing/fake"
)

type noJudge struct{}

func (noJudge) Judge(context.Context, criteria.JudgeRequest) (criteria.JudgeVerdict, error) {
	return criteria.JudgeVerdict{}, errors.New("no judge in tests")
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// startServer wires the real API, store (in memory), runner and criteria with
// a fake agent. The fake answers turn 2 of "mortgage" ("cuota") with an amount
// the v1 rubric does not accept, so the journey has a failure to review.
func startServer() (*httptest.Server, func(), error) {
	st, err := store.Open(":memory:")
	if err != nil {
		return nil, nil, err
	}
	reg := criteria.NewRegistry(noJudge{})
	ag := fake.New(func(ref string, h []string, in agent.TurnInput) (string, []domain.Span, error) {
		if strings.Contains(in.Message, "cuota") {
			return "La cuota sería de USD 1,500 al mes", []domain.Span{{Kind: "tool", Name: "search", DurationMs: 120}, {Kind: "tool", Name: "getSource", DurationMs: 180}}, nil
		}
		return fake.Echo(ref, h, in)
	})
	run := runner.New(st, ag, tracefake.Source{}, reg, memory.New(16), runner.Config{Workers: 2, Backoff: time.Millisecond, Logger: quiet})
	ctx, cancel := context.WithCancel(context.Background())
	if err := run.Start(ctx); err != nil {
		cancel()
		return nil, nil, err
	}
	health := map[string]Check{
		"database": {Fn: st.Ping},
		"judge":    {Fn: func(context.Context) error { return errors.New("JUDGE_API_KEY is not set") }, Optional: true},
	}
	srv := httptest.NewServer(New(st, run, suite.Options{MinTurns: 3, Validator: reg}, health, quiet).
		WithCORS("http://localhost:5173").WithAnalysis(analysis.New(st, judge.Fake{}, quiet)).Handler())
	return srv, func() { srv.Close(); cancel(); _ = st.Close() }, nil
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv, stop, err := startServer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path, body string, wantStatus int, out any) http.Header {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, bytes.NewBufferString(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, wantStatus, raw)
	}
	if out != nil {
		if s, ok := out.(*string); ok {
			*s = string(raw)
		} else if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s %s: decoding %s: %v", method, path, raw, err)
		}
	}
	return resp.Header
}

const suiteV1 = `name: Hipotecas
scenarios:
  - key: mortgage
    name: "mortgage | follow-up"
    turns:
      - question: "¿Qué es el crédito HomeLoan?"
        criteria: [{type: regex, pattern: "answer 1"}, {type: tools_required, tools: [search]}]
      - question: "¿Y cuánto sería la cuota?"
        criteria: [{type: regex, pattern: "USD 1,200"}]
      - question: "Resume lo anterior"
        criteria: [{type: regex, pattern: "answer 3"}]
  - key: stable
    turns:
      - question: "q1"
        criteria: [{type: regex, pattern: "answer 1"}]
      - question: "q2"
        criteria: [{type: regex, pattern: "answer 2"}]
      - question: "q3"
        criteria: [{type: regex, pattern: "answer 3"}]
`

func waitCompleted(t *testing.T, srv *httptest.Server, id int64) evaluationView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var v evaluationView
		call(t, srv, "GET", fmt.Sprintf("/api/evaluations/%d", id), "", 200, &v)
		if v.Evaluation.Status == domain.EvaluationCompleted {
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("evaluation did not complete")
	return evaluationView{}
}

func TestErrors(t *testing.T) {
	srv := newServer(t)
	var v1 domain.Suite
	call(t, srv, "POST", "/api/suites", suiteV1, 201, &v1)

	call(t, srv, "POST", "/api/suites", "name: [", 400, nil)
	call(t, srv, "GET", "/api/suites/999", "", 404, nil)
	call(t, srv, "GET", "/api/suites/abc", "", 400, nil)
	call(t, srv, "GET", fmt.Sprintf("/api/suites/%d/scenarios/nope", v1.ID), "", 404, nil)
	call(t, srv, "POST", "/api/evaluations", `{"suiteId":999}`, 400, nil)
	call(t, srv, "POST", "/api/evaluations", fmt.Sprintf(`{"suiteId":%d,"concurrency":99}`, v1.ID), 400, nil)
	call(t, srv, "POST", "/api/evaluations", fmt.Sprintf(`{"suiteId":%d,"agentConfig":"x"}`, v1.ID), 400, nil)
	call(t, srv, "POST", "/api/evaluations", `{"suiteId":1,"unknown":true}`, 400, nil)
	call(t, srv, "GET", "/api/evaluations/999", "", 404, nil)

	var ev domain.Evaluation
	call(t, srv, "POST", "/api/evaluations", fmt.Sprintf(`{"suiteId":%d}`, v1.ID), 202, &ev)
	waitCompleted(t, srv, ev.ID)
	call(t, srv, "GET", fmt.Sprintf("/api/evaluations/%d/scenarios/mortgage/turns/9", ev.ID), "", 404, nil)
	call(t, srv, "GET", fmt.Sprintf("/api/evaluations/%d/comparison", ev.ID), "", 400, nil)
	call(t, srv, "GET", fmt.Sprintf("/api/evaluations/%d/comparison?with=999", ev.ID), "", 404, nil)

	var list []any
	call(t, srv, "GET", "/api/evaluations?suite=unknown", "", 200, &list)
	if len(list) != 0 {
		t.Fatalf("unknown suite must give an empty list, got %v", list)
	}
}

func TestCORS(t *testing.T) {
	srv := newServer(t)
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/api/suites", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/suites", nil)
	req.Header.Set("Origin", "http://evil.example")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("other origins must not be allowed")
	}
}
