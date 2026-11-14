package httpagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// funcs available to the request template. `json` renders any value as JSON,
// which also takes care of escaping.
var funcs = template.FuncMap{
	"json": func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	},
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// templateData is what the request template sees.
type templateData struct {
	Message        string
	Index          int
	IdempotencyKey string
	Ref            string              // conversation id (stateful agents)
	History        []agentkit.Exchange // completed turns
	Messages       []message           // history as user/assistant messages, plus the current message
	Config         any                 // the evaluation's agentConfig
}

func newTemplateData(in agentkit.TurnInput, ref string, config any) templateData {
	d := templateData{Message: in.Message, Index: in.Index, IdempotencyKey: in.IdempotencyKey, Ref: ref, History: in.History, Config: config}
	for _, h := range in.History {
		d.Messages = append(d.Messages, message{"user", h.Question}, message{"assistant", h.Answer})
	}
	d.Messages = append(d.Messages, message{"user", in.Message})
	return d
}

// renderBody produces the request body, which must be valid JSON. Without a
// template the body is empty (e.g. a GET).
func (s *spec) renderBody(d templateData) ([]byte, error) {
	if s.body == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	if err := s.body.Execute(&buf, d); err != nil {
		return nil, fmt.Errorf("turn.body: %w", err)
	}
	if !json.Valid(buf.Bytes()) {
		return nil, fmt.Errorf("turn.body does not render valid JSON: %s", buf.String())
	}
	return buf.Bytes(), nil
}
