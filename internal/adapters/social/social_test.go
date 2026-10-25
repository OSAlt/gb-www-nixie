package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// YouTube
func TestYouTube_Fetch(t *testing.T) {
	var gotQueries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQueries = append(gotQueries, r.URL.Query().Get("forHandle")+":"+r.URL.Query().Get("forUsername"))
		fmt.Fprintf(w, `{"items":[{"statistics":{"subscriberCount":"202000"}}]}`)
	}))
	defer srv.Close()

	f := &YouTube{APIKey: "test-key", Channels: []string{"nixiedoeslinux"}, BaseURL: srv.URL}
	count, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 202000 {
		t.Errorf("got %d, want 202000", count)
	}
	if gotQueries[0] != "nixiedoeslinux:" {
		t.Errorf("expected forHandle selector first, got %q", gotQueries[0])
	}
}

func TestYouTube_Fetch_CombinesChannels(t *testing.T) {
	subscribers := map[string]string{
		"NixiePixelGB":        "102000",
		"user/nixiedoeslinux": "202000",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("forHandle") == "NixiePixelGB":
			fmt.Fprintf(w, `{"items":[{"statistics":{"subscriberCount":"%s"}}]}`, subscribers["NixiePixelGB"])
		case q.Get("forUsername") == "nixiedoeslinux":
			fmt.Fprintf(w, `{"items":[{"statistics":{"subscriberCount":"%s"}}]}`, subscribers["user/nixiedoeslinux"])
		default:
			fmt.Fprint(w, `{"items":[]}`)
		}
	}))
	defer srv.Close()

	f := &YouTube{
		APIKey:   "test-key",
		Channels: []string{"NixiePixelGB", "user/nixiedoeslinux"},
		BaseURL:  srv.URL,
	}
	count, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 102000 + 202000
	if count != 304000 {
		t.Errorf("got %d, want 304000", count)
	}
}

func TestYouTube_Fetch_AllOrNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("forHandle") == "NixiePixelGB" {
			fmt.Fprint(w, `{"items":[{"statistics":{"subscriberCount":"102000"}}]}`)
			return
		}
		// The second channel cannot be resolved: no partial sum allowed.
		fmt.Fprint(w, `{"items":[]}`)
	}))
	defer srv.Close()

	f := &YouTube{
		APIKey:   "test-key",
		Channels: []string{"NixiePixelGB", "does-not-exist"},
		BaseURL:  srv.URL,
	}
	if _, err := f.Fetch(context.Background()); err == nil {
		t.Fatal("expected error so the last-known-good combined total is kept")
	}
}

func TestYouTube_NoChannels(t *testing.T) {
	f := &YouTube{APIKey: "test-key", Channels: nil}
	if _, err := f.Fetch(context.Background()); err == nil {
		t.Fatal("expected error when no channels configured")
	}
}

func TestYouTube_Fetch_ChannelID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "UC123" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"items":[]}`)
			return
		}
		fmt.Fprintf(w, `{"items":[{"statistics":{"subscriberCount":"500"}}]}`)
	}))
	defer srv.Close()

	f := &YouTube{APIKey: "test-key", Channels: []string{"UC123"}, BaseURL: srv.URL}
	count, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 500 {
		t.Errorf("got %d, want 500", count)
	}
}

func TestYouTube_MissingAPIKey(t *testing.T) {
	f := &YouTube{Channels: []string{"nixiedoeslinux"}}
	if _, err := f.Fetch(context.Background()); err == nil {
		t.Fatal("expected error when API key missing")
	}
}

// Twitch
func TestTwitch_Fetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("grant_type") != "client_credentials" || r.FormValue("client_id") == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"access_token":"tok123","expires_in":3600}`)
	})
	mux.HandleFunc("/helix/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" || r.Header.Get("Client-Id") != "cid" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"999"}]}`)
	})
	mux.HandleFunc("/helix/channels/followers", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("broadcaster_id") != "999" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `{"total":2500}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := &Twitch{
		ClientID:     "cid",
		ClientSecret: "secret",
		Login:        "nixiepixel",
		BaseURL:      srv.URL,
		IDBaseURL:    srv.URL,
		client:       srv.Client(),
	}
	count, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2500 {
		t.Errorf("got %d, want 2500", count)
	}
}

// Discord
func TestDiscord_Fetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bot tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !strings.Contains(r.URL.RequestURI(), "/guilds/123") || !strings.Contains(r.URL.RawQuery, "with_counts=true") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `{"approximate_member_count":6336}`)
	}))
	defer srv.Close()

	f := &Discord{BotToken: "tok", GuildID: "123", BaseURL: srv.URL, client: srv.Client()}
	count, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 6336 {
		t.Errorf("got %d, want 6336", count)
	}
}

