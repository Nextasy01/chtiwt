package chat

import (
	"sync"
	"time"
)

// manager owns the rooms map and the per-user rate limiter map. Both are
// process-scoped; rooms are keyed by channel name, limiters by user ID so
// quota follows the human across tabs and channels.
type manager struct {
	mu      sync.Mutex
	rooms   map[string]*roomEntry
	limits  *limiterMap
	stopped bool
}

type roomEntry struct {
	room      *Room
	reapTimer *time.Timer // non-nil while empty and pending reap
}

func newManager(burst int, perSecond float64) *manager {
	return &manager{
		rooms:  make(map[string]*roomEntry),
		limits: newLimiterMap(burst, perSecond),
	}
}

// getOrCreate returns the Room for channelName, starting its actor goroutine
// on first creation. If a reap timer was pending, it is cancelled.
func (m *manager) getOrCreate(channelName string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return nil
	}
	if e, ok := m.rooms[channelName]; ok {
		if e.reapTimer != nil {
			e.reapTimer.Stop()
			e.reapTimer = nil
		}
		return e.room
	}
	r := newRoom(channelName, func() { m.scheduleReap(channelName) })
	m.rooms[channelName] = &roomEntry{room: r}
	go r.run()
	return r
}

// scheduleReap is invoked from inside the Room's goroutine when it goes
// empty. We start a timer; if a new client joins before it fires, the
// timer is cancelled in getOrCreate.
func (m *manager) scheduleReap(channelName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.rooms[channelName]
	if !ok {
		return
	}
	if e.reapTimer != nil {
		e.reapTimer.Stop()
	}
	e.reapTimer = time.AfterFunc(roomIdleAfter, func() { m.reap(channelName) })
}

func (m *manager) reap(channelName string) {
	m.mu.Lock()
	e, ok := m.rooms[channelName]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.rooms, channelName)
	m.mu.Unlock()
	close(e.room.stop)
}

// shutdownAll stops every room. Called from main on graceful shutdown.
func (m *manager) shutdownAll() {
	m.mu.Lock()
	m.stopped = true
	rooms := make([]*roomEntry, 0, len(m.rooms))
	for name, e := range m.rooms {
		if e.reapTimer != nil {
			e.reapTimer.Stop()
		}
		rooms = append(rooms, e)
		delete(m.rooms, name)
	}
	m.mu.Unlock()

	for _, e := range rooms {
		close(e.room.stop)
	}
}

func (m *manager) allowUser(userID int64) bool {
	return m.limits.allow(userID)
}

// viewerCount returns the cached unique-viewer count from the Room with
// the given name, or 0 if no room exists for that channel.
func (m *manager) viewerCount(channelName string) int {
	m.mu.Lock()
	e, ok := m.rooms[channelName]
	m.mu.Unlock()
	if !ok {
		return 0
	}
	return int(e.room.viewerCount.Load())
}
