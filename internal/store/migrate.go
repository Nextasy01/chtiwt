package store

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate brings the database schema up to head. Embedded SQL files are
// applied via golang-migrate's iofs source + the pgx/v5 driver, so the
// production container needs neither the migrate CLI nor a writable disk
// copy of the migrations directory.
//
// dsn must be a libpq-style connection string. golang-migrate's pgx5 driver
// accepts the standard postgres:// URL — we don't need to rewrite the scheme.
func Migrate(dsn string) error {
	src, err := iofs.New(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrate source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, "pgx5://"+trimPrefix(dsn))
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// trimPrefix strips a scheme like "postgres://" or "postgresql://" from a
// DSN so we can prepend "pgx5://". golang-migrate uses the URL scheme to
// dispatch to the right database driver, and the pgx/v5 driver is
// registered under "pgx5".
func trimPrefix(dsn string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if len(dsn) >= len(p) && dsn[:len(p)] == p {
			return dsn[len(p):]
		}
	}
	return dsn
}
