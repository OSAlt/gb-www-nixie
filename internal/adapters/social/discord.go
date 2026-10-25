package social

import (
	"context"
	"fmt"
	"net/url"
)

const discordAPIBase = "https://discord.com/api"

// Discord fetches the server's member count via the official bot REST API
// (no privileged intents needed). Discord has no "followers"; the llama
// chart's Discord slice uses member count.
type Discord struct {
	BotToken string
	GuildID  string

	// BaseURL is overridable for tests.
	BaseURL string
	client  httpClient
}

func (f *Discord) Key() string { return "discord" }

func (f *Discord) Fetch(ctx context.Context) (int, error) {
	if f.BotToken == "" || f.GuildID == "" {
		return 0, errMissingConfig("discord_bot_token discord_guild_id")
	}
	base := f.BaseURL
	if base == "" {
		base = discordAPIBase
	}

	reqURL := fmt.Sprintf("%s/guilds/%s?with_counts=true", base, url.PathEscape(f.GuildID))
	var payload struct {
		// approximate_member_count is the live member count.
		ApproximateMemberCount int `json:"approximate_member_count"`
	}
	if err := fetchJSON(ctx, f.client, reqURL, map[string]string{
		"Authorization": "Bot " + f.BotToken,
	}, &payload); err != nil {
		return 0, err
	}
	return payload.ApproximateMemberCount, nil
}
