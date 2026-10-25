package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/adapters/blog"
	"github.com/OSAlt/gb-www-nixie/internal/adapters/contact"
	db_adapter "github.com/OSAlt/gb-www-nixie/internal/adapters/db"
	http_adapter "github.com/OSAlt/gb-www-nixie/internal/adapters/http"
	"github.com/OSAlt/gb-www-nixie/internal/adapters/media"
	"github.com/OSAlt/gb-www-nixie/internal/adapters/social"
	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Wiring
	mediaSvc := media.NewService(cfg)

	// Use the real DB if DATABASE_URL is provided, otherwise no DB service is
	// wired at all.
	var (
		dbSvc ports.DBService
		uow   ports.UnitOfWork
	)
	if cfg.DatabaseURL != "" {
		// Run migrations
		migrationsDir := os.Getenv("MIGRATIONS_DIR")
		if migrationsDir == "" {
			migrationsDir = "db/migrations"
		}
		if err := db_adapter.RunMigrations(cfg.DatabaseURL, migrationsDir); err != nil {
			fmt.Printf("Warning: Failed to run migrations: %v\n", err)
		}

		pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err == nil {
			// pgxpool connects lazily, so ping to preserve the old behaviour of
			// warning (and continuing without a DB) when the database is down.
			if pingErr := pool.Ping(context.Background()); pingErr != nil {
				err = pingErr
				pool.Close()
			}
		}
		if err != nil {
			fmt.Printf("Warning: Unable to connect to database: %v. Using mock DB.\n", err)
		} else {
			defer pool.Close()
			adapter := db_adapter.NewAdapter(pool)
			dbSvc = adapter
			uow = adapter
		}
	}
	contactSvc := contact.NewService(cfg, dbSvc)
	blogSvc := blog.NewService(cfg)

	// Social follower-count refresh (see doc/social_migration.md)
	if dbSvc != nil {
		if fetcher, ok := newSocialAggregator(cfg); ok {
			go runSocialMetricsWorker(context.Background(), dbSvc, uow, fetcher, cfg)
		}
	}

	h := http_adapter.NewHandler(mediaSvc, contactSvc, dbSvc, blogSvc, cfg)

	// Routes
	mux := http.NewServeMux()

	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("GET /{$}", h.Index)
	mux.HandleFunc("GET /blog", h.Blog)
	mux.HandleFunc("GET /faq", h.FAQ)
	mux.HandleFunc("GET /portfolio", h.Portfolio)
	mux.HandleFunc("GET /contact", h.Contact)
	mux.HandleFunc("GET /social", h.Social)

	// API Routes
	mux.HandleFunc("GET /api/v1/contact/list", h.APIListContacts)
	mux.HandleFunc("GET /api/v1/social/count/all", h.APISocialCountAll)
	mux.HandleFunc("GET /api/v1/social/health", h.APISocialHealth)

	// Catch-all for other routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	fmt.Printf("Server starting on http://localhost:%s\n", cfg.Port)
	cwd, _ := os.Getwd()
	fmt.Printf("Current working directory: %s\n", cwd)
	if _, err := os.Stat("static"); err != nil {
		fmt.Printf("Error: static directory not found: %v\n", err)
	} else {
		fmt.Println("Static directory found")
		files, _ := os.ReadDir("static")
		fmt.Print("Contents of static: ")
		for _, f := range files {
			fmt.Printf("%s ", f.Name())
		}
		fmt.Println()
	}

	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		os.Exit(1)
	}
}

