package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Alexander-D-Karpov/wheel/internal/idgen"
	"github.com/lib/pq"
)

// ResolveLag is the grace period added to a spin's duration before the server
// announces the result, so that a browser a little behind the clock still sees
// the wheel come to a stop before the winner is named.
const ResolveLag = 200 * time.Millisecond

const playColumns = `id, slug, owner_session, title, mode, spin_seconds,
	status, round, rotation, current_spin_id, last_winner_id`

// CreatePlay freezes the chosen entries into a new, shareable play session.
// Later edits to the wheel do not affect it.
func (s *Store) CreatePlay(ctx context.Context, sessionID, wheelID string, entryIDs []string, mode string, spinSeconds int) (string, error) {
	wheel, err := s.GetWheel(ctx, wheelID, sessionID)
	if err != nil {
		return "", err
	}

	wanted := make(map[string]bool, len(entryIDs))
	for _, id := range entryIDs {
		wanted[id] = true
	}
	picked := make([]Entry, 0, len(wheel.Entries))
	for _, e := range wheel.Entries {
		// An empty selection means "use the whole wheel".
		if len(wanted) == 0 || wanted[e.ID] {
			picked = append(picked, e)
		}
	}
	if len(picked) < 2 {
		return "", fmt.Errorf("%w: pick at least two entries to start a play session", ErrInvalid)
	}

	// A slug collision is vanishingly unlikely, but a failed insert poisons the
	// whole transaction in Postgres, so the retry restarts it from scratch.
	for attempt := 0; ; attempt++ {
		slug := idgen.Slug(9)
		playID := idgen.UUID()

		err := s.inTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO plays (id, slug, wheel_id, owner_session, title, mode, spin_seconds)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				playID, slug, wheelID, sessionID, wheel.Title, mode, spinSeconds); err != nil {
				return err
			}
			for i, e := range picked {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO play_entries (id, play_id, label, weight, position)
					VALUES ($1, $2, $3, $4, $5)`,
					idgen.UUID(), playID, e.Label, e.Weight, i); err != nil {
					return err
				}
			}
			return nil
		})
		if err == nil {
			return slug, nil
		}

		var pgErr *pq.Error
		if attempt < 4 && errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return "", err
	}
}

// PlayState returns the full public view of a play session. sessionID may be
// empty, in which case IsOwner is false.
func (s *Store) PlayState(ctx context.Context, slug, sessionID string) (*PlayState, error) {
	p, err := loadPlay(ctx, s.db, `WHERE slug = $1`, slug)
	if err != nil {
		return nil, err
	}
	return buildState(ctx, s.db, p, sessionID)
}

// StartSpin picks the winner, works out the rotation and stores it. Only the
// creator may call it, and only when no spin is in flight.
func (s *Store) StartSpin(ctx context.Context, slug, sessionID string) (*SpinView, error) {
	var view *SpinView

	err := s.inTx(ctx, func(tx *sql.Tx) error {
		p, err := loadPlay(ctx, tx, `WHERE slug = $1 FOR UPDATE`, slug)
		if err != nil {
			return err
		}
		if p.OwnerSession != sessionID {
			return ErrNotOwner
		}
		switch p.Status {
		case StatusSpinning:
			return ErrBusy
		case StatusFinished:
			return ErrFinished
		}

		active, err := listPlayEntries(ctx, tx, p.ID, true)
		if err != nil {
			return err
		}
		if len(active) < 2 {
			return ErrNotEnoughEntries
		}

		index := pickWinner(active)
		winner := active[index]
		delta := spinDelta(active, index, p.Rotation, p.SpinSeconds)

		layout, err := json.Marshal(active)
		if err != nil {
			return err
		}

		spinID := idgen.UUID()
		round := p.Round + 1
		durationMs := p.SpinSeconds * 1000
		startedAt := time.Now().UTC()

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO spins (id, play_id, round, winner_id, winner_label,
			                   rot_from, rot_delta, duration_ms, started_at, layout)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			spinID, p.ID, round, winner.ID, winner.Label,
			p.Rotation, delta, durationMs, startedAt, layout); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE plays SET status = $1, round = $2, current_spin_id = $3, updated_at = now()
			WHERE id = $4`, StatusSpinning, round, spinID, p.ID); err != nil {
			return err
		}

		view = &SpinView{
			ID:         spinID,
			Round:      round,
			From:       p.Rotation,
			Delta:      delta,
			StartedAt:  startedAt.UnixMilli(),
			DurationMs: durationMs,
			Layout:     active,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// ResolveDue finishes every spin whose animation has run its course. It is
// driven by a ticker, so a restart mid-spin still settles correctly.
func (s *Store) ResolveDue(ctx context.Context, limit int) ([]SpinResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id
		FROM plays p
		JOIN spins s ON s.id = p.current_spin_id
		WHERE p.status = $1
		  AND s.resolved = false
		  AND s.started_at + make_interval(secs => (s.duration_ms + $2) / 1000.0) <= now()
		ORDER BY s.started_at
		LIMIT $3`, StatusSpinning, ResolveLag.Milliseconds(), limit)
	if err != nil {
		return nil, err
	}

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]SpinResult, 0, len(ids))
	for _, id := range ids {
		res, err := s.resolvePlay(ctx, id)
		if err != nil {
			return out, fmt.Errorf("resolve play %s: %w", id, err)
		}
		if res != nil {
			out = append(out, *res)
		}
	}
	return out, nil
}

// resolvePlay settles one finished spin: it fixes the resting rotation, applies
// the mode's consequence and works out whether the session is over.
func (s *Store) resolvePlay(ctx context.Context, playID string) (*SpinResult, error) {
	var result *SpinResult

	err := s.inTx(ctx, func(tx *sql.Tx) error {
		p, err := loadPlay(ctx, tx, `WHERE id = $1 FOR UPDATE`, playID)
		if err != nil {
			return err
		}
		// Another worker got here first.
		if p.Status != StatusSpinning || p.CurrentSpinID == "" {
			return nil
		}

		var (
			round       int
			winnerID    string
			winnerLabel string
			rotFrom     float64
			rotDelta    float64
		)
		if err := tx.QueryRowContext(ctx, `
			SELECT round, winner_id, winner_label, rot_from, rot_delta
			FROM spins WHERE id = $1 AND resolved = false FOR UPDATE`, p.CurrentSpinID).
			Scan(&round, &winnerID, &winnerLabel, &rotFrom, &rotDelta); err != nil {
			if noRows(err) {
				return nil
			}
			return err
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE spins SET resolved = true WHERE id = $1`, p.CurrentSpinID); err != nil {
			return err
		}

		status := StatusIdle
		finalWinner := winnerID
		eliminated := false

		if p.Mode == ModeExclusion {
			if _, err := tx.ExecContext(ctx,
				`UPDATE play_entries SET active = false WHERE id = $1 AND play_id = $2`,
				winnerID, p.ID); err != nil {
				return err
			}
			eliminated = true

			var survivor sql.NullString
			var remaining int
			if err := tx.QueryRowContext(ctx, `
				SELECT count(*), min(id)
				FROM play_entries WHERE play_id = $1 AND active = true`, p.ID).
				Scan(&remaining, &survivor); err != nil {
				return err
			}
			if remaining <= 1 {
				status = StatusFinished
				if survivor.Valid {
					finalWinner = survivor.String
				}
			}
		}

		rotation := normalizeRotation(rotFrom + rotDelta)
		if _, err := tx.ExecContext(ctx, `
			UPDATE plays
			SET status = $1, rotation = $2, current_spin_id = NULL,
			    last_winner_id = $3, updated_at = now()
			WHERE id = $4`, status, rotation, finalWinner, p.ID); err != nil {
			return err
		}

		p.Status = status
		p.Rotation = rotation
		p.CurrentSpinID = ""
		p.LastWinnerID = finalWinner

		state, err := buildState(ctx, tx, p, "")
		if err != nil {
			return err
		}
		result = &SpinResult{
			Slug:        p.Slug,
			Round:       round,
			PickedID:    winnerID,
			PickedLabel: winnerLabel,
			Eliminated:  eliminated,
			Finished:    status == StatusFinished,
			State:       state,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ResetPlay puts every entry back in play and clears the history.
func (s *Store) ResetPlay(ctx context.Context, slug, sessionID string) (*PlayState, error) {
	var state *PlayState

	err := s.inTx(ctx, func(tx *sql.Tx) error {
		p, err := loadPlay(ctx, tx, `WHERE slug = $1 FOR UPDATE`, slug)
		if err != nil {
			return err
		}
		if p.OwnerSession != sessionID {
			return ErrNotOwner
		}
		if p.Status == StatusSpinning {
			return ErrBusy
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE plays
			SET status = $1, round = 0, rotation = 0, current_spin_id = NULL,
			    last_winner_id = NULL, updated_at = now()
			WHERE id = $2`, StatusIdle, p.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM spins WHERE play_id = $1`, p.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE play_entries SET active = true WHERE play_id = $1`, p.ID); err != nil {
			return err
		}

		p.Status, p.Round, p.Rotation = StatusIdle, 0, 0
		p.CurrentSpinID, p.LastWinnerID = "", ""

		state, err = buildState(ctx, tx, p, sessionID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}

func loadPlay(ctx context.Context, q querier, where string, args ...any) (*play, error) {
	var (
		p        play
		spinID   sql.NullString
		winnerID sql.NullString
	)
	err := q.QueryRowContext(ctx, `SELECT `+playColumns+` FROM plays `+where, args...).
		Scan(&p.ID, &p.Slug, &p.OwnerSession, &p.Title, &p.Mode, &p.SpinSeconds,
			&p.Status, &p.Round, &p.Rotation, &spinID, &winnerID)
	if noRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.CurrentSpinID = spinID.String
	p.LastWinnerID = winnerID.String
	return &p, nil
}

func buildState(ctx context.Context, q querier, p *play, sessionID string) (*PlayState, error) {
	all, err := listPlayEntries(ctx, q, p.ID, false)
	if err != nil {
		return nil, err
	}
	history, err := listHistory(ctx, q, p.ID)
	if err != nil {
		return nil, err
	}

	state := &PlayState{
		Slug:        p.Slug,
		Title:       p.Title,
		Mode:        p.Mode,
		SpinSeconds: p.SpinSeconds,
		Status:      p.Status,
		Round:       p.Round,
		Rotation:    p.Rotation,
		Entries:     all,
		History:     history,
		IsOwner:     sessionID != "" && sessionID == p.OwnerSession,
		ServerNow:   time.Now().UnixMilli(),
	}

	if p.Status == StatusSpinning && p.CurrentSpinID != "" {
		view, err := loadSpinView(ctx, q, p.CurrentSpinID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		state.Spin = view
	}

	if p.LastWinnerID != "" {
		for i := range all {
			if all[i].ID == p.LastWinnerID {
				winner := all[i]
				state.Winner = &winner
				break
			}
		}
	}
	return state, nil
}

func listPlayEntries(ctx context.Context, q querier, playID string, onlyActive bool) ([]PlayEntry, error) {
	query := `SELECT id, label, weight, position, active FROM play_entries WHERE play_id = $1`
	if onlyActive {
		query += ` AND active = true`
	}
	query += ` ORDER BY position, id`

	rows, err := q.QueryContext(ctx, query, playID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PlayEntry{}
	for rows.Next() {
		var e PlayEntry
		if err := rows.Scan(&e.ID, &e.Label, &e.Weight, &e.Position, &e.Active); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func loadSpinView(ctx context.Context, q querier, spinID string) (*SpinView, error) {
	var (
		view      SpinView
		layout    []byte
		startedAt time.Time
	)
	err := q.QueryRowContext(ctx, `
		SELECT id, round, rot_from, rot_delta, duration_ms, started_at, layout
		FROM spins WHERE id = $1`, spinID).
		Scan(&view.ID, &view.Round, &view.From, &view.Delta, &view.DurationMs, &startedAt, &layout)
	if noRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(layout, &view.Layout); err != nil {
		return nil, err
	}
	view.StartedAt = startedAt.UnixMilli()
	return &view, nil
}

func listHistory(ctx context.Context, q querier, playID string) ([]HistoryItem, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT round, winner_id, winner_label, started_at
		FROM spins WHERE play_id = $1 AND resolved = true ORDER BY round`, playID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []HistoryItem{}
	for rows.Next() {
		var (
			item HistoryItem
			at   time.Time
		)
		if err := rows.Scan(&item.Round, &item.WinnerID, &item.WinnerLabel, &at); err != nil {
			return nil, err
		}
		item.At = at.UnixMilli()
		out = append(out, item)
	}
	return out, rows.Err()
}
