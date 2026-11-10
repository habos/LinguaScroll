package feed

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"
)

func ptr[T any](v T) *T { return &v }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestScoreFixedInputs(t *testing.T) {
	w := DefaultConfig().Weights
	cases := []struct {
		name  string
		s     Signals
		noise float64
		want  float64
	}{
		{"all zero", Signals{}, 0, 0},
		{"all one", Signals{1, 1, 1, 1, 1, 1, 1, 1}, 0, 0.25 + 0.15 + 0.15 + 0.10 + 0.15 + 0.10 + 0.05 - 0.30},
		{"new device, popular fresh clip", Signals{Completion: 0.5, ViewsPct: 0.9, LikesPct: 0.8, Level: 0.5, Fresh: 1}, 0.02,
			0.25*0.5 + 0.15*0.9 + 0.15*0.8 + 0.10*0.5 + 0.05*1 + 0.02},
		{"hidden a lot", Signals{Completion: 0.5, Hide: 0.5}, 0, 0.125 - 0.15},
		{"channel affinity", Signals{Channel: 1}, 0, 0.15},
	}
	for _, c := range cases {
		if got := Score(w, c.s, c.noise); !near(got, c.want) {
			t.Errorf("%s: Score = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBaseScoreIgnoresPersonalSignals(t *testing.T) {
	w := DefaultConfig().Weights
	s := Signals{Completion: 0.6, ViewsPct: 0.5, LikesPct: 0.4, Level: 1, Channel: 1, Category: 1, Fresh: 0.5, Hide: 0.1}
	want := 0.25*0.6 + 0.15*0.5 + 0.15*0.4 + 0.05*0.5 - 0.30*0.1
	if got := BaseScore(w, s); !near(got, want) {
		t.Fatalf("BaseScore = %v, want %v", got, want)
	}
}

func TestSignalHelpers(t *testing.T) {
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	days := func(d int) *time.Time { return ptr(now.AddDate(0, 0, -d)) }
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{
		{"completion with no views is 0.5", CompletionRate(0, 0), 0.5},
		{"completion smoothed", CompletionRate(8, 10), 10.0 / 14},
		{"hide rate smoothed", HideRate(1, 0), 0.2},
		{"hide rate capped", HideRate(100, 1), 1},
		{"fresh today", Freshness(days(0), now), 1},
		{"fresh 30 days", Freshness(days(30), now), 1},
		{"fresh halfway", Freshness(days(30+335/2), now), 1 - float64(335/2)/335},
		{"fresh a year", Freshness(days(365), now), 0},
		{"fresh unknown", Freshness(nil, now), 0},
		{"level same", LevelCloseness(ptr(3), ptr(3)), 1},
		{"level one away", LevelCloseness(ptr(3), ptr(4)), 0.5},
		{"level two away", LevelCloseness(ptr(3), ptr(5)), 0},
		{"level clip unknown", LevelCloseness(ptr(3), nil), 0.5},
		{"level device unknown", LevelCloseness(nil, ptr(2)), 0.5},
	} {
		if !near(c.got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestRankChannelAffinityWins(t *testing.T) {
	now := time.Now()
	a := Clip{ID: uuid.New(), ChannelID: "A", ViewsPct: 0.9, LikesPct: 0.9}
	b := Clip{ID: uuid.New(), ChannelID: "B", ViewsPct: 0.8, LikesPct: 0.8}
	w := DefaultConfig().Weights
	w.Noise = 0
	clips := []Clip{a, b}
	Rank(clips, Profile{}, w, now, rand.New(rand.NewPCG(1, 1)))
	if clips[0].ChannelID != "A" {
		t.Fatalf("without affinity want A first, got %s", clips[0].ChannelID)
	}
	clips = []Clip{a, b}
	Rank(clips, Profile{ChannelAffinity: map[string]float64{"B": 1}}, w, now, rand.New(rand.NewPCG(1, 1)))
	if clips[0].ChannelID != "B" {
		t.Fatalf("liked channel B should rank first, got %s", clips[0].ChannelID)
	}
}

func TestExploreSlots(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 7))
	counts := map[int]int{}
	for range 1000 {
		counts[ExploreSlots(10, 0.15, rng)]++
	}
	if len(counts) != 2 || counts[1] == 0 || counts[2] == 0 {
		t.Fatalf("want only 1 or 2 explore slots, got %v", counts)
	}
	if ExploreSlots(10, 0, rng) != 0 {
		t.Fatal("share 0 must give 0 slots")
	}
}

// pool makes n clips over `channels` channels, best first.
func pool(n, channels int, category func(i int) string) []Clip {
	out := make([]Clip, n)
	for i := range out {
		out[i] = Clip{
			ID:         uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprint(i))),
			ChannelID:  fmt.Sprintf("ch%d", i%channels),
			CategoryID: category(i),
			Score:      float64(n - i),
		}
	}
	return out
}

func checkSpacing(t *testing.T, seq []string, spacing int) {
	t.Helper()
	for i := range seq {
		for j := i + 1; j < len(seq) && j < i+spacing; j++ {
			if seq[i] == seq[j] {
				t.Fatalf("channel %s at %d and %d (spacing %d): %v", seq[i], i, j, spacing, seq)
			}
		}
	}
}

func TestAssembleChannelSpacingAcrossPages(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 3))
	// One channel dominates the top of the ranking.
	ranked := pool(200, 8, func(int) string { return "27" })
	for i := 0; i < 20; i++ {
		ranked[i].ChannelID = "ch0"
	}
	var seq, recent []string
	used := map[uuid.UUID]bool{}
	for range 10 {
		var rest []Clip
		for _, c := range ranked {
			if !used[c.ID] {
				rest = append(rest, c)
			}
		}
		page := Assemble(rest, nil, 10, 0, 5, 0.2, recent, rng)
		if len(page) != 10 {
			t.Fatalf("page has %d items", len(page))
		}
		for _, c := range page {
			used[c.ID] = true
			seq = append(seq, c.ChannelID)
		}
		recent = RecentChannels(recent, page, 4)
	}
	checkSpacing(t, seq, 5)
}

