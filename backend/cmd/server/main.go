// Command server is the demo evaluator: it bundles only the offline fake agent
// (AGENT=fake). To evaluate your own agent, build a program like this one that
// imports your adapter instead; see the README.
package main

import (
	"github.com/jsolana/agent-evaluator/backend/evaluator"

	_ "github.com/jsolana/agent-evaluator/backend/adapters/fake"
)

func main() { evaluator.Main() }
