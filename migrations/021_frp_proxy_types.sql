-- FRP 规则类型与端口约束修正（文档 FRP_CROSS_MODULE_IMPLEMENTATION_PLAN.md 23.2）
-- 原 014_frp.sql 的 type CHECK 只有 tcp/http/https，但校验器与前端已支持
-- udp/tcpmux/stcp/sudp/xtcp：这些类型会在校验通过后因 SQLite CHECK 约束插入失败。
-- SQLite 无法 ALTER CHECK 约束，这里重建 frp_proxies 表并统一约束。
CREATE TABLE IF NOT EXISTS frp_proxies_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES frp_servers(id) ON DELETE CASCADE,
    name TEXT NOT NULL COLLATE NOCASE,
    type TEXT NOT NULL CHECK (type IN ('tcp', 'udp', 'http', 'https', 'tcpmux', 'stcp', 'sudp', 'xtcp')),
    local_ip TEXT NOT NULL DEFAULT '127.0.0.1',
    local_port INTEGER NOT NULL CHECK (local_port > 0 AND local_port <= 65535),
    remote_port INTEGER CHECK (
        remote_port IS NULL
        OR (remote_port > 0 AND remote_port <= 65535 AND type IN ('tcp', 'udp'))
    ),
    custom_domains_json TEXT NOT NULL DEFAULT '[]',
    host_header_rewrite TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL DEFAULT '{}',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    remark TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(server_id, name)
);

INSERT INTO frp_proxies_new
    (id, server_id, name, type, local_ip, local_port, remote_port, custom_domains_json, host_header_rewrite, config_json, enabled, remark, created_at, updated_at)
SELECT
    id, server_id, name, type, local_ip, local_port, remote_port, custom_domains_json, host_header_rewrite, COALESCE(config_json, '{}'), enabled, remark, created_at, updated_at
FROM frp_proxies;

DROP TABLE frp_proxies;
ALTER TABLE frp_proxies_new RENAME TO frp_proxies;

-- tcp 与 udp 的远程端口在同一 server_id 内分别查重（原索引混用两种协议）
DROP INDEX IF EXISTS idx_frp_proxies_tcp_remote_port;
CREATE UNIQUE INDEX IF NOT EXISTS idx_frp_proxies_tcp_remote_port
ON frp_proxies(server_id, remote_port)
WHERE type = 'tcp' AND remote_port IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_frp_proxies_udp_remote_port
ON frp_proxies(server_id, remote_port)
WHERE type = 'udp' AND remote_port IS NOT NULL;
