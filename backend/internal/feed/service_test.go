package feed_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/habos/linguascroll/backend/internal/feed"
	"github.com/habos/linguascroll/backend/internal/platform/dbtest"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The seed has no German clips, so each test builds its own catalog.
const lang = "de"

type clipSpec struct {
	channel  string
	category string
	level    *int
	ytViews  int64
	ytLikes  int64
	views    int // in-app
	done     int // in-app completions
	ageDays  int
}

type fixture struct {
	t    *testing.T
	ctx  context.Context
	db   *pgxpool.Pool
	user uuid.UUID
	svc  feed.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, ctx: context.Background(), db: dbtest.New(t, true)}
	f.user = f.newUser(nil)
	seed := uint64(1)
	f.svc = feed.Service{DB: f.db, Rand: func() *rand.Rand { seed++; return rand.New(rand.NewPCG(seed, seed)) }}
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(f.ctx, sql, args...); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

func (f *fixture) newUser(level *int) uuid.UUID {
	f.t.Helper()
	id := uuid.Must(uuid.NewV7())
	f.exec(`INSERT INTO users (id, install_id, age_band, ui_language, native_language) VALUES ($1, $2, 'adult', 'en', 'en')`, id, id.String())
	if level != nil {
		f.exec(`INSERT INTO learner_languages (user_id, language, level, is_active) VALUES ($1, $2, $3, true)`, id, lang, *level)
	}
	return id
}

func (f *fixture) addClip(c clipSpec) uuid.UUID {
	f.t.Helper()
	id := uuid.Must(uuid.NewV7())
	if c.category == "" {
		c.category = "27"
	}
	f.exec(`INSERT INTO videos (id, source, youtube_video_id, youtube_channel_id, youtube_channel_title, youtube_title,
	           youtube_category_id, language, language_source, level, duration_ms, aspect, view_count, like_count, published_at)
	        VALUES ($1, 'youtube', $2, $3, $3, 'clip', $4, $5, 'manual', $6, 20000, 0.5625, $7, $8, now() - make_interval(days => $9))`,
		id, "yt"+id.String()[:20], c.channel, c.category, lang, c.level, c.ytViews, c.ytLikes, c.ageDays)
	f.exec(`INSERT INTO video_stats (video_id, impressions, views, completions) VALUES ($1, $2, $2, $3)`, id, c.views, c.done)
	return id
}

// catalog adds n clips over `channels` channels with rising YouTube views,
// all past the explore threshold.
func (f *fixture) catalog(n, channels int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = f.addClip(clipSpec{
			channel: fmt.Sprintf("UC%03d", i%channels), ytViews: int64(1000 * (i + 1)), ytLikes: int64(10 * (i + 1)),
			views: 100, done: 50, ageDays: 5,
		})
	}
	f.refresh()
	return ids
}

func (f *fixture) refresh() {
	f.t.Helper()
	if _, err := feed.RefreshStats(f.ctx, f.db, time.Now()); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) page(user uuid.UUID, cur feed.Cursor) feed.Page {
	f.t.Helper()
	p, err := f.svc.Next(f.ctx, user, lang, cur)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) baseScores() map[uuid.UUID]float64 {
	rows, err := f.db.Query(f.ctx, `SELECT video_id, base_score FROM video_stats`)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[uuid.UUID]float64{}
	for rows.Next() {
		var id uuid.UUID
		var s float32
		_ = rows.Scan(&id, &s)
		out[id] = float64(s)
	}
	return out
}

func TestFreshDeviceGetsQualityOrderedPageWithExplore(t *testing.T) {
	f := newFixture(t)
	f.catalog(60, 30)
	explore := map[uuid.UUID]bool{}
	for i := range 6 {
		explore[f.addClip(clipSpec{channel: fmt.Sprintf("UCnew%d", i), ytViews: 10, views: 3, done: 1})] = true
	}
	f.refresh()
	base := f.baseScores()

	sawOne, sawTwo := false, false
	for range 20 {
		user := f.newUser(nil)
		p := f.page(user, feed.Cursor{})
		if len(p.Items) != 10 {
			t.Fatalf("page has %d items", len(p.Items))
		}
		n := 0
		var main []float64
		for _, it := range p.Items {
			if explore[it.ID] {
				n++
			} else {
				main = append(main, base[it.ID])
			}
		}
		switch n {
		case 1:
			sawOne = true
		case 2:
			sawTwo = true
		default:
			t.Fatalf("explore clips = %d, want 1 or 2", n)
		}
		// Quality order: the non-explore items follow base score, give or
		// take the 0.05 noise.
		for i := 1; i < len(main); i++ {
			if main[i] > main[i-1]+0.05+1e-6 {
				t.Fatalf("item %d base %.3f after %.3f: not quality ordered", i, main[i], main[i-1])
			}
		}
	}
	if !sawOne || !sawTwo {
		t.Errorf("over 20 fresh devices want both 1 and 2 explore clips (one=%v two=%v)", sawOne, sawTwo)
	}
}

