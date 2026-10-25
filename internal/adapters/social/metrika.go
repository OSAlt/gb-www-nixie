package social

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

const (
	metrikaDefaultBaseURL = "https://metrika.run"
	// metrikaMaxBatch is the documented per-request resource cap.
	metrikaMaxBatch = 10
)

// errMetrikaValidation marks a 422 response. Metrika rejects the *entire*
// batch when a single identifier is malformed (a leading "@" or a pasted URL
// is enough), so the caller has to isolate and quarantine the offender.
var errMetrikaValidation = errors.New("metrika: request rejected as invalid")

// MetrikaTarget is one resource in a Metrika batch request.
type MetrikaTarget struct {
	Platform   string `json:"platform"`
	Type       string `json:"type"`
	Identifier string `json:"identifier"`

	// ChartKey is the stats.js platform key this resource feeds. Several
	// resources may share one key (the two YouTube channels); their values
	// are summed and all of them must resolve, otherwise the key is skipped
	// so a partial total never looks like an audience drop.
	ChartKey string `json:"-"`
}

// DefaultMetrikaTargets are Nixie's accounts on Metrika's platform codes.
// Identifiers are bare usernames: Metrika rejects "@handles" and URLs with a
// 422. Note that the legacy youtube.com/user/nixiedoeslinux channel resolves
// under the handle "NixiePixel", not "nixiedoeslinux".
func DefaultMetrikaTargets(discordVanity string) []MetrikaTarget {
	targets := []MetrikaTarget{
		{Platform: "youtube", Type: "account", Identifier: "NixiePixel", ChartKey: "youtube"},
		{Platform: "youtube", Type: "account", Identifier: "NixiePixelGB", ChartKey: "youtube"},
		{Platform: "x", Type: "account", Identifier: "NixiePixel", ChartKey: "twitter"},
		{Platform: "tiktok", Type: "account", Identifier: "nixiepixel", ChartKey: "tiktok"},
		{Platform: "facebook", Type: "account", Identifier: "nixiepixel", ChartKey: "facebook"},
		{Platform: "instagram", Type: "account", Identifier: "nixie.pixel", ChartKey: "instagram"},
		{Platform: "twitch", Type: "account", Identifier: "nixiepixel", ChartKey: "twitch"},
	}
	if discordVanity != "" {
		// Discord has no handles: the identifier is the server's vanity
		// invite code (discord.gg/geekbeacon => "geekbeacon").
		targets = append(targets, MetrikaTarget{
			Platform:   "discord",
			Type:       "community",
			Identifier: discordVanity,
			ChartKey:   "discord",
		})
	}
	return targets
}

// Metrika fetches every platform from metrika.run in one batched request
// (free tier: 500 requests/month, up to 10 resources per request). It is a
// BulkFetcher: the per-platform adapters stay registered as gap-fillers for
// whatever Metrika cannot deliver (pending, stale, unavailable, or rejected).
type Metrika struct {
	APIKey  string
	Targets []MetrikaTarget

	// BaseURL is overridable for tests.
	BaseURL string
	client  httpClient

	mu sync.Mutex
	// invalid holds identifiers Metrika rejected with 422 so one bad entry
	// cannot blind every platform on every subsequent refresh.
	invalid map[string]bool
}

var _ BulkFetcher = (*Metrika)(nil)

func (f *Metrika) Name() string { return "metrika" }

// FetchAll returns one entry per platform Metrika could resolve. Platforms
// that came back pending/stale/unavailable are logged and omitted, so the
// aggregator can gap-fill them from another source.
func (f *Metrika) FetchAll(ctx context.Context) (map[string]int, error) {
	if f.APIKey == "" {
		return nil, errMissingConfig("metrika_api_key")
	}
	targets := f.activeTargets()
	if len(targets) == 0 {
		return nil, fmt.Errorf("metrika: no resources configured")
	}

	data, err := f.search(ctx, targets)
	if errors.Is(err, errMetrikaValidation) {
		slog.Warn("metrika rejected the batch; isolating identifiers", "error", err)
		data, err = f.searchIndividually(ctx, targets)
	}
	if err != nil {
		return nil, err
	}
	return summarizeMetrika(targets, data), nil
}

// search sends targets in batches of metrikaMaxBatch.
func (f *Metrika) search(ctx context.Context, targets []MetrikaTarget) ([]metrikaResult, error) {
	out := make([]metrikaResult, 0, len(targets))
	for start := 0; start < len(targets); start += metrikaMaxBatch {
		end := min(start+metrikaMaxBatch, len(targets))
		data, err := f.post(ctx, targets[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, data...)
	}
	return out, nil
}

// searchIndividually re-runs a rejected batch one resource at a time so a
// single malformed identifier only costs its own platform. Offenders are
// quarantined until restart, keeping later cycles at one request.
func (f *Metrika) searchIndividually(ctx context.Context, targets []MetrikaTarget) ([]metrikaResult, error) {
	out := make([]metrikaResult, 0, len(targets))
	var firstErr error
	for _, target := range targets {
		data, err := f.search(ctx, []MetrikaTarget{target})
		if errors.Is(err, errMetrikaValidation) {
			f.quarantine(target)
			slog.Error("metrika permanently rejected an identifier; ignoring it until restart",
				"platform", target.Platform, "identifier", target.Identifier, "error", err)
			continue
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, data...)
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func (f *Metrika) quarantine(target MetrikaTarget) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.invalid == nil {
		f.invalid = make(map[string]bool)
	}
	f.invalid[metrikaTargetKey(target)] = true
}

func (f *Metrika) activeTargets() []MetrikaTarget {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]MetrikaTarget, 0, len(f.Targets))
	for _, target := range f.Targets {
		if f.invalid[metrikaTargetKey(target)] {
			continue
		}
		out = append(out, target)
	}
	return out
}

