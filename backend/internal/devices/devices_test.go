package devices_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/habos/linguascroll/backend/internal/api"
	"github.com/habos/linguascroll/backend/internal/devices"
	"github.com/habos/linguascroll/backend/internal/platform/auth"
	"github.com/habos/linguascroll/backend/internal/platform/dbtest"
	"github.com/habos/linguascroll/backend/internal/platform/httpx"
	"github.com/habos/linguascroll/backend/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"
)

type client struct {
	t     *testing.T
	base  string
	token string
}

func setup(t *testing.T) (*client, *pgxpool.Pool) {
	t.Helper()
	pool := dbtest.New(t, true)
	h := server.New(server.Deps{
		DB: pool, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Tokens: auth.Store{DB: pool}, RateLimitPerMinute: 10000,
	})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return &client{t: t, base: ts.URL}, pool
}

// call sends body as JSON and decodes a 2xx answer into out; it returns the
// status and, for errors, the error code.
func (c *client) call(method, path string, body, out any) (int, string) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		var e httpx.Error
		_ = json.NewDecoder(res.Body).Decode(&e)
		return res.StatusCode, e.Error.Code
	}
	if out != nil && res.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			c.t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return res.StatusCode, ""
}

func registration(installID string, birthYear int) api.AnonymousSessionRequest {
	return api.AnonymousSessionRequest{
		InstallId: installID, BirthYear: birthYear, UiLanguage: "en", NativeLanguage: "en",
		Learning: []api.LearningLanguageInput{{Language: "es", Level: 1}},
	}
}

