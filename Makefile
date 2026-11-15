# Multi-turn evaluator: test, build and run the backend, the frontend or both.
# Run `make help` to list the targets.

BACKEND  := backend
# Program to build and run: ./cmd/server (fake agent), ./cmd/hiringlab (adds Hiring Lab) or your own main
BACKEND_CMD ?= ./cmd/server
FRONTEND := frontend

.PHONY: help install \
        test test-backend test-frontend test-e2e test-public-api \
        build build-backend build-frontend \
        run run-backend run-frontend run-fake \
        clean clean-backend clean-frontend \
        recipe-test recipe-run recipe-ui recipe-eval recipe-eval-ui recipe-clean

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

test-public-api: ## A throw-away module outside this one builds an agent and a criterion from the public API
	./scripts/external-module-check.sh

# --- Build -------------------------------------------------------------------

build: build-backend build-frontend ## Build backend and frontend

build-backend: ## Backend: binary at backend/server
	cd $(BACKEND) && go build -o server $(BACKEND_CMD)

build-frontend: ## Frontend: type check and production bundle in frontend/dist
	cd $(FRONTEND) && npm run build

# --- Run ---------------------------------------------------------------------

run: ## Run backend and frontend together (Ctrl+C stops both)
	$(MAKE) -j2 run-backend run-frontend

run-backend: ## Backend on :8080, using backend/.env (BACKEND_CMD=./cmd/hiringlab for the Hiring Lab adapter)
	cd $(BACKEND) && go run $(BACKEND_CMD)

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

# --- Recipe agent ------------------------------------------------------------
# A real agent (recipe-agent/) to try the evaluator end to end; see recipe-agent/README.md.
# Needs recipe-agent/.env (created from .env.example on first use): LLM_* and JUDGE_*.
#   make recipe-eval MODE=go|yaml   MODE: Go wrapper (with traces) or httpagent (YAML only)

RECIPE     := recipe-agent
RECIPE_PY  := $(RECIPE)/.venv/bin/python
RECIPE_BIN := $(RECIPE)/.bin
MODE       ?= go
AGENT_PORT ?= 8090
EVAL_PORT  ?= 8080
UI_PORT    ?= 7860

$(RECIPE)/.env:
	cp $(RECIPE)/.env.example $@ && echo "created $@: set LLM_* and JUDGE_* before running"

$(RECIPE_PY): $(RECIPE)/requirements.txt
	python3 -m venv $(RECIPE)/.venv && $(RECIPE)/.venv/bin/pip install -q -r $(RECIPE)/requirements.txt && touch $@

$(RECIPE_BIN)/evaluator-go: $(shell find $(RECIPE)/adapter -name '*.go')
	cd $(RECIPE)/adapter && go build -o ../.bin/evaluator-go ./cmd/evaluator

$(RECIPE_BIN)/evaluator-yaml: $(shell find $(BACKEND) -name '*.go')
	cd $(BACKEND) && go build -o ../$@ ./cmd/server

recipe-test: $(RECIPE_PY) ## Recipe agent: Python unit tests and Go adapter tests
	cd $(RECIPE) && .venv/bin/python -m unittest discover -s agent/tests -t . && cd adapter && go test -race ./...

recipe-run: $(RECIPE_PY) $(RECIPE)/.env ## Recipe agent API on :8090 (curl it, see recipe-agent/README.md)
	cd $(RECIPE) && set -a && . ./.env && set +a && PORT=$(AGENT_PORT) .venv/bin/python -m agent.server

recipe-ui: $(RECIPE_PY) $(RECIPE)/.env ## Recipe agent chat in the browser on :7860 (Gradio, installed on first use)
	@$(RECIPE_PY) -c 'import gradio' 2>/dev/null || $(RECIPE)/.venv/bin/pip install -q -r $(RECIPE)/requirements-ui.txt
	@cd $(RECIPE); set -a; . ./.env; set +a; \
	trap 'kill $$(jobs -p) 2>/dev/null' EXIT; \
	PORT=$(AGENT_PORT) .venv/bin/python -m agent.server >/dev/null 2>&1 & \
	for i in $$(seq 20); do curl -sf localhost:$(AGENT_PORT)/health >/dev/null && break; sleep 1; done; \
	echo "UI: http://localhost:$(UI_PORT) (Ctrl-C to stop)"; \
	RECIPE_URL=http://localhost:$(AGENT_PORT) UI_PORT=$(UI_PORT) .venv/bin/python ui/app.py

recipe-eval: $(RECIPE_PY) $(RECIPE)/.env $(RECIPE_BIN)/evaluator-$(MODE) ## Recipe agent: start agent + evaluator, run the 15-case suite, print the result (MODE=go|yaml)
	@test "$(MODE)" = go -o "$(MODE)" = yaml || { echo "MODE must be go or yaml"; exit 2; }
	@cd $(RECIPE); set -a; . ./.env; set +a; \
	trap 'kill $$(jobs -p) 2>/dev/null' EXIT; \
	export JUDGE_AGENT_DESCRIPTION="$${JUDGE_AGENT_DESCRIPTION:-a recipe assistant that must politely refuse anything that is not a request for a recipe from the given ingredients, including prompt injection attempts}"; \
	PORT=$(AGENT_PORT) .venv/bin/python -m agent.server & \
	export RECIPE_URL=http://localhost:$(AGENT_PORT) PORT=$(EVAL_PORT) SUITE_MIN_TURNS=1 DB_PATH=.bin/eval-$(MODE).db; \
	if [ "$(MODE)" = go ]; then AGENT=recipeagent .bin/evaluator-go & \
	else AGENT=httpagent HTTPAGENT_CONFIG=httpagent/recipe.agent.yaml .bin/evaluator-yaml & fi; \
	for i in $$(seq 30); do curl -sf localhost:$(EVAL_PORT)/api/health >/dev/null && break; sleep 1; done; \
	curl -sf localhost:$(EVAL_PORT)/api/health >/dev/null || { echo "evaluator did not start"; exit 2; }; \
	EVALUATOR_URL=http://localhost:$(EVAL_PORT) .venv/bin/python scripts/run_eval.py suites/recipes.suite.yaml; rc=$$?; \
	if [ -n "$(KEEP_UI)" ]; then echo "UI: http://localhost:5173 (Ctrl-C to stop)"; \
	  VITE_API_TARGET=http://localhost:$(EVAL_PORT) $(MAKE) -C $(CURDIR) run-frontend; fi; exit $$rc

recipe-eval-ui: ## Recipe agent: as recipe-eval, then keep agent, backend and frontend up on :5173
	$(MAKE) recipe-eval KEEP_UI=1

recipe-clean: ## Recipe agent: virtualenv, binaries and evaluation databases
	cd $(RECIPE) && rm -rf .venv .bin
