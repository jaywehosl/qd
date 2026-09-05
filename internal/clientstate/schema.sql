PRAGMA journal_mode = WAL;

CREATE TABLE IF NOT EXISTS subscription (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    uri          TEXT    NOT NULL DEFAULT '',
    key          TEXT    NOT NULL DEFAULT '',
    label        TEXT    NOT NULL DEFAULT '',
    admin        INTEGER NOT NULL DEFAULT 0,
    allow_exit   INTEGER NOT NULL DEFAULT 0,
    expires_at   INTEGER NOT NULL DEFAULT 0,
    tag          TEXT    NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL DEFAULT 0,
    last_refresh INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS nodes (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL DEFAULT '',
    role       TEXT    NOT NULL DEFAULT 'ingress',
    address    TEXT    NOT NULL,
    port       INTEGER NOT NULL,
    latency_ms INTEGER NOT NULL DEFAULT -1,
    reachable  INTEGER NOT NULL DEFAULT 0,
    selected   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rules (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    process TEXT    NOT NULL,
    path    TEXT    NOT NULL DEFAULT '',
    role    TEXT    NOT NULL,
    matched INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_rules_target ON rules(process, path);

CREATE TABLE IF NOT EXISTS notifications (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    severity TEXT    NOT NULL DEFAULT 'info',
    text     TEXT    NOT NULL,
    ts       INTEGER NOT NULL,
    read     INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_notifications_ts ON notifications(ts DESC);

CREATE TABLE IF NOT EXISTS samples (
    t            INTEGER PRIMARY KEY,
    up           INTEGER NOT NULL DEFAULT 0,
    down         INTEGER NOT NULL DEFAULT 0,
    pkt_out      INTEGER NOT NULL DEFAULT 0,
    pkt_in       INTEGER NOT NULL DEFAULT 0,
    lost         INTEGER NOT NULL DEFAULT 0,
    drops        INTEGER NOT NULL DEFAULT 0,
    reorder      INTEGER NOT NULL DEFAULT 0,
    retries      INTEGER NOT NULL DEFAULT 0,
    send_drop    INTEGER NOT NULL DEFAULT 0,
    send_err     INTEGER NOT NULL DEFAULT 0,
    dns_queries  INTEGER NOT NULL DEFAULT 0,
    dns_cached   INTEGER NOT NULL DEFAULT 0,
    dns_upstream INTEGER NOT NULL DEFAULT 0,
    adblock      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS sites (
    host TEXT PRIMARY KEY,
    hits INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS traffic (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    up   INTEGER NOT NULL DEFAULT 0,
    down INTEGER NOT NULL DEFAULT 0
);
