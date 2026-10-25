package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/ports"
)

func TestAdapter_ContentCache(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)
	ctx := context.Background()

	if _, err := adapter.LoadContentCache(ctx, "missing"); !errors.Is(err, ports.ErrContentCacheMiss) {
		t.Fatalf("LoadContentCache (missing) error = %v, want ErrContentCacheMiss", err)
	}

	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := adapter.SaveContentCache(ctx, "youtube:recent", []byte(`[{"ID":"abc"}]`), expiresAt); err != nil {
		t.Fatalf("SaveContentCache: %v", err)
	}

	entry, err := adapter.LoadContentCache(ctx, "youtube:recent")
	if err != nil {
		t.Fatalf("LoadContentCache: %v", err)
	}
	if got := string(entry.Payload); got != `[{"ID": "abc"}]` {
		t.Errorf("payload = %s", got)
	}
	if !entry.ExpiresAt.Equal(expiresAt) {
		t.Errorf("expires_at = %v, want %v", entry.ExpiresAt, expiresAt)
	}
	if age := time.Since(entry.FetchedAt); age < 0 || age > time.Minute {
		t.Errorf("fetched_at looks wrong: %v ago", age)
	}

	// Upsert must replace both payload and expiry.
	newExpiry := expiresAt.Add(time.Hour)
	if err := adapter.SaveContentCache(ctx, "youtube:recent", []byte(`[]`), newExpiry); err != nil {
		t.Fatalf("SaveContentCache (update): %v", err)
	}
	entry, err = adapter.LoadContentCache(ctx, "youtube:recent")
	if err != nil {
		t.Fatalf("LoadContentCache (updated): %v", err)
	}
	if got := string(entry.Payload); got != `[]` {
		t.Errorf("updated payload = %s", got)
	}
	if !entry.ExpiresAt.Equal(newExpiry) {
		t.Errorf("updated expires_at = %v, want %v", entry.ExpiresAt, newExpiry)
	}

	// Different keys are independent (media, blog, contact share one table).
	if err := adapter.SaveContentCache(ctx, "blog:recent", []byte(`[]`), newExpiry); err != nil {
		t.Fatalf("SaveContentCache (blog): %v", err)
	}
	if got := string(mustLoad(t, adapter, ctx, "youtube:recent").Payload); got != `[]` {
		t.Errorf("media key was overwritten: %s", got)
	}
}

func TestAdapter_ContentCache_CancelledContext(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)

	if _, err := adapter.LoadContentCache(cancelledContext(), "youtube:recent"); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
	if err := adapter.SaveContentCache(cancelledContext(), "youtube:recent", []byte(`[]`), time.Now()); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func mustLoad(t *testing.T, adapter *Adapter, ctx context.Context, key string) ports.ContentCacheEntry {
	t.Helper()
	entry, err := adapter.LoadContentCache(ctx, key)
	if err != nil {
		t.Fatalf("LoadContentCache(%q): %v", key, err)
	}
	return entry
}
