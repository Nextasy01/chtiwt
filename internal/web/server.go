package web

import (
	"net/http"

	"github.com/Nextasy01/chtiwt/internal/auth"
)

type Server struct {
	svc  *auth.Service
	tmpl *Templates
}

func NewServer(svc *auth.Service, tmpl *Templates) *Server {
	return &Server{svc: svc, tmpl: tmpl}
}

// Mount registers the web routes on mux. Routes that require a logged-in
// user are wrapped with auth.RequireAuth here; the global auth.Middleware
// that loads the user into context is mounted around the whole mux in main.
func (s *Server) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /", s.home)
	mux.Handle("GET /dashboard", auth.RequireAuth(http.HandlerFunc(s.dashboard)))
	mux.Handle("POST /dashboard/title", auth.RequireAuth(http.HandlerFunc(s.updateTitle)))
	mux.Handle("POST /dashboard/regenerate", auth.RequireAuth(http.HandlerFunc(s.regenerateKey)))
}
