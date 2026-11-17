package catalog

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

// SourcesFile is infra/youtube_sources.yaml: language code -> sources.
type SourcesFile map[string]struct {
	Queries  []SourceEntry `yaml:"queries"`
	Channels []SourceEntry `yaml:"channels"`
}

// SourceEntry is one query or channel. Channels use ID, queries Text.
type SourceEntry struct {
	Text  string `yaml:"text"`
	ID    string `yaml:"id"`
	Level *int   `yaml:"level"`
}

// LoadSourcesFile reads and parses path.
func LoadSourcesFile(path string) (SourcesFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f SourcesFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// ApplySources upserts every source in f and disables the ones not in it.
// It returns how many sources are enabled.
func ApplySources(ctx context.Context, db *pgxpool.Pool, f SourcesFile) (int, error) {
	n := 0
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE youtube_sources SET enabled = false`); err != nil {
			return err
		}
		upsert := func(lang, kind, value string, level *int) error {
			if value == "" {
				return fmt.Errorf("%s: empty %s", lang, kind)
			}
			if level != nil && (*level < 1 || *level > 6) {
				return fmt.Errorf("%s %q: level must be 1 to 6", lang, value)
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO youtube_sources (language, kind, value, level_hint, enabled) VALUES ($1, $2, $3, $4, true)
				ON CONFLICT (language, kind, value) DO UPDATE SET level_hint = EXCLUDED.level_hint, enabled = true`,
				lang, kind, value, level)
			if err != nil {
				return fmt.Errorf("%s %q: %w", lang, value, err)
			}
			n++
			return nil
		}
		for lang, s := range f {
			for _, q := range s.Queries {
				if err := upsert(lang, "query", q.Text, q.Level); err != nil {
					return err
				}
			}
			for _, c := range s.Channels {
				if err := upsert(lang, "channel", c.ID, c.Level); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return n, err
}
