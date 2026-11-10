// Command seed loads languages, config defaults, starter YouTube sources and
// 30 placeholder clips. Idempotent.
package main

import (
	"context"
	"os"

	"github.com/habos/linguascroll/backend/internal/platform/config"
	"github.com/habos/linguascroll/backend/internal/platform/db"
	"github.com/habos/linguascroll/backend/internal/platform/logx"
	"github.com/habos/linguascroll/backend/seed"
)

func main() {
	cfg, err := config.Load()
	log := logx.New(cfg.Env)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := seed.Apply(ctx, pool); err != nil {
		log.Error("seed", "err", err)
		os.Exit(1)
	}
	log.Info("seed applied")
}
