package db

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	// pgx v5 database driver for golang-migrate (registers the "pgx5" URL scheme).
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies all pending migrations using an owner (DDL-capable) connection.
// migrateURL must use the pgx5 scheme, e.g.
//
//	pgx5://sap_owner:pw@postgres:5432/sap?sslmode=disable
//
// The runtime pool connects separately as the non-privileged, NOBYPASSRLS
// sap_app role — migrations and runtime never share a connection identity.
func Migrate(migrateURL string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("db.Migrate: open embedded source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateURL)
	if err != nil {
		return fmt.Errorf("db.Migrate: init: %w", err)
	}
	defer func() { _, _ = m.Close() }() // returns (sourceErr, dbErr); nothing actionable on close

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("db.Migrate: up: %w", err)
	}
	return nil
}
