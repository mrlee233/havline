-- Agent TLS 轮换与主机防火墙辅助状态。
ALTER TABLE frp_servers ADD COLUMN agent_tls_pin_prev TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN agent_tls_pin_prev_until TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN agent_tls_not_after TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN agent_firewall_state TEXT NOT NULL DEFAULT '';
