package hiringlab

import "github.com/jsolana/agent-evaluator/backend/agentkit"

func init() { agentkit.Register("hiringlab", factory) }

// factory reads HIRING_LAB_BASE_URL and HIRING_LAB_PROJECT_KEY.
func factory(env agentkit.Env) (agentkit.Plugin, error) {
	a := New(Config{BaseURL: env("HIRING_LAB_BASE_URL"), Token: env("HIRING_LAB_PROJECT_KEY")})
	return agentkit.Plugin{Agent: a, Traces: Source{}}, nil
}
