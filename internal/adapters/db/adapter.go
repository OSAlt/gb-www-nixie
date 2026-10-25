package db

import (
	"context"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/domain"
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
	// Values are refreshed daily by the social metrics worker in
	// cmd/server/main.go (see doc/social_migration.md); the seeded rows keep
	// the chart working until the first refresh.
	rows, err := a.queries.GetSocialCounts(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.Platform] = int(row.Followers)
	}
	return counts, nil
}

func (a *Adapter) UpsertSocialCount(ctx context.Context, platform string, followers int) error {
	return a.queries.UpsertSocialCount(ctx, UpsertSocialCountParams{
		Platform:  platform,
		Followers: int64(followers),
	})
}

// GetSocialMetricStates exposes the same rows as GetSocialCounts plus their
// refreshed_at timestamp, which the health endpoint uses to spot a platform
// whose count has silently stopped updating.
func (a *Adapter) GetSocialMetricStates(ctx context.Context) ([]domain.SocialMetricState, error) {
	rows, err := a.queries.GetSocialCounts(ctx)
	if err != nil {
		return nil, err
	}
	states := make([]domain.SocialMetricState, 0, len(rows))
	for _, row := range rows {
		states = append(states, domain.SocialMetricState{
			Platform:    row.Platform,
			Followers:   int(row.Followers),
			RefreshedAt: row.RefreshedAt.Time,
		})
	}
	return states, nil
}

func (a *Adapter) LastSocialRefresh(ctx context.Context) (time.Time, error) {
	ts, err := a.queries.LastSocialRefresh(ctx)
	if err != nil {
		return time.Time{}, err
	}
	return ts.Time, nil
}
