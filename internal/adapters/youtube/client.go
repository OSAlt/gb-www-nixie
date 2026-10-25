// Package youtube is the shared YouTube Data API v3 client used by the media
// and social adapters. It resolves channel selectors (handles, legacy
// usernames, channel IDs) and fetches uploads playlists, subscriber counts and
// playlist items.
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production API host. Tests override it.
const DefaultBaseURL = "https://www.googleapis.com"

// ErrNoAPIKey is returned when no API key is configured. Callers use it to
// distinguish "not configured" from a real failure.
var ErrNoAPIKey = errors.New("youtube api key not configured")

// HTTPClient is the small part of *http.Client this package needs.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var defaultHTTPClient = &http.Client{Timeout: 8 * time.Second}

// Client talks to the Data API. The zero value is valid except for APIKey.
type Client struct {
	APIKey     string
	BaseURL    string // empty means DefaultBaseURL
	HTTPClient HTTPClient
}

// Channel is the subset of channels.list both adapters need.
type Channel struct {
	UploadsPlaylist string
	SubscriberCount int
}

// Video is one playlist item.
type Video struct {
	ID          string
	Title       string
	ImageURL    string
	PublishedAt time.Time
}

type selector struct{ param, value string }

// selectors lists v3 channel selectors to try in order: a "UC..." value is a
// channel ID, a legacy "user/..." entry is a username, everything else is
// treated as a handle first and username second. A hit short-circuits, so a
// normal handle costs one request.
func selectors(channel string) []selector {
	handle := strings.TrimPrefix(channel, "@")
	if strings.HasPrefix(channel, "UC") {
		return []selector{{"id", channel}}
	}
	if after, ok := strings.CutPrefix(channel, "user/"); ok {
		return []selector{{"forUsername", after}}
	}
	return []selector{
		{"forHandle", handle},
		{"forUsername", handle},
	}
}

// Channel resolves a configured channel and returns its uploads playlist and
// subscriber count in one request (part=contentDetails,statistics).
func (c Client) Channel(ctx context.Context, channel string) (Channel, error) {
	if c.APIKey == "" {
		return Channel{}, ErrNoAPIKey
	}

	var lastErr error
	for _, sel := range selectors(channel) {
		reqURL := fmt.Sprintf("%s/youtube/v3/channels?part=contentDetails,statistics&%s=%s&key=%s",
			c.baseURL(), sel.param, url.QueryEscape(sel.value), url.QueryEscape(c.APIKey))

		var payload struct {
			Items []struct {
				ContentDetails struct {
					RelatedPlaylists struct {
						Uploads string `json:"uploads"`
					} `json:"relatedPlaylists"`
				} `json:"contentDetails"`
				Statistics struct {
					SubscriberCount string `json:"subscriberCount"`
				} `json:"statistics"`
			} `json:"items"`
		}
		if err := c.getJSON(ctx, reqURL, &payload); err != nil {
			// Quota/auth errors will not be fixed by another selector.
			return Channel{}, err
		}
		if len(payload.Items) == 0 {
			lastErr = fmt.Errorf("channel %q not found via %s", channel, sel.param)
			continue
		}

		item := payload.Items[0]
		subscribers := 0
		if item.Statistics.SubscriberCount != "" {
			n, err := strconv.Atoi(item.Statistics.SubscriberCount)
			if err != nil {
				return Channel{}, fmt.Errorf("parse subscriberCount for %q: %w", channel, err)
			}
			subscribers = n
		}

		return Channel{
			UploadsPlaylist: item.ContentDetails.RelatedPlaylists.Uploads,
			SubscriberCount: subscribers,
		}, nil
	}
	return Channel{}, lastErr
}

// PlaylistItems returns up to count videos from a playlist, newest first as
// returned by the API.
func (c Client) PlaylistItems(ctx context.Context, playlistID string, count int) ([]Video, error) {
	if c.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	if count <= 0 {
		return nil, nil
	}

	reqURL := fmt.Sprintf("%s/youtube/v3/playlistItems?part=snippet&playlistId=%s&maxResults=%d&key=%s",
		c.baseURL(), url.QueryEscape(playlistID), count, url.QueryEscape(c.APIKey))

	var payload struct {
		Items []struct {
			Snippet struct {
				Title       string    `json:"title"`
				PublishedAt time.Time `json:"publishedAt"`
				ResourceID  struct {
					VideoID string `json:"videoId"`
				} `json:"resourceId"`
				Thumbnails thumbnails `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, reqURL, &payload); err != nil {
		return nil, err
	}

	videos := make([]Video, 0, len(payload.Items))
	for _, item := range payload.Items {
		id := item.Snippet.ResourceID.VideoID
		if id == "" {
			continue
		}
		videos = append(videos, Video{
			ID:          id,
			Title:       strings.TrimSpace(item.Snippet.Title),
			ImageURL:    bestThumbnail(item.Snippet.Thumbnails),
			PublishedAt: item.Snippet.PublishedAt,
		})
	}
	return videos, nil
}

type thumbnail struct {
	URL string `json:"url"`
}

type thumbnails struct {
	Default  thumbnail `json:"default"`
	Medium   thumbnail `json:"medium"`
	High     thumbnail `json:"high"`
	Standard thumbnail `json:"standard"`
	Maxres   thumbnail `json:"maxres"`
}

func bestThumbnail(t thumbnails) string {
	for _, candidate := range []thumbnail{t.Maxres, t.Standard, t.High, t.Medium, t.Default} {
		if candidate.URL != "" {
			return candidate.URL
		}
	}
	return ""
}

func (c Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

func (c Client) httpClient() HTTPClient {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return defaultHTTPClient
}

func (c Client) getJSON(ctx context.Context, reqURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
