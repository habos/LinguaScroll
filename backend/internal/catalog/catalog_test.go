package catalog_test

import (
	"context"
	"testing"
	"time"

	"github.com/habos/linguascroll/backend/internal/appconfig"
	"github.com/habos/linguascroll/backend/internal/catalog"
	"github.com/habos/linguascroll/backend/internal/langdetect"
	"github.com/habos/linguascroll/backend/internal/platform/dbtest"
	"github.com/habos/linguascroll/backend/internal/youtube"
	"github.com/habos/linguascroll/backend/internal/youtube/fake"
	"github.com/jackc/pgx/v5/pgxpool"
)

var spanishTitles = []string{"Hola, ¿cómo estás?", "Qué hacer en Madrid", "Hola amigos"}

func ptr(i int) *int { return &i }

func setup(t *testing.T, sources catalog.SourcesFile) (*catalog.Service, *fake.Client, *pgxpool.Pool, *time.Time) {
	t.Helper()
	pool := dbtest.New(t, true)
	if _, err := pool.Exec(context.Background(), `DELETE FROM videos`); err != nil { // drop the placeholder clips
		t.Fatal(err)
	}
	if _, err := catalog.ApplySources(context.Background(), pool, sources); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	yt := fake.New()
	s := &catalog.Service{DB: pool, YT: yt, Detect: langdetect.DefaultFake, Now: func() time.Time { return now }}
	return s, yt, pool, &now
}

