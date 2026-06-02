package stream

import "sync"

// liveRegistry is the single source of truth for currently-live channels.
// The DB has no is_live column on purpose — a crashed process must not
// leave a channel stuck "live".
type liveRegistry struct {
	mu       sync.RWMutex
	byName   map[string]*session // keyed by channel name
}

func newLiveRegistry() *liveRegistry {
	return &liveRegistry{byName: make(map[string]*session)}
}

// add inserts a session. Returns false if a session already exists for the
// channel — callers should treat that as ErrAlreadyLive.
func (r *liveRegistry) add(s *session) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[s.channelName]; ok {
		return false
	}
	r.byName[s.channelName] = s
	return true
}

func (r *liveRegistry) remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byName, name)
}

func (r *liveRegistry) get(name string) (*session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byName[name]
	return s, ok
}

func (r *liveRegistry) snapshot() []*session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*session, 0, len(r.byName))
	for _, s := range r.byName {
		out = append(out, s)
	}
	return out
}
