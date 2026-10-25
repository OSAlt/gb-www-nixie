package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/mock"
)

func TestBlogRefreshWorker_Refreshes(t *testing.T) {
	refresher := mocks.NewMockBlogRefresher(t)
	refresher.EXPECT().Refresh(mock.Anything).Return(12, nil)

	w := &BlogRefreshWorker{Refresher: refresher}
	if err := w.Work(context.Background(), &river.Job[BlogRefreshArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

func TestBlogRefreshWorker_FreshIsNotAnError(t *testing.T) {
	refresher := mocks.NewMockBlogRefresher(t)
	refresher.EXPECT().Refresh(mock.Anything).Return(0, nil)

	w := &BlogRefreshWorker{Refresher: refresher}
	if err := w.Work(context.Background(), &river.Job[BlogRefreshArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

func TestBlogRefreshWorker_ErrorIsRetryable(t *testing.T) {
	refresher := mocks.NewMockBlogRefresher(t)
	refresher.EXPECT().Refresh(mock.Anything).Return(0, errors.New("feeds down"))

	w := &BlogRefreshWorker{Refresher: refresher}
	if err := w.Work(context.Background(), &river.Job[BlogRefreshArgs]{}); err == nil {
		t.Fatal("expected an error so River retries")
	}
}

func TestBlogRefreshWorker_RequiresRefresher(t *testing.T) {
	w := &BlogRefreshWorker{}
	if err := w.Work(context.Background(), &river.Job[BlogRefreshArgs]{}); err == nil {
		t.Fatal("expected a validation error")
	}
}

func TestContactRefreshWorker_Refreshes(t *testing.T) {
	refresher := mocks.NewMockContactRefresher(t)
	refresher.EXPECT().Refresh(mock.Anything).Return(4, nil)

	w := &ContactRefreshWorker{Refresher: refresher}
	if err := w.Work(context.Background(), &river.Job[ContactRefreshArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

func TestContactRefreshWorker_FreshIsNotAnError(t *testing.T) {
	refresher := mocks.NewMockContactRefresher(t)
	refresher.EXPECT().Refresh(mock.Anything).Return(0, nil)

	w := &ContactRefreshWorker{Refresher: refresher}
	if err := w.Work(context.Background(), &river.Job[ContactRefreshArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

func TestContactRefreshWorker_ErrorIsRetryable(t *testing.T) {
	refresher := mocks.NewMockContactRefresher(t)
	refresher.EXPECT().Refresh(mock.Anything).Return(0, errors.New("upstream down"))

	w := &ContactRefreshWorker{Refresher: refresher}
	if err := w.Work(context.Background(), &river.Job[ContactRefreshArgs]{}); err == nil {
		t.Fatal("expected an error so River retries")
	}
}

func TestContactRefreshWorker_RequiresRefresher(t *testing.T) {
	w := &ContactRefreshWorker{}
	if err := w.Work(context.Background(), &river.Job[ContactRefreshArgs]{}); err == nil {
		t.Fatal("expected a validation error")
	}
}
