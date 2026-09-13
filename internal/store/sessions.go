package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Alexander-D-Karpov/wheel/internal/idgen"
)

// EnsureSession validates an incoming cookie value and returns the session to
// use. The second result reports whether a new session was created, which tells
// the HTTP layer that it has to set a cookie.
func (s *Store) EnsureSession(ctx context.Context, candidate string) (string, bool, error) {
	if len(candidate) == 64 {
		res, err := s.db.ExecContext(ctx,
			`UPDATE sessions SET seen_at = now() WHERE id = $1`, candidate)
		if err != nil {
			return "", false, err
		}
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			return candidate, false, nil
		}
	}

	id := idgen.Token()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO sessions (id) VALUES ($1)`, id); err != nil {
		return "", false, err
	}
	return id, true, nil
}

// PurgeStale removes sessions that have not been seen for the retention window
// (cascading to their wheels) and play sessions nobody has touched. A zero or
// negative day count disables the corresponding sweep.
func (s *Store) PurgeStale(ctx context.Context, sessionDays, playDays int) (sessions, plays int64, err error) {
	if sessionDays > 0 {
		res, err := s.db.ExecContext(ctx, `
			DELETE FROM sessions
			WHERE seen_at < now() - make_interval(days => $1)`, sessionDays)
		if err != nil {
			return 0, 0, err
		}
		sessions, _ = res.RowsAffected()
	}
	if playDays > 0 {
		res, err := s.db.ExecContext(ctx, `
			DELETE FROM plays
			WHERE updated_at < now() - make_interval(days => $1)`, playDays)
		if err != nil {
			return sessions, 0, err
		}
		plays, _ = res.RowsAffected()
	}
	return sessions, plays, nil
}

func noRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
