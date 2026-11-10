# LinguaScroll dev tasks. `make dev` runs the v0 backend with no API keys.
SHELL := /bin/bash
.DEFAULT_GOAL := help

-include .env
export

DATABASE_URL      ?= postgres://postgres:postgres@localhost:5432/linguascroll?sslmode=disable
TEST_DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable
COMPOSE           := docker compose -f infra/docker-compose.yml
SWIFT_SPEC        := ios/Packages/APIClient/Sources/APIClient/openapi.yaml

.PHONY: help
help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

.PHONY: db
db: ## Start Postgres in Docker and wait until it is ready
	$(COMPOSE) up -d --wait db

.PHONY: dev
dev: db migrate seed ## Run Postgres, migrations, seed, then the API and worker
	cd backend && go build -o ../bin/api ./cmd/api && go build -o ../bin/worker ./cmd/worker
	@trap 'kill %1 %2 2>/dev/null' EXIT INT TERM; \
	./bin/worker & \
	./bin/api & \
	wait

.PHONY: migrate
migrate: ## Apply app and River migrations
	cd backend && go run ./cmd/migrate

.PHONY: seed
seed: ## Load languages, config defaults, starter sources, sample clips
	cd backend && go run ./cmd/seed

.PHONY: gen
gen: ## Generate the Go server interface and copy the spec into the Swift package
	cd backend && go tool oapi-codegen -config oapi-codegen.yaml ../api/openapi.yaml
	cp api/openapi.yaml $(SWIFT_SPEC)

.PHONY: gen-check
gen-check: gen ## Fail if generated files are out of date
	git diff --exit-code -- backend/internal/api $(SWIFT_SPEC)

.PHONY: test
test: ## Go tests (needs Postgres at TEST_DATABASE_URL; `make db` starts one)
	cd backend && go test ./...

.PHONY: lint
lint: ## go vet, gofmt and OpenAPI lint
	cd backend && go vet ./...
	@out=$$(cd backend && gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	npx --yes @redocly/cli@1.34.5 lint api/openapi.yaml

.PHONY: down
down: ## Stop Postgres (data is kept in a Docker volume)
	$(COMPOSE) down
