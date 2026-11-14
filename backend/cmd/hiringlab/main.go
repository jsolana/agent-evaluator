// Command hiringlab is the evaluator bundled with the Hiring Lab adapter
// (AGENT=hiringlab) and the offline fake agent.
package main

import (
	"github.com/jsolana/agent-evaluator/backend/evaluator"

	_ "github.com/jsolana/agent-evaluator/backend/adapters/fake"
	_ "github.com/jsolana/agent-evaluator/backend/adapters/hiringlab"
)

func main() { evaluator.Main() }
