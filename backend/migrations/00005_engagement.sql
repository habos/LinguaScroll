-- +goose Up
-- Module V4 engagement: the stats rollup reads events by arrival time
-- (clients may send events up to 7 days late), un-hide is logged, and a
-- "not this language" report flags the clip for a language re-check.
ALTER TABLE events ADD COLUMN received_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX events_received_at_idx ON events (received_at);

ALTER TABLE events DROP CONSTRAINT events_type_check;
ALTER TABLE events ADD CONSTRAINT events_type_check CHECK (type IN (
    'impression', 'view_end', 'share', 'player_error', 'like', 'unlike', 'save', 'unsave',
    'hide', 'unhide', 'channel_block', 'channel_unblock', 'report'));

ALTER TABLE videos ADD COLUMN language_recheck boolean NOT NULL DEFAULT false;

CREATE INDEX reports_target_open_idx ON reports (target_id) WHERE status = 'open';
CREATE INDEX channel_blocks_user_created_idx ON channel_blocks (user_id, created_at DESC);

-- How far each rollup has read, by events.received_at.
CREATE TABLE rollup_state (
    name       text PRIMARY KEY,
    done_until timestamptz NOT NULL
);
INSERT INTO rollup_state (name, done_until) VALUES ('events', '-infinity');

-- +goose Down
-- Forward-only: no down migration.
