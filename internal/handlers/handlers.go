package handlers

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/osalt/nixiesite/internal/templates"
)

func HelloHandler(w http.ResponseWriter, r *http.Request) {
	component := templates.Hello("Friend")
	templ.Handler(component).ServeHTTP(w, r)
}
