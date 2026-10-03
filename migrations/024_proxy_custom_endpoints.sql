-- 手动（custom）Nginx 规则的监听占用：custom 配置的正本在文件系统（rules.nginx_mode 只记开关），
-- 本表只存解析出的 (域名, 端口)，专供保存时做「同域名 + 同端口」冲突校验；
-- 不参与配置生成、流量统计与前端表单预填（那些是 proxy_hosts 的语义）。
CREATE TABLE IF NOT EXISTS proxy_custom_endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id INTEGER NOT NULL REFERENCES proxy_rules(id) ON DELETE CASCADE,
    hostname TEXT NOT NULL COLLATE NOCASE,
    listen_port INTEGER NOT NULL CHECK (listen_port > 0 AND listen_port <= 65535),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(rule_id, hostname, listen_port)
);

CREATE INDEX IF NOT EXISTS idx_proxy_custom_endpoints_rule_id ON proxy_custom_endpoints(rule_id);
CREATE INDEX IF NOT EXISTS idx_proxy_custom_endpoints_binding ON proxy_custom_endpoints(hostname, listen_port);
