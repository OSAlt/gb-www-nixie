package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/mock"
)

func runMediaWorker(w *MediaRefreshWorker) error {
	return w.Work(context.Background(), &river.Job[MediaRefreshArgs]{})
}

func TestMediaRefreshWorker_Refreshes(t *testing.T) {
	refresher := mocks.NewMockMediaRefresher(t)
	refresher.EXPECT().Refresh(mock.Anything, mediaRefreshCount).Return(10, nil)

	if err := runMediaWorker(&MediaRefreshWorker{Refresher: refresher}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

func TestMediaRefreshWorker_FreshIsNotAnError(t *testing.T) {
	refresher := mocks.NewMockMediaRefresher(t)
	refresher.EXPECT().Refresh(mock.Anything, mediaRefreshCount).Return(0, nil)

	if err := runMediaWorker(&MediaRefreshWorker{Refresher: refresher}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

func TestMediaRefreshWorker_ErrorIsRetryable(t *testing.T) {
	refresher := mocks.NewMockMediaRefresher(t)
	refresher.EXPECT().Refresh(mock.Anything, mediaRefreshCount).Return(0, errors.New("upstream down"))

	if err := runMediaWorker(&MediaRefreshWorker{Refresher: refresher}); err == nil {
		t.Fatal("expected an error so River retries")
	}
}

func TestMediaRefreshWorker_RequiresRefresher(t *testing.T) {
	if err := runMediaWorker(&MediaRefreshWorker{}); err == nil {
		t.Fatal("expected a validation error")
	}
}
