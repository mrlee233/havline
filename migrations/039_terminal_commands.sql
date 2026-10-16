-- 常用命令：终端页右侧面板保存的快捷命令，点一下发送到当前会话。
-- 只存命令文本本身，不存执行结果、不存凭据。
CREATE TABLE IF NOT EXISTS terminal_commands (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    command    TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_terminal_commands_created ON terminal_commands(created_at);
