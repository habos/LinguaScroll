# LinguaScroll

A vertical swipe feed of short videos in the language you're learning, so learning feels like watching TV. iOS first (SwiftUI), Go backend.

**Current target: v0**, a YouTube-only build: the feed is YouTube Shorts found with the YouTube Data API, ranked from metadata and how each device watches. No accounts (each install gets a random ID in the Keychain), no uploads, no ads.

- Spec: see the "v0 build" tab of the product spec doc (modules V1 to V7).
- Project skills for Claude live in `.claude/skills/` (UX, iOS, backend, QA, feed ranking, YouTube rules).

## Run it locally

Needs Go (see `backend/go.mod`), Docker and Node (for the OpenAPI linter). No API keys.

```sh
make dev     # Postgres in Docker, migrations, seed, then API on :8080 and the worker
make test    # Go tests (each test gets its own throwaway database)
make lint    # go vet, gofmt, OpenAPI lint
make gen     # regenerate the Go server interface and Swift client spec after editing api/openapi.yaml
make help    # everything else
```

Try it: `curl localhost:8080/healthz`. Every other route answers 401 without a token, then 501 until its module (V2 to V4) is built.

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
