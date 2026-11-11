-- +goose Up
-- Module V3: rotate publishedAfter windows per source and log every search
-- (quota spent, clips found and kept) so the daily budget can be split
-- across languages.
ALTER TABLE youtube_sources ADD COLUMN run_count integer NOT NULL DEFAULT 0;

CREATE TABLE youtube_search_log (
    id        uuid PRIMARY KEY,
    source_id uuid REFERENCES youtube_sources (id) ON DELETE SET NULL,
    language  text NOT NULL REFERENCES languages (code),
    ran_at    timestamptz NOT NULL DEFAULT now(),
    units     integer NOT NULL,
    found     integer NOT NULL,
    kept      integer NOT NULL
);
CREATE INDEX youtube_search_log_lang_day_idx ON youtube_search_log (language, ran_at);

-- IDs a search found but the filters rejected, so later searches don't spend
-- videos.list quota on them again. Only the ID is kept, for 30 days.
CREATE TABLE youtube_rejects (
    youtube_video_id text PRIMARY KEY,
    reason           text NOT NULL CHECK (reason IN ('filters', 'language')),
    rejected_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- Forward-only: no down migration.
