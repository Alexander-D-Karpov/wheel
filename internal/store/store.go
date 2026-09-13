// Package store owns every database interaction and all of the rules that
// decide how a wheel spins. The HTTP layer above it only translates.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Sentinel errors the HTTP layer maps onto status codes.
var (
	// ErrNotFound covers both "no such row" and "not yours"; the two cases are
	// deliberately indistinguishable from outside.
	ErrNotFound = errors.New("not found")
	// ErrInvalid marks a request the caller could fix by sending other values.
	ErrInvalid = errors.New("invalid request")
	// ErrNotOwner means the action is reserved for the creator of the play.
	ErrNotOwner = errors.New("not the owner of this play session")
	// ErrBusy means a spin is already running.
	ErrBusy = errors.New("a spin is already running")
	// ErrFinished means the play session has no further rounds.
	ErrFinished = errors.New("this play session is finished")
	// ErrNotEnoughEntries means fewer than two entries are still in play.
	ErrNotEnoughEntries = errors.New("at least two entries must be in play")
)

// Store is a thin, query-owning wrapper around the connection pool.
type Store struct {
	db *sql.DB
}

// querier is satisfied by both *sql.DB and *sql.Tx so read helpers can be
// shared between transactional and non-transactional callers.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Open connects to Postgres and verifies the connection before returning.
func Open(ctx context.Context, dsn string, maxOpenConns int) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(max(2, maxOpenConns/2))
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(10 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// inTx runs fn inside a transaction, rolling back on any error or panic.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
