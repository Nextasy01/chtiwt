package stream

import "errors"

var (
	// ErrUnknownStreamKey is returned by the ingest handler when a publisher
	// presents a stream key that does not map to any channel.
	ErrUnknownStreamKey = errors.New("unknown stream key")

	// ErrAlreadyLive is returned when a publisher tries to start a stream
	// for a channel that already has an active session.
	ErrAlreadyLive = errors.New("channel already live")
)