func TestAssembleMusicCap(t *testing.T) {
	rng := rand.New(rand.NewPCG(4, 4))
	// Top half of the ranking is all music.
	ranked := pool(100, 20, func(i int) string {
		if i < 50 {
			return MusicCategory
		}
		return "27"
	})
	page := Assemble(ranked, nil, 10, 0, 5, 0.2, nil, rng)
	music := 0
	for _, c := range page {
		if c.CategoryID == MusicCategory {
			music++
		}
	}
	if music != 2 {
		t.Fatalf("music clips = %d, want 2 (cap 0.2 of 10)", music)
	}
}

func TestAssembleExploreSlots(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 5))
	ranked := pool(50, 25, func(int) string { return "27" })
	explore := []Clip{
		{ID: uuid.New(), ChannelID: "x1"},
		{ID: uuid.New(), ChannelID: "x2"},
		{ID: uuid.New(), ChannelID: "x3"},
	}
	page := Assemble(ranked, explore, 10, 2, 5, 0.2, nil, rng)
	n := 0
	for i, c := range page {
		if c.ChannelID[0] == 'x' {
			n++
			if i == 0 {
				t.Error("explore clip in first slot")
			}
		}
	}
	if n != 2 {
		t.Fatalf("explore clips = %d, want 2", n)
	}
}

func TestAssembleSmallCatalogStillFills(t *testing.T) {
	rng := rand.New(rand.NewPCG(6, 6))
	// Two channels can't satisfy spacing 5; the page still fills.
	ranked := pool(12, 2, func(int) string { return MusicCategory })
	page := Assemble(ranked, nil, 10, 0, 5, 0.2, nil, rng)
	if len(page) != 10 {
		t.Fatalf("page has %d items, want 10", len(page))
	}
	// And it still prefers alternating channels.
	if page[0].ChannelID == page[1].ChannelID {
		t.Fatalf("first two items share a channel: %v %v", page[0].ChannelID, page[1].ChannelID)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	c := Cursor{Page: 3, Recent: []string{"a", "b"}}
	got, err := DecodeCursor(c.Encode())
	if err != nil || got.Page != 3 || len(got.Recent) != 2 || got.Recent[1] != "b" {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	for _, bad := range []string{"!!", "e30", "bm9wZQ"} {
		if _, err := DecodeCursor(bad); err != ErrBadCursor {
			t.Errorf("DecodeCursor(%q) err = %v, want ErrBadCursor", bad, err)
		}
	}
	if c, err := DecodeCursor(""); err != nil || c.Page != 0 {
		t.Fatalf("empty cursor: %+v %v", c, err)
	}
}
