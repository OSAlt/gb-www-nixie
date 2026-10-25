package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/OSAlt/gb-www-nixie/internal/mocks"
	"github.com/stretchr/testify/mock"
)

func serve(t *testing.T, handler http.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func blogPosts() []domain.BlogPost {
	return []domain.BlogPost{
		{Title: "Linux Adventures", Description: "All about Linux", URL: "/1", Categories: []string{"Linux"}},
		{Title: "Gaming Rig", Description: "Building a PC", URL: "/2", Categories: []string{"Gaming"}},
		{Title: "Privacy Tools", Description: "Stay safe online", Content: "tor and vpn", URL: "/3", Categories: []string{"Privacy"}},
		{Title: "Open Source", Description: "Community", URL: "/4", Categories: []string{"Linux"}},
		{Title: "Reviews", Description: "Hardware", URL: "/5", Categories: []string{"Gaming"}},
		{Title: "Geeks Abroad", Description: "Travel show", URL: "/6", Categories: []string{"Travel"}},
		{Title: "Kernel News", Description: "What's new", URL: "/7", Categories: []string{"Linux"}},
	}
}

func TestIndex_Renders(t *testing.T) {
	media := mocks.NewMockMediaService(t)
	media.EXPECT().GetRecentPosts(mock.Anything, 6).Return([]domain.MediaPost{{ID: "1", Caption: "Hello"}}, nil)
	contact := mocks.NewMockContactService(t)
	contact.EXPECT().GetSubjects(mock.Anything).Return([]string{"Say Hello", "Business Opportunity"}, nil)

	h := NewHandler(media, contact, nil, nil, &config.Config{})

	rec := serve(t, h.Index, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Who I Am & What I Do") {
		t.Error("home page content missing")
	}
}

func TestIndex_FallsBackToDefaultSubjects(t *testing.T) {
	media := mocks.NewMockMediaService(t)
	media.EXPECT().GetRecentPosts(mock.Anything, 6).Return(nil, errors.New("media down"))
	contact := mocks.NewMockContactService(t)
	contact.EXPECT().GetSubjects(mock.Anything).Return(nil, errors.New("subjects down"))

	h := NewHandler(media, contact, nil, nil, &config.Config{})

	rec := serve(t, h.Index, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Who I Am & What I Do") {
		t.Error("home page should still render when subjects fail")
	}
}

func blogHandler(t *testing.T, posts []domain.BlogPost, err error) *Handler {
	t.Helper()
	svc := mocks.NewMockBlogService(t)
	svc.EXPECT().GetRecentPosts(mock.Anything).Return(posts, err)
	return NewHandler(nil, nil, nil, svc, &config.Config{})
}

func TestBlog_DefaultPage(t *testing.T) {
	h := blogHandler(t, blogPosts(), nil)

	rec := serve(t, h.Blog, "/blog")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// pageSize is 5, so the 7 posts span two pages.
	if !strings.Contains(body, "Linux Adventures") || !strings.Contains(body, "Reviews") {
		t.Error("expected posts from the first page")
	}
	if strings.Contains(body, "Geeks Abroad") {
		t.Error("sixth post must be on the second page")
	}
	if !strings.Contains(body, "Blog pagination") {
		t.Error("expected pagination for 7 posts")
	}
	if !strings.Contains(body, "Categories") {
		t.Error("expected the category sidebar")
	}
}

func TestBlog_PageTwo(t *testing.T) {
	h := blogHandler(t, blogPosts(), nil)

	rec := serve(t, h.Blog, "/blog?page=2")
	body := rec.Body.String()

	if !strings.Contains(body, "Geeks Abroad") || !strings.Contains(body, "Kernel News") {
		t.Error("expected posts six and seven on page two")
	}
	if strings.Contains(body, "Linux Adventures") {
		t.Error("first page posts must not render on page two")
	}
	if !strings.Contains(body, `page-item active`) {
		t.Error("expected page two to be active")
	}
}

func TestBlog_PageClampedAndInvalid(t *testing.T) {
	h := blogHandler(t, blogPosts(), nil)

	// A page past the end is clamped to the last page.
	rec := serve(t, h.Blog, "/blog?page=999")
	if rec.Code != http.StatusOK {
		t.Fatalf("page=999: got status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Geeks Abroad") {
		t.Error("page=999: expected the page to be clamped to the last page")
	}

	// Invalid or non-positive pages fall back to the first page.
	for _, target := range []string{"/blog?page=abc", "/blog?page=0", "/blog?page=-3"} {
		rec := serve(t, h.Blog, target)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got status %d", target, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Linux Adventures") {
			t.Errorf("%s: expected the first page", target)
		}
	}
}

func TestBlog_Search(t *testing.T) {
	h := blogHandler(t, blogPosts(), nil)

	tests := []struct {
		query string
		want  string
		not   string
	}{
		{query: "kernel", want: "Kernel News", not: "Gaming Rig"},     // title
		{query: "privacy", want: "Privacy Tools", not: "Kernel News"}, // title + category
		{query: "building", want: "Gaming Rig", not: "Kernel News"},   // description
		{query: "tor", want: "Privacy Tools", not: "Gaming Rig"},      // content
		{query: "travel", want: "Geeks Abroad", not: "Privacy Tools"}, // category
		{query: "no-such-thing", want: "No blog posts found", not: "Linux Adventures"},
	}

	for _, tc := range tests {
		rec := serve(t, h.Blog, "/blog?q="+tc.query)
		body := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Fatalf("%q: got status %d", tc.query, rec.Code)
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("%q: expected %q in the body", tc.query, tc.want)
		}
		if strings.Contains(body, tc.not) {
			t.Errorf("%q: did not expect %q in the body", tc.query, tc.not)
		}
	}
}

