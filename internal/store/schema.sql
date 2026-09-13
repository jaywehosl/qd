PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS nodes (
    id          INTEGER PRIMARY KEY,
    tag         TEXT    NOT NULL UNIQUE,
    address     TEXT    NOT NULL,
    port        INTEGER NOT NULL,
    role        TEXT    NOT NULL CHECK (role IN ('ingress', 'egress')),
    enable      INTEGER NOT NULL DEFAULT 1,
    uuid        TEXT    NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS entrypoints (
    id          INTEGER PRIMARY KEY,
    node_id     INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    port        INTEGER NOT NULL,
    remark      TEXT    NOT NULL DEFAULT '',
    enable      INTEGER NOT NULL DEFAULT 1,
    created_at  INTEGER NOT NULL,
    UNIQUE (node_id, port)
);

CREATE TABLE IF NOT EXISTS groups (
    id           INTEGER PRIMARY KEY,
    tag          TEXT    NOT NULL UNIQUE,
    allow_exit   INTEGER NOT NULL DEFAULT 0,
    device_limit INTEGER NOT NULL DEFAULT 0,
    relay_enable INTEGER NOT NULL DEFAULT 0,
    relays       TEXT    NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS group_entrypoints (
    group_id       INTEGER NOT NULL REFERENCES groups(id)      ON DELETE CASCADE,
    entrypoint_id  INTEGER NOT NULL REFERENCES entrypoints(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, entrypoint_id)
);

CREATE TABLE IF NOT EXISTS clients (
    id          INTEGER PRIMARY KEY,
    tag         TEXT    NOT NULL UNIQUE,
    uuid        TEXT    NOT NULL UNIQUE,
    group_id    INTEGER          REFERENCES groups(id) ON DELETE SET NULL,
    enable      INTEGER NOT NULL DEFAULT 1,
    expiry_at   INTEGER NOT NULL DEFAULT 0,
    comment     TEXT    NOT NULL DEFAULT '',
    admin        INTEGER NOT NULL DEFAULT 0,
    device_limit INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_clients_group ON clients(group_id);

CREATE TABLE IF NOT EXISTS revisions (
    number        INTEGER PRIMARY KEY,
    created_at    INTEGER NOT NULL,
    published_at  INTEGER NOT NULL DEFAULT 0,
    state_json    TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_revisions_published ON revisions(published_at);

CREATE TABLE IF NOT EXISTS network (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    key             TEXT    NOT NULL,
    created_at      INTEGER NOT NULL,
    refresh_minutes INTEGER NOT NULL DEFAULT 480,
    dns_primary     TEXT    NOT NULL DEFAULT '1.1.1.1',
    dns_secondary   TEXT    NOT NULL DEFAULT '8.8.8.8',
    dns_cache       INTEGER NOT NULL DEFAULT 4096,
    dns_min_ttl     INTEGER NOT NULL DEFAULT 60,
    dns_max_ttl     INTEGER NOT NULL DEFAULT 3600,
    dns_stale       INTEGER NOT NULL DEFAULT 60,
    mtu             INTEGER NOT NULL DEFAULT 1500,
    stats_seconds   INTEGER NOT NULL DEFAULT 5,

    pool              TEXT    NOT NULL DEFAULT '10.7.0.0/16',
    brutal_mbit       INTEGER NOT NULL DEFAULT 0,
    max_streams       INTEGER NOT NULL DEFAULT 65536,
    stream_window     INTEGER NOT NULL DEFAULT 2048,
    max_stream_window INTEGER NOT NULL DEFAULT 6144,
    conn_window       INTEGER NOT NULL DEFAULT 3072,
    max_conn_window   INTEGER NOT NULL DEFAULT 15360,
    idle_seconds      INTEGER NOT NULL DEFAULT 90,
    keepalive_seconds INTEGER NOT NULL DEFAULT 15,
    socket_buffer     INTEGER NOT NULL DEFAULT 2048
);

CREATE TABLE IF NOT EXISTS dns_records (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    suffix  TEXT    NOT NULL,
    v4      TEXT    NOT NULL DEFAULT '',
    v6      TEXT    NOT NULL DEFAULT '',
    comment TEXT    NOT NULL DEFAULT '',
    enable  INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_dns_records_suffix ON dns_records(suffix);

CREATE TABLE IF NOT EXISTS node_state (
    node_id           INTEGER PRIMARY KEY,
    applied_revision  INTEGER NOT NULL DEFAULT 0,
    staged_revision   INTEGER NOT NULL DEFAULT 0,
    status            TEXT    NOT NULL DEFAULT 'unknown',
    last_seen         INTEGER NOT NULL DEFAULT 0,
    epoch             INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS client_traffic (
    client_id   INTEGER NOT NULL,
    node_id     INTEGER NOT NULL,
    epoch       INTEGER NOT NULL DEFAULT 0,
    last_up     INTEGER NOT NULL DEFAULT 0,
    last_down   INTEGER NOT NULL DEFAULT 0,
    up          INTEGER NOT NULL DEFAULT 0,
    down        INTEGER NOT NULL DEFAULT 0,
    at          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (client_id, node_id)
);

CREATE INDEX IF NOT EXISTS idx_traffic_client ON client_traffic(client_id);

CREATE TABLE IF NOT EXISTS peer_traffic (
    peer_id     INTEGER NOT NULL,
    node_id     INTEGER NOT NULL,
    epoch       INTEGER NOT NULL DEFAULT 0,
    last_up     INTEGER NOT NULL DEFAULT 0,
    last_down   INTEGER NOT NULL DEFAULT 0,
    up          INTEGER NOT NULL DEFAULT 0,
    down        INTEGER NOT NULL DEFAULT 0,
    at          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (peer_id, node_id)
);

CREATE INDEX IF NOT EXISTS idx_traffic_peer ON peer_traffic(node_id);

CREATE TABLE IF NOT EXISTS devices (
    client_id    INTEGER NOT NULL,
    fingerprint  TEXT    NOT NULL,
    platform     TEXT    NOT NULL DEFAULT '',
    model        TEXT    NOT NULL DEFAULT '',
    kind         TEXT    NOT NULL DEFAULT 'desktop',
    node_id      INTEGER NOT NULL DEFAULT 0,
    first_seen   INTEGER NOT NULL DEFAULT 0,
    last_seen    INTEGER NOT NULL DEFAULT 0,
    up           INTEGER NOT NULL DEFAULT 0,
    down         INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (client_id, fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_devices_seen ON devices(client_id, last_seen);

CREATE TABLE IF NOT EXISTS ip_log (
    client_id    INTEGER NOT NULL,
    fingerprint  TEXT    NOT NULL DEFAULT '',
    ip           TEXT    NOT NULL,
    node_id      INTEGER NOT NULL DEFAULT 0,
    last_seen    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (client_id, fingerprint, ip)
);

CREATE INDEX IF NOT EXISTS idx_ip_log_seen ON ip_log(client_id, last_seen);

CREATE TABLE IF NOT EXISTS exit_log (
    client_id INTEGER NOT NULL,
    node_id   INTEGER NOT NULL,
    first_seen INTEGER NOT NULL DEFAULT 0,
    last_seen  INTEGER NOT NULL DEFAULT 0,
    up         INTEGER NOT NULL DEFAULT 0,
    down       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (client_id, node_id)
);

CREATE TABLE IF NOT EXISTS node_metrics (
    node_id  INTEGER NOT NULL,
    metric   TEXT    NOT NULL,
    t        INTEGER NOT NULL,
    v        REAL    NOT NULL,
    PRIMARY KEY (node_id, metric, t)
);

CREATE INDEX IF NOT EXISTS idx_node_metrics_window ON node_metrics(node_id, metric, t);
