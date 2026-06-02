package web

import "net/http"

type homeData struct {
	Live []liveCard
}

type liveCard struct {
	ChannelName string
	Title       string
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	// Only render home on the literal "/" path; net/http's ServeMux uses
	// "GET /" as a catch-all for un-matched routes, so without this guard
	// a typo'd URL like /foo would silently render the homepage.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	live := s.live.ListLive()
	cards := make([]liveCard, 0, len(live))
	for _, c := range live {
		cards = append(cards, liveCard{ChannelName: c.ChannelName, Title: c.Title})
	}
	_ = s.tmpl.Render(w, r, "home", homeData{Live: cards})
}
