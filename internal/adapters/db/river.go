package db

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

const (
	// RiverSchema keeps River's tables out of the public, nixie and config
	// schemas. db/schema.sql (and therefore sqlc) only dumps nixie and config.
	RiverSchema = "river"

	// RiverSchemaVersion pins the River migration the client expects. Bump it
	// together with github.com/riverqueue/river; the version is the highest
	// number in riverdriver/riverpgxv5/migration/main (8 at v0.49.0).
	RiverSchemaVersion = 8
)

// MigrateRiver creates the River schema (if needed) and applies River's own
// migrations, up to RiverSchemaVersion. It is safe to call on every boot and
// from several instances at once: a Postgres advisory lock serialises it.
//
// River migrations are deliberately not managed by goose. Goose's version table
// belongs to the application, and the two migration histories would otherwise
// collide.
func MigrateRiver(ctx context.Context, pool *pgxpool.Pool) error {
	// The schema is created in its own committed step. River's migrator runs on
	// pooled connections, which cannot see a schema created inside an
	// uncommitted transaction.
	if err := withAdvisoryLock(ctx, pool, func(ctx context.Context) error {
		// River does not create alternate schemas itself.
		if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+RiverSchema); err != nil {
			return fmt.Errorf("create river schema: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := withAdvisoryLock(ctx, pool, func(ctx context.Context) error {
		migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Schema: RiverSchema})
		if err != nil {
			return fmt.Errorf("create river migrator: %w", err)
		}
		if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{
			TargetVersion: RiverSchemaVersion,
		}); err != nil {
			return fmt.Errorf("migrate river schema: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// withAdvisoryLock runs fn while holding a transaction-scoped Postgres advisory
// lock, so simultaneous boots do not migrate at the same time. The lock is
// released when the wrapping transaction commits or rolls back.
func withAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin river migration lock: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", advisoryLockID("gb-www-nixie:river_migrations")); err != nil {
		return fmt.Errorf("acquire river migration lock: %w", err)
	}

	if err := fn(ctx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("release river migration lock: %w", err)
	}
	return nil
}

// advisoryLockID turns a name into a stable Postgres advisory lock ID.
func advisoryLockID(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return int64(h.Sum64())
}