func TestLikesFromChannelRaiseIt(t *testing.T) {
	f := newFixture(t)
	ids := f.catalog(60, 30)
	// UC000 owns ids 0, 30: the lowest-quality clips in the catalog.
	position := func(user uuid.UUID) int {
		p := f.page(user, feed.Cursor{})
		for i, it := range p.Items {
			if it.ChannelID == "UC000" {
				return i
			}
		}
		return -1
	}
	if pos := position(f.newUser(nil)); pos != -1 {
		t.Fatalf("UC000 shown to a fresh device at %d; test assumes it ranks low", pos)
	}
	fan := f.newUser(nil)
	f.exec(`INSERT INTO likes (user_id, video_id) VALUES ($1, $2)`, fan, ids[0])
	if pos := position(fan); pos == -1 || pos > 2 {
		t.Fatalf("after a like, UC000 position = %d, want in the top 3", pos)
	}
}

func TestNoRepeatsAndChannelSpacingOver20Pages(t *testing.T) {
	f := newFixture(t)
	f.catalog(500, 40)
	seen := map[uuid.UUID]bool{}
	var channels []string
	cur := feed.Cursor{}
	for pg := range 20 {
		p := f.page(f.user, cur)
		if len(p.Items) != 10 || p.Next == nil {
			t.Fatalf("page %d: %d items, next %v", pg, len(p.Items), p.Next)
		}
		for _, it := range p.Items {
			if seen[it.ID] {
				t.Fatalf("page %d repeats %s", pg, it.ID)
			}
			seen[it.ID] = true
			channels = append(channels, it.ChannelID)
		}
		// Round-trip the cursor like a client would.
		var err error
		if cur, err = feed.DecodeCursor(p.Next.Encode()); err != nil {
			t.Fatal(err)
		}
	}
	for i := range channels {
		for j := i + 1; j < len(channels) && j < i+5; j++ {
			if channels[i] == channels[j] {
				t.Fatalf("channel %s at items %d and %d", channels[i], i, j)
			}
		}
	}
}

func TestHiddenBlockedRemovedAndOffLevelNeverReturn(t *testing.T) {
	f := newFixture(t)
	user := f.newUser(ptrTo(2))
	hidden := f.addClip(clipSpec{channel: "UChide", ytViews: 1e9, views: 100, done: 100})
	blocked := f.addClip(clipSpec{channel: "UCblock", ytViews: 1e9, views: 100, done: 100})
	removed := f.addClip(clipSpec{channel: "UCgone", ytViews: 1e9, views: 100, done: 100})
	tooHard := f.addClip(clipSpec{channel: "UChard", level: ptrTo(4), ytViews: 1e9, views: 100, done: 100})
	oneUp := f.addClip(clipSpec{channel: "UCok", level: ptrTo(3), ytViews: 1e9, views: 100, done: 100})
	f.exec(`UPDATE videos SET status = 'removed' WHERE id = $1`, removed)
	f.exec(`INSERT INTO hides (user_id, video_id) VALUES ($1, $2)`, user, hidden)
	f.exec(`INSERT INTO channel_blocks (user_id, youtube_channel_id) VALUES ($1, 'UCblock')`, user)
	f.catalog(40, 20)

	found := map[uuid.UUID]bool{}
	cur := feed.Cursor{}
	for range 5 {
		p := f.page(user, cur)
		for _, it := range p.Items {
			found[it.ID] = true
		}
		if p.Next == nil {
			break
		}
		cur = *p.Next
	}
	for name, id := range map[string]uuid.UUID{"hidden": hidden, "blocked": blocked, "removed": removed, "level 4 for a level 2": tooHard} {
		if found[id] {
			t.Errorf("%s clip was served", name)
		}
	}
	if !found[oneUp] {
		t.Error("clip one level up was never served")
	}
}

