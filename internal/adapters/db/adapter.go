package db

import (
	"context"

	"github.com/osalt/nixiesite/internal/domain"
)

type Adapter struct {
	queries *Queries
	db      DBTX
}

func NewAdapter(db DBTX) *Adapter {
	return &Adapter{
		queries: New(db),
		db:      db,
	}
}

func (a *Adapter) ListContacts(ctx context.Context, domainName string) ([]domain.ContactMessage, error) {
	if domainName == "" {
		domainName = "nixiepixel.com"
	}
	contacts, err := a.queries.ListContacts(ctx, domainName)
	if err != nil {
		return nil, err
	}

	result := make([]domain.ContactMessage, 0, len(contacts))
	for _, c := range contacts {
		result = append(result, domain.ContactMessage{
			Email:   c.Email,
			Subject: c.Description, // description in DB seems to be used as subject/title
		})
	}
	return result, nil
}

func (a *Adapter) GetSocialCounts(ctx context.Context) (map[string]int, error) {
	// Re-implementing the hardcoded counts for now, but via the DB-oriented adapter
	// In oldapi these were hardcoded in services/social.go
	return map[string]int{
		"youtube":   330000,
		"twitter":   31102,
		"twitch":    2500,
		"discord":   6336,
		"facebook":  50466,
		"instagram": 7498,
	}, nil
}

func (a *Adapter) GetInstagramActivity(ctx context.Context, limit int) ([]any, error) {
	// Re-implementing file-based instagram activity from oldapi as a placeholder
	// In a real scenario, this would fetch from DB or a cache updated by a worker
	return []any{}, nil
}
