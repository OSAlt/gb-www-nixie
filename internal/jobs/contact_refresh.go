package jobs

import (
	"context"
	"errors"
	"log/slog"

	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/riverqueue/river"
)

// ContactRefreshArgs takes no arguments: there is one shared subject list.
type ContactRefreshArgs struct{}

func (ContactRefreshArgs) Kind() string { return "contact_subjects_refresh" }

var _ river.Worker[ContactRefreshArgs] = (*ContactRefreshWorker)(nil)

// ContactRefreshWorker keeps the contact-form subject list cached, so rendering
// / or /contact never waits on the 10-second upstream API.
type ContactRefreshWorker struct {
	river.WorkerDefaults[ContactRefreshArgs]

	Refresher ports.ContactRefresher
}

func (w *ContactRefreshWorker) Work(ctx context.Context, _ *river.Job[ContactRefreshArgs]) error {
	if w.Refresher == nil {
		return errors.New("contact refresh: no refresher")
	}

	count, err := w.Refresher.Refresh(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		slog.Info("contact subjects cache is fresh; nothing to refresh")
		return nil
	}
	slog.Info("contact subjects refreshed", "subjects", count)
	return nil
}
