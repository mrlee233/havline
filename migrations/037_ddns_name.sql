-- DDNS 服务商支持自定义名称：同一服务商可建多条配置，用名称区分（如「家庭」「公司」）。
ALTER TABLE ddns_configs ADD COLUMN name TEXT NOT NULL DEFAULT '';
