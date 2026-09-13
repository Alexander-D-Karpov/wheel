package server

import (
	"net/http"
	"runtime/debug"
	"time"
)

// statusRecorder remembers the response code for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusRecorder) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	if !w.wrote {
		w.status = http.StatusOK
		w.wrote = true
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.NewResponseController reach the real writer, which the SSE
// handler needs in order to flush.
func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		level := s.log.Debug
		if rec.status >= 500 {
			level = s.log.Error
		} else if rec.status >= 400 {
			level = s.log.Warn
		}
		level("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).Round(time.Millisecond).String(),
		)
	})
}

func (s *Server) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			// A client that walks away mid-write is normal, not a crash.
			if p == http.ErrAbortHandler {
				panic(p)
			}
			s.log.Error("panic serving request",
				"path", r.URL.Path, "panic", p, "stack", string(debug.Stack()))
			if isAPIPath(r.URL.Path) {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}
