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
| V1-09 **QA+** | Expired `sessions` rows are cleaned up | A | `engagement.TestCleanupSessions` (daily `session_cleanup` job) | pass @8ddf657 (PR #8) |

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

## V3 YouTube catalog (PR #6)

| ID | Criterion | Type | Covered by | Status |
|---|---|---|---|---|
| V3-01 | Fake client, Spanish run over 50 IDs (20 good, 5 landscape, 5 kids, 5 `en`, 15 untagged Spanish titles) stores exactly 35 | A | `catalog.TestSearchAcceptanceMix` | pass @042aa26 (PR #6) |
| V3-02 | "para principiantes" title → level 1 or 2, `level_source = keyword` | A | `catalog.TestGuessLevel`, `TestStoredLevelFromKeywords` | pass @042aa26 (PR #6) |
| V3-03 | Search job stops at the daily unit budget and resumes after midnight UTC | A | `catalog.TestQuotaStopsAndResumesAtMidnightUTC`, `TestYouTubeQuotaErrorStopsQuietly` | pass @042aa26 (PR #6) |
| V3-04 | Clip refreshed 26 days ago is re-read; a deleted one ends `removed` with text fields cleared | A | `catalog.TestRefresh` | pass @042aa26 (PR #6) |
| V3-05 | Real run with an API key: es, fr, ja each get ≥ 50 live clips in one day | M | [manual/catalog-real-run.md](manual/catalog-real-run.md) | todo |
| V3-06 **QA+** | `es-419` audio tag counts as `es`; `en-US` tag on a Spanish search is dropped | A | `catalog.TestClassifyLanguage` | pass @042aa26 (PR #6) |
| V3-07 **QA+** | Age-restricted and > 180 s clips are dropped | A | `catalog.TestKeepFilters` | pass @042aa26 (PR #6) |
| V3-08 **QA+** | Quota counter: `search.list` = 100 units, `videos.list` = 1 unit per ≤ 50 IDs | A | `catalog.TestSearchAcceptanceMix`, `youtube.TestHTTPClient` | pass @042aa26 (PR #6) |
| V3-09 **QA+** | No test or CI path calls the real YouTube API (fake used when `FAKE_VENDORS=1` or no key) | A | `youtube.TestHTTPClient` (httptest), config tests | pass @042aa26 (PR #6) |
| V3-10 **QA+** | Level refinement: 20 completed views → level = rounded average viewer level, `level_source = engagement` | A | `catalog.TestRefineLevels` | pass @042aa26 (PR #6) |
| V3-12 **QA+** | Refresh removes clips that became made-for-kids or age-restricted | A | `catalog.TestRefresh` (kids, adult cases) | pass @042aa26 (PR #6) |
| V3-13 **QA+** | Removed clips keep only IDs: stats, category, publish time cleared (Y9) | A | `catalog.TestRefresh`; `qa/sql/compliance.sql` Y9 | pass @042aa26 (PR #6), worker run + compliance.sql 0 rows |
| V3-11 **QA+** | `sql/compliance.sql` returns 0 rows after a refresh run | A | QA run: worker with fake vendors after backdating, 0 rows | pass @042aa26 (PR #6); CI step after a refresh still todo |
| V3-14 **QA+** | Clips that only mention the language are dropped for every language: hashtag-only titles (even with a matching audio tag), titles mostly in another script, low share of the language's script, `defaultLanguage` mismatch | A | `catalog.TestClassifyLanguageRejectsLanguageMentions`, `TestScriptShare` | added (thread "Keep off-language clips out of the feed") |
| V3-15 **QA+** | `youtube.blocked_channels` clips are never stored (reject reason `channel`) | A | `catalog.TestSearchDropsBlockedChannelsAndHashtagSpam` | added |
| V3-16 **QA+** | `make recheck` removes stored clips failing the current rules (data cleared, `hold_reason` language/channel), keeps manual ones; refresh rechecks reported clips | A | `catalog.TestRecheckLanguagesPurgesOldMismatches`, `TestRefreshRechecksReportedClips` | added |
