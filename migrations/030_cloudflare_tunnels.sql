-- Cloudflare Tunnel（cloudflared）独立模块：每条隧道一份本地配置 + 子进程 + 日志。
CREATE TABLE IF NOT EXISTS cf_tunnels (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL UNIQUE,
    mode            TEXT NOT NULL DEFAULT 'token-local',
    tunnel_id       TEXT NOT NULL DEFAULT '',
    account_tag     TEXT NOT NULL DEFAULT '',
    credentials_enc TEXT NOT NULL DEFAULT '',
    cert_pem_enc    TEXT NOT NULL DEFAULT '',
    config_path     TEXT NOT NULL DEFAULT '',
    creds_path      TEXT NOT NULL DEFAULT '',
    log_path        TEXT NOT NULL DEFAULT '',
    network_json    TEXT NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'stopped',
    last_error      TEXT NOT NULL DEFAULT '',
    metrics_port    INTEGER NOT NULL DEFAULT 0,
    auto_start      INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
