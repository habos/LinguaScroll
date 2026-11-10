package httpx

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterRefills(t *testing.T) {
	l := NewRateLimiter(60) // one a second, burst 60
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	for range 60 {
		if !l.Allow("k") {
			t.Fatal("burst denied")
		}
	}
	if l.Allow("k") {
		t.Fatal("61st allowed")
	}
	if !l.Allow("other") {
		t.Fatal("keys share a bucket")
	}
	now = now.Add(time.Second)
	if !l.Allow("k") {
		t.Fatal("no refill after a second")
	}
}

func TestBearer(t *testing.T) {
	for _, c := range []struct {
		header, token string
		ok            bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"Bearer ", "", false},
		{"Basic abc", "", false},
		{"", "", false},
	} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if c.header != "" {
			r.Header.Set("Authorization", c.header)
		}
		got, ok := bearer(r)
		if got != c.token || ok != c.ok {
			t.Errorf("%q: got (%q, %v), want (%q, %v)", c.header, got, ok, c.token, c.ok)
		}
	}
}

func TestRecoverWritesErrorShape(t *testing.T) {
	h := Recover(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != 500 || w.Body.String() != "{\"error\":{\"code\":\"internal\",\"message\":\"internal error\"}}\n" {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
