-- Cookie-bound anonymous identities. Everything a visitor owns hangs off one row.
CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    seen_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_seen_idx ON sessions (seen_at);

-- A reusable wheel definition owned by one session.
CREATE TABLE wheels (
    id            TEXT PRIMARY KEY,
    owner_session TEXT NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    title         TEXT NOT NULL DEFAULT '',
    mode          TEXT NOT NULL DEFAULT 'selection'
        CHECK (mode IN ('selection', 'exclusion')),
    spin_seconds  INT NOT NULL DEFAULT 15 CHECK (spin_seconds > 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX wheels_owner_idx ON wheels (owner_session, created_at DESC);

CREATE TABLE entries (
    id       TEXT PRIMARY KEY,
    wheel_id TEXT NOT NULL REFERENCES wheels (id) ON DELETE CASCADE,
    label    TEXT NOT NULL,
    weight   DOUBLE PRECISION NOT NULL DEFAULT 1 CHECK (weight > 0),
    position INT NOT NULL DEFAULT 0
);
CREATE INDEX entries_wheel_idx ON entries (wheel_id, position, id);

-- A play session: an immutable snapshot of the entries chosen at start time,
-- addressable by a public slug. Editing the wheel afterwards changes nothing here.
CREATE TABLE plays (
    id              TEXT PRIMARY KEY,
    slug            TEXT NOT NULL UNIQUE,
    wheel_id        TEXT REFERENCES wheels (id) ON DELETE SET NULL,
    owner_session   TEXT NOT NULL,
    title           TEXT NOT NULL DEFAULT '',
    mode            TEXT NOT NULL CHECK (mode IN ('selection', 'exclusion')),
    spin_seconds    INT NOT NULL CHECK (spin_seconds > 0),
    status          TEXT NOT NULL DEFAULT 'idle'
        CHECK (status IN ('idle', 'spinning', 'finished')),
    round           INT NOT NULL DEFAULT 0,
    rotation        DOUBLE PRECISION NOT NULL DEFAULT 0,
    current_spin_id TEXT,
    last_winner_id  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX plays_owner_idx ON plays (owner_session, created_at DESC);
CREATE INDEX plays_updated_idx ON plays (updated_at);

CREATE TABLE play_entries (
    id       TEXT PRIMARY KEY,
    play_id  TEXT NOT NULL REFERENCES plays (id) ON DELETE CASCADE,
    label    TEXT NOT NULL,
    weight   DOUBLE PRECISION NOT NULL DEFAULT 1 CHECK (weight > 0),
    position INT NOT NULL DEFAULT 0,
    active   BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE INDEX play_entries_play_idx ON play_entries (play_id, position, id);

-- One row per spin. The rotation maths is decided here, once, by the server;
-- every browser replays the same numbers so the animation stays in sync.
CREATE TABLE spins (
    id           TEXT PRIMARY KEY,
    play_id      TEXT NOT NULL REFERENCES plays (id) ON DELETE CASCADE,
    round        INT NOT NULL,
    winner_id    TEXT NOT NULL,
    winner_label TEXT NOT NULL,
    rot_from     DOUBLE PRECISION NOT NULL,
    rot_delta    DOUBLE PRECISION NOT NULL,
    duration_ms  INT NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    layout       JSONB NOT NULL,
    resolved     BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (play_id, round)
);
CREATE INDEX spins_play_idx ON spins (play_id, round);
CREATE INDEX spins_pending_idx ON spins (started_at) WHERE resolved = FALSE;

ALTER TABLE plays
    ADD CONSTRAINT plays_current_spin_fk
    FOREIGN KEY (current_spin_id) REFERENCES spins (id) ON DELETE SET NULL;
