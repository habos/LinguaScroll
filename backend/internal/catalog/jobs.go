package catalog

import (
	"context"
	"time"

	"github.com/habos/linguascroll/backend/internal/appconfig"
	"github.com/habos/linguascroll/backend/internal/youtube"
	"github.com/riverqueue/river"
)

// SearchInterval is how often the search job runs; each run spends about
// its share of the daily budget, so searches spread through the day.
const SearchInterval = 30 * time.Minute

// SearchArgs is the youtube_search River job.
type SearchArgs struct{}

// Kind implements river.JobArgs.
func (SearchArgs) Kind() string { return "youtube_search" }

// SearchWorker runs this interval's share of searches.
type SearchWorker struct {
	river.WorkerDefaults[SearchArgs]
	S *Service
}

// Work implements river.Worker.
func (w *SearchWorker) Work(ctx context.Context, _ *river.Job[SearchArgs]) error {
	budget, err := appconfig.Get(ctx, w.S.cfg(), "youtube.daily_quota_units", 9000)
	if err != nil {
		return err
	}
	runsPerDay := int(24 * time.Hour / SearchInterval)
	perRun := max(1, budget/(youtube.SearchCost+2)/runsPerDay)
	_, _, err = w.S.RunSearches(ctx, perRun)
	return err
}

// RefreshArgs is the daily youtube_refresh job.
type RefreshArgs struct{}

// Kind implements river.JobArgs.
func (RefreshArgs) Kind() string { return "youtube_refresh" }

// RefreshWorker re-reads clips before the 30-day limit.
type RefreshWorker struct {
	river.WorkerDefaults[RefreshArgs]
	S *Service
}

// Work implements river.Worker.
func (w *RefreshWorker) Work(ctx context.Context, _ *river.Job[RefreshArgs]) error {
	refreshed, removed, err := w.S.Refresh(ctx)
	w.S.log().Info("youtube refresh", "refreshed", refreshed, "removed", removed)
	return err
}

// LevelArgs is the nightly youtube_level job.
type LevelArgs struct{}

// Kind implements river.JobArgs.
func (LevelArgs) Kind() string { return "youtube_level" }

// LevelWorker refines levels from engagement.
type LevelWorker struct {
	river.WorkerDefaults[LevelArgs]
	S *Service
}

// Work implements river.Worker.
func (w *LevelWorker) Work(ctx context.Context, _ *river.Job[LevelArgs]) error {
	n, err := w.S.RefineLevels(ctx)
	w.S.log().Info("youtube level refinement", "updated", n)
	return err
}

// Register adds the catalog workers and their schedules.
func Register(workers *river.Workers, s *Service) []*river.PeriodicJob {
	river.AddWorker(workers, &SearchWorker{S: s})
	river.AddWorker(workers, &RefreshWorker{S: s})
	river.AddWorker(workers, &LevelWorker{S: s})
	every := func(d time.Duration, args river.JobArgs) *river.PeriodicJob {
		return river.NewPeriodicJob(river.PeriodicInterval(d),
			func() (river.JobArgs, *river.InsertOpts) { return args, nil },
			&river.PeriodicJobOpts{RunOnStart: true})
	}
	return []*river.PeriodicJob{
		every(SearchInterval, SearchArgs{}),
		every(24*time.Hour, RefreshArgs{}),
		every(24*time.Hour, LevelArgs{}),
	}
}
