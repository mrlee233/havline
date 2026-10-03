-- Cloudflare API Token（cfat_...）：用于连接验证与后续云端同步，与隧道 Token 分开存储。
ALTER TABLE cf_settings ADD COLUMN api_token_enc TEXT NOT NULL DEFAULT '';
