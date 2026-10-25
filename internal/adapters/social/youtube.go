package social

import (
	"context"
	"fmt"
	"strings"

	"github.com/OSAlt/gb-www-nixie/internal/adapters/youtube"
)

// YouTube fetches subscriber counts via the official, free YouTube Data API
// v3 (1 quota unit per call out of 10,000/day). Multiple channels can be
// configured; their counts are summed into a single "youtube" total (Nixie's
// audience is split across @NixiePixelGB and user/nixiedoeslinux).
type YouTube struct {
	APIKey string
	// Channels are combined (summed). Each entry is "@name", a legacy
	// "user/name", a bare handle/username, or a channel ID ("UC...").
	Channels []string

	// BaseURL is overridable for tests.
	BaseURL string
	client  httpClient
}

func (f *YouTube) Key() string { return "youtube" }

func (f *YouTube) Fetch(ctx context.Context) (int, error) {
	if f.APIKey == "" {
		return 0, errMissingConfig("youtube_api_key")
	}

	client := youtube.Client{
		APIKey:     f.APIKey,
		BaseURL:    f.BaseURL,
		HTTPClient: f.client,
	}

	total := 0
	configured := 0
	for _, channel := range f.Channels {
		channel = strings.TrimSpace(channel)
		if channel == "" {
			continue
		}
		configured++

		info, err := client.Channel(ctx, channel)
		if err != nil {
			// All-or-nothing: a partial sum would look like an audience drop.
			return 0, fmt.Errorf("channel %q: %w", channel, err)
		}
		total += info.SubscriberCount
	}
	if configured == 0 {
		return 0, fmt.Errorf("no youtube channels configured")
	}
	return total, nil
}
