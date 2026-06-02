package stream

import "time"

// LiveChannel is the snapshot exposed to consumers (web feed, watch page).
// It is intentionally decoupled from the internal session struct so callers
// can never touch the live pipeline.
type LiveChannel struct {
	ChannelName string
	Title       string
	StartedAt   time.Time
}
