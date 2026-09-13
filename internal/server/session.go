package server

import (
	"context"
	"net/http"
	"time"
)

type contextKey int

const sessionContextKey contextKey = iota

// withSession attaches an anonymous identity to every application request,
// creating one on first visit. There is no login: the cookie is the account.
func (s *Server) withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		candidate := ""
		if c, err := r.Cookie(s.cfg.CookieName); err == nil {
			candidate = c.Value
		}

		id, created, err := s.store.EnsureSession(r.Context(), candidate)
		if err != nil {
			s.log.Error("ensure session", "err", err)
			if isAPIPath(r.URL.Path) {
				writeError(w, http.StatusServiceUnavailable, "the service is not reachable right now")
			} else {
				http.Error(w, "the service is not reachable right now", http.StatusServiceUnavailable)
			}
			return
		}

		if created {
			http.SetCookie(w, &http.Cookie{
				Name:     s.cfg.CookieName,
				Value:    id,
				Path:     "/",
				Expires:  time.Now().Add(time.Duration(s.cfg.SessionDays) * 24 * time.Hour),
				MaxAge:   s.cfg.SessionDays * 24 * 60 * 60,
				HttpOnly: true,
				Secure:   s.cfg.CookieSecure,
				SameSite: http.SameSiteLaxMode,
			})
		}

		ctx := context.WithValue(r.Context(), sessionContextKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sessionID returns the caller's identity, or "" outside the session middleware.
func sessionID(r *http.Request) string {
	id, _ := r.Context().Value(sessionContextKey).(string)
	return id
}
