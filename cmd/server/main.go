package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/OSAlt/gb-www-nixie/internal/adapters/contact"
	db_adapter "github.com/OSAlt/gb-www-nixie/internal/adapters/db"
	http_adapter "github.com/OSAlt/gb-www-nixie/internal/adapters/http"
	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/jackc/pgx/v5"
)

// mockMediaService is still needed as no real media adapter exists yet
type mockMediaService struct{}

func (s *mockMediaService) GetRecentPosts(ctx context.Context, count int) ([]domain.MediaPost, error) {
	return []domain.MediaPost{}, nil
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Wiring
	mediaSvc := &mockMediaService{}

	// Use real DB if DATABASE_URL is provided, otherwise mock
	var dbSvc ports.DBService
	if cfg.DatabaseURL != "" {
		// Run migrations
		migrationsDir := os.Getenv("MIGRATIONS_DIR")
		if migrationsDir == "" {
			migrationsDir = "db/migrations"
		}
		if err := db_adapter.RunMigrations(cfg.DatabaseURL, migrationsDir); err != nil {
			fmt.Printf("Warning: Failed to run migrations: %v\n", err)
		}

		conn, err := pgx.Connect(context.Background(), cfg.DatabaseURL)
		if err != nil {
			fmt.Printf("Warning: Unable to connect to database: %v. Using mock DB.\n", err)
		} else {
			dbSvc = db_adapter.NewAdapter(conn)
		}
	}
	contactSvc := contact.NewService(cfg, dbSvc)

	h := http_adapter.NewHandler(mediaSvc, contactSvc, dbSvc)

	// Routes
	mux := http.NewServeMux()

	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("GET /{$}", h.Index)

	// API Routes
	mux.HandleFunc("GET /api/v1/contact/list", h.APIListContacts)
	mux.HandleFunc("GET /api/v1/social/count/all", h.APISocialCountAll)
	mux.HandleFunc("GET /api/v1/social/INSTAGRAM/activity", h.APISocialInstagramActivity)

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
