// Command sources loads infra/youtube_sources.yaml into youtube_sources.
// Usage: sources [path] (default ../infra/youtube_sources.yaml).
package main

import (
	"context"
	"os"

	"github.com/habos/linguascroll/backend/internal/catalog"
	"github.com/habos/linguascroll/backend/internal/platform/config"
	"github.com/habos/linguascroll/backend/internal/platform/db"
	"github.com/habos/linguascroll/backend/internal/platform/logx"
)

func main() {
	cfg, err := config.Load()
	log := logx.New(cfg.Env)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	path := "../infra/youtube_sources.yaml"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	f, err := catalog.LoadSourcesFile(path)
	if err != nil {
		log.Error("sources file", "err", err)
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	n, err := catalog.ApplySources(ctx, pool, f)
	if err != nil {
		log.Error("apply sources", "err", err)
		os.Exit(1)
	}
	log.Info("youtube sources loaded", "enabled", n, "file", path)
}
