package ports

import (
	"context"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/domain"
)

type MediaService interface {
	GetRecentPosts(ctx context.Context, count int) ([]domain.MediaPost, error)
	CacheState(ctx context.Context) (CacheState, error)
}

type ContactService interface {
	GetSubjects(ctx context.Context) ([]string, error)
	CacheState(ctx context.Context) (CacheState, error)
}

// ContactRefresher fetches the subject list and stores it.
type ContactRefresher interface {
	Refresh(ctx context.Context) (int, error)
}

type DBService interface {
	ListContacts(ctx context.Context, domain string) ([]domain.ContactMessage, error)
	GetSocialCounts(ctx context.Context) (map[string]int, error)
	// GetSocialMetricStates returns each platform's count and the time it was
	// last refreshed, which the health endpoint uses for staleness checks.
	GetSocialMetricStates(ctx context.Context) ([]domain.SocialMetricState, error)
	UpsertSocialCount(ctx context.Context, platform string, followers int) error
	// LastSocialRefresh is the newest refreshed_at across all platforms; it
	// lets the worker skip a boot fetch when stored data is still fresh.
	LastSocialRefresh(ctx context.Context) (time.Time, error)
}

type BlogService interface {
	GetRecentPosts(ctx context.Context) ([]domain.BlogPost, error)
	CacheState(ctx context.Context) (CacheState, error)
}

// BlogRefresher fetches the blog feeds and stores them.
type BlogRefresher interface {
	Refresh(ctx context.Context) (int, error)
}
