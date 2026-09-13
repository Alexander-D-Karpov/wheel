package server

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/Alexander-D-Karpov/wheel/internal/config"
	"github.com/Alexander-D-Karpov/wheel/internal/store"
	"github.com/Alexander-D-Karpov/wheel/web"
)

// resolveInterval is how often the server checks for spins that have come to a
// stop. It is short because it decides when the result is announced.
const resolveInterval = 150 * time.Millisecond

// Server wires configuration, storage and the live-event hub into HTTP.
type Server struct {
	cfg   *config.Config
	store *store.Store
	hub   *Hub
	tpl   *template.Template
	log   *slog.Logger
}

func New(cfg *config.Config, st *store.Store, log *slog.Logger) (*Server, error) {
	tpl, err := template.ParseFS(web.Templates, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, store: st, hub: NewHub(), tpl: tpl, log: log}, nil
}

// Hub exposes the event hub so main can release listeners during shutdown.
func (s *Server) Hub() *Hub { return s.hub }

// Handler builds the routing tree.
func (s *Server) Handler() http.Handler {
	// Application routes all need a session; static files and the health probe
	// deliberately do not, so they never touch the database.
	app := http.NewServeMux()

	app.HandleFunc("GET /{$}", s.pageIndex)
	app.HandleFunc("GET /wheel/{id}", s.pageWheel)
	app.HandleFunc("GET /p/{slug}", s.pagePlay)

	app.HandleFunc("GET /api/time", s.apiTime)
	app.HandleFunc("GET /api/wheels", s.apiListWheels)
	app.HandleFunc("POST /api/wheels", s.apiCreateWheel)
	app.HandleFunc("GET /api/wheels/{id}", s.apiGetWheel)
	app.HandleFunc("PATCH /api/wheels/{id}", s.apiUpdateWheel)
	app.HandleFunc("DELETE /api/wheels/{id}", s.apiDeleteWheel)
	app.HandleFunc("POST /api/wheels/{id}/entries", s.apiAddEntry)
	app.HandleFunc("POST /api/wheels/{id}/entries/bulk", s.apiAddEntriesBulk)
	app.HandleFunc("PATCH /api/entries/{id}", s.apiUpdateEntry)
	app.HandleFunc("DELETE /api/entries/{id}", s.apiDeleteEntry)
	app.HandleFunc("POST /api/wheels/{id}/plays", s.apiCreatePlay)

	app.HandleFunc("GET /api/plays/{slug}", s.apiGetPlay)
	app.HandleFunc("GET /api/plays/{slug}/events", s.apiPlayEvents)
	app.HandleFunc("POST /api/plays/{slug}/spin", s.apiSpin)
	app.HandleFunc("POST /api/plays/{slug}/reset", s.apiResetPlay)

	app.HandleFunc("/", s.notFound)

	root := http.NewServeMux()
	root.Handle("GET /static/", staticCache(http.StripPrefix("/static/", http.FileServerFS(web.Static))))
	root.HandleFunc("GET /healthz", s.health)
	root.Handle("/", s.withSession(app))

	return s.withRecover(s.withRequestLog(root))
}

// StartBackground launches the spin resolver and the retention sweep. Both stop
// with ctx.
func (s *Server) StartBackground(ctx context.Context) {
	go s.runResolver(ctx)
	go s.runCleanup(ctx)
}

// runResolver announces results for spins whose animation has finished.
func (s *Server) runResolver(ctx context.Context) {
	ticker := time.NewTicker(resolveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			results, err := s.store.ResolveDue(ctx, 100)
			if err != nil && !errors.Is(err, context.Canceled) {
				s.log.Error("resolve spins", "err", err)
			}
			for _, r := range results {
				s.hub.Publish(r.Slug, "result", map[string]any{
					"state":        r.State,
					"round":        r.Round,
					"picked_id":    r.PickedID,
					"picked_label": r.PickedLabel,
					"eliminated":   r.Eliminated,
					"finished":     r.Finished,
					"server_now":   time.Now().UnixMilli(),
				})
				s.log.Info("spin resolved",
					"play", r.Slug, "round", r.Round, "picked", r.PickedLabel, "finished", r.Finished)
			}
		}
	}
}

// runCleanup drops abandoned sessions and stale play sessions once a day.
func (s *Server) runCleanup(ctx context.Context) {
	if s.cfg.PlayRetentionDays <= 0 && s.cfg.SessionDays <= 0 {
		return
	}
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		sessions, plays, err := s.store.PurgeStale(ctx, s.cfg.SessionDays, s.cfg.PlayRetentionDays)
		switch {
		case err != nil && !errors.Is(err, context.Canceled):
			s.log.Error("purge stale rows", "err", err)
		case sessions > 0 || plays > 0:
			s.log.Info("purged stale rows", "sessions", sessions, "plays", plays)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) apiTime(w http.ResponseWriter, r *http.Request) {
	// The browser calls this a few times to work out its offset from the server
	// clock, which is what keeps every spectator's animation in step.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]int64{"now": time.Now().UnixMilli()})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	if isAPIPath(r.URL.Path) {
		writeError(w, http.StatusNotFound, "no such endpoint")
		return
	}
	s.renderPage(w, r, http.StatusNotFound, "error.html", pageData{
		Title:   "Not found",
		Message: "There is nothing at this address.",
	})
}

func staticCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			// The URL carries a content digest, so this exact body is permanent.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		next.ServeHTTP(w, r)
	})
}

func isAPIPath(p string) bool {
	return len(p) >= 5 && p[:5] == "/api/"
}
