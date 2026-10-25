package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ ports.ContentCacheStore = (*Adapter)(nil)

// LoadContentCache returns the last-known-good payload for key. It does not
// enforce expiry: callers decide whether stale data is still good enough to
// serve (the feed services do, deliberately).
func (a *Adapter) LoadContentCache(ctx context.Context, key string) (ports.ContentCacheEntry, error) {
	row, err := a.queries.GetContentCache(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.ContentCacheEntry{}, ports.ErrContentCacheMiss
		}
		return ports.ContentCacheEntry{}, err
	}
	if !row.FetchedAt.Valid || !row.ExpiresAt.Valid {
		return ports.ContentCacheEntry{}, fmt.Errorf("content cache %q has invalid timestamps", key)
	}
	return ports.ContentCacheEntry{
		Payload:   row.Payload,
		FetchedAt: row.FetchedAt.Time,
		ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

func (a *Adapter) SaveContentCache(ctx context.Context, key string, payload []byte, expiresAt time.Time) error {
	return a.queries.UpsertContentCache(ctx, UpsertContentCacheParams{
		Key:       key,
		Payload:   payload,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}
