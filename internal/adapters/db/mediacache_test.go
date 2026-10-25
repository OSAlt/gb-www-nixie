package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/ports"
)

func TestAdapter_MediaCache(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)
	ctx := context.Background()

	if _, err := adapter.LoadMediaCache(ctx, "missing"); !errors.Is(err, ports.ErrMediaCacheMiss) {
		t.Fatalf("LoadMediaCache (missing) error = %v, want ErrMediaCacheMiss", err)
	}

	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := adapter.SaveMediaCache(ctx, "youtube:recent", []byte(`[{"ID":"abc"}]`), expiresAt); err != nil {
		t.Fatalf("SaveMediaCache: %v", err)
	}

	entry, err := adapter.LoadMediaCache(ctx, "youtube:recent")
	if err != nil {
		t.Fatalf("LoadMediaCache: %v", err)
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
	if err := adapter.SaveMediaCache(ctx, "youtube:recent", []byte(`[]`), newExpiry); err != nil {
		t.Fatalf("SaveMediaCache (update): %v", err)
	}
	entry, err = adapter.LoadMediaCache(ctx, "youtube:recent")
	if err != nil {
		t.Fatalf("LoadMediaCache (updated): %v", err)
	}
	if got := string(entry.Payload); got != `[]` {
		t.Errorf("updated payload = %s", got)
	}
	if !entry.ExpiresAt.Equal(newExpiry) {
		t.Errorf("updated expires_at = %v, want %v", entry.ExpiresAt, newExpiry)
	}
}

func TestAdapter_MediaCache_CancelledContext(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)

	if _, err := adapter.LoadMediaCache(cancelledContext(), "youtube:recent"); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
	if err := adapter.SaveMediaCache(cancelledContext(), "youtube:recent", []byte(`[]`), time.Now()); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}
