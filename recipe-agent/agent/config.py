"""Runtime configuration, read once from the environment."""
import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Config:
    base_url: str
    api_key: str
    model: str
    max_tokens: int = 512
    temperature: float = 0.3
    timeout: float = 60.0
    port: int = 8090

    @classmethod
    def from_env(cls, env=os.environ) -> "Config":
        return cls(
            base_url=env.get("LLM_BASE_URL", "https://api.openai.com/v1").rstrip("/"),
            api_key=env.get("LLM_API_KEY", ""),
            model=env.get("LLM_MODEL", "gpt-4o-mini"),
            max_tokens=int(env.get("LLM_MAX_TOKENS", 512)),
            temperature=float(env.get("LLM_TEMPERATURE", 0.3)),
            timeout=float(env.get("LLM_TIMEOUT", 60)),
            port=int(env.get("PORT", 8090)),
        )
