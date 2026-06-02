package web

import (
	"net/http"

	"github.com/Nextasy01/chtiwt/internal/auth"
	"github.com/Nextasy01/chtiwt/internal/stream"
)

// liveDirectory is the narrow read-only view of the stream package that web
// depends on. Declared here (consumer-side) so web isn't coupled to the
// rest of the stream API surface.
type liveDirectory interface {
	ListLive() []stream.LiveChannel
	GetByName(name string) (stream.LiveChannel, bool)
}

type Server struct {
	svc      *auth.Service
	live     liveDirectory
	stateDir string
	tmpl     *Templates
}

func NewServer(svc *auth.Service, live liveDirectory, stateDir string, tmpl *Templates) *Server {
	return &Server{svc: svc, live: live, stateDir: stateDir, tmpl: tmpl}
}

// Mount registers the web routes on mux. Routes that require a logged-in
// user are wrapped with auth.RequireAuth here; the global auth.Middleware
// that loads the user into context is mounted around the whole mux in main.
func (s *Server) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /", s.home)
	mux.HandleFunc("GET /c/{channel}", s.watch)
	mux.Handle("GET /hls/", s.hlsFileServer())
	mux.Handle("GET /dashboard", auth.RequireAuth(http.HandlerFunc(s.dashboard)))
	mux.Handle("POST /dashboard/title", auth.RequireAuth(http.HandlerFunc(s.updateTitle)))
	mux.Handle("POST /dashboard/regenerate", auth.RequireAuth(http.HandlerFunc(s.regenerateKey)))
}

// hlsFileServer serves HLS playlists and TS segments from the on-disk
// state dir. Only files under the configured root are reachable; we rely
// on http.FileServer + http.Dir's built-in path cleaning to prevent
// traversal. We disable caching so playlist updates are picked up.
func (s *Server) hlsFileServer() http.Handler {
	fs := http.StripPrefix("/hls/", http.FileServer(http.Dir(s.stateDir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fs.ServeHTTP(w, r)
	})
}
