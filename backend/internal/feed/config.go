// Package feed ranks YouTube clips for GET /v1/feed (module V4). The rules
// live in .claude/skills/linguascroll-feed-ranking; every tunable number is
// read from app_config, and the defaults here only cover a missing key.
package feed

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Weights are the score coefficients (app_config key feed.weights). Hide is
// stored negative, so score adds Hide*h.
type Weights struct {
	Completion float64 `json:"completion"`
	ViewsPct   float64 `json:"views_pct"`
	LikesPct   float64 `json:"likes_pct"`
	Level      float64 `json:"level"`
	Channel    float64 `json:"channel"`
	Category   float64 `json:"category"`
	Fresh      float64 `json:"fresh"`
	Hide       float64 `json:"hide"`
	Noise      float64 `json:"noise"`
}

// Config is every feed.* key.
type Config struct {
	PageSize        int
	LevelWindow     int
	ExploreShare    float64
	ExploreMaxViews int
	ChannelSpacing  int
	MusicCap        float64
	Candidates      int
	Weights         Weights
}

// DefaultConfig mirrors seed/02_app_config.sql.
func DefaultConfig() Config {
	return Config{
		PageSize:        10,
		LevelWindow:     1,
		ExploreShare:    0.15,
		ExploreMaxViews: 50,
		ChannelSpacing:  5,
		MusicCap:        0.2,
		Candidates:      400,
		Weights: Weights{
			Completion: 0.25, ViewsPct: 0.15, LikesPct: 0.15, Level: 0.10,
			Channel: 0.15, Category: 0.10, Fresh: 0.05, Hide: -0.30, Noise: 0.05,
		},
	}
}

// Querier is the part of pgx used here (pool or tx).
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// LoadConfig reads feed.* from app_config over the defaults.
func LoadConfig(ctx context.Context, q Querier) (Config, error) {
	c := DefaultConfig()
	rows, err := q.Query(ctx, `SELECT key, value FROM app_config WHERE key LIKE 'feed.%'`)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	targets := map[string]any{
		"feed.page_size":         &c.PageSize,
		"feed.level_window":      &c.LevelWindow,
		"feed.explore_share":     &c.ExploreShare,
		"feed.explore_max_views": &c.ExploreMaxViews,
		"feed.channel_spacing":   &c.ChannelSpacing,
		"feed.music_cap":         &c.MusicCap,
		"feed.candidates":        &c.Candidates,
		"feed.weights":           &c.Weights,
	}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return c, err
		}
		if t, ok := targets[key]; ok {
			if err := json.Unmarshal(raw, t); err != nil {
				return c, fmt.Errorf("app_config %s: %w", key, err)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return c, err
	}
	if c.PageSize < 1 || c.PageSize > 50 {
		return c, fmt.Errorf("app_config feed.page_size: %d out of range 1..50", c.PageSize)
	}
	if c.ChannelSpacing < 1 {
		c.ChannelSpacing = 1
	}
	return c, nil
}
