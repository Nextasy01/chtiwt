package chat

import "time"

// MessageKind discriminates outbound message envelopes.
type MessageKind string

const (
	KindMessage MessageKind = "message"
	KindSystem  MessageKind = "system"
	KindError   MessageKind = "error"
	KindViewers MessageKind = "viewers"
)

// Message is the on-the-wire envelope, JSON-encoded both directions.
// Inbound from clients only uses Body; the server fills in everything else.
type Message struct {
	ID    string      `json:"id,omitempty"`
	Type  MessageKind `json:"type,omitempty"`
	User  string      `json:"user,omitempty"`
	Body  string      `json:"body"`
	At    time.Time   `json:"at,omitzero"`
	Count int         `json:"count,omitempty"` // set on KindViewers
}
