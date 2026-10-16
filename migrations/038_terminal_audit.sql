-- 内置 SSH 终端会话审计：每次会话结束写一条，回答「谁、何时、连了哪台、多久、为什么断开」。
-- 只记元数据，不落终端内容——需要留存的场景再单独做录制开关。
CREATE TABLE IF NOT EXISTS terminal_audit_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL,
    server_id   INTEGER NOT NULL,
    target      TEXT NOT NULL,
    username    TEXT NOT NULL,
    client_ip   TEXT NOT NULL,
    started_at  TEXT NOT NULL,
    ended_at    TEXT NOT NULL,
    duration_ms INTEGER NOT NULL,
    reason      TEXT,
    bytes_in    INTEGER NOT NULL DEFAULT 0,
    bytes_out   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_terminal_audit_ended ON terminal_audit_logs(ended_at);
