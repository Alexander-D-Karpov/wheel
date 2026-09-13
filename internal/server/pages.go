package server

import (
	"bytes"
	"net/http"

	"github.com/Alexander-D-Karpov/wheel/web"
)

// pageData is the shape every template receives.
type pageData struct {
	Title              string
	AssetVersion       string
	WheelID            string
	Slug               string
	BaseURL            string
	DefaultSpinSeconds int
	MinSpinSeconds     int
	MaxSpinSeconds     int
	MaxEntries         int
	Message            string
}

func (s *Server) newPageData(title string) pageData {
	return pageData{
		Title:              title,
		AssetVersion:       web.AssetVersion,
		BaseURL:            s.cfg.BaseURL,
		DefaultSpinSeconds: s.cfg.DefaultSpinSeconds,
		MinSpinSeconds:     s.cfg.MinSpinSeconds,
		MaxSpinSeconds:     s.cfg.MaxSpinSeconds,
		MaxEntries:         s.cfg.MaxEntries,
	}
}

func (s *Server) pageIndex(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, http.StatusOK, "index.html", s.newPageData("AWheel"))
}

func (s *Server) pageWheel(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData("AWheel: edit wheel")
	data.WheelID = r.PathValue("id")
	s.renderPage(w, r, http.StatusOK, "wheel.html", data)
}

func (s *Server) pagePlay(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData("AWheel: live")
	data.Slug = r.PathValue("slug")
	s.renderPage(w, r, http.StatusOK, "play.html", data)
}

// renderPage buffers the template so a mid-render failure cannot leave a
// half-written page with a 200 status on the wire.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, status int, name string, data pageData) {
	if data.AssetVersion == "" {
		data.AssetVersion = web.AssetVersion
	}

	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.log.Error("render template", "template", name, "path", r.URL.Path, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
