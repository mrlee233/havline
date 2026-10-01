package frp

import "time"

type RuntimeInfo struct {
	Platform        string   `json:"platform"`
	ConfiguredBin   string   `json:"configured_bin"`
	ActiveBin       string   `json:"active_bin,omitempty"`
	ActiveVersion   string   `json:"active_version,omitempty"`
	DetectedVersion string   `json:"detected_version,omitempty"`
	DownloadProxy   string   `json:"download_proxy"`
	ConfigPath      string   `json:"config_path"`
	ConfigPaths     []string `json:"config_paths,omitempty"`
	ConfigExists    bool     `json:"config_exists"`
}

type Release struct {
	Version     string `json:"version"`
	PublishedAt string `json:"published_at,omitempty"`
	AssetName   string `json:"asset_name"`
	Size        int64  `json:"size"`
	Checksum    string `json:"-"`
	AssetID     int64  `json:"-"`
}

type Binary struct {
	Version     string `json:"version"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	InstalledAt string `json:"installed_at"`
	Active      bool   `json:"active"`
}

// DiagnosticCheck 单项诊断结果；State 取 ok / failed / unknown（unknown 表示无法确认，不等于失败）。
type DiagnosticCheck struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Detail    string `json:"detail,omitempty"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
}

// ServerDiagnostics 服务端诊断结果。Available/LatencyMS/Error 保持 tcp_reachable 口径
// 兼容历史延迟曲线；定位等非阻断信息只出现在 Checks 中，不参与 Error。
type ServerDiagnostics struct {
	ServerID  int64             `json:"server_id"`
	Available bool              `json:"available"`
	LatencyMS int64             `json:"latency_ms,omitempty"`
	Location  string            `json:"location,omitempty"`
	Checks    []DiagnosticCheck `json:"checks"`
	Error     string            `json:"error,omitempty"`
}

type ServerSample struct {
	CheckedAt string `json:"checked_at"`
	Available bool   `json:"available"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

type ServerStatusDetail struct {
	Available      bool           `json:"available"`
	Availability   float64        `json:"availability"`
	AverageLatency int64          `json:"average_latency_ms"`
	PeakLatency    int64          `json:"peak_latency_ms"`
	Samples        []ServerSample `json:"samples"`
	LastCheckedAt  string         `json:"last_checked_at,omitempty"`
	LastError      string         `json:"last_error,omitempty"`
}

type ServerDetail struct {
	Server  Server             `json:"server"`
	Status  ServerStatusDetail `json:"status"`
	Proxies []Proxy            `json:"proxies"`
}

type DownloadTask struct {
	Version    string `json:"version,omitempty"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Error      string `json:"error,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type Server struct {
	ID                   int64         `json:"id"`
	Name                 string        `json:"name"`
	AgentURL             string        `json:"agent_url,omitempty"`
	AgentTransport       string        `json:"agent_transport"`
	AgentLocalPort       int           `json:"agent_local_port,omitempty"`
	AgentTLSPin          string        `json:"agent_tls_pin,omitempty"`
	AgentTLSPinPrev      string        `json:"agent_tls_pin_prev,omitempty"`
	AgentTLSPinPrevUntil string        `json:"agent_tls_pin_prev_until,omitempty"`
	AgentTLSNotAfter     string        `json:"agent_tls_not_after,omitempty"`
	AgentFirewallState   string        `json:"agent_firewall_state,omitempty"`
	AgentListenAddr      string        `json:"agent_listen_addr,omitempty"`
	AgentTransportState  string        `json:"agent_transport_state"`
	AgentTransportError  string        `json:"agent_transport_error,omitempty"`
	AgentConfigured      bool          `json:"agent_configured"`
	SSHConfigured        bool          `json:"ssh_configured"`
	MgmtEnabled          bool          `json:"mgmt_enabled"`
	SSHHost              string        `json:"ssh_host,omitempty"`
	SSHPort              int           `json:"ssh_port,omitempty"`
	SSHUser              string        `json:"ssh_user,omitempty"`
	SSHAuth              string        `json:"ssh_auth,omitempty"`
	ServerAddr           string        `json:"server_addr"`
	ServerPort           int           `json:"server_port"`
	TLS                  bool          `json:"tls_enabled"`
	TLSServerName        string        `json:"tls_server_name,omitempty"`
	Enabled              bool          `json:"enabled"`
	HasToken             bool          `json:"has_token"`
	HasOIDCSecret        bool          `json:"has_oidc_client_secret"`
	DashboardHasPwd      bool          `json:"dashboard_has_pwd"`
	Options              ServerOptions `json:"options"`
	BootStatus           string        `json:"boot_status"`
	DesiredState         string        `json:"desired_state"`
	ProcessState         string        `json:"process_state"`
	ConnectionState      string        `json:"connection_state"`
	StateSource          string        `json:"state_source"`
	ProcessPID           int           `json:"process_pid,omitempty"`
	ExitCode             int           `json:"exit_code,omitempty"`
	LastConnectedAt      string        `json:"last_connected_at,omitempty"`
	LastDisconnectedAt   string        `json:"last_disconnected_at,omitempty"`
	LastError            string        `json:"last_error,omitempty"`
	LastStartedAt        string        `json:"last_started_at,omitempty"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
}

