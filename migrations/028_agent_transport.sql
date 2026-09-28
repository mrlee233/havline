-- Agent 传输安全：默认支持 SSH 隧道回环访问，保留 http 作为升级兼容模式。
ALTER TABLE frp_servers ADD COLUMN agent_transport TEXT NOT NULL DEFAULT 'http';
ALTER TABLE frp_servers ADD COLUMN agent_local_port INTEGER NOT NULL DEFAULT 0;
ALTER TABLE frp_servers ADD COLUMN agent_tls_pin TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN agent_listen_addr TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN agent_transport_state TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE frp_servers ADD COLUMN agent_transport_error TEXT NOT NULL DEFAULT '';
