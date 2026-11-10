// Package httpx has the HTTP plumbing shared by every module: the error
// shape, request IDs, panic recovery, access logs, bearer auth and the
// per-token rate limit.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/habos/linguascroll/backend/internal/platform/auth"
	"golang.org/x/time/rate"
)

// Error is the body of every error response:
// {"error": {"code": "snake_case", "message": "..."}}.
type Error struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody is the inner object of Error.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteError writes status and the standard error body.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Error{Error: ErrorBody{Code: code, Message: message}})
}

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies mws so the first one is outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

type requestIDKey struct{}

// RequestIDFrom returns the request's ID, if set.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// RequestID reuses a sane incoming X-Request-ID or makes one, and echoes it.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

// Recover turns a panic into a logged 500 in the error shape.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler { //nolint:errorlint // sentinel compared by identity
						panic(v)
					}
					log.ErrorContext(r.Context(), "panic", "panic", v, "request_id", RequestIDFrom(r.Context()), "stack", string(debug.Stack()))
					WriteError(w, http.StatusInternalServerError, "internal", "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// AccessLog logs one JSON line per request.
func AccessLog(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			log.InfoContext(r.Context(), "request",
				"method", r.Method, "path", r.URL.Path, "status", sw.status,
				"duration_ms", time.Since(start).Milliseconds(), "request_id", RequestIDFrom(r.Context()))
		})
	}
}

// TokenLookup resolves a bearer token to a user.
type TokenLookup interface {
	Lookup(ctx context.Context, token string) (auth.CurrentUser, error)
}

// PublicRoute reports whether a request needs no token.
type PublicRoute func(r *http.Request) bool

// Auth requires a valid bearer token on every non-public route and puts
// the user in the request context.
func Auth(tokens TokenLookup, public PublicRoute, log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if public(r) {
				next.ServeHTTP(w, r)
				return
			}
			token, ok := bearer(r)
			if !ok {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
				return
			}
			u, err := tokens.Lookup(r.Context(), token)
			if errors.Is(err, auth.ErrInvalidToken) {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
				return
			}
			if err != nil {
				log.ErrorContext(r.Context(), "token lookup", "err", err, "request_id", RequestIDFrom(r.Context()))
				WriteError(w, http.StatusInternalServerError, "internal", "internal error")
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
		})
	}
}

func bearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) <= len(p) || !strings.EqualFold(h[:len(p)], p) {
		return "", false
	}
	return strings.TrimSpace(h[len(p):]), true
}

// RateLimiter allows perMinute requests a minute per key (the bearer token,
// or the client address for requests without one), with a burst of the same.
type RateLimiter struct {
	perMinute int
	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
	now       func() time.Time
}

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

// NewRateLimiter returns a limiter of perMinute requests a minute per key.
func NewRateLimiter(perMinute int) *RateLimiter {
	return &RateLimiter{perMinute: perMinute, buckets: map[string]*bucket{}, now: time.Now}
}

// Allow reports whether key may make another request now.
func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastSweep) > 5*time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.seen) > 5*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(rate.Limit(float64(l.perMinute)/60), l.perMinute)}
		l.buckets[key] = b
	}
	b.seen = now
	return b.lim.AllowN(now, 1)
}

// Middleware rejects requests over the limit with 429.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := bearer(r)
		if ok {
			key = "t:" + key
		} else {
			key = "a:" + clientAddr(r)
		}
		if !l.Allow(key) {
			w.Header().Set("Retry-After", "1")
			WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientAddr(r *http.Request) string {
	addr := r.RemoteAddr
	if i := strings.LastIndexByte(addr, ':'); i > 0 {
		return addr[:i]
	}
	return addr
}

// APIError is an error a handler returns to answer with a specific status
// and error code. The server's response error handler writes it in the
// standard shape; any other error becomes a logged 500.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// E returns an *APIError.
func E(status int, code, message string) error {
	return &APIError{Status: status, Code: code, Message: message}
}
