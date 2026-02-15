package ports

import (
	"context"
	"github.com/osalt/nixiesite/internal/domain"
)

type MediaService interface {
	GetRecentPosts(ctx context.Context, count int) ([]domain.MediaPost, error)
}

type ContactService interface {
	SendMessage(ctx context.Context, msg domain.ContactMessage) error
	GetSubjects(ctx context.Context) ([]string, error)
}
