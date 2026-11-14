// Command server runs the multi-turn evaluator API. It is the only place where
// concrete implementations are wired together.
package main

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

	"github.com/jsolana/agent-evaluator/backend/internal/analysis"
	"github.com/jsolana/agent-evaluator/backend/internal/api"
	"github.com/jsolana/agent-evaluator/backend/internal/criteria"
	"github.com/jsolana/agent-evaluator/backend/internal/judge"
	"github.com/jsolana/agent-evaluator/backend/internal/queue/memory"
	"github.com/jsolana/agent-evaluator/backend/internal/runner"
	"github.com/jsolana/agent-evaluator/backend/internal/store"
	"github.com/jsolana/agent-evaluator/backend/internal/suite"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	loadDotEnv(".env", log)
	if err := run(log); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(env("DB_PATH", "evaluator.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	judgeImpl, analyzer, judgeCheck, err := buildJudge()
	if err != nil {
		return err
	}
	registry := criteria.NewRegistry(judgeImpl)

	ag, traces, agentCheck, err := buildAgent(os.Getenv("AGENT"))
	if err != nil {
		return err
	}
	r := runner.New(st, ag, traces, registry, memory.New(256), runner.Config{
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
		"agent":    {Fn: agentCheck},
		"judge":    {Fn: judgeCheck, Optional: true}, // only LLM criteria need it
	}
	srv := &http.Server{
		Addr: ":" + env("PORT", "8080"),
		Handler: api.New(st, r, suite.Options{MinTurns: envInt("SUITE_MIN_TURNS", 3), Validator: registry}, health, log).
			WithCORS(os.Getenv("CORS_ALLOWED_ORIGIN")).WithAnalysis(analysis.New(st, analyzer, log)).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("listening", "addr", srv.Addr, "agent", os.Getenv("AGENT"))
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// buildJudge selects the LLM judge and analyzer: JUDGE_PROVIDER=openai-compatible
// (default, any Chat Completions-compatible endpoint), anthropic, or fake.
func buildJudge() (criteria.Judge, analysis.Analyzer, api.HealthCheck, error) {
	cfg := judge.Config{
		AgentDescription: os.Getenv("JUDGE_AGENT_DESCRIPTION"),
		BaseURL:          os.Getenv("JUDGE_BASE_URL"),
		APIKey:           os.Getenv("JUDGE_API_KEY"),
		Model:            os.Getenv("JUDGE_MODEL"),
		Effort:           os.Getenv("JUDGE_EFFORT"),
		ResponseFormat:   os.Getenv("JUDGE_RESPONSE_FORMAT"),
	}
	switch provider := env("JUDGE_PROVIDER", "openai-compatible"); provider {
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

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
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
