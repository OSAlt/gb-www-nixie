package http

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

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
	end := start + pageSize
	if end > totalPosts {
		end = totalPosts
	}

	var paginatedPosts []domain.BlogPost
	if start < totalPosts {
		paginatedPosts = filteredPosts[start:end]
	}

	templ.Handler(templates.Blog(paginatedPosts, categories, query, page, totalPages)).ServeHTTP(w, r)
}

func (h *Handler) FAQ(w http.ResponseWriter, r *http.Request) {
	templ.Handler(templates.FAQ()).ServeHTTP(w, r)
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

func (h *Handler) APISocialInstagramActivity(w http.ResponseWriter, r *http.Request) {
	countStr := r.URL.Query().Get("count")
	count, err := strconv.Atoi(countStr)
	if err != nil {
		count = 20
	}
	activity, err := h.dbService.GetInstagramActivity(r.Context(), count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(activity)
}
