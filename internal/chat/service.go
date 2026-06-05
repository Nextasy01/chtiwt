package chat

// Options bundles tunables for the chat Service.
type Options struct {
	RateBurst     int     // per-user burst, default 5
	RatePerSecond float64 // per-user sustained, default 1.0
}

// Service is the public facade for the chat subsystem. Web depends on a
// narrow interface it declares itself; this struct provides the methods.
type Service struct {
	manager *manager
}

func NewService(opts Options) *Service {
	burst := opts.RateBurst
	if burst <= 0 {
		burst = 5
	}
	perSec := opts.RatePerSecond
	if perSec <= 0 {
		perSec = 1.0
	}
	return &Service{manager: newManager(burst, perSec)}
}

// Shutdown closes every active room. Safe to call once.
func (s *Service) Shutdown() {
	s.manager.shutdownAll()
}

// ViewerCount returns the number of unique viewers currently in the room
// for channelName, or 0 if no room exists. Lock-free read of an atomic
// cached value the Room maintains.
func (s *Service) ViewerCount(channelName string) int {
	return s.manager.viewerCount(channelName)
}
