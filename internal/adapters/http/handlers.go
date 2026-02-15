package http

import (
    "net/http"

    "github.com/a-h/templ"
    "github.com/osalt/nixiesite/internal/domain"
    "github.com/osalt/nixiesite/internal/ports"
    "github.com/osalt/nixiesite/internal/templates"
)

type Handler struct {
    mediaService   ports.MediaService
    contactService ports.ContactService
}

func NewHandler(mediaService ports.MediaService, contactService ports.ContactService) *Handler {
    return &Handler{
        mediaService:   mediaService,
        contactService: contactService,
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
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    r.ParseForm()
    msg := domain.ContactMessage{
        Name:    r.FormValue("name"),
        Email:   r.FormValue("email"),
        Subject: r.FormValue("subject"),
        Message: r.FormValue("message"),
    }

    h.contactService.SendMessage(r.Context(), msg)
    templ.Handler(templates.ContactSuccess()).ServeHTTP(w, r)
}
