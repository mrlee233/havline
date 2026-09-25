-- 指标历史：按 (scope, metric) 纵向存点，避免「每加一个指标就改一次表结构」。
-- scope 形如 host / server:3 / route:3:example.com；采样跟随巡检轮次（2 分钟）。
-- 行量很小（本机 3 条指标约每天 2160 行），因此不做降采样表，查询时按桶取平均。
CREATE TABLE IF NOT EXISTS metrics_samples (
    id     INTEGER PRIMARY KEY AUTOINCREMENT,
    scope  TEXT NOT NULL,
    metric TEXT NOT NULL,
    value  REAL NOT NULL,
    at     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_metrics_samples_lookup ON metrics_samples(scope, metric, at);
