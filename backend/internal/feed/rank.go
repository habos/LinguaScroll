package feed

import (
	"math"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/google/uuid"
)

// MusicCategory is YouTube's "Music" category ID.
const MusicCategory = "10"

// Signals are the score inputs for one clip and one device, each in 0..1.
type Signals struct {
	Completion float64 // c: smoothed in-app completion rate
	ViewsPct   float64 // p_views
	LikesPct   float64 // p_likes
	Level      float64 // v: level closeness
	Channel    float64 // k: channel affinity
	Category   float64 // g: device's completed share in this category
	Fresh      float64 // f
	Hide       float64 // h: in-app hide rate
}

// Score is the v0 formula. noise is e, drawn by the caller from
// [0, w.Noise) so tests can pin it.
func Score(w Weights, s Signals, noise float64) float64 {
	return w.Completion*s.Completion +
		w.ViewsPct*s.ViewsPct +
		w.LikesPct*s.LikesPct +
		w.Level*s.Level +
		w.Channel*s.Channel +
		w.Category*s.Category +
		w.Fresh*s.Fresh +
		w.Hide*s.Hide +
		noise
}

// BaseScore is the device-independent part of Score, precomputed by the
// stats job to pick the top candidates: quality and freshness only.
func BaseScore(w Weights, s Signals) float64 {
	s.Level, s.Channel, s.Category = 0, 0, 0
	return Score(w, s, 0)
}

// CompletionRate is c, smoothed toward 0.5 so new clips aren't 0 or 1.
func CompletionRate(completions, views int) float64 {
	return float64(completions+2) / float64(views+4)
}

// HideRate is h: hides per impression, smoothed so one early hide on a
// clip with a handful of impressions doesn't bury it.
func HideRate(hides, impressions int) float64 {
	return math.Min(1, float64(hides)/float64(impressions+5))
}

// Freshness is f: 1 within 30 days of publishing, then linear to 0 at a year.
func Freshness(publishedAt *time.Time, now time.Time) float64 {
	if publishedAt == nil {
		return 0
	}
	days := now.Sub(*publishedAt).Hours() / 24
	switch {
	case days <= 30:
		return 1
	case days >= 365:
		return 0
	default:
		return 1 - (days-30)/(365-30)
	}
}

// LevelCloseness is v: 1 at the device's level, 0.5 one away or when either
// level is unknown, 0 further away.
func LevelCloseness(deviceLevel, clipLevel *int) float64 {
	if deviceLevel == nil || clipLevel == nil {
		return 0.5
	}
	switch d := *deviceLevel - *clipLevel; {
	case d == 0:
		return 1
	case d == 1 || d == -1:
		return 0.5
	default:
		return 0
	}
}

// Clip is a candidate with everything ranking and the response need.
type Clip struct {
	ID           uuid.UUID
	YouTubeID    string
	ChannelID    string
	ChannelTitle string
	Title        string
	CategoryID   string
	Level        *int
	DurationMS   int
	Aspect       float32
	HasCaptions  bool
	PublishedAt  *time.Time

	Impressions int
	Views       int
	Completions int
	Hides       int
	ViewsPct    float64
	LikesPct    float64

	Score float64
}

// Profile is what we know about one device for personal signals.
type Profile struct {
	Level *int
	// ChannelAffinity is k per channel (1 liked/saved, 0.5 completed >= 2).
	ChannelAffinity map[string]float64
	// CategoryShare is g per YouTube category.
	CategoryShare map[string]float64
}

// Signals derives a clip's score inputs for p.
func (p Profile) Signals(c Clip, now time.Time) Signals {
	return Signals{
		Completion: CompletionRate(c.Completions, c.Views),
		ViewsPct:   c.ViewsPct,
		LikesPct:   c.LikesPct,
		Level:      LevelCloseness(p.Level, c.Level),
		Channel:    p.ChannelAffinity[c.ChannelID],
		Category:   p.CategoryShare[c.CategoryID],
		Fresh:      Freshness(c.PublishedAt, now),
		Hide:       HideRate(c.Hides, c.Impressions),
	}
}

