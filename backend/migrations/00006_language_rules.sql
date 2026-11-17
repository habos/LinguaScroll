-- +goose Up
-- Tighter language rules (V3): search results from a blocked channel are
-- remembered as rejects too, so repeat searches don't pay videos.list for
-- them.
ALTER TABLE youtube_rejects DROP CONSTRAINT youtube_rejects_reason_check;
ALTER TABLE youtube_rejects ADD CONSTRAINT youtube_rejects_reason_check CHECK (reason IN ('filters', 'language', 'channel'));

-- +goose Down
-- Forward-only: no down migration.
