package catalog_test

import (
	"context"
	"testing"

	"github.com/habos/linguascroll/backend/internal/catalog"
	"github.com/habos/linguascroll/backend/internal/langdetect"
	"github.com/habos/linguascroll/backend/internal/youtube"
	"github.com/habos/linguascroll/backend/internal/youtube/fake"
)

// A detector that, like lingua, calls any Hangul text Korean.
var hangulDetector = langdetect.Fake{"ko": {"한국", "먹었어요", "오늘", "떡볶이", "바이럴", "웃긴"}, "hi": {"में"}, "en": {"korean", "girl", "funny"}}

func koRules() catalog.LanguageRules {
	return catalog.LanguageRules{Target: "ko", Script: "Hangul", SeedChannels: map[string]bool{"UCseed": true},
		Detector: hangulDetector, MinConfidence: 0.8, MinScriptShare: 0.5}
}

func TestScriptShare(t *testing.T) {
	cases := []struct {
		text, script string
		share        float64
		words        int
	}{
		{"떡볶이 먹방 vlog", "Hangul", 2.0 / 3, 3},
		{"Korean girl reacts", "Hangul", 0, 3},
		{"日本語を勉強しています", "Japanese", 1, 1},
		{"नमस्ते दोस्तों", "Devanagari", 1, 2}, // vowel signs stay in their word
		{"Привет мир hello", "Cyrillic", 2.0 / 3, 3},
		{"", "Hangul", 0, 0},
		{"anything", "Klingon", -1, 1}, // unknown script: no check
	}
	for _, c := range cases {
		share, n := catalog.ScriptShare(c.text, c.script)
		if n != c.words || share < c.share-0.01 || share > c.share+0.01 {
			t.Errorf("ScriptShare(%q, %s) = %.2f, %d; want %.2f, %d", c.text, c.script, share, n, c.share, c.words)
		}
	}
	if got := catalog.CleanText("#한국바이럴 #웃긴영상 @someone https://x.y/z 진짜 ＃쇼츠"); got != "진짜" {
		t.Errorf("CleanText = %q", got)
	}
}

// The clips found in the first real Korean run (2026-11-17) that were not
// Korean: hashtag-only titles from reupload channels, some with a false
// Korean audio tag.
func TestClassifyLanguageRejectsLanguageMentions(t *testing.T) {
	v := func(audio, metaLang, channel, title string) youtube.Video {
		x := fake.Video("id", title)
		x.DefaultAudioLanguage, x.DefaultLanguage, x.ChannelID = audio, metaLang, channel
		return x
	}
	spam := "#한국바이럴 #웃긴영상 #코미디 #꿀잼 #유튜브쇼츠 #쇼츠 #바이럴영상"
	cases := []struct {
		name   string
		v      youtube.Video
		ok     bool
		source string
	}{
		{"hashtag-only title, no audio tag", v("", "", "UCx", spam), false, ""},
		{"hashtag-only title, false Korean audio tag", v("ko", "", "UCx", spam), false, ""},
		{"hashtags with one stray word", v("ko", "", "UCx", "#한국바이럴 #웃긴영상 #바이럴 트렌드 #재미"), false, ""},
		{"real title with many hashtags", v("ko", "", "UCx", "전 세계가 놀랐던 북한 브라질 경기 #shorts #short #쇼츠 #축구 #한국"), true, "audio_tag"},
		{"hashtag-only title from a seed channel", v("", "", "UCseed", spam), true, "seed_channel"},
		{"English title mentioning Korean", v("", "", "UCx", "Korean girl funny #한국 #korean"), false, ""},
		{"Hindi title on a clip tagged Korean", v("ko", "", "UCx", "कोरियन ड्रामा में मज़ा #한국"), false, ""},
		{"English title on a clip tagged Korean", v("ko", "", "UCx", "Day in Seoul 🇰🇷"), true, "audio_tag"},
		{"Korean title, Korean audio", v("ko", "", "UCx", "한국 수출 역대급 호황 #쇼츠"), true, "audio_tag"},
		{"Korean title, detected", v("", "", "UCx", "오늘 떡볶이 먹었어요 #쇼츠"), true, "text_detect"},
		{"Korean title, metadata says English", v("", "en", "UCx", "오늘 떡볶이 먹었어요"), false, ""},
		{"Korean title, metadata says Korean", v("", "ko", "UCx", "오늘 떡볶이 먹었어요"), true, "text_detect"},
		{"mostly Latin title detected as Korean", v("", "", "UCx", "한국 trip vlog day one"), false, ""},
	}
	for _, c := range cases {
		got, ok := catalog.ClassifyLanguage(c.v, koRules())
		if ok != c.ok || got.Source != c.source {
			t.Errorf("%s: got %+v %v, want ok=%v source=%q", c.name, got, ok, c.ok, c.source)
		}
	}
}

