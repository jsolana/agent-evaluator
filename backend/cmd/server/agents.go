package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/agent"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/agent/fake"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/agent/hiringlab"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/api"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/tracing"
	tracefake "github.com/jsolana/hiring-lab/part-2/backend/internal/tracing/fake"
	tracehl "github.com/jsolana/hiring-lab/part-2/backend/internal/tracing/hiringlab"
)

// agentFactory builds an evaluated system from the environment: the agent, the
// source of its traces and the health check of its configuration.
type agentFactory func() (agent.Agent, tracing.Source, api.HealthCheck)

// agents lists the systems the evaluator can run against, selected by AGENT.
// To evaluate another agent, implement its adapters and add one entry here.
var agents = map[string]agentFactory{
	"hiringlab": hiringLabAgent,
	"fake":      fakeAgent, // deterministic and offline: demos and UI tests
}

func buildAgent(name string) (agent.Agent, tracing.Source, api.HealthCheck, error) {
	factory, ok := agents[name]
	if !ok {
		names := make([]string, 0, len(agents))
		for n := range agents {
			names = append(names, n)
		}
		slices.Sort(names)
		return nil, nil, nil, fmt.Errorf("AGENT must be one of %s, got %q", strings.Join(names, ", "), name)
	}
	ag, traces, check := factory()
	return ag, traces, check, nil
}

func hiringLabAgent() (agent.Agent, tracing.Source, api.HealthCheck) {
	base, key := os.Getenv("HIRING_LAB_BASE_URL"), os.Getenv("HIRING_LAB_PROJECT_KEY")
	check := all(required("HIRING_LAB_BASE_URL", base), required("HIRING_LAB_PROJECT_KEY", key))
	return hiringlab.New(hiringlab.Config{BaseURL: base, Token: key}), tracehl.Source{}, check
}

func fakeAgent() (agent.Agent, tracing.Source, api.HealthCheck) {
	a := fake.New(nil)
	a.Delay = 300 * time.Millisecond
	return a, tracefake.Source{}, func(context.Context) error { return nil }
}
