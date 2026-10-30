package contact

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/stretchr/testify/mock"
)

func contactLister(t *testing.T, subjects ...string) ports.DBService {
	t.Helper()
	m := mocks.NewMockDBService(t)
	msgs := make([]domain.ContactMessage, 0, len(subjects))
	for _, s := range subjects {
		msgs = append(msgs, domain.ContactMessage{Subject: s})
	}
	m.EXPECT().ListContacts(mock.Anything, "nixiepixel.com").Return(msgs, nil)
	return m
}

func contactServiceFor(lister ports.DBService, store ports.ContentCacheStore) *Service {
	svc := NewService(&config.Config{ContactRefreshSeconds: 3600}).WithStore(store)
	if lister != nil {
		svc = svc.WithLister(lister)
	}
	return svc
}

func cachedSubjects(t *testing.T, subjects ...string) []byte {
	t.Helper()
	payload, err := json.Marshal(subjects)
	if err != nil {
		t.Fatalf("marshal subjects: %v", err)
	}
	return payload
}

func TestService_GetSubjects_UsesStore(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, ContactCacheKey).Return(ports.ContentCacheEntry{
		Payload:   cachedSubjects(t, "Say Hello", "Business Opportunity"),
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	// No lister: an empty result would mean the store was ignored.
	svc := contactServiceFor(nil, store)

	subjects, err := svc.GetSubjects(context.Background())
	if err != nil {
		t.Fatalf("GetSubjects: %v", err)
	}
	if len(subjects) != 2 || subjects[0] != "Say Hello" {
		t.Fatalf("subjects = %v, want the cached list", subjects)
	}
}

func TestService_GetSubjects_StoreMissFetchesAndSaves(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, ContactCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)
	store.EXPECT().SaveContentCache(mock.Anything, ContactCacheKey, mock.Anything, mock.Anything).Return(nil)

	svc := contactServiceFor(contactLister(t, "Say Hello", "Content Collaboration"), store)

	subjects, err := svc.GetSubjects(context.Background())
	if err != nil {
		t.Fatalf("GetSubjects: %v", err)
	}
	if len(subjects) != 2 || subjects[1] != "Content Collaboration" {
		t.Fatalf("subjects = %v", subjects)
	}
}

func TestService_GetSubjects_StoreErrorFallsBack(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, ContactCacheKey).Return(ports.ContentCacheEntry{}, errors.New("db down"))
	store.EXPECT().SaveContentCache(mock.Anything, ContactCacheKey, mock.Anything, mock.Anything).Return(nil)

	svc := contactServiceFor(contactLister(t, "Say Hello"), store)

	subjects, err := svc.GetSubjects(context.Background())
	if err != nil {
		t.Fatalf("GetSubjects: %v", err)
	}
	if len(subjects) != 1 {
		t.Fatalf("subjects = %v, want the local fallback", subjects)
	}
}

func TestService_Refresh_SavesToStore(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, ContactCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)
	store.EXPECT().SaveContentCache(mock.Anything, ContactCacheKey, mock.Anything, mock.Anything).Return(nil)

	svc := contactServiceFor(contactLister(t, "Say Hello", "Speaking Engagements"), store)

	count, err := svc.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if count != 2 {
		t.Errorf("Refresh count = %d, want 2", count)
	}
}

func TestService_Refresh_SkipsFreshCache(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, ContactCacheKey).Return(ports.ContentCacheEntry{
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	svc := contactServiceFor(nil, store)

	count, err := svc.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if count != 0 {
		t.Errorf("Refresh count = %d, want 0 (fresh cache must be a no-op)", count)
	}
}

func TestService_Refresh_EmptyUpstreamIsAnError(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, ContactCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)

	empty := mocks.NewMockDBService(t)
	empty.EXPECT().ListContacts(mock.Anything, "nixiepixel.com").Return([]domain.ContactMessage{}, nil)
	svc := contactServiceFor(empty, store)

	if _, err := svc.Refresh(context.Background()); err == nil {
		t.Fatal("expected an error for an empty subject list")
	}
}

func TestService_CacheState(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, ContactCacheKey).Return(ports.ContentCacheEntry{
		Payload:   cachedSubjects(t, "Say Hello"),
		FetchedAt: time.Now().Add(-time.Minute),
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	state, err := contactServiceFor(nil, store).CacheState(context.Background())
	if err != nil {
		t.Fatalf("CacheState: %v", err)
	}
	if state.Items != 1 || state.Missing || state.Stale {
		t.Errorf("state = %+v, want one fresh item", state)
	}
}

func TestService_CacheState_Missing(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, ContactCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)

	state, err := contactServiceFor(nil, store).CacheState(context.Background())
	if err != nil {
		t.Fatalf("CacheState: %v", err)
	}
	if !state.Missing {
		t.Errorf("state = %+v, want missing", state)
	}
}
