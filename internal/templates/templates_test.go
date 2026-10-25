package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/a-h/templ"
)

func render(t *testing.T, component templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestLayout(t *testing.T) {
	body := render(t, Layout("NixiePixel - Test"))
	for _, want := range []string{
		"<!doctype html>",
		"<title>NixiePixel - Test</title>",
		`href="/static/css/style.css?v=10"`,
		`href="/blog"`,
		`id="socialWeb"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing %q", want)
		}
	}

	// Render the header and social list directly too, so the nil-children
	// branch and the reusable components are exercised on their own.
	if body := render(t, Header()); !strings.Contains(body, `/contact`) {
		t.Error("header missing contact link")
	}
	if body := render(t, Social("customSocial")); !strings.Contains(body, `id="customSocial"`) {
		t.Error("social list did not use the supplied id")
	}
}

func TestIndex(t *testing.T) {
	body := render(t, Index(
		[]domain.MediaPost{{ID: "1", ImageURL: "/static/imgs/a.png", Caption: "Hi", URL: "/a"}},
		[]string{"Say Hello", "Business Opportunity"},
	))
	for _, want := range []string{
		"NixiePixel",
		`id="toTop"`,
		"Who I Am & What I Do",
		"about-card-purple",
		"https://geekbeacon.org/",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestBlog_Empty(t *testing.T) {
	body := render(t, Blog(nil, nil, "", 1, 0))

	if !strings.Contains(body, "No blog posts found matching your criteria.") {
		t.Error("expected the empty-state alert")
	}
	if strings.Contains(body, "Search results for") {
		t.Error("search header must not render without a query")
	}
	if strings.Contains(body, "Blog pagination") {
		t.Error("pagination must not render for a single page")
	}
	if strings.Contains(body, ">Categories<") {
		t.Error("category card must not render when there are no categories")
	}
}

func TestBlog_EmptyWithSearch(t *testing.T) {
	body := render(t, Blog(nil, nil, "nonexistent", 1, 0))

	if !strings.Contains(body, "Search results for:") {
		t.Error("expected the search header")
	}
	if !strings.Contains(body, "nonexistent") {
		t.Error("expected the query to be echoed")
	}
	if !strings.Contains(body, "Clear search") {
		t.Error("expected a clear-search link")
	}
}

func TestBlog_WithPostsAndPagination(t *testing.T) {
	posts := []domain.BlogPost{
		{
			Title:       "Linux Adventures",
			Description: "<p>All about Linux</p>",
			URL:         "https://example.com/linux",
			PublishedAt: "January 02, 2006",
			Author:      "Nixie Pixel",
			ImageURL:    "https://example.com/linux.png",
			Categories:  []string{"Linux", "Open Source"},
		},
		{
			Title:       "No Author Here",
			Description: "Plain text",
			URL:         "https://example.com/plain",
			PublishedAt: "January 03, 2006",
		},
	}

	body := render(t, Blog(posts, []string{"Linux", "Open Source"}, "", 1, 2))

	for _, want := range []string{
		"Linux Adventures",
		`src="https://example.com/linux.png"`,
		"By Nixie Pixel on January 02, 2006",
		"Posted on January 03, 2006",
		`/blog?q=Linux`,
		"<p>All about Linux</p>", // rendered raw
		"Blog pagination",
		"Previous",
		"Next",
		`page-item active`,
		"Categories",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("blog page missing %q", want)
		}
	}
}

func TestBlog_PageTwo(t *testing.T) {
	posts := []domain.BlogPost{{Title: "Second page post", URL: "https://example.com/2"}}

	body := render(t, Blog(posts, []string{"Gaming"}, "Gaming", 2, 2))

	if !strings.Contains(body, "Second page post") {
		t.Error("expected the post to render")
	}
	// On the last page "Next" is disabled and "Previous" is not.
	if !strings.Contains(body, `page-item disabled`) {
		t.Error("expected a disabled pagination control on the last page")
	}
	if !strings.Contains(body, `page-item active`) {
		t.Error("expected the current page to be active")
	}
	if !strings.Contains(body, `/blog?page=1&amp;q=Gaming`) {
		t.Error("expected a previous-page link preserving the query")
	}
}

func TestFAQ(t *testing.T) {
	body := render(t, FAQ())
	for _, want := range []string{
		"Frequently Asked Questions",
		"What is GeekBeacon?",
		"How can I support Nixie Pixel?",
		`href="/contact"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("faq missing %q", want)
		}
	}
}

func TestSocialPage(t *testing.T) {
	body := render(t, SocialPage())
	for _, want := range []string{
		`id="social-page"`,
		`id="chart"`,
		`id="llama-hat-select"`,
		`id="curator-feed-default-feed-layout"`,
		"https://www.tiktok.com/@nixiepixel",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("social page missing %q", want)
		}
	}
}

func TestPortfolio(t *testing.T) {
	body := render(t, Portfolio())
	for _, want := range []string{
		"GBF (GeekBeacon Festival)",
		"Geekbeacon",
		"Product Reviews",
		"Trusted By Industry Leaders",
		"Newsletter",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("portfolio missing %q", want)
		}
	}
}

func TestContactPage(t *testing.T) {
	newsletters := []config.Newsletter{
		{Name: "GeekBeacon", Description: "community", URL: "https://geekbeacon.org/newsletter"},
		{Name: "OS.ALT", Description: "tech", URL: "https://example.com/osalt"},
	}

	body := render(t, ContactPage([]string{"Say Hello", "Speaking Engagements"}, newsletters))

	for _, want := range []string{
		"Contact Me",
		"Newsletters",
		"GeekBeacon",
		"OS.ALT",
		"https://geekbeacon.org/newsletter",
		"Subscribe to GeekBeacon",
		`name="subject"`,
		"Say Hello",
		"Speaking Engagements",
		`action="https://formspree.io/f/xleadqng"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("contact page missing %q", want)
		}
	}
}

func TestContactPage_NoNewslettersOrSubjects(t *testing.T) {
	body := render(t, ContactPage(nil, nil))
	if strings.Contains(body, "Subscribe to") {
		t.Error("no newsletter cards should render when none are configured")
	}
	if !strings.Contains(body, "Contact Me") {
		t.Error("contact form should still render")
	}
}

func TestContactFormAndSuccess(t *testing.T) {
	if body := render(t, ContactForm([]string{"Say Hello"})); !strings.Contains(body, "Say Hello") {
		t.Error("contact form missing subject")
	}
	if body := render(t, ContactSuccess()); !strings.Contains(body, "Your message has been sent successfully!") {
		t.Error("success banner missing")
	}
}
