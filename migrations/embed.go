// Package migrations embeds the SQL database migrations
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
