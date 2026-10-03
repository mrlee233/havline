-- Cloudflare 隧道健康事件：只记录状态变化，供巡检通知与公开状态页计算 24h 可用率。
CREATE TABLE IF NOT EXISTS cf_health_events (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    tunnel_id INTEGER NOT NULL,
    domain    TEXT NOT NULL,
    state     TEXT NOT NULL,
    reason    TEXT NOT NULL DEFAULT '',
    detail    TEXT NOT NULL DEFAULT '',
    at        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cf_health_events_lookup ON cf_health_events(tunnel_id, domain, id);
CREATE INDEX IF NOT EXISTS idx_cf_health_events_at ON cf_health_events(at);
