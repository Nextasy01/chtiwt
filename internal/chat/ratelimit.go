package chat

import (
	"sync"
	"time"
)

// limiter is a leaky/refilling token bucket keyed per user.
// Burst = max tokens; refill = 1 token every refillEvery.
type limiter struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func newLimiter(burst int, perSecond float64) *limiter {
	return &limiter{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: perSecond,
		lastRefill: time.Now(),
	}
}

// allow consumes one token. Returns true if the action is permitted.
func (l *limiter) allow(now time.Time) bool {
	elapsed := now.Sub(l.lastRefill).Seconds()
	if elapsed > 0 {
		l.tokens = minFloat(l.maxTokens, l.tokens+elapsed*l.refillRate)
		l.lastRefill = now
	}
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// limiterMap is the per-user bucket store. Lives at manager (process) scope
// so a user opening multiple tabs or channels shares a single quota.
type limiterMap struct {
	mu        sync.Mutex
	byUser    map[int64]*limiter
	burst     int
	perSecond float64
}

func newLimiterMap(burst int, perSecond float64) *limiterMap {
	return &limiterMap{
		byUser:    make(map[int64]*limiter),
		burst:     burst,
		perSecond: perSecond,
	}
}

func (m *limiterMap) allow(userID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.byUser[userID]
	if !ok {
		l = newLimiter(m.burst, m.perSecond)
		m.byUser[userID] = l
	}
	return l.allow(time.Now())
}
