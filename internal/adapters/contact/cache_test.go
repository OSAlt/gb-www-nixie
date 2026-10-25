package contact

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/stretchr/testify/mock"
)

func subjectsServer(t *testing.T, subjects ...string) *httptest.Server {
	t.Helper()
	items := make([]contactItem, 0, len(subjects))
	for i, subject := range subjects {
		items = append(items, contactItem{
			Email:       "user" + string(rune('a'+i)) + "@example.com",
			Description: subject,
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(items)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func contactServiceFor(srv *httptest.Server, store ports.ContentCacheStore) *Service {
	svc := NewService(&config.Config{ContactRefreshSeconds: 3600}).WithStore(store)
	if srv != nil {
		svc.url = srv.URL
		svc.client = srv.Client()
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

	// No upstream server: an empty result would mean the store was ignored.
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

	srv := subjectsServer(t, "Say Hello", "Content Collaboration")
	svc := contactServiceFor(srv, store)

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

	srv := subjectsServer(t, "Say Hello")
	svc := contactServiceFor(srv, store)

	subjects, err := svc.GetSubjects(context.Background())
	if err != nil {
		t.Fatalf("GetSubjects: %v", err)
	}
	if len(subjects) != 1 {
		t.Fatalf("subjects = %v, want the upstream fallback", subjects)
	}
}

func TestService_Refresh_SavesToStore(t *testing.T) {
	store := mocks.NewMockContentCacheStore(t)
	store.EXPECT().LoadContentCache(mock.Anything, ContactCacheKey).Return(ports.ContentCacheEntry{}, ports.ErrContentCacheMiss)
	store.EXPECT().SaveContentCache(mock.Anything, ContactCacheKey, mock.Anything, mock.Anything).Return(nil)

	srv := subjectsServer(t, "Say Hello", "Speaking Engagements")
	svc := contactServiceFor(srv, store)

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

	srv := subjectsServer(t)
	svc := contactServiceFor(srv, store)

	if _, err := svc.Refresh(context.Background()); err == nil {
		t.Fatal("expected an error for an empty upstream list")
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
