package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
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
