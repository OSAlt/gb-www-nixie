package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_Channel_HandleFallback(t *testing.T) {
	var selectors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("forHandle") != "":
			selectors = append(selectors, "forHandle")
			fmt.Fprint(w, `{"items":[]}`)
		case q.Get("forUsername") != "":
			selectors = append(selectors, "forUsername")
			fmt.Fprint(w, `{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU123"}},"statistics":{"subscriberCount":"202000"}}]}`)
		default:
			t.Errorf("unexpected query %v", q)
		}
	}))
	t.Cleanup(srv.Close)

	client := Client{APIKey: "key", BaseURL: srv.URL}
	info, err := client.Channel(context.Background(), "nixiedoeslinux")
	if err != nil {
		t.Fatalf("Channel: %v", err)
	}
	if info.UploadsPlaylist != "UU123" {
		t.Errorf("uploads = %q", info.UploadsPlaylist)
	}
	if info.SubscriberCount != 202000 {
		t.Errorf("subscribers = %d", info.SubscriberCount)
	}
	if len(selectors) != 2 || selectors[0] != "forHandle" || selectors[1] != "forUsername" {
		t.Errorf("selectors = %v, want forHandle then forUsername", selectors)
	}
}

func TestClient_Channel_ChannelID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("id"); got != "UC123" {
			t.Errorf("id = %q", got)
		}
		if got := r.URL.Query().Get("key"); got != "key" {
			t.Errorf("key = %q", got)
		}
		fmt.Fprint(w, `{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU123"}}}]}`)
	}))
	t.Cleanup(srv.Close)

	info, err := Client{APIKey: "key", BaseURL: srv.URL}.Channel(context.Background(), "UC123")
	if err != nil {
		t.Fatalf("Channel: %v", err)
	}
	if info.UploadsPlaylist != "UU123" {
		t.Errorf("uploads = %q", info.UploadsPlaylist)
	}
	if info.SubscriberCount != 0 {
		t.Errorf("subscribers = %d, want 0 when statistics are absent", info.SubscriberCount)
	}
}

func TestClient_Channel_NoAPIKey(t *testing.T) {
	if _, err := (Client{}).Channel(context.Background(), "NixiePixel"); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("err = %v, want ErrNoAPIKey", err)
	}
}

func TestClient_Channel_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "quota exceeded", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	if _, err := (Client{APIKey: "key", BaseURL: srv.URL}).Channel(context.Background(), "NixiePixel"); err == nil {
		t.Fatal("expected an HTTP error")
	}
}

func TestClient_Channel_InvalidSubscriberCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"items":[{"statistics":{"subscriberCount":"not-a-number"}}]}`)
	}))
	t.Cleanup(srv.Close)

	if _, err := (Client{APIKey: "key", BaseURL: srv.URL}).Channel(context.Background(), "NixiePixel"); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestClient_Channel_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"items":[]}`)
	}))
	t.Cleanup(srv.Close)

	if _, err := (Client{APIKey: "key", BaseURL: srv.URL}).Channel(context.Background(), "missing"); err == nil {
		t.Fatal("expected a not-found error")
	}
}

func TestClient_PlaylistItems(t *testing.T) {
	published := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("playlistId"); got != "UU123" {
			t.Errorf("playlistId = %q", got)
		}
		if got := r.URL.Query().Get("maxResults"); got != "6" {
			t.Errorf("maxResults = %q", got)
		}
		fmt.Fprintf(w, `{"items":[
		  {"snippet":{"title":" First ","publishedAt":%q,"resourceId":{"videoId":"v1"},"thumbnails":{"default":{"url":"d.jpg"},"maxres":{"url":"m.jpg"}}}},
		  {"snippet":{"title":"No id","publishedAt":%q,"resourceId":{"videoId":""}}}
		]}`, published.Format(time.RFC3339), published.Format(time.RFC3339))
	}))
	t.Cleanup(srv.Close)

	videos, err := Client{APIKey: "key", BaseURL: srv.URL}.PlaylistItems(context.Background(), "UU123", 6)
	if err != nil {
		t.Fatalf("PlaylistItems: %v", err)
	}
	if len(videos) != 1 {
		t.Fatalf("got %d videos, want 1 (items without an id are skipped)", len(videos))
	}
	if videos[0].ID != "v1" || videos[0].Title != "First" || videos[0].ImageURL != "m.jpg" {
		t.Errorf("video = %+v", videos[0])
	}
	if !videos[0].PublishedAt.Equal(published) {
		t.Errorf("published = %v, want %v", videos[0].PublishedAt, published)
	}
}

func TestClient_PlaylistItems_NoAPIKey(t *testing.T) {
	if _, err := (Client{}).PlaylistItems(context.Background(), "UU123", 6); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("err = %v, want ErrNoAPIKey", err)
	}
}

func TestClient_PlaylistItems_NonPositiveCount(t *testing.T) {
	videos, err := Client{APIKey: "key"}.PlaylistItems(context.Background(), "UU123", 0)
	if err != nil {
		t.Fatalf("PlaylistItems: %v", err)
	}
	if videos != nil {
		t.Errorf("got %v, want nil", videos)
	}
}
