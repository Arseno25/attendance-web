SHELL := /bin/bash
-include .env
export DATABASE_URL API_ADDRESS WEB_ORIGIN SCHOOL_ID NUXT_PUBLIC_API_BASE NUXT_PORT AI_SERVICE_TOKEN POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD

UV := $(CURDIR)/.tools/uv
GOOSE := $(CURDIR)/.tools/goose
GOOSE_VERSION := v3.29.0
GOOSE_TAGS := no_clickhouse,no_libsql,no_mssql,no_mysql,no_sqlite3,no_vertica,no_ydb

.PHONY: setup tools env check lint test build db-up db-stop migrate migration-status db-test seed dev-web dev-api dev-ai compose-check

tools: $(UV) $(GOOSE)

$(UV):
	mkdir -p .tools
	curl -LsSf https://astral.sh/uv/0.13.0/install.sh -o .tools/uv-install.sh
	UV_INSTALL_DIR=$(CURDIR)/.tools UV_NO_MODIFY_PATH=1 sh .tools/uv-install.sh

$(GOOSE):
	mkdir -p .tools
	GOBIN=$(CURDIR)/.tools GOTOOLCHAIN=go1.27.2 go install -tags='$(GOOSE_TAGS)' github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)

env:
	python3 scripts/setup_env.py

setup: tools env
	npm --prefix apps/web ci
	cd services/api && go mod download
	cd services/ai && $(UV) sync --locked

lint:
	npm --prefix apps/web run lint
	npm --prefix apps/web run typecheck
	cd services/api && go vet ./... && test -z "$$(gofmt -l cmd internal)"
	cd services/ai && $(UV) run ruff check . && $(UV) run ruff format --check .
	$(UV) run --project services/ai ruff check scripts

test:
	cd services/api && go test -race ./...
	cd services/ai && $(UV) run pytest -q
	python3 -m unittest discover -s scripts -p 'test_*.py'
	node --test scripts/test_ui_flows.mjs

check: lint test

build:
	npm --prefix apps/web run build
	mkdir -p .tools
	cd services/api && CGO_ENABLED=0 go build -trimpath -o ../../.tools/api ./cmd/api
	cd services/api && CGO_ENABLED=0 go build -trimpath -o ../../.tools/bootstrap ./cmd/bootstrap
	cd services/ai && $(UV) run python -c 'import app.main, onnxruntime; assert "CPUExecutionProvider" in onnxruntime.get_available_providers()'

db-up:
	docker compose --env-file .env -f deploy/compose.database.yml up -d --wait

db-stop:
	docker compose --env-file .env -f deploy/compose.database.yml stop

migrate: $(GOOSE)
	@test -n "$$DATABASE_URL" || { echo 'Run make env and configure DATABASE_URL first'; exit 1; }
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" $(GOOSE) -dir services/api/migrations up

migration-status: $(GOOSE)
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" $(GOOSE) -dir services/api/migrations status

db-test:
	psql "$$DATABASE_URL" -f services/api/tests/foundation.sql
	cd services/api && TEST_DATABASE_URL="$$DATABASE_URL" go test -race ./internal/auth ./internal/masterdata ./internal/attendance -run Database -count=1

seed:
	@test "$$SEED_DEVELOPMENT" = 1 || { echo 'Dummy data only: run SEED_DEVELOPMENT=1 make seed on a development DB'; exit 1; }
	psql "$$DATABASE_URL" -v ON_ERROR_STOP=1 -f services/api/seeds/development.sql

dev-web:
	npm --prefix apps/web run dev -- --host 127.0.0.1 --port "$${NUXT_PORT:-3000}"

dev-api:
	cd services/api && go run ./cmd/api

dev-ai:
	cd services/ai && $(UV) run fastapi dev --host 127.0.0.1 --port 8001

compose-check:
	docker compose --env-file .env -f deploy/compose.database.yml config --quiet
	docker compose --env-file deploy/.env.example -f deploy/compose.application.yml config --quiet
