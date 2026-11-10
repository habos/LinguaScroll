package feed

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// RefreshStats precomputes, for every live clip, the language-wide
// percentiles of YouTube views and likes per view, and the base score used
// to pick candidates. It only writes views_pct, likes_pct and base_score,
// so it never races the events rollup, which owns the counters.
func RefreshStats(ctx context.Context, db *pgxpool.Pool, now time.Time) (int64, error) {
	cfg, err := LoadConfig(ctx, db)
	if err != nil {
		return 0, err
	}
	w := cfg.Weights
	// Mirrors BaseScore: completion, views/likes percentiles, freshness and
	// hide rate. TestBaseScoreSQLMatchesGo keeps the two in step.
	tag, err := db.Exec(ctx, `
		WITH pct AS (
			SELECT v.id,
			       percent_rank() OVER (PARTITION BY v.language ORDER BY v.view_count) AS views_pct,
			       percent_rank() OVER (PARTITION BY v.language
			                            ORDER BY v.like_count::float8 / greatest(v.view_count, 1)) AS likes_pct,
			       (coalesce(s.completions, 0) + 2)::float8 / (coalesce(s.views, 0) + 4) AS completion,
			       least(1, coalesce(s.hides, 0)::float8 / (coalesce(s.impressions, 0) + 5)) AS hide,
			       CASE WHEN v.published_at IS NULL THEN 0
			            WHEN v.published_at >= $1::timestamptz - interval '30 days' THEN 1
			            WHEN v.published_at <= $1::timestamptz - interval '365 days' THEN 0
			            ELSE 1 - (extract(epoch FROM $1::timestamptz - v.published_at)::float8 / 86400 - 30) / 335
			       END AS fresh
			  FROM videos v
			  LEFT JOIN video_stats s ON s.video_id = v.id
			 WHERE v.status = 'live'
		)
		INSERT INTO video_stats AS s (video_id, views_pct, likes_pct, base_score, updated_at)
		SELECT id, views_pct, likes_pct,
		       $2 * completion + $3 * views_pct + $4 * likes_pct + $5 * fresh + $6 * hide, now()
		  FROM pct
		ON CONFLICT (video_id) DO UPDATE SET
		       views_pct  = excluded.views_pct,
		       likes_pct  = excluded.likes_pct,
		       base_score = excluded.base_score,
		       updated_at = now()`,
		now, w.Completion, w.ViewsPct, w.LikesPct, w.Fresh, w.Hide)
	if err != nil {
		return 0, fmt.Errorf("refresh feed stats: %w", err)
	}
	return tag.RowsAffected(), nil
}

// StatsArgs is the periodic feed_stats job.
type StatsArgs struct{}

// Kind implements river.JobArgs.
func (StatsArgs) Kind() string { return "feed_stats" }

// StatsInterval is how often feed_stats runs.
const StatsInterval = 10 * time.Minute

// StatsWorker runs RefreshStats.
type StatsWorker struct {
	river.WorkerDefaults[StatsArgs]
	DB *pgxpool.Pool
}

// Work implements river.Worker.
func (w *StatsWorker) Work(ctx context.Context, _ *river.Job[StatsArgs]) error {
	_, err := RefreshStats(ctx, w.DB, time.Now())
	return err
}