func TestSeenWindowFallback(t *testing.T) {
	f := newFixture(t)
	f.catalog(25, 25)
	user := f.newUser(nil)
	total := 0
	cur := feed.Cursor{}
	for range 3 {
		p := f.page(user, cur)
		total += len(p.Items)
		if p.Next != nil {
			cur = *p.Next
		}
	}
	if total != 25 {
		t.Fatalf("served %d of 25 clips over 3 pages", total)
	}
	// Everything served 2 days ago: nothing to show.
	f.exec(`UPDATE feed_seen SET served_at = now() - interval '2 days' WHERE user_id = $1`, user)
	if p := f.page(user, cur); len(p.Items) != 0 || p.Next != nil {
		t.Fatalf("2 days later: %d items, next %v; want empty page, nil cursor", len(p.Items), p.Next)
	}
	// Served 8 days ago: allowed again because fewer than a page are fresh.
	f.exec(`UPDATE feed_seen SET served_at = now() - interval '8 days' WHERE user_id = $1`, user)
	if p := f.page(user, cur); len(p.Items) != 10 {
		t.Fatalf("8 days later: %d items, want 10", len(p.Items))
	}
}

func TestViewerStateAndUnknownLanguage(t *testing.T) {
	f := newFixture(t)
	ids := f.catalog(10, 10)
	for _, id := range ids {
		f.exec(`INSERT INTO saves (user_id, video_id) VALUES ($1, $2)`, f.user, id)
	}
	p := f.page(f.user, feed.Cursor{})
	for _, it := range p.Items {
		if !it.Saved || it.Liked {
			t.Fatalf("viewer state = liked %v saved %v, want saved only", it.Liked, it.Saved)
		}
	}
	f.exec(`UPDATE languages SET is_learnable = false WHERE code = 'it'`)
	for _, code := range []string{"xx", "it"} {
		if _, err := f.svc.Next(f.ctx, f.user, code, feed.Cursor{}); !errors.Is(err, feed.ErrUnknownLanguage) {
			t.Errorf("language %q: err = %v, want ErrUnknownLanguage", code, err)
		}
	}
}

func TestRefreshStatsMatchesGoFormula(t *testing.T) {
	f := newFixture(t)
	low := f.addClip(clipSpec{channel: "a", ytViews: 10, ytLikes: 0, views: 0, done: 0, ageDays: 200})
	mid := f.addClip(clipSpec{channel: "b", ytViews: 100, ytLikes: 10, views: 10, done: 8, ageDays: 10})
	high := f.addClip(clipSpec{channel: "c", ytViews: 1000, ytLikes: 50, views: 20, done: 2, ageDays: 400})
	f.exec(`UPDATE video_stats SET hides = 5 WHERE video_id = $1`, high)
	f.refresh()

	w := feed.DefaultConfig().Weights
	now := time.Now()
	want := map[uuid.UUID]struct{ vp, lp, base float64 }{
		low:  {0, 0, feed.BaseScore(w, feed.Signals{Completion: 0.5, Fresh: feed.Freshness(ptrTo(now.AddDate(0, 0, -200)), now)})},
		mid:  {0.5, 1, feed.BaseScore(w, feed.Signals{Completion: 10.0 / 14, ViewsPct: 0.5, LikesPct: 1, Fresh: 1})},
		high: {1, 0.5, feed.BaseScore(w, feed.Signals{Completion: 4.0 / 24, ViewsPct: 1, LikesPct: 0.5, Hide: 5.0 / 25})},
	}
	for id, w := range want {
		var vp, lp, base float32
		if err := f.db.QueryRow(f.ctx, `SELECT views_pct, likes_pct, base_score FROM video_stats WHERE video_id = $1`, id).Scan(&vp, &lp, &base); err != nil {
			t.Fatal(err)
		}
		if math.Abs(float64(vp)-w.vp) > 1e-6 || math.Abs(float64(lp)-w.lp) > 1e-6 || math.Abs(float64(base)-w.base) > 1e-4 {
			t.Errorf("%s: got pct %v/%v base %v, want %v/%v base %v", id, vp, lp, base, w.vp, w.lp, w.base)
		}
	}
	// Clips with no video_stats row get one.
	f.exec(`DELETE FROM video_stats WHERE video_id = $1`, low)
	f.refresh()
	var n int
	_ = f.db.QueryRow(f.ctx, `SELECT count(*) FROM video_stats WHERE video_id = $1`, low).Scan(&n)
	if n != 1 {
		t.Fatal("RefreshStats did not create a missing video_stats row")
	}
}

