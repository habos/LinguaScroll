// Command recheck runs every live YouTube clip through the current language
// rules, filters and channel blocklist, and removes the ones that fail
// (module V3). Run it after tightening the rules or blocking a channel. It
// spends 1 quota unit per 50 clips.
package main

import (
	"context"
	"os"

	"github.com/habos/linguascroll/backend/internal/catalog"
	"github.com/habos/linguascroll/backend/internal/langdetect"
	"github.com/habos/linguascroll/backend/internal/platform/config"
	"github.com/habos/linguascroll/backend/internal/platform/db"
	"github.com/habos/linguascroll/backend/internal/platform/logx"
	"github.com/habos/linguascroll/backend/internal/youtube"
	"github.com/habos/linguascroll/backend/internal/youtube/fake"
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
	s := &catalog.Service{DB: pool, Log: log}
	if cfg.FakeVendors {
		yt := fake.New()
		if err := catalog.FillDevFake(ctx, pool, yt); err != nil {
			log.Error("fill dev fake", "err", err)
			os.Exit(1)
		}
		s.YT, s.Detect = yt, langdetect.DefaultFake
	} else {
		s.YT, s.Detect = &youtube.HTTPClient{Key: cfg.YouTubeAPIKey}, &langdetect.Lingua{}
	}
	kept, removed, err := s.RecheckLanguages(ctx)
	if err != nil {
		log.Error("recheck", "err", err, "kept", kept, "removed", removed)
		os.Exit(1)
	}
	log.Info("recheck done", "kept", kept, "removed", removed)
}
