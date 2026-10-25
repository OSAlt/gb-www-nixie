package blog

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/mmcdole/gofeed"
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

type Service struct {
	cfg   *config.Config
	cache cacheEntry
	mu    sync.RWMutex
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) GetRecentPosts(ctx context.Context) ([]domain.BlogPost, error) {
	s.mu.RLock()
	if time.Now().Before(s.cache.expiresAt) {
		posts := s.cache.posts
		s.mu.RUnlock()
		return posts, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double check after acquiring write lock
	if time.Now().Before(s.cache.expiresAt) {
		return s.cache.posts, nil
	}

	fp := gofeed.NewParser()
	var items []feedItem
	seen := make(map[string]struct{})
	successes := 0

	for _, url := range s.cfg.BlogURLs {
		feed, err := fp.ParseURLWithContext(url, ctx)
		if err != nil {
			fmt.Printf("Warning: failed to fetch feed from %s: %v\n", url, err)
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

			// Try to find an image URL
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

	allPosts := make([]domain.BlogPost, len(items))
	for i, item := range items {
		allPosts[i] = item.post
	}

	if successes == 0 {
		// Do not cache a total failure. A bad URL or a transient outage would
		// otherwise leave the blog empty for the whole TTL, so fixing the config
		// would appear to do nothing until the process restarted.
		return allPosts, nil
	}

	// Update cache (30 minute TTL)
	s.cache = cacheEntry{
		posts:     allPosts,
		expiresAt: time.Now().Add(30 * time.Minute),
	}

	return allPosts, nil
}
