package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestGetFeedHTTP(t *testing.T) {
	ts, token := newServer(t, 1000)
	for _, c := range []struct {
		name, path string
		status     int
		code       string
	}{
		{"unknown language", "/v1/feed?language=xx", 400, "unknown_language"},
		{"bad cursor", "/v1/feed?language=es&cursor=nope", 400, "bad_cursor"},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, e := do(t, "GET", ts.URL+c.path, token)
			if status != c.status || e.Error.Code != c.code {
				t.Fatalf("got %d %q, want %d %q", status, e.Error.Code, c.status, c.code)
			}
		})
	}

	// Two pages over the seed's 10 Spanish clips: all served once.
	seen := map[string]bool{}
	cursor := ""
	for page := range 2 {
		req, _ := http.NewRequest("GET", ts.URL+"/v1/feed?language=es&cursor="+cursor, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Items []struct {
				ID      string `json:"id"`
				Source  string `json:"source"`
				Youtube struct {
					VideoID string `json:"video_id"`
				} `json:"youtube"`
			} `json:"items"`
			NextCursor *string `json:"next_cursor"`
		}
		err = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("page %d: status %d, %v", page, res.StatusCode, err)
		}
		for _, it := range body.Items {
			if seen[it.ID] || it.Source != "youtube" || it.Youtube.VideoID == "" {
				t.Fatalf("page %d: bad or repeated item %+v", page, it)
			}
			seen[it.ID] = true
		}
		if page == 0 && (len(body.Items) != 10 || body.NextCursor == nil) {
			t.Fatalf("first page: %d items, next %v", len(body.Items), body.NextCursor)
		}
		if page == 1 {
			if len(body.Items) != 0 || body.NextCursor != nil {
				t.Fatalf("second page: %d items, next %v; want the end of the feed", len(body.Items), body.NextCursor)
			}
		}
		if body.NextCursor != nil {
			cursor = *body.NextCursor
		}
	}
}
