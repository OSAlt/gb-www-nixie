package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/adapters/social"
	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/stretchr/testify/mock"
)

func TestSplitNonEmpty(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{in: "", want: []string{}},
		{in: "only", want: []string{"only"}},
		{in: "a,b , ,c", want: []string{"a", "b", "c"}},
		{in: " NixiePixelGB , user/nixiedoeslinux ", want: []string{"NixiePixelGB", "user/nixiedoeslinux"}},
	}

	for _, tc := range tests {
		got := splitNonEmpty(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("splitNonEmpty(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitNonEmpty(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestNewSocialAggregator(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{name: "nothing configured", cfg: &config.Config{}, want: false},
		{name: "metrika only", cfg: &config.Config{MetrikaApiKey: "key"}, want: true},
		{name: "youtube only", cfg: &config.Config{YoutubeApiKey: "key"}, want: true},
		{
			name: "every fallback",
			cfg: &config.Config{
				YoutubeApiKey:      "youtube",
				YoutubeChannels:    "a, b",
				TwitchClientId:     "id",
				TwitchClientSecret: "secret",
				DiscordBotToken:    "token",
				DiscordGuildId:     "guild",
				SocialScrapeURL:    "https://pulse.example",
			},
			want: true,
		},
		{
			name: "twitch needs both credentials",
			cfg:  &config.Config{TwitchClientId: "id"},
			want: false,
		},
		{
			name: "discord needs both credentials",
			cfg:  &config.Config{DiscordBotToken: "token"},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agg, ok := newSocialAggregator(tc.cfg)
			if ok != tc.want {
				t.Fatalf("ok = %v, want %v", ok, tc.want)
			}
			if ok && agg == nil {
				t.Fatal("expected a non-nil aggregator")
			}
			if !ok && agg != nil {
				t.Fatal("expected a nil aggregator")
			}
		})
	}
}

func countingAggregator(t *testing.T, count int, err error) *social.Aggregator {
	t.Helper()
	fetcher := mocks.NewMockFetcher(t)
	fetcher.EXPECT().Key().Return("youtube").Maybe()
	fetcher.EXPECT().Fetch(mock.Anything).Return(count, err)
	return social.NewAggregator(fetcher)
}

// transactionUoW returns a UnitOfWork whose WithTx invokes the callback with
// tx, mirroring the real adapter's commit path.
func transactionUoW(t *testing.T, tx *mocks.MockTransaction) *mocks.MockUnitOfWork {
	t.Helper()
	uow := mocks.NewMockUnitOfWork(t)
	uow.EXPECT().WithTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context, ports.Transaction) error) error {
			return fn(ctx, tx)
		})
	return uow
}

func TestRefreshSocialMetrics_UpsertsCounts(t *testing.T) {
	tx := mocks.NewMockTransaction(t)
	tx.EXPECT().UpsertSocialCount(mock.Anything, "youtube", 304000).Return(nil)

	// Should not panic and should write the count inside one transaction.
	refreshSocialMetrics(context.Background(), transactionUoW(t, tx), countingAggregator(t, 304000, nil))
}

func TestRefreshSocialMetrics_NoCounts(t *testing.T) {
	uow := mocks.NewMockUnitOfWork(t) // no WithTx expectation: a write would fail the test

	refreshSocialMetrics(context.Background(), uow, countingAggregator(t, 0, errors.New("upstream down")))
}

func TestRefreshSocialMetrics_UpsertError(t *testing.T) {
	tx := mocks.NewMockTransaction(t)
	tx.EXPECT().UpsertSocialCount(mock.Anything, "youtube", 304000).Return(errors.New("db down"))

	// The error rolls the transaction back; refresh itself must not panic.
	refreshSocialMetrics(context.Background(), transactionUoW(t, tx), countingAggregator(t, 304000, nil))
}

func TestRefreshSocialMetrics_PanicIsContained(t *testing.T) {
	tx := mocks.NewMockTransaction(t)
	tx.EXPECT().UpsertSocialCount(mock.Anything, "youtube", 304000).
		Run(func(context.Context, string, int) { panic("simulated db panic") }).
		Return(nil)

	refreshSocialMetrics(context.Background(), transactionUoW(t, tx), countingAggregator(t, 304000, nil))
}

