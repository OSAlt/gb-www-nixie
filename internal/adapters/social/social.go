// Package social fetches follower/member counts for the platforms shown on
// the site's social llama chart. Each platform has its own Fetcher; some use
// official free APIs (YouTube, Twitch, Discord) and some use a free public
// scraping aggregator. See doc/social_migration.md for credential setup.
package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

const (
	fetchTimeout = 15 * time.Second
	// bulkFetchTimeout covers one batched request that resolves up to 10
	// resources, so it gets a little more headroom than a single fetch.
	bulkFetchTimeout = 30 * time.Second
)

// Fetcher retrieves the current follower/member count for exactly one
// platform. Key() must match the lowercase platform keys used by
// static/js/stats.js ("youtube", "twitter", "twitch", "discord",
// "facebook", "instagram", "tiktok").
type Fetcher interface {
	Key() string
	Fetch(ctx context.Context) (int, error)
}

// BulkFetcher retrieves several platforms in a single request (Metrika).
// Its results are merged first; per-platform Fetchers then only fill the
// platforms the bulk source could not supply.
type BulkFetcher interface {
	Name() string
	FetchAll(ctx context.Context) (map[string]int, error)
}

// httpClient is the subset of net/http.Client platform clients need, so
// tests can point a client at an httptest server via BaseURL.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// defaultHTTPClient is used by platform clients that don't set their own.
var defaultHTTPClient = &http.Client{Timeout: fetchTimeout}

// Aggregator runs all configured fetchers and returns only the counts it
// could obtain. A failed platform is reported via slog and simply left out of
// the result so callers keep their last-known-good value.
//
// Bulk sources (Metrika) run first and cover most platforms with one request.
// The cheaper per-platform fetchers stay registered as gap-fillers: they only
// run for platforms the bulk source could not resolve this cycle.
type Aggregator struct {
	bulk     []BulkFetcher
	fetchers []Fetcher
}

func NewAggregator(fetchers ...Fetcher) *Aggregator {
	return &Aggregator{fetchers: fetchers}
}

// NewAggregatorWithBulk builds an aggregator whose bulk sources take
// precedence, with per-platform fetchers filling any gaps.
func NewAggregatorWithBulk(bulk []BulkFetcher, fetchers ...Fetcher) *Aggregator {
	return &Aggregator{bulk: bulk, fetchers: fetchers}
}

// Fetch collects counts from all sources. Returns a map with one entry per
// successful platform; an empty map means every source failed.
func (a *Aggregator) Fetch(ctx context.Context) map[string]int {
	out := make(map[string]int)

	for _, b := range a.bulk {
		counts, err := a.fetchBulk(ctx, b)
		if err != nil {
			// Best-effort: the gap-fillers below still run.
			slog.Warn("social bulk fetch failed", "source", b.Name(), "error", err)
			continue
		}
		for platform, count := range counts {
			out[platform] = count
		}
	}

	// Read-only once the goroutines start.
	covered := make(map[string]bool, len(out))
	for platform := range out {
		covered[platform] = true
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	for _, f := range a.fetchers {
		if covered[f.Key()] {
			continue
		}
		wg.Add(1)
		go func(f Fetcher) {
			defer wg.Done()
			// A panicking platform adapter must not take down the process.
			defer recoverPanic(f.Key())

			fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
			defer cancel()

			count, err := f.Fetch(fetchCtx)
			if err != nil {
				// Best-effort by design: the DB keeps last-known-good values.
				slog.Warn("social count fetch failed", "platform", f.Key(), "error", err)
				return
			}
			// No defer here: the mutex must never be held across a panic.
			mu.Lock()
			out[f.Key()] = count
			mu.Unlock()
		}(f)
	}
	wg.Wait()

	return out
}

// fetchBulk runs one bulk source with panic containment, so a broken adapter
// degrades to "this source failed" instead of killing the server.
func (a *Aggregator) fetchBulk(ctx context.Context, b BulkFetcher) (counts map[string]int, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic while fetching social counts",
				"source", b.Name(), "panic", r, "stack", string(debug.Stack()))
			counts, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()

	bulkCtx, cancel := context.WithTimeout(ctx, bulkFetchTimeout)
	defer cancel()
	return b.FetchAll(bulkCtx)
}

// recoverPanic is deferred around a single fetch so that a panic is logged and
// contained. Call it directly via defer (recover only works from the deferred
// function itself).
func recoverPanic(source string) {
	if r := recover(); r != nil {
		slog.Error("panic while fetching social counts",
			"source", source, "panic", r, "stack", string(debug.Stack()))
	}
}

func errMissingConfig(configKeys string) error {
	return fmt.Errorf("missing credentials: %s not set", configKeys)
}

// newRequest builds an HTTP request with optional body and headers.
func newRequest(ctx context.Context, method, reqURL string, body io.Reader, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// decodeBody validates the status code and JSON-decodes the body into out.
func decodeBody(resp *http.Response, out any) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 256))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w (body: %s)", err, truncate(string(body), 256))
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// fetchJSON is the shared GET+JSON helper for platform clients; a nil
// client falls back to the package-default HTTP client.
func fetchJSON(ctx context.Context, client httpClient, reqURL string, headers map[string]string, out any) error {
	if client == nil {
		client = defaultHTTPClient
	}
	req, err := newRequest(ctx, http.MethodGet, reqURL, nil, headers)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s failed: %w", reqURL, err)
	}
	defer resp.Body.Close()
	if err := decodeBody(resp, out); err != nil {
		return fmt.Errorf("GET %s: %w", reqURL, err)
	}
	return nil
}
