package httpagent

import (
	"errors"
	"fmt"
	"os"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

func init() { agentkit.Register("httpagent", factory) }

// factory loads the YAML named by HTTPAGENT_CONFIG. A missing or invalid file
// can never work, so it stops the start-up; missing ${VAR} settings are
// reported by Check instead.
func factory(env agentkit.Env) (agentkit.Plugin, error) {
	path := env("HTTPAGENT_CONFIG")
	if path == "" {
		return agentkit.Plugin{}, errors.New("HTTPAGENT_CONFIG must name the agent description file")
	}
	f, err := os.Open(path)
	if err != nil {
		return agentkit.Plugin{}, fmt.Errorf("HTTPAGENT_CONFIG: %w", err)
	}
	defer f.Close()
	s, err := parse(f, env)
	if err != nil {
		return agentkit.Plugin{}, fmt.Errorf("%s: %w", path, err)
	}
	p := agentkit.Plugin{Agent: newAgent(s, nil)}
	if s.Traces != nil {
		p.Traces = source{cfg: s.Traces}
	}
	return p, nil
}
