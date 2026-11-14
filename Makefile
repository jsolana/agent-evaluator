# Multi-turn evaluator: test, build and run the backend, the frontend or both.
# Run `make help` to list the targets.

BACKEND  := backend
FRONTEND := frontend

.PHONY: help install \
        test test-backend test-frontend test-e2e \
        build build-backend build-frontend \
        run run-backend run-frontend run-fake \
        clean clean-backend clean-frontend

help: ## List the targets
	@grep -E '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-16s %s\n", $$1, $$2}'

install: ## Install frontend dependencies (exact versions from the lockfile)
	cd $(FRONTEND) && npm ci

# --- Test --------------------------------------------------------------------

test: test-backend test-frontend ## Test backend and frontend

test-backend: ## Backend: unit, integration and acceptance tests (race detector on)
	cd $(BACKEND) && go test -race ./...

test-frontend: ## Frontend: lint and unit tests
	cd $(FRONTEND) && npm run lint && npm test

test-e2e: ## Frontend: Playwright journey (starts its own backend with fakes)
	cd $(FRONTEND) && npx playwright install chromium && npm run test:e2e

# --- Build -------------------------------------------------------------------

build: build-backend build-frontend ## Build backend and frontend

build-backend: ## Backend: binary at backend/server
	cd $(BACKEND) && go build -o server ./cmd/server

build-frontend: ## Frontend: type check and production bundle in frontend/dist
	cd $(FRONTEND) && npm run build

# --- Run ---------------------------------------------------------------------

run: ## Run backend and frontend together (Ctrl+C stops both)
	$(MAKE) -j2 run-backend run-frontend

run-backend: ## Backend on :8080, using backend/.env
	cd $(BACKEND) && go run ./cmd/server

run-frontend: ## Frontend dev server on :5173, proxying /api to the backend
	cd $(FRONTEND) && npm run dev

run-fake: ## Backend and frontend with fake agent and judge (no credentials, separate database)
	AGENT=fake JUDGE_PROVIDER=fake DB_PATH=evaluator-fake.db $(MAKE) -j2 run-backend run-frontend

# --- Clean -------------------------------------------------------------------
# Removes generated files only. Keeps backend/evaluator.db (your evaluations)
# and frontend/node_modules (run `make install` to refresh them).

clean: clean-backend clean-frontend ## Clean backend and frontend

clean-backend: ## Backend: binary, fake-run database and Go test cache
	cd $(BACKEND) && rm -f server evaluator-fake.db evaluator-fake.db-shm evaluator-fake.db-wal && go clean -testcache

clean-frontend: ## Frontend: production bundle and Playwright results
	cd $(FRONTEND) && rm -rf dist test-results playwright-report
