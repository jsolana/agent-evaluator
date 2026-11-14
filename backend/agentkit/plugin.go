package agentkit

import (
	"context"
	"errors"
)

// Env reads configuration by name, typically os.Getenv. Factories receive it
// instead of reading the process environment so they stay testable.
type Env func(key string) string

// Plugin is what a Factory builds: the evaluated agent and, optionally, the
// source of its traces.
type Plugin struct {
	Agent  Agent
	Traces TraceSource // nil means the agent exposes no trace
}

// Factory builds a Plugin from the environment. Missing credentials should not
// make it fail: return an Agent whose Check reports them, so the server starts
// and its health endpoint explains what is wrong. Return an error only for
// configuration that can never work.
type Factory func(env Env) (Plugin, error)

// Require returns a Check function that fails when any of the named settings is
// empty. It is meant to be used by HealthChecker implementations.
func Require(env Env, names ...string) func(context.Context) error {
	return func(context.Context) error {
		for _, n := range names {
			if env(n) == "" {
				return errors.New(n + " is not set")
			}
		}
		return nil
	}
}
