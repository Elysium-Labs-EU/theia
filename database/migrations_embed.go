package database

import "embed"

//go:embed migrations/*.sql
var MigrationsFS embed.FS

const MigrationsPath = "migrations"
