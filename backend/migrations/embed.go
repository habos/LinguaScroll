// Package migrations embeds the goose SQL migrations (forward-only).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
