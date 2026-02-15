package blog

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/mmcdole/gofeed"
)

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
	var allPosts []domain.BlogPost

	for _, url := range s.cfg.BlogURLs {
		feed, err := fp.ParseURLWithContext(url, ctx)
		if err != nil {
			fmt.Printf("Warning: failed to fetch feed from %s: %v\n", url, err)
			continue
		}

		for _, item := range feed.Items {
			post := domain.BlogPost{
				Title:       item.Title,
				Description: item.Description,
				Content:     item.Content,
				URL:         item.Link,
				Author:      "",
			}

			if item.PublishedParsed != nil {
				post.PublishedAt = item.PublishedParsed.Format("January 02, 2006")
			} else if item.UpdatedParsed != nil {
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

			allPosts = append(allPosts, post)
		}
	}

	// Update cache (5 minute TTL)
	s.cache = cacheEntry{
		posts:     allPosts,
		expiresAt: time.Now().Add(30 * time.Minute),
	}

	return allPosts, nil
}
