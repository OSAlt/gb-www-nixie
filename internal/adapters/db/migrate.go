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

	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
