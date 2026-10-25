package db

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func RunMigrations(databaseURL string, migrationsDir string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Ensure the nixie schema exists so we can use it for goose version table
	if _, err := db.Exec("CREATE SCHEMA IF NOT EXISTS nixie"); err != nil {
		fmt.Printf("Warning: failed to create nixie schema: %v\n", err)
	}

	goose.SetDialect("postgres")
	goose.SetTableName("nixie.goose_db_version")

	// Deployed databases can record a migration version that no longer matches
	// the checked-in filenames (for example after a migration was renamed), which
	// otherwise blocks startup with "found N missing migrations before current
	// version". Allow missing migrations so newer files still apply; the last
	// migration is written defensively (CREATE/DROP IF [NOT] EXISTS) so it is
	// safe when its objects already exist.
	if err := goose.Up(db, migrationsDir, goose.WithAllowMissing()); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
