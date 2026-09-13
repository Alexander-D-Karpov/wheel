package server

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) apiCreatePlay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EntryIDs    []string `json:"entry_ids"`
		Mode        string   `json:"mode"`
		SpinSeconds *int     `json:"spin_seconds"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}

	wheelID, session := r.PathValue("id"), sessionID(r)
	wheel, err := s.store.GetWheel(r.Context(), wheelID, session)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	// The wheel's saved settings are the default; the start form may override them.
	mode := wheel.Mode
	if body.Mode != "" {
		mode = cleanMode(body.Mode)
	}
	seconds := wheel.SpinSeconds
	if body.SpinSeconds != nil {
		seconds = s.cfg.ClampSeconds(*body.SpinSeconds)
	}

	slug, err := s.store.CreatePlay(r.Context(), session, wheelID, body.EntryIDs, mode, seconds)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.log.Info("play session created", "play", slug, "wheel", wheelID, "mode", mode, "spin_seconds", seconds)
	writeJSON(w, http.StatusCreated, map[string]string{
		"slug": slug,
		"path": "/p/" + slug,
		"url":  s.publicURL(r, "/p/"+slug),
	})
}

func (s *Server) apiGetPlay(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.PlayState(r.Context(), r.PathValue("slug"), sessionID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) apiSpin(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	spin, err := s.store.StartSpin(r.Context(), slug, sessionID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}

	// Everyone watching, including the caller, learns about the spin through the
	// event stream, so all browsers start from the same message.
	s.hub.Publish(slug, "spin", map[string]any{
		"spin":       spin,
		"round":      spin.Round,
		"server_now": time.Now().UnixMilli(),
	})
	s.log.Info("spin started", "play", slug, "round", spin.Round, "duration_ms", spin.DurationMs)

	writeJSON(w, http.StatusAccepted, map[string]any{"spin": spin, "server_now": time.Now().UnixMilli()})
}

func (s *Server) apiResetPlay(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	state, err := s.store.ResetPlay(r.Context(), slug, sessionID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}

	// Spectators get the same state, minus the owner flag.
	broadcast := *state
	broadcast.IsOwner = false
	s.hub.Publish(slug, "state", map[string]any{"state": &broadcast})
	s.log.Info("play session reset", "play", slug)

	writeJSON(w, http.StatusOK, state)
}

// publicURL builds an absolute link for sharing. BASE_URL wins when set,
// otherwise the request's own host is used, honouring the proxy headers nginx
// sends in the bundled config.
func (s *Server) publicURL(r *http.Request, path string) string {
	if s.cfg.BaseURL != "" {
		return s.cfg.BaseURL + path
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.TrimSpace(strings.Split(proto, ",")[0])
	}

	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	return scheme + "://" + host + path
}
