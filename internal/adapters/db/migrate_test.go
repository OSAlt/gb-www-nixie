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
	latestMigration    = int64(3)
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
		"nixie.media_cache",
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

// TestRunMigrations_AppliesMigrationBehindNewerVersion reproduces a deployed
// database that recorded a newer migration version than the checked-in files
// while the last migration's version was never recorded. Startup must replay
// the latest migration's real SQL without failing on the objects it already
// created.
func TestRunMigrations_AppliesMigrationBehindNewerVersion(t *testing.T) {
	dsn := newDatabase(t)

	// Replay the real migrations from db/migrations.
	if err := RunMigrations(dsn, migrationsDir()); err != nil {
		t.Fatalf("initial RunMigrations: %v", err)
	}
	db := openSQL(t, dsn)
	ctx := context.Background()

	// Keep a marker so we can prove the replayed seed does not overwrite data.
	if _, err := db.ExecContext(ctx,
		`UPDATE nixie.social_metrics SET followers = 999999 WHERE platform = 'youtube'`); err != nil {
		t.Fatalf("mark existing row: %v", err)
	}

	// The deployed database recorded a migration version newer than any
	// checked-in file and never recorded the latest migration under its
	// current version.
	const legacyVersion = int64(20260215054924)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO nixie.goose_db_version (version_id, is_applied) VALUES ($1, true)`,
		legacyVersion); err != nil {
		t.Fatalf("record legacy version: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`DELETE FROM nixie.goose_db_version WHERE version_id = $1`, latestMigration); err != nil {
		t.Fatalf("forget migration %d: %v", latestMigration, err)
	}

	// Without allow-missing this is where goose reports "found 1 missing
	// migrations before current version".
	if err := RunMigrations(dsn, migrationsDir()); err != nil {
		t.Fatalf("RunMigrations with a newer recorded version: %v", err)
	}

	var applied bool
	if err := db.QueryRowContext(ctx,
		`SELECT is_applied FROM nixie.goose_db_version WHERE version_id = $1`,
		latestMigration).Scan(&applied); err != nil {
		t.Fatalf("read migration %d: %v", latestMigration, err)
	}
	if !applied {
		t.Errorf("migration %d was not recorded as applied", latestMigration)
	}

	// The replayed migration must tolerate the existing table and rows.
	var youtube int
	if err := db.QueryRowContext(ctx,
		`SELECT followers FROM nixie.social_metrics WHERE platform = 'youtube'`).Scan(&youtube); err != nil {
		t.Fatalf("read youtube followers: %v", err)
	}
	if youtube != 999999 {
		t.Errorf("replayed seed overwrote existing data: got %d, want 999999", youtube)
	}

	var platforms int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM nixie.social_metrics`).Scan(&platforms); err != nil {
		t.Fatalf("count seeded metrics: %v", err)
	}
	if platforms != seededPlatformRows {
		t.Errorf("got %d seeded platforms, want %d", platforms, seededPlatformRows)
	}
}

// TestRunMigrations_RollbackLatestMigration verifies that the most recent
// migration has a working Down section: rolling it back removes only the
// objects it created, leaves earlier migrations in place, and can be
// re-applied.
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

	// 003 owns media_cache, so it must be gone.
	if tableExists(t, db, "nixie.media_cache") {
		t.Error("nixie.media_cache still exists after rolling back 003")
	}
	// 002 still owns social_metrics; 001 owns contact_form and the config schema.
	if !tableExists(t, db, "nixie.social_metrics") {
		t.Error("nixie.social_metrics should survive rolling back 003")
	}
	if !tableExists(t, db, "nixie.contact_form") {
		t.Error("nixie.contact_form should survive rolling back 003")
	}
	if !tableExists(t, db, "config.social_app") {
		t.Error("config.social_app should survive rolling back 003")
	}

	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("GetDBVersion: %v", err)
	}
	if version != 2 {
		t.Errorf("goose version after rollback = %d, want 2", version)
	}

	// Re-applying restores the cache table.
	results, err := provider.Up(ctx)
	if err != nil {
		t.Fatalf("re-apply migrations: %v", err)
	}
	if len(results) != 1 || results[0].Source.Version != latestMigration {
		t.Fatalf("got %d re-applied migrations, want just %d", len(results), latestMigration)
	}
	if !tableExists(t, db, "nixie.media_cache") {
		t.Fatal("nixie.media_cache was not recreated")
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
