-- +goose Up
-- Feed ranking (V4). Language-wide percentiles are precomputed by the
-- feed_stats job so GET /v1/feed only reads them.
ALTER TABLE video_stats
    ADD COLUMN views_pct real NOT NULL DEFAULT 0, -- percentile of YouTube view_count within the language
    ADD COLUMN likes_pct real NOT NULL DEFAULT 0; -- percentile of YouTube likes per view within the language

-- Personal signals (channel affinity, category share) read a device's
-- recent views.
CREATE INDEX events_user_at_idx ON events (user_id, at DESC) WHERE type = 'view_end';

-- +goose Down
-- Forward-only: no down migration.