func TestDiscord_MissingConfig(t *testing.T) {
	f := &Discord{}
	if _, err := f.Fetch(context.Background()); err == nil {
		t.Fatal("expected error when bot token/guild id missing")
	}
}

// Scrape aggregator
func TestScrape_FetchOne(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("url"); got != "https://www.tiktok.com/@nixiepixel" {
			http.Error(w, "bad request: "+got, http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"platform":"tiktok","handle":"nixiepixel","followers":3615,"posts":118}`)
	}))
	defer srv.Close()

	s := &Scrape{
		BaseURL: srv.URL,
		client:  srv.Client(),
		Profiles: map[string]string{
			"tiktok": "https://www.tiktok.com/@nixiepixel",
		},
	}
	count, err := s.fetchOne(context.Background(), "tiktok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3615 {
		t.Errorf("got %d, want 3615", count)
	}
}

func TestScrape_InBandError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Aggregators like Pulse report login walls in-band with HTTP 200.
		fmt.Fprint(w, `{"error":"login_required","detail":"login wall"}`)
	}))
	defer srv.Close()

	s := &Scrape{
		BaseURL: srv.URL,
		client:  srv.Client(),
		Profiles: map[string]string{
			"instagram": "https://www.instagram.com/nixie.pixel",
		},
	}
	if _, err := s.fetchOne(context.Background(), "instagram"); err == nil {
		t.Fatal("expected in-band error to surface")
	}
}

func TestScrape_PlatformWrapper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"followers":41655}`)
	}))
	defer srv.Close()

	s := &Scrape{
		BaseURL: srv.URL,
		client:  srv.Client(),
		Profiles: map[string]string{
			"twitter": "https://x.com/NixiePixel",
		},
	}
	fetchers := s.Fetchers()
	if len(fetchers) != 1 {
		t.Fatalf("got %d fetchers, want 1", len(fetchers))
	}
	if fetchers[0].Key() != "twitter" {
		t.Errorf("got key %q, want \"twitter\"", fetchers[0].Key())
	}
	count, err := fetchers[0].Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 41655 {
		t.Errorf("got %d, want 41655", count)
	}
}

// Aggregator
func TestAggregator_Fetch_MergesSuccesses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"followers":3615}`)
	}))
	defer srv.Close()

	scraper := &Scrape{
		BaseURL: srv.URL,
		client:  srv.Client(),
		Profiles: map[string]string{
			"tiktok": "https://www.tiktok.com/@nixiepixel",
		},
	}
	// Missing credentials => fetch fails and must not appear in the result.
	failing := &YouTube{Channels: []string{"nixiedoeslinux"}}

	agg := NewAggregator(append(scraper.Fetchers(), failing)...)
	got := agg.Fetch(context.Background())

	want := map[string]int{"tiktok": 3615}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Metrika (bulk source)

func assertCounts(t *testing.T, got, want map[string]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for platform, count := range want {
		if got[platform] != count {
			t.Errorf("%s: got %d, want %d", platform, got[platform], count)
		}
	}
}

// metrikaFullPayload mirrors a healthy response for all eight resources.
const metrikaFullPayload = `{"data":[
  {"platform":"youtube","type":"account","identifier":"NixiePixel","status":"available","metrics":{"followers":{"value":202000,"status":"available","semantic":"subscribers"}}},
  {"platform":"youtube","type":"account","identifier":"NixiePixelGB","status":"available","metrics":{"followers":{"value":102000,"status":"available","semantic":"subscribers"}}},
  {"platform":"x","type":"account","identifier":"NixiePixel","status":"available","metrics":{"followers":{"value":26000,"status":"available","semantic":"followers"}}},
  {"platform":"tiktok","type":"account","identifier":"nixiepixel","status":"available","metrics":{"followers":{"value":3616,"status":"available","semantic":"followers"}}},
  {"platform":"facebook","type":"account","identifier":"nixiepixel","status":"available","metrics":{"followers":{"value":41655,"status":"available","semantic":"likes"}}},
  {"platform":"instagram","type":"account","identifier":"nixie.pixel","status":"available","metrics":{"followers":{"value":18000,"status":"available","semantic":"followers"}}},
  {"platform":"twitch","type":"account","identifier":"nixiepixel","status":"available","metrics":{"followers":{"value":2595,"status":"available","semantic":"followers"}}},
  {"platform":"discord","type":"community","identifier":"geekbeacon","status":"available","metrics":{"followers":{"value":5425,"status":"available","semantic":"members"}}}
]}`

func TestMetrika_FetchAll_BatchesEverythingIntoOneRequest(t *testing.T) {
	var (
		requests int
		auth     string
		rawBody  string
		targets  []MetrikaTarget
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/resources/search" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		requests++
		auth = r.Header.Get("Authorization")
		rawBody = string(body)
		var payload struct {
			Resources []MetrikaTarget `json:"resources"`
		}
		_ = json.Unmarshal(body, &payload)
		targets = payload.Resources
		fmt.Fprint(w, metrikaFullPayload)
	}))
	defer srv.Close()

	f := &Metrika{
		APIKey:  "test-token",
		BaseURL: srv.URL,
		Targets: DefaultMetrikaTargets("geekbeacon"),
		client:  srv.Client(),
	}
	counts, err := f.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All eight resources (seven platforms) cost exactly one request.
	if requests != 1 {
		t.Errorf("got %d requests, want 1", requests)
	}
	if auth != "Bearer test-token" {
		t.Errorf("got Authorization %q", auth)
	}
	if len(targets) != 8 {
		t.Errorf("got %d resources in request, want 8", len(targets))
	}
	if !strings.Contains(rawBody, `"type":"community"`) {
		t.Errorf("discord resource should be sent as community: %s", rawBody)
	}
	if strings.Contains(rawBody, "ChartKey") || strings.Contains(rawBody, "chart_key") {
		t.Errorf("ChartKey must not be serialized: %s", rawBody)
	}

	assertCounts(t, counts, map[string]int{
		"youtube":   304000, // 202000 + 102000
		"twitter":   26000,
		"tiktok":    3616,
		"facebook":  41655,
		"instagram": 18000,
		"twitch":    2595,
		"discord":   5425,
	})
}

func TestMetrika_FetchAll_SkipsUnavailablePlatform(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[
		  {"platform":"youtube","type":"account","identifier":"NixiePixel","status":"available","metrics":{"followers":{"value":202000,"status":"available"}}},
		  {"platform":"youtube","type":"account","identifier":"NixiePixelGB","status":"available","metrics":{"followers":{"value":102000,"status":"available"}}},
		  {"platform":"instagram","type":"account","identifier":"nixie.pixel","status":"pending"}
		]}`)
	}))
	defer srv.Close()

	f := &Metrika{
		APIKey:  "test-token",
		BaseURL: srv.URL,
		Targets: DefaultMetrikaTargets("geekbeacon"),
		client:  srv.Client(),
	}
	counts, err := f.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := counts["instagram"]; ok {
		t.Error("pending platform must be omitted so it can be gap-filled")
	}
	if counts["youtube"] != 304000 {
		t.Errorf("youtube: got %d, want 304000", counts["youtube"])
	}
}

