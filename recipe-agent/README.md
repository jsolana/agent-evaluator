# recipe-agent

A deliberately small agent that proposes recipes from what is in your fridge, used to validate agent-evaluator end to end with a real LLM. It refuses everything else: off-topic questions, food questions that are not recipe requests, and prompt-injection attempts, including ones disguised as recipes.

- **Agent** (`agent/`): Python. HTTP with the standard library; LLM through the `openai` SDK (any OpenAI-compatible API). `POST /chat`, `GET /health`, `GET /traces/{id}`.
- **Execution traces**: [OpenInference](https://github.com/Arize-ai/openinference) instruments the LLM calls automatically (model, input/output messages, token counts, latency) and the agent adds one `AGENT` span around them. No hand-written spans.
- **Two ways to connect it** (same agent, same suite):
  - `adapter/`: Go wrapper with `agentkit`. Takes tokens and latency from the agent's telemetry and fetches the OpenInference spans of each turn by trace id: the `AGENT` span and its child `LLM` span (with messages and token counts).
  - `httpagent/recipe.agent.yaml`: YAML only, no Go. Tokens and latency, no spans (YAML traces only describe tool calls, and this agent has none).
- **Suite** (`suites/recipes.suite.yaml`): 15 scenarios: 6 legitimate, 3 out of scope, 6 attacks. Deterministic `regex` checks (fixed refusal sentence, no leaked canary) plus `llm_judge`.

## Run

From the **repository root** (there is a single Makefile; the targets of this agent start with `recipe-`):

```bash
cp recipe-agent/.env.example recipe-agent/.env   # set LLM_* (agent) and JUDGE_* (judge); any OpenAI-compatible API
make recipe-test               # unit tests
make recipe-eval MODE=go       # agent + evaluator (Go wrapper), runs the suite, prints the result
make recipe-eval MODE=yaml     # same through httpagent (YAML only)
make recipe-eval-ui MODE=go    # same, then keeps agent, backend and the evaluator UI on http://localhost:5173
make recipe-ui                 # chat with the agent as a user: http://localhost:7860 (optional Gradio, installed on first use)
```

`make recipe-eval` exits non-zero if any scenario fails. `make help` lists everything. Requires Python 3.10+, Go and (for the evaluator UI) Node and `make install`.

## Configuration (environment)

| Variable | Default | |
|---|---|---|
| `LLM_BASE_URL` | `https://api.openai.com/v1` | OpenAI-compatible base URL |
| `LLM_API_KEY`, `LLM_MODEL` | – / `gpt-4o-mini` | |
| `LLM_MAX_TOKENS`, `LLM_TEMPERATURE`, `LLM_TIMEOUT` | `512`, `0.3`, `60` | |
| `PORT` | `8090` | agent port |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | also export spans to an OpenInference-compatible backend (Phoenix, Langfuse...) |

To run the agent alone: `make recipe-run`, then
`curl -s localhost:8090/chat -d '{"messages":[{"role":"user","content":"Tengo huevos y patatas"}]}'`.

## How it resists prompt injection

The system prompt (`agent/prompt.py`) judges the *intent* of the message, not its vocabulary; user text is delimited as untrusted data (`<user_input>`) and cannot close the block; refusals use one fixed sentence so tests can assert them; and a canary string in the prompt must never appear in an answer. The server also validates roles and sizes. These are mitigations, not guarantees: the suite is how you measure them, so rerun it whenever the prompt or the model changes.

Traces include the full LLM input (system prompt and canary included) and `/traces/{id}` is unauthenticated: use it for evaluation only, do not expose the agent publicly.
