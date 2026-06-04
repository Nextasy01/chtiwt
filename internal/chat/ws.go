package chat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const (
	// maxMessageBytes caps a single inbound frame.
	maxMessageBytes = 4 * 1024

	// maxMessageChars caps the user-visible message length.
	maxMessageChars = 500

	// writeTimeout bounds a single websocket write.
	writeTimeout = 5 * time.Second
)

// client is one connected viewer. The room reads/writes only via its
// channels — `out` is the room→client message queue; the client's reader
// goroutine forwards user inbound to the room.
type client struct {
	username string // empty for guests
	userID   int64  // 0 for guests
	anonKey  string // long-lived per-browser ID for guests; "" for logged in
	out      chan Message
	closed   bool // set by Room; makes removal idempotent
}

// viewerKey returns the identity used for viewer-count deduping. Logged-in
// users dedupe by username (same human across all tabs/devices); guests
// dedupe by their anonymous cookie ID (same browser profile). If both are
// empty (rare: WS connected with no session and no cookie), every
// connection counts as unique.
func (c *client) viewerKey() string {
	switch {
	case c.username != "":
		return "u:" + c.username
	case c.anonKey != "":
		return "g:" + c.anonKey
	default:
		return ""
	}
}

// ServeWS upgrades the HTTP connection to a WebSocket and runs the
// client lifecycle. username == "" / userID == 0 means a guest (read-only);
// anonKey is the guest's long-lived dedup ID (may be "" if missing).
func (s *Service) ServeWS(w http.ResponseWriter, r *http.Request, channelName, username string, userID int64, anonKey string) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// We only ever talk to our own page on the same origin.
		InsecureSkipVerify: false,
	})
	if err != nil {
		// Accept already wrote the response.
		return
	}
	defer conn.CloseNow()

	room := s.manager.getOrCreate(channelName)
	if room == nil {
		_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
		return
	}

	c := &client{
		username: username,
		userID:   userID,
		anonKey:  anonKey,
		out:      make(chan Message, clientSendBuffer),
	}

	// Register with the room.
	select {
	case room.join <- c:
	case <-r.Context().Done():
		return
	}

	// Reader goroutine: WebSocket → room (only authenticated users may send).
	readerCtx, readerCancel := context.WithCancel(r.Context())
	defer readerCancel()
	go s.readLoop(readerCtx, conn, room, c)

	// Writer loop (this goroutine): room → WebSocket.
	for msg := range c.out {
		wctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		err := writeJSON(wctx, conn, msg)
		cancel()
		if err != nil {
			break
		}
	}

	// c.out was closed by the room (either we were evicted, or the room
	// is shutting down). Make sure the room knows we're gone — harmless
	// if it already does (leave is buffered).
	select {
	case room.leave <- c:
	default:
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
}

func (s *Service) readLoop(ctx context.Context, conn *websocket.Conn, room *Room, c *client) {
	conn.SetReadLimit(maxMessageBytes)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			// Tell the writer side to wind down by leaving the room. The
			// room closes c.out, which breaks the writer's range loop.
			select {
			case room.leave <- c:
			default:
			}
			return
		}

		if c.userID == 0 {
			s.sendErr(ctx, conn, ErrLoginRequired)
			continue
		}

		var in Message
		if err := json.Unmarshal(data, &in); err != nil {
			s.sendErr(ctx, conn, errors.New("bad json"))
			continue
		}
		body := strings.TrimSpace(in.Body)
		if body == "" {
			s.sendErr(ctx, conn, ErrEmptyMessage)
			continue
		}
		if len([]rune(body)) > maxMessageChars {
			s.sendErr(ctx, conn, ErrMessageTooLong)
			continue
		}
		if !s.manager.allowUser(c.userID) {
			s.sendErr(ctx, conn, ErrRateLimited)
			continue
		}

		select {
		case room.send <- inbound{user: c.username, body: body}:
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) sendErr(ctx context.Context, conn *websocket.Conn, err error) {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_ = writeJSON(wctx, conn, Message{Type: KindError, Body: err.Error(), At: time.Now().UTC()})
}

func writeJSON(ctx context.Context, conn *websocket.Conn, m Message) error {
	buf, err := json.Marshal(m)
	if err != nil {
		slog.Warn("chat marshal", "err", err)
		return err
	}
	return conn.Write(ctx, websocket.MessageText, buf)
}
