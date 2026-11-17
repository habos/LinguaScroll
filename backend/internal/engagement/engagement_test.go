package engagement_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/habos/linguascroll/backend/internal/engagement"
	"github.com/habos/linguascroll/backend/internal/platform/dbtest"
	"github.com/habos/linguascroll/backend/internal/platform/httpx"
)

func setup(t *testing.T) (*engagement.Service, *pgxpool.Pool, []uuid.UUID) {
	t.Helper()
	pool := dbtest.New(t, true)
	clips := dbtest.InsertClips(t, pool, "es", 5, 2, nil)
	return &engagement.Service{DB: pool}, pool, clips
}

// newUser makes a device learning lang at level.
func newUser(t *testing.T, pool *pgxpool.Pool, lang string, level int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, install_id, age_band, ui_language, native_language) VALUES ($1, $2, 'adult', 'en', 'en')`, id, id.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO learner_languages (user_id, language, level, is_active) VALUES ($1, $2, $3, true)`, id, lang, level); err != nil {
		t.Fatal(err)
	}
	return id
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func status(err error) int {
	if e, ok := err.(*httpx.APIError); ok {
		return e.Status
	}
	if err != nil {
		return 500
	}
	return 204
}

// V4-10: PUT and DELETE are idempotent; each state change logs one event.
func TestTogglesAreIdempotent(t *testing.T) {
	s, pool, clips := setup(t)
	ctx := context.Background()
	u := newUser(t, pool, "es", 2)
	v := clips[0]
	steps := []struct {
		name string
		f    func(context.Context, uuid.UUID, uuid.UUID) error
	}{
		{"like", s.Like}, {"like", s.Like}, {"unlike", s.Unlike}, {"unlike", s.Unlike},
		{"save", s.Save}, {"save", s.Save}, {"unsave", s.Unsave}, {"unsave", s.Unsave},
		{"hide", s.Hide}, {"hide", s.Hide}, {"unhide", s.Unhide}, {"unhide", s.Unhide},
	}
	for _, st := range steps {
		if err := st.f(ctx, u, v); err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
	}
	for _, typ := range []string{"like", "unlike", "save", "unsave", "hide", "unhide"} {
		if n := count(t, pool, `SELECT count(*) FROM events WHERE user_id = $1 AND type = $2`, u, typ); n != 1 {
			t.Errorf("%s events = %d, want 1", typ, n)
		}
	}
	if n := count(t, pool, `SELECT (SELECT count(*) FROM likes) + (SELECT count(*) FROM saves) + (SELECT count(*) FROM hides)`); n != 0 {
		t.Errorf("%d rows left", n)
	}
	for _, ch := range []string{"UCa", "UCa"} {
		if err := s.BlockChannel(ctx, u, ch); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := s.UnblockChannel(ctx, u, "UCa"); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM events WHERE user_id = $1 AND type IN ('channel_block', 'channel_unblock')`, u); n != 2 {
		t.Errorf("block events = %d, want 2", n)
	}
	// Adding state needs a live clip; removing never fails.
	if got := status(s.Like(ctx, u, uuid.Must(uuid.NewV7()))); got != 404 {
		t.Errorf("like unknown = %d", got)
	}
	if _, err := pool.Exec(ctx, `UPDATE videos SET status = 'removed' WHERE id = $1`, clips[1]); err != nil {
		t.Fatal(err)
	}
	if got := status(s.Save(ctx, u, clips[1])); got != 404 {
		t.Errorf("save removed = %d", got)
	}
	if got := status(s.BlockChannel(ctx, u, "")); got != 400 {
		t.Errorf("empty channel = %d", got)
	}
}

func TestChannelBlocksListNewestFirstWithTitles(t *testing.T) {
	s, pool, _ := setup(t)
	ctx := context.Background()
	u := newUser(t, pool, "es", 2)
	now := time.Now()
	for i, ch := range []string{"UCtest_es_0", "UCgone"} {
		s.Now = func() time.Time { return now.Add(time.Duration(i) * time.Minute) }
		if err := s.BlockChannel(ctx, u, ch); err != nil {
			t.Fatal(err)
		}
	}
	bs, err := s.ChannelBlocks(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 || bs[0].ChannelID != "UCgone" || bs[0].Title != nil || bs[1].ChannelID != "UCtest_es_0" || bs[1].Title == nil || *bs[1].Title == "" {
		t.Fatalf("blocks = %+v", bs)
	}
	if other, _ := s.ChannelBlocks(ctx, newUser(t, pool, "es", 1)); len(other) != 0 {
		t.Errorf("another device sees %d blocks", len(other))
	}
}

// V4-05 and V4-13.
func TestThreeReportsRemoveClipAndRepeatsCountOnce(t *testing.T) {
	s, pool, clips := setup(t)
	ctx := context.Background()
	v := clips[0]
	a, b, c := newUser(t, pool, "es", 2), newUser(t, pool, "es", 2), newUser(t, pool, "es", 2)
	for _, r := range []struct {
		u      uuid.UUID
		reason string
	}{{a, "spam"}, {a, "spam"}, {a, "other"}, {b, "not_this_language"}} {
		if err := s.Report(ctx, r.u, v, r.reason); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE id = $1 AND status = 'live' AND language_recheck`, v); n != 1 {
		t.Fatal("after 2 devices the clip should be live and flagged for a language re-check")
	}
	if err := s.Report(ctx, c, v, "inappropriate"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE id = $1 AND status = 'removed' AND hold_reason = 'reports'
		AND youtube_title IS NULL AND youtube_channel_title IS NULL AND youtube_description IS NULL AND youtube_tags = '{}'
		AND youtube_category_id IS NULL AND published_at IS NULL AND view_count = 0 AND like_count = 0`, v); n != 1 {
		t.Fatal("clip not removed with its YouTube data cleared")
	}
	if n := count(t, pool, `SELECT count(*) FROM reports WHERE target_id = $1 AND status = 'actioned'`, v); n != 3 {
		t.Errorf("actioned reports = %d, want 3", n)
	}
	if got := status(s.Report(ctx, a, v, "rude")); got != 422 {
		t.Errorf("bad reason = %d", got)
	}
	if got := status(s.Report(ctx, a, uuid.Must(uuid.NewV7()), "spam")); got != 404 {
		t.Errorf("unknown clip = %d", got)
	}
}

func viewEnd(v uuid.UUID, at time.Time, watched float64, completed bool) engagement.Event {
	return engagement.Event{ID: uuid.Must(uuid.NewV7()), Type: "view_end", At: at, VideoID: &v,
		Props: map[string]any{"watched_ms": watched, "completed": completed}}
}

// V4-08 and V4-11.
func TestIngestDedupesAndDropsBadEvents(t *testing.T) {
	s, pool, clips := setup(t)
	ctx := context.Background()
	u := newUser(t, pool, "es", 2)
	now := time.Now()
	v := clips[0]
	good := viewEnd(v, now, 5000, true)
	imp := engagement.Event{ID: uuid.Must(uuid.NewV7()), Type: "impression", At: now, VideoID: &v, Props: map[string]any{"position": 1.0}}
	share := engagement.Event{ID: uuid.Must(uuid.NewV7()), Type: "share", At: now}
	bad := []engagement.Event{
		viewEnd(v, now.Add(-8*24*time.Hour), 5000, true),    // too old
		viewEnd(v, now.Add(2*time.Hour), 5000, true),        // too far ahead
		viewEnd(v, now.Add(100*365*24*time.Hour), 1, false), // year 2126
		{ID: uuid.Must(uuid.NewV7()), Type: "view_end", At: now, VideoID: &v, Props: map[string]any{"watched_ms": "5000", "completed": true}},
		{ID: uuid.Must(uuid.NewV7()), Type: "view_end", At: now, VideoID: &v, Props: map[string]any{"watched_ms": 5000.0, "completed": "true"}},
		{ID: uuid.Must(uuid.NewV7()), Type: "impression", At: now},
		{ID: uuid.Must(uuid.NewV7()), Type: "like", At: now, VideoID: &v},
	}
	batch := append([]engagement.Event{good, good, imp, share}, bad...)
	if _, err := pool.Exec(ctx, `SELECT ensure_event_partitions(2)`); err != nil { // the worker does this on start
		t.Fatal(err)
	}
	r, err := s.Ingest(ctx, u, batch)
	if err != nil {
		t.Fatal(err)
	}
	if r.Accepted != 3 || r.Duplicates != 1 || r.Dropped != len(bad) {
		t.Fatalf("result %+v", r)
	}
	// A retry of the same batch stores nothing new.
	r, err = s.Ingest(ctx, u, batch)
	if err != nil || r.Accepted != 0 || r.Duplicates != 4 {
		t.Fatalf("retry %+v %v", r, err)
	}
	if n := count(t, pool, `SELECT count(*) FROM events WHERE user_id = $1`, u); n != 3 {
		t.Errorf("stored %d events", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM events_default`); n != 0 {
		t.Errorf("%d events landed in the default partition", n)
	}
	too := make([]engagement.Event, engagement.MaxEventsPerBatch+1)
	for i := range too {
		too[i] = viewEnd(v, now, 1, false)
	}
	if _, err := s.Ingest(ctx, u, too); status(err) != 400 {
		t.Errorf("201 events: %v", err)
	}
	if _, err := s.Ingest(ctx, u, nil); status(err) != 400 {
		t.Errorf("empty batch: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT ensure_event_partitions(2)`); err != nil {
		t.Errorf("partitions after ingest: %v", err)
	}
}

func TestRollupCountsOnceAndUpserts(t *testing.T) {
	s, pool, clips := setup(t)
	ctx := context.Background()
	v, w := clips[0], clips[1]
	u3, u4 := newUser(t, pool, "es", 3), newUser(t, pool, "es", 4)
	now := time.Now()
	// The feed_stats job may have created the row already.
	if _, err := pool.Exec(ctx, `INSERT INTO video_stats (video_id, base_score, views_pct) VALUES ($1, 0.7, 0.5)`, v); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ingest(ctx, u3, []engagement.Event{
		viewEnd(v, now, 30_000, true),  // full 30 s clip
		viewEnd(v, now, 1_500, false),  // under 2 s: not a view
		viewEnd(w, now, 15_000, false), // half
		{ID: uuid.Must(uuid.NewV7()), Type: "impression", At: now, VideoID: &v},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ingest(ctx, u4, []engagement.Event{viewEnd(v, now.Add(-6*24*time.Hour), 15_000, false)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Like(ctx, u3, v); err != nil {
		t.Fatal(err)
	}
	if err := s.Hide(ctx, u4, w); err != nil {
		t.Fatal(err)
	}
	// Events must be older than RollupLag to be read.
	age := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE events SET received_at = received_at - $1::interval`, (engagement.RollupLag + time.Second).String()); err != nil {
			t.Fatal(err)
		}
	}
	age()
	r, err := s.Rollup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Events != 7 {
		t.Errorf("rolled up %d events, want 7", r.Events)
	}
	check := func() {
		t.Helper()
		var imp, views, done, likes, hides int
		var ratio float64
		var base float32
		if err := pool.QueryRow(ctx, `SELECT impressions, views, completions, avg_watch_ratio, likes, hides, base_score FROM video_stats WHERE video_id = $1`, v).
			Scan(&imp, &views, &done, &ratio, &likes, &hides, &base); err != nil {
			t.Fatal(err)
		}
		if imp != 1 || views != 2 || done != 1 || likes != 1 || hides != 0 || base != 0.7 || ratio < 0.74 || ratio > 0.76 {
			t.Errorf("v stats: imp %d views %d done %d ratio %.3f likes %d hides %d base %.1f", imp, views, done, ratio, likes, hides, base)
		}
		if n := count(t, pool, `SELECT count(*) FROM video_stats WHERE video_id = $1 AND views = 1 AND hides = 1`, w); n != 1 {
			t.Error("w stats wrong")
		}
		if n := count(t, pool, `SELECT count(*) FROM video_level_stats WHERE video_id = $1 AND
			((viewer_level = 3 AND views = 1 AND completions = 1 AND likes = 1) OR (viewer_level = 4 AND views = 1 AND completions = 0))`, v); n != 2 {
			t.Error("level stats wrong")
		}
	}
	check()
	// Running again adds nothing.
	if r, err := s.Rollup(ctx); err != nil || r.Events != 0 {
		t.Fatalf("second run %+v %v", r, err)
	}
	check()
	// Unliking is recounted on the next run.
	if err := s.Unlike(ctx, u3, v); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE events SET received_at = now() - interval '30 seconds' WHERE type = 'unlike'`); err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return time.Now().Add(time.Minute) }
	if _, err := s.Rollup(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, `SELECT likes FROM video_stats WHERE video_id = $1`, v); n != 0 {
		t.Errorf("likes after unlike = %d", n)
	}
}

// V4-14 for likes and saves, plus paging.
func TestLikesListPagesAndSkipsRemoved(t *testing.T) {
	s, pool, clips := setup(t)
	ctx := context.Background()
	u := newUser(t, pool, "es", 2)
	base := time.Now()
	for i, v := range clips {
		s.Now = func() time.Time { return base.Add(time.Duration(i) * time.Second) }
		if _, err := pool.Exec(ctx, `INSERT INTO likes (user_id, video_id, created_at) VALUES ($1, $2, $3)`, u, v, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Save(ctx, u, clips[4]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE videos SET status = 'removed' WHERE id = $1`, clips[3]); err != nil {
		t.Fatal(err)
	}
	var got []uuid.UUID
	var cur *engagement.ListCursor
	for page := 0; ; page++ {
		cards, next, err := s.Likes(ctx, u, cur, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cards {
			got = append(got, c.ID)
			if !c.Liked || c.Saved != (c.ID == clips[4]) || c.Title == "" {
				t.Errorf("card %+v", c)
			}
		}
		if next == nil {
			break
		}
		if page > 5 {
			t.Fatal("paging does not end")
		}
		round, err := engagement.DecodeListCursor(next.Encode())
		if err != nil {
			t.Fatal(err)
		}
		cur = round
	}
	want := []uuid.UUID{clips[4], clips[2], clips[1], clips[0]}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("likes order %v, want %v", got, want)
	}
	saves, _, err := s.Saves(ctx, u, nil, 30)
	if err != nil || len(saves) != 1 {
		t.Errorf("saves %v %v", saves, err)
	}
}

func TestCleanupSessions(t *testing.T) {
	s, pool, _ := setup(t)
	ctx := context.Background()
	u := newUser(t, pool, "es", 2)
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ('a', $1, now() - interval '1 minute'), ('b', $1, now() + interval '1 day')`, u); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CleanupSessions(ctx); err != nil || n != 1 {
		t.Fatalf("deleted %d, %v", n, err)
	}
}