func (c *client) register(installID string) api.SessionResponse {
	c.t.Helper()
	var s api.SessionResponse
	if status, code := c.call("POST", "/v1/sessions/anonymous", registration(installID, 2000), &s); status != 201 {
		c.t.Fatalf("register: %d %s", status, code)
	}
	c.token = s.Token
	return s
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAgeBand(t *testing.T) {
	for _, c := range []struct {
		birth int
		band  string
		ok    bool
	}{
		{2016, "", false}, {2014, "", false}, {2013, "teen", true}, {2009, "teen", true}, {2008, "adult", true}, {1950, "adult", true},
	} {
		band, ok := devices.AgeBand(c.birth, 2026)
		if band != c.band || ok != c.ok {
			t.Errorf("AgeBand(%d) = %q, %v; want %q, %v", c.birth, band, ok, c.band, c.ok)
		}
	}
}

func TestUnder13IsBlockedAndNothingStored(t *testing.T) {
	c, pool := setup(t)
	status, code := c.call("POST", "/v1/sessions/anonymous", registration("install-under-13-0001", 2016), nil)
	if status != 403 || code != "age_blocked" {
		t.Fatalf("got %d %s, want 403 age_blocked", status, code)
	}
	if n := count(t, pool, `SELECT count(*) FROM users`); n != 0 {
		t.Fatalf("%d users stored", n)
	}
}

func TestRegisterTwiceReturnsSameUser(t *testing.T) {
	c, pool := setup(t)
	first := c.register("install-same-000001")
	if first.User.AgeBand != api.Adult || len(first.User.Learning) != 1 || !first.User.Learning[0].IsActive {
		t.Fatalf("unexpected user: %+v", first.User)
	}
	// The app lost its token: registering again, even with other answers,
	// gives the same user and keeps the stored languages.
	again := registration("install-same-000001", 2000)
	again.Learning = []api.LearningLanguageInput{{Language: "fr", Level: 5}}
	var second api.SessionResponse
	if status, code := c.call("POST", "/v1/sessions/anonymous", again, &second); status != 201 {
		t.Fatalf("second register: %d %s", status, code)
	}
	if second.User.Id != first.User.Id {
		t.Fatalf("user changed: %s -> %s", first.User.Id, second.User.Id)
	}
	if second.Token == first.Token {
		t.Fatal("token not renewed")
	}
	if len(second.User.Learning) != 1 || second.User.Learning[0].Language != "es" {
		t.Fatalf("languages changed: %+v", second.User.Learning)
	}
	if n := count(t, pool, `SELECT count(*) FROM users`); n != 1 {
		t.Fatalf("%d users", n)
	}
	// Both tokens work.
	c.token = first.Token
	if status, _ := c.call("GET", "/v1/me", nil, nil); status != 200 {
		t.Fatalf("old token: %d", status)
	}
}

func TestBirthYearIsNotStored(t *testing.T) {
	_, pool := setup(t)
	if n := count(t, pool, `SELECT count(*) FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'birth_year'`); n != 0 {
		t.Fatal("users has a birth_year column")
	}
}

func TestRegisterValidation(t *testing.T) {
	c, _ := setup(t)
	bad := func(name string, mutate func(*api.AnonymousSessionRequest), wantStatus int, wantCode string) {
		t.Helper()
		r := registration("install-validation-01", 2000)
		mutate(&r)
		if status, code := c.call("POST", "/v1/sessions/anonymous", r, nil); status != wantStatus || code != wantCode {
			t.Errorf("%s: got %d %s, want %d %s", name, status, code, wantStatus, wantCode)
		}
	}
	bad("unknown learning language", func(r *api.AnonymousSessionRequest) { r.Learning[0].Language = "xx" }, 422, "unknown_language")
	bad("unknown native language", func(r *api.AnonymousSessionRequest) { r.NativeLanguage = "xx" }, 422, "unknown_language")
	bad("level 7", func(r *api.AnonymousSessionRequest) { r.Learning[0].Level = 7 }, 422, "invalid_request")
	bad("no languages", func(r *api.AnonymousSessionRequest) { r.Learning = nil }, 422, "invalid_request")
	bad("future birth year", func(r *api.AnonymousSessionRequest) { r.BirthYear = 3000 }, 422, "invalid_request")
	bad("short install id", func(r *api.AnonymousSessionRequest) { r.InstallId = "short" }, 422, "invalid_request")
	bad("duplicate language", func(r *api.AnonymousSessionRequest) {
		r.Learning = append(r.Learning, api.LearningLanguageInput{Language: "es", Level: 3})
	}, 422, "invalid_request")
}

func TestLanguages(t *testing.T) {
	c, _ := setup(t)
	c.register("install-languages-01")

	// Add Japanese at level 3, then remove Spanish: one active language left.
	var u api.User
	if status, code := c.call("PUT", "/v1/me/languages/ja", api.LearningLanguagePut{Level: 3}, &u); status != 200 {
		t.Fatalf("add ja: %d %s", status, code)
	}
	if len(u.Learning) != 2 || u.Learning[0].Language != "es" || !u.Learning[0].IsActive || u.Learning[1].IsActive {
		t.Fatalf("after adding ja: %+v", u.Learning)
	}
	if status, code := c.call("DELETE", "/v1/me/languages/es", nil, &u); status != 200 {
		t.Fatalf("remove es: %d %s", status, code)
	}
	if len(u.Learning) != 1 || u.Learning[0].Language != "ja" || u.Learning[0].Level != 3 || !u.Learning[0].IsActive {
		t.Fatalf("after removing es: %+v", u.Learning)
	}

	// Removing the last language is refused.
	if status, code := c.call("DELETE", "/v1/me/languages/ja", nil, nil); status != 422 || code != "last_language" {
		t.Fatalf("remove last: %d %s", status, code)
	}
	// Removing one you don't learn is 404.
	if status, code := c.call("DELETE", "/v1/me/languages/fr", nil, nil); status != 404 || code != "not_found" {
		t.Fatalf("remove unknown: %d %s", status, code)
	}

	// Switch the active language and change a level.
	yes := true
	if status, code := c.call("PUT", "/v1/me/languages/fr", api.LearningLanguagePut{Level: 5, IsActive: &yes}, &u); status != 200 {
		t.Fatalf("add fr active: %d %s", status, code)
	}
	if u.Learning[0].Language != "fr" || !u.Learning[0].IsActive || u.Learning[1].IsActive {
		t.Fatalf("fr not the only active: %+v", u.Learning)
	}
	if status, _ := c.call("PUT", "/v1/me/languages/ja", api.LearningLanguagePut{Level: 4}, &u); status != 200 {
		t.Fatal("change level")
	}
	for _, l := range u.Learning {
		if l.Language == "ja" && (l.Level != 4 || l.IsActive) {
			t.Fatalf("ja after level change: %+v", l)
		}
	}
	if status, code := c.call("PUT", "/v1/me/languages/xx", api.LearningLanguagePut{Level: 1}, nil); status != 422 || code != "unknown_language" {
		t.Fatalf("unknown language: %d %s", status, code)
	}
	if status, code := c.call("PUT", "/v1/me/languages/ja", api.LearningLanguagePut{Level: 0}, nil); status != 422 || code != "invalid_request" {
		t.Fatalf("level 0: %d %s", status, code)
	}
}

func TestUpdateMe(t *testing.T) {
	c, _ := setup(t)
	c.register("install-update-me-01")
	es, xx := "es", "xx"
	var u api.User
	if status, _ := c.call("PATCH", "/v1/me", api.UserPatch{NativeLanguage: &es}, &u); status != 200 || u.NativeLanguage != "es" || u.UiLanguage != "en" {
		t.Fatalf("patch native: %d %+v", status, u)
	}
	if status, code := c.call("PATCH", "/v1/me", api.UserPatch{UiLanguage: &xx}, nil); status != 422 || code != "unknown_language" {
		t.Fatalf("patch unknown ui: %d %s", status, code)
	}
}

func TestDeleteMe(t *testing.T) {
	c, pool := setup(t)
	s := c.register("install-delete-me-01")
	ctx := context.Background()
	var video string
	if err := pool.QueryRow(ctx, `SELECT id FROM videos LIMIT 1`).Scan(&video); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO likes (user_id, video_id) VALUES ($1, $2)`, s.User.Id, video); err != nil {
		t.Fatal(err)
	}
	if status, code := c.call("DELETE", "/v1/me", nil, nil); status != 204 {
		t.Fatalf("delete: %d %s", status, code)
	}
	if status, code := c.call("GET", "/v1/me", nil, nil); status != 401 || code != "unauthorized" {
		t.Fatalf("old token after delete: %d %s", status, code)
	}
	if n := count(t, pool, `SELECT count(*) FROM likes`); n != 0 {
		t.Fatalf("%d likes left", n)
	}
	// The same install can start fresh.
	fresh := c.register("install-delete-me-01")
	if fresh.User.Id == s.User.Id {
		t.Fatal("deleted user came back")
	}
}

func TestPublicEndpoints(t *testing.T) {
	c, _ := setup(t)
	var cfg api.AppConfig
	if status, _ := c.call("GET", "/v1/config", nil, &cfg); status != 200 || cfg.FeedPageSize != 10 || !cfg.YoutubeCaptionsDefault {
		t.Fatalf("config: %d %+v", status, cfg)
	}
	var langs api.LanguageList
	if status, _ := c.call("GET", "/v1/languages", nil, &langs); status != 200 {
		t.Fatalf("languages: %d", status)
	}
	learnable := 0
	for _, l := range langs.Items {
		if l.IsLearnable {
			learnable++
		}
	}
	if learnable != 15 || langs.Items[0].Code != "en" {
		t.Fatalf("languages: %d learnable, first %q", learnable, langs.Items[0].Code)
	}
}
