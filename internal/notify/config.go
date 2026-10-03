package notify

const (
	EventDDNSIPChange       = "ddns_ip_change"
	EventDDNSFailure        = "ddns_failure"
	EventCertExpiry         = "cert_expiry"
	EventCertRenewSuccess   = "cert_renew_success"
	EventCertRenewFailure   = "cert_renew_failure"
	EventIPFrequentAccess   = "ip_frequent_access"
	EventLoginFailure       = "login_failure"
	EventNginxReloadFailure = "nginx_reload_failure"
	EventResourceThreshold  = "resource_threshold"
	EventTunnelDown         = "tunnel_down"
	EventFrpsServerDown     = "frps_server_down"
	EventAgentUnreachable   = "agent_unreachable"
	EventRouteDrift         = "route_drift"
	EventBackupFailure      = "backup_failure"

	MaskedSecret = "********"

	DefaultBarkServer                = "https://api.day.app"
	DefaultNtfyServer                = "https://ntfy.sh"
	DefaultWecomServer               = "https://qyapi.weixin.qq.com"
	DefaultDingtalkServer            = "https://oapi.dingtalk.com"
	DefaultFeishuServer              = "https://open.feishu.cn"
	DefaultIPFrequentThreshold       = 100
	DefaultIPFrequentWindowSec       = 60
	DefaultIPFrequentAlertCooldown   = 15 * 60 // seconds
	DefaultLoginFailureThreshold     = 5
	DefaultLoginFailureWindowSec     = 300
	DefaultLoginFailureAlertCooldown = 15 * 60 // seconds
	DefaultCPUThreshold              = 90
	DefaultMemoryThreshold           = 90
	DefaultDiskThreshold             = 85
	DefaultTunnelFailThreshold       = 3
	DefaultFrpsFailThreshold         = 2
	DefaultAgentFailThreshold        = 3
	DefaultDriftCooldownHours        = 24
	// DefaultResourceAlertCooldown 是资源类告警的静默期：持续高占用时每半小时提醒一次，
	// 期间恢复正常会清掉冷却，再次超标可立即再报。
	DefaultResourceAlertCooldown = 30 * 60 // seconds
)

type NotifyType string

const (
	NotifyTypeEmail    NotifyType = "email"
	NotifyTypeWebhook  NotifyType = "webhook"
	NotifyTypeTelegram NotifyType = "telegram"
)

type WebhookProvider string

const (
	WebhookBark     WebhookProvider = "bark"
	WebhookNtfy     WebhookProvider = "ntfy"
	WebhookGotify   WebhookProvider = "gotify"
	WebhookWecom    WebhookProvider = "wecom"
	WebhookDingtalk WebhookProvider = "dingtalk"
	WebhookFeishu   WebhookProvider = "feishu"
	WebhookCustom   WebhookProvider = "custom"
)

// providerUsesKey 表示该预设把凭据放在 Webhook.Key（而不是 WebhookSecret）里。
// 保存时的「已配置」判定与密钥落库都按这份名单走，新增预设时只需改这一处。
func providerUsesKey(provider WebhookProvider) bool {
	switch provider {
	case WebhookBark, WebhookNtfy, WebhookGotify, WebhookWecom, WebhookDingtalk, WebhookFeishu:
		return true
	default:
		return false
	}
}

type EmailConfig struct {
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Username    string   `json:"username"`
	From        string   `json:"from"`
	To          []string `json:"to"`
	TLS         bool     `json:"tls"`
	HasPassword bool     `json:"has_password"`
}

type WebhookConfig struct {
	Provider  WebhookProvider `json:"provider"`
	Server    string          `json:"server"`
	Key       string          `json:"key"`
	Topic     string          `json:"topic"`
	URL       string          `json:"url"`
	HasSecret bool            `json:"has_secret"`
}

