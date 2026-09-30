-- Cloudflare 隧道全局托管开关：关闭后停止所有隧道，且应用重启不再自动拉起。
ALTER TABLE cf_settings ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1;
