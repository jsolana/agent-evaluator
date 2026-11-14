#!/usr/bin/env bash
# Builds a program in a throw-away module OUTSIDE this one that uses only the
# public API: an agent adapter, a custom criterion and evaluator.Main. If a
# change breaks what a third party can do (an internal import leaks into the
# contract, a public type disappears), this fails. Needs network on first run
# to resolve the dependencies of the evaluator.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mod=github.com/jsolana/agent-evaluator/backend

cd "$work"
go mod init example.com/smoke >/dev/null 2>&1
go mod edit -require="$mod@v0.0.0" -replace="$mod=$root/backend"
mkdir plugin

cat > plugin/plugin.go <<'GO'
package plugin

import (
	"context"
	"encoding/json"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
	"github.com/jsolana/agent-evaluator/backend/criterionkit"
)

func init() {
	agentkit.Register("smoke", func(agentkit.Env) (agentkit.Plugin, error) { return agentkit.Plugin{Agent: agent{}}, nil })
	criterionkit.Register("smoke.nonempty", func(p json.RawMessage) (criterionkit.Criterion, error) {
		var v struct{}
		return nonEmpty{}, criterionkit.DecodeParams(p, &v)
	})
}

type agent struct{}
type conv struct{}

func (agent) NewConversation(context.Context, agentkit.ConversationSpec) (agentkit.Conversation, error) {
	return conv{}, nil
}
func (conv) Ref() string { return "" }
func (conv) Send(_ context.Context, in agentkit.TurnInput) (agentkit.TurnOutput, error) {
	return agentkit.TurnOutput{Text: in.Message}, nil
}

type nonEmpty struct{}

func (nonEmpty) Evaluate(_ context.Context, in criterionkit.Input) (criterionkit.Result, error) {
	return criterionkit.Result{Passed: in.Answer != "", Explanation: "answer is not empty"}, nil
}
GO

cat > main.go <<'GO'
package main

import (
	"github.com/jsolana/agent-evaluator/backend/evaluator"

	_ "example.com/smoke/plugin"
)

func main() { evaluator.Main() }
GO

GOFLAGS=-mod=mod go mod tidy
go vet ./...
go build -o "$work/smoke" .
echo "external module check: OK"
