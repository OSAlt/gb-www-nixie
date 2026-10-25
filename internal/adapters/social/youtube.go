package social

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

const youtubeAPIBase = "https://www.googleapis.com"

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

type youTubeSelector struct{ param, value string }

// selectors lists v3 channel selectors to try in order: a "UC..." value is
// a channel ID, a legacy "user/..." entry is a username, everything else is
// treated as a handle first and username second (each attempt costs 1 unit,
// and only one is needed per channel since a hit short-circuits).
func youTubeSelectors(channel string) []youTubeSelector {
	handle := strings.TrimPrefix(channel, "@")
	if strings.HasPrefix(channel, "UC") {
		return []youTubeSelector{{"id", channel}}
	}
	if after, ok := strings.CutPrefix(channel, "user/"); ok {
		return []youTubeSelector{{"forUsername", after}}
	}
	return []youTubeSelector{
		{"forHandle", handle},
		{"forUsername", handle},
	}
}

func (f *YouTube) Fetch(ctx context.Context) (int, error) {
	if f.APIKey == "" {
		return 0, errMissingConfig("youtube_api_key")
	}

	total := 0
	configured := 0
	for _, channel := range f.Channels {
		channel = strings.TrimSpace(channel)
		if channel == "" {
			continue
		}
		configured++

		count, err := f.resolveChannel(ctx, channel)
		if err != nil {
			// All-or-nothing: a partial sum would look like an audience drop.
			return 0, fmt.Errorf("channel %q: %w", channel, err)
		}
		total += count
	}
	if configured == 0 {
		return 0, fmt.Errorf("no youtube channels configured")
	}
	return total, nil
}

// resolveChannel returns one channel's subscriber count, trying each selector
// (channel ID, handle, or legacy username) until the API returns a match.
func (f *YouTube) resolveChannel(ctx context.Context, channel string) (int, error) {
	base := f.BaseURL
	if base == "" {
		base = youtubeAPIBase
	}

	var lastErr error
	for _, sel := range youTubeSelectors(channel) {
		reqURL := fmt.Sprintf("%s/youtube/v3/channels?part=statistics&%s=%s&key=%s",
			base, sel.param, url.QueryEscape(sel.value), url.QueryEscape(f.APIKey))

		var payload struct {
			Items []struct {
				Statistics struct {
					SubscriberCount string `json:"subscriberCount"`
				} `json:"statistics"`
			} `json:"items"`
		}
		if err := fetchJSON(ctx, f.client, reqURL, nil, &payload); err != nil {
			// Quota/auth errors will not be fixed by another selector.
			return 0, err
		}
		if len(payload.Items) == 0 {
			lastErr = fmt.Errorf("not found via %s", sel.param)
			continue
		}

		count, err := strncpyInt(payload.Items[0].Statistics.SubscriberCount)
		if err != nil {
			return 0, fmt.Errorf("parse subscriberCount: %w", err)
		}
		return count, nil
	}
	return 0, lastErr
}

// strncpyInt parses YouTube's string-encoded statistics values. (Named for
// the oddball string type, not for truncation.)
func strncpyInt(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, fmt.Errorf("not an integer: %q", s)
	}
	return n, nil
}
