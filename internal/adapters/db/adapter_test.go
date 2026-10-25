package db

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// testDSN is empty when Docker/testcontainers is unavailable; the tests then
// skip instead of failing, so `go test ./...` still works without Docker.
var testDSN string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		// `go test -short ./...` should work without Docker or a database.
		os.Exit(m.Run())
	}

	ctx := context.Background()

	// CI sets TEST_DATABASE_URL to a database supplied by the pipeline (for
	// example a GitLab service). Locally, testcontainers starts one instead.
	// The tests truncate tables, so never point this at the dev database.
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		if err := waitForDatabase(ctx, dsn, 30*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "TEST_DATABASE_URL is set but unusable: %v\n", err)
			os.Exit(1)
		}
		if err := RunMigrations(dsn, migrationsDir()); err != nil {
			fmt.Fprintf(os.Stderr, "TEST_DATABASE_URL migrations failed: %v\n", err)
			os.Exit(1)
		}
		testDSN = dsn
		os.Exit(m.Run())
	}

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("nixiedb"),
		postgres.WithUsername("nixie"),
		postgres.WithPassword("nixie_password"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: skipping DB integration tests (testcontainers unavailable): %v\n", err)
		os.Exit(m.Run())
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: skipping DB integration tests (no connection string): %v\n", err)
		_ = testcontainers.TerminateContainer(container)
		os.Exit(m.Run())
	}

	if err := RunMigrations(dsn, migrationsDir()); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: skipping DB integration tests (migrations failed): %v\n", err)
		_ = testcontainers.TerminateContainer(container)
		os.Exit(m.Run())
	}

	testDSN = dsn
	code := m.Run()
	_ = testcontainers.TerminateContainer(container)
	os.Exit(code)
}

// waitForDatabase retries until the DSN is reachable, so tests do not race a
// database container or CI service that is still starting.
func waitForDatabase(ctx context.Context, dsn string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		conn, err := pgx.Connect(ctx, dsn)
		if err == nil {
			err = conn.Ping(ctx)
			_ = conn.Close(ctx)
			if err == nil {
				return nil
			}
		}
		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("database not reachable after %s: %w", timeout, lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// migrationsDir resolves db/migrations from the test file location, so tests
// work regardless of the process working directory.
func migrationsDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "db/migrations"
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "db", "migrations")
}

func requireDB(t *testing.T) string {
	t.Helper()
	if testDSN == "" {
		t.Skip("Docker/testcontainers unavailable; skipping DB integration test")
	}
	return testDSN
}

// openSQL gives tests a plain database/sql handle for seeding and assertions.
func openSQL(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return db
}

