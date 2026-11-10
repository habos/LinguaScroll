// Command migrate applies the app's goose migrations and River's own schema.
package main

import (
	"context"
	"os"

	"github.com/habos/linguascroll/backend/internal/platform/config"
	"github.com/habos/linguascroll/backend/internal/platform/db"
	"github.com/habos/linguascroll/backend/internal/platform/logx"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
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

	if err := db.Migrate(ctx, pool); err != nil {
		log.Error("app migrations", "err", err)
		os.Exit(1)
	}
	m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		log.Error("river migrator", "err", err)
		os.Exit(1)
	}
	if _, err := m.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		log.Error("river migrations", "err", err)
		os.Exit(1)
	}
	log.Info("migrations applied")
}
