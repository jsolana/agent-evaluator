# Backend

Go API of Agent Evaluator. All documentation lives in the [root README](../README.md): [configuration](../README.md#configuration), [evaluate a REST agent without code](../README.md#evaluate-a-rest-agent-without-code), [connect your own agent in Go](../README.md#connect-your-own-agent-in-go), [add your own criteria](../README.md#add-your-own-criteria), [REST API](../README.md#rest-api-api) and [testing](../README.md#testing).

```bash
cp .env.example .env
AGENT=fake go run ./cmd/server          # fake and httpagent adapters
AGENT=echo go run ./cmd/examples        # example adapters and criteria
go test -race ./...
```
