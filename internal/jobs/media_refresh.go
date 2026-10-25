package jobs

import (
	"context"
	"errors"
	"log/slog"

	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/riverqueue/river"
)

// mediaRefreshCount is how many posts the cache keeps. The home page asks for
// six; the extra headroom means a short video burst does not empty the list.
const mediaRefreshCount = 10

// MediaRefreshArgs takes no arguments: there is one shared video cache.
type MediaRefreshArgs struct{}

func (MediaRefreshArgs) Kind() string { return "media_refresh" }

var _ river.Worker[MediaRefreshArgs] = (*MediaRefreshWorker)(nil)

// MediaRefreshWorker keeps the shared video cache warm so request paths never
// call the YouTube API. Freshness lives in the cache row, so RunOnStart after a
// deploy is free.
type MediaRefreshWorker struct {
	river.WorkerDefaults[MediaRefreshArgs]

	Refresher ports.MediaRefresher
}

func (w *MediaRefreshWorker) Work(ctx context.Context, _ *river.Job[MediaRefreshArgs]) error {
	if w.Refresher == nil {
		return errors.New("media refresh: no refresher")
	}

	count, err := w.Refresher.Refresh(ctx, mediaRefreshCount)
	if err != nil {
		return err
	}
	if count == 0 {
		slog.Info("media cache is fresh; nothing to refresh")
		return nil
	}
	slog.Info("media cache refreshed", "posts", count)
	return nil
}
