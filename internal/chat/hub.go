package chat

import (
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	// historySize is the number of recent messages a new joiner receives.
	historySize = 50

	// clientSendBuffer is the size of each subscriber's outbound queue. If
	// it fills (slow consumer / dead TCP) the room evicts that client.
	clientSendBuffer = 32

	// roomIdleAfter is how long an empty room lingers before the manager
	// reaps it. Set so a brief reconnect doesn't churn the room.
	roomIdleAfter = 30 * time.Second
)

// inbound carries a user-authored message into the Room. The Room is
// responsible for stamping it with id/at and broadcasting.
type inbound struct {
	user string
	body string
}

// Room is the per-channel chat hub. All state lives inside a single
// goroutine (run); join/leave/inbound channels are the only API surface.
// No mutexes — concurrent mutation is impossible by construction.
type Room struct {
	name string

	join  chan *client
	leave chan *client
	send  chan inbound
	stop  chan struct{}

	// viewerCount is a read-side cache of len(viewerCounts) inside the
	// run() loop. The actor writes it under its own (implicit) ownership;
	// outside callers (the directory page) read it lock-free via
	// atomic.LoadInt32. Source of truth still lives in the actor's map.
	viewerCount atomic.Int32

	// onEmpty fires when the last client leaves; the manager uses it to
	// schedule reaping.
	onEmpty func()
}

func newRoom(name string, onEmpty func()) *Room {
	return &Room{
		name:    name,
		join:    make(chan *client, 4),
		leave:   make(chan *client, 4),
		send:    make(chan inbound, 64),
		stop:    make(chan struct{}),
		onEmpty: onEmpty,
	}
}

// run is the actor loop. Exits when stop is closed.
//
// Presence: userPresence counts active connections per *username*, not per
// client. That way opening N tabs as alice broadcasts "alice joined" once
// (on the first tab) and "alice left" once (when the last tab disconnects).
// Guests have username "" and are excluded from presence entirely — they
// receive messages but their come-and-go is invisible.
//
// Viewer count: viewerCounts is keyed by viewerKey ("u:<username>" or
// "g:<anonID>") so the same human across tabs/devices counts once. We
// broadcast a KindViewers update whenever the count changes.
func (r *Room) run() {
	clients := make(map[*client]struct{})
	history := make([]Message, 0, historySize)
	userPresence := make(map[string]int)
	viewerCounts := make(map[string]int)

	viewerMessage := func() Message {
		return Message{Type: KindViewers, Count: len(viewerCounts)}
	}

	publishCount := func() {
		r.viewerCount.Store(int32(len(viewerCounts)))
	}

	appendHistory := func(m Message) {
		if len(history) == historySize {
			history = append(history[:0], history[1:]...)
		}
		history = append(history, m)
	}

	// removeClient is the single point of client removal. Idempotent via
	// c.closed so the writer-goroutine's deferred leave (sent after a
	// dispatch-time eviction) is a harmless no-op. Returns:
	//   wasLastPresence — last connection for this username (caller broadcasts "left")
	//   viewerChanged   — last connection for this viewerKey (caller broadcasts viewer count)
	removeClient := func(c *client) (wasLastPresence, viewerChanged bool) {
		if c.closed {
			return false, false
		}
		c.closed = true
		delete(clients, c)
		close(c.out)

		if vk := c.viewerKey(); vk != "" {
			viewerCounts[vk]--
			if viewerCounts[vk] <= 0 {
				delete(viewerCounts, vk)
				viewerChanged = true
			}
		}

		if c.username != "" {
			userPresence[c.username]--
			if userPresence[c.username] <= 0 {
				delete(userPresence, c.username)
				wasLastPresence = true
			}
		}
		return wasLastPresence, viewerChanged
	}

	dispatch := func(m Message) {
		for c := range clients {
			select {
			case c.out <- m:
			default:
				// Slow consumer — evict. We don't broadcast "left" here
				// to avoid recursing back into dispatch; the user will
				// just silently disappear (rare enough not to matter).
				slog.Info("chat client evicted (slow)", "room", r.name, "user", c.username)
				removeClient(c)
			}
		}
	}

	for {
		select {
		case c := <-r.join:
			// Deliver history first; if the client's buffer can't even
			// take the backlog we won't register them at all.
			overflow := false
			for _, m := range history {
				select {
				case c.out <- m:
				default:
					overflow = true
				}
				if overflow {
					break
				}
			}
			if overflow {
				close(c.out)
				continue
			}
			clients[c] = struct{}{}

			// Viewer count: increment unique viewers; broadcast on transitions.
			viewerChanged := false
			if vk := c.viewerKey(); vk != "" {
				viewerCounts[vk]++
				if viewerCounts[vk] == 1 {
					viewerChanged = true
				}
			}

			// Presence: only for logged-in users, only on first connection.
			if c.username != "" {
				userPresence[c.username]++
				if userPresence[c.username] == 1 {
					dispatch(Message{
						Type: KindSystem,
						Body: c.username + " joined",
						At:   time.Now().UTC(),
					})
				}
			}

			if viewerChanged {
				publishCount()
				dispatch(viewerMessage())
			} else {
				// Same-human reconnect: tell only the new tab the current count.
				select {
				case c.out <- viewerMessage():
				default:
				}
			}

		case c := <-r.leave:
			wasLastPresence, viewerChanged := removeClient(c)
			if wasLastPresence {
				dispatch(Message{
					Type: KindSystem,
					Body: c.username + " left",
					At:   time.Now().UTC(),
				})
			}
			if viewerChanged {
				publishCount()
				dispatch(viewerMessage())
			}
			if len(clients) == 0 && r.onEmpty != nil {
				r.onEmpty()
			}

		case msg := <-r.send:
			m := Message{
				ID:   uuid.Must(uuid.NewV7()).String(),
				Type: KindMessage,
				User: msg.user,
				Body: msg.body,
				At:   time.Now().UTC(),
			}
			appendHistory(m)
			dispatch(m)

		case <-r.stop:
			for c := range clients {
				removeClient(c)
			}
			return
		}
	}
}
