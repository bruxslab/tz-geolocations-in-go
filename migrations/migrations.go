// Package migrations embeds versioned PostgreSQL migrations for the CLI.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