func TestCategoryAndCompletedChannelSignals(t *testing.T) {
	f := newFixture(t)
	// 20 clips of equal quality; one channel posts in category 22.
	var target uuid.UUID
	for i := range 20 {
		c := clipSpec{channel: fmt.Sprintf("UC%03d", i), ytViews: 5000, ytLikes: 50, views: 100, done: 50}
		if i == 7 {
			c.category = "22"
			target = f.addClip(c)
			continue
		}
		f.addClip(c)
	}
	f.refresh()
	// 3 completed views of that clip in the last 30 days give the channel
	// affinity 0.5 and category 22 a share of 1.
	for range 3 {
		f.exec(`INSERT INTO events (id, user_id, video_id, type, at, props)
		        VALUES ($1, $2, $3, 'view_end', now() - interval '1 day', '{"watched_ms": 15000, "completed": true}')`,
			uuid.New(), f.user, target)
	}
	// A short view (< 2 s) and malformed props are ignored, not errors.
	f.exec(`INSERT INTO events (id, user_id, video_id, type, at, props) VALUES ($1, $2, $3, 'view_end', now(), '{"watched_ms": 500}')`, uuid.New(), f.user, target)
	f.exec(`INSERT INTO events (id, user_id, video_id, type, at, props) VALUES ($1, $2, $3, 'view_end', now(), '{"watched_ms": "x"}')`, uuid.New(), f.user, target)

	if p := f.page(f.user, feed.Cursor{}); p.Items[0].ID != target {
		t.Fatalf("first item %s (%s), want the clip from the completed channel and category", p.Items[0].ID, p.Items[0].ChannelID)
	}
	// A fresh device has no such preference.
	other := f.newUser(nil)
	hits := 0
	for range 10 {
		f.exec(`DELETE FROM feed_seen`)
		if f.page(other, feed.Cursor{}).Items[0].ID == target {
			hits++
		}
	}
	if hits > 5 {
		t.Fatalf("fresh device got the clip first %d/10 times", hits)
	}
}

func ptrTo[T any](v T) *T { return &v }

// TestLatencyAt20kClips checks the p95 budget (200 ms at 20,000 live clips
// per language). It takes ~20 s, so it only runs with FEED_LOAD_TEST=1.
func TestLatencyAt20kClips(t *testing.T) {
	if os.Getenv("FEED_LOAD_TEST") == "" {
		t.Skip("set FEED_LOAD_TEST=1 to run")
	}
	f := newFixture(t)
	f.exec(`INSERT INTO videos (id, source, youtube_video_id, youtube_channel_id, youtube_channel_title, youtube_title,
	           youtube_category_id, language, language_source, level, duration_ms, aspect, view_count, like_count, published_at)
	        SELECT gen_random_uuid(), 'youtube', 'yt' || g, 'UC' || (g % 2000), 'ch', 'clip',
	               CASE WHEN g % 7 = 0 THEN '10' ELSE '27' END, $1, 'manual',
	               CASE WHEN g % 4 = 0 THEN NULL ELSE 1 + g % 6 END, 30000, 0.5625,
	               (random() * 1e6)::bigint, (random() * 1e4)::bigint, now() - make_interval(days => g % 400)
	          FROM generate_series(1, 20000) g`, lang)
	f.exec(`INSERT INTO video_stats (video_id, impressions, views, completions, hides)
	        SELECT id, n, n, (n * random())::int, (n * random() * 0.05)::int
	          FROM (SELECT id, (random() * 500)::int AS n FROM videos WHERE language = $1) x`, lang)
	f.refresh()
	f.exec(`ANALYZE`)
	user := f.newUser(ptrTo(3))
	// Some history: likes, 100 views, a few hides and blocks.
	f.exec(`INSERT INTO likes (user_id, video_id) SELECT $1, id FROM videos WHERE language = $2 LIMIT 30`, user, lang)
	f.exec(`INSERT INTO hides (user_id, video_id) SELECT $1, id FROM videos WHERE language = $2 OFFSET 100 LIMIT 30`, user, lang)
	f.exec(`INSERT INTO events (id, user_id, video_id, type, at, props)
	        SELECT gen_random_uuid(), $1, id, 'view_end', now() - make_interval(mins => row_number() OVER ()::int),
	               '{"watched_ms": 20000, "completed": true}'
	          FROM videos WHERE language = $2 OFFSET 200 LIMIT 300`, user, lang)

	var times []time.Duration
	cur := feed.Cursor{}
	for range 60 {
		start := time.Now()
		p := f.page(user, cur)
		times = append(times, time.Since(start))
		if len(p.Items) != 10 {
			t.Fatalf("page has %d items", len(p.Items))
		}
		cur = *p.Next
	}
	slices.Sort(times)
	p50, p95 := times[len(times)/2], times[len(times)*95/100]
	t.Logf("GET feed at 20k clips: p50 %v, p95 %v", p50, p95)
	if p95 > 200*time.Millisecond {
		t.Fatalf("p95 %v over the 200 ms budget", p95)
	}
}
