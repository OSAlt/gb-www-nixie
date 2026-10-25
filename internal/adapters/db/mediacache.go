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

var _ ports.MediaCacheStore = (*Adapter)(nil)

// LoadMediaCache returns the last-known-good payload for key. It does not
// enforce expiry: callers decide whether stale data is still good enough to
// serve (the media service does, deliberately).
func (a *Adapter) LoadMediaCache(ctx context.Context, key string) (ports.MediaCacheEntry, error) {
	row, err := a.queries.GetMediaCache(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.MediaCacheEntry{}, ports.ErrMediaCacheMiss
		}
		return ports.MediaCacheEntry{}, err
	}
	if !row.FetchedAt.Valid || !row.ExpiresAt.Valid {
		return ports.MediaCacheEntry{}, fmt.Errorf("media cache %q has invalid timestamps", key)
	}
	return ports.MediaCacheEntry{
		Payload:   row.Payload,
		FetchedAt: row.FetchedAt.Time,
		ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

func (a *Adapter) SaveMediaCache(ctx context.Context, key string, payload []byte, expiresAt time.Time) error {
	return a.queries.UpsertMediaCache(ctx, UpsertMediaCacheParams{
		Key:       key,
		Payload:   payload,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}
