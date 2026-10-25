package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/OSAlt/gb-www-nixie/internal/config"
)

const channelAFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"
      xmlns:yt="http://www.youtube.com/xml/schemas/2015"
      xmlns:media="http://search.yahoo.com/mrss/">
  <title>NixiePixel</title>
  <id>UCMyAVYPgP_179gj9OIJZd4A</id>
  <entry>
    <id>yt:video:older</id>
    <yt:videoId>older</yt:videoId>
    <title>Older Video</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=older"/>
    <published>2024-01-01T00:00:00+00:00</published>
    <media:group>
      <media:title>Older Video</media:title>
      <media:thumbnail url="https://i.ytimg.com/vi/older/hqdefault.jpg" width="480" height="360"/>
    </media:group>
  </entry>
  <entry>
    <id>yt:video:undated</id>
    <yt:videoId>undated</yt:videoId>
    <title>Undated Video</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=undated"/>
  </entry>
</feed>`

const channelBFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"
      xmlns:yt="http://www.youtube.com/xml/schemas/2015"
      xmlns:media="http://search.yahoo.com/mrss/">
  <title>NixieDoesLinux</title>
  <id>UCBE-FO9JUOghSysV9gjTeHw</id>
  <entry>
    <id>yt:video:newer</id>
    <yt:videoId>newer</yt:videoId>
    <title>Newest Video</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=newer"/>
    <published>2024-02-01T00:00:00+00:00</published>
    <media:group>
      <media:thumbnail url="https://i.ytimg.com/vi/newer/hqdefault.jpg" width="480" height="360"/>
    </media:group>
  </entry>
  <entry>
    <id>yt:video:older</id>
    <yt:videoId>older</yt:videoId>
    <title>Older Video (cross-posted)</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=older"/>
    <published>2024-01-01T00:00:00+00:00</published>
  </entry>
</feed>`

func feedServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		switch r.URL.Path {
		case "/channel-a.xml":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(channelAFeed))
		case "/channel-b.xml":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(channelBFeed))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestService_GetRecentPosts_ParsesAndSorts(t *testing.T) {
	srv := feedServer(t, nil)
	svc := NewService(&config.Config{MediaFeedURLs: []string{
		srv.URL + "/channel-a.xml",
		srv.URL + "/channel-b.xml",
	}})

	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 3 {
		t.Fatalf("got %d posts, want 3 unique posts: %+v", len(posts), posts)
	}

	newest := posts[0]
	if newest.ID != "newer" {
		t.Errorf("ID = %q, want newer", newest.ID)
	}
	if newest.Caption != "Newest Video" {
		t.Errorf("Caption = %q", newest.Caption)
	}
	if newest.URL != "https://www.youtube.com/watch?v=newer" {
		t.Errorf("URL = %q", newest.URL)
	}
	if newest.ImageURL != "https://i.ytimg.com/vi/newer/hqdefault.jpg" {
		t.Errorf("ImageURL = %q", newest.ImageURL)
	}

	if posts[1].ID != "older" {
		t.Errorf("second post = %q, want the deduped older video", posts[1].ID)
	}
	if posts[2].ID != "undated" {
		t.Errorf("undated posts should sort last, got %q", posts[2].ID)
	}
}

func TestService_GetRecentPosts_LimitsCount(t *testing.T) {
	srv := feedServer(t, nil)
	svc := NewService(&config.Config{MediaFeedURLs: []string{
		srv.URL + "/channel-a.xml",
		srv.URL + "/channel-b.xml",
	}})

	posts, err := svc.GetRecentPosts(context.Background(), 2)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2", len(posts))
	}
	if posts[0].ID != "newer" || posts[1].ID != "older" {
		t.Errorf("got %q, %q", posts[0].ID, posts[1].ID)
	}
}

func TestService_GetRecentPosts_Caches(t *testing.T) {
	var hits atomic.Int32
	srv := feedServer(t, &hits)
	svc := NewService(&config.Config{MediaFeedURLs: []string{srv.URL + "/channel-a.xml"}})

	first, err := svc.GetRecentPosts(context.Background(), 2)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := svc.GetRecentPosts(context.Background(), 2)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if hits.Load() != 1 {
		t.Errorf("got %d feed requests, want 1 (second call should be cached)", hits.Load())
	}
	if len(first) != len(second) || first[0].ID != second[0].ID {
		t.Errorf("cached posts differ: %+v vs %+v", first, second)
	}
}

func TestService_GetRecentPosts_SkipsBrokenFeed(t *testing.T) {
	srv := feedServer(t, nil)
	svc := NewService(&config.Config{MediaFeedURLs: []string{
		srv.URL + "/missing.xml",
		srv.URL + "/channel-a.xml",
	}})

	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 2 {
		t.Errorf("got %d posts, want the 2 from the working feed", len(posts))
	}
}

