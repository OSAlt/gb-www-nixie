package contact

import (
	"context"
	"testing"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/stretchr/testify/mock"
)

func TestService_GetSubjects(t *testing.T) {
	lister := mocks.NewMockDBService(t)
	lister.EXPECT().ListContacts(mock.Anything, "nixiepixel.com").Return([]domain.ContactMessage{
		{Subject: "Subject 1"},
		{Subject: "Subject 2"},
	}, nil)

	s := NewService(&config.Config{}).WithLister(lister)

	subjects, err := s.GetSubjects(context.Background())
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
