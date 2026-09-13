package server

import (
	"net/http"

	"github.com/Alexander-D-Karpov/wheel/internal/store"
)

func (s *Server) apiListWheels(w http.ResponseWriter, r *http.Request) {
	wheels, err := s.store.ListWheels(r.Context(), sessionID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wheels)
}

func (s *Server) apiCreateWheel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string `json:"title"`
		Mode        string `json:"mode"`
		SpinSeconds *int   `json:"spin_seconds"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}

	seconds := s.cfg.DefaultSpinSeconds
	if body.SpinSeconds != nil {
		seconds = s.cfg.ClampSeconds(*body.SpinSeconds)
	}

	wheel, err := s.store.CreateWheel(r.Context(), sessionID(r), cleanTitle(body.Title), cleanMode(body.Mode), seconds)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, wheel)
}

func (s *Server) apiGetWheel(w http.ResponseWriter, r *http.Request) {
	wheel, err := s.store.GetWheel(r.Context(), r.PathValue("id"), sessionID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wheel)
}

func (s *Server) apiUpdateWheel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       *string `json:"title"`
		Mode        *string `json:"mode"`
		SpinSeconds *int    `json:"spin_seconds"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}

	patch := store.WheelPatch{}
	if body.Title != nil {
		title := cleanTitle(*body.Title)
		patch.Title = &title
	}
	if body.Mode != nil {
		mode := cleanMode(*body.Mode)
		patch.Mode = &mode
	}
	if body.SpinSeconds != nil {
		seconds := s.cfg.ClampSeconds(*body.SpinSeconds)
		patch.SpinSeconds = &seconds
	}
	if patch.Title == nil && patch.Mode == nil && patch.SpinSeconds == nil {
		s.fail(w, r, errBadRequest("nothing to update"))
		return
	}

	wheel, err := s.store.UpdateWheel(r.Context(), r.PathValue("id"), sessionID(r), patch)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wheel)
}

func (s *Server) apiDeleteWheel(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteWheel(r.Context(), r.PathValue("id"), sessionID(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiAddEntry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label  string   `json:"label"`
		Weight *float64 `json:"weight"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}

	label := cleanLabel(body.Label)
	if label == "" {
		s.fail(w, r, errBadRequest("an entry needs a name"))
		return
	}
	weight := 1.0
	if body.Weight != nil {
		weight = clampWeight(*body.Weight)
	}

	added, err := s.store.AddEntries(r.Context(), r.PathValue("id"), sessionID(r),
		[]store.Entry{{Label: label, Weight: weight}}, s.cfg.MaxEntries)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, added[0])
}

func (s *Server) apiAddEntriesBulk(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}

	parsed := parseBulk(body.Text)
	if len(parsed) == 0 {
		s.fail(w, r, errBadRequest("no entries were found in that text"))
		return
	}

	added, err := s.store.AddEntries(r.Context(), r.PathValue("id"), sessionID(r), parsed, s.cfg.MaxEntries)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, added)
}

func (s *Server) apiUpdateEntry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label  *string  `json:"label"`
		Weight *float64 `json:"weight"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}

	patch := store.EntryPatch{}
	if body.Label != nil {
		label := cleanLabel(*body.Label)
		if label == "" {
			s.fail(w, r, errBadRequest("an entry needs a name"))
			return
		}
		patch.Label = &label
	}
	if body.Weight != nil {
		weight := clampWeight(*body.Weight)
		patch.Weight = &weight
	}
	if patch.Label == nil && patch.Weight == nil {
		s.fail(w, r, errBadRequest("nothing to update"))
		return
	}

	entry, err := s.store.UpdateEntry(r.Context(), r.PathValue("id"), sessionID(r), patch)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) apiDeleteEntry(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteEntry(r.Context(), r.PathValue("id"), sessionID(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
