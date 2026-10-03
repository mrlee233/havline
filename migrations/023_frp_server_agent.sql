-- 公网 havline-agent 远程管理配置
ALTER TABLE frp_servers ADD COLUMN agent_url TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN agent_token_enc TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN ssh_host TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN ssh_port INTEGER NOT NULL DEFAULT 22;
ALTER TABLE frp_servers ADD COLUMN ssh_user TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN ssh_auth TEXT NOT NULL DEFAULT 'key';
ALTER TABLE frp_servers ADD COLUMN ssh_secret_enc TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN mgmt_enabled INTEGER NOT NULL DEFAULT 0 CHECK (mgmt_enabled IN (0, 1));
