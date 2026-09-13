package store

import "time"

// Wheel modes.
const (
	ModeSelection = "selection" // pick a winner, the wheel keeps every entry
	ModeExclusion = "exclusion" // the picked entry drops out, spin until one is left
)

// Play statuses.
const (
	StatusIdle     = "idle"
	StatusSpinning = "spinning"
	StatusFinished = "finished"
)

// Wheel is a saved, editable wheel definition.
type Wheel struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Mode        string    `json:"mode"`
	SpinSeconds int       `json:"spin_seconds"`
	CreatedAt   time.Time `json:"created_at"`
	EntryCount  int       `json:"entry_count"`
	Entries     []Entry   `json:"entries"`
}

// Entry is one option on a saved wheel.
type Entry struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Weight   float64 `json:"weight"`
	Position int     `json:"position"`
}

// PlayEntry is an entry frozen into a play session. Active goes false once an
// entry is knocked out in exclusion mode.
type PlayEntry struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Weight   float64 `json:"weight"`
	Position int     `json:"position"`
	Active   bool    `json:"active"`
}

// SpinView is everything a browser needs to draw a spin frame-accurately.
// It deliberately carries no winner field: the result is announced separately
// when the wheel actually stops.
type SpinView struct {
	ID         string      `json:"id"`
	Round      int         `json:"round"`
	From       float64     `json:"from"`
	Delta      float64     `json:"delta"`
	StartedAt  int64       `json:"started_at"` // unix milliseconds, server clock
	DurationMs int         `json:"duration_ms"`
	Layout     []PlayEntry `json:"layout"`
}

// HistoryItem is one finished round.
type HistoryItem struct {
	Round       int    `json:"round"`
	WinnerID    string `json:"winner_id"`
	WinnerLabel string `json:"winner_label"`
	At          int64  `json:"at"`
}

// PlayState is the complete public view of a play session.
type PlayState struct {
	Slug        string        `json:"slug"`
	Title       string        `json:"title"`
	Mode        string        `json:"mode"`
	SpinSeconds int           `json:"spin_seconds"`
	Status      string        `json:"status"`
	Round       int           `json:"round"`
	Rotation    float64       `json:"rotation"`
	Entries     []PlayEntry   `json:"entries"`
	Spin        *SpinView     `json:"spin"`
	History     []HistoryItem `json:"history"`
	Winner      *PlayEntry    `json:"winner"`
	IsOwner     bool          `json:"is_owner"`
	ServerNow   int64         `json:"server_now"`
}

// SpinResult describes a spin that has just come to a stop.
type SpinResult struct {
	Slug        string     `json:"slug"`
	Round       int        `json:"round"`
	PickedID    string     `json:"picked_id"`
	PickedLabel string     `json:"picked_label"`
	Eliminated  bool       `json:"eliminated"`
	Finished    bool       `json:"finished"`
	State       *PlayState `json:"state"`
}

// play is the internal row shape; it is never serialised as-is.
type play struct {
	ID            string
	Slug          string
	OwnerSession  string
	Title         string
	Mode          string
	SpinSeconds   int
	Status        string
	Round         int
	Rotation      float64
	CurrentSpinID string
	LastWinnerID  string
}
