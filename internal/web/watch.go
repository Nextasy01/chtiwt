package web

import "net/http"

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

	data := watchData{ChannelName: name}
	if live, ok := s.live.GetByName(name); ok {
		data.Title = live.Title
		data.Live = true
	}
	_ = s.tmpl.Render(w, r, "watch", data)
}
