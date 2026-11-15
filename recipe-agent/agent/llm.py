"""LLM access through the OpenAI SDK, which works with any OpenAI-compatible API.
Calls are traced automatically by the OpenInference instrumentor (see telemetry.py)."""
from dataclasses import dataclass

import openai

from .config import Config


class LLMError(Exception):
    def __init__(self, message: str, status: int = 502):
        super().__init__(message)
        self.status = status  # HTTP status the server should answer (429 stays 429)


@dataclass(frozen=True)
class Completion:
    text: str
    input_tokens: int
    output_tokens: int


class LLMClient:
    def __init__(self, cfg: Config):
        self.cfg = cfg
        self.client = openai.OpenAI(base_url=cfg.base_url, api_key=cfg.api_key or "none",
                                    timeout=cfg.timeout, max_retries=0)

    def complete(self, messages: list[dict]) -> Completion:
        try:
            r = self.client.chat.completions.create(
                model=self.cfg.model, messages=messages,
                max_tokens=self.cfg.max_tokens, temperature=self.cfg.temperature)
        except openai.RateLimitError as e:
            raise LLMError(f"llm rate limited: {e}", 429) from e
        except openai.OpenAIError as e:
            raise LLMError(f"llm error: {e}") from e
        usage = r.usage
        return Completion(r.choices[0].message.content or "",
                          usage.prompt_tokens if usage else 0, usage.completion_tokens if usage else 0)
