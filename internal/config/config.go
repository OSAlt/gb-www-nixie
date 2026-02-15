package config

import (
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
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

	return &cfg, nil
}
