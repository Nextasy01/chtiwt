package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/Nextasy01/chtiwt/internal/auth"
	"github.com/Nextasy01/chtiwt/internal/store/queries"
)

//go:embed templates/*.html
var templateFS embed.FS

// PageData is the shape every template receives: the current user (nil if
// anonymous) plus a page-specific payload.
type PageData struct {
	User *queries.User
	Data any
}

type Templates struct {
	pages map[string]*template.Template
}

func LoadTemplates() (*Templates, error) {
	pageNames := []string{"home", "signup", "login", "dashboard", "key_shown", "watch"}
	pages := make(map[string]*template.Template, len(pageNames))
	for _, name := range pageNames {
		t, err := template.ParseFS(templateFS,
			"templates/base.html",
			"templates/"+name+".html",
		)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		pages[name] = t
	}
	return &Templates{pages: pages}, nil
}

// Render satisfies auth.Renderer. Always renders through the base layout.
func (t *Templates) Render(w http.ResponseWriter, r *http.Request, name string, data any) error {
	page, ok := t.pages[name]
	if !ok {
		return fmt.Errorf("unknown template: %s", name)
	}
	user, _ := auth.UserFromContext(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return page.ExecuteTemplate(w, "base", PageData{User: user, Data: data})
}
