-- +goose Up
-- ensure_event_partitions now moves rows that already landed in
-- events_default for a month into that month's new partition, instead of
-- failing for good ("updated partition constraint for default partition
-- would be violated"). A future-dated event from a phone with a wrong clock
-- could otherwise block the daily partition job.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_event_partitions(months_ahead integer) RETURNS void
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

-- +goose Down
-- Forward-only: no down migration.
