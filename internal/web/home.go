package web

import "net/http"

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	// Only render home on the literal "/" path; net/http's ServeMux uses
	// "GET /" as a catch-all for un-matched routes, so without this guard
	// a typo'd URL like /foo would silently render the homepage.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	_ = s.tmpl.Render(w, r, "home", nil)
}