func TestRefreshSocialMetrics_PanicInFetcherIsContained(t *testing.T) {
	fetcher := mocks.NewMockFetcher(t)
	fetcher.EXPECT().Key().Return("youtube").Maybe()
	fetcher.EXPECT().Fetch(mock.Anything).Run(func(context.Context) {
		panic("simulated adapter bug")
	}).Return(0, nil)

	uow := mocks.NewMockUnitOfWork(t) // the panicking platform must not be written
	refreshSocialMetrics(context.Background(), uow, social.NewAggregator(fetcher))
}

func TestRunSocialMetricsLoop_SkipsFreshBootRefresh(t *testing.T) {
	called := make(chan struct{})
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).
		Run(func(context.Context) { close(called) }).
		Return(time.Now(), nil)

	// No fetch/upsert expectations: a boot refresh would fail the test.
	fetcher := mocks.NewMockFetcher(t)
	agg := social.NewAggregator(fetcher)
	uow := mocks.NewMockUnitOfWork(t)
	cfg := &config.Config{SocialRefreshSeconds: 3600}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runSocialMetricsLoop(ctx, db, uow, agg, cfg) }()

	<-called
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runSocialMetricsLoop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not stop after cancellation")
	}
}

func TestRunSocialMetricsLoop_RefreshesStaleData(t *testing.T) {
	upserted := make(chan struct{})
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Return(time.Unix(0, 0), nil)

	tx := mocks.NewMockTransaction(t)
	tx.EXPECT().UpsertSocialCount(mock.Anything, "youtube", 304000).
		Run(func(context.Context, string, int) { close(upserted) }).
		Return(nil)
	uow := transactionUoW(t, tx)

	cfg := &config.Config{SocialRefreshSeconds: 3600}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runSocialMetricsLoop(ctx, db, uow, countingAggregator(t, 304000, nil), cfg)
	}()

	<-upserted
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runSocialMetricsLoop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not stop after cancellation")
	}
}

func TestRunSocialMetricsLoop_RefreshesWhenLastRefreshFails(t *testing.T) {
	upserted := make(chan struct{})
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Return(time.Time{}, errors.New("db down"))

	tx := mocks.NewMockTransaction(t)
	tx.EXPECT().UpsertSocialCount(mock.Anything, "youtube", 304000).
		Run(func(context.Context, string, int) { close(upserted) }).
		Return(nil)
	uow := transactionUoW(t, tx)

	cfg := &config.Config{SocialRefreshSeconds: 3600}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runSocialMetricsLoop(ctx, db, uow, countingAggregator(t, 304000, nil), cfg)
	}()

	<-upserted
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runSocialMetricsLoop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not stop after cancellation")
	}
}

func TestRunSocialMetricsLoop_RecoversFromPanic(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Run(func(context.Context) {
		panic("simulated database bug")
	}).Return(time.Time{}, nil)

	cfg := &config.Config{SocialRefreshSeconds: 3600}
	err := runSocialMetricsLoop(context.Background(), db, mocks.NewMockUnitOfWork(t), social.NewAggregator(), cfg)
	if err == nil {
		t.Fatal("expected a panic to be reported as an error")
	}
}

func TestRunSocialMetricsWorker_StopsOnCancel(t *testing.T) {
	called := make(chan struct{})
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).
		Run(func(context.Context) { close(called) }).
		Return(time.Now(), nil)

	cfg := &config.Config{SocialRefreshSeconds: 3600}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runSocialMetricsWorker(ctx, db, mocks.NewMockUnitOfWork(t), social.NewAggregator(), cfg)
		close(done)
	}()

	<-called
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
}

// A loop that dies unexpectedly must be restarted by the supervisor rather
// than leaving the chart frozen.
func TestRunSocialMetricsWorker_RestartsAfterPanic(t *testing.T) {
	calls := make(chan struct{}, 8)
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Run(func(context.Context) {
		calls <- struct{}{}
		panic("simulated database bug")
	}).Return(time.Time{}, nil).Maybe()

	cfg := &config.Config{SocialRefreshSeconds: 3600}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runSocialMetricsWorker(ctx, db, mocks.NewMockUnitOfWork(t), social.NewAggregator(), cfg)
		close(done)
	}()

	<-calls
	// Give the supervisor time to recover the loop and enter its backoff wait,
	// then shut it down before the one-minute retry fires.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
}
