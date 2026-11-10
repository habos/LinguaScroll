package youtube_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/habos/linguascroll/backend/internal/youtube"
)

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"PT59S": 59 * time.Second, "PT1M5S": 65 * time.Second, "PT3M": 3 * time.Minute,
		"PT1H2M3S": time.Hour + 2*time.Minute + 3*time.Second, "P1DT1S": 24*time.Hour + time.Second,
	} {
		got, err := youtube.ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("%s: %v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "P", "PT", "1M", "PT1X"} {
		if _, err := youtube.ParseDuration(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestHTTPClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("key") != "k" {
			t.Errorf("no key")
		}
		switch r.URL.Path {
		case "/search":
			for k, v := range map[string]string{"type": "video", "videoDuration": "short", "videoEmbeddable": "true", "safeSearch": "strict", "relevanceLanguage": "es", "q": "hola"} {
				if q.Get(k) != v {
					t.Errorf("search %s = %q, want %q", k, q.Get(k), v)
				}
			}
			_, _ = w.Write([]byte(`{"nextPageToken":"n","items":[{"id":{"videoId":"a"}},{"id":{"videoId":"b"}}]}`))
		case "/videos":
			if q.Get("id") != "a,b" || q.Get("maxWidth") != "1080" {
				t.Errorf("videos query %v", q)
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"a",
				"snippet":{"title":"T","channelId":"UC1","channelTitle":"C","categoryId":"27","tags":["x"],"defaultAudioLanguage":"es-419","publishedAt":"2026-01-02T03:04:05Z"},
				"contentDetails":{"duration":"PT45S","caption":"true","contentRating":{"ytRating":"ytAgeRestricted"}},
				"statistics":{"viewCount":"1234","likeCount":"56"},
				"status":{"embeddable":true,"madeForKids":false,"privacyStatus":"public"},
				"player":{"embedWidth":"608","embedHeight":1080}}]}`))
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"message":"quota","errors":[{"reason":"quotaExceeded"}]}}`))
		}
	}))
	defer ts.Close()
	c := &youtube.HTTPClient{Key: "k", BaseURL: ts.URL}
	ctx := context.Background()

	ids, next, err := c.Search(ctx, youtube.SearchQuery{Query: "hola", RelevanceLanguage: "es"})
	if err != nil || len(ids) != 2 || next != "n" {
		t.Fatalf("search: %v %q %v", ids, next, err)
	}
	vs, err := c.Videos(ctx, []string{"a", "b"})
	if err != nil || len(vs) != 1 {
		t.Fatalf("videos: %v %v", vs, err)
	}
	v := vs[0]
	if v.DurationMs != 45_000 || !v.HasCaptions || !v.AgeRestricted || v.ViewCount != 1234 || v.LikeCount != 56 ||
		v.EmbedWidth != 608 || v.EmbedHeight != 1080 || !v.Public || v.DefaultAudioLanguage != "es-419" || v.Tags[0] != "x" {
		t.Fatalf("parsed %+v", v)
	}

	c.BaseURL = ts.URL + "/quota"
	if _, _, err := c.Search(ctx, youtube.SearchQuery{Query: "x"}); !errors.Is(err, youtube.ErrQuotaExceeded) {
		t.Fatalf("quota error = %v", err)
	}
}