type TelegramConfig struct {
	ChatID      string `json:"chat_id"`
	ProxyURL    string `json:"proxy_url"`
	HasBotToken bool   `json:"has_bot_token"`
}

type Config struct {
	Type                  NotifyType     `json:"type"`
	Email                 EmailConfig    `json:"email"`
	Webhook               WebhookConfig  `json:"webhook"`
	Telegram              TelegramConfig `json:"telegram"`
	OnDDNSIPChange        bool           `json:"on_ddns_ip_change"`
	OnDDNSFailure         bool           `json:"on_ddns_failure"`
	OnCertExpiry          bool           `json:"on_cert_expiry"`
	OnCertRenewSuccess    bool           `json:"on_cert_renew_success"`
	OnCertRenewFailure    bool           `json:"on_cert_renew_failure"`
	OnIPFrequentAccess    bool           `json:"on_ip_frequent_access"`
	OnLoginFailure        bool           `json:"on_login_failure"`
	OnNginxReloadFailure  bool           `json:"on_nginx_reload_failure"`
	IPFrequentThreshold   int            `json:"ip_frequent_threshold"`
	IPFrequentWindowSec   int            `json:"ip_frequent_window_sec"`
	LoginFailureThreshold int            `json:"login_failure_threshold"`
	LoginFailureWindowSec int            `json:"login_failure_window_sec"`
	OnResourceThreshold   bool           `json:"on_resource_threshold"`
	OnTunnelDown          bool           `json:"on_tunnel_down"`
	CPUThreshold          int            `json:"cpu_threshold"`
	MemoryThreshold       int            `json:"memory_threshold"`
	DiskThreshold         int            `json:"disk_threshold"`
	TunnelFailThreshold   int            `json:"tunnel_fail_threshold"`
	OnFrpsDown            bool           `json:"on_frps_down"`
	OnAgentUnreachable    bool           `json:"on_agent_unreachable"`
	OnRouteDrift          bool           `json:"on_route_drift"`
	FrpsFailThreshold     int            `json:"frps_fail_threshold"`
	AgentFailThreshold    int            `json:"agent_fail_threshold"`
	DriftCooldownHours    int            `json:"drift_cooldown_hours"`
	OnBackupFailure       bool           `json:"on_backup_failure"`
}

type RuntimeConfig struct {
	Config
	SMTPPassword  string
	WebhookSecret string
	TelegramToken string
}

type SaveInput struct {
	Type                  NotifyType
	Email                 EmailConfig
	SMTPPassword          string
	Webhook               WebhookConfig
	WebhookSecret         string
	Telegram              TelegramConfig
	TelegramToken         string
	OnDDNSIPChange        bool
	OnDDNSFailure         bool
	OnCertExpiry          bool
	OnCertRenewSuccess    bool
	OnCertRenewFailure    bool
	OnIPFrequentAccess    bool
	OnLoginFailure        bool
	OnNginxReloadFailure  bool
	IPFrequentThreshold   int
	IPFrequentWindowSec   int
	LoginFailureThreshold int
	LoginFailureWindowSec int
	OnResourceThreshold   bool
	OnTunnelDown          bool
	CPUThreshold          int
	MemoryThreshold       int
	DiskThreshold         int
	TunnelFailThreshold   int
	OnFrpsDown            bool
	OnAgentUnreachable    bool
	OnRouteDrift          bool
	FrpsFailThreshold     int
	AgentFailThreshold    int
	DriftCooldownHours    int
	OnBackupFailure       bool
}

type TestInput struct {
	Config
	SMTPPassword  string `json:"smtp_password"`
	WebhookSecret string `json:"webhook_secret"`
	TelegramToken string `json:"telegram_token"`
}

func (c Config) Runtime(password, webhookSecret, telegramToken string) RuntimeConfig {
	return RuntimeConfig{
		Config:        c,
		SMTPPassword:  password,
		WebhookSecret: webhookSecret,
		TelegramToken: telegramToken,
	}
}
