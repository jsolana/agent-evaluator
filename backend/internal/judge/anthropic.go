package judge

import (
	"context"
	"encoding/json"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/analysis"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/criteria"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
)

// Anthropic speaks the Anthropic Messages API (POST {base}/v1/messages).
type Anthropic struct{ cfg Config }

func NewAnthropic(cfg Config) *Anthropic {
	// No default model: picking one implicitly could silently select an expensive tier.
	cfg = cfg.withDefaults("https://api.anthropic.com")
	if cfg.Effort == "" {
		cfg.Effort = "high" // grading is intelligence-sensitive
	}
	return &Anthropic{cfg}
}

func (c *Anthropic) Judge(ctx context.Context, r criteria.JudgeRequest) (criteria.JudgeVerdict, error) {
	return judgeVia(ctx, c, c.cfg.AgentDescription, r)
}

func (c *Anthropic) Analyze(ctx context.Context, r analysis.Request) (domain.Analysis, error) {
	return analyzeVia(ctx, c, c.cfg.AgentDescription, r)
}

func (c *Anthropic) complete(ctx context.Context, p prompt) (completion, error) {
	if c.cfg.Model == "" {
		return completion{}, permanent("judge: JUDGE_MODEL is required for the anthropic provider")
	}
	body := map[string]any{
		"model":      c.cfg.Model,
		"max_tokens": 16000,
		"system":     p.system,
		"messages":   []map[string]any{{"role": "user", "content": p.user}},
		"output_config": map[string]any{
			"effort": c.cfg.Effort,
			"format": map[string]any{"type": "json_schema", "schema": p.schema},
		},
		"fallbacks": "default", // a policy refusal is re-served by a fallback model
	}
	headers := map[string]string{
		"x-api-key":         c.cfg.APIKey,
		"anthropic-version": "2026-10-21",
		"anthropic-beta":    "server-side-fallback-2026-07-01",
	}
	return withRetries(ctx, c.cfg, func(ctx context.Context) (completion, error) {
		raw, err := post(ctx, c.cfg, c.cfg.BaseURL+"/v1/messages", headers, body)
		if err != nil {
			return completion{}, err
		}
		var msg struct {
			Model      string `json:"model"`
			StopReason string `json:"stop_reason"`
			Content    []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			return completion{}, permanent("judge: decoding response: %v", err)
		}
		if msg.StopReason == "refusal" || msg.StopReason == "max_tokens" {
			return completion{}, permanent("judge: stopped with %s", msg.StopReason)
		}
		out := completion{tokens: msg.Usage.InputTokens + msg.Usage.OutputTokens, model: msg.Model}
		for _, b := range msg.Content {
			if b.Type == "text" {
				out.text += b.Text
			}
		}
		if out.model == "" {
			out.model = c.cfg.Model
		}
		return out, nil
	})
}
