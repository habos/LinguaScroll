-- YouTube compliance queries. Every query must return 0 rows.
-- Run: psql "$DATABASE_URL" -f qa/sql/compliance.sql
-- Each query selects a label so failing rows are easy to read.

-- Y8: live YouTube data older than 30 days (must be refreshed or removed).
SELECT 'Y8 stale live clip' AS check, id, youtube_video_id, refreshed_at
  FROM videos
 WHERE source = 'youtube' AND status = 'live'
   AND refreshed_at < now() - interval '30 days';

-- Y9: removed clips that still hold YouTube text fields.
SELECT 'Y9 removed clip keeps text' AS check, id, youtube_video_id
  FROM videos
 WHERE source = 'youtube' AND status = 'removed'
   AND (youtube_title IS NOT NULL
        OR youtube_description IS NOT NULL
        OR youtube_channel_title IS NOT NULL
        OR cardinality(youtube_tags) > 0);

-- Y11: live clips outside the content filters stored in the table
-- (longer than 180 s, or not vertical: aspect = width / height).
SELECT 'Y11 live clip outside filters' AS check, id, youtube_video_id, duration_ms, aspect
  FROM videos
 WHERE source = 'youtube' AND status = 'live'
   AND (duration_ms > 180000 OR aspect >= 1);

-- Y13: a UTC day over the quota budget in app_config.
SELECT 'Y13 quota over budget' AS check, q.day, q.units
  FROM youtube_quota q
 WHERE q.units > COALESCE((SELECT (value #>> '{}')::int FROM app_config
                            WHERE key = 'youtube.daily_quota_units'), 9000);
