package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/adapters/social"
	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/riverqueue/river"
)

// SocialMetricsArgs takes no arguments: the job refreshes every configured
// source at once.
type SocialMetricsArgs struct{}

func (SocialMetricsArgs) Kind() string { return "social_metrics_refresh" }

var _ river.Worker[SocialMetricsArgs] = (*SocialMetricsWorker)(nil)

// SocialMetricsWorker refreshes follower counts, storing each successful
// result as the new last-known-good value. All writes happen in one
// transaction, so a database error leaves the previous refresh fully intact.
type SocialMetricsWorker struct {
	river.WorkerDefaults[SocialMetricsArgs]

	Config     *config.Config
	DB         ports.DBService
	UnitOfWork ports.UnitOfWork
	Fetcher    *social.Aggregator
}

// Work implements river.Worker. Returning an error lets River retry transient
// failures (bounded by MaxAttempts on the periodic job).
func (w *SocialMetricsWorker) Work(ctx context.Context, _ *river.Job[SocialMetricsArgs]) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic during social metrics refresh", "panic", r, "stack", string(debug.Stack()))
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	if err := w.validate(); err != nil {
		return err
	}

	interval := time.Duration(w.Config.SocialRefreshSeconds) * time.Second
	if last, lastErr := w.DB.LastSocialRefresh(ctx); lastErr != nil {
		// A read failure is not a reason to skip the refresh; the write below
		// will fail too if the database is really gone.
		slog.Warn("social metrics: cannot read last refresh time; refreshing now", "error", lastErr)
	} else if age := time.Since(last); age < interval {
		// The periodic job runs on start, but a restart should not spend API
		// quota on data that only changes once a day.
		slog.Info("social metrics: stored data is still fresh; skipping refresh",
			"age", age.Round(time.Minute), "interval", interval)
		return nil
	}

	counts := w.Fetcher.Fetch(ctx)
	if len(counts) == 0 {
		return errors.New("social metrics: no platform fetches succeeded")
	}

	if err := w.UnitOfWork.WithTx(ctx, func(ctx context.Context, tx ports.Transaction) error {
		for platform, followers := range counts {
			if err := tx.UpsertSocialCount(ctx, platform, followers); err != nil {
				return fmt.Errorf("upsert %s: %w", platform, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("social metrics: storing counts: %w", err)
	}

	for platform, followers := range counts {
		slog.Info("social metrics refreshed", "platform", platform, "followers", followers)
	}
	return nil
}

func (w *SocialMetricsWorker) validate() error {
	switch {
	case w.Config == nil:
		return errors.New("social metrics: no config")
	case w.DB == nil:
		return errors.New("social metrics: no database service")
	case w.UnitOfWork == nil:
		return errors.New("social metrics: no unit of work")
	case w.Fetcher == nil:
		return errors.New("social metrics: no fetcher")
	}
	return nil
}
