package config

import "testing"

func TestLoad_FromEnvironment(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("METRIKA_API_KEY", "metrika-key")
	t.Setenv("METRIKA_BASE_URL", "https://metrika.example")
	t.Setenv("METRIKA_DISCORD_VANITY", "custom-vanity")
	t.Setenv("YOUTUBE_API_KEY", "youtube-key")
	t.Setenv("YOUTUBE_CHANNELS", "chan-a,chan-b")
	t.Setenv("TWITCH_CLIENT_ID", "twitch-id")
	t.Setenv("TWITCH_CLIENT_SECRET", "twitch-secret")
	t.Setenv("TWITCH_LOGIN", "twitch-login")
	t.Setenv("DISCORD_BOT_TOKEN", "discord-token")
	t.Setenv("DISCORD_GUILD_ID", "discord-guild")
	t.Setenv("SOCIAL_SCRAPE_URL", "https://pulse.example")
	t.Setenv("SOCIAL_REFRESH_SECONDS", "120")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	checks := map[string]struct{ got, want string }{
		"Port":                 {cfg.Port, "9999"},
		"DatabaseURL":          {cfg.DatabaseURL, "postgres://user:pass@localhost:5432/db"},
		"MetrikaApiKey":        {cfg.MetrikaApiKey, "metrika-key"},
		"MetrikaBaseUrl":       {cfg.MetrikaBaseUrl, "https://metrika.example"},
		"MetrikaDiscordVanity": {cfg.MetrikaDiscordVanity, "custom-vanity"},
		"YoutubeApiKey":        {cfg.YoutubeApiKey, "youtube-key"},
		"YoutubeChannels":      {cfg.YoutubeChannels, "chan-a,chan-b"},
		"TwitchClientId":       {cfg.TwitchClientId, "twitch-id"},
		"TwitchClientSecret":   {cfg.TwitchClientSecret, "twitch-secret"},
		"TwitchLogin":          {cfg.TwitchLogin, "twitch-login"},
		"DiscordBotToken":      {cfg.DiscordBotToken, "discord-token"},
		"DiscordGuildId":       {cfg.DiscordGuildId, "discord-guild"},
		"SocialScrapeURL":      {cfg.SocialScrapeURL, "https://pulse.example"},
	}
	for name, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", name, c.got, c.want)
		}
	}
	if cfg.SocialRefreshSeconds != 120 {
		t.Errorf("SocialRefreshSeconds = %d, want 120", cfg.SocialRefreshSeconds)
	}
}

func TestLoad_Defaults(t *testing.T) {
	// Explicitly blank the optional knobs so the defaults are exercised even
	// if the developer's shell happens to export them.
	for _, key := range []string{
		"PORT", "METRIKA_DISCORD_VANITY", "YOUTUBE_CHANNELS",
		"TWITCH_LOGIN", "SOCIAL_SCRAPE_URL", "SOCIAL_REFRESH_SECONDS",
	} {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.MetrikaDiscordVanity != "geekbeacon" {
		t.Errorf("MetrikaDiscordVanity = %q, want geekbeacon", cfg.MetrikaDiscordVanity)
	}
	if cfg.YoutubeChannels != "NixiePixelGB,user/nixiedoeslinux" {
		t.Errorf("YoutubeChannels = %q", cfg.YoutubeChannels)
	}
	if cfg.TwitchLogin != "nixiepixel" {
		t.Errorf("TwitchLogin = %q, want nixiepixel", cfg.TwitchLogin)
	}
	if cfg.SocialScrapeURL != "https://pulse.walls.sh" {
		t.Errorf("SocialScrapeURL = %q, want https://pulse.walls.sh", cfg.SocialScrapeURL)
	}
	if cfg.SocialRefreshSeconds != 86400 {
		t.Errorf("SocialRefreshSeconds = %d, want 86400", cfg.SocialRefreshSeconds)
	}
}

func TestLoad_NonPositiveRefreshFallsBackToDaily(t *testing.T) {
	t.Setenv("SOCIAL_REFRESH_SECONDS", "-5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SocialRefreshSeconds != 86400 {
		t.Errorf("SocialRefreshSeconds = %d, want 86400", cfg.SocialRefreshSeconds)
	}
}

func TestLoad_InvalidNumberFails(t *testing.T) {
	t.Setenv("SOCIAL_REFRESH_SECONDS", "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("expected an unmarshal error for a non-numeric refresh interval")
	}
}
