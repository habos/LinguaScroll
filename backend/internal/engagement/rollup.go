package engagement

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// RollupLag keeps the rollup behind transactions that started before its
// cut-off but commit after it.
const RollupLag = time.Minute

// RollupInterval is how often the rollup job runs.
const RollupInterval = 5 * time.Minute

// RollupResult says what one run read.
type RollupResult struct {
	Events int
	Until  time.Time
}

// Rollup adds events received since the last run into video_stats and
// video_level_stats. Counters are only ever added to (the feed_stats job
// owns the other video_stats columns), likes and hides are recounted from
// their tables for clips whose likes or hides changed, and a view is a
// view_end with at least ViewMinMs watched. A viewer's level is their
// level for the clip's language at rollup time.
func (s *Service) Rollup(ctx context.Context) (RollupResult, error) {
	var r RollupResult
	err := pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		var from time.Time
		row := tx.QueryRow(ctx, `SELECT CASE WHEN done_until = '-infinity' THEN 'epoch'::timestamptz ELSE done_until END FROM rollup_state WHERE name = 'events' FOR UPDATE`)
		if err := row.Scan(&from); errors.Is(err, pgx.ErrNoRows) {
			from = time.Unix(0, 0)
			if _, err := tx.Exec(ctx, `INSERT INTO rollup_state (name, done_until) VALUES ('events', 'epoch')`); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		until := s.now().Add(-RollupLag)
		if !until.After(from) {
			r.Until = from
			return nil
		}
		r.Until = until
		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE rollup_ev ON COMMIT DROP AS
			SELECT e.video_id, e.user_id, e.type, v.language,
			       CASE WHEN e.type = 'view_end' THEN (e.props->>'watched_ms')::float8 END AS watched_ms,
			       CASE WHEN e.type = 'view_end' THEN coalesce((e.props->>'completed')::boolean, false) END AS completed,
			       v.duration_ms
			  FROM events e JOIN videos v ON v.id = e.video_id
			 WHERE e.received_at > $1 AND e.received_at <= $2`, from, until); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM rollup_ev`).Scan(&r.Events); err != nil {
			return err
		}
		// Event counters, with a running average of the watch ratio.
		if _, err := tx.Exec(ctx, `
			INSERT INTO video_stats AS s (video_id, impressions, views, completions, avg_watch_ratio, updated_at)
			SELECT video_id,
			       count(*) FILTER (WHERE type = 'impression'),
			       count(*) FILTER (WHERE type = 'view_end' AND watched_ms >= $1),
			       count(*) FILTER (WHERE type = 'view_end' AND watched_ms >= $1 AND completed),
			       coalesce(avg(least(1, watched_ms / duration_ms)) FILTER (WHERE type = 'view_end' AND watched_ms >= $1), 0),
			       now()
			  FROM rollup_ev GROUP BY video_id
			ON CONFLICT (video_id) DO UPDATE SET
			       impressions = s.impressions + EXCLUDED.impressions,
			       views = s.views + EXCLUDED.views,
			       completions = s.completions + EXCLUDED.completions,
			       avg_watch_ratio = CASE WHEN s.views + EXCLUDED.views = 0 THEN s.avg_watch_ratio
			                         ELSE (s.avg_watch_ratio * s.views + EXCLUDED.avg_watch_ratio * EXCLUDED.views) / (s.views + EXCLUDED.views) END,
			       updated_at = now()`, ViewMinMs); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE video_stats s
			   SET likes = (SELECT count(*) FROM likes l WHERE l.video_id = s.video_id),
			       hides = (SELECT count(*) FROM hides h WHERE h.video_id = s.video_id)
			 WHERE s.video_id IN (SELECT video_id FROM rollup_ev WHERE type IN ('like', 'unlike', 'hide', 'unhide'))`); err != nil {
			return err
		}
		// Per viewer level, for level refinement.
		_, err := tx.Exec(ctx, `
			INSERT INTO video_level_stats AS s (video_id, viewer_level, views, completions, likes, hides)
			SELECT e.video_id, ll.level,
			       count(*) FILTER (WHERE e.type = 'view_end' AND e.watched_ms >= $1),
			       count(*) FILTER (WHERE e.type = 'view_end' AND e.watched_ms >= $1 AND e.completed),
			       count(*) FILTER (WHERE e.type = 'like') - count(*) FILTER (WHERE e.type = 'unlike'),
			       count(*) FILTER (WHERE e.type = 'hide') - count(*) FILTER (WHERE e.type = 'unhide')
			  FROM rollup_ev e JOIN learner_languages ll ON ll.user_id = e.user_id AND ll.language = e.language
			 WHERE e.type IN ('view_end', 'like', 'unlike', 'hide', 'unhide')
			 GROUP BY e.video_id, ll.level
			ON CONFLICT (video_id, viewer_level) DO UPDATE SET
			       views = s.views + EXCLUDED.views,
			       completions = s.completions + EXCLUDED.completions,
			       likes = greatest(0, s.likes + EXCLUDED.likes),
			       hides = greatest(0, s.hides + EXCLUDED.hides)`, ViewMinMs)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE rollup_state SET done_until = $1 WHERE name = 'events'`, until)
		return err
	})
	return r, err
}

// CleanupSessions deletes expired sessions.
func (s *Service) CleanupSessions(ctx context.Context) (int64, error) {
	tag, err := s.DB.Exec(ctx, `DELETE FROM sessions WHERE expires_at < $1`, s.now())
	return tag.RowsAffected(), err
}

// RollupArgs runs Rollup.
type RollupArgs struct{}

// Kind implements river.JobArgs.
func (RollupArgs) Kind() string { return "events_rollup" }

// RollupWorker runs the rollup.
type RollupWorker struct {
	river.WorkerDefaults[RollupArgs]
	S *Service
}

// Work implements river.Worker.
func (w *RollupWorker) Work(ctx context.Context, _ *river.Job[RollupArgs]) error {
	_, err := w.S.Rollup(ctx)
	return err
}

// SessionCleanupArgs runs CleanupSessions.
type SessionCleanupArgs struct{}

// Kind implements river.JobArgs.
func (SessionCleanupArgs) Kind() string { return "session_cleanup" }

// SessionCleanupWorker deletes expired sessions daily.
type SessionCleanupWorker struct {
	river.WorkerDefaults[SessionCleanupArgs]
	S *Service
}

// Work implements river.Worker.
func (w *SessionCleanupWorker) Work(ctx context.Context, _ *river.Job[SessionCleanupArgs]) error {
	_, err := w.S.CleanupSessions(ctx)
	return err
}

// Register adds the V4 workers and returns their periodic jobs.
func Register(workers *river.Workers, db *pgxpool.Pool) []*river.PeriodicJob {
	s := &Service{DB: db}
	river.AddWorker(workers, &RollupWorker{S: s})
	river.AddWorker(workers, &SessionCleanupWorker{S: s})
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(RollupInterval),
			func() (river.JobArgs, *river.InsertOpts) { return RollupArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return SessionCleanupArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true}),
	}
}
