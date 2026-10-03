CREATE TABLE IF NOT EXISTS frp_server_diagnostics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES frp_servers(id) ON DELETE CASCADE,
    checked_at TEXT NOT NULL DEFAULT (datetime('now')),
    available INTEGER NOT NULL DEFAULT 0 CHECK (available IN (0, 1)),
    latency_ms INTEGER,
    error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_frp_server_diagnostics_server_time
ON frp_server_diagnostics(server_id, checked_at);
