package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Nextasy01/chtiwt/internal/store/queries"
)

type contextKey int

const userCtxKey contextKey = 0

// Middleware loads the session user (if any) into the request context.
// Anonymous requests pass through; a bad/expired cookie also passes through
// (and gets cleared so the browser stops sending it).
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		user, err := s.SessionUser(r.Context(), cookie.Value)
		if err != nil {
			if !errors.Is(err, ErrNoSession) {
				slog.Warn("session lookup failed", "err", err)
			}
			clearSessionCookie(w, false) // secure flag isn't load-bearing on a clear
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, &user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func UserFromContext(ctx context.Context) (*queries.User, bool) {
	u, ok := ctx.Value(userCtxKey).(*queries.User)
	return u, ok
}

// RequireAuth wraps a handler and redirects anonymous visitors to /login.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFromContext(r.Context()); !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireGuest wraps a handler and redirects logged-in users to /dashboard.
// Useful for the signup and login pages.
func RequireGuest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFromContext(r.Context()); ok {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}
