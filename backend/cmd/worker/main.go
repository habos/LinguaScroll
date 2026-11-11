// Command worker runs River background jobs. Feature modules add their
// workers and periodic jobs here (V3 catalog, V4 stats).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/habos/linguascroll/backend/internal/catalog"
	"github.com/habos/linguascroll/backend/internal/feed"
	"github.com/habos/linguascroll/backend/internal/jobs"
	"github.com/habos/linguascroll/backend/internal/langdetect"
	"github.com/habos/linguascroll/backend/internal/platform/config"
	"github.com/habos/linguascroll/backend/internal/platform/db"
	"github.com/habos/linguascroll/backend/internal/platform/logx"
	"github.com/habos/linguascroll/backend/internal/youtube"
	"github.com/habos/linguascroll/backend/internal/youtube/fake"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func main() {
	cfg, err := config.Load()
	log := logx.New(cfg.Env)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	workers := river.NewWorkers()
	river.AddWorker(workers, &jobs.EventPartitionsWorker{DB: pool})
	river.AddWorker(workers, &feed.StatsWorker{DB: pool})

	cat := &catalog.Service{DB: pool, Log: log}
	if cfg.FakeVendors {
		// No network: a canned catalog per source, and the clips already
		// stored, so `make dev` grows a feed and refresh keeps it.
		yt := fake.New()
		if err := catalog.FillDevFake(ctx, pool, yt); err != nil {
			log.Error("fill dev fake", "err", err)
			os.Exit(1)
		}
		cat.YT, cat.Detect = yt, langdetect.DefaultFake
	} else {
		cat.YT, cat.Detect = &youtube.HTTPClient{Key: cfg.YouTubeAPIKey}, &langdetect.Lingua{}
	}
	periodic := catalog.Register(workers, cat)

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:  log,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
		Workers: workers,
		PeriodicJobs: append(periodic,
			river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return jobs.EventPartitionsArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(feed.StatsInterval),
				func() (river.JobArgs, *river.InsertOpts) { return feed.StatsArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true})),
	})
	if err != nil {
		log.Error("river client", "err", err)
		os.Exit(1)
	}
	if err := client.Start(ctx); err != nil {
		log.Error("river start", "err", err)
		os.Exit(1)
	}
	log.Info("worker started", "env", cfg.Env, "fake_vendors", cfg.FakeVendors)
	<-ctx.Done()
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Stop(stopCtx); err != nil {
		log.Error("river stop", "err", err)
	}
}
