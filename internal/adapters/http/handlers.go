package http

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OSAlt/gb-www-nixie/internal/config"
	"github.com/OSAlt/gb-www-nixie/internal/domain"
	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/OSAlt/gb-www-nixie/internal/templates"
	"github.com/a-h/templ"
)

type Handler struct {
	mediaService   ports.MediaService
	contactService ports.ContactService
	dbService      ports.DBService
	blogService    ports.BlogService
	cfg            *config.Config
}

func NewHandler(mediaService ports.MediaService, contactService ports.ContactService, dbService ports.DBService, blogService ports.BlogService, cfg *config.Config) *Handler {
	return &Handler{
		mediaService:   mediaService,
		contactService: contactService,
		dbService:      dbService,
		blogService:    blogService,
		cfg:            cfg,
	}
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	media, _ := h.mediaService.GetRecentPosts(r.Context(), 6)
	subjects, _ := h.contactService.GetSubjects(r.Context())

	if subjects == nil {
		subjects = []string{"Say Hello", "Speaking Engagements", "Business Opportunity", "Content Collaboration"}
	}

	templ.Handler(templates.Index(media, subjects)).ServeHTTP(w, r)
}

func (h *Handler) Blog(w http.ResponseWriter, r *http.Request) {
	posts, _ := h.blogService.GetRecentPosts(r.Context())

	// Collect unique categories and handle search
	query := strings.ToLower(r.URL.Query().Get("q"))
	pageStr := r.URL.Query().Get("page")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	pageSize := 5

	categoryMap := make(map[string]struct{})
	filteredPosts := make([]domain.BlogPost, 0, len(posts))

	for _, post := range posts {
		for _, cat := range post.Categories {
			categoryMap[cat] = struct{}{}
		}

		if query == "" {
			filteredPosts = append(filteredPosts, post)
		} else {
			// Search in title, description, content, or categories
			match := strings.Contains(strings.ToLower(post.Title), query) ||
				strings.Contains(strings.ToLower(post.Description), query) ||
				strings.Contains(strings.ToLower(post.Content), query)

			if !match {
				for _, cat := range post.Categories {
					if strings.Contains(strings.ToLower(cat), query) {
						match = true
						break
					}
				}
			}

			if match {
				filteredPosts = append(filteredPosts, post)
			}
		}
	}

	categories := slices.Sorted(maps.Keys(categoryMap))

	// Pagination
	totalPosts := len(filteredPosts)
	totalPages := (totalPosts + pageSize - 1) / pageSize
	if page > totalPages && totalPages > 0 {
		page = totalPages
	}

	start := (page - 1) * pageSize
	end := min(start+pageSize, totalPosts)

	var paginatedPosts []domain.BlogPost
	if start < totalPosts {
		paginatedPosts = filteredPosts[start:end]
	}

	templ.Handler(templates.Blog(paginatedPosts, categories, query, page, totalPages)).ServeHTTP(w, r)
}

func (h *Handler) FAQ(w http.ResponseWriter, r *http.Request) {
	templ.Handler(templates.FAQ()).ServeHTTP(w, r)
}

func (h *Handler) Social(w http.ResponseWriter, r *http.Request) {
	templ.Handler(templates.SocialPage()).ServeHTTP(w, r)
}

func (h *Handler) Portfolio(w http.ResponseWriter, r *http.Request) {
	templ.Handler(templates.Portfolio()).ServeHTTP(w, r)
}

func (h *Handler) Contact(w http.ResponseWriter, r *http.Request) {
	subjects, _ := h.contactService.GetSubjects(r.Context())

	if subjects == nil {
		subjects = []string{"Say Hello", "Speaking Engagements", "Business Opportunity", "Content Collaboration"}
	}

	templ.Handler(templates.ContactPage(subjects, h.cfg.Newsletters)).ServeHTTP(w, r)
}

func (h *Handler) APIListContacts(w http.ResponseWriter, r *http.Request) {
	domainName := r.URL.Query().Get("domain")
	contacts, err := h.dbService.ListContacts(r.Context(), domainName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(contacts)
}

func (h *Handler) APISocialCountAll(w http.ResponseWriter, r *http.Request) {
	counts, err := h.dbService.GetSocialCounts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(counts)
}

// APISocialHealth reports how fresh each platform's stored follower count is.
//
// The worker rewrites refreshed_at on every successful update, so its age is a
// heartbeat: a platform that stops advancing means the fetch is failing (or
// the worker is no longer running) even though the site itself stays up.
// Monitoring should treat status "degraded" as the alert condition; the HTTP
// status stays 200 so a health check can never be mistaken for a dead server.
func (h *Handler) APISocialHealth(w http.ResponseWriter, r *http.Request) {
	respond := func(code int, payload any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(payload)
	}

	if h.dbService == nil {
		respond(http.StatusServiceUnavailable, map[string]any{
			"status": "unavailable",
			"error":  "database not configured",
		})
		return
	}

	states, err := h.dbService.GetSocialMetricStates(r.Context())
	if err != nil {
		respond(http.StatusServiceUnavailable, map[string]any{
			"status": "unavailable",
			"error":  err.Error(),
		})
		return
	}

	interval := 24 * time.Hour
	if h.cfg != nil && h.cfg.SocialRefreshSeconds > 0 {
		interval = time.Duration(h.cfg.SocialRefreshSeconds) * time.Second
	}
	// Two missed cycles' worth of slack: one failed refresh is normal.
	staleAfter := 2 * interval

	type platformHealth struct {
		Followers   int    `json:"followers"`
		RefreshedAt string `json:"refreshed_at"`
		AgeSeconds  int64  `json:"age_seconds"`
		Stale       bool   `json:"stale"`
	}

	now := time.Now()
	platforms := make(map[string]platformHealth, len(states))
	stale := make([]string, 0, len(states))
	for _, state := range states {
		age := now.Sub(state.RefreshedAt)
		isStale := age > staleAfter
		platforms[state.Platform] = platformHealth{
			Followers:   state.Followers,
			RefreshedAt: state.RefreshedAt.UTC().Format(time.RFC3339),
			AgeSeconds:  int64(age.Seconds()),
			Stale:       isStale,
		}
		if isStale {
			stale = append(stale, state.Platform)
		}
	}
	sort.Strings(stale)

	status := "ok"
	switch {
	case len(states) == 0:
		// No rows at all: nothing has ever been refreshed.
		status = "degraded"
	case len(stale) > 0:
		status = "degraded"
	}

	respond(http.StatusOK, map[string]any{
		"status":              status,
		"checked_at":          now.UTC().Format(time.RFC3339),
		"interval_seconds":    int(interval.Seconds()),
		"stale_after_seconds": int(staleAfter.Seconds()),
		"stale_platforms":     stale,
		"platforms":           platforms,
	})
}
