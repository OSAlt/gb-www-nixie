package blog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/OSAlt/gb-www-nixie/internal/config"
)

const rssFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Nixie Blog</title>
    <link>https://example.com</link>
    <description>Blog</description>
    <item>
      <title>Linux Adventures</title>
      <link>https://example.com/linux</link>
      <description><![CDATA[<p>All about <img src="https://example.com/linux.png"/> Linux</p>]]></description>
      <pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate>
      <author>nixie@example.com (Nixie Pixel)</author>
      <category>Linux</category>
      <category>Open Source</category>
    </item>
    <item>
      <title>Just Audio</title>
      <link>https://example.com/audio</link>
      <description>A podcast episode</description>
      <enclosure url="https://example.com/audio.mp3" type="audio/mpeg" length="123"/>
    </item>
    <item>
      <title>With Image Enclosure</title>
      <link>https://example.com/photo</link>
      <description>A photo post</description>
      <enclosure url="https://example.com/photo.png" type="image/png" length="456"/>
    </item>
  </channel>
</rss>`

const atomFeed = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Nixie Vlog</title>
  <id>urn:uuid:1</id>
  <updated>2006-01-02T15:04:05Z</updated>
  <entry>
    <title>Vlog Entry</title>
    <id>urn:uuid:2</id>
    <link href="https://example.com/vlog"/>
    <updated>2006-01-03T15:04:05Z</updated>
    <content type="html">Vlog content</content>
    <category term="Vlog"/>
    <author><name>Nixie</name></author>
  </entry>
</feed>`

func feedServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		switch r.URL.Path {
		case "/rss.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rssFeed))
		case "/atom.xml":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(atomFeed))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestService_GetRecentPosts_ParsesRSSAndAtom(t *testing.T) {
	srv := feedServer(t, nil)
	svc := NewService(&config.Config{BlogURLs: []string{
		srv.URL + "/rss.xml",
		srv.URL + "/atom.xml",
	}})

	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 4 {
		t.Fatalf("got %d posts, want 4: %+v", len(posts), posts)
	}

	// Newest first: the Atom entry is dated Jan 3, the RSS item Jan 2, and the
	// two undated items keep their feed order at the end.
	vlog := posts[0]
	if vlog.Title != "Vlog Entry" {
		t.Errorf("got atom title %q", vlog.Title)
	}
	if vlog.PublishedAt != "January 03, 2006" {
		t.Errorf("got atom date %q, want the updated date", vlog.PublishedAt)
	}
	if vlog.Author != "Nixie" {
		t.Errorf("got atom author %q", vlog.Author)
	}
	if !strings.Contains(vlog.Content, "Vlog content") {
		t.Errorf("got atom content %q", vlog.Content)
	}
	if len(vlog.Categories) != 1 || vlog.Categories[0] != "Vlog" {
		t.Errorf("got atom categories %v", vlog.Categories)
	}

	linux := posts[1]
	if linux.Title != "Linux Adventures" {
		t.Errorf("got title %q", linux.Title)
	}
	if linux.URL != "https://example.com/linux" {
		t.Errorf("got URL %q", linux.URL)
	}
	if linux.PublishedAt != "January 02, 2006" {
		t.Errorf("got published date %q", linux.PublishedAt)
	}
	if linux.Author == "" {
		t.Error("expected an author from the RSS author element")
	}
	if linux.ImageURL != "https://example.com/linux.png" {
		t.Errorf("got image %q, want the <img> found in the description", linux.ImageURL)
	}
	if len(linux.Categories) != 2 {
		t.Errorf("got categories %v, want 2", linux.Categories)
	}

	// Audio enclosures are not images.
	if posts[2].ImageURL != "" {
		t.Errorf("audio enclosure should not become an image, got %q", posts[2].ImageURL)
	}
	// Image enclosures are.
	if posts[3].ImageURL != "https://example.com/photo.png" {
		t.Errorf("got image %q, want the image enclosure", posts[3].ImageURL)
	}
}

