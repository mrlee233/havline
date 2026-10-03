-- Cloudflare 账号 ID：与 API Token 一起用于创建与管理隧道。
ALTER TABLE cf_settings ADD COLUMN account_id TEXT NOT NULL DEFAULT '';
