// Command evaluator runs the agent-evaluator with the recipe agent registered.
// AGENT=recipeagent RECIPE_URL=http://localhost:8090 go run ./cmd/evaluator
package main

import (
	"github.com/jsolana/agent-evaluator/backend/evaluator"

	_ "github.com/jsolana/agent-evaluator/recipe-agent/adapter"
)

func main() { evaluator.Main() }
