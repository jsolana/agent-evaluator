# Hiring Lab adapter

Adapts the Hiring Lab assistant (`POST /ask`) to `agent.Agent`; the matching trace extractor is in [`../../tracing/hiringlab`](../../tracing/hiringlab). Select it with `AGENT=hiringlab`.

| Variable | Required | Purpose |
|---|---|---|
| `HIRING_LAB_BASE_URL` | yes | Base URL of the private API, e.g. `https://<host>/api/private/projects/<project id>` |
| `HIRING_LAB_PROJECT_KEY` | yes | API key (Bearer) |

Suggested `JUDGE_AGENT_DESCRIPTION`: `a banking assistant`.

The agent keeps the conversation by id, deduplicates turns by `Idempotency-Key` and returns its trace inline. Its tools are `ragSearch` and `ragGetSourceChunks`.

An example suite for this agent is in [`examples/multiturn.suite.yaml`](examples/multiturn.suite.yaml).
