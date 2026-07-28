// Package db exposes the goose migration files as an embedded filesystem so the
// compiled binary can migrate its own database with no external SQL files on the
// target Windows host. This is what makes "ship a single .exe" viable.
package db

import "embed"

// Migrations holds every goose migration under db/migrations. Callers pass it to
// goose.SetBaseFS and run against the "migrations" subdirectory.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// MigrationsDir is the path within Migrations that goose should read.
const MigrationsDir = "migrations"
