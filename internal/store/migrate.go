package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrationLockID is an arbitrary but stable key for pg_advisory_lock, so two
// instances starting at the same moment cannot both apply the same migration.
const migrationLockID int64 = 774_120_193

type migration struct {
	version int
	name    string
	body    string
}

// Migrate applies every embedded migration that has not run yet. It is safe to
// call on every start and from several instances concurrently.
func (s *Store) Migrate(ctx context.Context) error {
	list, err := loadMigrations()
	if err != nil {
		return err
	}

	// The advisory lock is session-scoped, so it has to be taken and released
	// on one pinned connection rather than through the pool.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		if _, err := conn.ExecContext(context.WithoutCancel(ctx),
			`SELECT pg_advisory_unlock($1)`, migrationLockID); err != nil {
			slog.Warn("release migration lock", "err", err)
		}
	}()

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range list {
		if applied[m.version] {
			continue
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, m.body); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", m.name, err)
		}
		slog.Info("migration applied", "version", m.version, "name", m.name)
	}
	return nil
}

// loadMigrations reads migrations/NNNN_name.sql and orders them by version.
func loadMigrations() ([]migration, error) {
	files, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}

	out := make([]migration, 0, len(files))
	seen := map[int]string{}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}
		digits, _, ok := strings.Cut(f.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("migration %q must be named NNNN_description.sql", f.Name())
		}
		version, err := strconv.Atoi(digits)
		if err != nil {
			return nil, fmt.Errorf("migration %q has a non-numeric version: %w", f.Name(), err)
		}
		if other, dup := seen[version]; dup {
			return nil, fmt.Errorf("migrations %q and %q share version %d", other, f.Name(), version)
		}
		seen[version] = f.Name()

		body, err := migrationFS.ReadFile("migrations/" + f.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: version, name: f.Name(), body: string(body)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}
