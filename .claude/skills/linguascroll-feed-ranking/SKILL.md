---
name: linguascroll-feed-ranking
description: The LinguaScroll feed algorithm: candidate selection, scoring formula from YouTube metadata and device engagement, explore share, diversity rules and how to tune it. Use when changing, debugging or tuning what clips the feed shows.
---

# LinguaScroll feed ranking (v0)

Goal: for one device in one language, pick clips the learner will watch to the end, at roughly their level, without repetition. Only YouTube metadata and in-app engagement are available (no transcripts).

## Pipeline for `GET /v1/feed?language=&cursor=`

1. Candidates: `live` clips in the language; level within `feed.level_window` (1) of the device's level, unknown level passes; not hidden by this device; channel not blocked; not in `feed_seen` for this device in the last 30 days. Take the top 400 by precomputed base score (quality + freshness, refreshed by the stats job).
2. Score each candidate:

   score = 0.25 c + 0.15 p_views + 0.15 p_likes + 0.10 v + 0.15 k + 0.10 g + 0.05 f - 0.30 h + e

   - c: in-app completion rate, smoothed (completions + 2) / (views + 4)
   - p_views: percentile of YouTube view count within the language
   - p_likes: percentile of YouTube likes per view within the language
   - v: level closeness (1 same level; 0.5 one away or unknown)
   - k: channel affinity (1 if this device liked/saved a clip from the channel in 30 days; 0.5 if it completed >= 2 of its clips; else 0)
   - g: device's share of completed views in this YouTube category over its last 100 views
   - f: freshness (1 if published within 30 days, linear to 0 at one year)
   - h: in-app hide rate
   - e: random 0 to 0.05
   Weights live in `app_config` key `feed.weights`, not in code.
3. New device: no k, g or c history, so YouTube quality and global completion carry the first pages. That's intended.
4. Explore: `feed.explore_share` (0.15) of each page is clips with < 50 in-app views, random from candidates.
5. Diversity: at most one clip per channel in any `feed.channel_spacing` (5) consecutive items, across page boundaries (carry the last 4 channel IDs in the cursor). Music category (10) capped at `feed.music_cap` (0.2) of a page.
6. Record served clips in `feed_seen`. If fewer than a page remain, allow clips served more than 7 days ago.
7. Page size `feed.page_size` (10). Cursor is opaque; no other per-request server state.

## Where it lives (implemented 2026-11-10)

- `backend/internal/feed`: `rank.go` (pure: `Score`, signal helpers, `Rank`, `Assemble`), `service.go` (SQL + page assembly), `cursor.go`, `stats.go` (`feed_stats` job every 10 min), `config.go` (reads `feed.*`).
- `server/feed.go` maps it to `GET /v1/feed`. Errors: 400 `unknown_language`, 400 `bad_cursor`. An empty page with `next_cursor: null` means nothing is left to serve.
- `feed_stats` writes `video_stats.views_pct`, `likes_pct`, `base_score` for every live clip (creating the row if missing). The events rollup owns the counters, so it must upsert, never plain-insert.
- Implementation choices the formula above leaves open:
  - h = hides / (impressions + 5), capped at 1, so one early hide doesn't bury a clip.
  - Explore pool is random from the whole filtered set (not the top 400), so clips with low base score still get views. Explore slots per page = page_size x share rounded randomly (1 or 2 at defaults), never the first slot.
  - k = 0.5 and g come from the device's last 100 views (>= 2 s, last 180 days); k = 0.5 counts only those within 30 days.
  - When no clip fits the rules, the music cap is relaxed first, then channel spacing, so a small catalog still fills a page.
- Extra keys: `feed.explore_max_views` (50), `feed.candidates` (400).
- Tests: `go test ./internal/feed/`; `FEED_LOAD_TEST=1` runs the 20,000-clip latency check (p95 about 45 ms in a cloud container on 2026-11-10).
- Tuning log: `docs/feed-tuning-log.md`.

## Level

- Level comes from the source hint, then metadata keywords, else null. Nightly, once a clip has 20 completed views, its level becomes the rounded average level of those viewers (`level_source = engagement`).
- Treat level as rough. Don't raise `v`'s weight to compensate for bad level data; fix the data.

## Signals from events

- A view counts at >= 2 s watched. `completed` comes from the client. Loops count as engagement but don't double count views.
- `video_stats` and `video_level_stats` are rolled up every 5 minutes from `events`.
- Hides and channel blocks are hard filters for the device and a soft negative (h) globally.

## Tuning rules

1. Change weights in `app_config`, never by editing constants. Record each change (date, old, new, why).
2. Measure before and after on: completion rate, average watch ratio, hide rate, share of sessions > 5 minutes, wrong-language reports.
3. One change at a time; give it at least a few days of data.
4. Keep the score unit tests passing with fixed inputs; add a case for every bug.
5. Performance budget: p95 < 200 ms at 20,000 live clips per language. Precompute anything global (percentiles, base score) in the stats job.

## Later phases (don't build in v0)

Native videos mixed with YouTube (tunable mix), creator boost from likes, ad slots between native videos, word-based resurfacing from My words, level from transcripts.

Source of truth: spec doc "v0 build" tab, V4 (https://claude.ai/artifact/54gzKQZdV44NukYnP1DikG).
