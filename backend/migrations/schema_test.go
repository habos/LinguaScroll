package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/habos/linguascroll/backend/internal/platform/db"
	"github.com/habos/linguascroll/backend/internal/platform/dbtest"
	"github.com/habos/linguascroll/backend/seed"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrateAndSeed(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t, true)

	// Running both again is a no-op.
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if err := seed.Apply(ctx, pool); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	for _, c := range []struct {
		q    string
		want int
	}{
		{`SELECT count(*) FROM languages WHERE is_learnable`, 15},
		{`SELECT count(*) FROM videos WHERE status = 'live'`, 30},
		{`SELECT count(DISTINCT language) FROM videos`, 3},
		{`SELECT count(*) FROM app_config WHERE key = 'feed.weights'`, 1},
		{`SELECT count(*) FROM languages WHERE direction = 'rtl'`, 1},
	} {
		var got int
		if err := pool.QueryRow(ctx, c.q).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.q, err)
		}
		if got != c.want {
			t.Errorf("%s = %d, want %d", c.q, got, c.want)
		}
	}
}

func newUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, install_id, age_band, ui_language, native_language) VALUES ($1, $2, 'adult', 'en', 'en')`,
		id, id.String())
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func firstVideo(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT id FROM videos ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("video: %v", err)
	}
	return id
}

func TestConstraints(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t, true)
	u := newUser(t, pool)
	v := firstVideo(t, pool)

	mustFail := func(name, q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	mustFail("level 0", `INSERT INTO learner_languages (user_id, language, level) VALUES ($1, 'es', 0)`, u)
	mustFail("level 7", `INSERT INTO learner_languages (user_id, language, level) VALUES ($1, 'es', 7)`, u)
	mustFail("unknown language", `INSERT INTO learner_languages (user_id, language, level) VALUES ($1, 'xx', 1)`, u)

	if _, err := pool.Exec(ctx, `INSERT INTO learner_languages (user_id, language, level, is_active) VALUES ($1, 'es', 1, true)`, u); err != nil {
		t.Fatal(err)
	}
	mustFail("two active languages", `INSERT INTO learner_languages (user_id, language, level, is_active) VALUES ($1, 'fr', 1, true)`, u)

	if _, err := pool.Exec(ctx, `INSERT INTO likes (user_id, video_id) VALUES ($1, $2)`, u, v); err != nil {
		t.Fatal(err)
	}
	mustFail("duplicate like", `INSERT INTO likes (user_id, video_id) VALUES ($1, $2)`, u, v)
	mustFail("youtube clip without youtube id",
		`INSERT INTO videos (id, source, language, language_source, duration_ms, aspect) VALUES ($1, 'youtube', 'es', 'manual', 1000, 0.56)`,
		uuid.Must(uuid.NewV7()))
	mustFail("under-13 age band", `INSERT INTO users (id, age_band, ui_language, native_language) VALUES ($1, 'child', 'en', 'en')`, uuid.Must(uuid.NewV7()))
}

func TestDeleteUserCascades(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t, true)
	u := newUser(t, pool)
	v := firstVideo(t, pool)

	for _, q := range []string{
		`INSERT INTO learner_languages (user_id, language, level, is_active) VALUES ($1, 'es', 1, true)`,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ('\x01', $1, now() + interval '1 day')`,
		`INSERT INTO likes (user_id, video_id) VALUES ($1, $2)`,
		`INSERT INTO saves (user_id, video_id) VALUES ($1, $2)`,
		`INSERT INTO hides (user_id, video_id) VALUES ($1, $2)`,
		`INSERT INTO channel_blocks (user_id, youtube_channel_id) VALUES ($1, 'UCx')`,
		`INSERT INTO reports (id, reporter_user_id, target_id, reason) VALUES (gen_random_uuid(), $1, $2, 'spam')`,
		`INSERT INTO feed_seen (user_id, video_id) VALUES ($1, $2)`,
		`INSERT INTO events (id, user_id, video_id, type, at) VALUES (gen_random_uuid(), $1, $2, 'impression', now())`,
	} {
		args := []any{u}
		if countParams(q) == 2 {
			args = append(args, v)
		}
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, u); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM learner_languages WHERE user_id = $1) +
		(SELECT count(*) FROM sessions WHERE user_id = $1) +
		(SELECT count(*) FROM likes WHERE user_id = $1) +
		(SELECT count(*) FROM saves WHERE user_id = $1) +
		(SELECT count(*) FROM hides WHERE user_id = $1) +
		(SELECT count(*) FROM channel_blocks WHERE user_id = $1) +
		(SELECT count(*) FROM reports WHERE reporter_user_id = $1) +
		(SELECT count(*) FROM feed_seen WHERE user_id = $1)`, u).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d child rows left after deleting the user", left)
	}
	var events, orphaned int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE user_id IS NULL) FROM events`).Scan(&events, &orphaned); err != nil {
		t.Fatal(err)
	}
	if events != 1 || orphaned != 1 {
		t.Errorf("events = %d (user nulled %d), want the event kept with user_id null", events, orphaned)
	}
}

func TestEnsureEventPartitions(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t, false)
	for range 2 { // idempotent
		if _, err := pool.Exec(ctx, `SELECT ensure_event_partitions(2)`); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_inherits WHERE inhparent = 'events'::regclass`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 { // default + this month + 2 ahead
		t.Errorf("partitions = %d, want 4", n)
	}
}

func countParams(q string) int {
	if strings.Contains(q, "$2") {
		return 2
	}
	return 1
}