func metrikaTargetKey(target MetrikaTarget) string {
	return target.Platform + "/" + target.Type + "/" + target.Identifier
}

// post performs one search request. Network and API errors are returned as
// errors; per-resource statuses are returned as data for the caller to judge.
func (f *Metrika) post(ctx context.Context, targets []MetrikaTarget) ([]metrikaResult, error) {
	base := strings.TrimSuffix(f.BaseURL, "/")
	if base == "" {
		base = metrikaDefaultBaseURL
	}
	reqURL := base + "/api/v1/resources/search"

	body, err := json.Marshal(struct {
		Resources []MetrikaTarget `json:"resources"`
	}{Resources: targets})
	if err != nil {
		return nil, fmt.Errorf("metrika: encode request: %w", err)
	}

	req, err := newRequest(ctx, http.MethodPost, reqURL, bytes.NewReader(body), map[string]string{
		"Authorization": "Bearer " + f.APIKey,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	})
	if err != nil {
		return nil, err
	}

	client := f.client
	if client == nil {
		client = defaultHTTPClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("metrika: POST %s failed: %w", reqURL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("metrika: read response: %w", err)
	}
	var payload metrikaResponse
	// Error responses are JSON too; decode best-effort so their messages
	// make it into the logs.
	_ = json.Unmarshal(raw, &payload)

	switch resp.StatusCode {
	case http.StatusOK:
		return payload.Data, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("metrika: invalid API token (401)")
	case http.StatusForbidden:
		// Free tier is 500 requests/month; the message says when it resets.
		return nil, fmt.Errorf("metrika: monthly quota exhausted (403): %s", metrikaErrorDetail(payload, raw))
	case http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("%w: %s", errMetrikaValidation, metrikaErrorDetail(payload, raw))
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("metrika: rate limited (429): %s", metrikaErrorDetail(payload, raw))
	default:
		return nil, fmt.Errorf("metrika: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
}

func metrikaErrorDetail(payload metrikaResponse, raw []byte) string {
	if len(payload.Errors) > 0 {
		return strings.Join(payload.Errors, "; ")
	}
	if payload.Error != "" {
		return payload.Error
	}
	return truncate(string(raw), 256)
}

type metrikaResponse struct {
	Data   []metrikaResult `json:"data"`
	Errors []string        `json:"errors"`
	Error  string          `json:"error"`
}

type metrikaResult struct {
	Platform   string                  `json:"platform"`
	Type       string                  `json:"type"`
	Identifier string                  `json:"identifier"`
	Status     string                  `json:"status"`
	Metrics    map[string]metrikaValue `json:"metrics"`
}

type metrikaValue struct {
	Value       *int   `json:"value"`
	Status      string `json:"status"`
	Semantic    string `json:"semantic"`
	CollectedAt string `json:"collected_at"`
}

// summarizeMetrika turns per-resource results into per-platform counts.
// Resources sharing a ChartKey are summed; if any one of them lacks a value
// the whole key is skipped (all-or-nothing).
func summarizeMetrika(targets []MetrikaTarget, data []metrikaResult) map[string]int {
	values := make(map[string]*int, len(data))
	statuses := make(map[string]string, len(data))
	for _, result := range data {
		key := result.Platform + "/" + result.Type + "/" + result.Identifier
		statuses[key] = result.Status

		metric, ok := result.Metrics["followers"]
		if !ok || metric.Value == nil {
			continue
		}
		if metric.Status != "" && metric.Status != "available" {
			continue
		}
		values[key] = metric.Value
		slog.Debug("metrika datapoint", "resource", key, "followers", *metric.Value,
			"semantic", metric.Semantic, "collected_at", metric.CollectedAt, "status", result.Status)
	}

	groups := make(map[string][]MetrikaTarget, len(targets))
	order := make([]string, 0, len(targets))
	for _, target := range targets {
		if _, seen := groups[target.ChartKey]; !seen {
			order = append(order, target.ChartKey)
		}
		groups[target.ChartKey] = append(groups[target.ChartKey], target)
	}

	counts := make(map[string]int, len(order))
	for _, chartKey := range order {
		total := 0
		missing := ""
		for _, target := range groups[chartKey] {
			key := metrikaTargetKey(target)
			value, ok := values[key]
			if !ok {
				status := statuses[key]
				switch status {
				case "":
					status = "not in response"
				case "available":
					// Resource exists but carries no usable followers datapoint.
					status = "no followers datapoint"
				}
				missing = fmt.Sprintf("%s (%s)", target.Identifier, status)
				break
			}
			total += *value
		}
		if missing != "" {
			slog.Warn("metrika: platform unavailable, keeping last-known-good",
				"platform", chartKey, "resource", missing)
			continue
		}
		counts[chartKey] = total
	}
	return counts
}
