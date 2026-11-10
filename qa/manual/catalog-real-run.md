# Real YouTube catalog run (V3-05) and weekly feed spot-check

These are the only QA steps that spend real YouTube quota. Never run them from CI.

## Real catalog run (V3-05)

1. Put a real `YOUTUBE_API_KEY` in `.env` (free Google Cloud project, YouTube Data API v3 enabled). Leave `FAKE_VENDORS` empty.
2. `make dev`, let the worker run the search jobs for one UTC day (or trigger them by hand).
3. Check:
   ```sql
   SELECT language, count(*) FROM videos WHERE status = 'live' AND source = 'youtube' GROUP BY 1 ORDER BY 1;
   SELECT day, units FROM youtube_quota ORDER BY day DESC LIMIT 3;
   ```
4. **Pass:** es, fr and ja each have at least 50 live clips; units for the day ≤ 9,000.
5. Run `psql "$DATABASE_URL" -f qa/sql/compliance.sql`: every query returns 0 rows.

## Feed quality spot-check (weekly while tuning)

For each of es, fr, ja: fresh device (or new install ID), swipe 30 clips, note:

| Language | Wrong-language audio | Music clips | Repeated channel within 5 | Inappropriate | Notes |
|---|---|---|---|---|---|
| es | /30 | /30 | | | |
| fr | /30 | /30 | | | |
| ja | /30 | /30 | | | |

More than 2 wrong-language clips in 30 means tuning `infra/youtube_sources.yaml` or `youtube.min_language_confidence`. Music above 6 of 30 (20%) means checking `feed.music_cap`. Send findings to the feed-tuning thread.
