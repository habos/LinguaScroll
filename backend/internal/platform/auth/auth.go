// Package auth holds bearer-token helpers and the current user in a context.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TokenTTL is the sliding session lifetime.
const TokenTTL = 180 * 24 * time.Hour

// CurrentUser is the user a request's bearer token belongs to.
type CurrentUser struct {
	ID      uuid.UUID
	AgeBand string
}

type ctxKey struct{}

// WithUser returns ctx carrying u.
func WithUser(ctx context.Context, u CurrentUser) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom returns the current user, if the request was authenticated.
func UserFrom(ctx context.Context) (CurrentUser, bool) {
	u, ok := ctx.Value(ctxKey{}).(CurrentUser)
	return u, ok
}

// NewToken returns a fresh token (32 random bytes, base64url) and its hash.
func NewToken() (token string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken is how tokens are stored: SHA-256 of the token string.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// ErrInvalidToken means the token is unknown or expired.
var ErrInvalidToken = errors.New("invalid or expired token")

// Store looks tokens up in the sessions table.
type Store struct{ DB *pgxpool.Pool }

// Lookup resolves a token and slides its expiry (at most once an hour).
func (s Store) Lookup(ctx context.Context, token string) (CurrentUser, error) {
	var u CurrentUser
	err := s.DB.QueryRow(ctx, `
		WITH hit AS (
			UPDATE sessions
			   SET last_seen_at = now(), expires_at = now() + make_interval(secs => $2)
			 WHERE token_hash = $1 AND expires_at > now() AND last_seen_at < now() - interval '1 hour'
			RETURNING user_id
		)
		SELECT u.id, u.age_band
		  FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`,
		HashToken(token), TokenTTL.Seconds()).Scan(&u.ID, &u.AgeBand)
	if errors.Is(err, pgx.ErrNoRows) {
		return CurrentUser{}, ErrInvalidToken
	}
	return u, err
}
