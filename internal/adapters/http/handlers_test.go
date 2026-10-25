package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/stretchr/testify/mock"
)

func TestSocial_RendersAllPlatforms(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.Social(rec, httptest.NewRequest(http.MethodGet, "/social", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, url := range []string{
		"https://www.youtube.com/user/nixiedoeslinux",
		"https://twitter.com/NixiePixel",
		"https://www.facebook.com/nixiepixel",
		"https://www.instagram.com/nixie.pixel",
		"https://www.twitch.tv/nixiepixel",
		"https://www.patreon.com/nixiepixel",
		"https://github.com/nixiepixel",
		"https://keybase.io/nixiepixel",
	} {
		if !strings.Contains(body, url) {
			t.Errorf("expected page to contain %s", url)
		}
	}

	if !strings.Contains(body, `id="chart"`) {
		t.Errorf("expected stats chart container on social page")
	}

	if !strings.Contains(body, `id="llama-hat-select"`) {
		t.Errorf("expected holiday hat selector on social page")
	}

	if !strings.Contains(body, `id="curator-feed-default-feed-layout"`) {
		t.Errorf("expected Curator feed container on social page")
	}
}

type socialHealthBody struct {
	Status         string   `json:"status"`
	StalePlatforms []string `json:"stale_platforms"`
	Platforms      map[string]struct {
		Followers  int   `json:"followers"`
		AgeSeconds int64 `json:"age_seconds"`
		Stale      bool  `json:"stale"`
	} `json:"platforms"`
}

func decodeSocialHealth(t *testing.T, rec *httptest.ResponseRecorder) socialHealthBody {
	t.Helper()
	var body socialHealthBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v (body: %s)", err, rec.Body.String())
	}
	return body
}

func TestAPISocialHealth_OK(t *testing.T) {
	now := time.Now()
	db := mocks.NewMockDBService(t)
	db.EXPECT().GetSocialMetricStates(mock.Anything).Return([]domain.SocialMetricState{
		{Platform: "youtube", Followers: 304000, RefreshedAt: now.Add(-time.Hour)},
		{Platform: "discord", Followers: 5425, RefreshedAt: now},
	}, nil)
	h := NewHandler(nil, nil, db, nil, &config.Config{SocialRefreshSeconds: 86400})

	rec := httptest.NewRecorder()
	h.APISocialHealth(rec, httptest.NewRequest(http.MethodGet, "/api/v1/social/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := decodeSocialHealth(t, rec)
	if body.Status != "ok" {
		t.Errorf("got status %q, want ok", body.Status)
	}
	if len(body.StalePlatforms) != 0 {
		t.Errorf("expected no stale platforms, got %v", body.StalePlatforms)
	}
	if body.Platforms["youtube"].Followers != 304000 {
		t.Errorf("got youtube %+v", body.Platforms["youtube"])
	}
}

func TestAPISocialHealth_DegradedWhenStale(t *testing.T) {
	now := time.Now()
	db := mocks.NewMockDBService(t)
	db.EXPECT().GetSocialMetricStates(mock.Anything).Return([]domain.SocialMetricState{
		{Platform: "facebook", Followers: 41655, RefreshedAt: now.Add(-72 * time.Hour)},
		{Platform: "tiktok", Followers: 3616, RefreshedAt: now},
	}, nil)
	h := NewHandler(nil, nil, db, nil, &config.Config{SocialRefreshSeconds: 86400})

	rec := httptest.NewRecorder()
	h.APISocialHealth(rec, httptest.NewRequest(http.MethodGet, "/api/v1/social/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 even when degraded", rec.Code)
	}
	body := decodeSocialHealth(t, rec)
	if body.Status != "degraded" {
		t.Errorf("got status %q, want degraded", body.Status)
	}
	if len(body.StalePlatforms) != 1 || body.StalePlatforms[0] != "facebook" {
		t.Errorf("got stale platforms %v, want [facebook]", body.StalePlatforms)
	}
	if !body.Platforms["facebook"].Stale {
		t.Error("facebook should be marked stale")
	}
	if body.Platforms["tiktok"].Stale {
		t.Error("fresh platform must not be marked stale")
	}
}

func TestAPISocialHealth_NoDatabase(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.APISocialHealth(rec, httptest.NewRequest(http.MethodGet, "/api/v1/social/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"unavailable"`) {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestAPISocialHealth_DatabaseError(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().GetSocialMetricStates(mock.Anything).Return(nil, context.DeadlineExceeded)
	h := NewHandler(nil, nil, db, nil, &config.Config{SocialRefreshSeconds: 86400})

	rec := httptest.NewRecorder()
	h.APISocialHealth(rec, httptest.NewRequest(http.MethodGet, "/api/v1/social/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d, want 503", rec.Code)
	}
}

func TestAPISocialHealth_EmptyTableIsDegraded(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().GetSocialMetricStates(mock.Anything).Return(nil, nil)
	h := NewHandler(nil, nil, db, nil, &config.Config{SocialRefreshSeconds: 86400})

	rec := httptest.NewRecorder()
	h.APISocialHealth(rec, httptest.NewRequest(http.MethodGet, "/api/v1/social/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if body := decodeSocialHealth(t, rec); body.Status != "degraded" {
		t.Errorf("got status %q, want degraded for an empty table", body.Status)
	}
}

type contentHealthBody struct {
	Status     string   `json:"status"`
	StaleFeeds []string `json:"stale_feeds"`
	Feeds      map[string]struct {
		Items   int  `json:"items"`
		Stale   bool `json:"stale"`
		Missing bool `json:"missing"`
	} `json:"feeds"`
}

func decodeContentHealth(t *testing.T, rec *httptest.ResponseRecorder) contentHealthBody {
	t.Helper()
	var body contentHealthBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v (body: %s)", err, rec.Body.String())
	}
	return body
}

func TestAPIContentHealth_OK(t *testing.T) {
	now := time.Now()
	media := mocks.NewMockMediaService(t)
	media.EXPECT().CacheState(mock.Anything).Return(ports.CacheState{
		Items: 20, FetchedAt: now, ExpiresAt: now.Add(time.Hour),
	}, nil)
	blog := mocks.NewMockBlogService(t)
	blog.EXPECT().CacheState(mock.Anything).Return(ports.CacheState{
		Items: 12, FetchedAt: now, ExpiresAt: now.Add(time.Hour),
	}, nil)
	contact := mocks.NewMockContactService(t)
	contact.EXPECT().CacheState(mock.Anything).Return(ports.CacheState{
		Items: 4, FetchedAt: now, ExpiresAt: now.Add(time.Hour),
	}, nil)

	h := NewHandler(media, contact, nil, blog, nil)

	rec := httptest.NewRecorder()
	h.APIContentHealth(rec, httptest.NewRequest(http.MethodGet, "/api/v1/content/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := decodeContentHealth(t, rec)
	if body.Status != "ok" {
		t.Errorf("got status %q, want ok", body.Status)
	}
	if len(body.StaleFeeds) != 0 {
		t.Errorf("expected no stale feeds, got %v", body.StaleFeeds)
	}
	if body.Feeds["media"].Items != 20 || body.Feeds["blog"].Items != 12 {
		t.Errorf("got feeds %+v", body.Feeds)
	}
}

func TestAPIContentHealth_DegradedWhenStaleOrMissing(t *testing.T) {
	now := time.Now()
	media := mocks.NewMockMediaService(t)
	media.EXPECT().CacheState(mock.Anything).Return(ports.CacheState{
		Items: 20, FetchedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour), Stale: true,
	}, nil)
	blog := mocks.NewMockBlogService(t)
	blog.EXPECT().CacheState(mock.Anything).Return(ports.CacheState{Missing: true, Stale: true}, nil)
	contact := mocks.NewMockContactService(t)
	contact.EXPECT().CacheState(mock.Anything).Return(ports.CacheState{
		Items: 4, FetchedAt: now, ExpiresAt: now.Add(time.Hour),
	}, nil)

	h := NewHandler(media, contact, nil, blog, nil)

	rec := httptest.NewRecorder()
	h.APIContentHealth(rec, httptest.NewRequest(http.MethodGet, "/api/v1/content/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 even when degraded", rec.Code)
	}
	body := decodeContentHealth(t, rec)
	if body.Status != "degraded" {
		t.Errorf("got status %q, want degraded", body.Status)
	}
	if len(body.StaleFeeds) != 2 || body.StaleFeeds[0] != "blog" || body.StaleFeeds[1] != "media" {
		t.Errorf("got stale feeds %v, want [blog media]", body.StaleFeeds)
	}
	if !body.Feeds["blog"].Missing {
		t.Error("blog should be marked missing")
	}
}

func TestAPIContentHealth_ReporterError(t *testing.T) {
	media := mocks.NewMockMediaService(t)
	media.EXPECT().CacheState(mock.Anything).Return(ports.CacheState{}, context.DeadlineExceeded)

	h := NewHandler(media, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.APIContentHealth(rec, httptest.NewRequest(http.MethodGet, "/api/v1/content/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := decodeContentHealth(t, rec)
	if body.Status != "degraded" {
		t.Errorf("got status %q, want degraded", body.Status)
	}
	if len(body.StaleFeeds) != 1 || body.StaleFeeds[0] != "media" {
		t.Errorf("got stale feeds %v, want [media]", body.StaleFeeds)
	}
}