func TestMetrika_FetchAll_YouTubeAllOrNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[
		  {"platform":"youtube","type":"account","identifier":"NixiePixel","status":"available","metrics":{"followers":{"value":202000,"status":"available"}}},
		  {"platform":"youtube","type":"account","identifier":"NixiePixelGB","status":"unavailable"},
		  {"platform":"twitch","type":"account","identifier":"nixiepixel","status":"available","metrics":{"followers":{"value":2595,"status":"available"}}}
		]}`)
	}))
	defer srv.Close()

	f := &Metrika{
		APIKey:  "test-token",
		BaseURL: srv.URL,
		Targets: DefaultMetrikaTargets("geekbeacon"),
		client:  srv.Client(),
	}
	counts, err := f.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := counts["youtube"]; ok {
		t.Error("a partial sum must never be charted")
	}
	if counts["twitch"] != 2595 {
		t.Errorf("other platforms must still be returned, got %v", counts)
	}
}

func TestMetrika_FetchAll_ValidationIsolatesAndQuarantines(t *testing.T) {
	var (
		mu       sync.Mutex
		requests int
		batchN   int
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Resources []MetrikaTarget `json:"resources"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)

		mu.Lock()
		requests++
		batch := len(payload.Resources) > 1
		if batch {
			batchN++
		}
		firstBatch := batch && batchN == 1
		mu.Unlock()

		switch {
		case firstBatch:
			// Metrika rejects the whole batch when one identifier is bad.
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"errors":["Resources identifier '@bad' must not include the leading '@'"]}`)
		case len(payload.Resources) == 1 && payload.Resources[0].Identifier == "bad":
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"errors":["Resources identifier 'bad' is not valid on platform 'tiktok'"]}`)
		default:
			fmt.Fprint(w, `{"data":[{"platform":"x","type":"account","identifier":"NixiePixel","status":"available","metrics":{"followers":{"value":26000,"status":"available"}}}]}`)
		}
	}))
	defer srv.Close()

	f := &Metrika{
		APIKey:  "test-token",
		BaseURL: srv.URL,
		client:  srv.Client(),
		Targets: []MetrikaTarget{
			{Platform: "x", Type: "account", Identifier: "NixiePixel", ChartKey: "twitter"},
			{Platform: "tiktok", Type: "account", Identifier: "bad", ChartKey: "tiktok"},
		},
	}

	counts, err := f.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := counts["tiktok"]; ok {
		t.Error("rejected resource must be omitted")
	}
	if counts["twitter"] != 26000 {
		t.Errorf("healthy resource must survive isolation, got %v", counts)
	}
	if requests != 3 { // rejected batch + two single retries
		t.Errorf("got %d requests, want 3", requests)
	}

	// The offender is quarantined, so the next cycle is a single request.
	requests = 0
	counts, err = f.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on second fetch: %v", err)
	}
	if requests != 1 {
		t.Errorf("quarantined identifier should not be retried, got %d requests", requests)
	}
	if counts["twitter"] != 26000 {
		t.Errorf("got %v", counts)
	}
}

