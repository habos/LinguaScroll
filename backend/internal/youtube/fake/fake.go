// Package fake is an in-memory youtube.Client for tests and FAKE_VENDORS=1.
// It never touches the network or spends quota.
package fake

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/habos/linguascroll/backend/internal/youtube"
)

// Client serves canned videos. Search returns every video whose
// SearchKey matches the query text or channel, in insertion order.
type Client struct {
	mu       sync.Mutex
	videos   map[string]youtube.Video
	order    []string
	keys     map[string][]string // search key -> video IDs
	Searches []youtube.SearchQuery
	VideoIDs [][]string // each Videos call's IDs
	// Err, when set, is returned by every call.
	Err error
}

// New returns an empty fake.
func New() *Client {
	return &Client{videos: map[string]youtube.Video{}, keys: map[string][]string{}}
}

// Add stores videos and makes them findable by searching key (a query
// string or a channel ID).
func (c *Client) Add(key string, vs ...youtube.Video) *Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, v := range vs {
		if _, ok := c.videos[v.ID]; !ok {
			c.order = append(c.order, v.ID)
		}
		c.videos[v.ID] = v
		c.keys[key] = append(c.keys[key], v.ID)
	}
	return c
}

// Remove deletes a video, as if it was taken down on YouTube.
func (c *Client) Remove(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.videos, id)
}

// Update changes a stored video in place.
func (c *Client) Update(id string, f func(*youtube.Video)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.videos[id]
	f(&v)
	c.videos[id] = v
}

// Search implements youtube.Client.
func (c *Client) Search(_ context.Context, q youtube.SearchQuery) ([]string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Searches = append(c.Searches, q)
	if c.Err != nil {
		return nil, "", c.Err
	}
	key := q.Query
	if q.ChannelID != "" {
		key = q.ChannelID
	}
	var ids []string
	for _, id := range c.keys[key] {
		if _, ok := c.videos[id]; ok {
			ids = append(ids, id)
		}
	}
	if len(ids) > youtube.MaxIDs {
		ids = ids[:youtube.MaxIDs]
	}
	return ids, "", nil
}

// Videos implements youtube.Client.
func (c *Client) Videos(_ context.Context, ids []string) ([]youtube.Video, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.VideoIDs = append(c.VideoIDs, append([]string(nil), ids...))
	if c.Err != nil {
		return nil, c.Err
	}
	if len(ids) > youtube.MaxIDs {
		return nil, fmt.Errorf("fake: %d IDs > %d", len(ids), youtube.MaxIDs)
	}
	var out []youtube.Video
	for _, id := range ids {
		if v, ok := c.videos[id]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

// IDs lists every stored video ID, sorted.
func (c *Client) IDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]string, 0, len(c.videos))
	for id := range c.videos {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Video returns a good vertical Short with the given ID; tweak it with the
// With* helpers or by editing fields.
func Video(id, title string) youtube.Video {
	return youtube.Video{
		ID: id, Title: title, ChannelID: "UCfake_" + id, ChannelTitle: "Fake " + id, CategoryID: "27",
		DurationMs: 30_000, PublishedAt: time.Now().Add(-48 * time.Hour), ViewCount: 10_000, LikeCount: 500,
		Embeddable: true, Public: true, HasCaptions: true, EmbedWidth: 608, EmbedHeight: 1080,
	}
}

// AcceptanceMix builds the V3 acceptance set under query key for target
// language lang: 20 vertical Shorts tagged lang, 5 landscape, 5 made for
// kids, 5 tagged "en", 15 untagged with titles from untaggedTitles (cycled).
// It returns the IDs that should be kept: the 20 tagged plus the 15
// untagged, 35 in all (when the titles are detected as lang).
func AcceptanceMix(c *Client, key, lang string, untaggedTitles []string) (keep []string) {
	n := 0
	next := func() string { n++; return fmt.Sprintf("mix_%s_%02d", lang, n) }
	for range 20 {
		v := Video(next(), "Clip")
		v.DefaultAudioLanguage = lang
		c.Add(key, v)
		keep = append(keep, v.ID)
	}
	for range 5 {
		v := Video(next(), "Landscape")
		v.DefaultAudioLanguage = lang
		v.EmbedWidth, v.EmbedHeight = 1080, 608
		c.Add(key, v)
	}
	for range 5 {
		v := Video(next(), "Kids")
		v.DefaultAudioLanguage = lang
		v.MadeForKids = true
		c.Add(key, v)
	}
	for range 5 {
		v := Video(next(), "English")
		v.DefaultAudioLanguage = "en"
		c.Add(key, v)
	}
	for i := range 15 {
		v := Video(next(), untaggedTitles[i%len(untaggedTitles)])
		c.Add(key, v)
		keep = append(keep, v.ID)
	}
	return keep
}
