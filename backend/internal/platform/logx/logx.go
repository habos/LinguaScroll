// Package logx builds the process logger: JSON lines via log/slog.
package logx

import (
	"log/slog"
	"os"
)

// New returns a JSON slog logger; debug level outside prod.
func New(env string) *slog.Logger {
	level := slog.LevelDebug
	if env == "prod" {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