func TestBlog_FeedError(t *testing.T) {
	h := blogHandler(t, nil, errors.New("feed down"))

	rec := serve(t, h.Blog, "/blog")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No blog posts found") {
		t.Error("expected the empty state when the feed fails")
	}
}

func TestFAQAndPortfolio(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, &config.Config{})

	if rec := serve(t, h.FAQ, "/faq"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Frequently Asked Questions") {
		t.Errorf("faq: status %d", rec.Code)
	}
	if rec := serve(t, h.Portfolio, "/portfolio"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Trusted By Industry Leaders") {
		t.Errorf("portfolio: status %d", rec.Code)
	}
}

func TestContact_RendersNewsletters(t *testing.T) {
	contact := mocks.NewMockContactService(t)
	contact.EXPECT().GetSubjects(mock.Anything).Return([]string{"Say Hello"}, nil)
	h := NewHandler(nil, contact, nil, nil, &config.Config{
		Newsletters: []config.Newsletter{
			{Name: "GeekBeacon", Description: "community", URL: "https://geekbeacon.org/newsletter"},
		},
	})

	rec := serve(t, h.Contact, "/contact")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "GeekBeacon") || !strings.Contains(body, "Say Hello") {
		t.Error("expected newsletters and subjects on the contact page")
	}
}

func TestContact_FallsBackToDefaultSubjects(t *testing.T) {
	contact := mocks.NewMockContactService(t)
	contact.EXPECT().GetSubjects(mock.Anything).Return(nil, nil)
	h := NewHandler(nil, contact, nil, nil, &config.Config{})

	rec := serve(t, h.Contact, "/contact")
	if !strings.Contains(rec.Body.String(), "Content Collaboration") {
		t.Error("expected the fallback subjects")
	}
}

func TestAPIListContacts(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().ListContacts(mock.Anything, "geekbeacon.org").
		Return([]domain.ContactMessage{{Email: "a@b", Subject: "Hi"}}, nil)
	h := NewHandler(nil, nil, db, nil, &config.Config{})

	rec := serve(t, h.APIListContacts, "/api/v1/contact/list?domain=geekbeacon.org")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("got content type %q", ct)
	}
	var got []domain.ContactMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got) != 1 || got[0].Email != "a@b" {
		t.Errorf("unexpected contacts: %+v", got)
	}
}

func TestAPIListContacts_Error(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().ListContacts(mock.Anything, "").Return(nil, errors.New("db down"))
	h := NewHandler(nil, nil, db, nil, &config.Config{})

	rec := serve(t, h.APIListContacts, "/api/v1/contact/list")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want 500", rec.Code)
	}
}

func TestAPISocialCountAll(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().GetSocialCounts(mock.Anything).Return(map[string]int{"youtube": 304000, "discord": 5425}, nil)
	h := NewHandler(nil, nil, db, nil, &config.Config{})

	rec := serve(t, h.APISocialCountAll, "/api/v1/social/count/all")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	var got map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got["youtube"] != 304000 || got["discord"] != 5425 {
		t.Errorf("unexpected counts: %v", got)
	}
}

func TestAPISocialCountAll_Error(t *testing.T) {
	db := mocks.NewMockDBService(t)
	db.EXPECT().GetSocialCounts(mock.Anything).Return(nil, errors.New("db down"))
	h := NewHandler(nil, nil, db, nil, &config.Config{})

	rec := serve(t, h.APISocialCountAll, "/api/v1/social/count/all")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want 500", rec.Code)
	}
}
