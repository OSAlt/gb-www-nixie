package db

import (
	"context"
	"testing"
)

func TestMigrateRiver(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()

	if err := MigrateRiver(ctx, pool); err != nil {
		t.Fatalf("MigrateRiver: %v", err)
	}

	// A second boot must be a no-op rather than an error.
	if err := MigrateRiver(ctx, pool); err != nil {
		t.Fatalf("MigrateRiver (second run): %v", err)
	}

	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = 'river_job'
		)`, RiverSchema).Scan(&exists); err != nil {
		t.Fatalf("query river_job: %v", err)
	}
	if !exists {
		t.Fatalf("expected %s.river_job to exist after migrating", RiverSchema)
	}

	// The pinned version must be recorded, otherwise a later boot would try to
	// re-apply migrations.
	var version int
	if err := pool.QueryRow(ctx, "SELECT max(version) FROM "+RiverSchema+".river_migration").Scan(&version); err != nil {
		t.Fatalf("query river_migration: %v", err)
	}
	if version != RiverSchemaVersion {
		t.Fatalf("river migration version = %d, want %d", version, RiverSchemaVersion)
	}
}
