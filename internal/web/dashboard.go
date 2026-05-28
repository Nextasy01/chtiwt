package web

import (
	"log/slog"
	"net/http"

	"github.com/Nextasy01/chtiwt/internal/auth"
	"github.com/Nextasy01/chtiwt/internal/store/queries"
)

type dashboardData struct {
	Channel queries.Channel
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context()) // RequireAuth guarantees presence
	channel, err := s.svc.ChannelForUser(r.Context(), user.ID)
	if err != nil {
		slog.Error("dashboard load channel", "user_id", user.ID, "err", err)
		http.Error(w, "could not load channel", http.StatusInternalServerError)
		return
	}
	_ = s.tmpl.Render(w, r, "dashboard", dashboardData{Channel: channel})
}

func (s *Server) updateTitle(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	channel, err := s.svc.ChannelForUser(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "no channel", http.StatusInternalServerError)
		return
	}
	if err := s.svc.UpdateChannelTitle(r.Context(), channel.ID, r.PostForm.Get("title")); err != nil {
		slog.Error("update title", "channel_id", channel.ID, "err", err)
		http.Error(w, "could not update title", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) regenerateKey(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	channel, err := s.svc.ChannelForUser(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "no channel", http.StatusInternalServerError)
		return
	}
	plain, err := s.svc.RegenerateStreamKey(r.Context(), channel.ID)
	if err != nil {
		slog.Error("regenerate key", "channel_id", channel.ID, "err", err)
		http.Error(w, "could not regenerate", http.StatusInternalServerError)
		return
	}
	_ = s.tmpl.Render(w, r, "key_shown", map[string]any{
		"Heading": "New stream key",
		"Lead":    "Your old key is now invalid. Copy the new one — it won't be shown again.",
		"Plain":   plain,
		"CTA":     "Back to dashboard",
		"CTAHref": "/dashboard",
	})
}
