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
		conn, err := pgx.Connect(context.Background(), cfg.DatabaseURL)
		if err != nil {
			fmt.Printf("Warning: Unable to connect to database: %v. Using mock DB.\n", err)
		} else {
			dbSvc = db_adapter.NewAdapter(conn)
		}
	}
	contactSvc := contact.NewService(cfg, dbSvc)

	h := http_adapter.NewHandler(mediaSvc, contactSvc, dbSvc)

	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Routes
	http.HandleFunc("/", h.Index)

	// API Routes
	http.HandleFunc("/api/v1/contact/list", h.APIListContacts)
	http.HandleFunc("/api/v1/social/count/all", h.APISocialCountAll)
	http.HandleFunc("/api/v1/social/INSTAGRAM/activity", h.APISocialInstagramActivity)

	fmt.Printf("Server starting on http://localhost:%s\n", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		os.Exit(1)
	}
}
