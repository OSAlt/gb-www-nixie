package social

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

const pulseDefaultBaseURL = "https://pulse.walls.sh"

// Scrape fetches follower counts from a public-profile scraping aggregator
// (Pulse: GET <base>/profile?url=<profile-url>), used for platforms with no
// usable free official API (TikTok, Facebook, plus X/Instagram as
// best-effort). Scrapers are inherently best-effort: login walls and bot
// protections make individual platforms fail, and the caller keeps the
// last-known-good value for anything missing.
type Scrape struct {
	// BaseURL is overridable for tests.
	BaseURL string
	client  httpClient

	// Profiles maps the chart's lowercase platform keys to public profile
	// URLs to scrape.
	Profiles map[string]string
}

// DefaultProfiles are the profile URLs for the site's socials.
func DefaultProfiles() map[string]string {
	return map[string]string{
		"tiktok":    "https://www.tiktok.com/@nixiepixel",
		"facebook":  "https://www.facebook.com/nixiepixel",
		"twitter":   "https://x.com/NixiePixel",
		"instagram": "https://www.instagram.com/nixie.pixel",
	}
}

var _ Fetcher = ScrapePlatform{}

// Fetchers returns one Fetcher per configured profile so the Aggregator
// can treat scraped platforms like any other.
func (s *Scrape) Fetchers() []Fetcher {
	fetchers := make([]Fetcher, 0, len(s.Profiles))
	for key := range s.Profiles {
		fetchers = append(fetchers, ScrapePlatform{scrape: s, key: key})
	}
	return fetchers
}

// ScrapePlatform adapts Scrape to the single-platform Fetcher interface.
type ScrapePlatform struct {
	scrape *Scrape
	key    string
}

func (p ScrapePlatform) Key() string { return p.key }

func (p ScrapePlatform) Fetch(ctx context.Context) (int, error) {
	return p.scrape.fetchOne(ctx, p.key)
}

func (s *Scrape) fetchOne(ctx context.Context, key string) (int, error) {
	profileURL, ok := s.Profiles[key]
	if !ok {
		return 0, fmt.Errorf("no profile URL configured for %q", key)
	}
	base := s.BaseURL
	if base == "" {
		base = pulseDefaultBaseURL
	}

	reqURL := fmt.Sprintf("%s/profile?url=%s", base, url.QueryEscape(profileURL))
	var payload struct {
		Error     string      `json:"error"`
		Followers json.Number `json:"followers"`
	}
	if err := fetchJSON(ctx, s.client, reqURL, nil, &payload); err != nil {
		return 0, err
	}
	if payload.Error != "" {
		// Aggregators report per-profile problems in-band, e.g. instagram
		// "login_required".
		return 0, fmt.Errorf("%s", payload.Error)
	}
	if payload.Followers == "" {
		return 0, fmt.Errorf("no follower count in response")
	}

	// Counts can be plain ints or floats depending on how rounded the
	// platform's own display value is.
	f, err := payload.Followers.Float64()
	if err != nil {
		return 0, fmt.Errorf("parse followers %q: %w", payload.Followers, err)
	}
	return int(f), nil
}
