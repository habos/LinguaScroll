# YouTube compliance checks (release-blocking)

Any failure here blocks a release or TestFlight build. Rules come from `.claude/skills/linguascroll-youtube-rules` (YouTube Required Minimum Functionality and Developer Policies, read 2026-10-22). Re-read those pages before any public launch.

| # | Rule | How to verify | Matrix |
|---|---|---|---|
| Y1 | Nothing drawn over the player frame | View-hierarchy snapshot test; visual check in device pass | V6-05 |
| Y2 | No gestures or tap actions on the player | Unit test that the player view has no gesture recognisers of ours; like only via the bar | V6-08 |
| Y3 | Autoplay only when > 50% visible | Test on visibility calculation; device pass step | V6-04 |
| Y4 | One playing player; no background or offscreen audio | Device pass: 30 fast swipes, background the app | V6-03, V6-12 |
| Y5 | Player ≥ 200×200 pt | Layout test on iPhone SE size | V6-09 |
| Y6 | Referer: `loadHTMLString` with baseURL `https://<bundle id lowercased>` | Unit test on the player config | V6-10 |
| Y7 | YouTube branding stays visible; YouTube's controls may be off only through the official `controls=0` player parameter (Harry's choice, 2026-11-11; revisit before the API compliance audit); channel name links out to YouTube | Device pass | V6-01 |
| Y18 | Only the current clip plays; the next one is cued, never played muted offscreen to pre-buffer | Code review; device pass step 5 | V6-03 |
| Y8 | Stored API data refreshed or deleted within 30 days | `sql/compliance.sql` returns 0 rows | V3-04, V3-11 |
| Y9 | Removed clips keep only IDs: title, description, tags, channel title, category, publish time and stats cleared | `sql/compliance.sql` | V3-04 |
| Y10 | Only IDs + needed metadata stored; no thumbnails or media stored | Schema review; no image columns; V7 uses public thumbnail URL | V7-06 |
| Y11 | Content filters: embeddable, not for kids, not age-restricted, ≤ 180 s, vertical | V3 tests; `sql/compliance.sql` | V3-01, V3-07 |
| Y12 | `safeSearch=strict` on every search | V3 test asserting the fake client received it | V3-01 |
| Y13 | Quota stays under `youtube.daily_quota_units` (9,000) | V3 quota test | V3-03, V3-08 |
| Y14 | Real API never called from tests or CI | CI has no `YOUTUBE_API_KEY`; config forces fakes without a key | V3-09 |
| Y15 | No ads, paywall or overlays near YouTube content | Code review: no ad SDK in v0 | n/a |
| Y16 | Share links are `https://youtube.com/shorts/<id>` | Unit test | V6-14 |
| Y17 | Captions are YouTube's own only (`cc_load_policy`, `cc_lang_pref`); no caption fetching | Code review: no `captions.*` API calls | V6-07 |
