package blog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
)

func TestZZRepro(t *testing.T) {
	cfg := &config.Config{
		BlogURLs: []string{
			"https://www.geekbeacon.org/tag/geekbeacon-fest/rss/",
			"https://www.geekbeacon.org/tag/geeksabroad/rss/",
			"https://www.geekbeacon.org/tag/youtube/rss/",
			"https://www.geekbeacon.org/tag/pixelplayback/rss/",
		},
		BlogRefreshSeconds: 1800,
	}
	svc := NewService(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	n, err := svc.Refresh(ctx)
	fmt.Printf("Refresh returned n=%d err=%v\n", n, err)
}
