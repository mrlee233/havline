CREATE TABLE IF NOT EXISTS frp_servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE COLLATE NOCASE,
    server_addr TEXT NOT NULL,
    server_port INTEGER NOT NULL CHECK (server_port > 0 AND server_port <= 65535),
    auth_token_enc TEXT,
    tls_enabled INTEGER NOT NULL DEFAULT 1 CHECK (tls_enabled IN (0, 1)),
    tls_server_name TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS frp_proxies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES frp_servers(id) ON DELETE CASCADE,
    name TEXT NOT NULL COLLATE NOCASE,
    type TEXT NOT NULL CHECK (type IN ('tcp', 'http', 'https')),
    local_ip TEXT NOT NULL DEFAULT '127.0.0.1',
    local_port INTEGER NOT NULL CHECK (local_port > 0 AND local_port <= 65535),
    remote_port INTEGER CHECK (remote_port IS NULL OR (remote_port > 0 AND remote_port <= 65535)),
    custom_domains_json TEXT NOT NULL DEFAULT '[]',
    host_header_rewrite TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    remark TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(server_id, name)
);

CREATE TABLE IF NOT EXISTS frp_runtime_status (
    server_id INTEGER PRIMARY KEY REFERENCES frp_servers(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'stopped',
    pid INTEGER,
    frpc_version TEXT NOT NULL DEFAULT '',
    last_started_at TEXT,
    last_reloaded_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_frp_proxies_tcp_remote_port
ON frp_proxies(server_id, remote_port)
WHERE type = 'tcp' AND remote_port IS NOT NULL;
