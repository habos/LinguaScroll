// Package youtube is the YouTube Data API v3 client behind the
// YouTubeClient interface. Only IDs and metadata are read; nothing is
// downloaded. See the linguascroll-youtube-rules skill.
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Quota costs in units (default budget 10,000 a day).
const (
	SearchCost = 100 // search.list
	VideosCost = 1   // videos.list, up to 50 IDs
	MaxIDs     = 50
)

// Client is what the catalog needs from YouTube.
type Client interface {
	// Search runs search.list (100 units) and returns video IDs.
	Search(ctx context.Context, q SearchQuery) (ids []string, nextPage string, err error)
	// Videos runs videos.list (1 unit per call of up to 50 IDs). IDs that no
	// longer exist are simply missing from the result.
	Videos(ctx context.Context, ids []string) ([]Video, error)
}

// SearchQuery is one search.list call. Query or ChannelID is set.
type SearchQuery struct {
	Query, ChannelID, RelevanceLanguage, PageToken string
	PublishedAfter, PublishedBefore                time.Time
}

// Video is the metadata v0 stores for a clip.
type Video struct {
	ID, Title, Description, ChannelID, ChannelTitle, CategoryID string
	Tags                                                        []string
	DefaultAudioLanguage, DefaultLanguage                       string
	DurationMs                                                  int
	PublishedAt                                                 time.Time
	ViewCount, LikeCount                                        int64
	Embeddable, MadeForKids, AgeRestricted, HasCaptions         bool
	Public                                                      bool
	EmbedWidth, EmbedHeight                                     int // part=player with maxWidth=1080
}

// ErrQuotaExceeded is returned when YouTube says the daily quota is spent.
var ErrQuotaExceeded = errors.New("youtube: quota exceeded")

// HTTPClient calls the real API with an API key.
type HTTPClient struct {
	Key     string
	BaseURL string // default https://www.googleapis.com/youtube/v3
	HTTP    *http.Client
}

func (c *HTTPClient) base() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "https://www.googleapis.com/youtube/v3"
}

