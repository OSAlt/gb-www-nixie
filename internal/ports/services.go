package ports

import (
	"context"

	"github.com/OSAlt/gb-www-nixie/internal/domain"
)

type MediaService interface {
	GetRecentPosts(ctx context.Context, count int) ([]domain.MediaPost, error)
}

type ContactService interface {
	GetSubjects(ctx context.Context) ([]string, error)
}

type DBService interface {
	ListContacts(ctx context.Context, domain string) ([]domain.ContactMessage, error)
	GetSocialCounts(ctx context.Context) (map[string]int, error)
	GetInstagramActivity(ctx context.Context, limit int) ([]any, error)
}
