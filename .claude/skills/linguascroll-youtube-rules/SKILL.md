---
name: linguascroll-youtube-rules
description: YouTube API Services compliance, quota budgeting and embedded-player rules that LinguaScroll must follow. Use whenever code, design or data touches YouTube: the Data API, stored metadata, the IFrame player in WKWebView, or anything shown near a YouTube clip.
---

# YouTube rules for LinguaScroll

v0 is a YouTube-only app, so breaking these rules can kill the API key or the App Store submission. Check them before shipping any change that touches YouTube.

## Legal route

- Use only the YouTube Data API v3 and the official IFrame player. No scraping, no downloading audio/video, no yt-dlp, no generating captions from YouTube media.
- No TikTok or Instagram content at all.
- Captions: only YouTube's own, shown inside its player (`cc_load_policy=1`, `cc_lang_pref`). We can't fetch caption tracks (owner-only) or draw our own, so no tap-to-translate on YouTube clips.

## Embedded player rules

- Nothing overlaid on the player: no buttons, text, gradients, captions or gestures in its frame.
- No touch actions on the player (no tap/double-tap to like). Our controls go in the bar under it.
- Player at least 200x200 pt.
- Autoplay only when > 50% visible. Only one autoplaying player per screen. No background audio from offscreen players; pause on background.
- WKWebView must send a Referer: load the IFrame API via `loadHTMLString(_:baseURL:)` with baseURL `https://<bundle id lowercased>`.
- Don't hide or obscure YouTube branding or controls; channel name links out to the YouTube app or Safari.
- Share links point to `https://youtube.com/shorts/<id>`.

## Data storage rules

- Store only IDs and the metadata we need (title, channel, category, tags, duration, stats, language/level decisions).
- Refresh or delete stored API data within 30 days. v0 re-reads clips older than 25 days in batches of 50; gone, private or non-embeddable clips become `removed` with title, description, tags and channel title cleared.
- Thumbnails: use YouTube's public thumbnail URL; never store images.

## Ads and money

- No ads on or next to screens showing YouTube content. v0 has no ads; ads return only between native videos in a later phase.
- Don't sell or paywall access to YouTube content.

## Content filters

Keep a clip only if: embeddable, not made for kids, not age-restricted, <= 180 s, vertical (embed height > width). Search with `safeSearch=strict`. Users can report; 3 distinct reports remove a clip.

## Quota (default 10,000 units/day, resets midnight Pacific; we count per UTC day and stay under)

| Call | Cost |
|---|---|
| `search.list` | 100 units |
| `videos.list` (up to 50 IDs) | 1 unit |
| `channels.list` | 1 unit |

- Budget `youtube.daily_quota_units` = 9,000, counted in Postgres; jobs stop when spent.
- About 4,500 new clips a day across all languages is the ceiling; plenty for v0.
- Prefer seed channels (cheap via uploads playlists/`videos.list`) over keyword search where possible.
- Split search budget across languages, more to languages with fewer live clips.
- Never call the real API from tests or CI (`FAKE_VENDORS=1`).
- Raising quota needs YouTube's compliance audit; do that before public launch.

## App Store risk

An app that's mostly a YouTube viewer risks rejection under guideline 4.2 (minimum functionality / aggregators). v0 is for the owner and testers only; submit once native uploads or the learning layer make it more than a viewer.

## Review checklist for any YouTube-touching change

- [ ] No view, gesture or overlay inside the player frame
- [ ] Autoplay/visibility/single-audio rules intact
- [ ] Referer/baseURL unchanged
- [ ] New stored fields covered by the 25-day refresh and cleared on removal
- [ ] Quota cost estimated and within the daily budget
- [ ] No ads or paywall near YouTube content

Rules read 2026-10-22 from YouTube's Required Minimum Functionality and Developer Policies pages; re-check those pages before a public launch, they change.
