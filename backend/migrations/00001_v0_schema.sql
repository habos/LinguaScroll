-- +goose Up
-- v0 schema (YouTube-only build). Column names follow the full schema (F3)
-- so v1 adds tables and columns without renaming anything.

CREATE TABLE languages (
    code             text PRIMARY KEY,
    english_name     text NOT NULL,
    native_name      text NOT NULL,
    script           text NOT NULL,
    direction        text NOT NULL CHECK (direction IN ('ltr', 'rtl')),
    segmenter        text NOT NULL DEFAULT 'space',
    is_learnable     boolean NOT NULL DEFAULT false,
    is_native_option boolean NOT NULL DEFAULT true,
    is_ui            boolean NOT NULL DEFAULT false,
    launch_order     integer
);

-- One row per install in v0 (kind = 'anonymous'). Only the age band is
-- stored, never the birth year.
CREATE TABLE users (
    id              uuid PRIMARY KEY,
    kind            text NOT NULL DEFAULT 'anonymous' CHECK (kind IN ('anonymous', 'account')),
    install_id      text UNIQUE,
    age_band        text NOT NULL CHECK (age_band IN ('teen', 'adult')),
    ui_language     text NOT NULL REFERENCES languages (code),
    native_language text NOT NULL REFERENCES languages (code),
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token_hash   bytea PRIMARY KEY, -- SHA-256 of the bearer token
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

CREATE TABLE learner_languages (
    user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    language  text NOT NULL REFERENCES languages (code),
    level     integer NOT NULL CHECK (level BETWEEN 1 AND 6),
    is_active boolean NOT NULL DEFAULT false,
    PRIMARY KEY (user_id, language)
);
-- At most one active learning language per user.
CREATE UNIQUE INDEX learner_languages_one_active ON learner_languages (user_id) WHERE is_active;

CREATE TABLE videos (
    id                    uuid PRIMARY KEY,
    source                text NOT NULL CHECK (source IN ('youtube', 'native')),
    youtube_video_id      text UNIQUE,
    youtube_channel_id    text,
    youtube_channel_title text,
    youtube_title         text,
    youtube_description   text,
    youtube_category_id   text,
    youtube_tags          text[] NOT NULL DEFAULT '{}',
    language              text NOT NULL REFERENCES languages (code),
    language_source       text NOT NULL CHECK (language_source IN ('audio_tag', 'seed_channel', 'text_detect', 'manual')),
    language_confidence   real,
    level                 integer CHECK (level BETWEEN 1 AND 6),
    level_source          text CHECK (level_source IN ('hint', 'keyword', 'engagement', 'manual')),
    duration_ms           integer NOT NULL CHECK (duration_ms > 0),
    aspect                real NOT NULL CHECK (aspect > 0), -- width / height
    view_count            bigint NOT NULL DEFAULT 0,
    like_count            bigint NOT NULL DEFAULT 0,
    has_captions          boolean NOT NULL DEFAULT false,
    status                text NOT NULL DEFAULT 'live' CHECK (status IN ('live', 'removed')),
    hold_reason           text,
    published_at          timestamptz,
    refreshed_at          timestamptz NOT NULL DEFAULT now(),
    created_at            timestamptz NOT NULL DEFAULT now(),
    CHECK (source <> 'youtube' OR youtube_video_id IS NOT NULL)
);
CREATE INDEX videos_feed_idx ON videos (language, status, level) WHERE status = 'live';
CREATE INDEX videos_refresh_idx ON videos (refreshed_at) WHERE source = 'youtube';
CREATE INDEX videos_channel_idx ON videos (youtube_channel_id);

-- Per language: search terms and seed channels, each with an optional level hint.
CREATE TABLE youtube_sources (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    language    text NOT NULL REFERENCES languages (code),
    kind        text NOT NULL CHECK (kind IN ('query', 'channel')),
    value       text NOT NULL,
    level_hint  integer CHECK (level_hint BETWEEN 1 AND 6),
    enabled     boolean NOT NULL DEFAULT true,
    last_run_at timestamptz,
    UNIQUE (language, kind, value)
);

CREATE TABLE likes (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    video_id   uuid NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, video_id)
);
CREATE INDEX likes_user_created_idx ON likes (user_id, created_at DESC);