func TestMetrika_FetchAll_QuotaExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"You have exceeded your quota of 500 requests, resets on 2026-11-01."}`)
	}))
	defer srv.Close()

	f := &Metrika{APIKey: "test-token", BaseURL: srv.URL, Targets: DefaultMetrikaTargets("geekbeacon"), client: srv.Client()}
	_, err := f.FetchAll(context.Background())
	if err == nil {
		t.Fatal("expected quota error")
	}
	if !strings.Contains(err.Error(), "quota") {
		t.Errorf("error should mention the quota, got %v", err)
	}
}

func TestMetrika_MissingAPIKey(t *testing.T) {
	f := &Metrika{Targets: DefaultMetrikaTargets("geekbeacon")}
	if _, err := f.FetchAll(context.Background()); err == nil {
		t.Fatal("expected error when API key missing")
	}
}

// Aggregator: bulk precedence and gap-filling

type stubFetcher struct {
	key   string
	count int
	err   error
	calls *int32
}

func (s stubFetcher) Key() string { return s.key }

func (s stubFetcher) Fetch(ctx context.Context) (int, error) {
	if s.calls != nil {
		atomic.AddInt32(s.calls, 1)
	}
	return s.count, s.err
}

type stubBulk struct {
	counts map[string]int
	err    error
}

func (s stubBulk) Name() string { return "stub" }

func (s stubBulk) FetchAll(ctx context.Context) (map[string]int, error) {
	return s.counts, s.err
}

func TestAggregator_BulkTakesPrecedenceAndGapFills(t *testing.T) {
	var tiktokCalls, facebookCalls int32
	agg := NewAggregatorWithBulk(
		[]BulkFetcher{stubBulk{counts: map[string]int{"tiktok": 9999}}},
		stubFetcher{key: "tiktok", count: 1, calls: &tiktokCalls},
		stubFetcher{key: "facebook", count: 42, calls: &facebookCalls},
	)

	got := agg.Fetch(context.Background())
	assertCounts(t, got, map[string]int{"tiktok": 9999, "facebook": 42})

	if tiktokCalls != 0 {
		t.Errorf("bulk-covered platform should not be re-fetched, got %d calls", tiktokCalls)
	}
	if facebookCalls != 1 {
		t.Errorf("gap should be filled once, got %d calls", facebookCalls)
	}
}

type panickingFetcher struct{ key string }

func (p panickingFetcher) Key() string { return p.key }

func (p panickingFetcher) Fetch(context.Context) (int, error) {
	panic("simulated adapter bug")
}

type panickingBulk struct{}

func (panickingBulk) Name() string { return "panicky" }

func (panickingBulk) FetchAll(context.Context) (map[string]int, error) {
	panic("simulated bulk bug")
}

func TestAggregator_PanicInFetcherIsContained(t *testing.T) {
	agg := NewAggregator(
		panickingFetcher{key: "facebook"},
		stubFetcher{key: "tiktok", count: 7},
	)

	// A panicking platform must not prevent the others from reporting.
	got := agg.Fetch(context.Background())
	assertCounts(t, got, map[string]int{"tiktok": 7})
}

func TestAggregator_PanicInBulkIsContained(t *testing.T) {
	agg := NewAggregatorWithBulk(
		[]BulkFetcher{panickingBulk{}},
		stubFetcher{key: "tiktok", count: 7},
	)

	// A panicking bulk source degrades to a failed source; gap-fillers run.
	got := agg.Fetch(context.Background())
	assertCounts(t, got, map[string]int{"tiktok": 7})
}

func TestAggregator_BulkFailureFallsBackToPerPlatform(t *testing.T) {
	agg := NewAggregatorWithBulk(
		[]BulkFetcher{stubBulk{err: fmt.Errorf("quota exhausted")}},
		stubFetcher{key: "tiktok", count: 7},
	)

	got := agg.Fetch(context.Background())
	assertCounts(t, got, map[string]int{"tiktok": 7})
}
