-- API Token：给无法保存会话 cookie 的调用方（脚本 / HomeAssistant / 手机快捷指令）用。
-- token_hash 是明文令牌的 SHA-256：明文只在创建响应里返回一次，库里不留。
-- scope: read = 只允许 GET/HEAD，write = 与登录会话同权。
CREATE TABLE IF NOT EXISTS api_tokens (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    scope        TEXT NOT NULL DEFAULT 'read',
    expires_at   TEXT,
    last_used_at TEXT,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);
