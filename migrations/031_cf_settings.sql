-- Cloudflare 隧道模块的应用级配置：默认 Token、下载镜像与默认网络设置。
CREATE TABLE IF NOT EXISTS cf_settings (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    default_token_enc TEXT NOT NULL DEFAULT '',
    mirror            TEXT NOT NULL DEFAULT 'official',
    network_json      TEXT NOT NULL DEFAULT '{}',
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT OR IGNORE INTO cf_settings(id) VALUES (1);