func esQuery(text string, level *int) catalog.SourcesFile {
	return catalog.SourcesFile{"es": {Queries: []catalog.SourceEntry{{Text: text, Level: level}}}}
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// V3 acceptance: a Spanish run over 50 IDs stores exactly the 20 tagged plus
// the 15 detected.
func TestSearchAcceptanceMix(t *testing.T) {
	s, yt, pool, _ := setup(t, esQuery("aprender español", nil))
	keep := fake.AcceptanceMix(yt, "aprender español", "es", spanishTitles)
	kept, err := s.SearchOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if kept != 35 || len(keep) != 35 {
		t.Fatalf("kept %d, want 35", kept)
	}
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE youtube_video_id = ANY($1) AND language = 'es' AND status = 'live'`, keep); n != 35 {
		t.Fatalf("%d of the expected clips stored", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE language_source = 'audio_tag'`); n != 20 {
		t.Errorf("audio_tag = %d, want 20", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE language_source = 'text_detect'`); n != 15 {
		t.Errorf("text_detect = %d, want 15", n)
	}
	// 100 for the search plus 1 for one videos.list batch of 50.
	if used, _ := s.UnitsUsedToday(context.Background()); used != 101 {
		t.Errorf("units used = %d, want 101", used)
	}
	q := yt.Searches[0]
	if q.RelevanceLanguage != "es" || q.Query != "aprender español" || q.PublishedAfter.IsZero() {
		t.Errorf("search query = %+v", q)
	}
	// The 15 rejected IDs are remembered, so running again finds only known
	// IDs: no videos.list call, nothing new.
	if n := count(t, pool, `SELECT count(*) FROM youtube_rejects`); n != 15 {
		t.Errorf("rejects = %d, want 15", n)
	}
	calls := len(yt.VideoIDs)
	if kept, err := s.SearchOnce(context.Background()); err != nil || kept != 0 || len(yt.VideoIDs) != calls {
		t.Fatalf("second run kept %d, err %v, videos calls %d -> %d", kept, err, calls, len(yt.VideoIDs))
	}
}

func TestKeepFilters(t *testing.T) {
	good := fake.Video("a", "x")
	cases := map[string]func(*youtube.Video){
		"too long":       func(v *youtube.Video) { v.DurationMs = 181_000 },
		"not embeddable": func(v *youtube.Video) { v.Embeddable = false },
		"private":        func(v *youtube.Video) { v.Public = false },
		"made for kids":  func(v *youtube.Video) { v.MadeForKids = true },
		"age restricted": func(v *youtube.Video) { v.AgeRestricted = true },
		"landscape":      func(v *youtube.Video) { v.EmbedWidth, v.EmbedHeight = 1080, 608 },
		"square":         func(v *youtube.Video) { v.EmbedWidth, v.EmbedHeight = 1080, 1080 },
	}
	if !catalog.Keep(good) {
		t.Fatal("good clip dropped")
	}
	edge := good
	edge.DurationMs = 180_000
	if !catalog.Keep(edge) {
		t.Error("exactly 180 s dropped")
	}
	for name, f := range cases {
		v := good
		f(&v)
		if catalog.Keep(v) {
			t.Errorf("%s kept", name)
		}
	}
}

func TestClassifyLanguage(t *testing.T) {
	det := langdetect.DefaultFake
	seeds := map[string]bool{"UCseed": true}
	v := func(audio, channel, title string) youtube.Video {
		x := fake.Video("id", title)
		x.DefaultAudioLanguage, x.ChannelID = audio, channel
		return x
	}
	cases := []struct {
		name, target string
		v            youtube.Video
		ok           bool
		source       string
	}{
		{"audio tag matches", "es", v("es", "UCx", "anything"), true, "audio_tag"},
		{"regional audio tag counts", "es", v("es-419", "UCx", "anything"), true, "audio_tag"},
		{"pt-BR target matches pt", "pt-BR", v("pt", "UCx", "x"), true, "audio_tag"},
		{"audio tag wins over seed channel", "es", v("en", "UCseed", "hola"), false, ""},
		{"seed channel", "es", v("", "UCseed", "no words"), true, "seed_channel"},
		{"text detection", "es", v("", "UCx", "Hola amigos"), true, "text_detect"},
		{"detected other language", "es", v("", "UCx", "Bonjour"), false, ""},
		{"nothing to go on", "es", v("", "UCx", "zzz"), false, ""},
	}
	for _, c := range cases {
		got, ok := catalog.ClassifyLanguage(c.v, c.target, seeds, det, 0.8)
		if ok != c.ok || got.Source != c.source {
			t.Errorf("%s: got %+v %v, want ok=%v source=%q", c.name, got, ok, c.ok, c.source)
		}
	}
	// Below the confidence threshold: dropped.
	if _, ok := catalog.ClassifyLanguage(v("", "UCx", "Hola"), "es", nil, det, 0.99); ok {
		t.Error("low-confidence detection kept")
	}
}

func TestGuessLevel(t *testing.T) {
	kw := catalog.LevelKeywords{1: {"principiantes", "a1", "初心者"}, 3: {"intermedio", "b1"}, 5: {"avanzado"}}
	v := func(title string) youtube.Video { return fake.Video("id", title) }
	cases := []struct {
		title  string
		hint   *int
		want   *int
		source string
	}{
		{"Español para principiantes", nil, ptr(1), "keyword"},
		{"Nivel intermedio", nil, ptr(3), "keyword"},
		{"Lección A1", nil, ptr(1), "keyword"},
		{"A10 highway", nil, nil, ""}, // whole words only
		{"日本語 初心者向け", nil, ptr(1), "keyword"},
		{"intermedio o avanzado", nil, ptr(3), "keyword"}, // lowest level wins
		{"Nada", nil, nil, ""},
		{"Español para principiantes", ptr(4), ptr(4), "hint"}, // source hint first
	}
	for _, c := range cases {
		got, src := catalog.GuessLevel(v(c.title), c.hint, kw)
		if src != c.source || (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%q: got %v %q", c.title, got, src)
		}
	}
}

// V3 acceptance: a "para principiantes" title gets level 1 or 2 from keywords.
func TestStoredLevelFromKeywords(t *testing.T) {
	s, yt, pool, _ := setup(t, esQuery("q", nil))
	v := fake.Video("lvl1", "Español para principiantes")
	v.DefaultAudioLanguage = "es"
	yt.Add("q", v)
	if _, err := s.SearchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var level int
	var src string
	if err := pool.QueryRow(context.Background(), `SELECT level, level_source FROM videos WHERE youtube_video_id = 'lvl1'`).Scan(&level, &src); err != nil {
		t.Fatal(err)
	}
	if level < 1 || level > 2 || src != "keyword" {
		t.Fatalf("level %d %q", level, src)
	}
}

func TestSourceLevelHintStored(t *testing.T) {
	s, yt, pool, _ := setup(t, esQuery("q", ptr(5)))
	v := fake.Video("hinted", "Clip")
	v.DefaultAudioLanguage = "es"
	yt.Add("q", v)
	if _, err := s.SearchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE level = 5 AND level_source = 'hint'`); n != 1 {
		t.Fatal("hint not stored")
	}
}

// V3 acceptance: the search job stops at the daily unit budget and resumes
// after midnight UTC.
func TestQuotaStopsAndResumesAtMidnightUTC(t *testing.T) {
	s, yt, pool, now := setup(t, esQuery("q", nil))
	ctx := context.Background()
	if err := appconfig.Set(ctx, appconfig.Store{DB: pool}, "youtube.daily_quota_units", 250); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		v := fake.Video("q"+string(rune('a'+i)), "Clip")
		v.DefaultAudioLanguage = "es"
		yt.Add("q", v)
	}
	searches, _, err := s.RunSearches(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if searches != 2 { // 2 x 100 (+1 for the first videos.list) fits 250; a third search doesn't
		t.Fatalf("searches = %d, want 2", searches)
	}
	if used, _ := s.UnitsUsedToday(ctx); used > 250 {
		t.Fatalf("spent %d of 250", used)
	}
	// Still today: nothing more runs.
	*now = now.Add(11*time.Hour + 59*time.Minute) // 23:59 UTC
	if n, _, _ := s.RunSearches(ctx, 10); n != 0 {
		t.Fatalf("ran %d searches over budget", n)
	}
	// Past midnight UTC: a new budget.
	*now = now.Add(2 * time.Minute)
	if n, _, _ := s.RunSearches(ctx, 1); n != 1 {
		t.Fatalf("did not resume after midnight UTC (ran %d)", n)
	}
	if used, _ := s.UnitsUsedToday(ctx); used != 100 {
		t.Fatalf("new day used %d", used)
	}
}

func TestYouTubeQuotaErrorStopsQuietly(t *testing.T) {
	s, yt, _, _ := setup(t, esQuery("q", nil))
	yt.Err = youtube.ErrQuotaExceeded
	n, _, err := s.RunSearches(context.Background(), 5)
	if err != nil || n != 0 {
		t.Fatalf("got %d, %v", n, err)
	}
}

func TestPickSourceFavoursLanguagesWithFewerSearchesAndClips(t *testing.T) {
	s, yt, pool, _ := setup(t, catalog.SourcesFile{
		"es": {Queries: []catalog.SourceEntry{{Text: "es1"}, {Text: "es2"}}},
		"fr": {Queries: []catalog.SourceEntry{{Text: "fr1"}}},
	})
	// Spanish already has plenty of clips.
	dbtest.InsertClips(t, pool, "es", 1000, 10, nil)
	ctx := context.Background()
	for range 3 {
		if _, err := s.SearchOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, q := range yt.Searches {
		got = append(got, q.Query)
	}
	// fr first (no clips), then es (fr now has a search; es has 1000 clips
	// = 2 searches' weight, so fr goes again first).
	if got[0] != "fr1" || got[1] != "fr1" {
		t.Fatalf("search order = %v", got)
	}
	// The same source rotates its publishedAfter window.
	if yt.Searches[0].PublishedAfter.Equal(yt.Searches[1].PublishedAfter) {
		t.Error("window did not rotate between runs of the same source")
	}
}

// V3 acceptance: a clip refreshed 26 days ago is re-read; a deleted one
// ends removed with its text fields cleared.
func TestRefresh(t *testing.T) {
	s, yt, pool, _ := setup(t, esQuery("q", nil))
	ctx := context.Background()
	for _, id := range []string{"keep", "gone", "private", "recent"} {
		v := fake.Video(id, "Hola "+id)
		v.DefaultAudioLanguage = "es"
		v.Tags = []string{"tag"}
		yt.Add("q", v)
	}
	if _, err := s.SearchOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE videos SET refreshed_at = now() - interval '26 days' WHERE youtube_video_id <> 'recent'`); err != nil {
		t.Fatal(err)
	}
	yt.Remove("gone")
	yt.Update("private", func(v *youtube.Video) { v.Public = false })
	yt.Update("keep", func(v *youtube.Video) { v.ViewCount = 999_999; v.HasCaptions = false })
	calls := len(yt.VideoIDs)

	if _, err := pool.Exec(ctx, `INSERT INTO youtube_rejects VALUES ('old', 'filters', now() - interval '31 days'), ('new', 'language', now())`); err != nil {
		t.Fatal(err)
	}
	refreshed, removed, err := s.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, `SELECT count(*) FROM youtube_rejects WHERE youtube_video_id IN ('old', 'new')`); n != 1 {
		t.Errorf("expired rejects not pruned: %d left", n)
	}
	if refreshed != 1 || removed != 2 {
		t.Fatalf("refreshed %d removed %d", refreshed, removed)
	}
	if len(yt.VideoIDs) != calls+1 || len(yt.VideoIDs[calls]) != 3 {
		t.Fatalf("expected one videos.list call for the 3 stale clips, got %v", yt.VideoIDs[calls:])
	}
	var views int64
	var captions bool
	if err := pool.QueryRow(ctx, `SELECT view_count, has_captions FROM videos WHERE youtube_video_id = 'keep' AND status = 'live' AND refreshed_at > now() - interval '1 minute'`).Scan(&views, &captions); err != nil {
		t.Fatal(err)
	}
	if views != 999_999 || captions {
		t.Errorf("stats not updated: %d %v", views, captions)
	}
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE youtube_video_id IN ('gone', 'private') AND status = 'removed'
		AND youtube_title IS NULL AND youtube_description IS NULL AND youtube_channel_title IS NULL AND youtube_tags = '{}'`); n != 2 {
		t.Fatalf("%d removed clips with text cleared, want 2", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE youtube_video_id = 'recent' AND refreshed_at > now() - interval '1 minute'`); n != 1 {
		t.Error("recent clip touched")
	}
	// Nothing older than 30 days stays live.
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE status = 'live' AND refreshed_at < now() - interval '30 days'`); n != 0 {
		t.Errorf("%d live clips older than 30 days", n)
	}
}

func TestRefineLevels(t *testing.T) {
	s, _, pool, _ := setup(t, esQuery("q", nil))
	ctx := context.Background()
	ids := dbtest.InsertClips(t, pool, "es", 2, 1, func(i int, c *dbtest.Clip) { c.Level = ptr(1) })
	// Clip 0: 20 completed views from level 3 and 4 viewers -> 3.5 rounds to 4.
	// Clip 1: 19 completed views: not enough.
	for _, r := range []struct {
		id                 any
		level, completions int
	}{{ids[0], 3, 10}, {ids[0], 4, 10}, {ids[1], 5, 19}} {
		if _, err := pool.Exec(ctx, `INSERT INTO video_level_stats (video_id, viewer_level, views, completions) VALUES ($1, $2, $3, $3)`, r.id, r.level, r.completions); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.RefineLevels(ctx)
	if err != nil || n != 1 {
		t.Fatalf("updated %d, %v", n, err)
	}
	if c := count(t, pool, `SELECT count(*) FROM videos WHERE id = $1 AND level = 4 AND level_source = 'engagement'`, ids[0]); c != 1 {
		t.Error("clip 0 not refined to 4")
	}
	if c := count(t, pool, `SELECT count(*) FROM videos WHERE id = $1 AND level = 1`, ids[1]); c != 1 {
		t.Error("clip 1 changed with only 19 completions")
	}
	if n, _ := s.RefineLevels(ctx); n != 0 {
		t.Errorf("second run updated %d", n)
	}
}

func TestApplySourcesDisablesMissing(t *testing.T) {
	_, _, pool, _ := setup(t, catalog.SourcesFile{"es": {Queries: []catalog.SourceEntry{{Text: "a"}, {Text: "b"}}}})
	if _, err := catalog.ApplySources(context.Background(), pool, catalog.SourcesFile{
		"es": {Queries: []catalog.SourceEntry{{Text: "a", Level: ptr(2)}}, Channels: []catalog.SourceEntry{{ID: "UCx"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, `SELECT count(*) FROM youtube_sources WHERE enabled`); n != 2 {
		t.Errorf("enabled = %d, want 2", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM youtube_sources WHERE value = 'b' AND NOT enabled`); n != 1 {
		t.Error("b not disabled")
	}
	if n := count(t, pool, `SELECT count(*) FROM youtube_sources WHERE value = 'a' AND level_hint = 2`); n != 1 {
		t.Error("level hint not updated")
	}
	if _, err := catalog.ApplySources(context.Background(), pool, catalog.SourcesFile{"xx": {Queries: []catalog.SourceEntry{{Text: "a"}}}}); err == nil {
		t.Error("unknown language accepted")
	}
}

func TestSourcesFileParses(t *testing.T) {
	f, err := catalog.LoadSourcesFile("../../../infra/youtube_sources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(f["es"].Queries) == 0 {
		t.Fatal("no Spanish queries")
	}
}
