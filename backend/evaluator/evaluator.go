// Package evaluator is the composition root of the multi-turn evaluator: it
// wires the agent chosen from an agentkit.Registry with the store, runner,
// judge and HTTP API. A program that evaluates its own agent imports its
// adapter for the side effect of registering it and calls Main.
package evaluator

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
	"github.com/jsolana/agent-evaluator/backend/criterionkit"
	"github.com/jsolana/agent-evaluator/backend/internal/analysis"
	"github.com/jsolana/agent-evaluator/backend/internal/api"
	"github.com/jsolana/agent-evaluator/backend/internal/criteria"
	"github.com/jsolana/agent-evaluator/backend/internal/judge"
	"github.com/jsolana/agent-evaluator/backend/internal/queue/memory"
	"github.com/jsolana/agent-evaluator/backend/internal/runner"
	"github.com/jsolana/agent-evaluator/backend/internal/store"
	"github.com/jsolana/agent-evaluator/backend/internal/suite"
)

// Config is everything Run needs; nothing is read from the process environment.
type Config struct {
	Registry *agentkit.Registry     // adapters to choose from; agentkit.Default when nil
	Criteria *criterionkit.Registry // custom criteria to offer to suites; criterionkit.Default when nil
	Env      agentkit.Env           // settings: AGENT, PORT, DB_PATH, JUDGE_*, ...; the process environment when nil
	Logger   *slog.Logger           // slog.Default when nil
}

// Main runs the evaluator as a program: it loads .env, serves until
// interrupted and exits with status 1 on error.
func Main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	loadDotEnv(".env", log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := Run(ctx, Config{Logger: log}); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// Run serves the evaluator until ctx is cancelled.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Registry == nil {
		cfg.Registry = agentkit.Default
	}
	if cfg.Criteria == nil {
		cfg.Criteria = criterionkit.Default
	}
	if cfg.Env == nil {
		cfg.Env = os.Getenv
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	log, getenv := cfg.Logger, cfg.Env
	env := func(name, def string) string {
		if v := getenv(name); v != "" {
			return v
		}
		return def
	}
	envInt := func(name string, def int) int {
		if v, err := strconv.Atoi(getenv(name)); err == nil && v > 0 {
			return v
		}
		return def
	}

	st, err := store.Open(env("DB_PATH", "evaluator.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	judgeImpl, analyzer, judgeCheck, err := buildJudge(getenv)
	if err != nil {
		return err
	}
	registry := criteria.NewRegistry(judgeImpl)
	if err := registry.Adopt(cfg.Criteria); err != nil {
		return err
	}

	plugin, err := cfg.Registry.Build(getenv("AGENT"), getenv)
	if err != nil {
		return err
	}
	r := runner.New(st, plugin.Agent, plugin.Traces, registry, memory.New(256), runner.Config{
		Workers: envInt("WORKERS", 4), DefaultConcurrency: envInt("DEFAULT_CONCURRENCY", 2), Logger: log,
	})
	if err := r.Start(ctx); err != nil {
		return err
	}
	if err := r.Recover(ctx); err != nil {
		return err
	}

	health := map[string]api.Check{
		"database": {Fn: st.Ping},
		"agent":    {Fn: agentCheck(plugin.Agent)},
		"judge":    {Fn: judgeCheck, Optional: true}, // only LLM criteria need it
	}
	srv := &http.Server{
		Addr: ":" + env("PORT", "8080"),
		Handler: api.New(st, r, suite.Options{MinTurns: envInt("SUITE_MIN_TURNS", 3), Validator: registry}, health, log).
			WithCORS(getenv("CORS_ALLOWED_ORIGIN")).WithAnalysis(analysis.New(st, analyzer, log)).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("listening", "addr", srv.Addr, "agent", getenv("AGENT"))
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// agentCheck uses the agent's own health check when it has one.
func agentCheck(a agentkit.Agent) api.HealthCheck {
	if c, ok := a.(agentkit.HealthChecker); ok {
		return c.Check
	}
	return func(context.Context) error { return nil }
}

// buildJudge selects the LLM judge and analyzer: JUDGE_PROVIDER=openai-compatible
// (default, any Chat Completions-compatible endpoint), anthropic, or fake.
func buildJudge(getenv agentkit.Env) (criteria.Judge, analysis.Analyzer, api.HealthCheck, error) {
	cfg := judge.Config{
		AgentDescription: getenv("JUDGE_AGENT_DESCRIPTION"),
		BaseURL:          getenv("JUDGE_BASE_URL"),
		APIKey:           getenv("JUDGE_API_KEY"),
		Model:            getenv("JUDGE_MODEL"),
		Effort:           getenv("JUDGE_EFFORT"),
		ResponseFormat:   getenv("JUDGE_RESPONSE_FORMAT"),
	}
	provider := getenv("JUDGE_PROVIDER")
	if provider == "" {
		provider = "openai-compatible"
	}
	switch provider {
	case "openai-compatible":
		j := judge.NewOpenAICompatible(cfg)
		return j, j, all(required("JUDGE_BASE_URL", cfg.BaseURL), required("JUDGE_API_KEY", cfg.APIKey)), nil
	case "anthropic":
		j := judge.NewAnthropic(cfg)
		return j, j, all(required("JUDGE_API_KEY", cfg.APIKey), required("JUDGE_MODEL", cfg.Model)), nil
	case "fake": // deterministic, offline: demos and UI tests only
		return judge.Fake{}, judge.Fake{}, func(context.Context) error { return nil }, nil
	default:
		return nil, nil, nil, errors.New("JUDGE_PROVIDER must be openai-compatible, anthropic or fake, got " + provider)
	}
}

func all(checks ...api.HealthCheck) api.HealthCheck {
	return func(ctx context.Context) error {
		for _, c := range checks {
			if err := c(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}

func required(name, value string) api.HealthCheck {
	return func(context.Context) error {
		if value == "" {
			return errors.New(name + " is not set")
		}
		return nil
	}
}

// loadDotEnv sets KEY=VALUE pairs from a .env file without overriding the
// real environment. Missing file is fine.
func loadDotEnv(path string, log *slog.Logger) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			_ = os.Setenv(k, v)
		}
	}
	log.Info("loaded environment file", "path", path)
}
