// Package migrations embeds the SQL migrations, applied in order by the
// in-process migrator on boot (internal/store). No external migration tool.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