func koQuery(text string) catalog.SourcesFile {
	return catalog.SourcesFile{"ko": {Queries: []catalog.SourceEntry{{Text: text}}}}
}

func TestSearchDropsBlockedChannelsAndHashtagSpam(t *testing.T) {
	s, yt, pool, _ := setup(t, koQuery("한국어"))
	s.Detect = hangulDetector
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE app_config SET value = '["UCblocked"]' WHERE key = 'youtube.blocked_channels'`); err != nil {
		t.Fatal(err)
	}
	good := fake.Video("good", "오늘 떡볶이 먹었어요")
	blocked := fake.Video("blocked", "오늘 떡볶이 먹었어요")
	blocked.ChannelID = "UCblocked"
	spam := fake.Video("spam", "#한국바이럴 #웃긴영상")
	spam.DefaultAudioLanguage = "ko"
	yt.Add("한국어", good, blocked, spam)
	kept, err := s.SearchOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Fatalf("kept %d, want 1", kept)
	}
	if n := count(t, pool, `SELECT count(*) FROM youtube_rejects WHERE (youtube_video_id, reason) IN (('blocked', 'channel'), ('spam', 'language'))`); n != 2 {
		t.Errorf("%d of 2 rejects recorded with the right reason", n)
	}
}

func TestRecheckLanguagesPurgesOldMismatches(t *testing.T) {
	s, yt, pool, _ := setup(t, koQuery("한국어"))
	ctx := context.Background()
	// Stored under the old rules: tags and hashtags counted, so these got in.
	if _, err := pool.Exec(ctx, `
		INSERT INTO videos (id, source, youtube_video_id, youtube_channel_id, youtube_title, language, language_source, language_confidence, duration_ms, aspect, status, refreshed_at)
		VALUES (gen_random_uuid(), 'youtube', 'spam', 'UCx', '#한국바이럴', 'ko', 'audio_tag', 1, 30000, 0.56, 'live', now()),
		       (gen_random_uuid(), 'youtube', 'good', 'UCx', '오늘 떡볶이 먹었어요', 'ko', 'text_detect', 1, 30000, 0.56, 'live', now()),
		       (gen_random_uuid(), 'youtube', 'blocked', 'UCbad', '오늘 떡볶이 먹었어요', 'ko', 'text_detect', 1, 30000, 0.56, 'live', now()),
		       (gen_random_uuid(), 'youtube', 'handpicked', 'UCx', 'x', 'ko', 'manual', 1, 30000, 0.56, 'live', now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE app_config SET value = '["UCbad"]' WHERE key = 'youtube.blocked_channels'`); err != nil {
		t.Fatal(err)
	}
	s.Detect = hangulDetector
	spam := fake.Video("spam", "#한국바이럴 #웃긴영상")
	spam.DefaultAudioLanguage = "ko"
	good := fake.Video("good", "오늘 떡볶이 먹었어요")
	blocked := fake.Video("blocked", "오늘 떡볶이 먹었어요")
	blocked.ChannelID = "UCbad"
	yt.Add("", spam, good, blocked, fake.Video("handpicked", "x"))

	kept, removed, err := s.RecheckLanguages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kept != 2 || removed != 2 {
		t.Fatalf("kept %d removed %d, want 2 and 2", kept, removed)
	}
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE status = 'removed' AND youtube_title IS NULL
		AND ((youtube_video_id = 'spam' AND hold_reason = 'language') OR (youtube_video_id = 'blocked' AND hold_reason = 'channel'))`); n != 2 {
		t.Errorf("%d of 2 clips removed with data cleared and the right reason", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM videos WHERE status = 'live' AND youtube_video_id IN ('good', 'handpicked')`); n != 2 {
		t.Errorf("%d of 2 good clips still live", n)
	}
	if used, _ := s.UnitsUsedToday(ctx); used != 1 {
		t.Errorf("units used = %d, want 1 (one videos.list for 4 clips)", used)
	}
}

func TestRefreshRechecksReportedClips(t *testing.T) {
	s, yt, pool, _ := setup(t, koQuery("한국어"))
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO videos (id, source, youtube_video_id, youtube_channel_id, youtube_title, language, language_source, language_confidence, duration_ms, aspect, status, refreshed_at, language_recheck)
		VALUES (gen_random_uuid(), 'youtube', 'reported', 'UCx', 'Korean girl funny', 'ko', 'text_detect', 1, 30000, 0.56, 'live', now(), true)`); err != nil {
		t.Fatal(err)
	}
	s.Detect = hangulDetector
	yt.Add("", fake.Video("reported", "Korean girl funny"))
	_, removed, err := s.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || count(t, pool, `SELECT count(*) FROM videos WHERE youtube_video_id = 'reported' AND status = 'removed' AND hold_reason = 'language' AND NOT language_recheck`) != 1 {
		t.Errorf("reported clip not rechecked and removed (removed=%d)", removed)
	}
}
