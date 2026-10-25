package blog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/mmcdole/gofeed"
	"golang.org/x/sync/singleflight"
)

const (
	// BlogCacheKey is the shared row the /blog page reads and the refresh job
	// writes.
	BlogCacheKey = "blog:recent"

	defaultRefreshTTL = 30 * time.Minute
	refreshTimeout    = 30 * time.Second
)

// feedItem keeps the parsed date around for sorting. domain.BlogPost only
// carries a human-readable date, which does not sort correctly as a string.
type feedItem struct {
	post      domain.BlogPost
	published time.Time
}

type cacheEntry struct {
	posts     []domain.BlogPost
	expiresAt time.Time
}

// Service merges the configured feeds. With a ContentCacheStore it is
// read-through: request paths serve the shared last-known-good payload and the
// refresh job owns freshness.
type Service struct {
	cfg   *config.Config
	store ports.ContentCacheStore

	mu    sync.RWMutex
	cache cacheEntry

	refreshing atomic.Bool
	group      singleflight.Group
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

func (s *Service) GetRecentPosts(ctx context.Context) ([]domain.BlogPost, error) {
	if posts, ok := s.fromStore(ctx); ok {
		return posts, nil
	}

	s.mu.RLock()
	entry := s.cache
	s.mu.RUnlock()

	if len(entry.posts) > 0 {
		if time.Now().Before(entry.expiresAt) {
			return entry.posts, nil
		}
		s.refreshAsync()
		return entry.posts, nil
	}

	return s.refresh(ctx)
}

// Refresh fetches the feeds and stores them. It is a no-op while the shared
// cache is fresh.
func (s *Service) Refresh(ctx context.Context) (int, error) {
	if s.store != nil {
		entry, err := s.store.LoadContentCache(ctx, BlogCacheKey)
		switch {
		case err == nil && time.Now().Before(entry.ExpiresAt):
			slog.Info("blog cache is fresh; skipping refresh", "expires_at", entry.ExpiresAt)
			return 0, nil
		case err != nil && !errors.Is(err, ports.ErrContentCacheMiss):
			slog.Warn("blog cache read failed; refreshing anyway", "error", err)
		}
	}

	posts, err := s.forceRefresh(ctx)
	if err != nil {
		return 0, err
	}
	if len(posts) == 0 {
		return 0, errors.New("blog: no posts available")
	}
	return len(posts), nil
}

// CacheState reports the shared (or in-memory) cache for health checks.
func (s *Service) CacheState(ctx context.Context) (ports.CacheState, error) {
	state := ports.CacheState{Key: BlogCacheKey, Missing: true}

	if s.store != nil {
		entry, err := s.store.LoadContentCache(ctx, BlogCacheKey)
		if err != nil {
			if errors.Is(err, ports.ErrContentCacheMiss) {
				return state, nil
			}
			return state, err
		}
		var posts []domain.BlogPost
		if err := json.Unmarshal(entry.Payload, &posts); err != nil {
			return state, fmt.Errorf("decode blog cache: %w", err)
		}
		state.Items = len(posts)
		state.FetchedAt = entry.FetchedAt
		state.ExpiresAt = entry.ExpiresAt
		state.Stale = time.Now().After(entry.ExpiresAt)
		state.Missing = false
		return state, nil
	}

	s.mu.RLock()
	entry := s.cache
	s.mu.RUnlock()
	if len(entry.posts) > 0 {
		state.Items = len(entry.posts)
		state.ExpiresAt = entry.expiresAt
		state.Stale = time.Now().After(entry.expiresAt)
		state.Missing = false
	}
	return state, nil
}

func (s *Service) fromStore(ctx context.Context) ([]domain.BlogPost, bool) {
	if s.store == nil {
		return nil, false
	}
	entry, err := s.store.LoadContentCache(ctx, BlogCacheKey)
	if err != nil {
		if !errors.Is(err, ports.ErrContentCacheMiss) {
			slog.Warn("blog cache read failed", "error", err)
		}
		return nil, false
	}

	var posts []domain.BlogPost
	if err := json.Unmarshal(entry.Payload, &posts); err != nil {
		slog.Warn("blog cache payload is invalid", "error", err)
		return nil, false
	}
	if len(posts) == 0 {
		return nil, false
	}
	return posts, true
}

func (s *Service) refresh(ctx context.Context) ([]domain.BlogPost, error) {
	return s.doRefresh(ctx, "refresh", false)
}

func (s *Service) forceRefresh(ctx context.Context) ([]domain.BlogPost, error) {
	return s.doRefresh(ctx, "force-refresh", true)
}

func (s *Service) doRefresh(ctx context.Context, key string, force bool) ([]domain.BlogPost, error) {
	v, err, _ := s.group.Do(key, func() (any, error) {
		if !force {
			s.mu.RLock()
			fresh := len(s.cache.posts) > 0 && time.Now().Before(s.cache.expiresAt)
			posts := s.cache.posts
			s.mu.RUnlock()
			if fresh {
				return posts, nil
			}
		}

		posts, successes := s.fetch(ctx)
		if successes == 0 {
			// A total failure must not be cached, so a transient outage or a
			// fixed URL is picked up on the next attempt.
			return []domain.BlogPost(nil), nil
		}

		expiresAt := time.Now().Add(s.ttl())
		s.mu.Lock()
		s.cache = cacheEntry{posts: posts, expiresAt: expiresAt}
		s.mu.Unlock()

		if s.store != nil {
			payload, err := json.Marshal(posts)
			if err != nil {
				return nil, fmt.Errorf("encode blog cache: %w", err)
			}
			if err := s.store.SaveContentCache(ctx, BlogCacheKey, payload, expiresAt); err != nil {
				return nil, fmt.Errorf("save blog cache: %w", err)
			}
		}
		return posts, nil
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.([]domain.BlogPost), nil
}

func (s *Service) refreshAsync() {
	if !s.refreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.refreshing.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		defer cancel()
		if _, err := s.refresh(ctx); err != nil {
			slog.Warn("background blog refresh failed", "error", err)
		}
	}()
}

func (s *Service) ttl() time.Duration {
	if s.cfg != nil && s.cfg.BlogRefreshSeconds > 0 {
		return time.Duration(s.cfg.BlogRefreshSeconds) * time.Second
	}
	return defaultRefreshTTL
}

func (s *Service) fetch(ctx context.Context) ([]domain.BlogPost, int) {
	fp := gofeed.NewParser()
	var items []feedItem
	seen := make(map[string]struct{})
	successes := 0

	for _, url := range s.cfg.BlogURLs {
		feed, err := fp.ParseURLWithContext(url, ctx)
		if err != nil {
			slog.Warn("failed to fetch blog feed", "url", url, "error", err)
			continue
		}
		successes++

		for _, item := range feed.Items {
			post := domain.BlogPost{
				Title:       item.Title,
				Description: item.Description,
				Content:     item.Content,
				URL:         item.Link,
				Author:      "",
			}

			var published time.Time
			if item.PublishedParsed != nil {
				published = *item.PublishedParsed
				post.PublishedAt = item.PublishedParsed.Format("January 02, 2006")
			} else if item.UpdatedParsed != nil {
				published = *item.UpdatedParsed
				post.PublishedAt = item.UpdatedParsed.Format("January 02, 2006")
			}

			if len(item.Authors) > 0 {
				post.Author = item.Authors[0].Name
			}

			if len(item.Categories) > 0 {
				post.Categories = item.Categories
			}

			if item.Image != nil {
				post.ImageURL = item.Image.URL
			} else if len(item.Enclosures) > 0 {
				for _, enc := range item.Enclosures {
					if enc.Type == "image/jpeg" || enc.Type == "image/png" || enc.Type == "image/gif" {
						post.ImageURL = enc.URL
						break
					}
				}
			}

			// The same post can appear in more than one tag feed. Prefer the
			// link as the identity, falling back to title and date for feeds
			// that omit links.
			key := post.URL
			if key == "" {
				key = post.Title + "|" + post.PublishedAt
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			items = append(items, feedItem{post: post, published: published})
		}
	}

	// Newest first. Undated posts keep their feed order at the end.
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].published.IsZero() {
			return false
		}
		if items[j].published.IsZero() {
			return true
		}
		return items[i].published.After(items[j].published)
	})

	posts := make([]domain.BlogPost, len(items))
	for i, item := range items {
		posts[i] = item.post
	}
	return posts, successes
}
