package web

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

// anonCookieName is the long-lived cookie identifying an anonymous viewer.
// Combined with the username for logged-in users, it forms the "viewer key"
// the chat Room uses to dedupe viewer counts across tabs.
const anonCookieName = "chtiwt_anon"

// ensureAnonCookie returns the viewer's anonymous ID, generating and
// setting a fresh cookie if none exists. Path=/ so the ID is shared across
// channels — a viewer who switches channels is still the same person.
// HttpOnly because the client never needs to read it; the server reads it
// from the request headers (including during the WebSocket upgrade).
func ensureAnonCookie(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(anonCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Falling back to a stable-but-unhelpful value is worse than
		// "this connection won't dedupe" — let it through and the chat
		// Room will treat them as a unique guest for this connection only.
		return ""
	}
	id := base64.RawURLEncoding.EncodeToString(buf[:])
	http.SetCookie(w, &http.Cookie{
		Name:     anonCookieName,
		Value:    id,
		Path:     "/",
		MaxAge:   365 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return id
}
