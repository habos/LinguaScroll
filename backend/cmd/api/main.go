// Command api serves the LinguaScroll HTTP API.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/habos/linguascroll/backend/internal/platform/auth"
	"github.com/habos/linguascroll/backend/internal/platform/config"
	"github.com/habos/linguascroll/backend/internal/platform/db"
	"github.com/habos/linguascroll/backend/internal/platform/logx"
	"github.com/habos/linguascroll/backend/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logx.New("dev").Error("config", "err", err)
		os.Exit(1)
	}
	log := logx.New(cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: server.New(server.Deps{
			DB: pool, Log: log, Tokens: auth.Store{DB: pool}, RateLimitPerMinute: cfg.RateLimitPerMinute,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env, "fake_vendors", cfg.FakeVendors)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
