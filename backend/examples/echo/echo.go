// Package echo is a minimal third-party style adapter and a template for your
// own: a stateless agent that calls a chat-completions-like HTTP endpoint (here
// a function, to keep the example offline). It imports only agentkit.
//
// Wire it into a program with a blank import and select it with AGENT=echo:
//
//	import _ "github.com/jsolana/agent-evaluator/backend/examples/echo"
package echo

import (
	"context"
	"fmt"
	"strings"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

func init() { agentkit.Register("echo", factory) }

// factory reads ECHO_PREFIX (optional) and builds the agent. It never fails on
// missing settings; required ones would be reported by Check instead.
func factory(env agentkit.Env) (agentkit.Plugin, error) {
	return agentkit.Plugin{Agent: &Agent{Prefix: env("ECHO_PREFIX")}}, nil // no Traces: this agent exposes none
}

// Agent answers by quoting the conversation so far. It is stateless: it needs
// TurnInput.History to know the context and has no conversation id (Ref).
type Agent struct{ Prefix string }

func (a *Agent) NewConversation(context.Context, agentkit.ConversationSpec) (agentkit.Conversation, error) {
	return &conversation{agent: a}, nil
}

type conversation struct{ agent *Agent }

func (*conversation) Ref() string { return "" }

func (c *conversation) Send(ctx context.Context, in agentkit.TurnInput) (agentkit.TurnOutput, error) {
	if err := ctx.Err(); err != nil {
		return agentkit.TurnOutput{}, fmt.Errorf("%w: %v", agentkit.ErrRetryable, err)
	}
	var seen []string
	for _, h := range in.History {
		seen = append(seen, h.Question)
	}
	text := fmt.Sprintf("%sturn %d: %s (earlier: %s)", c.agent.Prefix, in.Index, in.Message, strings.Join(seen, " | "))
	return agentkit.TurnOutput{Text: text}, nil
}