CREATE TABLE saves (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    video_id   uuid NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, video_id)
);
CREATE INDEX saves_user_created_idx ON saves (user_id, created_at DESC);

CREATE TABLE hides (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    video_id   uuid NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, video_id)
);

CREATE TABLE channel_blocks (
    user_id            uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    youtube_channel_id text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, youtube_channel_id)
);

CREATE TABLE reports (
    id               uuid PRIMARY KEY,
    reporter_user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_type      text NOT NULL DEFAULT 'video' CHECK (target_type IN ('video')),
    target_id        uuid NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    reason           text NOT NULL CHECK (reason IN ('not_this_language', 'inappropriate', 'spam', 'other')),
    status           text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'actioned', 'dismissed')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (reporter_user_id, target_id)
);

-- Raw client events, partitioned by month. A default partition catches
-- anything outside the monthly partitions the worker creates ahead.
CREATE TABLE events (
    id       uuid NOT NULL,
    user_id  uuid REFERENCES users (id) ON DELETE SET NULL,
    video_id uuid,
    type     text NOT NULL CHECK (type IN ('impression', 'view_end', 'share', 'player_error', 'like', 'unlike', 'save', 'unsave', 'hide', 'channel_block', 'channel_unblock', 'report')),
    at       timestamptz NOT NULL,
    props    jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY (id, at)
) PARTITION BY RANGE (at);
CREATE TABLE events_default PARTITION OF events DEFAULT;
CREATE INDEX events_video_at_idx ON events (video_id, at);

-- +goose StatementBegin
-- ensure_event_partitions creates monthly partitions from this month to
-- months_ahead months ahead. Idempotent; the worker calls it daily. Rows
-- that already landed in events_default for a month are moved into the new
-- partition, so a stray future-dated event can't block the job.
CREATE FUNCTION ensure_event_partitions(months_ahead integer) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
    m date := date_trunc('month', now())::date;
    name text;
BEGIN
    FOR i IN 0..months_ahead LOOP
        name := format('events_%s', to_char(m, 'YYYY_MM'));
        IF to_regclass(name) IS NULL THEN
            EXECUTE format('CREATE TABLE %I (LIKE events INCLUDING DEFAULTS INCLUDING CONSTRAINTS)', name);
            EXECUTE format('WITH moved AS (DELETE FROM events_default WHERE at >= %L AND at < %L RETURNING *)
                            INSERT INTO %I SELECT * FROM moved', m, (m + interval '1 month')::date, name);
            EXECUTE format('ALTER TABLE events ATTACH PARTITION %I FOR VALUES FROM (%L) TO (%L)',
                           name, m, (m + interval '1 month')::date);
        END IF;
        m := (m + interval '1 month')::date;
    END LOOP;
END;
$$;
-- +goose StatementEnd

CREATE TABLE feed_seen (
    user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    video_id  uuid NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    served_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, video_id)
);

CREATE TABLE video_stats (
    video_id        uuid PRIMARY KEY REFERENCES videos (id) ON DELETE CASCADE,
    impressions     integer NOT NULL DEFAULT 0,
    views           integer NOT NULL DEFAULT 0,
    completions     integer NOT NULL DEFAULT 0,
    avg_watch_ratio real NOT NULL DEFAULT 0,
    likes           integer NOT NULL DEFAULT 0,
    hides           integer NOT NULL DEFAULT 0,
    base_score      real NOT NULL DEFAULT 0,
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- The same counts split by viewer level, for level refinement.
CREATE TABLE video_level_stats (
    video_id     uuid NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    viewer_level integer NOT NULL CHECK (viewer_level BETWEEN 1 AND 6),
    views        integer NOT NULL DEFAULT 0,
    completions  integer NOT NULL DEFAULT 0,
    likes        integer NOT NULL DEFAULT 0,
    hides        integer NOT NULL DEFAULT 0,
    PRIMARY KEY (video_id, viewer_level)
);

-- YouTube Data API units spent per UTC day.
CREATE TABLE youtube_quota (
    day   date PRIMARY KEY,
    units integer NOT NULL DEFAULT 0
);

CREATE TABLE app_config (
    key        text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- Forward-only: no down migration.