func TestService_GetRecentPosts_DeduplicatesAcrossFeeds(t *testing.T) {
	srv := feedServer(t, nil)
	svc := NewService(&config.Config{BlogURLs: []string{
		srv.URL + "/rss.xml",
		srv.URL + "/rss.xml",
	}})

	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 3 {
		t.Fatalf("got %d posts, want 3 unique posts: %+v", len(posts), posts)
	}
	seen := make(map[string]bool)
	for _, post := range posts {
		if seen[post.URL] {
			t.Errorf("duplicate post URL %q", post.URL)
		}
		seen[post.URL] = true
	}
}

func TestService_GetRecentPosts_SortsNewestFirst(t *testing.T) {
	const feed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Nixie Blog</title>
    <link>https://example.com</link>
    <description>Blog</description>
    <item>
      <title>Oldest</title>
      <link>https://example.com/oldest</link>
      <pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate>
    </item>
    <item>
      <title>Undated</title>
      <link>https://example.com/undated</link>
    </item>
    <item>
      <title>Newest</title>
      <link>https://example.com/newest</link>
      <pubDate>Wed, 04 Jan 2006 15:04:05 GMT</pubDate>
    </item>
    <item>
      <title>Middle</title>
      <link>https://example.com/middle</link>
      <pubDate>Tue, 03 Jan 2006 15:04:05 GMT</pubDate>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(feed))
	}))
	t.Cleanup(srv.Close)

	svc := NewService(&config.Config{BlogURLs: []string{srv.URL + "/rss.xml"}})
	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}

	want := []string{"Newest", "Middle", "Oldest", "Undated"}
	if len(posts) != len(want) {
		t.Fatalf("got %d posts, want %d", len(posts), len(want))
	}
	for i, title := range want {
		if posts[i].Title != title {
			t.Errorf("post %d: got %q, want %q", i, posts[i].Title, title)
		}
	}
}

func TestService_GetRecentPosts_Caches(t *testing.T) {
	var hits atomic.Int32
	srv := feedServer(t, &hits)
	svc := NewService(&config.Config{BlogURLs: []string{srv.URL + "/rss.xml"}})

	first, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if hits.Load() != 1 {
		t.Errorf("got %d feed requests, want 1 (second call should be cached)", hits.Load())
	}
	if len(first) != len(second) || first[0].Title != second[0].Title {
		t.Errorf("cached posts differ: %+v vs %+v", first, second)
	}
}

func TestService_GetRecentPosts_SkipsBrokenFeed(t *testing.T) {
	srv := feedServer(t, nil)
	svc := NewService(&config.Config{BlogURLs: []string{
		srv.URL + "/missing.xml",
		srv.URL + "/rss.xml",
	}})

	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 3 {
		t.Errorf("got %d posts, want the 3 from the working feed", len(posts))
	}
}

func TestService_GetRecentPosts_AllFeedsFail(t *testing.T) {
	var hits atomic.Int32
	srv := feedServer(t, &hits)
	svc := NewService(&config.Config{BlogURLs: []string{
		srv.URL + "/missing.xml",
		srv.URL + "/also-missing.xml",
	}})

	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts must not fail when feeds are unreachable: %v", err)
	}
	if len(posts) != 0 {
		t.Errorf("got %d posts, want none", len(posts))
	}

	// A total failure must not be cached, otherwise a transient outage (or a
	// fixed URL) would leave the blog empty until the process restarts.
	if _, err := svc.GetRecentPosts(context.Background()); err != nil {
		t.Fatalf("second GetRecentPosts: %v", err)
	}
	if got := hits.Load(); got != 4 {
		t.Errorf("got %d feed requests, want 4 (failures must not be cached)", got)
	}
}

func TestService_GetRecentPosts_NoURLsConfigured(t *testing.T) {
	svc := NewService(&config.Config{})

	posts, err := svc.GetRecentPosts(context.Background())
	if err != nil {
		t.Fatalf("GetRecentPosts: %v", err)
	}
	if len(posts) != 0 {
		t.Errorf("got %d posts, want none", len(posts))
	}
}
