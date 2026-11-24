// Package migrate применяет встроенные SQL-миграции при старте приложения.
package migrate

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	// Регистрирует SQL-драйвер "pgx" через свой init().
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/kseno/currency-api/migrations"
)

// Run применяет все доступные миграции к базе из databaseURL.
func Run(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database for migrations: %w", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			fmt.Printf("close migration db: %v\n", cerr)
		}
	}()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("load embedded migrations: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
