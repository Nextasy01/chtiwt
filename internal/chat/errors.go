package chat

import "errors"

var (
	// ErrLoginRequired is returned when a guest tries to send a message.
	// Read-only WebSocket access is allowed without auth.
	ErrLoginRequired = errors.New("must be logged in to chat")

	// ErrRateLimited is returned to the client when their token bucket is empty.
	ErrRateLimited = errors.New("rate limited")

	// ErrMessageTooLong is returned for messages over the size cap.
	ErrMessageTooLong = errors.New("message too long")

	// ErrEmptyMessage is returned for blank submissions.
	ErrEmptyMessage = errors.New("empty message")
)
