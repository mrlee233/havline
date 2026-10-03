ALTER TABLE frp_servers ADD COLUMN config_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE frp_servers ADD COLUMN oidc_client_secret_enc TEXT NOT NULL DEFAULT '';
