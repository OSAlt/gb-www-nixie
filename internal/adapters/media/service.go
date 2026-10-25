package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/mmcdole/gofeed"
)

const (
	cacheTTL       = 30 * time.Minute
	youtubeAPIBase = "https://www.googleapis.com"
)

var errNoAPIKey = errors.New("youtube api key not configured")

type cacheEntry struct {
	posts     []domain.MediaPost
	expiresAt time.Time
}

// Service reads the latest videos from the YouTube Data API when a key is
// configured, and otherwise (or on API failure) falls back to public
// YouTube/Atom feeds. It is deliberately tolerant: the home page hides the
// video section when nothing is available.
type Service struct {
	cfg     *config.Config
	apiBase string
	client  *http.Client
	cache   cacheEntry
	mu      sync.RWMutex
}

func NewService(cfg *config.Config) *Service {
	return &Service{
		cfg:     cfg,
		apiBase: youtubeAPIBase,
		client:  &http.Client{Timeout: 8 * time.Second},
	}
}

func (s *Service) GetRecentPosts(ctx context.Context, count int) ([]domain.MediaPost, error) {
	if count <= 0 {
		return nil, nil
	}

	s.mu.RLock()
	if time.Now().Before(s.cache.expiresAt) {
		posts := limit(s.cache.posts, count)
		s.mu.RUnlock()
		return posts, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double check after acquiring the write lock.
	if time.Now().Before(s.cache.expiresAt) {
		return limit(s.cache.posts, count), nil
	}

	// The Data API is exact and does not depend on the public RSS endpoint,
	// whose availability varies by network. Fall back to feeds when no key is
	// configured or the API call fails.
	posts, apiErr := s.fetchFromAPI(ctx, max(count, 10))
	if apiErr != nil && !errors.Is(apiErr, errNoAPIKey) {
		fmt.Printf("Warning: YouTube API media fetch failed: %v\n", apiErr)
	}
	if len(posts) == 0 {
		feedPosts, successes := s.fetchFeeds(ctx)
		if successes > 0 {
			posts = feedPosts
		}
	}

	if len(posts) > 0 {
		s.cache = cacheEntry{
			posts:     posts,
			expiresAt: time.Now().Add(cacheTTL),
		}
	}

	return limit(posts, count), nil
}

// fetchFeeds merges the configured Atom/RSS feeds. It returns the number of
// feeds that loaded so a total failure is not cached.
func (s *Service) fetchFeeds(ctx context.Context) ([]domain.MediaPost, int) {
	fp := gofeed.NewParser()
	fp.Client = s.client

	var items []mediaItem
	seen := make(map[string]struct{})
	successes := 0

	for _, feedURL := range s.cfg.MediaFeedURLs {
		feed, err := fp.ParseURLWithContext(feedURL, ctx)
		if err != nil {
			fmt.Printf("Warning: failed to fetch media feed from %s: %v\n", feedURL, err)
			continue
		}
		successes++

		for _, item := range feed.Items {
			post := toPost(item)

			key := dedupeKey(post)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			items = append(items, mediaItem{post: post, published: publishedAt(item)})
		}
	}

	return sortAndConvert(items), successes
}

func (s *Service) fetchFromAPI(ctx context.Context, count int) ([]domain.MediaPost, error) {
	if s.cfg.YoutubeApiKey == "" {
		return nil, errNoAPIKey
	}

	channels := splitChannels(s.cfg.YoutubeChannels)
	if len(channels) == 0 {
		return nil, fmt.Errorf("no youtube channels configured")
	}

	var items []mediaItem
	seen := make(map[string]struct{})

	for _, channel := range channels {
		playlistID, err := s.uploadsPlaylist(ctx, channel)
		if err != nil {
			return nil, fmt.Errorf("channel %q: %w", channel, err)
		}

		channelItems, err := s.playlistItems(ctx, playlistID, count)
		if err != nil {
			return nil, fmt.Errorf("channel %q playlist: %w", channel, err)
		}

		for _, item := range channelItems {
			key := dedupeKey(item.post)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, item)
		}
	}

	return sortAndConvert(items), nil
}

type youTubeSelector struct{ param, value string }

func youTubeSelectors(channel string) []youTubeSelector {
	handle := strings.TrimPrefix(channel, "@")
	if strings.HasPrefix(channel, "UC") {
		return []youTubeSelector{{"id", channel}}
	}
	if strings.HasPrefix(channel, "user/") {
		return []youTubeSelector{{"forUsername", strings.TrimPrefix(channel, "user/")}}
	}
	return []youTubeSelector{
		{"forHandle", handle},
		{"forUsername", handle},
	}
}

