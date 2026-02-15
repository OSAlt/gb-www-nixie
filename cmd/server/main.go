package main

import (
    "context"
    "fmt"
    "net/http"
    "os"

    http_adapter "github.com/osalt/nixiesite/internal/adapters/http"
    "github.com/osalt/nixiesite/internal/domain"
)

// Mock services for now
type mockMediaService struct{}

func (s *mockMediaService) GetRecentPosts(ctx context.Context, count int) ([]domain.MediaPost, error) {
    return []domain.MediaPost{}, nil
}

type mockContactService struct{}

func (s *mockContactService) SendMessage(ctx context.Context, msg domain.ContactMessage) error {
    return nil
}
func (s *mockContactService) GetSubjects(ctx context.Context) ([]string, error) {
    return []string{"Say Hello", "Speaking Engagements", "Business Opportunity", "Content Collaboration"}, nil
}

func main() {
    // Wiring
    mediaSvc := &mockMediaService{}
    contactSvc := &mockContactService{}
    h := http_adapter.NewHandler(mediaSvc, contactSvc)

    // Serve static files
    fs := http.FileServer(http.Dir("static"))
    http.Handle("/static/", http.StripPrefix("/static/", fs))

    // Routes
    http.HandleFunc("/", h.Index)
    http.HandleFunc("/contact", h.Contact)

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    fmt.Printf("Server starting on http://localhost:%s\n", port)
    if err := http.ListenAndServe(":"+port, nil); err != nil {
        fmt.Printf("Error starting server: %v\n", err)
        os.Exit(1)
    }
}
