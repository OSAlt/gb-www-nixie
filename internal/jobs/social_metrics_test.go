package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/adapters/social"
	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/mock"
)

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

func newTestWorker(db ports.DBService, uow ports.UnitOfWork, fetcher *social.Aggregator) *SocialMetricsWorker {
	return &SocialMetricsWorker{
		Config:     &config.Config{SocialRefreshSeconds: 86400},
		DB:         db,
		UnitOfWork: uow,
		Fetcher:    fetcher,
	}
}

func runWorker(w *SocialMetricsWorker) error {
	return w.Work(context.Background(), &river.Job[SocialMetricsArgs]{})
}

func TestSocialMetricsWorker_RefreshesStaleData(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Return(time.Unix(0, 0), nil)

	tx := mocks.NewMockTransaction(t)
	tx.EXPECT().UpsertSocialCount(mock.Anything, "youtube", 304000).Return(nil)

	w := newTestWorker(db, transactionUoW(t, tx), countingAggregator(t, 304000, nil))
	if err := runWorker(w); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

func TestSocialMetricsWorker_SkipsFreshData(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Return(time.Now(), nil)

	// No Fetch expectation: a refresh would fail the test.
	fetcher := mocks.NewMockFetcher(t)
	fetcher.EXPECT().Key().Return("youtube").Maybe()

	w := newTestWorker(db, mocks.NewMockUnitOfWork(t), social.NewAggregator(fetcher))
	if err := runWorker(w); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

func TestSocialMetricsWorker_RefreshesWhenLastRefreshFails(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Return(time.Time{}, errors.New("db down"))

	tx := mocks.NewMockTransaction(t)
	tx.EXPECT().UpsertSocialCount(mock.Anything, "youtube", 304000).Return(nil)

	w := newTestWorker(db, transactionUoW(t, tx), countingAggregator(t, 304000, nil))
	if err := runWorker(w); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

func TestSocialMetricsWorker_NoCountsIsRetryable(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Return(time.Unix(0, 0), nil)

	// No WithTx expectation: a write would fail the test.
	w := newTestWorker(db, mocks.NewMockUnitOfWork(t), countingAggregator(t, 0, errors.New("upstream down")))
	if err := runWorker(w); err == nil {
		t.Fatal("expected an error so River retries")
	}
}

func TestSocialMetricsWorker_UpsertErrorIsRetryable(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Return(time.Unix(0, 0), nil)

	tx := mocks.NewMockTransaction(t)
	tx.EXPECT().UpsertSocialCount(mock.Anything, "youtube", 304000).Return(errors.New("db down"))

	w := newTestWorker(db, transactionUoW(t, tx), countingAggregator(t, 304000, nil))
	if err := runWorker(w); err == nil {
		t.Fatal("expected an error so River retries")
	}
}

func TestSocialMetricsWorker_PanicIsContained(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Return(time.Unix(0, 0), nil)

	tx := mocks.NewMockTransaction(t)
	tx.EXPECT().UpsertSocialCount(mock.Anything, "youtube", 304000).
		Run(func(context.Context, string, int) { panic("simulated db panic") }).
		Return(nil)

	w := newTestWorker(db, transactionUoW(t, tx), countingAggregator(t, 304000, nil))
	if err := runWorker(w); err == nil {
		t.Fatal("expected the panic to be reported as an error")
	}
}

func TestSocialMetricsWorker_PanicInFetcherIsContained(t *testing.T) {
	fetcher := mocks.NewMockFetcher(t)
	fetcher.EXPECT().Key().Return("youtube").Maybe()
	fetcher.EXPECT().Fetch(mock.Anything).Run(func(context.Context) {
		panic("simulated adapter bug")
	}).Return(0, nil)

	db := mocks.NewMockDBService(t)
	db.EXPECT().LastSocialRefresh(mock.Anything).Return(time.Unix(0, 0), nil)

	// No WithTx expectation: the panicking platform must not be written.
	w := newTestWorker(db, mocks.NewMockUnitOfWork(t), social.NewAggregator(fetcher))
	if err := runWorker(w); err == nil {
		t.Fatal("expected the panic to be reported as an error")
	}
}

func TestSocialMetricsWorker_RequiresDependencies(t *testing.T) {
	fetcher := social.NewAggregator()
	uow := mocks.NewMockUnitOfWork(t)

	tests := map[string]*SocialMetricsWorker{
		"no config":   {DB: mocks.NewMockDBService(t), UnitOfWork: uow, Fetcher: fetcher},
		"no database": {Config: &config.Config{SocialRefreshSeconds: 86400}, UnitOfWork: uow, Fetcher: fetcher},
		"no uow":      {Config: &config.Config{SocialRefreshSeconds: 86400}, DB: mocks.NewMockDBService(t), Fetcher: fetcher},
		"no fetcher":  {Config: &config.Config{SocialRefreshSeconds: 86400}, DB: mocks.NewMockDBService(t), UnitOfWork: uow},
	}

	for name, w := range tests {
		t.Run(name, func(t *testing.T) {
			if err := runWorker(w); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestPeriodicJobs(t *testing.T) {
	cfg := &config.Config{SocialRefreshSeconds: 86400}

	if jobs := PeriodicJobs(cfg, nil); len(jobs) != 0 {
		t.Fatalf("PeriodicJobs with no fetcher = %d jobs, want 0", len(jobs))
	}
	if jobs := PeriodicJobs(cfg, social.NewAggregator()); len(jobs) != 1 {
		t.Fatalf("PeriodicJobs = %d jobs, want 1", len(jobs))
	}
	if jobs := PeriodicJobs(nil, social.NewAggregator()); len(jobs) != 0 {
		t.Fatalf("PeriodicJobs with no config = %d jobs, want 0", len(jobs))
	}
}

func TestNewWorkers(t *testing.T) {
	// Missing dependencies register nothing but must not fail.
	workers, err := NewWorkers(Deps{})
	if err != nil {
		t.Fatalf("NewWorkers: %v", err)
	}
	if workers == nil {
		t.Fatal("expected a workers bundle")
	}
}
