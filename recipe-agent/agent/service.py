"""The agent: validated conversation in, recipe + usage + trace id out."""
from dataclasses import dataclass

from openinference.instrumentation import OITracer

from .config import Config
from .llm import LLMClient
from .prompt import build_messages


@dataclass(frozen=True)
class Reply:
    answer: str
    input_tokens: int
    output_tokens: int
    latency_ms: int
    trace_id: str


class RecipeAgent:
    def __init__(self, cfg: Config, llm: LLMClient, tracer: OITracer):
        self.cfg, self.llm, self.tracer = cfg, llm, tracer

    def reply(self, history: list[dict]) -> Reply:
        # AGENT span; the LLM call below becomes its child LLM span via the instrumentor.
        with self.tracer.start_as_current_span("recipe-agent", openinference_span_kind="agent") as root:
            root.set_attribute("input.value", history[-1]["content"])
            c = self.llm.complete(build_messages(history))
            root.set_attribute("output.value", c.text)
            trace_id = f"{root.get_span_context().trace_id:032x}"
        latency = (root.end_time - root.start_time) // 1_000_000  # the trace is the source of truth
        return Reply(c.text, c.input_tokens, c.output_tokens, latency, trace_id)
