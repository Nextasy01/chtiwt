package web

import (
	"errors"
	"net/http"

	"github.com/Nextasy01/chtiwt/internal/auth"
)

type watchData struct {
	ChannelName string
	Title       string
	Live        bool
}

func (s *Server) watch(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("channel")
	if name == "" {
		http.NotFound(w, r)
		return
	}

	ch, err := s.svc.ChannelByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, auth.ErrNoChannel) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Plant the anon cookie now so it's present on the subsequent
	// WebSocket upgrade triggered by the page's JS.
	ensureAnonCookie(w, r)

	data := watchData{ChannelName: ch.Name, Title: ch.Title}
	if live, ok := s.live.GetByName(ch.Name); ok {
		data.Title = live.Title
		data.Live = true
	}
	_ = s.tmpl.Render(w, r, "watch", data)
}