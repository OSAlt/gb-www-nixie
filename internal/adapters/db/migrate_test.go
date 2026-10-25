package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

const (
	latestMigration    = int64(2)
	seededPlatformRows = 7
)

func gooseVersion(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var version int64
	if err := db.QueryRow(`SELECT max(version_id) FROM nixie.goose_db_version`).Scan(&version); err != nil {
		t.Fatalf("read goose version: %v", err)
	}
	return version
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name *string
	if err := db.QueryRow(`SELECT to_regclass($1)::text`, table).Scan(&name); err != nil {
		t.Fatalf("to_regclass(%s): %v", table, err)
	}
	return name != nil
}

func TestRunMigrations_AppliesAllMigrations(t *testing.T) {
	dsn := newDatabase(t)

	if err := RunMigrations(dsn, migrationsDir()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	db := openSQL(t, dsn)
	ctx := context.Background()

	if got := gooseVersion(t, db); got != latestMigration {
		t.Errorf("goose version = %d, want %d", got, latestMigration)
	}

	var applied int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM nixie.goose_db_version WHERE is_applied AND version_id > 0`).Scan(&applied); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	if applied != int(latestMigration) {
		t.Errorf("%d applied migrations, want %d", applied, latestMigration)
	}

	for _, table := range []string{
		"nixie.contact_form",
		"nixie.social_metrics",
		"config.social_app",
		"config.social_app_type",
		"config.user_social",
	} {
		if !tableExists(t, db, table) {
			t.Errorf("expected table %s to exist after migrations", table)
		}
	}

	var platforms int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM nixie.social_metrics`).Scan(&platforms); err != nil {
		t.Fatalf("count seeded metrics: %v", err)
	}
	if platforms != seededPlatformRows {
		t.Errorf("got %d seeded platforms, want %d", platforms, seededPlatformRows)
	}

	// Every seeded row must look unrefreshed so the worker fills it on boot.
	var refreshed int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM nixie.social_metrics
		WHERE refreshed_at <> 'epoch'::timestamptz`).Scan(&refreshed); err != nil {
		t.Fatalf("check seed timestamps: %v", err)
	}
	if refreshed != 0 {
		t.Errorf("%d seeded rows are not timestamped 'epoch'", refreshed)
	}

	// Running again must be a no-op rather than an error or duplicate rows.
	if err := RunMigrations(dsn, migrationsDir()); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}
	if got := gooseVersion(t, db); got != latestMigration {
		t.Errorf("after re-run goose version = %d, want %d", got, latestMigration)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM nixie.social_metrics`).Scan(&platforms); err != nil {
		t.Fatalf("count after re-run: %v", err)
	}
	if platforms != seededPlatformRows {
		t.Errorf("after re-run got %d rows, want %d", platforms, seededPlatformRows)
	}
}

// TestRunMigrations_RollbackLatestMigration verifies that the most recent
// migration has a working Down section: rolling it back removes only the
// objects it created, leaves migration 001 in place, and can be re-applied.
func TestRunMigrations_RollbackLatestMigration(t *testing.T) {
	dsn := newDatabase(t)
	if err := RunMigrations(dsn, migrationsDir()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	db := openSQL(t, dsn)
	ctx := context.Background()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(migrationsDir()),
		goose.WithTableName("nixie.goose_db_version"))
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })

	result, err := provider.Down(ctx)
	if err != nil {
		t.Fatalf("roll back latest migration: %v", err)
	}
	if result == nil || result.Source == nil {
		t.Fatal("no migration was rolled back")
	}
	if result.Source.Version != latestMigration {
		t.Errorf("rolled back migration %d, want %d", result.Source.Version, latestMigration)
	}

	// 002 owns social_metrics, so it must be gone.
	if tableExists(t, db, "nixie.social_metrics") {
		t.Error("nixie.social_metrics still exists after rolling back 002")
	}
	// 001 still owns contact_form and the config schema.
	if !tableExists(t, db, "nixie.contact_form") {
		t.Error("nixie.contact_form should survive rolling back 002")
	}
	if !tableExists(t, db, "config.social_app") {
		t.Error("config.social_app should survive rolling back 002")
	}

	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("GetDBVersion: %v", err)
	}
	if version != 1 {
		t.Errorf("goose version after rollback = %d, want 1", version)
	}

	// Re-applying restores the table and its seeds.
	results, err := provider.Up(ctx)
	if err != nil {
		t.Fatalf("re-apply migrations: %v", err)
	}
	if len(results) != 1 || results[0].Source.Version != latestMigration {
		t.Fatalf("got %d re-applied migrations, want just %d", len(results), latestMigration)
	}
	if !tableExists(t, db, "nixie.social_metrics") {
		t.Fatal("nixie.social_metrics was not recreated")
	}
	var platforms int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM nixie.social_metrics`).Scan(&platforms); err != nil {
		t.Fatalf("count re-seeded metrics: %v", err)
	}
	if platforms != seededPlatformRows {
		t.Errorf("got %d rows after re-apply, want %d", platforms, seededPlatformRows)
	}
}

func TestRunMigrations_MissingMigrationsDir(t *testing.T) {
	dsn := newDatabase(t)

	missing := filepath.Join(t.TempDir(), "no-such-migrations")
	if err := RunMigrations(dsn, missing); err == nil {
		t.Fatal("expected an error when the migrations directory is missing")
	}
}

func TestRunMigrations_InvalidDSN(t *testing.T) {
	if err := RunMigrations("://not-a-dsn", migrationsDir()); err == nil {
		t.Fatal("expected an error for a malformed DSN")
	}
}
