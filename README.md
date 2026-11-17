# LinguaScroll

A vertical swipe feed of short videos in the language you're learning, so learning feels like watching TV. iOS first (SwiftUI), Go backend.

**Current target: v0**, a YouTube-only build: the feed is YouTube Shorts found with the YouTube Data API, ranked from metadata and how each device watches. No accounts (each install gets a random ID in the Keychain), no uploads, no ads.

- Spec: see the "v0 build" tab of the product spec doc (modules V1 to V7).
- Project skills for Claude live in `.claude/skills/` (UX, iOS, backend, QA, feed ranking, YouTube rules).

## Run it locally

No API keys needed. One-time setup on a Mac:

1. Install Go and Node: `brew install go node` (Go version: see `backend/go.mod`; Go downloads a newer toolchain itself if needed).
2. Install [Docker Desktop](https://www.docker.com/products/docker-desktop/) and open it. It must be running whenever you use `make dev`, `make db` or `make test`.
3. Optional: `cp .env.example .env` to change settings. If you made a `.env` before 2026-11-17, update its database URLs to port 54320.

The database runs in Docker on **port 54320** (not 5432), so a Postgres you already have from Homebrew or Postgres.app doesn't get in the way. `make check` says what's missing.

```sh
make dev     # checks Go and Docker, Postgres in Docker, migrations, seed, then API on :8080 and the worker
make test    # Go tests (each test gets its own throwaway database)
make lint    # go vet, gofmt, OpenAPI lint
make gen     # regenerate the Go server interface and Swift client spec after editing api/openapi.yaml
make help    # everything else
```

Try it: `curl localhost:8080/healthz`. Every other route except `/v1/config`, `/v1/languages` and `POST /v1/sessions/anonymous` needs a token from that last one.

The seed loads 15 languages, config defaults, starter YouTube search terms and 30 placeholder clips. The placeholder clips have fake YouTube IDs; real clips arrive with the V3 catalog job.

## Layout

```
api/openapi.yaml       the contract; Go server interface and Swift client are generated from it
backend/cmd/           api, worker (River jobs), migrate, seed
backend/internal/      api (generated), server, jobs, platform (config, db, http, auth, logging)
backend/migrations/    goose SQL, forward-only
backend/seed/          idempotent reference and dev data
ios/Packages/APIClient generated Swift client (swift-openapi-generator plugin)
infra/                 docker-compose
```
