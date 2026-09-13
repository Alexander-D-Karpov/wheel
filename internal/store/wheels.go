package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Alexander-D-Karpov/wheel/internal/idgen"
)

// ListWheels returns the caller's wheels, newest first, without their entries.
func (s *Store) ListWheels(ctx context.Context, sessionID string) ([]Wheel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, w.title, w.mode, w.spin_seconds, w.created_at,
		       (SELECT count(*) FROM entries e WHERE e.wheel_id = w.id)
		FROM wheels w
		WHERE w.owner_session = $1
		ORDER BY w.created_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Wheel{}
	for rows.Next() {
		var w Wheel
		if err := rows.Scan(&w.ID, &w.Title, &w.Mode, &w.SpinSeconds, &w.CreatedAt, &w.EntryCount); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// CreateWheel inserts an empty wheel owned by the session.
func (s *Store) CreateWheel(ctx context.Context, sessionID, title, mode string, spinSeconds int) (*Wheel, error) {
	id := idgen.UUID()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO wheels (id, owner_session, title, mode, spin_seconds)
		VALUES ($1, $2, $3, $4, $5)`, id, sessionID, title, mode, spinSeconds)
	if err != nil {
		return nil, err
	}
	return s.GetWheel(ctx, id, sessionID)
}

// GetWheel loads one wheel with its entries. Wheels owned by another session
// report ErrNotFound.
func (s *Store) GetWheel(ctx context.Context, id, sessionID string) (*Wheel, error) {
	var w Wheel
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, mode, spin_seconds, created_at
		FROM wheels WHERE id = $1 AND owner_session = $2`, id, sessionID).
		Scan(&w.ID, &w.Title, &w.Mode, &w.SpinSeconds, &w.CreatedAt)
	if noRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	w.Entries, err = listEntries(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	w.EntryCount = len(w.Entries)
	return &w, nil
}

// WheelPatch carries the fields an update may change; nil means "leave alone".
type WheelPatch struct {
	Title       *string
	Mode        *string
	SpinSeconds *int
}

// UpdateWheel applies a partial update and returns the stored result.
func (s *Store) UpdateWheel(ctx context.Context, id, sessionID string, p WheelPatch) (*Wheel, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE wheels
		SET title        = COALESCE($1::text, title),
		    mode         = COALESCE($2::text, mode),
		    spin_seconds = COALESCE($3::int, spin_seconds),
		    updated_at   = now()
		WHERE id = $4 AND owner_session = $5`,
		nullString(p.Title), nullString(p.Mode), nullInt(p.SpinSeconds), id, sessionID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetWheel(ctx, id, sessionID)
}

// DeleteWheel removes a wheel and, by cascade, its entries.
func (s *Store) DeleteWheel(ctx context.Context, id, sessionID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM wheels WHERE id = $1 AND owner_session = $2`, id, sessionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AddEntries appends entries to a wheel, refusing to exceed maxEntries.
func (s *Store) AddEntries(ctx context.Context, wheelID, sessionID string, in []Entry, maxEntries int) ([]Entry, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("%w: no entries given", ErrInvalid)
	}

	var out []Entry
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		owned, err := ownsWheel(ctx, tx, wheelID, sessionID)
		if err != nil {
			return err
		}
		if !owned {
			return ErrNotFound
		}

		var count, nextPos int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*), COALESCE(max(position) + 1, 0)
			FROM entries WHERE wheel_id = $1`, wheelID).Scan(&count, &nextPos); err != nil {
			return err
		}
		if count+len(in) > maxEntries {
			return fmt.Errorf("%w: a wheel holds at most %d entries", ErrInvalid, maxEntries)
		}

		out = make([]Entry, 0, len(in))
		for i, e := range in {
			e.ID = idgen.UUID()
			e.Position = nextPos + i
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO entries (id, wheel_id, label, weight, position)
				VALUES ($1, $2, $3, $4, $5)`, e.ID, wheelID, e.Label, e.Weight, e.Position); err != nil {
				return err
			}
			out = append(out, e)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// EntryPatch carries the fields an entry update may change.
type EntryPatch struct {
	Label  *string
	Weight *float64
}

// UpdateEntry applies a partial update to a single entry the caller owns.
func (s *Store) UpdateEntry(ctx context.Context, entryID, sessionID string, p EntryPatch) (*Entry, error) {
	var e Entry
	err := s.db.QueryRowContext(ctx, `
		UPDATE entries e
		SET label  = COALESCE($1::text, e.label),
		    weight = COALESCE($2::double precision, e.weight)
		FROM wheels w
		WHERE e.id = $3 AND w.id = e.wheel_id AND w.owner_session = $4
		RETURNING e.id, e.label, e.weight, e.position`,
		nullString(p.Label), nullFloat(p.Weight), entryID, sessionID).
		Scan(&e.ID, &e.Label, &e.Weight, &e.Position)
	if noRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// DeleteEntry removes one entry the caller owns.
func (s *Store) DeleteEntry(ctx context.Context, entryID, sessionID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM entries e
		USING wheels w
		WHERE e.id = $1 AND w.id = e.wheel_id AND w.owner_session = $2`, entryID, sessionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func listEntries(ctx context.Context, q querier, wheelID string) ([]Entry, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, label, weight, position
		FROM entries WHERE wheel_id = $1 ORDER BY position, id`, wheelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Label, &e.Weight, &e.Position); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func ownsWheel(ctx context.Context, q querier, wheelID, sessionID string) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx,
		`SELECT true FROM wheels WHERE id = $1 AND owner_session = $2`, wheelID, sessionID).Scan(&ok)
	if noRows(err) {
		return false, nil
	}
	return ok, err
}

func nullString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
