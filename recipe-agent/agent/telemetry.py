"""Agent tracing with OpenInference (the span conventions for LLM agents).

OpenAIInstrumentor records every LLM call automatically: model, input/output
messages, token counts and latency. The agent adds one AGENT span around it.
Spans are kept in a bounded in-memory exporter so the evaluator can fetch the
trace of a turn by id; set OTEL_EXPORTER_OTLP_ENDPOINT to also ship them to an
OpenInference-compatible backend (Phoenix, Langfuse...).
"""
import os
from collections import deque

from openinference.instrumentation import OITracer, TraceConfig
from openinference.instrumentation.openai import OpenAIInstrumentor
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor, SpanExporter, SpanExportResult


class RingExporter(SpanExporter):
    """Keeps the last `size` finished spans."""

    def __init__(self, size: int = 2000):
        self._spans = deque(maxlen=size)

    def export(self, spans):
        self._spans.extend(spans)
        return SpanExportResult.SUCCESS

    def trace(self, trace_id: str) -> list[str]:
        """JSON of every span of a trace (32 hex chars)."""
        return [s.to_json(indent=None) for s in self._spans if f"{s.context.trace_id:032x}" == trace_id]


def setup(service: str = "recipe-agent") -> tuple[OITracer, RingExporter]:
    provider = TracerProvider(resource=Resource.create({"service.name": service}))
    ring = RingExporter()
    provider.add_span_processor(SimpleSpanProcessor(ring))
    if os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT"):
        from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
        from opentelemetry.sdk.trace.export import BatchSpanProcessor
        provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
    instrumentor = OpenAIInstrumentor()  # singleton: re-bind it to this provider
    instrumentor.uninstrument()
    instrumentor.instrument(tracer_provider=provider, skip_dep_check=True)
    return OITracer(provider.get_tracer(service), TraceConfig()), ring
