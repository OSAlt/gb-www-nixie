package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
