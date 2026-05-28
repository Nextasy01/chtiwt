package auth

import (
	"errors"
	"log/slog"
	"net/http"
)

// Renderer is the seam between auth handlers and the web package's template
// engine. The web.Templates type satisfies it.
type Renderer interface {
	Render(w http.ResponseWriter, r *http.Request, name string, data any) error
}

type Handlers struct {
	svc           *Service
	tmpl          Renderer
	secureCookies bool
	sessionTTL    int // seconds, for cookie MaxAge
}

func NewHandlers(svc *Service, tmpl Renderer, secureCookies bool) *Handlers {
	return &Handlers{
		svc:           svc,
		tmpl:          tmpl,
		secureCookies: secureCookies,
		sessionTTL:    int(svc.sessionTTL.Seconds()),
	}
}

// Mount registers the auth-related routes on the given mux.
func (h *Handlers) Mount(mux *http.ServeMux) {
	mux.Handle("GET /signup", RequireGuest(http.HandlerFunc(h.signupForm)))
	mux.Handle("POST /signup", RequireGuest(http.HandlerFunc(h.signup)))
	mux.Handle("GET /login", RequireGuest(http.HandlerFunc(h.loginForm)))
	mux.Handle("POST /login", RequireGuest(http.HandlerFunc(h.login)))
	mux.Handle("POST /logout", RequireAuth(http.HandlerFunc(h.logout)))
}

type signupFormData struct {
	Username string
	Email    string
	Error    string
}

func (h *Handlers) signupForm(w http.ResponseWriter, r *http.Request) {
	_ = h.tmpl.Render(w, r, "signup", signupFormData{})
}

func (h *Handlers) signup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	username := r.PostForm.Get("username")
	email := r.PostForm.Get("email")
	password := r.PostForm.Get("password")

	user, plainKey, err := h.svc.Signup(r.Context(), username, email, password)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = h.tmpl.Render(w, r, "signup", signupFormData{
			Username: username,
			Email:    email,
			Error:    err.Error(),
		})
		return
	}

	token, err := h.svc.CreateSession(r.Context(), user.ID)
	if err != nil {
		slog.Error("create session after signup", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, token, h.secureCookies, h.sessionTTL)

	_ = h.tmpl.Render(w, r, "key_shown", map[string]any{
		"Heading": "Welcome to chtiwt",
		"Lead":    "This is your stream key. Copy it now — for security it won't be shown again.",
		"Plain":   plainKey,
		"CTA":     "Continue to dashboard",
		"CTAHref": "/dashboard",
	})
}

type loginFormData struct {
	Username string
	Error    string
}

func (h *Handlers) loginForm(w http.ResponseWriter, r *http.Request) {
	_ = h.tmpl.Render(w, r, "login", loginFormData{})
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	username := r.PostForm.Get("username")
	password := r.PostForm.Get("password")

	user, err := h.svc.Login(r.Context(), username, password)
	if err != nil {
		msg := err.Error()
		if !errors.Is(err, ErrBadCredentials) {
			slog.Warn("login failed", "username", username, "err", err)
			msg = "login failed"
		}
		w.WriteHeader(http.StatusUnauthorized)
		_ = h.tmpl.Render(w, r, "login", loginFormData{
			Username: username,
			Error:    msg,
		})
		return
	}

	token, err := h.svc.CreateSession(r.Context(), user.ID)
	if err != nil {
		slog.Error("create session after login", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, token, h.secureCookies, h.sessionTTL)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookieName); err == nil {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			slog.Warn("logout delete session", "err", err)
		}
	}
	clearSessionCookie(w, h.secureCookies)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
