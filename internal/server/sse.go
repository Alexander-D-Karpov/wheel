package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// heartbeat keeps proxies from closing an idle stream and gives the browser a
// fresh server timestamp to re-check its clock against.
const heartbeat = 15 * time.Second

// apiPlayEvents streams the live state of a play session. Spectators are
// read-only: the stream is the only thing they get.
func (s *Server) apiPlayEvents(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	// Fetch the opening state before committing to a 200, so a bad slug still
	// produces an ordinary JSON error instead of an empty stream.
	state, err := s.store.PlayState(r.Context(), slug, sessionID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}

	// SSE connections are long-lived, so the server-wide read and write
	// deadlines have to be lifted for this one request.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		s.log.Debug("clear write deadline", "err", err)
	}
	if err := rc.SetReadDeadline(time.Time{}); err != nil {
		s.log.Debug("clear read deadline", "err", err)
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // nginx: do not buffer this response
	w.WriteHeader(http.StatusOK)

	listener, count := s.hub.Subscribe(slug)
	if count == 0 {
		return // the hub is shutting down
	}
	defer func() {
		remaining := s.hub.Unsubscribe(slug, listener)
		if remaining > 0 {
			s.hub.Publish(slug, "viewers", map[string]int{"count": remaining})
		}
	}()

	send := func(event string, payload any) bool {
		body, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		frame := "event: " + event + "\ndata: " + string(body) + "\n\n"
		if _, err := w.Write([]byte(frame)); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	// retry tells the browser how long to wait before reconnecting; EventSource
	// handles the reconnect itself.
	if _, err := w.Write([]byte("retry: 3000\n\n")); err != nil {
		return
	}
	if !send("state", map[string]any{"state": state}) {
		return
	}
	s.hub.Publish(slug, "viewers", map[string]int{"count": count})

	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case frame, open := <-listener.events:
			if !open {
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}

		case <-ticker.C:
			if !send("ping", map[string]int64{"server_now": time.Now().UnixMilli()}) {
				return
			}
		}
	}
}