type ServerRuntimeStatus struct {
	ServerID           int64
	Status             string
	DesiredState       string
	ProcessState       string
	ConnectionState    string
	StateSource        string
	ProcessPID         int
	ExitCode           int
	LastError          string
	LastStartedAt      string
	LastConnectedAt    string
	LastDisconnectedAt string
}

type ServerOptions struct {
	User                   string `json:"user,omitempty"`
	Location               string `json:"location,omitempty"`
	AuthMethod             string `json:"auth_method"`
	OIDCClientID           string `json:"oidc_client_id,omitempty"`
	OIDCAudience           string `json:"oidc_audience,omitempty"`
	OIDCScope              string `json:"oidc_scope,omitempty"`
	OIDCTokenEndpointURL   string `json:"oidc_token_endpoint_url,omitempty"`
	OIDCTrustedCAPath      string `json:"oidc_trusted_ca_path,omitempty"`
	OIDCInsecureSkipVerify bool   `json:"oidc_insecure_skip_verify"`
	OIDCProxyURL           string `json:"oidc_proxy_url,omitempty"`
	// VPS frps 的 vhost 监听端口（http/https 反代回源端口），与 frps.toml 生成器和 Nginx upstream 同源
	VhostHTTPPort  int `json:"vhost_http_port,omitempty"`
	VhostHTTPSPort int `json:"vhost_https_port,omitempty"`
	// frps 进阶配置：子域名/404/强制 TLS/端口白名单/每客户端端口上限/tcpmux/文件日志
	SubDomainHost         string `json:"sub_domain_host,omitempty"`
	Custom404Page         string `json:"custom_404_page,omitempty"`
	TLSForce              bool   `json:"tls_force"`
	AllowPorts            string `json:"allow_ports,omitempty"`
	MaxPortsPerClient     int    `json:"max_ports_per_client,omitempty"`
	TCPMuxHTTPConnectPort int    `json:"tcpmux_http_connect_port,omitempty"`
	FrpsLogToFile         bool   `json:"frps_log_to_file"`
	FrpsLogMaxDays        int    `json:"frps_log_max_days,omitempty"`
	// frps 管理接口（webServer），仅用于 Havline 拉取穿透流量等只读信息，不写入 frpc.toml
	DashboardAddr             string `json:"dashboard_addr,omitempty"`
	DashboardPort             int    `json:"dashboard_port,omitempty"`
	DashboardUser             string `json:"dashboard_user,omitempty"`
	DashboardHasPwd           bool   `json:"dashboard_has_pwd"`
	LogLevel                  string `json:"log_level"`
	LogMaxDays                int    `json:"log_max_days"`
	Protocol                  string `json:"protocol"`
	ProxyURL                  string `json:"proxy_url,omitempty"`
	DialServerTimeout         int    `json:"dial_server_timeout"`
	DialServerKeepalive       int    `json:"dial_server_keepalive"`
	ConnectServerLocalIP      string `json:"connect_server_local_ip,omitempty"`
	DNSServer                 string `json:"dns_server,omitempty"`
	PoolCount                 int    `json:"pool_count"`
	TCPMux                    bool   `json:"tcp_mux"`
	TCPMuxKeepaliveInterval   int    `json:"tcp_mux_keepalive_interval"`
	HeartbeatInterval         int    `json:"heartbeat_interval"`
	HeartbeatTimeout          int    `json:"heartbeat_timeout"`
	LoginFailExit             bool   `json:"login_fail_exit"`
	TLSDisableCustomFirstByte bool   `json:"tls_disable_custom_first_byte"`
	AutoStart                 bool   `json:"auto_start"`
	Remark                    string `json:"remark,omitempty"`
	TLSCertificateConfigured  bool   `json:"tls_certificate_configured"`
	TLSKeyConfigured          bool   `json:"tls_key_configured"`
	TLSTrustedCAConfigured    bool   `json:"tls_trusted_ca_configured"`
}

