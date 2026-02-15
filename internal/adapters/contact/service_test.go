package contact

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/osalt/nixiesite/internal/config"
)

func TestService_GetSubjects(t *testing.T) {
	// 1. Setup mock server
	mockItems := []contactItem{
		{Email: "test1@example.com", Description: "Subject 1"},
		{Email: "test2@example.com", Description: "Subject 2"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(mockItems)
	}))
	defer ts.Close()

	s := &Service{
		client: ts.Client(),
		url:    ts.URL,
		cfg:    &config.Config{},
	}

	subjects, err := s.GetSubjects(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(subjects) != 2 {
		t.Errorf("expected 2 subjects, got %d", len(subjects))
	}

	if subjects[0] != "Subject 1" || subjects[1] != "Subject 2" {
		t.Errorf("unexpected subjects: %v", subjects)
	}
}
