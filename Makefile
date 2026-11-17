# LinguaScroll dev tasks. `make dev` runs the v0 backend with no API keys.
SHELL := /bin/bash
.DEFAULT_GOAL := help

-include .env
export

DB_PORT           := 54320
DATABASE_URL      ?= postgres://postgres:postgres@127.0.0.1:$(DB_PORT)/linguascroll?sslmode=disable
TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:$(DB_PORT)/postgres?sslmode=disable
COMPOSE           := docker compose -f infra/docker-compose.yml
SWIFT_SPEC        := ios/Packages/APIClient/Sources/APIClient/openapi.yaml

.PHONY: help
help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

.PHONY: check
check: ## Check that Go and a running Docker are available
	@command -v go >/dev/null || { echo "Go is not installed. Install it with: brew install go"; exit 1; }
	@command -v docker >/dev/null || { echo "Docker is not installed. Install Docker Desktop: https://www.docker.com/products/docker-desktop/"; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "Docker is installed but not running. Open Docker Desktop, wait for it to start, then run make again."; exit 1; }
	@case "$(DATABASE_URL)" in *:$(DB_PORT)/*) ;; *) echo "Note: DATABASE_URL points at $(DATABASE_URL), not the Docker database on port $(DB_PORT). Check your .env.";; esac

.PHONY: db
db: check ## Start Postgres in Docker on port 54320, wait, and create the app database
	$(COMPOSE) up -d --wait db
	@$(COMPOSE) exec -T db psql -U postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'linguascroll'" | grep -q 1 \
		|| $(COMPOSE) exec -T db createdb -U postgres linguascroll

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
seed: ## Load languages, config defaults, sample clips and YouTube sources
	cd backend && go run ./cmd/seed
	$(MAKE) sources

.PHONY: sources
sources: ## Load infra/youtube_sources.yaml into youtube_sources
	cd backend && go run ./cmd/sources ../infra/youtube_sources.yaml

.PHONY: recheck
recheck: ## Re-check every live clip against the current language rules and remove failures
	cd backend && go run ./cmd/recheck

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
