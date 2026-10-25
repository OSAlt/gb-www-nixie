package ports

import (
	"context"
	"errors"
	"time"
)

// ErrMediaCacheMiss is returned by MediaCacheStore when no row exists for the
// key. Callers treat it as "not cached yet", not as a failure.
var ErrMediaCacheMiss = errors.New("media cache miss")

// MediaCacheEntry is one cached feed payload plus its freshness window.
type MediaCacheEntry struct {
	Payload   []byte
	FetchedAt time.Time
	ExpiresAt time.Time
}

// MediaCacheStore persists feed payloads so every replica serves the same
// last-known-good data without calling the upstream API from a request.
type MediaCacheStore interface {
	LoadMediaCache(ctx context.Context, key string) (MediaCacheEntry, error)
	SaveMediaCache(ctx context.Context, key string, payload []byte, expiresAt time.Time) error
}

// MediaRefresher fetches the latest posts and stores them. Refresh is a no-op
// when the shared cache is still fresh.
type MediaRefresher interface {
	Refresh(ctx context.Context, count int) (int, error)
}