// splitNonEmpty splits a comma-separated config value, trimming blanks.
func splitNonEmpty(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// newSocialAggregator builds the follower-count sources from config.
// Metrika (one key, one batched request) is the primary source when
// METRIKA_API_KEY is set; the per-platform adapters stay registered as
// gap-fillers for whatever it cannot deliver. Returns ok=false when nothing
// is configured.
func newSocialAggregator(cfg *config.Config) (*social.Aggregator, bool) {
	var (
		bulk     []social.BulkFetcher
		fetchers []social.Fetcher
	)

	if cfg.MetrikaApiKey != "" {
		bulk = append(bulk, &social.Metrika{
			APIKey:  cfg.MetrikaApiKey,
			BaseURL: cfg.MetrikaBaseUrl,
			Targets: social.DefaultMetrikaTargets(cfg.MetrikaDiscordVanity),
		})
	}

	if cfg.YoutubeApiKey != "" {
		fetchers = append(fetchers, &social.YouTube{
			APIKey:   cfg.YoutubeApiKey,
			Channels: splitNonEmpty(cfg.YoutubeChannels),
		})
	}
	if cfg.TwitchClientId != "" && cfg.TwitchClientSecret != "" {
		fetchers = append(fetchers, &social.Twitch{
			ClientID:     cfg.TwitchClientId,
			ClientSecret: cfg.TwitchClientSecret,
			Login:        cfg.TwitchLogin,
		})
	}
	if cfg.DiscordBotToken != "" && cfg.DiscordGuildId != "" {
		fetchers = append(fetchers, &social.Discord{
			BotToken: cfg.DiscordBotToken,
			GuildID:  cfg.DiscordGuildId,
		})
	}
	if cfg.SocialScrapeURL != "" {
		scraper := &social.Scrape{
			BaseURL:  cfg.SocialScrapeURL,
			Profiles: social.DefaultProfiles(),
		}
		fetchers = append(fetchers, scraper.Fetchers()...)
	}

	if len(bulk) == 0 && len(fetchers) == 0 {
		return nil, false
	}
	return social.NewAggregatorWithBulk(bulk, fetchers...), true
}

// runSocialMetricsWorker supervises the refresh loop. If it ever exits or
// panics unexpectedly, it is restarted with exponential backoff instead of
// leaving the chart frozen until the next deploy.
func runSocialMetricsWorker(ctx context.Context, dbSvc ports.DBService, uow ports.UnitOfWork, fetcher *social.Aggregator, cfg *config.Config) {
	backoff := time.Minute
	for {
		err := runSocialMetricsLoop(ctx, dbSvc, uow, fetcher, cfg)
		if ctx.Err() != nil {
			// Shutting down; not a crash.
			return
		}

		slog.Error("social metrics worker stopped unexpectedly; restarting",
			"error", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Hour)
	}
}

// runSocialMetricsLoop refreshes follower counts once per interval, storing
// each successful result as the new last-known-good value. Per-platform
// failures are logged and leave the stored value untouched.
//
// The boot refresh is skipped when the stored data is newer than the interval,
// so restarts and deploys do not spend Metrika requests on data that only
// changes once a day.
func runSocialMetricsLoop(ctx context.Context, dbSvc ports.DBService, uow ports.UnitOfWork, fetcher *social.Aggregator, cfg *config.Config) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in social metrics worker", "panic", r, "stack", string(debug.Stack()))
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	interval := time.Duration(cfg.SocialRefreshSeconds) * time.Second
	fmt.Printf("social metrics worker started (interval: %s)\n", interval)

	if last, lastErr := dbSvc.LastSocialRefresh(ctx); lastErr != nil {
		slog.Warn("social metrics: cannot read last refresh time; refreshing now", "error", lastErr)
		refreshSocialMetrics(ctx, uow, fetcher)
	} else if age := time.Since(last); age < interval {
		fmt.Printf("social metrics: last refresh %s ago (< %s), skipping boot refresh\n",
			age.Round(time.Minute), interval)
	} else {
		refreshSocialMetrics(ctx, uow, fetcher)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			refreshSocialMetrics(ctx, uow, fetcher)
		}
	}
}

// refreshSocialMetrics performs one refresh cycle. All platform counts are
// written in a single transaction, so a database error leaves every stored
// value on the previous (last-known-good) refresh instead of a partial update.
// Panics are contained here as well as in the fetchers, so a single bad cycle
// cannot stop the ticker.
func refreshSocialMetrics(ctx context.Context, uow ports.UnitOfWork, fetcher *social.Aggregator) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic during social metrics refresh", "panic", r, "stack", string(debug.Stack()))
		}
	}()

	counts := fetcher.Fetch(ctx)
	if len(counts) == 0 {
		fmt.Printf("social metrics: no platform fetches succeeded; keeping last-known-good values\n")
		return
	}

	err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Transaction) error {
		for platform, followers := range counts {
			if err := tx.UpsertSocialCount(ctx, platform, followers); err != nil {
				return fmt.Errorf("upsert %s: %w", platform, err)
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("social metrics refresh failed; keeping last-known-good values", "error", err)
		return
	}

	for platform, followers := range counts {
		fmt.Printf("social metrics: %s = %d\n", platform, followers)
	}
}
