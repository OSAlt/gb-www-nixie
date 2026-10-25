package blog

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/stretchr/testify/mock"
)

func cachedBlogPayload(t *testing.T, posts ...domain.BlogPost) []byte {
	t.Helper()
	payload, err := json.Marshal(posts)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

func blogConfig(feedURL string) *config.Config {
	return &config.Config{
		BlogRefreshSeconds: 1800,
		BlogURLs:           []string{feedURL},
	}
}

func TestService_GetRecentPosts_UsesStore(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, BlogCacheKey).Return(ports.ContentCacheEntry{
		Payload:   cachedBlogPayload(t, domain.BlogPost{Title: "Cached"}),
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	// No feeds: an empty result would mean the store was ignored.
	svc := NewService(blogConfig("")).WithStore(store)

	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 1 || posts[0].Title != "Cached" {
		t.Fatalf("posts = %+v, want the cached one", posts)
	}
}

func TestService_GetRecentPosts_ServesStaleFromStore(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, BlogCacheKey).Return(ports.ContentCacheEntry{
		Payload:   cachedBlogPayload(t, domain.BlogPost{Title: "Stale"}),
		FetchedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-time.Hour),
	}, nil)

	svc := NewService(blogConfig("")).WithStore(store)

	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 1 || posts[0].Title != "Stale" {
		t.Fatalf("posts = %+v, want the stale one", posts)
	}
}

func TestService_GetRecentPosts_StoreMissFetchesAndSaves(t *testing.T) {
	var hits atomic.Int32
	srv := feedServer(t, &hits)

	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, BlogCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)
	store.EXPECT().SaveContentCache(mock.Anything, BlogCacheKey, mock.Anything, mock.Anything).Return(nil)

	svc := NewService(blogConfig(srv.URL + "/rss.xml")).WithStore(store)

	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) == 0 {
		t.Fatal("expected posts from the feed")
	}
	if hits.Load() != 1 {
		t.Errorf("got %d feed requests, want 1", hits.Load())
	}
}

func TestService_Refresh_SavesToStore(t *testing.T) {
	srv := feedServer(t, nil)

	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, BlogCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)
	store.EXPECT().SaveContentCache(mock.Anything, BlogCacheKey, mock.Anything, mock.Anything).Return(nil)

	svc := NewService(blogConfig(srv.URL + "/rss.xml")).WithStore(store)

	count, err := svc.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if count == 0 {
		t.Error("Refresh count = 0, want the number of parsed posts")
	}
}

func TestService_Refresh_SkipsFreshCache(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, BlogCacheKey).Return(ports.ContentCacheEntry{
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	svc := NewService(blogConfig("")).WithStore(store)

	count, err := svc.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if count != 0 {
		t.Errorf("Refresh count = %d, want 0 (fresh cache must be a no-op)", count)
	}
}

func TestService_Refresh_NoPostsIsAnError(t *testing.T) {
	srv := feedServer(t, nil)
	svc := NewService(blogConfig(srv.URL + "/missing.xml"))

	if _, err := svc.Refresh(context.Background()); err == nil {
		t.Fatal("expected an error when no feed could be read")
	}
}

func TestService_GetRecentPosts_ServesStaleInMemoryAndRefreshes(t *testing.T) {
	var hits atomic.Int32
	srv := feedServer(t, &hits)

	svc := NewService(blogConfig(srv.URL + "/rss.xml"))
	svc.cache = cacheEntry{
		posts:     []domain.BlogPost{{Title: "Stale"}},
		expiresAt: time.Now().Add(-time.Minute),
	}

	start := time.Now()
	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 1 || posts[0].Title != "Stale" {
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

func TestService_CacheState(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, BlogCacheKey).Return(ports.ContentCacheEntry{
		Payload:   cachedBlogPayload(t, domain.BlogPost{Title: "One"}, domain.BlogPost{Title: "Two"}),
		FetchedAt: time.Now().Add(-time.Minute),
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	state, err := NewService(blogConfig("")).WithStore(store).CacheState(context.Background())
	if err != nil {
		t.Fatalf("CacheState: %v", err)
	}
	if state.Items != 2 || state.Missing || state.Stale {
		t.Errorf("state = %+v, want 2 fresh items", state)
	}
}

func TestService_CacheState_Missing(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, BlogCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)

	state, err := NewService(blogConfig("")).WithStore(store).CacheState(context.Background())
	if err != nil {
		t.Fatalf("CacheState: %v", err)
	}
	if !state.Missing {
		t.Errorf("state = %+v, want missing", state)
	}
}

func TestService_GetRecentPosts_StoreReadErrorFallsBack(t *testing.T) {
	var hits atomic.Int32
	srv := feedServer(t, &hits)

	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, BlogCacheKey).Return(ports.ContentCacheEntry{}, errors.New("db down"))
	store.EXPECT().SaveContentCache(mock.Anything, BlogCacheKey, mock.Anything, mock.Anything).Return(nil)

	svc := NewService(blogConfig(srv.URL + "/rss.xml")).WithStore(store)

	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) == 0 {
		t.Fatal("expected the feed fallback")
	}
	if hits.Load() != 1 {
		t.Errorf("got %d feed requests, want 1", hits.Load())
	}
}