func TestService_GetRecentPosts_AllFeedsFail(t *testing.T) {
	var hits atomic.Int32
	srv := feedServer(t, &hits)
	svc := NewService(&config.Config{MediaFeedURLs: []string{
		srv.URL + "/missing.xml",
		srv.URL + "/also-missing.xml",
	}})

	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts must not fail when feeds are unreachable: %v", err)
	}
	if len(posts) != 0 {
		t.Errorf("got %d posts, want none", len(posts))
	}

	// Total failure must not be cached.
	if _, err := svc.GetRecentPosts(context.Background(), 6); err != nil {
		t.Fatalf("second GetRecentPosts: %v", err)
	}
	if got := hits.Load(); got != 4 {
		t.Errorf("got %d feed requests, want 4 (failures must not be cached)", got)
	}
}

func TestService_GetRecentPosts_NoURLsConfigured(t *testing.T) {
	svc := NewService(&config.Config{})

	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 0 {
		t.Errorf("got %d posts, want none", len(posts))
	}
}

func TestService_GetRecentPosts_NonPositiveCount(t *testing.T) {
	svc := NewService(&config.Config{MediaFeedURLs: []string{"https://example.com/feed.xml"}})

	posts, err := svc.GetRecentPosts(context.Background(), 0)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if posts != nil {
		t.Errorf("got %v, want nil", posts)
	}
}

const channelsJSON = `{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UUmain"}}}]}`

const playlistJSON = `{"items":[
  {"snippet":{
    "title":"API Older",
    "publishedAt":"2024-01-01T00:00:00Z",
    "resourceId":{"videoId":"api-older"},
    "thumbnails":{"default":{"url":"https://i.ytimg.com/vi/api-older/default.jpg"},"high":{"url":"https://i.ytimg.com/vi/api-older/hqdefault.jpg"}}
  }},
  {"snippet":{
    "title":"API Newer",
    "publishedAt":"2024-03-01T00:00:00Z",
    "resourceId":{"videoId":"api-newer"},
    "thumbnails":{"maxres":{"url":"https://i.ytimg.com/vi/api-newer/maxresdefault.jpg"}}
  }}
]}`

func TestService_GetRecentPosts_UsesYouTubeAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/youtube/v3/channels":
			if got := r.URL.Query().Get("forHandle"); got != "NixiePixelGB" {
				t.Errorf("forHandle = %q", got)
			}
			if got := r.URL.Query().Get("key"); got != "key" {
				t.Errorf("key = %q", got)
			}
			_, _ = w.Write([]byte(channelsJSON))
		case "/youtube/v3/playlistItems":
			if got := r.URL.Query().Get("playlistId"); got != "UUmain" {
				t.Errorf("playlistId = %q", got)
			}
			_, _ = w.Write([]byte(playlistJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := NewService(&config.Config{
		YoutubeApiKey:   "key",
		YoutubeChannels: "NixiePixelGB",
	})
	svc.apiBase = srv.URL

	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2", len(posts))
	}

	newest := posts[0]
	if newest.ID != "api-newer" || newest.Caption != "API Newer" {
		t.Errorf("first post = %+v", newest)
	}
	if newest.URL != "https://www.youtube.com/watch?v=api-newer" {
		t.Errorf("URL = %q", newest.URL)
	}
	if newest.ImageURL != "https://i.ytimg.com/vi/api-newer/maxresdefault.jpg" {
		t.Errorf("ImageURL = %q, want the maxres thumbnail", newest.ImageURL)
	}

	older := posts[1]
	if older.ID != "api-older" {
		t.Errorf("second post = %q", older.ID)
	}
	if older.ImageURL != "https://i.ytimg.com/vi/api-older/hqdefault.jpg" {
		t.Errorf("ImageURL = %q, want the best available thumbnail", older.ImageURL)
	}
}

func TestService_GetRecentPosts_APIFailureFallsBackToFeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/youtube/v3/channels":
			http.Error(w, "quota exceeded", http.StatusForbidden)
		case "/feed.xml":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(channelAFeed))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := NewService(&config.Config{
		YoutubeApiKey:   "key",
		YoutubeChannels: "NixiePixelGB",
		MediaFeedURLs:   []string{srv.URL + "/feed.xml"},
	})
	svc.apiBase = srv.URL

	posts, err := svc.GetRecentPosts(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want the 2 from the fallback feed", len(posts))
	}
	if posts[0].ID != "older" {
		t.Errorf("first post = %q, want the dated feed item", posts[0].ID)
	}
}
