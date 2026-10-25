package db

import (
	"context"
	"errors"
	"testing"

	"github.com/OSAlt/gb-www-nixie/internal/ports"
)

// socialCount returns the stored follower count for a platform, or false when
// the row does not exist.
func socialCount(t *testing.T, adapter *Adapter, platform string) (int, bool) {
	t.Helper()
	counts, err := adapter.GetSocialCounts(context.Background())
	if err != nil {
		t.Fatalf("GetSocialCounts: %v", err)
	}
	count, ok := counts[platform]
	return count, ok
}

func TestAdapter_WithTx_Commits(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)
	ctx := context.Background()

	err := adapter.WithTx(ctx, func(ctx context.Context, tx ports.Transaction) error {
		if tx.PgxTx() == nil {
			t.Fatal("PgxTx returned nil inside a transaction")
		}
		if err := tx.UpsertSocialCount(ctx, "youtube", 304000); err != nil {
			return err
		}
		return tx.UpsertSocialCount(ctx, "twitter", 26000)
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}

	for platform, want := range map[string]int{"youtube": 304000, "twitter": 26000} {
		got, ok := socialCount(t, adapter, platform)
		if !ok {
			t.Errorf("%s was not committed", platform)
			continue
		}
		if got != want {
			t.Errorf("%s = %d, want %d", platform, got, want)
		}
	}
}

func TestAdapter_WithTx_RollsBackOnError(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)
	ctx := context.Background()
	abort := errors.New("abort")

	err := adapter.WithTx(ctx, func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.UpsertSocialCount(ctx, "youtube", 304000); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("WithTx error = %v, want %v", err, abort)
	}

	if _, ok := socialCount(t, adapter, "youtube"); ok {
		t.Error("rolled-back row is visible")
	}
}

func TestAdapter_WithTx_RollsBackOnPanic(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)
	ctx := context.Background()

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected the panic to propagate")
			}
		}()
		_ = adapter.WithTx(ctx, func(ctx context.Context, tx ports.Transaction) error {
			if err := tx.UpsertSocialCount(ctx, "youtube", 304000); err != nil {
				return err
			}
			panic("simulated bug")
		})
	}()

	if _, ok := socialCount(t, adapter, "youtube"); ok {
		t.Error("rolled-back row is visible after a panic")
	}
}

func TestAdapter_WithTx_CancelledContext(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)
	called := false

	err := adapter.WithTx(cancelledContext(), func(context.Context, ports.Transaction) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
	if called {
		t.Error("callback ran despite a cancelled context")
	}
}

func TestAdapter_WithTx_RejectsNestedTransaction(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)
	ctx := context.Background()

	err := adapter.WithTx(ctx, func(ctx context.Context, tx ports.Transaction) error {
		return tx.(*TxAdapter).WithTx(ctx, func(context.Context, ports.Transaction) error {
			return nil
		})
	})
	if !errors.Is(err, errNestedTx) {
		t.Fatalf("nested WithTx error = %v, want %v", err, errNestedTx)
	}
}

func TestAdapter_WithTx_WithoutPool(t *testing.T) {
	if err := (&Adapter{}).WithTx(context.Background(), func(context.Context, ports.Transaction) error {
		return nil
	}); err == nil {
		t.Fatal("expected an error when the adapter has no pool")
	}
}
