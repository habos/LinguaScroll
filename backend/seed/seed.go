// Package seed applies the idempotent dev and reference data in this
// directory, in file-name order.
package seed

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var files embed.FS

// Apply runs every seed file in one transaction. Safe to run repeatedly.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	for _, n := range names {
		b, err := files.ReadFile(n)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(b)); err != nil {
			return fmt.Errorf("seed %s: %w", n, err)
		}
	}
	return tx.Commit(ctx)
}
