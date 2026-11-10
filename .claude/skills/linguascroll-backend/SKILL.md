---
name: linguascroll-backend
description: How to build the LinguaScroll Go backend: API, River worker, Postgres schema, OpenAPI contract, YouTube catalog jobs, and the playbook for adding a new language. Use when writing, reviewing or debugging LinguaScroll server code or data.
---

# LinguaScroll backend (Go)

## Stack

- Go API (`backend/cmd/api`) and Go worker (`backend/cmd/worker`, River jobs in Postgres). Postgres 16. No Redis in v0.
- `log/slog` JSON logs, config from env (`.env.example` lists every key; no secrets committed).
- goose migrations in `backend/migrations`. Feature code in `backend/internal/<module>`, vendor code only in `backend/internal/platform`.
- `api/openapi.yaml` (OpenAPI 3.1) is the contract. `make gen` produces the Go strict-server interface (oapi-codegen) and the Swift client. Change the YAML first, then regenerate.
- Makefile: `dev`, `test`, `lint`, `gen`, `migrate`, `seed`, `sources`. Fresh clone + `make dev` must run with no API keys (`FAKE_VENDORS=1`).
- Outside services behind interfaces with fakes: only `YouTubeClient` and `LanguageDetector` (lingua-go) in v0.

## Conventions

- REST + JSON under `/v1`. IDs UUIDv7 strings; times RFC 3339 UTC; durations integer ms named `*_ms`; language codes from `languages.code` (`es`, `pt-BR`, `zh-Hans`).
- Errors: status + `{"error": {"code": "snake_case", "message": "..."}}`. Lists: `?cursor=&limit=` in, `{"items": [...], "next_cursor": ...}` out. Cursors opaque.
- Middleware: bearer token, request ID, panic recovery, 120 req/min per token.
- Column names follow the full schema (F3) so v1 adds, never renames.

## Identity (V2)

- `POST /v1/sessions/anonymous` with install ID, birth year, languages, level. Under 13: 403 `age_blocked`, nothing stored. Store only `age_band` (`teen`/`adult`), never birth year.
- Same install ID again returns a new token for the existing user.
- Tokens: 32 random bytes base64url, stored as SHA-256, 180-day sliding expiry.
- Level 1 to 6; at least one active learning language (removing the last is 422).
- `DELETE /v1/me` removes the user and likes/saves/hides/blocks/reports; events keep rows with user ID nulled.

## Tables (v0)

`languages`, `users` (kind=anonymous, install_id unique), `sessions`, `learner_languages`, `videos` (source=youtube, metadata, language + language_source + confidence, level + level_source, has_captions, status live/removed, refreshed_at), `youtube_sources`, `likes`, `saves`, `hides`, `channel_blocks`, `reports`, `events` (partitioned by month), `video_stats`, `video_level_stats`, `feed_seen`, `app_config`.

## YouTube catalog (V3)

River jobs: `youtube_search` (spread through the day), `youtube_refresh` (daily), `youtube_level` (nightly). Details of quotas, filters and the 30-day rule are in the linguascroll-youtube-rules skill; follow it. Summary:
- Search `type=video`, `videoDuration=short`, `videoEmbeddable=true`, `safeSearch=strict`, `relevanceLanguage`, rotate `publishedAfter`.
- Keep only <= 180 s, embeddable, not made for kids, not age-restricted, vertical.
- Language: audio tag match > seed channel > text detection >= `youtube.min_language_confidence` > drop. Record the source.
- Level: source hint > `youtube.level_keywords` > null; nightly engagement refinement after 20 completed views overrides.
- Quota counted per UTC day in Postgres, stop at `youtube.daily_quota_units` (9,000).

## Feed and events (V4)

Ranking lives in the linguascroll-feed-ranking skill. Likes/saves/blocks are idempotent PUT/DELETE, hide PUT only, each also writes an event. Events: max 200 per batch, deduped by client ID, a view counts at >= 2 s. A 5-minute job rolls events into `video_stats` and `video_level_stats`. Reports from `reports.hide_after` (3) distinct devices set a clip to `removed`.

## Adding a language (data, not code)

Adding a language must never need a code change. Checklist:
1. Add a row to the `languages` seed: code, English and native name, script, direction (ltr/rtl), `learnable = true`, launch order.
2. Add search terms and seed channels (with optional level hints) for it in `infra/youtube_sources.yaml`; run `make sources`.
3. Add level keywords in that language to `youtube.level_keywords` in `app_config`.
4. Confirm `LanguageDetector` (lingua-go) supports it; if not, it can still work via audio tag and seed channels, but say so.
5. Check the iOS side needs nothing: name comes from `Locale`, RTL handled by leading/trailing layout.
6. Run the catalog for one day and check it reaches >= 50 live clips; spot-check 10 for wrong-language audio.
If any step needs code, treat that as a bug in the data model and fix it generally.

## When writing code

1. Name the module (V1 to V4) and stay in it. Update `openapi.yaml` first for any API change.
2. Table-driven tests against a real Postgres (CI has a Postgres service); fakes for vendors.
3. Keep p95 feed latency < 200 ms at 20,000 live clips per language.

Source of truth: spec doc "v0 build" tab, V1 to V4 (https://claude.ai/artifact/54gzKQZdV44NukYnP1DikG).
