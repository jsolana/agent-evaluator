# Backend

Go API of Agent Evaluator. All documentation lives in the [root README](../README.md): [configuration](../README.md#configuration), [connect your own agent](../README.md#connect-your-own-agent), [REST API](../README.md#rest-api-api) and [testing](../README.md#testing).

```bash
cp .env.example .env
AGENT=fake go run ./cmd/server
go test -race ./...
```
