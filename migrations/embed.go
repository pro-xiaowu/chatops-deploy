package migrations

import "embed"

// FS contains versioned PostgreSQL migrations.
//
//go:embed *.sql
var FS embed.FS
