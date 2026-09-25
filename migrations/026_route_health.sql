-- 反代规则健康状态变更记录：只存「状态变化」（up / down / unknown），
-- 可用率与不可用区间由变更序列推导——避免每 2 分钟一行原始采样把表撑大。
-- unknown 表示「frps 管理接口不可用、判不出来」，与 down 严格区分（判不了不等于不通）。
CREATE TABLE IF NOT EXISTS route_health_events (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL,
    domain    TEXT NOT NULL,
    state     TEXT NOT NULL,
    reason    TEXT,
    detail    TEXT,
    at        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_route_health_events_lookup ON route_health_events(server_id, domain, at);