// Rank scores clips for p and sorts them best first (ties by ID).
func Rank(clips []Clip, p Profile, w Weights, now time.Time, rng *rand.Rand) {
	for i := range clips {
		noise := 0.0
		if w.Noise > 0 {
			noise = rng.Float64() * w.Noise
		}
		clips[i].Score = Score(w, p.Signals(clips[i], now), noise)
	}
	sort.SliceStable(clips, func(i, j int) bool {
		if clips[i].Score != clips[j].Score {
			return clips[i].Score > clips[j].Score
		}
		return clips[i].ID.String() < clips[j].ID.String()
	})
}

// ExploreSlots is how many of n items are explore picks: n*share rounded
// randomly, so 10 * 0.15 gives 1 or 2 (2 half the time).
func ExploreSlots(n int, share float64, rng *rand.Rand) int {
	x := float64(n) * share
	k := int(x)
	if rng.Float64() < x-float64(k) {
		k++
	}
	return min(k, n)
}

// Assemble builds one page of up to n clips from ranked (best first) and
// explore (already shuffled) pools. recent holds the channels of the items
// just before this page, oldest first (from the cursor).
//
// Rules, in order of strictness:
//   - no channel twice within any `spacing` consecutive items, counting recent;
//   - at most floor(musicCap*n) music clips per page;
//   - explore clips go in `exploreN` random slots after the first.
//
// When no clip satisfies the rules the music cap is dropped first, then
// spacing, so a small catalog still fills a page rather than ending the feed.
func Assemble(ranked, explore []Clip, n, exploreN, spacing int, musicCap float64, recent []string, rng *rand.Rand) []Clip {
	used := make(map[uuid.UUID]bool, n)
	window := append([]string(nil), recent...)
	maxMusic := int(math.Floor(musicCap*float64(n) + 1e-9))
	music := 0

	exploreAt := map[int]bool{}
	if n > 1 && exploreN > 0 {
		for _, i := range rng.Perm(n - 1)[:min(exploreN, n-1)] {
			exploreAt[i+1] = true
		}
	}

	ok := func(c Clip, checkSpacing, checkMusic bool) bool {
		if used[c.ID] {
			return false
		}
		if checkMusic && c.CategoryID == MusicCategory && music >= maxMusic {
			return false
		}
		if checkSpacing {
			lo := max(0, len(window)-(spacing-1))
			for _, ch := range window[lo:] {
				if ch == c.ChannelID {
					return false
				}
			}
		}
		return true
	}
	pick := func(pool []Clip, checkSpacing, checkMusic bool) (Clip, bool) {
		for _, c := range pool {
			if ok(c, checkSpacing, checkMusic) {
				return c, true
			}
		}
		return Clip{}, false
	}

	out := make([]Clip, 0, n)
	for slot := 0; slot < n; slot++ {
		pools := [][]Clip{ranked}
		if exploreAt[slot] {
			pools = [][]Clip{explore, ranked}
		}
		var c Clip
		found := false
		for _, rule := range [][2]bool{{true, true}, {true, false}, {false, false}} {
			for _, pool := range pools {
				if c, found = pick(pool, rule[0], rule[1]); found {
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			break
		}
		used[c.ID] = true
		if c.CategoryID == MusicCategory {
			music++
		}
		window = append(window, c.ChannelID)
		out = append(out, c)
	}
	return out
}

// RecentChannels returns the last `keep` channels of recent followed by page.
func RecentChannels(recent []string, page []Clip, keep int) []string {
	all := append([]string(nil), recent...)
	for _, c := range page {
		all = append(all, c.ChannelID)
	}
	if keep <= 0 {
		return nil
	}
	if len(all) > keep {
		all = all[len(all)-keep:]
	}
	return all
}
