# v0 acceptance matrix

Source of truth for criteria: the spec's "v0 build" tab, modules V1 to V7. Rows marked **QA+** are extra cases QA added (not in the spec) from reviews or the skills' check lists.

Type: **A** = automated test, **M** = manual (script in `manual/`). Status values are in [README](README.md#statuses-used-in-the-matrix).

## V1 Backend foundation (PR #1, merged at 1d0595d)

| ID | Criterion | Type | Covered by | Status |
|---|---|---|---|---|
| V1-01 | Fresh clone + `make dev` gives a running API and worker with no API keys | M | [run 2026-11-10](runs/2026-11-10-pr1-v1.md) | pass @1d0595d |
| V1-02 | `make migrate && make seed` on an empty DB gives 15 languages and 30 sample clips | A+M | `migrations` tests (idempotent migrate+seed); CI "Migrate and seed" step | pass @1d0595d |
| V1-03 | `openapi.yaml` lints clean; `make gen` gives the Go interface and a Swift package that compiles | A | CI `make gen-check`, `make lint`, `ios-client` job | pass @1d0595d |
| V1-04 | Protected route without a token returns 401 in the error shape | A | `server` tests (public vs protected, 401 missing/bad/expired) | pass @1d0595d |
| V1-05 | CI is green | A | GitHub Actions on PR #1 | pass @1d0595d |
| V1-06 **QA+** | Bogus bearer tokens from one address still hit the rate limit (429) | A | `server.TestRateLimitIgnoresMadeUpTokens` | pass @d0dd1b8 (PR #3) |
| V1-07 **QA+** | `ensure_event_partitions` keeps working when `events_default` has rows | A | `migrations.TestEventPartitionsAbsorbStrayDefaultRows` (migration 00003) | pass @d0dd1b8 (PR #3) |
| V1-08 **QA+** | Before hosting: rate limit keys on the real client IP behind the proxy (`Fly-Client-IP`), not the proxy's address | A | | gap (pre-deploy) |
| V1-09 **QA+** | Expired `sessions` rows are cleaned up | A | | gap |

## V2 Devices and languages (PR #3)

| ID | Criterion | Type | Covered by | Status |
|---|---|---|---|---|
| V2-01 | Birth year 2016 gets 403 `age_blocked` and no row | A | `devices.TestUnder13IsBlockedAndNothingStored` | pass @d0dd1b8 (PR #3) |
| V2-02 | Registering twice with the same install ID returns the same user ID | A | `devices.TestRegisterTwiceReturnsSameUser` | pass @d0dd1b8 (PR #3) |
| V2-03 | Adding Japanese at level 3 then removing Spanish leaves one active language | A | `devices.TestLanguages` | pass @d0dd1b8 (PR #3) |
| V2-04 | Removing the last language returns 422 | A | `devices.TestLanguages` | pass @d0dd1b8 (PR #3) |
| V2-05 | After `DELETE /v1/me` the old token returns 401 | A | `devices.TestDeleteMe` | pass @d0dd1b8 (PR #3) |
| V2-06 **QA+** | Birth year is never stored (no column, not in logs) | A | `devices.TestBirthYearIsNotStored` | pass @d0dd1b8 (PR #3) |
| V2-07 **QA+** | Age boundaries: exactly 13 → `teen`, 17 → `teen`, 18 → `adult`, 12 → 403 | A | `devices.TestAgeBand` | pass @d0dd1b8 (PR #3) |
| V2-08 **QA+** | Re-register keeps existing languages and likes; old token still valid or cleanly replaced | A | `devices.TestRegisterTwiceReturnsSameUser` | pass @d0dd1b8 (PR #3) |
| V2-09 **QA+** | Level outside 1 to 6, unknown language code, non-learnable language → 400/422 in error shape | A | `devices.TestRegisterValidation`, `devices.TestLanguages` | pass @d0dd1b8 (PR #3) |
| V2-10 **QA+** | `GET /v1/languages` returns 15 learnable plus native options, no token needed | A | `devices.TestPublicEndpoints` | pass @d0dd1b8 (PR #3) |
| V2-11 **QA+** | `DELETE /v1/me` removes likes/saves/hides/blocks/reports and nulls `events.user_id` | A | `devices.TestDeleteMe`, `migrations.TestDeleteUserCascades` | pass @d0dd1b8 (PR #3) |

## V3 YouTube catalog

| ID | Criterion | Type | Covered by | Status |
|---|---|---|---|---|
| V3-01 | Fake client, Spanish run over 50 IDs (20 good, 5 landscape, 5 kids, 5 `en`, 15 untagged Spanish titles) stores exactly 35 | A | | todo |
| V3-02 | "para principiantes" title → level 1 or 2, `level_source = keyword` | A | | todo |
| V3-03 | Search job stops at the daily unit budget and resumes after midnight UTC | A | | todo |
| V3-04 | Clip refreshed 26 days ago is re-read; a deleted one ends `removed` with text fields cleared | A | | todo |
| V3-05 | Real run with an API key: es, fr, ja each get ≥ 50 live clips in one day | M | [manual/catalog-real-run.md](manual/catalog-real-run.md) | todo |
| V3-06 **QA+** | `es-419` audio tag counts as `es`; `en-US` tag on a Spanish search is dropped | A | | todo |
| V3-07 **QA+** | Age-restricted and > 180 s clips are dropped | A | | todo |
| V3-08 **QA+** | Quota counter: `search.list` = 100 units, `videos.list` = 1 unit per ≤ 50 IDs | A | | todo |
| V3-09 **QA+** | No test or CI path calls the real YouTube API (fake used when `FAKE_VENDORS=1` or no key) | A | | todo |
| V3-10 **QA+** | Level refinement: 20 completed views → level = rounded average viewer level, `level_source = engagement` | A | | todo |
| V3-11 **QA+** | `sql/compliance.sql` returns 0 rows after a refresh run | A | | todo |

## V4 Feed, likes, saves and events

| ID | Criterion | Type | Covered by | Status |
|---|---|---|---|---|
| V4-01 | Fresh device gets a page ordered mostly by YouTube quality, with 1 or 2 explore clips | A | | todo |
| V4-02 | After liking 3 clips from one channel, the next page includes that channel (within spacing) | A | | todo |
| V4-03 | No channel twice in any 5 consecutive items, across page boundaries | A | | todo |
| V4-04 | Hidden clip and blocked channel's clips never come back | A | | todo |
| V4-05 | Three reports from three devices remove a clip from every feed | A | | todo |
| V4-06 | No clip repeats across 20 consecutive pages (500 live clips) | A | | todo |
| V4-07 | Score unit tests with fixed inputs, including the new-device case | A | | todo |
| V4-08 **QA+** | Duplicate client event IDs counted once; batch > 200 rejected | A | | todo |
| V4-09 **QA+** | p95 `GET /v1/feed` < 200 ms at 20,000 live clips per language | A (bench) | | todo |
| V4-10 **QA+** | Like/save/block PUT and DELETE are idempotent; hide is PUT only | A | | todo |
| V4-11 **QA+** | Event `at` far in the future or past is rejected or clamped, and never breaks partition creation | A | | todo |
| V4-12 **QA+** | Music-category clips ≤ `feed.music_cap` of a page | A | | todo |
| V4-13 **QA+** | Same report from the same device twice counts once | A | | todo |
| V4-14 **QA+** | `removed` clips never appear in feed, likes or saves lists | A | | todo |

## V5 iOS shell and onboarding

| ID | Criterion | Type | Covered by | Status |
|---|---|---|---|---|
| V5-01 | Fresh install in simulator: onboarding to feed placeholder in < 30 s | M | [manual/ios-install.md](manual/ios-install.md) | todo |
| V5-02 | Delete and reinstall: no onboarding, same user ID, likes still there | M | [manual/ios-install.md](manual/ios-install.md) | todo |
| V5-03 | Birth year 2016 shows the blocked screen, which survives reinstall | M | [manual/ios-install.md](manual/ios-install.md) | todo |
| V5-04 | Killing the app mid-batch sends pending events once after relaunch | A+M | | todo |
| V5-05 | Unit tests: Keychain install ID, 401 recovery, event retry | A | | todo |
| V5-06 **QA+** | No IDFA access, no ATT prompt (no `AdSupport`/`AppTrackingTransparency` import) | A | | todo |
| V5-07 **QA+** | 401 recovery retries once only (no loop when re-register also fails) | A | | todo |
| V5-08 **QA+** | No hard-coded user-facing strings (String Catalog) | A (lint) | | todo |

## V6 iOS YouTube feed (YouTube-compliance heavy)

| ID | Criterion | Type | Covered by | Status |
|---|---|---|---|---|
| V6-01 | Swiping over the player moves to the next clip on a real device | M | [manual/device-pass.md](manual/device-pass.md) | todo |
| V6-02 | Median `ttff_ms` for cued clips < 800 ms on Wi-Fi (target, logged) | M | [manual/device-pass.md](manual/device-pass.md) | todo |
| V6-03 | Never two audio tracks at once through 30 fast swipes | M | [manual/device-pass.md](manual/device-pass.md) | todo |
| V6-04 | A clip doesn't start until > 50% visible | A+M | | todo |
| V6-05 | View-hierarchy snapshot test: nothing overlaps the player frame | A | | todo |
| V6-06 | Memory flat over 50 swipes (still 2 web views) | M | [manual/device-pass.md](manual/device-pass.md) | todo |
| V6-07 | Clips with captions show YouTube's captions in the clip language | M | [manual/device-pass.md](manual/device-pass.md) | todo |
| V6-08 **QA+** | No gesture recognisers or tap actions attached to the player view | A | | todo |
| V6-09 **QA+** | Player ≥ 200×200 pt on the smallest supported device (iPhone SE) | A | | todo |
| V6-10 **QA+** | `baseURL` is `https://<bundle id lowercased>` | A | | todo |
| V6-11 **QA+** | Errors 101/150 auto-advance and call hide; clip never returns | A+M | | todo |
| V6-12 **QA+** | Pauses on backgrounding; no audio from offscreen player | M | [manual/device-pass.md](manual/device-pass.md) | todo |
| V6-13 **QA+** | Like/save roll back with a toast when the call fails | A | | todo |
| V6-14 **QA+** | Share link is `https://youtube.com/shorts/<id>` | A | | todo |

## V7 iOS Saved, Liked and settings

| ID | Criterion | Type | Covered by | Status |
|---|---|---|---|---|
| V7-01 | Saving a clip in the feed shows it at the top of Saved on return to Me | A+M | | todo |
| V7-02 | Playing from Saved swipes through saved clips only, in order | M | | todo |
| V7-03 | Changing Spanish's level changes the next feed page's level window | A | | todo |
| V7-04 | Reset my data returns to onboarding and old likes are gone | M | [manual/ios-install.md](manual/ios-install.md) | todo |
| V7-05 | Unblocking a channel lets its clips appear again | A | | todo |
| V7-06 **QA+** | Thumbnails load from YouTube's public URL; nothing cached to disk as stored data | A | | todo |
| V7-07 **QA+** | Captions off in Settings → `cc_load_policy` not set | A | | todo |
