package contact

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
)

const defaultContactAPIURL = "https://social.geekbeacon.org/api/v1.0/contact/list"

type contactItem struct {
	Email       string `json:"email"`
	Description string `json:"description"`
}

type Service struct {
	client *http.Client
	url    string
	cfg    *config.Config
	db     ports.DBService
}

func NewService(cfg *config.Config, db ports.DBService) *Service {
	return &Service{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		url: defaultContactAPIURL,
		cfg: cfg,
		db:  db,
	}
}

func (s *Service) GetSubjects(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contact list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var items []contactItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	subjects := make([]string, 0, len(items))
	for _, item := range items {
		subjects = append(subjects, item.Description)
	}

	return subjects, nil
}
