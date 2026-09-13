package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Alexander-D-Karpov/wheel/internal/store"
)

// maxRequestBody caps request bodies; the largest legitimate one is a bulk
// paste of entries.
const maxRequestBody = 1 << 20

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// decodeJSON reads a JSON request body. An empty body is accepted and leaves
// the target untouched, so endpoints with all-optional fields work without one.
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errBadRequest("the request body is too large")
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return errBadRequest("field " + typeErr.Field + " has the wrong type")
		}
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return errBadRequest(strings.TrimPrefix(err.Error(), "json: "))
		}
		return errBadRequest("the request body is not valid JSON")
	}

	// Reject trailing content so a typo cannot be silently ignored.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errBadRequest("the request body must hold a single JSON object")
	}
	return nil
}

// badRequest carries a message that is safe to show the user.
type badRequest struct{ msg string }

func (e badRequest) Error() string { return e.msg }

func errBadRequest(msg string) error { return badRequest{msg: msg} }

// fail turns a store or validation error into the right HTTP response. Anything
// unrecognised is logged and reported as a generic 500, so internal details
// never reach the browser.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var bad badRequest
	switch {
	case errors.As(err, &bad):
		writeError(w, http.StatusBadRequest, bad.msg)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrInvalid):
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "invalid request: "))
	case errors.Is(err, store.ErrNotOwner):
		writeError(w, http.StatusForbidden, "only the creator of this play session can do that")
	case errors.Is(err, store.ErrBusy):
		writeError(w, http.StatusConflict, "a spin is already running")
	case errors.Is(err, store.ErrFinished):
		writeError(w, http.StatusConflict, "this play session is finished")
	case errors.Is(err, store.ErrNotEnoughEntries):
		writeError(w, http.StatusConflict, "at least two entries must still be in play")
	default:
		s.log.Error("request failed", "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
