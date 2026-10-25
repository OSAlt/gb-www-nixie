package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/mmcdole/gofeed"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

const (
	cacheTTL            = 30 * time.Minute
	defaultRefreshCount = 10
	fetchTimeout        = 8 * time.Second

	// MediaCacheKey is the row the home page reads and the refresh job writes.
	MediaCacheKey = "youtube:recent"

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
//
// With a MediaCacheStore the service becomes read-through: request paths serve
// the last-known-good payload written by the refresh job and never call the
// upstream API. Without a store it keeps the old in-process behaviour, but
// serves expired data immediately and refreshes in the background instead of
// blocking the request.
type Service struct {
	cfg     *config.Config
	apiBase string
	client  *http.Client
	store   ports.MediaCacheStore

	mu        sync.RWMutex
	cache     cacheEntry
	playlists map[string]string // channel -> uploads playlist ID

	refreshing atomic.Bool
	group      singleflight.Group
}

func NewService(cfg *config.Config) *Service {
	return &Service{
		cfg:       cfg,
		apiBase:   youtubeAPIBase,
		client:    &http.Client{Timeout: fetchTimeout},
		playlists: make(map[string]string),
	}
}

// WithStore makes the service read and write through a shared cache. It must
// be called before the service is used concurrently.
func (s *Service) WithStore(store ports.MediaCacheStore) *Service {
	s.store = store
	return s
}

func (s *Service) GetRecentPosts(ctx context.Context, count int) ([]domain.MediaPost, error) {
	if count <= 0 {
		return nil, nil
	}

	// The shared cache is the source of truth when configured. Expiry is not
	// checked here: showing slightly old videos beats showing none, and the
	// refresh job owns freshness.
	if posts, ok := s.fromStore(ctx); ok {
		return limit(posts, count), nil
	}

	s.mu.RLock()
	entry := s.cache
	s.mu.RUnlock()

	if len(entry.posts) > 0 {
		if time.Now().Before(entry.expiresAt) {
			return limit(entry.posts, count), nil
		}
		// Stale: answer immediately and refresh out of band.
		s.refreshAsync(count)
		return limit(entry.posts, count), nil
	}

	posts, err := s.refresh(ctx, count)
	if err != nil {
		return nil, err
	}
	return limit(posts, count), nil
}

// Refresh fetches the latest posts and stores them. It is a no-op while the
// shared cache is fresh, so RunOnStart after a deploy costs nothing.
func (s *Service) Refresh(ctx context.Context, count int) (int, error) {
	if count <= 0 {
		count = defaultRefreshCount
	}

	if s.store != nil {
		entry, err := s.store.LoadMediaCache(ctx, MediaCacheKey)
		switch {
		case err == nil && time.Now().Before(entry.ExpiresAt):
			slog.Info("media cache is fresh; skipping refresh", "expires_at", entry.ExpiresAt)
			return 0, nil
		case err != nil && !errors.Is(err, ports.ErrMediaCacheMiss):
			slog.Warn("media cache read failed; refreshing anyway", "error", err)
		}
	}

	posts, err := s.forceRefresh(ctx, count)
	if err != nil {
		return 0, err
	}
	if len(posts) == 0 {
		return 0, errors.New("media: no posts available")
	}
	return len(posts), nil
}

// fromStore returns the cached payload when one is present. A miss, an
// unreadable row, or invalid JSON all fall through to the in-memory path.
func (s *Service) fromStore(ctx context.Context) ([]domain.MediaPost, bool) {
	if s.store == nil {
		return nil, false
	}

	entry, err := s.store.LoadMediaCache(ctx, MediaCacheKey)
	if err != nil {
		if !errors.Is(err, ports.ErrMediaCacheMiss) {
			slog.Warn("media cache read failed", "error", err)
		}
		return nil, false
	}

	var posts []domain.MediaPost
	if err := json.Unmarshal(entry.Payload, &posts); err != nil {
		slog.Warn("media cache payload is invalid", "error", err)
		return nil, false
	}
	if len(posts) == 0 {
		return nil, false
	}
	return posts, true
}

func (s *Service) refresh(ctx context.Context, count int) ([]domain.MediaPost, error) {
	return s.doRefresh(ctx, count, "refresh", false)
}

func (s *Service) forceRefresh(ctx context.Context, count int) ([]domain.MediaPost, error) {
	return s.doRefresh(ctx, count, "force-refresh", true)
}

// doRefresh collapses concurrent refreshes into one fetch. force skips the
// in-memory freshness check (used by the refresh job, which decides freshness
// from the shared cache instead).
func (s *Service) doRefresh(ctx context.Context, count int, key string, force bool) ([]domain.MediaPost, error) {
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

		posts := s.fetch(ctx, count)
		if len(posts) == 0 {
			// A total upstream failure is not cached, so the next attempt (or
			// retry) can recover immediately.
			return []domain.MediaPost(nil), nil
		}

		expiresAt := time.Now().Add(s.ttl())
		s.mu.Lock()
		s.cache = cacheEntry{posts: posts, expiresAt: expiresAt}
		s.mu.Unlock()

		if s.store != nil {
			payload, err := json.Marshal(posts)
			if err != nil {
				return nil, fmt.Errorf("encode media cache: %w", err)
			}
			if err := s.store.SaveMediaCache(ctx, MediaCacheKey, payload, expiresAt); err != nil {
				return nil, fmt.Errorf("save media cache: %w", err)
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
	return v.([]domain.MediaPost), nil
}

// refreshAsync serves the stale cache and warms it in the background. Only one
// goroutine runs at a time; further stale reads keep serving the old data.
func (s *Service) refreshAsync(count int) {
	if !s.refreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.refreshing.Store(false)
		// The request context is gone by the time the response is written, so
		// the background refresh owns its own deadline.
		ctx, cancel := context.WithTimeout(context.Background(), 2*fetchTimeout)
		defer cancel()
		if _, err := s.refresh(ctx, count); err != nil {
			slog.Warn("background media refresh failed", "error", err)
		}
	}()
}

func (s *Service) ttl() time.Duration {
	if s.cfg != nil && s.cfg.MediaRefreshSeconds > 0 {
		return time.Duration(s.cfg.MediaRefreshSeconds) * time.Second
	}
	return cacheTTL
}

// fetch asks the Data API first and falls back to feeds. It never returns an
// error: a total failure yields no posts, which callers treat as "nothing to
// cache" rather than a request failure.
func (s *Service) fetch(ctx context.Context, count int) []domain.MediaPost {
	posts, apiErr := s.fetchFromAPI(ctx, max(count, defaultRefreshCount))
	if apiErr != nil && !errors.Is(apiErr, errNoAPIKey) {
		slog.Warn("youtube api media fetch failed", "error", apiErr)
	}
	if len(posts) == 0 {
		feedPosts, successes := s.fetchFeeds(ctx)
		if successes > 0 {
			posts = feedPosts
		}
	}
	return posts
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
			slog.Warn("failed to fetch media feed", "url", feedURL, "error", err)
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

	// Channels are independent, so fetch them concurrently. Results are kept
	// per channel and merged in config order to keep sorting deterministic.
	results := make([][]mediaItem, len(channels))
	group, groupCtx := errgroup.WithContext(ctx)
	for i, channel := range channels {
		group.Go(func() error {
			playlistID, err := s.uploadsPlaylist(groupCtx, channel)
			if err != nil {
				return fmt.Errorf("channel %q: %w", channel, err)
			}

			items, err := s.playlistItems(groupCtx, playlistID, count)
			if err != nil {
				return fmt.Errorf("channel %q playlist: %w", channel, err)
			}
			results[i] = items
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	var items []mediaItem
	seen := make(map[string]struct{})
	for _, channelItems := range results {
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
	if after, ok := strings.CutPrefix(channel, "user/"); ok {
		return []youTubeSelector{{"forUsername", after}}
	}
	return []youTubeSelector{
		{"forHandle", handle},
		{"forUsername", handle},
	}
}

// uploadsPlaylist resolves a channel's uploads playlist once and then serves it
// from memory. The value is stable for the lifetime of a channel, so this
// removes one channels.list call per channel per refresh after the first.
func (s *Service) uploadsPlaylist(ctx context.Context, channel string) (string, error) {
	s.mu.RLock()
	playlistID, ok := s.playlists[channel]
	s.mu.RUnlock()
	if ok {
		return playlistID, nil
	}

	playlistID, err := s.resolveUploadsPlaylist(ctx, channel)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.playlists[channel] = playlistID
	s.mu.Unlock()
	return playlistID, nil
}

func (s *Service) resolveUploadsPlaylist(ctx context.Context, channel string) (string, error) {
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

var _ ports.MediaRefresher = (*Service)(nil)
