package social

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	twitchAPIBase     = "https://api.twitch.tv"
	twitchIDBase      = "https://id.twitch.tv"
	twitchTokenJitter = 5 * time.Minute
)

// Twitch fetches the channel's follower count via the official, free Twitch
// Helix API using an app access token. Only the total follower count is
// returned with app tokens, which is all the llama chart needs.
type Twitch struct {
	ClientID     string
	ClientSecret string
	// Login is the broadcaster's channel login, e.g. "nixiepixel".
	Login string

	// BaseURL and IDBaseURL are overridable for tests.
	BaseURL   string
	IDBaseURL string

	client httpClient

	token        string
	tokenExpires time.Time
	// broadcasterID is resolved once via /helix/users and cached.
	broadcasterID string
}

func (f *Twitch) Key() string { return "twitch" }

// ensureToken performs the client-credentials flow, caching the app access
// token (refreshed slightly before expiry).
func (f *Twitch) ensureToken(ctx context.Context) error {
	if f.token != "" && time.Now().Before(f.tokenExpires) {
		return nil
	}

	idBase := f.IDBaseURL
	if idBase == "" {
		idBase = twitchIDBase
	}

	reqURL := fmt.Sprintf("%s/oauth2/token", idBase)
	form := url.Values{
		"client_id":     {f.ClientID},
		"client_secret": {f.ClientSecret},
		"grant_type":    {"client_credentials"},
	}

	req, err := newRequest(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := decodeBody(resp, &payload); err != nil {
		return fmt.Errorf("token error: %w", err)
	}
	if payload.AccessToken == "" {
		return fmt.Errorf("token error: empty access_token (client credentials rejected?)")
	}

	f.token = payload.AccessToken
	// Refresh a little before the advertised expiry.
	f.tokenExpires = time.Now().Add(time.Duration(payload.ExpiresIn)*time.Second - twitchTokenJitter)
	if f.tokenExpires.Before(time.Now()) {
		f.tokenExpires = time.Now().Add(time.Minute)
	}
	return nil
}

// resolveBroadcasterID caches the channel's numeric ID via /helix/users.
func (f *Twitch) resolveBroadcasterID(ctx context.Context) error {
	if f.broadcasterID != "" {
		return nil
	}
	base := f.BaseURL
	if base == "" {
		base = twitchAPIBase
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	err := f.helixGet(ctx, fmt.Sprintf("%s/helix/users?login=%s", base, url.QueryEscape(f.Login)), &payload)
	if err != nil {
		return err
	}
	if len(payload.Data) == 0 {
		return fmt.Errorf("twitch user %q not found", f.Login)
	}
	f.broadcasterID = payload.Data[0].ID
	return nil
}

// helixGet issues an authenticated Helix GET, retrying once after refreshing
// the app token if the cached token has been revoked (HTTP 401).
func (f *Twitch) helixGet(ctx context.Context, reqURL string, out any) error {
	err := f.doHelixGet(ctx, reqURL, out)
	if err != nil && strings.Contains(err.Error(), "401") {
		if tokenErr := f.ensureToken(ctx); tokenErr != nil {
			return tokenErr
		}
		return f.doHelixGet(ctx, reqURL, out)
	}
	return err
}

func (f *Twitch) doHelixGet(ctx context.Context, reqURL string, out any) error {
	req, err := newRequest(ctx, http.MethodGet, reqURL, nil, map[string]string{
		"Client-Id":     f.ClientID,
		"Authorization": "Bearer " + f.token,
	})
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s failed: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		// Drop the cached token so the next ensureToken run re-auths.
		f.token = ""
		return fmt.Errorf("GET %s: HTTP 401 (app token rejected)", reqURL)
	}
	return decodeBody(resp, out)
}

func (f *Twitch) Fetch(ctx context.Context) (int, error) {
	if f.ClientID == "" || f.ClientSecret == "" {
		return 0, errMissingConfig("twitch_client_id twitch_client_secret")
	}

	if err := f.ensureToken(ctx); err != nil {
		return 0, err
	}
	if err := f.resolveBroadcasterID(ctx); err != nil {
		return 0, err
	}
	base := f.BaseURL
	if base == "" {
		base = twitchAPIBase
	}

	var payload struct {
		Total int `json:"total"`
	}
	if err := f.helixGet(ctx, fmt.Sprintf("%s/helix/channels/followers?broadcaster_id=%s", base, f.broadcasterID), &payload); err != nil {
		return 0, err
	}
	return payload.Total, nil
}
