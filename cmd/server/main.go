package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/adapters/blog"
	"github.com/OSAlt/gb-www-nixie/internal/adapters/contact"
	db_adapter "github.com/OSAlt/gb-www-nixie/internal/adapters/db"
	http_adapter "github.com/OSAlt/gb-www-nixie/internal/adapters/http"
	"github.com/OSAlt/gb-www-nixie/internal/adapters/media"
	"github.com/OSAlt/gb-www-nixie/internal/adapters/social"
	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/jobs"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"riverqueue.com/riverui"
)

const shutdownTimeout = 15 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("error loading config", "error", err)
		os.Exit(1)
	}

	// One signal-aware context for startup work; the HTTP server and River are
	// stopped explicitly below so in-flight work can drain.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wiring
	mediaSvc := media.NewService(cfg)

	// Use the real DB if DATABASE_URL is provided, otherwise no DB service is
	// wired at all.
	var (
		dbSvc ports.DBService
		uow   ports.UnitOfWork
		pool  *pgxpool.Pool
	)
	if cfg.DatabaseURL != "" {
		// Run migrations
		migrationsDir := os.Getenv("MIGRATIONS_DIR")
		if migrationsDir == "" {
			migrationsDir = "db/migrations"
		}
		if err := db_adapter.RunMigrations(cfg.DatabaseURL, migrationsDir); err != nil {
			slog.Warn("failed to run migrations", "error", err)
		}

		pool, err = pgxpool.New(ctx, cfg.DatabaseURL)
		if err == nil {
			// pgxpool connects lazily, so ping to preserve the old behaviour of
			// warning (and continuing without a DB) when the database is down.
			if pingErr := pool.Ping(ctx); pingErr != nil {
				err = pingErr
				pool.Close()
				pool = nil
			}
		}
		if err != nil {
			slog.Warn("unable to connect to database; continuing without one", "error", err)
			pool = nil
		} else {
			defer pool.Close()
			adapter := db_adapter.NewAdapter(pool)
			dbSvc = adapter
			uow = adapter
		}
	}
	// When a database is available, the feed services read the shared cache
	// written by the refresh jobs and never call upstream from a request.
	var contentStore ports.ContentCacheStore
	if store, ok := dbSvc.(ports.ContentCacheStore); ok {
		contentStore = store
		mediaSvc.WithStore(store)
	}

	contactSvc := contact.NewService(cfg).WithStore(contentStore).WithLister(dbSvc)
	blogSvc := blog.NewService(cfg).WithStore(contentStore)

	// Social follower-count refresh (see docs/social_migration.md). The
	// aggregator is built here and handed to the River worker.
	var socialFetcher *social.Aggregator
	if dbSvc != nil {
		if fetcher, ok := newSocialAggregator(cfg); ok {
			socialFetcher = fetcher
		} else {
			slog.Info("no social sources configured; social refresh disabled")
		}
	}

	// Background jobs. River needs its own schema and at least one worker; the
	// media/blog/contact refresh workers are always registered, so a database is
	// enough.
	var riverClient *river.Client[pgx.Tx]
	if pool != nil {
		deps := jobs.Deps{
			Config:           cfg,
			DB:               dbSvc,
			UnitOfWork:       uow,
			SocialFetcher:    socialFetcher,
			MediaRefresher:   mediaSvc,
			BlogRefresher:    blogSvc,
			ContactRefresher: contactSvc,
		}
		if err := db_adapter.MigrateRiver(ctx, pool); err != nil {
			slog.Error("river migrations failed; background jobs disabled", "error", err)
		} else if riverClient, err = startRiver(ctx, pool, deps); err != nil {
			slog.Error("river client failed to start; background jobs disabled", "error", err)
			riverClient = nil
		}
	}

	h := http_adapter.NewHandler(mediaSvc, contactSvc, dbSvc, blogSvc, cfg)

	// Routes
	mux := http.NewServeMux()

	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Embedded River UI (optional, requires DB + credentials).
	if riverClient != nil && cfg.RiverUIUser != "" && cfg.RiverUIPass != "" {
		uiHandler, err := riverui.NewHandler(&riverui.HandlerOpts{
			Endpoints: riverui.NewEndpoints(riverClient, nil),
			Logger:    slog.Default(),
			Prefix:    "/ui/river",
		})
		if err != nil {
			slog.Error("river ui handler failed", "error", err)
		} else if err := uiHandler.Start(ctx); err != nil {
			slog.Error("river ui start failed", "error", err)
		} else {
			auth := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					user, pass, ok := r.BasicAuth()
					if !ok ||
						subtle.ConstantTimeCompare([]byte(user), []byte(cfg.RiverUIUser)) != 1 ||
						subtle.ConstantTimeCompare([]byte(pass), []byte(cfg.RiverUIPass)) != 1 {
						w.Header().Set("WWW-Authenticate", `Basic realm="River UI"`)
						http.Error(w, "Unauthorized", http.StatusUnauthorized)
						return
					}
					next.ServeHTTP(w, r)
				})
			}
			mux.Handle("/ui/river/", auth(uiHandler))
			slog.Info("river ui enabled", "path", "/ui/river/")
		}
	} else if riverClient != nil {
		slog.Info("river ui disabled: set river_ui_user and river_ui_pass to enable")
	}

	mux.HandleFunc("GET /{$}", h.Index)
	mux.HandleFunc("GET /blog", h.Blog)
	mux.HandleFunc("GET /faq", h.FAQ)
	mux.HandleFunc("GET /portfolio", h.Portfolio)
	mux.HandleFunc("GET /contact", h.Contact)
	mux.HandleFunc("GET /social", h.Social)

	// API Routes
	mux.HandleFunc("GET /api/v1/social/count/all", h.APISocialCountAll)
	mux.HandleFunc("GET /api/v1/social/health", h.APISocialHealth)
	mux.HandleFunc("GET /api/v1/content/health", h.APIContentHealth)

	// Catch-all for other routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	if _, err := os.Stat("static"); err != nil {
		slog.Warn("static directory not found; run the server from the repo root", "error", err)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           withCacheHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("server starting", "url", "http://localhost:"+cfg.Port)
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()

	var runErr error
	select {
	case runErr = <-serveErr:
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("http server did not shut down cleanly", "error", err)
	}
	if riverClient != nil {
		if err := riverClient.Stop(shutdownCtx); err != nil {
			slog.Warn("river client did not stop cleanly", "error", err)
		}
	}

	if runErr != nil && !errors.Is(runErr, http.ErrServerClosed) {
		slog.Error("server stopped with an error", "error", runErr)
		os.Exit(1)
	}
	slog.Info("shutdown complete")
}

// withCacheHeaders lets browsers and CDNs absorb repeat page views. JSON APIs
// are left uncached because they expose live follower data.
func withCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isRead := r.Method == http.MethodGet || r.Method == http.MethodHead
		if isRead &&
			!strings.HasPrefix(r.URL.Path, "/api/") &&
			!strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
		}
		next.ServeHTTP(w, r)
	})
}

// startRiver builds and starts the River client. Start is given a context that
// does not cancel on SIGTERM: the client is stopped explicitly in main so
// running jobs can finish (River's Stop waits for them).
func startRiver(ctx context.Context, pool *pgxpool.Pool, deps jobs.Deps) (*river.Client[pgx.Tx], error) {
	workers, err := jobs.NewWorkers(deps)
	if err != nil {
		return nil, err
	}

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema:       db_adapter.RiverSchema,
		Workers:      workers,
		PeriodicJobs: jobs.PeriodicJobs(deps),
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: jobs.DefaultQueueMaxWorkers},
		},
	})
	if err != nil {
		return nil, err
	}

	if err := client.Start(context.WithoutCancel(ctx)); err != nil {
		return nil, err
	}

	slog.Info("background jobs started", "schema", db_adapter.RiverSchema)
	return client, nil
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
