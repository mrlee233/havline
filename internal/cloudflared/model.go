package cloudflared

import "time"

// Mode 是隧道的托管模式；不同模式决定启动参数与规则生效侧。
const (
	ModeTokenLocal   = "token-local"   // token 解码成本地凭据，规则由 Havline 生成
	ModeTokenRemote  = "token-remote"  // 直接用 --token 启动，规则由 Cloudflare 面板管理
	ModeAccountLocal = "account-local" // cert.pem + credentials，规则本地生成，可云端同步
)

// NetworkSettings 是 cloudflared 的网络调优参数。
type NetworkSettings struct {
	TransportProtocol string `json:"transport_protocol"` // http2 / quic / auto
	EdgeIPVersion     string `json:"edge_ip_version"`    // 4 / 6 / auto
	HAConnections     int    `json:"ha_connections"`
	ProxyMode         string `json:"proxy_mode"` // system / disabled
	OriginCAPool      string `json:"origin_ca_pool"`
	NoTLSVerify       bool   `json:"no_tls_verify"`
	HTTPHostHeader    string `json:"http_host_header"`
}

// DefaultNetworkSettings 返回国内环境更稳的默认值。
func DefaultNetworkSettings() NetworkSettings {
	return NetworkSettings{
		TransportProtocol: "http2",
		EdgeIPVersion:     "4",
		HAConnections:     2,
		ProxyMode:         "system",
	}
}

// IngressRule 对应 config.yml 的 ingress 条目。
type IngressRule struct {
	Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	Path     string `json:"path,omitempty" yaml:"path,omitempty"`
	Service  string `json:"service" yaml:"service"`
}

// Tunnel 是一条 Cloudflare 隧道。
type Tunnel struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Mode           string          `json:"mode"`
	TunnelID       string          `json:"tunnel_id"`
	AccountTag     string          `json:"account_tag"`
	CredentialsEnc string          `json:"-"`
	CertPEMEnc     string          `json:"-"`
	ConfigPath     string          `json:"config_path"`
	CredsPath      string          `json:"creds_path"`
	LogPath        string          `json:"log_path"`
	Network        NetworkSettings `json:"network"`
	Status         string          `json:"status"`
	LastError      string          `json:"last_error"`
	MetricsPort    int             `json:"metrics_port"`
	AutoStart      bool            `json:"auto_start"`
	Managed        bool            `json:"managed"`
	Hostnames      []string        `json:"hostnames"`
	OriginService  string          `json:"origin_service"`
	DNSWarning     string          `json:"dns_warning,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// CreateInput 是创建 / 更新隧道时的入参。
type CreateInput struct {
	Name          string          `json:"name"`
	Mode          string          `json:"mode"`
	Token         string          `json:"token"`
	Hostnames     []string        `json:"hostnames"`
	OriginService string          `json:"origin_service"`
	Network       NetworkSettings `json:"network"`
	AutoStart     bool            `json:"auto_start"`
	Managed       bool            `json:"managed"`
}

// AppSettings 是模块级配置；默认 token 只回显掩码，不返回明文。
type AppSettings struct {
	AccountID              string          `json:"account_id"`
	Enabled                bool            `json:"enabled"`
	DefaultTokenConfigured bool            `json:"default_token_configured"`
	DefaultTokenMasked     string          `json:"default_token_masked"`
	APITokenConfigured     bool            `json:"api_token_configured"`
	APITokenMasked         string          `json:"api_token_masked"`
	Mirror                 string          `json:"mirror"`
	Network                NetworkSettings `json:"network"`
}

// AppSettingsInput 是保存应用配置的入参；Token 留空表示保留原值。
type AppSettingsInput struct {
	AccountID string          `json:"account_id"`
	Enabled   *bool           `json:"enabled,omitempty"`
	Token     string          `json:"token"`
	APIToken  string          `json:"api_token"`
	Mirror    string          `json:"mirror"`
	Network   NetworkSettings `json:"network"`
}

// RuntimeStatus 是隧道运行状态与指标快照。
type RuntimeStatus struct {
	Running         bool    `json:"running"`
	Status          string  `json:"status"`
	LastError       string  `json:"last_error"`
	MetricsPort     int     `json:"metrics_port"`
	Connections     int     `json:"connections"`
	LatencyMS       float64 `json:"latency_ms"`
	BandwidthIn     float64 `json:"bandwidth_in"`
	BandwidthOut    float64 `json:"bandwidth_out"`
	Transport       string  `json:"transport"`
	BandwidthNotice string  `json:"bandwidth_notice,omitempty"`
}
