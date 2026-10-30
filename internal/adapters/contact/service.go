package contact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
)

const (
	// ContactCacheKey is the shared row holding the contact-form subjects.
	ContactCacheKey = "contact:subjects"

	defaultSubjectsTTL = 6 * time.Hour

	// contactRefreshSlack keeps a periodic job that fires slightly before the
	// cache expires (because the interval and TTL share one value and the
	// fetch itself takes time) from skipping the refresh, which would
	// otherwise leave the cache stale for a whole cycle. It also lets a
	// refresh attempt proceed when the cache is only marginally fresh, so a
	// transient upstream outage can recover on the next run.
	contactRefreshSlack = 5 * time.Minute
)

// Service serves the contact-form subject list. With a ContentCacheStore it is
// read-through: requests serve the cached list and the refresh job owns
// freshness. Without one it falls back to the upstream API (10s timeout).
type Service struct {
	cfg    *config.Config
	store  ports.ContentCacheStore
	lister ports.DBService
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

// WithStore makes the service read and write through a shared cache. It must
// be called before the service is used concurrently.
func (s *Service) WithStore(store ports.ContentCacheStore) *Service {
	s.store = store
	return s
}

// WithLister gives the service a local source for the contact subjects. When
// configured it replaces the old remote social.geekbeacon.org fetch entirely.
func (s *Service) WithLister(lister ports.DBService) *Service {
	s.lister = lister
	return s
}

func (s *Service) GetSubjects(ctx context.Context) ([]string, error) {
	if subjects, ok := s.fromStore(ctx); ok {
		return subjects, nil
	}

	subjects, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	s.save(ctx, subjects)
	return subjects, nil
}

// Refresh fetches the subject list and stores it. It is a no-op while the
// shared cache is fresh.
func (s *Service) Refresh(ctx context.Context) (int, error) {
	if s.store != nil {
		entry, err := s.store.LoadContentCache(ctx, ContactCacheKey)
		switch {
		case err == nil && time.Now().Add(contactRefreshSlack).Before(entry.ExpiresAt):
			slog.Info("contact subjects cache is fresh; skipping refresh", "expires_at", entry.ExpiresAt)
			return 0, nil
		case err != nil && !errors.Is(err, ports.ErrContentCacheMiss):
			slog.Warn("contact subjects cache read failed; refreshing anyway", "error", err)
		}
	}

	subjects, err := s.fetch(ctx)
	if err != nil {
		return 0, err
	}
	if len(subjects) == 0 {
		return 0, errors.New("contact: upstream returned no subjects")
	}
	s.save(ctx, subjects)
	return len(subjects), nil
}

// CacheState reports the shared cache for health checks.
func (s *Service) CacheState(ctx context.Context) (ports.CacheState, error) {
	state := ports.CacheState{Key: ContactCacheKey, Missing: true}
	if s.store == nil {
		return state, nil
	}

	entry, err := s.store.LoadContentCache(ctx, ContactCacheKey)
	if err != nil {
		if errors.Is(err, ports.ErrContentCacheMiss) {
			return state, nil
		}
		return state, err
	}
	var subjects []string
	if err := json.Unmarshal(entry.Payload, &subjects); err != nil {
		return state, fmt.Errorf("decode contact subjects: %w", err)
	}
	state.Items = len(subjects)
	state.FetchedAt = entry.FetchedAt
	state.ExpiresAt = entry.ExpiresAt
	state.Stale = time.Now().After(entry.ExpiresAt)
	state.Missing = false
	return state, nil
}

func (s *Service) fromStore(ctx context.Context) ([]string, bool) {
	if s.store == nil {
		return nil, false
	}
	entry, err := s.store.LoadContentCache(ctx, ContactCacheKey)
	if err != nil {
		if !errors.Is(err, ports.ErrContentCacheMiss) {
			slog.Warn("contact subjects cache read failed", "error", err)
		}
		return nil, false
	}
	var subjects []string
	if err := json.Unmarshal(entry.Payload, &subjects); err != nil {
		slog.Warn("contact subjects payload is invalid", "error", err)
		return nil, false
	}
	if len(subjects) == 0 {
		return nil, false
	}
	return subjects, true
}

func (s *Service) save(ctx context.Context, subjects []string) {
	if s.store == nil {
		return
	}
	payload, err := json.Marshal(subjects)
	if err != nil {
		slog.Warn("encode contact subjects", "error", err)
		return
	}
	if err := s.store.SaveContentCache(ctx, ContactCacheKey, payload, time.Now().Add(s.ttl())); err != nil {
		slog.Warn("save contact subjects cache", "error", err)
	}
}

func (s *Service) ttl() time.Duration {
	if s.cfg != nil && s.cfg.ContactRefreshSeconds > 0 {
		return time.Duration(s.cfg.ContactRefreshSeconds) * time.Second
	}
	return defaultSubjectsTTL
}

func (s *Service) fetch(ctx context.Context) ([]string, error) {
	if s.lister == nil {
		// No database is configured: fall back to the built-in subject list so
		// the contact form keeps working without any upstream dependency.
		return []string{
			"Say Hello",
			"Speaking Engagements",
			"Business Opportunity",
			"Content Collaboration",
		}, nil
	}

	contacts, err := s.lister.ListContacts(ctx, "nixiepixel.com")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contact list: %w", err)
	}

	subjects := make([]string, 0, len(contacts))
	for _, c := range contacts {
		if c.Subject != "" {
			subjects = append(subjects, c.Subject)
		}
	}
	if len(subjects) == 0 {
		return nil, errors.New("contact: no subjects available")
	}
	return subjects, nil
}