func (c *HTTPClient) get(ctx context.Context, path string, q url.Values, out any) error {
	q.Set("key", c.Key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
				Errors  []struct {
					Reason string `json:"reason"`
				} `json:"errors"`
			} `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		for _, r := range e.Error.Errors {
			if r.Reason == "quotaExceeded" || r.Reason == "dailyLimitExceeded" {
				return ErrQuotaExceeded
			}
		}
		return fmt.Errorf("youtube %s: %d %s", path, res.StatusCode, e.Error.Message)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Search implements Client.
func (c *HTTPClient) Search(ctx context.Context, sq SearchQuery) ([]string, string, error) {
	q := url.Values{
		"part": {"id"}, "type": {"video"}, "videoDuration": {"short"}, "videoEmbeddable": {"true"},
		"safeSearch": {"strict"}, "maxResults": {"50"},
	}
	if sq.Query != "" {
		q.Set("q", sq.Query)
	}
	if sq.ChannelID != "" {
		q.Set("channelId", sq.ChannelID)
	}
	if sq.RelevanceLanguage != "" {
		q.Set("relevanceLanguage", sq.RelevanceLanguage)
	}
	if sq.PageToken != "" {
		q.Set("pageToken", sq.PageToken)
	}
	if !sq.PublishedAfter.IsZero() {
		q.Set("publishedAfter", sq.PublishedAfter.UTC().Format(time.RFC3339))
	}
	if !sq.PublishedBefore.IsZero() {
		q.Set("publishedBefore", sq.PublishedBefore.UTC().Format(time.RFC3339))
	}
	var out struct {
		NextPageToken string `json:"nextPageToken"`
		Items         []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
		} `json:"items"`
	}
	if err := c.get(ctx, "/search", q, &out); err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		if it.ID.VideoID != "" {
			ids = append(ids, it.ID.VideoID)
		}
	}
	return ids, out.NextPageToken, nil
}

// Videos implements Client.
func (c *HTTPClient) Videos(ctx context.Context, ids []string) ([]Video, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > MaxIDs {
		return nil, fmt.Errorf("youtube: at most %d IDs per videos.list", MaxIDs)
	}
	q := url.Values{
		"part":     {"snippet,contentDetails,statistics,status,player"},
		"id":       {strings.Join(ids, ",")},
		"maxWidth": {"1080"},
	}
	var out struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title                string    `json:"title"`
				Description          string    `json:"description"`
				ChannelID            string    `json:"channelId"`
				ChannelTitle         string    `json:"channelTitle"`
				CategoryID           string    `json:"categoryId"`
				Tags                 []string  `json:"tags"`
				DefaultAudioLanguage string    `json:"defaultAudioLanguage"`
				DefaultLanguage      string    `json:"defaultLanguage"`
				PublishedAt          time.Time `json:"publishedAt"`
			} `json:"snippet"`
			ContentDetails struct {
				Duration      string `json:"duration"`
				Caption       string `json:"caption"`
				ContentRating struct {
					YtRating string `json:"ytRating"`
				} `json:"contentRating"`
			} `json:"contentDetails"`
			Statistics struct {
				ViewCount string `json:"viewCount"`
				LikeCount string `json:"likeCount"`
			} `json:"statistics"`
			Status struct {
				Embeddable    bool   `json:"embeddable"`
				MadeForKids   bool   `json:"madeForKids"`
				PrivacyStatus string `json:"privacyStatus"`
			} `json:"status"`
			Player struct {
				EmbedWidth  json.Number `json:"embedWidth"`
				EmbedHeight json.Number `json:"embedHeight"`
			} `json:"player"`
		} `json:"items"`
	}
	if err := c.get(ctx, "/videos", q, &out); err != nil {
		return nil, err
	}
	vs := make([]Video, 0, len(out.Items))
	for _, it := range out.Items {
		dur, _ := ParseDuration(it.ContentDetails.Duration)
		views, _ := strconv.ParseInt(it.Statistics.ViewCount, 10, 64)
		likes, _ := strconv.ParseInt(it.Statistics.LikeCount, 10, 64)
		w, _ := it.Player.EmbedWidth.Int64()
		h, _ := it.Player.EmbedHeight.Int64()
		vs = append(vs, Video{
			ID: it.ID, Title: it.Snippet.Title, Description: it.Snippet.Description,
			ChannelID: it.Snippet.ChannelID, ChannelTitle: it.Snippet.ChannelTitle, CategoryID: it.Snippet.CategoryID,
			Tags: it.Snippet.Tags, DefaultAudioLanguage: it.Snippet.DefaultAudioLanguage, DefaultLanguage: it.Snippet.DefaultLanguage,
			DurationMs: int(dur.Milliseconds()), PublishedAt: it.Snippet.PublishedAt, ViewCount: views, LikeCount: likes,
			Embeddable: it.Status.Embeddable, MadeForKids: it.Status.MadeForKids,
			AgeRestricted: it.ContentDetails.ContentRating.YtRating == "ytAgeRestricted",
			HasCaptions:   it.ContentDetails.Caption == "true",
			Public:        it.Status.PrivacyStatus == "public" || it.Status.PrivacyStatus == "unlisted",
			EmbedWidth:    int(w), EmbedHeight: int(h),
		})
	}
	return vs, nil
}

var isoDuration = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// ParseDuration parses YouTube's ISO 8601 durations such as PT1M5S.
func ParseDuration(s string) (time.Duration, error) {
	m := isoDuration.FindStringSubmatch(s)
	if m == nil || s == "P" || s == "PT" {
		return 0, fmt.Errorf("youtube: bad duration %q", s)
	}
	units := []time.Duration{24 * time.Hour, time.Hour, time.Minute, time.Second}
	var d time.Duration
	for i, u := range units {
		if m[i+1] != "" {
			n, _ := strconv.Atoi(m[i+1])
			d += time.Duration(n) * u
		}
	}
	return d, nil
}
