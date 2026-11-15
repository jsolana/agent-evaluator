"""HTTP API (stdlib only): POST /chat, GET /health, GET /traces/{trace_id}."""
import json
import logging
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from .config import Config
from .llm import LLMClient, LLMError
from .service import RecipeAgent
from .telemetry import RingExporter, setup

MAX_MESSAGES, MAX_CHARS = 20, 2000


def validate(payload) -> list[dict]:
    """Return the messages or raise ValueError. Roles and sizes are enforced here."""
    msgs = payload.get("messages") if isinstance(payload, dict) else None
    if not isinstance(msgs, list) or not 0 < len(msgs) <= MAX_MESSAGES:
        raise ValueError(f"messages must be a list of 1..{MAX_MESSAGES} items")
    for m in msgs:
        if not isinstance(m, dict) or m.get("role") not in ("user", "assistant") \
                or not isinstance(m.get("content"), str) or len(m["content"]) > MAX_CHARS:
            raise ValueError(f"each message needs role user|assistant and content up to {MAX_CHARS} chars")
    if msgs[-1]["role"] != "user":
        raise ValueError("the last message must come from the user")
    return msgs


def make_handler(agent: RecipeAgent, ring: RingExporter):
    class Handler(BaseHTTPRequestHandler):
        def _send(self, status: int, body, headers=None):
            data = json.dumps(body).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            for k, v in (headers or {}).items():
                self.send_header(k, v)
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            if self.path == "/health":
                return self._send(200, {"status": "ok"})
            if self.path.startswith("/traces/"):
                spans = [json.loads(s) for s in ring.trace(self.path.removeprefix("/traces/"))]
                return self._send(200 if spans else 404, {"spans": spans})
            self._send(404, {"error": "not found"})

        def do_POST(self):
            if self.path != "/chat":
                return self._send(404, {"error": "not found"})
            try:
                msgs = validate(json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0)))))
            except (ValueError, json.JSONDecodeError) as e:
                return self._send(400, {"error": str(e)})
            try:
                r = agent.reply(msgs)
            except LLMError as e:
                return self._send(e.status, {"error": str(e)})
            self._send(200, {
                "answer": r.answer, "model": agent.cfg.model, "latency_ms": r.latency_ms, "trace_id": r.trace_id,
                "usage": {"prompt_tokens": r.input_tokens, "completion_tokens": r.output_tokens,
                          "total_tokens": r.input_tokens + r.output_tokens},
            })

        def log_message(self, fmt, *args):
            logging.info(fmt, *args)

    return Handler


def main():
    logging.basicConfig(level=logging.INFO)
    cfg = Config.from_env()
    tracer, ring = setup()
    server = ThreadingHTTPServer(("", cfg.port), make_handler(RecipeAgent(cfg, LLMClient(cfg), tracer), ring))
    logging.info("recipe-agent on :%d (model %s)", cfg.port, cfg.model)
    server.serve_forever()


if __name__ == "__main__":
    main()
