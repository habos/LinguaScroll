---
name: linguascroll-qa
description: Test strategy and release checks for LinguaScroll (Go backend and SwiftUI iOS app with the YouTube player). Use when writing tests, planning a test pass, reviewing a change for coverage, or checking a module against its acceptance criteria.
---

# LinguaScroll QA

## Approach

- Every module in the spec (V1 to V7) has acceptance criteria. A module is done only when each one has a test or a recorded manual check.
- Automated first: Go table-driven tests against real Postgres; Swift unit tests with protocol fakes. Manual only for what needs a real device (swipe over the WKWebView, audio, ttff).
- Fakes: `FAKE_VENDORS=1` gives a fake `YouTubeClient` and `LanguageDetector`; never hit the real YouTube API in CI (it spends quota).
- When a bug is found, first write the failing test, then fix.

## Backend checks (must stay green)

- Fresh clone + `make dev` runs with no keys; `make migrate && make seed` gives 15 languages and ~30 sample clips.
- `openapi.yaml` lints clean; `make gen` output compiles in Go and Swift.
- Protected route without token: 401 in the error shape.
- Age: birth year 2016 gives 403 `age_blocked` and no row; birth year is never stored.
- Same install ID twice returns the same user ID. Removing the last language: 422. After `DELETE /v1/me` the old token is 401.
- Catalog with the fake client: Spanish run over 50 IDs (20 good, 5 landscape, 5 made for kids, 5 tagged `en`, 15 untagged Spanish titles) stores exactly 35.
- "para principiantes" title gives level 1 or 2, `level_source = keyword`.
- Search job stops at the daily quota and resumes after midnight UTC.
- Refresh: clip refreshed 26 days ago is re-read; a deleted one becomes `removed` with text fields cleared.
- Feed: fresh device gets quality-ordered page with 1 to 2 explore clips; likes from a channel raise it; no channel twice in any 5 consecutive items across page boundaries; hidden/blocked never return; 3 reports from 3 devices remove a clip; no repeats across 20 pages with 500 clips; score unit tests with fixed inputs.
- Events: duplicate client IDs are counted once; batches > 200 rejected.
- Load: p95 `GET /v1/feed` < 200 ms at 20,000 live clips per language.

## iOS checks

- Fresh install: onboarding to feed in under 30 s.
- Delete and reinstall: no onboarding, same user, likes intact. Age block survives reinstall.
- Kill mid-batch: pending events sent exactly once after relaunch.
- Unit tests: Keychain install ID, 401 re-register-and-retry, event retry, player pool state machine.
- Like/save rollback toast when the call fails (use a failing fake).

## YouTube compliance checks (block release if any fail)

- View-hierarchy snapshot test: nothing overlaps the player frame.
- No gesture actions on the player; like only via the bar.
- Clip doesn't start until > 50% visible; never two audio tracks through 30 fast swipes; pauses on background.
- Player >= 200x200 pt. Referer/baseURL is `https://<bundle id>`.
- Stored YouTube data older than 30 days is refreshed or removed (query `videos` for `refreshed_at < now() - 30 days AND status = 'live'`, expect 0).

## Real-device manual pass (each V6 change)

1. Swipe over the player moves to the next clip; taps still reach YouTube controls.
2. Median `ttff_ms` from the event log (target < 800 ms on Wi-Fi; log it, don't gate).
3. Memory flat over 50 swipes (Instruments; still 2 web views).
4. Captions show in the clip language when available; off when disabled in Settings.
5. Error codes 101/150 auto-advance and the clip never returns.

## Feed quality spot-check (weekly while tuning)

For each of 3 languages, swipe 30 clips as a fresh device and note: wrong-language audio, music share (cap 20%), repeated channels, inappropriate clips. More than 2 wrong-language clips in 30 means tune `youtube_sources.yaml` or `min_language_confidence`.

## When asked to test something

1. Find the module and its acceptance criteria in the spec.
2. List the cases (happy path, edge, failure), mark each automated or manual.
3. Write the automated ones; give step-by-step instructions for the manual ones.
4. Report pass/fail per criterion, with output for failures.

Source of truth: acceptance criteria in the spec doc's V1 to V7 tabs (https://claude.ai/artifact/54gzKQZdV44NukYnP1DikG).
