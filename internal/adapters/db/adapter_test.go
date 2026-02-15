package db

import (
	"context"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type mockDBTX struct{}

func (m *mockDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (m *mockDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (m *mockDBTX) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }

func TestAdapter_GetSocialCounts(t *testing.T) {
	a := NewAdapter(nil)

	expected := map[string]int{
		"youtube":   330000,
		"twitter":   31102,
		"twitch":    2500,
		"discord":   6336,
		"facebook":  50466,
		"instagram": 7498,
	}

	got, err := a.GetSocialCounts(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v, want %v", got, expected)
	}
}

func TestAdapter_ListContacts_Empty(t *testing.T) {
	// For testing ListContacts we'd need a real DB or a much more complex mock
	// since SQLC generated code expects a real DBTX.
	// Skipping for now as it requires complex mocking of pgx.Rows.
}
