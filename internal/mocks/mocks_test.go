package mocks

import (
	"context"
	"testing"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/stretchr/testify/mock"
)

// These tests exercise the generated mocks themselves. They keep the
// generated code honest (a broken regeneration fails here) and give the
// package real statements under `go test -cover ./...`.

func TestMockMediaService(t *testing.T) {
	m := NewMockMediaService(t)
	ctx := context.Background()

	m.EXPECT().GetRecentPosts(mock.Anything, 6).
		Return([]domain.MediaPost{{ID: "1"}}, nil).Once()
	posts, err := m.GetRecentPosts(ctx, 6)
	if err != nil || len(posts) != 1 {
		t.Errorf("direct return: posts=%v err=%v", posts, err)
	}

	m.EXPECT().GetRecentPosts(mock.Anything, 2).
		RunAndReturn(func(context.Context, int) ([]domain.MediaPost, error) {
			return []domain.MediaPost{{ID: "2"}}, nil
		}).Once()
	posts, err = m.GetRecentPosts(ctx, 2)
	if err != nil || len(posts) != 1 || posts[0].ID != "2" {
		t.Errorf("function return: posts=%v err=%v", posts, err)
	}
}

func TestMockContactService(t *testing.T) {
	m := NewMockContactService(t)
	ctx := context.Background()

	m.EXPECT().GetSubjects(mock.Anything).Return([]string{"Say Hello"}, nil).Once()
	subjects, err := m.GetSubjects(ctx)
	if err != nil || len(subjects) != 1 {
		t.Errorf("direct return: subjects=%v err=%v", subjects, err)
	}

	m.EXPECT().GetSubjects(mock.Anything).
		RunAndReturn(func(context.Context) ([]string, error) { return nil, nil }).Once()
	if subjects, err = m.GetSubjects(ctx); err != nil || subjects != nil {
		t.Errorf("function return: subjects=%v err=%v", subjects, err)
	}
}

func TestMockBlogService(t *testing.T) {
	m := NewMockBlogService(t)
	ctx := context.Background()

	m.EXPECT().GetRecentPosts(mock.Anything).
		Return([]domain.BlogPost{{Title: "Hi"}}, nil).Once()
	posts, err := m.GetRecentPosts(ctx)
	if err != nil || len(posts) != 1 {
		t.Errorf("direct return: posts=%v err=%v", posts, err)
	}

	m.EXPECT().GetRecentPosts(mock.Anything).
		RunAndReturn(func(context.Context) ([]domain.BlogPost, error) { return nil, nil }).Once()
	if posts, err = m.GetRecentPosts(ctx); err != nil || posts != nil {
		t.Errorf("function return: posts=%v err=%v", posts, err)
	}
}

func TestMockDBService(t *testing.T) {
	m := NewMockDBService(t)
	ctx := context.Background()
	now := time.Now()

	m.EXPECT().ListContacts(mock.Anything, "nixiepixel.com").
		Return([]domain.ContactMessage{{Email: "a@b"}}, nil).Once()
	if contacts, err := m.ListContacts(ctx, "nixiepixel.com"); err != nil || len(contacts) != 1 {
		t.Errorf("ListContacts: contacts=%v err=%v", contacts, err)
	}
	m.EXPECT().ListContacts(mock.Anything, "func").
		RunAndReturn(func(context.Context, string) ([]domain.ContactMessage, error) { return nil, nil }).Once()
	if _, err := m.ListContacts(ctx, "func"); err != nil {
		t.Errorf("ListContacts (func): %v", err)
	}

	m.EXPECT().GetSocialCounts(mock.Anything).Return(map[string]int{"youtube": 1}, nil).Once()
	if counts, err := m.GetSocialCounts(ctx); err != nil || counts["youtube"] != 1 {
		t.Errorf("GetSocialCounts: counts=%v err=%v", counts, err)
	}
	m.EXPECT().GetSocialCounts(mock.Anything).
		RunAndReturn(func(context.Context) (map[string]int, error) { return nil, nil }).Once()
	if _, err := m.GetSocialCounts(ctx); err != nil {
		t.Errorf("GetSocialCounts (func): %v", err)
	}

	m.EXPECT().GetSocialMetricStates(mock.Anything).
		Return([]domain.SocialMetricState{{Platform: "youtube", Followers: 1, RefreshedAt: now}}, nil).Once()
	if states, err := m.GetSocialMetricStates(ctx); err != nil || len(states) != 1 {
		t.Errorf("GetSocialMetricStates: states=%v err=%v", states, err)
	}
	m.EXPECT().GetSocialMetricStates(mock.Anything).
		RunAndReturn(func(context.Context) ([]domain.SocialMetricState, error) { return nil, nil }).Once()
	if _, err := m.GetSocialMetricStates(ctx); err != nil {
		t.Errorf("GetSocialMetricStates (func): %v", err)
	}

	m.EXPECT().UpsertSocialCount(mock.Anything, "youtube", 1).Return(nil).Once()
	if err := m.UpsertSocialCount(ctx, "youtube", 1); err != nil {
		t.Errorf("UpsertSocialCount: %v", err)
	}
	m.EXPECT().UpsertSocialCount(mock.Anything, "func", 2).
		RunAndReturn(func(context.Context, string, int) error { return nil }).Once()
	if err := m.UpsertSocialCount(ctx, "func", 2); err != nil {
		t.Errorf("UpsertSocialCount (func): %v", err)
	}

	m.EXPECT().LastSocialRefresh(mock.Anything).Return(now, nil).Once()
	if got, err := m.LastSocialRefresh(ctx); err != nil || !got.Equal(now) {
		t.Errorf("LastSocialRefresh: got=%v err=%v", got, err)
	}
	m.EXPECT().LastSocialRefresh(mock.Anything).
		RunAndReturn(func(context.Context) (time.Time, error) { return now, nil }).Once()
	if _, err := m.LastSocialRefresh(ctx); err != nil {
		t.Errorf("LastSocialRefresh (func): %v", err)
	}
}

func TestMockFetcher(t *testing.T) {
	m := NewMockFetcher(t)
	ctx := context.Background()

	m.EXPECT().Key().Return("youtube").Once()
	if got := m.Key(); got != "youtube" {
		t.Errorf("Key = %q", got)
	}

	m.EXPECT().Fetch(mock.Anything).Return(304000, nil).Once()
	if count, err := m.Fetch(ctx); err != nil || count != 304000 {
		t.Errorf("Fetch: count=%d err=%v", count, err)
	}
	m.EXPECT().Fetch(mock.Anything).
		RunAndReturn(func(context.Context) (int, error) { return 7, nil }).Once()
	if count, err := m.Fetch(ctx); err != nil || count != 7 {
		t.Errorf("Fetch (func): count=%d err=%v", count, err)
	}
}

func TestMockBulkFetcher(t *testing.T) {
	m := NewMockBulkFetcher(t)
	ctx := context.Background()

	m.EXPECT().Name().Return("metrika").Once()
	if got := m.Name(); got != "metrika" {
		t.Errorf("Name = %q", got)
	}

	m.EXPECT().FetchAll(mock.Anything).Return(map[string]int{"youtube": 304000}, nil).Once()
	if counts, err := m.FetchAll(ctx); err != nil || counts["youtube"] != 304000 {
		t.Errorf("FetchAll: counts=%v err=%v", counts, err)
	}
	m.EXPECT().FetchAll(mock.Anything).
		RunAndReturn(func(context.Context) (map[string]int, error) { return nil, nil }).Once()
	if _, err := m.FetchAll(ctx); err != nil {
		t.Errorf("FetchAll (func): %v", err)
	}
}
