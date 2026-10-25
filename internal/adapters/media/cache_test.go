package media

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/stretchr/testify/mock"
)

func cachedPayload(t *testing.T, posts ...domain.MediaPost) []byte {
	t.Helper()
	payload, err := json.Marshal(posts)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

func mediaConfig(feedURL string) *config.Config {
	return &config.Config{
		MediaRefreshSeconds: 1800,
		MediaFeedURLs:       []string{feedURL},
	}
}

func TestService_GetRecentPosts_UsesStore(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, MediaCacheKey).Return(ports.ContentCacheEntry{
		Payload:   cachedPayload(t, domain.MediaPost{ID: "cached", Caption: "Cached"}),
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	// No feeds and no API key: an empty result would mean the store was ignored.
	svc := NewService(mediaConfig("")).WithStore(store)

	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 1 || posts[0].ID != "cached" {
		t.Fatalf("posts = %+v, want the cached one", posts)
	}
}

func TestService_GetRecentPosts_ServesStaleFromStore(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, MediaCacheKey).Return(ports.ContentCacheEntry{
		Payload:   cachedPayload(t, domain.MediaPost{ID: "stale"}),
		FetchedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-time.Hour),
	}, nil)

	svc := NewService(mediaConfig("")).WithStore(store)

	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 1 || posts[0].ID != "stale" {
		t.Fatalf("posts = %+v, want the stale one", posts)
	}
}

func TestService_GetRecentPosts_StoreMissFetchesAndSaves(t *testing.T) {
	var hits atomic.Int32
	srv := feedServer(t, &hits)

	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, MediaCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)
	store.EXPECT().SaveContentCache(mock.Anything, MediaCacheKey, mock.Anything, mock.Anything).Return(nil)

	svc := NewService(mediaConfig(srv.URL + "/channel-a.xml")).WithStore(store)

	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want the 2 from the feed", len(posts))
	}
	if hits.Load() != 1 {
		t.Errorf("got %d feed requests, want 1", hits.Load())
	}
}

func TestService_GetRecentPosts_StoreReadErrorFallsBack(t *testing.T) {
	var hits atomic.Int32
	srv := feedServer(t, &hits)

	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, MediaCacheKey).Return(ports.ContentCacheEntry{}, errors.New("db down"))
	store.EXPECT().SaveContentCache(mock.Anything, MediaCacheKey, mock.Anything, mock.Anything).Return(nil)

	svc := NewService(mediaConfig(srv.URL + "/channel-a.xml")).WithStore(store)

	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want the feed fallback", len(posts))
	}
	if hits.Load() != 1 {
		t.Errorf("got %d feed requests, want 1", hits.Load())
	}
}

func TestService_Refresh_SavesToStore(t *testing.T) {
	srv := feedServer(t, nil)

	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, MediaCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)
	store.EXPECT().SaveContentCache(mock.Anything, MediaCacheKey, mock.Anything, mock.Anything).Return(nil)

	svc := NewService(mediaConfig(srv.URL + "/channel-b.xml")).WithStore(store)

	count, err := svc.Refresh(context.Background(), 10)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if count != 2 {
		t.Errorf("Refresh count = %d, want 2", count)
	}
}

func TestService_Refresh_SkipsFreshCache(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, MediaCacheKey).Return(ports.ContentCacheEntry{
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	svc := NewService(mediaConfig("")).WithStore(store)

	count, err := svc.Refresh(context.Background(), 10)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if count != 0 {
		t.Errorf("Refresh count = %d, want 0 (fresh cache must be a no-op)", count)
	}
}

func TestService_Refresh_NoPostsIsAnError(t *testing.T) {
	srv := feedServer(t, nil)
	svc := NewService(mediaConfig(srv.URL + "/missing.xml"))

	if _, err := svc.Refresh(context.Background(), 10); err == nil {
		t.Fatal("expected an error when no upstream data is available")
	}
}

func TestService_Refresh_SaveErrorIsReturned(t *testing.T) {
	srv := feedServer(t, nil)

	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, MediaCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)
	store.EXPECT().SaveContentCache(mock.Anything, MediaCacheKey, mock.Anything, mock.Anything).Return(errors.New("db down"))

	svc := NewService(mediaConfig(srv.URL + "/channel-a.xml")).WithStore(store)

	if _, err := svc.Refresh(context.Background(), 10); err == nil {
		t.Fatal("expected the store error to propagate so the job retries")
	}
}

func TestService_GetRecentPosts_ServesStaleAndRefreshes(t *testing.T) {
	var hits atomic.Int32
	srv := feedServer(t, &hits)

	svc := NewService(mediaConfig(srv.URL + "/channel-a.xml"))
	svc.cache = cacheEntry{
		posts:     []domain.MediaPost{{ID: "stale"}},
		expiresAt: time.Now().Add(-time.Minute),
	}

	start := time.Now()
	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 1 || posts[0].ID != "stale" {
		t.Fatalf("posts = %+v, want the stale entry served immediately", posts)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("stale read took %s; it must not wait for the refresh", elapsed)
	}

	deadline := time.Now().Add(2 * time.Second)
	for hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if hits.Load() == 0 {
		t.Fatal("background refresh never hit the feed")
	}
}

func TestService_RefreshReusesUploadsPlaylist(t *testing.T) {
	var channelHits, playlistHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/youtube/v3/channels":
			channelHits.Add(1)
			_, _ = w.Write([]byte(channelsJSON))
		case "/youtube/v3/playlistItems":
			playlistHits.Add(1)
			_, _ = w.Write([]byte(playlistJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := NewService(&config.Config{
		YoutubeApiKey:       "key",
		YoutubeChannels:     "NixiePixelGB",
		MediaRefreshSeconds: 1800,
	})
	svc.apiBase = srv.URL

	ctx := context.Background()
	if _, err := svc.Refresh(ctx, 10); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	if _, err := svc.Refresh(ctx, 10); err != nil {
		t.Fatalf("second Refresh: %v", err)
	}

	if got := channelHits.Load(); got != 1 {
		t.Errorf("channels.list hits = %d, want 1 (the uploads playlist is cached)", got)
	}
	if got := playlistHits.Load(); got != 2 {
		t.Errorf("playlistItems hits = %d, want 2 (one per refresh)", got)
	}
}
