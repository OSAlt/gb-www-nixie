package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OSAlt/gb-www-nixie/internal/config"
)

func TestWithCacheHeaders(t *testing.T) {
	handler := withCacheHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name      string
		method    string
		path      string
		wantCache bool
	}{
		{name: "page", method: http.MethodGet, path: "/", wantCache: true},
		{name: "page head", method: http.MethodHead, path: "/", wantCache: true},
		{name: "blog", method: http.MethodGet, path: "/blog", wantCache: true},
		{name: "api", method: http.MethodGet, path: "/api/v1/social/count/all", wantCache: false},
		{name: "static", method: http.MethodGet, path: "/static/css/style.css", wantCache: false},
		{name: "post", method: http.MethodPost, path: "/", wantCache: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			got := rec.Header().Get("Cache-Control")
			if tc.wantCache && got == "" {
				t.Errorf("%s: missing Cache-Control", tc.path)
			}
			if !tc.wantCache && got != "" {
				t.Errorf("%s: Cache-Control = %q, want none", tc.path, got)
			}
		})
	}
}

func TestSplitNonEmpty(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{in: "", want: []string{}},
		{in: "only", want: []string{"only"}},
		{in: "a,b , ,c", want: []string{"a", "b", "c"}},
		{in: " NixiePixelGB , user/nixiedoeslinux ", want: []string{"NixiePixelGB", "user/nixiedoeslinux"}},
	}

	for _, tc := range tests {
		got := splitNonEmpty(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("splitNonEmpty(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitNonEmpty(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestNewSocialAggregator(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{name: "nothing configured", cfg: &config.Config{}, want: false},
		{name: "metrika only", cfg: &config.Config{MetrikaApiKey: "key"}, want: true},
		{name: "youtube only", cfg: &config.Config{YoutubeApiKey: "key"}, want: true},
		{
			name: "every fallback",
			cfg: &config.Config{
				YoutubeApiKey:      "youtube",
				YoutubeChannels:    "a, b",
				TwitchClientId:     "id",
				TwitchClientSecret: "secret",
				DiscordBotToken:    "token",
				DiscordGuildId:     "guild",
				SocialScrapeURL:    "https://pulse.example",
			},
			want: true,
		},
		{
			name: "twitch needs both credentials",
			cfg:  &config.Config{TwitchClientId: "id"},
			want: false,
		},
		{
			name: "discord needs both credentials",
			cfg:  &config.Config{DiscordBotToken: "token"},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agg, ok := newSocialAggregator(tc.cfg)
			if ok != tc.want {
				t.Fatalf("ok = %v, want %v", ok, tc.want)
			}
			if ok && agg == nil {
				t.Fatal("expected a non-nil aggregator")
			}
			if !ok && agg != nil {
				t.Fatal("expected a nil aggregator")
			}
		})
	}
}
