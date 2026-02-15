package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/OSAlt/gb-www-nixie/internal/ports"
	"github.com/OSAlt/gb-www-nixie/internal/templates"
	"github.com/a-h/templ"
)

type Handler struct {
	mediaService   ports.MediaService
	contactService ports.ContactService
	dbService      ports.DBService
}

func NewHandler(mediaService ports.MediaService, contactService ports.ContactService, dbService ports.DBService) *Handler {
	return &Handler{
		mediaService:   mediaService,
		contactService: contactService,
		dbService:      dbService,
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

func (h *Handler) Contact(w http.ResponseWriter, r *http.Request) {
	templ.Handler(templates.ContactSuccess()).ServeHTTP(w, r)
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