func (s *Service) uploadsPlaylist(ctx context.Context, channel string) (string, error) {
	var lastErr error
	for _, sel := range youTubeSelectors(channel) {
		reqURL := fmt.Sprintf("%s/youtube/v3/channels?part=contentDetails&%s=%s&key=%s",
			s.apiBase, sel.param, url.QueryEscape(sel.value), url.QueryEscape(s.cfg.YoutubeApiKey))

		var payload struct {
			Items []struct {
				ContentDetails struct {
					RelatedPlaylists struct {
						Uploads string `json:"uploads"`
					} `json:"relatedPlaylists"`
				} `json:"contentDetails"`
			} `json:"items"`
		}
		if err := s.getJSON(ctx, reqURL, &payload); err != nil {
			return "", err
		}
		if len(payload.Items) == 0 || payload.Items[0].ContentDetails.RelatedPlaylists.Uploads == "" {
			lastErr = fmt.Errorf("not found via %s", sel.param)
			continue
		}
		return payload.Items[0].ContentDetails.RelatedPlaylists.Uploads, nil
	}
	return "", lastErr
}

func (s *Service) playlistItems(ctx context.Context, playlistID string, count int) ([]mediaItem, error) {
	reqURL := fmt.Sprintf("%s/youtube/v3/playlistItems?part=snippet&playlistId=%s&maxResults=%d&key=%s",
		s.apiBase, url.QueryEscape(playlistID), count, url.QueryEscape(s.cfg.YoutubeApiKey))

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
	if err := s.getJSON(ctx, reqURL, &payload); err != nil {
		return nil, err
	}

	items := make([]mediaItem, 0, len(payload.Items))
	for _, item := range payload.Items {
		id := item.Snippet.ResourceID.VideoID
		if id == "" {
			continue
		}
		post := domain.MediaPost{
			ID:       id,
			Caption:  strings.TrimSpace(item.Snippet.Title),
			URL:      "https://www.youtube.com/watch?v=" + id,
			ImageURL: bestThumbnail(item.Snippet.Thumbnails),
		}
		items = append(items, mediaItem{post: post, published: item.Snippet.PublishedAt})
	}
	return items, nil
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

func (s *Service) getJSON(ctx context.Context, reqURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

type mediaItem struct {
	post      domain.MediaPost
	published time.Time
}

func sortAndConvert(items []mediaItem) []domain.MediaPost {
	// Newest first; undated entries keep their feed order at the end.
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].published.IsZero() {
			return false
		}
		if items[j].published.IsZero() {
			return true
		}
		return items[i].published.After(items[j].published)
	})

	posts := make([]domain.MediaPost, len(items))
	for i, item := range items {
		posts[i] = item.post
	}
	return posts
}

func toPost(item *gofeed.Item) domain.MediaPost {
	return domain.MediaPost{
		ID:       videoID(item),
		Caption:  strings.TrimSpace(item.Title),
		URL:      item.Link,
		ImageURL: imageURL(item),
	}
}

func dedupeKey(post domain.MediaPost) string {
	if post.ID != "" {
		return post.ID
	}
	if post.URL != "" {
		return post.URL
	}
	return post.Caption
}

func publishedAt(item *gofeed.Item) time.Time {
	if item.PublishedParsed != nil {
		return *item.PublishedParsed
	}
	if item.UpdatedParsed != nil {
		return *item.UpdatedParsed
	}
	return time.Time{}
}

func videoID(item *gofeed.Item) string {
	if value := extensionValue(item, "yt", "videoId"); value != "" {
		return value
	}
	if parsed, err := url.Parse(item.Link); err == nil {
		if id := parsed.Query().Get("v"); id != "" {
			return id
		}
	}
	return item.GUID
}

func imageURL(item *gofeed.Item) string {
	if item.Image != nil && item.Image.URL != "" {
		return item.Image.URL
	}

	// YouTube nests the thumbnail under media:group.
	for _, group := range item.Extensions["media"]["group"] {
		for _, thumb := range group.Children["thumbnail"] {
			if thumb.Attrs["url"] != "" {
				return thumb.Attrs["url"]
			}
		}
	}
	for _, thumb := range item.Extensions["media"]["thumbnail"] {
		if thumb.Attrs["url"] != "" {
			return thumb.Attrs["url"]
		}
	}
	return ""
}

func extensionValue(item *gofeed.Item, namespace, name string) string {
	for _, ext := range item.Extensions[namespace][name] {
		if ext.Value != "" {
			return ext.Value
		}
	}
	return ""
}

func splitChannels(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func limit(posts []domain.MediaPost, count int) []domain.MediaPost {
	if count > len(posts) {
		count = len(posts)
	}
	return posts[:count]
}
