package sqlite

import "embed"

// migrationsFS embeds the goose SQL migration files so they ship inside the
// application binary rather than depending on files present on disk.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS
