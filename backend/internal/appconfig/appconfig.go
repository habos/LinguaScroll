// Package appconfig reads runtime config from the app_config table.
// Values are JSON; tuning happens in the table, not in Go constants.
package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store reads app_config. It does no caching; reads are cheap and the
// table is tiny.
type Store struct{ DB *pgxpool.Pool }

// Get decodes key into a T, or returns def when the key is missing.
func Get[T any](ctx context.Context, s Store, key string, def T) (T, error) {
	var raw []byte
	err := s.DB.QueryRow(ctx, `SELECT value FROM app_config WHERE key = $1`, key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return def, fmt.Errorf("app_config %s: %w", key, err)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return def, fmt.Errorf("app_config %s: %w", key, err)
	}
	return v, nil
}

// Set stores v as key's JSON value.
func Set(ctx context.Context, s Store, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `
		INSERT INTO app_config (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, raw)
	return err
}
