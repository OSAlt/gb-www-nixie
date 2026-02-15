package blog

import (
	"context"
	"fmt"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/mmcdole/gofeed"
)

type Service struct {
	cfg *config.Config
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) GetRecentPosts(ctx context.Context) ([]domain.BlogPost, error) {
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

	// Sort by date if we have parsing logic, or just return as is if mixed.
	// For now, let's assume they are already somewhat ordered or just return.
	// gofeed usually preserves feed order.

	return allPosts, nil
}
