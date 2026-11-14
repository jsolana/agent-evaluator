package judge

import (
	"context"
	"encoding/json"

	"github.com/jsolana/agent-evaluator/backend/internal/analysis"
	"github.com/jsolana/agent-evaluator/backend/internal/criteria"
	"github.com/jsolana/agent-evaluator/backend/internal/domain"
)

// DefaultOpenAICompatibleModel is a low-cost "mini" model, used when
// JUDGE_MODEL is not set. It is deliberately not the evaluated agent's own
// model (gpt-6-luna), to avoid self-evaluation bias.
const DefaultOpenAICompatibleModel = "gpt-5.4-mini"

// OpenAICompatible speaks the Chat Completions protocol popularised by OpenAI
// and offered by many providers and gateways (POST {base}/chat/completions,
// Authorization: Bearer). No provider is assumed: the base URL is required and
// includes the version segment, e.g. https://<provider>/v1.
type OpenAICompatible struct{ cfg Config }

func NewOpenAICompatible(cfg Config) *OpenAICompatible {
	cfg = cfg.withDefaults("")
	if cfg.Model == "" {
		cfg.Model = DefaultOpenAICompatibleModel
	}
	if cfg.ResponseFormat == "" {
		cfg.ResponseFormat = "json_schema"
	}
	return &OpenAICompatible{cfg}
}

func (c *OpenAICompatible) Judge(ctx context.Context, r criteria.JudgeRequest) (criteria.JudgeVerdict, error) {
	return judgeVia(ctx, c, c.cfg.AgentDescription, r)
}

func (c *OpenAICompatible) Analyze(ctx context.Context, r analysis.Request) (domain.Analysis, error) {
	return analyzeVia(ctx, c, c.cfg.AgentDescription, r)
}

func (c *OpenAICompatible) complete(ctx context.Context, p prompt) (completion, error) {
	if c.cfg.BaseURL == "" {
		return completion{}, permanent("judge: JUDGE_BASE_URL is required for OpenAI-compatible providers")
	}
	// No temperature / max_tokens: several models and gateways reject them.
	body := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": p.system},
			{"role": "user", "content": p.user},
		},
	}
	switch c.cfg.ResponseFormat {
	case "json_schema":
		body["response_format"] = map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": p.name, "strict": true, "schema": p.schema},
		}
	case "json_object":
		body["response_format"] = map[string]any{"type": "json_object"}
	}
	headers := map[string]string{"Authorization": "Bearer " + c.cfg.APIKey}

	return withRetries(ctx, c.cfg, func(ctx context.Context) (completion, error) {
		raw, err := post(ctx, c.cfg, c.cfg.BaseURL+"/chat/completions", headers, body)
		if err != nil {
			return completion{}, err
		}
		var resp struct {
			Model   string `json:"model"`
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content string `json:"content"`
					Refusal string `json:"refusal"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Choices) == 0 {
			return completion{}, permanent("judge: unexpected response: %s", raw)
		}
		choice := resp.Choices[0]
		switch {
		case choice.Message.Refusal != "":
			return completion{}, permanent("judge: refused: %s", choice.Message.Refusal)
		case choice.FinishReason == "length" || choice.FinishReason == "content_filter":
			return completion{}, permanent("judge: stopped with %s", choice.FinishReason)
		}
		model := resp.Model
		if model == "" {
			model = c.cfg.Model
		}
		return completion{text: choice.Message.Content, tokens: resp.Usage.PromptTokens + resp.Usage.CompletionTokens, model: model}, nil
	})
}
