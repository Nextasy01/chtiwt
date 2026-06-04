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

// chatGateway is the narrow view of the chat package that web depends on.
// username/userID are zero for unauthenticated viewers (read-only chat);
// anonKey is a long-lived per-browser ID so guest viewers dedupe across tabs.
type chatGateway interface {
	ServeWS(w http.ResponseWriter, r *http.Request, channelName, username string, userID int64, anonKey string)
}

type Server struct {
	svc      *auth.Service
	live     liveDirectory
	chat     chatGateway
	stateDir string
	tmpl     *Templates
}

func NewServer(svc *auth.Service, live liveDirectory, chat chatGateway, stateDir string, tmpl *Templates) *Server {
	return &Server{svc: svc, live: live, chat: chat, stateDir: stateDir, tmpl: tmpl}
}

// Mount registers the web routes on mux. Routes that require a logged-in
// user are wrapped with auth.RequireAuth here; the global auth.Middleware
// that loads the user into context is mounted around the whole mux in main.
func (s *Server) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /", s.home)
	mux.HandleFunc("GET /c/{channel}", s.watch)
	mux.HandleFunc("GET /ws/chat/{channel}", s.chatWS)
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

// chatWS dispatches WebSocket upgrades to the chat gateway. We verify the
// channel exists first so random URLs can't create unbounded rooms. We
// pull the authenticated user from the auth-middleware context (nil for guests).
func (s *Server) chatWS(w http.ResponseWriter, r *http.Request) {
	channel := r.PathValue("channel")
	if channel == "" {
		http.NotFound(w, r)
		return
	}
	ch, err := s.svc.ChannelByName(r.Context(), channel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var (
		username string
		userID   int64
		anonKey  string
	)
	if u, ok := auth.UserFromContext(r.Context()); ok && u != nil {
		username = u.Username
		userID = u.ID
	} else {
		// Read the cookie set by the watch page. If a client connects
		// to the WS directly (no prior page load), we set one now —
		// upgrade responses can still write headers before the switch.
		anonKey = ensureAnonCookie(w, r)
	}
	s.chat.ServeWS(w, r, ch.Name, username, userID, anonKey)
}
