-- DDNS 支持自定义 IP：填写后更新记录时直接使用该 IP（如公网服务器 IP），不填则检测本机公网 IP。
ALTER TABLE ddns_configs ADD COLUMN custom_ipv4 TEXT NOT NULL DEFAULT '';
ALTER TABLE ddns_configs ADD COLUMN custom_ipv6 TEXT NOT NULL DEFAULT '';
