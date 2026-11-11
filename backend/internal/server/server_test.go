package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/habos/linguascroll/backend/internal/platform/auth"
	"github.com/habos/linguascroll/backend/internal/platform/dbtest"
	"github.com/habos/linguascroll/backend/internal/platform/httpx"
	"github.com/habos/linguascroll/backend/internal/server"
)

func newServer(t *testing.T, perMinute int) (*httptest.Server, string) {
	t.Helper()
	pool := dbtest.New(t, true)
	ctx := context.Background()
	uid := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, install_id, age_band, ui_language, native_language) VALUES ($1, $2, 'adult', 'en', 'en')`, uid, uid.String()); err != nil {
		t.Fatal(err)
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, now() + interval '1 day')`, hash, uid); err != nil {
		t.Fatal(err)
	}
	// An expired session for the same user.
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, now() - interval '1 second')`, auth.HashToken("expired"), uid); err != nil {
		t.Fatal(err)
	}
	h := server.New(server.Deps{
		DB: pool, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Tokens: auth.Store{DB: pool}, RateLimitPerMinute: perMinute,
	})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, token
}

func do(t *testing.T, method, url, token string) (int, httpx.Error) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var e httpx.Error
	if res.StatusCode >= 400 {
		if ct := res.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s %s: content type %q", method, url, ct)
		}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			t.Errorf("%s %s: error body not JSON: %v", method, url, err)
		}
	}
	if res.Header.Get("X-Request-ID") == "" {
		t.Errorf("%s %s: no X-Request-ID", method, url)
	}
	return res.StatusCode, e
}

func TestRoutes(t *testing.T) {
	ts, token := newServer(t, 1000)
	cases := []struct {
		name, method, path, token string
		status                    int
		code                      string
	}{
		{"healthz is public", "GET", "/healthz", "", 200, ""},
		{"protected route without token", "GET", "/v1/feed?language=es", "", 401, "unauthorized"},
		{"protected route with bad token", "GET", "/v1/me", "nope", 401, "unauthorized"},
		{"expired token", "GET", "/v1/me", "expired", 401, "unauthorized"},
		{"valid token reaches a stub", "GET", "/v1/me/likes", token, 501, "not_implemented"},
		{"languages are public", "GET", "/v1/languages", "", 200, ""},
		{"config is public", "GET", "/v1/config", "", 200, ""},
		{"anonymous session is public", "POST", "/v1/sessions/anonymous", "", 400, "bad_request"},
		{"missing required query param", "GET", "/v1/feed", token, 400, "bad_request"},
		{"bad uuid path param", "PUT", "/v1/videos/not-a-uuid/like", token, 400, "bad_request"},
		{"unknown route", "GET", "/v1/nope", token, 404, "not_found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, e := do(t, c.method, ts.URL+c.path, c.token)
			if status != c.status {
				t.Fatalf("status = %d, want %d (%+v)", status, c.status, e)
			}
			if e.Error.Code != c.code {
				t.Errorf("code = %q, want %q", e.Error.Code, c.code)
			}
		})
	}
}

func TestRateLimit(t *testing.T) {
	ts, token := newServer(t, 3)
	for i := range 3 {
		if status, _ := do(t, "GET", ts.URL+"/v1/me/likes", token); status != 501 {
			t.Fatalf("request %d: status %d", i, status)
		}
	}
	status, e := do(t, "GET", ts.URL+"/v1/me/likes", token)
	if status != 429 || e.Error.Code != "rate_limited" {
		t.Fatalf("4th request: %d %+v, want 429 rate_limited", status, e)
	}
}

func TestRateLimitIgnoresMadeUpTokens(t *testing.T) {
	ts, _ := newServer(t, 2) // per address: 2 * AddressLimitFactor
	limit := 2 * server.AddressLimitFactor
	got429 := false
	for i := range limit + 1 {
		status, e := do(t, "GET", ts.URL+"/v1/me/likes", "made-up-"+strconv.Itoa(i))
		if status == 429 && e.Error.Code == "rate_limited" {
			got429 = true
		}
	}
	if !got429 {
		t.Fatalf("%d distinct bogus tokens from one address never hit 429", limit+1)
	}
}
