package config

import (
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Newsletter struct {
	Name        string `koanf:"name"`
	Description string `koanf:"description"`
	URL         string `koanf:"url"`
}

type Config struct {
	Port        string       `koanf:"port"`
	DatabaseURL string       `koanf:"database_url"`
	BlogURLs    []string     `koanf:"blog_urls"`
	Newsletters []Newsletter `koanf:"newsletters"`

	// Social follower-count metrics (see doc/social_migration.md).
	// Metrika consolidates every platform behind one free API key. When it is
	// configured it is the primary source; the per-platform adapters below
	// only fill gaps it cannot deliver.
	MetrikaApiKey        string `koanf:"metrika_api_key"`
	MetrikaBaseUrl       string `koanf:"metrika_base_url"`
	MetrikaDiscordVanity string `koanf:"metrika_discord_vanity"`

	// Per-platform fallbacks. Missing credentials simply skip that platform;
	// the worker keeps the last-known-good value in nixie.social_metrics.
	YoutubeApiKey        string `koanf:"youtube_api_key"`
	YoutubeChannels      string `koanf:"youtube_channels"`
	TwitchClientId       string `koanf:"twitch_client_id"`
	TwitchClientSecret   string `koanf:"twitch_client_secret"`
	TwitchLogin          string `koanf:"twitch_login"`
	DiscordBotToken      string `koanf:"discord_bot_token"`
	DiscordGuildId       string `koanf:"discord_guild_id"`
	SocialScrapeURL      string `koanf:"social_scrape_url"`
	SocialRefreshSeconds int    `koanf:"social_refresh_seconds"`
}

func Load() (*Config, error) {
	k := koanf.New(".")

	// Load .env if it exists (optional)
	_ = godotenv.Load()

	// Load default config.yaml
	if err := k.Load(file.Provider("config/config.yaml"), yaml.Parser()); err != nil {
		// It's okay if config.yaml is missing, we might rely solely on env vars
	}

	// Load environment variables with a prefix if needed, or just all
	// Here we use NIXIE_ prefix and replace __ with . for nested structures if we had any
	err := k.Load(env.Provider("", ".", func(s string) string {
		return strings.ToLower(s)
	}), nil)
	if err != nil {
		return nil, fmt.Errorf("error loading env: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Set defaults if not provided
	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	// Social metrics defaults
	if cfg.MetrikaDiscordVanity == "" {
		// discord.gg/geekbeacon (a vanity code, never a generated invite).
		cfg.MetrikaDiscordVanity = "geekbeacon"
	}
	if cfg.YoutubeChannels == "" {
		// Nixie's audience spans two channels; their counts are combined.
		cfg.YoutubeChannels = "NixiePixelGB,user/nixiedoeslinux"
	}
	if cfg.TwitchLogin == "" {
		cfg.TwitchLogin = "nixiepixel"
	}
	if cfg.SocialScrapeURL == "" {
		cfg.SocialScrapeURL = "https://pulse.walls.sh"
	}
	if cfg.SocialRefreshSeconds <= 0 {
		// Daily: Metrika only refreshes its own data once a day, so a faster
		// cadence buys nothing and burns the 500 request/month free quota.
		cfg.SocialRefreshSeconds = 86400
	}

	return &cfg, nil
}