type Proxy struct {
	ID                int64        `json:"id"`
	ServerID          int64        `json:"server_id"`
	Name              string       `json:"name"`
	Type              string       `json:"type"`
	LocalIP           string       `json:"local_ip"`
	LocalPort         int          `json:"local_port"`
	RemotePort        *int         `json:"remote_port,omitempty"`
	CustomDomains     []string     `json:"custom_domains"`
	HostHeaderRewrite string       `json:"host_header_rewrite,omitempty"`
	Options           ProxyOptions `json:"options"`
	Enabled           bool         `json:"enabled"`
	Remark            string       `json:"remark,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

type ServerInput struct {
	AgentURL        string
	AgentToken      string
	ClearAgentToken bool
	SSHHost         string
	SSHPort         int
	SSHUser         string
	SSHAuth         string
	SSHSecret       string
	ClearSSHSecret  bool
	MgmtEnabled     bool

	Name                   string
	ServerAddr             string
	ServerPort             int
	TLS                    bool
	TLSServerName          string
	Enabled                bool
	Token                  string
	ClearToken             bool
	Options                ServerOptions
	OIDCClientSecret       string
	ClearOIDCClientSecret  bool
	DashboardPassword      string
	ClearDashboardPassword bool
	TLSCertificate         string
	TLSKey                 string
	TLSTrustedCA           string
	ClearTLSCertificate    bool
	ClearTLSKey            bool
	ClearTLSTrustedCA      bool
}

type ProxyInput struct {
	ServerID          int64
	Name              string
	Type              string
	LocalIP           string
	LocalPort         int
	RemotePort        *int
	CustomDomains     []string
	HostHeaderRewrite string
	Options           ProxyOptions
	Enabled           bool
	Remark            string
}

// ProxyOptions 对应 FRP [[proxies]] 中除基础地址外的扩展配置。
type ProxyOptions struct {
	Subdomain string `json:"subdomain,omitempty"`
	// WebSocket 开关：仅作用于公网 Nginx 反代模板（加 Upgrade/Connection 头），不写入 frpc.toml
	WebSocket bool `json:"websocket,omitempty"`
	// 公网反代进阶选项（不写入 frpc.toml）：强制跳转 HTTPS / IP 白名单 / Basic Auth 用户名（密码由 agent 生成）
	RedirectHTTPS bool     `json:"redirect_https,omitempty"`
	AllowIPs      []string `json:"allow_ips,omitempty"`
	BasicAuthUser string   `json:"basic_auth_user,omitempty"`
	// 公网反代第一梯队部署设置（同属公网 Nginx vhost 选项，不写入 frpc.toml）
	DenyIPs           []string `json:"deny_ips,omitempty"`             // IP 黑名单
	BasicAuthPassword string   `json:"basic_auth_password,omitempty"`  // 自定义 Basic Auth 密码；空则首次自动生成
	HTTPSDisabled     bool     `json:"https_disabled,omitempty"`       // 显式关闭 HTTPS
	ClientMaxBodySize string   `json:"client_max_body_size,omitempty"` // 上传大小，如 50m
	ProxyReadTimeout  string   `json:"proxy_read_timeout,omitempty"`   // 代理读超时，如 3600s
	SecurityHeaders   bool     `json:"security_headers,omitempty"`     // 安全响应头
	TLS13Only         bool     `json:"tls13_only,omitempty"`           // 仅 TLS 1.3
	// 第二梯队部署设置（同为公网 Nginx vhost 选项）
	RateLimitRate   int                     `json:"rate_limit_rate,omitempty"`  // 请求限流：每秒请求数
	RateLimitBurst  int                     `json:"rate_limit_burst,omitempty"` // 请求限流：突发允许量
	ConnLimitMax    int                     `json:"conn_limit_max,omitempty"`   // 每 IP 并发连接数上限
	ChinaOnly       bool                    `json:"china_only,omitempty"`       // 仅允许中国大陆 IP
	Locations       []string                `json:"locations,omitempty"`
	RouteByHTTPUser string                  `json:"route_by_http_user,omitempty"`
	HTTPUser        string                  `json:"http_user,omitempty"`
	HTTPPassword    string                  `json:"http_password,omitempty"`
	Multiplexer     string                  `json:"multiplexer,omitempty"`
	SecretKey       string                  `json:"secret_key,omitempty"`
	AllowUsers      []string                `json:"allow_users,omitempty"`
	Transport       ProxyTransportOptions   `json:"transport"`
	HealthCheck     ProxyHealthCheckOptions `json:"health_check"`
	LoadBalancer    ProxyLoadBalancer       `json:"load_balancer"`
	Metadatas       map[string]string       `json:"metadatas,omitempty"`
	RequestHeaders  map[string]string       `json:"request_headers,omitempty"`
	ResponseHeaders map[string]string       `json:"response_headers,omitempty"`
	Plugin          *ProxyPluginOptions     `json:"plugin,omitempty"`
	NATTraversal    *ProxyNATTraversal      `json:"nat_traversal,omitempty"`
}

type ProxyTransportOptions struct {
	BandwidthLimit       string `json:"bandwidth_limit,omitempty"`
	BandwidthLimitMode   string `json:"bandwidth_limit_mode,omitempty"`
	UseEncryption        bool   `json:"use_encryption"`
	UseCompression       bool   `json:"use_compression"`
	ProxyProtocolVersion string `json:"proxy_protocol_version,omitempty"`
}

type ProxyHealthCheckOptions struct {
	Type            string        `json:"type,omitempty"`
	Path            string        `json:"path,omitempty"`
	IntervalSeconds int           `json:"interval_seconds,omitempty"`
	MaxFailed       int           `json:"max_failed,omitempty"`
	TimeoutSeconds  int           `json:"timeout_seconds,omitempty"`
	HTTPHeaders     []ProxyHeader `json:"http_headers,omitempty"`
}

type ProxyHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ProxyLoadBalancer struct {
	Group    string `json:"group,omitempty"`
	GroupKey string `json:"group_key,omitempty"`
}

type ProxyPluginOptions struct {
	Type              string `json:"type"`
	UnixPath          string `json:"unix_path,omitempty"`
	LocalPath         string `json:"local_path,omitempty"`
	StripPrefix       string `json:"strip_prefix,omitempty"`
	LocalAddr         string `json:"local_addr,omitempty"`
	HTTPUser          string `json:"http_user,omitempty"`
	HTTPPassword      string `json:"http_password,omitempty"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	CRTPath           string `json:"crt_path,omitempty"`
	KeyPath           string `json:"key_path,omitempty"`
	HostHeaderRewrite string `json:"host_header_rewrite,omitempty"`
	DestinationIP     string `json:"destination_ip,omitempty"`
}

type ProxyNATTraversal struct {
	DisableAssistedAddrs bool `json:"disable_assisted_addrs"`
}

type RuntimeStatus struct {
	Status         string `json:"status"`
	PID            int    `json:"pid,omitempty"`
	Version        string `json:"frpc_version,omitempty"`
	LastStartedAt  string `json:"last_started_at,omitempty"`
	LastReloadedAt string `json:"last_reloaded_at,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	Configured     bool   `json:"configured"`
	EnabledProxies int    `json:"enabled_proxies"`
}
