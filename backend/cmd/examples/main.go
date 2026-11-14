// Command examples runs the evaluator with the example adapters and criteria, showing how a
// program evaluates its own agent: import the adapters for their side effect and
// run the evaluator. AGENT=echo go run ./cmd/examples (or AGENT=httpchat).
package main

import (
	"github.com/jsolana/agent-evaluator/backend/evaluator"

	_ "github.com/jsolana/agent-evaluator/backend/examples/criteria"
	_ "github.com/jsolana/agent-evaluator/backend/examples/echo"
	_ "github.com/jsolana/agent-evaluator/backend/examples/httpchat"
)

func main() { evaluator.Main() }
