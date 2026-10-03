-- 服务发布规则出口：默认仍走本机 Nginx，保持旧数据行为不变。
ALTER TABLE proxy_rules ADD COLUMN exits_json TEXT NOT NULL DEFAULT '["local"]';
ALTER TABLE proxy_rules ADD COLUMN cf_tunnel_id INTEGER NOT NULL DEFAULT 0;

-- 托管隧道标记：只有托管隧道才从反向代理规则派生 ingress，避免覆盖手工隧道。
ALTER TABLE cf_tunnels ADD COLUMN managed INTEGER NOT NULL DEFAULT 0;

-- 记录 Havline 自动创建或接管的 Cloudflare DNS 记录，删除时只清理自己管理的记录。
CREATE TABLE IF NOT EXISTS cf_dns_records (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tunnel_id   INTEGER NOT NULL,
    zone_id     TEXT NOT NULL,
    record_id   TEXT NOT NULL,
    hostname    TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_cf_dns_records_tunnel ON cf_dns_records(tunnel_id);
