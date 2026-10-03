package settings

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

const (
	KeyTheme              = "theme"
	KeyTimezone           = "timezone"
	KeyLogRetentionDays   = "log_retention_days"
	KeyDDNSCheckInterval  = "ddns_check_interval_minutes"
	KeyCertRenewThreshold = "cert_renew_threshold_days"
	KeyRootDomain         = "root_domain"
	KeyACMEEmail          = "acme_email"
	KeyACMECA             = "acme_ca"
	KeyZeroSSLAPIKey      = "zerossl_api_key"
	// KeyACMEDNSResolvers DNS-01 传播检查用的递归解析器（逗号分隔；system = 用系统解析器）
	KeyACMEDNSResolvers = "acme_dns_resolvers"
	// LiteSSL 强制 EAB：kid 与 hmac 在 FreeSSL 的「证书自动化 → EAB 管理」创建
	KeyLiteSSLEABKid               = "litessl_eab_kid"
	KeyLiteSSLEABHMAC              = "litessl_eab_hmac"
	KeyNotifyWebhookURL            = "notify_webhook_url"
	KeyNotifyType                  = "notify_type"
	KeyNotifyEmailJSON             = "notify_email_json"
	KeyNotifySMTPPassword          = "notify_smtp_password"
	KeyNotifyWebhookJSON           = "notify_webhook_json"
	KeyNotifyWebhookSecret         = "notify_webhook_secret"
	KeyNotifyTelegramJSON          = "notify_telegram_json"
	KeyNotifyTelegramToken         = "notify_telegram_token"
	KeyNotifyOnDDNSError           = "notify_on_ddns_error"  // legacy, migrated to KeyNotifyOnDDNSFailure
	KeyNotifyOnCertError           = "notify_on_cert_error"  // legacy, migrated to KeyNotifyOnCertExpiry
	KeyNotifyOnNginxError          = "notify_on_nginx_error" // legacy, ignored
	KeyNotifyOnDDNSIPChange        = "notify_on_ddns_ip_change"
	KeyNotifyOnDDNSFailure         = "notify_on_ddns_failure"
	KeyNotifyOnCertExpiry          = "notify_on_cert_expiry"
	KeyNotifyOnCertRenewSuccess    = "notify_on_cert_renew_success"
	KeyNotifyOnIPFrequentAccess    = "notify_on_ip_frequent_access"
	KeyNotifyOnCertRenewFailure    = "notify_on_cert_renew_failure"
	KeyNotifyOnLoginFailure        = "notify_on_login_failure"
	KeyNotifyOnNginxReloadFailure  = "notify_on_nginx_reload_failure"
	KeyNotifyIPFrequentThreshold   = "notify_ip_frequent_threshold"
	KeyNotifyIPFrequentWindowSec   = "notify_ip_frequent_window_sec"
	KeyNotifyLoginFailureThreshold = "notify_login_failure_threshold"
	KeyNotifyLoginFailureWindowSec = "notify_login_failure_window_sec"
	KeyNotifyOnResourceThreshold   = "notify_on_resource_threshold"
	KeyNotifyOnTunnelDown          = "notify_on_tunnel_down"
	KeyNotifyCPUThreshold          = "notify_cpu_threshold"
	KeyNotifyMemoryThreshold       = "notify_memory_threshold"
	KeyNotifyDiskThreshold         = "notify_disk_threshold"
	KeyNotifyTunnelFailThreshold   = "notify_tunnel_fail_threshold"
	KeyNotifyOnFrpsDown            = "notify_on_frps_down"
	KeyNotifyOnAgentUnreachable    = "notify_on_agent_unreachable"
	KeyNotifyOnRouteDrift          = "notify_on_route_drift"
	KeyNotifyFrpsFailThreshold     = "notify_frps_fail_threshold"
	KeyNotifyAgentFailThreshold    = "notify_agent_fail_threshold"
	KeyNotifyDriftCooldownHours    = "notify_drift_cooldown_hours"
	KeyBackupAutoEnabled           = "backup_auto_enabled"
	KeyBackupKeepCount             = "backup_keep_count"
	KeyRouteHealthEnabled          = "route_health_enabled"
	KeyStatusPageEnabled           = "status_page_enabled"
	KeyMetricsHistoryEnabled       = "metrics_history_enabled"
	// KeyStatusPageCodeHash 存访问码的 SHA-256；明文只在保存请求里出现一次
	KeyStatusPageCodeHash = "status_page_code_hash"
	// KeyStatusPageCode 只是「保存请求」里用的字段名，不会被直接写库
	KeyStatusPageCode        = "status_page_code"
	KeyNotifyOnBackupFailure = "notify_on_backup_failure"
	KeyTrustedProxy          = "trusted_proxy_json"
	KeyGlobalIPBlacklist     = "global_ip_blacklist"
	KeyGlobalIPWhitelist     = "global_ip_whitelist"
	KeyChinaCIDRUpdateHours  = "china_cidr_update_interval_hours"
	KeyChinaCIDRSourceV4     = "china_cidr_source_url_v4"
	KeyChinaCIDRSourceV6     = "china_cidr_source_url_v6"
	KeyChinaCIDRUpdatedAt    = "china_cidr_updated_at"
	KeyChinaCIDRCountV4      = "china_cidr_count_v4"
	KeyChinaCIDRCountV6      = "china_cidr_count_v6"
	KeyChinaCIDRLastError    = "china_cidr_last_error"
	KeyFRPEnabled            = "frp_enabled"
	KeyFRPServerAddr         = "frp_server_addr"
	KeyFRPServerPort         = "frp_server_port"
	KeyFRPAuthToken          = "frp_auth_token"
	KeyFRPTLSEnabled         = "frp_tls_enabled"
	KeyFRPCustomDomains      = "frp_custom_domains"
	KeyFRPLastError          = "frp_last_error"
	KeyFRPStartedAt          = "frp_started_at"
	KeyNginxGlobalMode       = "nginx_global_mode"
)

var Defaults = map[string]string{
	KeyTheme:                "system",
	KeyTimezone:             "Asia/Shanghai",
	KeyLogRetentionDays:     "30",
	KeyDDNSCheckInterval:    "5",
	KeyCertRenewThreshold:   "30",
	KeyACMECA:               "letsencrypt",
	KeyChinaCIDRUpdateHours: "24",
	KeyNginxGlobalMode:      "auto",
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		if def, ok := Defaults[key]; ok {
			return def, nil
		}
		return "", nil
	}
	return value, err
}

func (s *Store) Set(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

func (s *Store) GetBool(ctx context.Context, key string) (bool, error) {
	raw, err := s.Get(ctx, key)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	default:
		return false, nil
	}
}

func (s *Store) SetBool(ctx context.Context, key string, value bool) error {
	if value {
		return s.Set(ctx, key, "true")
	}
	return s.Set(ctx, key, "false")
}

func (s *Store) SetInt(ctx context.Context, key string, value int) error {
	return s.Set(ctx, key, strconv.Itoa(value))
}

func (s *Store) GetInt(ctx context.Context, key string) (int, error) {
	raw, err := s.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if raw == "" {
		if def, ok := Defaults[key]; ok {
			raw = def
		}
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) List(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for k, v := range Defaults {
		out[k] = v
	}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, rows.Err()
}

func (s *Store) SetMany(ctx context.Context, values map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for key, value := range values {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings(key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value
		`, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}
