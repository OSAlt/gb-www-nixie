package ports

import (
	"context"
	"errors"
	"time"
)

// ErrContentCacheMiss is returned by ContentCacheStore when no row exists for
// the key. Callers treat it as "not cached yet", not as a failure.
var ErrContentCacheMiss = errors.New("content cache miss")

// ContentCacheEntry is one cached payload plus its freshness window.
type ContentCacheEntry struct {
	Payload   []byte
	FetchedAt time.Time
	ExpiresAt time.Time
}

// ContentCacheStore persists external feed payloads (videos, blog posts,
// contact subjects) so every replica serves the same last-known-good data
// without calling the upstream API from a request.
type ContentCacheStore interface {
	LoadContentCache(ctx context.Context, key string) (ContentCacheEntry, error)
	SaveContentCache(ctx context.Context, key string, payload []byte, expiresAt time.Time) error
}

// MediaRefresher fetches the latest posts and stores them. Refresh is a no-op
// when the shared cache is still fresh.
type MediaRefresher interface {
	Refresh(ctx context.Context, count int) (int, error)
}

// CacheState describes a cached feed for health reporting. Missing is true when
// there is no cached payload at all.
type CacheState struct {
	Key       string
	Items     int
	FetchedAt time.Time
	ExpiresAt time.Time
	Stale     bool
	Missing   bool
}
