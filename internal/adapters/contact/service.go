package contact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
)

const (
	defaultContactAPIURL = "https://social.geekbeacon.org/api/v1.0/contact/list"

	// ContactCacheKey is the shared row holding the contact-form subjects.
	ContactCacheKey = "contact:subjects"

	defaultSubjectsTTL = 6 * time.Hour
)

type contactItem struct {
	Email       string `json:"email"`
	Description string `json:"description"`
}

// Service serves the contact-form subject list. With a ContentCacheStore it is
// read-through: requests serve the cached list and the refresh job owns
// freshness. Without one it falls back to the upstream API (10s timeout).
type Service struct {
	client *http.Client
	url    string
	cfg    *config.Config
	store  ports.ContentCacheStore
}

func NewService(cfg *config.Config) *Service {
	return &Service{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		url: defaultContactAPIURL,
		cfg: cfg,
	}
}

// WithStore makes the service read and write through a shared cache. It must
// be called before the service is used concurrently.
func (s *Service) WithStore(store ports.ContentCacheStore) *Service {
	s.store = store
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
		case err == nil && time.Now().Before(entry.ExpiresAt):
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contact list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var items []contactItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	subjects := make([]string, 0, len(items))
	for _, item := range items {
		subjects = append(subjects, item.Description)
	}
	return subjects, nil
}