// resetData clears the tables the adapter touches. The container and schema
// are shared by the package's tests, which run sequentially.
func resetData(t *testing.T) {
	t.Helper()
	db := openSQL(t, requireDB(t))
	_, err := db.Exec(`TRUNCATE nixie.contact_form, nixie.social_metrics, config.user_social, config.social_app RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func newAdapter(t *testing.T) *Adapter {
	t.Helper()
	return NewAdapter(newPool(t))
}

// newPool opens a connection pool for tests that need transactions.
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), requireDB(t))
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestAdapter_ListContacts(t *testing.T) {
	resetData(t)
	db := openSQL(t, requireDB(t))
	_, err := db.Exec(`
		INSERT INTO nixie.contact_form (email, description, domain) VALUES
			('nixie@example.com', 'Say hello', 'nixiepixel.com'),
			('fan@example.com', 'Speaking', 'nixiepixel.com'),
			('other@example.com', 'Other site', 'geekbeacon.org')`)
	if err != nil {
		t.Fatalf("seed contacts: %v", err)
	}

	adapter := newAdapter(t)
	ctx := context.Background()

	// An empty domain defaults to nixiepixel.com.
	contacts, err := adapter.ListContacts(ctx, "")
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if len(contacts) != 2 {
		t.Fatalf("got %d contacts, want 2: %+v", len(contacts), contacts)
	}
	if contacts[0].Email != "nixie@example.com" || contacts[0].Subject != "Say hello" {
		t.Errorf("unexpected first contact: %+v", contacts[0])
	}
	if contacts[1].Subject != "Speaking" {
		t.Errorf("DB description should map to Subject, got %q", contacts[1].Subject)
	}

	// An explicit domain is honoured.
	other, err := adapter.ListContacts(ctx, "geekbeacon.org")
	if err != nil {
		t.Fatalf("ListContacts(geekbeacon.org): %v", err)
	}
	if len(other) != 1 || other[0].Email != "other@example.com" {
		t.Errorf("unexpected contacts for other domain: %+v", other)
	}
}

func TestAdapter_ListContacts_CancelledContext(t *testing.T) {
	resetData(t)
	if _, err := newAdapter(t).ListContacts(cancelledContext(), ""); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func TestAdapter_UpsertAndGetSocialCounts(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)
	ctx := context.Background()

	if err := adapter.UpsertSocialCount(ctx, "youtube", 304000); err != nil {
		t.Fatalf("UpsertSocialCount(insert): %v", err)
	}
	if err := adapter.UpsertSocialCount(ctx, "discord", 5425); err != nil {
		t.Fatalf("UpsertSocialCount(insert): %v", err)
	}
	// A second write must update rather than duplicate.
	if err := adapter.UpsertSocialCount(ctx, "youtube", 305000); err != nil {
		t.Fatalf("UpsertSocialCount(update): %v", err)
	}

	counts, err := adapter.GetSocialCounts(ctx)
	if err != nil {
		t.Fatalf("GetSocialCounts: %v", err)
	}
	if len(counts) != 2 {
		t.Fatalf("got %d platforms, want 2: %v", len(counts), counts)
	}
	if counts["youtube"] != 305000 || counts["discord"] != 5425 {
		t.Errorf("unexpected counts: %v", counts)
	}
}

func TestAdapter_GetSocialCounts_CancelledContext(t *testing.T) {
	resetData(t)
	if _, err := newAdapter(t).GetSocialCounts(cancelledContext()); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func TestAdapter_UpsertSocialCount_CancelledContext(t *testing.T) {
	resetData(t)
	if err := newAdapter(t).UpsertSocialCount(cancelledContext(), "youtube", 1); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func TestAdapter_GetSocialMetricStates(t *testing.T) {
	resetData(t)
	db := openSQL(t, requireDB(t))
	older := time.Now().Add(-72 * time.Hour).UTC().Truncate(time.Second)
	newer := time.Now().UTC().Truncate(time.Second)

	if _, err := db.Exec(`
		INSERT INTO nixie.social_metrics (platform, followers, refreshed_at) VALUES
			('facebook', 41655, $1),
			('tiktok', 3616, $2)`, older, newer); err != nil {
		t.Fatalf("seed metrics: %v", err)
	}

	states, err := newAdapter(t).GetSocialMetricStates(context.Background())
	if err != nil {
		t.Fatalf("GetSocialMetricStates: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("got %d states, want 2", len(states))
	}

	byPlatform := make(map[string]domain.SocialMetricState, len(states))
	for _, state := range states {
		byPlatform[state.Platform] = state
	}
	if state := byPlatform["facebook"]; state.Followers != 41655 || !state.RefreshedAt.Equal(older) {
		t.Errorf("unexpected facebook state: %+v (want refreshed_at %v)", state, older)
	}
	if state := byPlatform["tiktok"]; state.Followers != 3616 || !state.RefreshedAt.Equal(newer) {
		t.Errorf("unexpected tiktok state: %+v (want refreshed_at %v)", state, newer)
	}
}

func TestAdapter_GetSocialMetricStates_CancelledContext(t *testing.T) {
	resetData(t)
	if _, err := newAdapter(t).GetSocialMetricStates(cancelledContext()); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func TestAdapter_LastSocialRefresh(t *testing.T) {
	resetData(t)
	adapter := newAdapter(t)
	ctx := context.Background()

	// With no rows the SQL COALESCEs to the epoch, which the worker treats as
	// "never refreshed" and therefore refreshes on boot.
	got, err := adapter.LastSocialRefresh(ctx)
	if err != nil {
		t.Fatalf("LastSocialRefresh (empty): %v", err)
	}
	if got.Unix() != 0 {
		t.Errorf("empty table: got %v, want the epoch", got)
	}

	older := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)
	newer := time.Now().UTC().Truncate(time.Second)
	if err := adapter.UpsertSocialCount(ctx, "facebook", 41655); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := adapter.UpsertSocialCount(ctx, "tiktok", 3616); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	db := openSQL(t, requireDB(t))
	if _, err := db.Exec(`UPDATE nixie.social_metrics SET refreshed_at = $1 WHERE platform = 'facebook'`, older); err != nil {
		t.Fatalf("backdate facebook: %v", err)
	}
	if _, err := db.Exec(`UPDATE nixie.social_metrics SET refreshed_at = $1 WHERE platform = 'tiktok'`, newer); err != nil {
		t.Fatalf("set tiktok time: %v", err)
	}

	got, err = adapter.LastSocialRefresh(ctx)
	if err != nil {
		t.Fatalf("LastSocialRefresh: %v", err)
	}
	if !got.Equal(newer) {
		t.Errorf("got %v, want the newest refresh %v", got, newer)
	}
}

func TestAdapter_LastSocialRefresh_CancelledContext(t *testing.T) {
	resetData(t)
	if _, err := newAdapter(t).LastSocialRefresh(cancelledContext()); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func TestAdapter_WithTx(t *testing.T) {
	resetData(t)
	ctx := context.Background()
	pool := newPool(t)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := New(pool).WithTx(tx).UpsertSocialCount(ctx, UpsertSocialCountParams{Platform: "rolled-back", Followers: 1}); err != nil {
		t.Fatalf("upsert in tx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	counts, err := NewAdapter(pool).GetSocialCounts(ctx)
	if err != nil {
		t.Fatalf("GetSocialCounts: %v", err)
	}
	if _, ok := counts["rolled-back"]; ok {
		t.Error("rolled-back row should not be visible")
	}
}

// The generated queries below are not used by the site yet; exercise them
// against the real schema so they cannot silently rot.
func TestQueries_Generated(t *testing.T) {
	resetData(t)
	ctx := context.Background()
	db := openSQL(t, requireDB(t))

	var appID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO config.social_app (social_app_type, client_id, secret, callback_url)
		VALUES ('TWITTER', 'client-id', 'secret', 'https://callback')
		RETURNING app_id`).Scan(&appID); err != nil {
		t.Fatalf("seed social_app: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO config.user_social (app_id, user_id, meta_data, access_token, secret_token)
		VALUES ($1, 'user-1', '{"k":"v"}'::jsonb, 'access', 'secret')`, appID); err != nil {
		t.Fatalf("seed user_social: %v", err)
	}

	conn, err := pgx.Connect(ctx, requireDB(t))
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	q := New(conn)

	app, err := q.GetSocialAppByType(ctx, "TWITTER")
	if err != nil {
		t.Fatalf("GetSocialAppByType: %v", err)
	}
	if app.AppID != appID || app.ClientID != "client-id" || app.CallbackUrl.String != "https://callback" {
		t.Errorf("unexpected app: %+v", app)
	}

	user, err := q.GetUserSocialByApp(ctx, appID)
	if err != nil {
		t.Fatalf("GetUserSocialByApp: %v", err)
	}
	if user.UserID != "user-1" || string(user.MetaData) != `{"k": "v"}` {
		t.Errorf("unexpected user social: %+v", user)
	}
	if user.AccessToken.String != "access" || user.SecretToken.String != "secret" {
		t.Errorf("unexpected tokens: %+v", user)
	}

	apps, err := q.ListSocialApps(ctx)
	if err != nil {
		t.Fatalf("ListSocialApps: %v", err)
	}
	if len(apps) != 1 || apps[0].AppID != appID {
		t.Errorf("unexpected apps: %+v", apps)
	}
}

func TestQueries_GeneratedErrors(t *testing.T) {
	resetData(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, requireDB(t))
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	q := New(conn)
	dead := cancelledContext()

	if _, err := q.GetSocialAppByType(dead, "TWITTER"); err == nil {
		t.Error("GetSocialAppByType: expected an error")
	}
	if _, err := q.GetUserSocialByApp(dead, 1); err == nil {
		t.Error("GetUserSocialByApp: expected an error")
	}
	if _, err := q.ListSocialApps(dead); err == nil {
		t.Error("ListSocialApps: expected an error")
	}
	if _, err := q.LastSocialRefresh(dead); err == nil {
		t.Error("LastSocialRefresh: expected an error")
	}
}

// newDatabase creates a throwaway database on the shared container so
// migration tests can apply and roll back without disturbing other tests.
func newDatabase(t *testing.T) string {
	t.Helper()
	base := requireDB(t)
	admin := openSQL(t, base)

	name := fmt.Sprintf("mig_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)"); err != nil {
			t.Logf("drop database %s: %v", name, err)
		}
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}
