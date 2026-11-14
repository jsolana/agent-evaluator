package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jsolana/agent-evaluator/backend/adapters/fake"
	"github.com/jsolana/agent-evaluator/backend/agentkit"
	"github.com/jsolana/agent-evaluator/backend/criterionkit"
)

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return strings.Split(l.Addr().String(), ":")[1]
}

func env(m map[string]string) agentkit.Env { return func(k string) string { return m[k] } }

func TestRunRejectsAnUnselectedOrUnknownAgent(t *testing.T) {
	reg := agentkit.NewRegistry()
	reg.Register("fake", func(agentkit.Env) (agentkit.Plugin, error) { return agentkit.Plugin{Agent: unhealthy{}}, nil })
	for _, name := range []string{"", "other"} {
		err := Run(context.Background(), Config{Registry: reg, Env: env(map[string]string{
			"AGENT": name, "DB_PATH": filepath.Join(t.TempDir(), "e.db"), "JUDGE_PROVIDER": "fake"})})
		if err == nil || !strings.Contains(err.Error(), "AGENT must be one of fake") {
			t.Fatalf("AGENT=%q: got %v", name, err)
		}
	}
}

// A program can serve any registered agent with an explicit registry and
// settings: nothing is read from the process environment.
func TestRunServesAnInjectedAgentAndReportsItsHealth(t *testing.T) {
	reg := agentkit.NewRegistry()
	reg.Register("broken", func(agentkit.Env) (agentkit.Plugin, error) {
		return agentkit.Plugin{Agent: unhealthy{}}, nil
	})
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Registry: reg, Env: env(map[string]string{
			"AGENT": "broken", "PORT": port, "DB_PATH": filepath.Join(t.TempDir(), "e.db"), "JUDGE_PROVIDER": "fake"})})
	}()

	var resp *http.Response
	var err error
	for i := 0; i < 50; i++ {
		if resp, err = http.Get("http://127.0.0.1:" + port + "/api/health"); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Status string
		Checks map[string]string
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body.Checks["agent"], "not configured") {
		t.Fatalf("status=%d body=%+v", resp.StatusCode, body)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("a cancelled context is a clean shutdown, got %v", err)
	}
}

// unhealthy is a stub agent whose configuration is incomplete.
type unhealthy struct{}

func (unhealthy) NewConversation(context.Context, agentkit.ConversationSpec) (agentkit.Conversation, error) {
	return nil, nil
}

func (unhealthy) Check(context.Context) error { return errors.New("agent not configured") }

func TestRunRejectsACustomCriterionThatShadowsABuiltInOne(t *testing.T) {
	reg := agentkit.NewRegistry()
	reg.Register("a", func(agentkit.Env) (agentkit.Plugin, error) { return agentkit.Plugin{Agent: unhealthy{}}, nil })
	crit := criterionkit.NewRegistry()
	crit.Register("regex", func(json.RawMessage) (criterionkit.Criterion, error) { return nil, nil })
	err := Run(context.Background(), Config{Registry: reg, Criteria: crit, Env: env(map[string]string{
		"AGENT": "a", "DB_PATH": filepath.Join(t.TempDir(), "e.db"), "JUDGE_PROVIDER": "fake"})})
	if err == nil || !strings.Contains(err.Error(), `"regex" is built in`) {
		t.Fatalf("got %v", err)
	}
}

type shortAnswers struct{}

func (shortAnswers) Evaluate(_ context.Context, in criterionkit.Input) (criterionkit.Result, error) {
	n := len(strings.Fields(in.Answer))
	return criterionkit.Result{Passed: n <= 4, Explanation: "words: " + strconv.Itoa(n)}, nil
}

// A criterion registered by a third party can be declared in a suite, is
// validated on import and appears in the report of an evaluation.
func TestRunServesACustomCriterionEndToEnd(t *testing.T) {
	agents := agentkit.NewRegistry()
	agents.Register("fake", func(agentkit.Env) (agentkit.Plugin, error) { return agentkit.Plugin{Agent: fake.New(nil)}, nil })
	crit := criterionkit.NewRegistry()
	crit.Register("short", func(p json.RawMessage) (criterionkit.Criterion, error) {
		var v struct{}
		if err := criterionkit.DecodeParams(p, &v); err != nil {
			return nil, err
		}
		return shortAnswers{}, nil
	})
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Run(ctx, Config{Registry: agents, Criteria: crit, Env: env(map[string]string{
		"AGENT": "fake", "PORT": port, "DB_PATH": filepath.Join(t.TempDir(), "e.db"), "JUDGE_PROVIDER": "fake"})})
	base := "http://127.0.0.1:" + port + "/api"
	for i := 0; ; i++ {
		if r, err := http.Get(base + "/health"); err == nil {
			r.Body.Close()
			break
		} else if i > 50 {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	post := func(path, ctype, body string) (int, string) {
		resp, err := http.Post(base+path, ctype, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	suite := func(extra string) string {
		turn := func(q string) string { return `{question: "` + q + `", criteria: [{type: short` + extra + `}]}` }
		return "name: S\nscenarios:\n  - key: k\n    turns: [" + turn("a") + ", " + turn("b") + ", " + turn("c") + "]\n"
	}
	if code, body := post("/suites", "text/yaml", suite(", typo: 1")); code != http.StatusBadRequest || !strings.Contains(body, "typo") {
		t.Fatalf("invalid params must be rejected on import: %d %s", code, body)
	}
	if code, body := post("/suites", "text/yaml", suite("")); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	if code, body := post("/evaluations", "application/json", `{"suiteId":1}`); code >= 300 {
		t.Fatalf("evaluate: %d %s", code, body)
	}
	var report string
	for i := 0; i < 50; i++ {
		resp, err := http.Get(base + "/evaluations/1/scenarios/k")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			report = string(b)
			if strings.Contains(report, `"criterionType":"short"`) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(report, `"criterionType":"short"`) || !strings.Contains(report, "words:") {
		t.Fatalf("the custom criterion must appear in the report: %s", report)
	}
}
