// Command server is the ready-made evaluator. It bundles the generic REST
// adapter (AGENT=httpagent, described by a YAML file) and the offline fake agent
// (AGENT=fake). To evaluate an agent that does not fit a YAML description,
// build a program like this one that imports your own adapter; see the README.
package main

import (
	"github.com/jsolana/agent-evaluator/backend/evaluator"

	_ "github.com/jsolana/agent-evaluator/backend/adapters/fake"
	_ "github.com/jsolana/agent-evaluator/backend/adapters/httpagent"
)

func main() { evaluator.Main() }
