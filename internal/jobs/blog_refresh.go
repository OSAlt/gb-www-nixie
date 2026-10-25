package jobs

import (
	"context"
	"errors"
	"log/slog"

	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/riverqueue/river"
)

// BlogRefreshArgs takes no arguments: there is one shared blog cache.
type BlogRefreshArgs struct{}

func (BlogRefreshArgs) Kind() string { return "blog_refresh" }

var _ river.Worker[BlogRefreshArgs] = (*BlogRefreshWorker)(nil)

// BlogRefreshWorker keeps the shared blog cache warm so /blog never waits on
// four RSS feeds. Freshness lives in the cache row, so RunOnStart is cheap.
type BlogRefreshWorker struct {
	river.WorkerDefaults[BlogRefreshArgs]

	Refresher ports.BlogRefresher
}

func (w *BlogRefreshWorker) Work(ctx context.Context, _ *river.Job[BlogRefreshArgs]) error {
	if w.Refresher == nil {
		return errors.New("blog refresh: no refresher")
	}

	count, err := w.Refresher.Refresh(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		slog.Info("blog cache is fresh; nothing to refresh")
		return nil
	}
	slog.Info("blog cache refreshed", "posts", count)
	return nil
}
