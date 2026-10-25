package config

import (
	"fmt"
	"os"
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
	Port          string       `koanf:"port"`
	DatabaseURL   string       `koanf:"database_url"`
	BlogURLs      []string     `koanf:"blog_urls"`
	MediaFeedURLs []string     `koanf:"media_feed_urls"`
	Newsletters   []Newsletter `koanf:"newsletters"`

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

	// Load the optional YAML config. config.yaml is the documented name, but
	// accept config.yml too: the checked-in example template uses .yml, so a
	// plain `cp config/config_example.yml config/config.yml` would otherwise be
	// silently ignored.
	for _, path := range []string{"config/config.yaml", "config/config.yml"} {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		// It's okay if the file cannot be parsed; env vars still apply.
		_ = k.Load(file.Provider(path), yaml.Parser())
		break
	}

	// Load environment variables with a prefix if needed, or just all
	// Here we use NIXIE_ prefix and replace __ with . for nested structures if we had any
	err := k.Load(env.Provider(".", env.Opt{
		TransformFunc: func(key, value string) (string, any) {
			return strings.ToLower(key), value
		},
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

	// Latest-video feeds for the home page: NixiePixel and NixieDoesLinux.
	// Override with media_feed_urls when the channels change.
	if len(cfg.MediaFeedURLs) == 0 {
		cfg.MediaFeedURLs = []string{
			"https://www.youtube.com/feeds/videos.xml?channel_id=UCMyAVYPgP_179gj9OIJZd4A",
			"https://www.youtube.com/feeds/videos.xml?channel_id=UCBE-FO9JUOghSysV9gjTeHw",
		}
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
