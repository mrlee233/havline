package frp

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func validateServerInput(in ServerInput) error {
	if strings.TrimSpace(in.Name) == "" || len([]rune(strings.TrimSpace(in.Name))) > 64 {
		return fmt.Errorf("服务端名称长度应为 1-64 个字符")
	}
	if !validHost(in.ServerAddr) {
		return fmt.Errorf("FRP 服务端地址无效")
	}
	if !validPort(in.ServerPort) {
		return fmt.Errorf("FRP 服务端端口无效")
	}
	if in.Options.DashboardPort != 0 && !validPort(in.Options.DashboardPort) {
		return fmt.Errorf("frps 管理接口端口无效")
	}
	if !validDashboardAddr(in.Options.DashboardAddr) {
		return fmt.Errorf("frps 管理接口地址无效")
	}
	if strings.TrimSpace(in.TLSServerName) != "" && !validHost(in.TLSServerName) {
		return fmt.Errorf("TLS 服务名无效")
	}
	if !validAuthMethod(in.Options.AuthMethod) {
		return fmt.Errorf("不支持的认证方式")
	}
	if !validLogLevel(in.Options.LogLevel) {
		return fmt.Errorf("日志级别无效")
	}
	if in.Options.LogMaxDays < 0 || in.Options.LogMaxDays > 3650 {
		return fmt.Errorf("日志保留天数应为 0-3650")
	}
	if !validProtocol(in.Options.Protocol) {
		return fmt.Errorf("传输协议无效")
	}
	if !validOptionalURL(in.Options.ProxyURL) || !validOptionalURL(in.Options.OIDCProxyURL) || !validOptionalURL(in.Options.OIDCTokenEndpointURL) {
		return fmt.Errorf("代理或 OIDC 地址无效")
	}
	if in.Options.AuthMethod == "oidc" && (strings.TrimSpace(in.Options.OIDCClientID) == "" || strings.TrimSpace(in.Options.OIDCTokenEndpointURL) == "") {
		return fmt.Errorf("OIDC 认证需要客户端 ID 和 Token Endpoint")
	}
	if in.Options.DialServerTimeout < 0 || in.Options.DialServerKeepalive < 0 || in.Options.PoolCount < 0 || in.Options.TCPMuxKeepaliveInterval < 0 || in.Options.HeartbeatInterval < 0 || in.Options.HeartbeatTimeout < 0 {
		return fmt.Errorf("传输参数不能小于 0")
	}
	if len([]rune(in.Options.Location)) > 120 {
		return fmt.Errorf("服务端位置不能超过 120 个字符")
	}
	return nil
}

func validateProxyInput(in ProxyInput) error {
	if !namePattern.MatchString(strings.TrimSpace(in.Name)) {
		return fmt.Errorf("规则名称仅支持字母、数字、下划线和连字符，长度 1-64")
	}
	if in.Options.Plugin == nil {
		if !validHost(in.LocalIP) {
			return fmt.Errorf("内网地址无效")
		}
		if !validPort(in.LocalPort) {
			return fmt.Errorf("内网端口无效")
		}
	}
	switch in.Type {
	case "tcp", "udp":
		if in.RemotePort == nil || !validPort(*in.RemotePort) {
			return fmt.Errorf("%s 规则必须填写有效的远程端口", strings.ToUpper(in.Type))
		}
		if len(in.CustomDomains) > 0 {
			return fmt.Errorf("%s 规则不能设置自定义域名", strings.ToUpper(in.Type))
		}
	case "http", "https":
		if in.RemotePort != nil {
			return fmt.Errorf("HTTP/HTTPS 规则不能设置远程端口")
		}
		if len(in.CustomDomains) == 0 {
			return fmt.Errorf("HTTP/HTTPS 规则至少需要一个域名")
		}
		for _, domain := range in.CustomDomains {
			if !validDomain(domain) {
				return fmt.Errorf("自定义域名无效：%s", domain)
			}
		}
	case "tcpmux":
		if in.Options.Multiplexer != "httpconnect" {
			return fmt.Errorf("TCPMUX 仅支持 httpconnect 多路复用器")
		}
		if len(in.CustomDomains) == 0 {
			return fmt.Errorf("TCPMUX 规则至少需要一个自定义域名")
		}
		for _, domain := range in.CustomDomains {
			if !validDomain(domain) {
				return fmt.Errorf("自定义域名无效：%s", domain)
			}
		}
		if in.RemotePort != nil {
			return fmt.Errorf("TCPMUX 规则不能设置远程端口")
		}
	case "stcp", "sudp", "xtcp":
		if strings.TrimSpace(in.Options.SecretKey) == "" {
			return fmt.Errorf("%s 规则必须填写访问密钥", strings.ToUpper(in.Type))
		}
		if in.RemotePort != nil {
			return fmt.Errorf("%s 规则不能设置远程端口", strings.ToUpper(in.Type))
		}
	default:
		return fmt.Errorf("不支持的 FRP 规则类型")
	}
	if err := validateProxyOptions(in); err != nil {
		return err
	}
	return nil
}

func validateProxyOptions(in ProxyInput) error {
	opts := in.Options
	if opts.Transport.BandwidthLimitMode != "" && opts.Transport.BandwidthLimitMode != "client" && opts.Transport.BandwidthLimitMode != "server" {
		return fmt.Errorf("限速位置只能是 client 或 server")
	}
	if opts.Transport.ProxyProtocolVersion != "" && opts.Transport.ProxyProtocolVersion != "v1" && opts.Transport.ProxyProtocolVersion != "v2" {
		return fmt.Errorf("代理协议版本只能是 v1 或 v2")
	}
	if opts.HealthCheck.Type != "" && opts.HealthCheck.Type != "tcp" && opts.HealthCheck.Type != "http" {
		return fmt.Errorf("健康检查类型只能是 tcp 或 http")
	}
	if opts.HealthCheck.IntervalSeconds < 0 || opts.HealthCheck.MaxFailed < 0 || opts.HealthCheck.TimeoutSeconds < 0 {
		return fmt.Errorf("健康检查参数不能小于 0")
	}
	if opts.LoadBalancer.Group != "" && opts.LoadBalancer.GroupKey == "" {
		return fmt.Errorf("负载均衡组设置后必须填写组密钥")
	}
	return validateProxyPlugin(opts.Plugin)
}

// validateProxyPlugin 按插件类型分别校验必填字段；不能只判断插件类型非空。
func validateProxyPlugin(plugin *ProxyPluginOptions) error {
	if plugin == nil {
		return nil
	}
	pluginType := strings.TrimSpace(plugin.Type)
	if pluginType == "" {
		return fmt.Errorf("插件类型不能为空")
	}
	validAddr := func(addr string, label string) error {
		addr = strings.TrimSpace(addr)
		host, portText, ok := strings.Cut(addr, ":")
		if !ok || !validHost(host) || !validPort(parseInt(portText)) {
			return fmt.Errorf("插件本地地址 %s 无效，应为 host:port 形式", label)
		}
		return nil
	}
	validSecretFile := func(path, label string) error {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("插件 %s 需要提供 %s", pluginType, label)
		}
		return nil
	}
	switch pluginType {
	case "unix_domain_socket":
		if strings.TrimSpace(plugin.UnixPath) == "" {
			return fmt.Errorf("插件 unix_domain_socket 需要提供 Unix Socket 路径")
		}
	case "static_file":
		if strings.TrimSpace(plugin.LocalPath) == "" {
			return fmt.Errorf("插件 static_file 需要提供静态文件目录")
		}
	case "http_proxy", "socks5":
		// 用户名/密码可选
	case "http2https", "http2http":
		if err := validAddr(plugin.LocalAddr, "localAddr"); err != nil {
			return err
		}
	case "https2http", "https2https", "tls2raw":
		if err := validAddr(plugin.LocalAddr, "localAddr"); err != nil {
			return err
		}
		if err := validSecretFile(plugin.CRTPath, "crtPath"); err != nil {
			return err
		}
		if err := validSecretFile(plugin.KeyPath, "keyPath"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("不支持的插件类型：%s", pluginType)
	}
	return nil
}

// parseInt 解析十进制端口号；非数字或越界返回 0,由 validPort 拒绝。
func parseInt(value string) int {
	value = strings.TrimSpace(value)
	number := 0
	if value == "" {
		return 0
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0
		}
		number = number*10 + int(ch-'0')
	}
	return number
}

// ValidateProxy 是 FRP 规则校验器的对外入口：关系表等跨模块模式校验必须复用此函数,
// 不能在编排服务中另行实现一套判断（FRP_CROSS_MODULE_IMPLEMENTATION_PLAN.md 23.2）。
func ValidateProxy(in ProxyInput) error {
	return validateProxyInput(normalizeProxyInput(in))
}

func normalizeServerInput(in ServerInput) ServerInput {
	in.Name = strings.TrimSpace(in.Name)
	in.ServerAddr = strings.ToLower(strings.TrimSpace(in.ServerAddr))
	in.TLSServerName = strings.ToLower(strings.TrimSpace(in.TLSServerName))
	in.Token = strings.TrimSpace(in.Token)
	in.OIDCClientSecret = strings.TrimSpace(in.OIDCClientSecret)
	in.Options = normalizeServerOptions(in.Options)
	return in
}

func normalizeServerOptions(in ServerOptions) ServerOptions {
	in.User = strings.TrimSpace(in.User)
	in.Location = strings.TrimSpace(in.Location)
	in.AuthMethod = strings.ToLower(strings.TrimSpace(in.AuthMethod))
	in.OIDCClientID = strings.TrimSpace(in.OIDCClientID)
	in.OIDCAudience = strings.TrimSpace(in.OIDCAudience)
	in.OIDCScope = strings.TrimSpace(in.OIDCScope)
	in.OIDCTokenEndpointURL = strings.TrimSpace(in.OIDCTokenEndpointURL)
	in.OIDCTrustedCAPath = strings.TrimSpace(in.OIDCTrustedCAPath)
	in.OIDCProxyURL = strings.TrimSpace(in.OIDCProxyURL)
	in.LogLevel = strings.ToLower(strings.TrimSpace(in.LogLevel))
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	in.ProxyURL = strings.TrimSpace(in.ProxyURL)
	in.ConnectServerLocalIP = strings.TrimSpace(in.ConnectServerLocalIP)
	in.DNSServer = strings.TrimSpace(in.DNSServer)
	in.Remark = strings.TrimSpace(in.Remark)
	if in.AuthMethod == "" {
		in.AuthMethod = "token"
	}
	if in.LogLevel == "" {
		in.LogLevel = "info"
	}
	if in.Protocol == "" {
		in.Protocol = "tcp"
	}
	if in.LogMaxDays == 0 {
		in.LogMaxDays = 3
	}
	if in.DialServerTimeout == 0 {
		in.DialServerTimeout = 10
	}
	if in.DialServerKeepalive == 0 {
		in.DialServerKeepalive = 7200
	}
	if in.TCPMuxKeepaliveInterval == 0 {
		in.TCPMuxKeepaliveInterval = 60
	}
	if in.HeartbeatInterval == 0 {
		in.HeartbeatInterval = 30
	}
	if in.HeartbeatTimeout == 0 {
		in.HeartbeatTimeout = 90
	}
	return in
}

func validAuthMethod(value string) bool {
	return value == "none" || value == "token" || value == "oidc"
}
func validLogLevel(value string) bool {
	return value == "trace" || value == "debug" || value == "info" || value == "warn" || value == "error"
}
func validProtocol(value string) bool {
	return value == "tcp" || value == "kcp" || value == "quic" || value == "websocket" || value == "wss"
}
func validOptionalURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func normalizeProxyInput(in ProxyInput) ProxyInput {
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	in.LocalIP = strings.TrimSpace(in.LocalIP)
	in.HostHeaderRewrite = strings.TrimSpace(in.HostHeaderRewrite)
	in.Remark = strings.TrimSpace(in.Remark)
	in.Options = normalizeProxyOptions(in.Options)
	seen := map[string]bool{}
	domains := make([]string, 0, len(in.CustomDomains))
	for _, domain := range in.CustomDomains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain != "" && !seen[domain] {
			seen[domain] = true
			domains = append(domains, domain)
		}
	}
	in.CustomDomains = domains
	return in
}

func normalizeProxyOptions(in ProxyOptions) ProxyOptions {
	in.Subdomain = strings.TrimSpace(in.Subdomain)
	in.RouteByHTTPUser = strings.TrimSpace(in.RouteByHTTPUser)
	in.HTTPUser = strings.TrimSpace(in.HTTPUser)
	in.HTTPPassword = strings.TrimSpace(in.HTTPPassword)
	in.Multiplexer = strings.ToLower(strings.TrimSpace(in.Multiplexer))
	in.SecretKey = strings.TrimSpace(in.SecretKey)
	for index, user := range in.AllowUsers {
		in.AllowUsers[index] = strings.TrimSpace(user)
	}
	in.Transport.BandwidthLimit = strings.TrimSpace(in.Transport.BandwidthLimit)
	in.Transport.BandwidthLimitMode = strings.ToLower(strings.TrimSpace(in.Transport.BandwidthLimitMode))
	in.Transport.ProxyProtocolVersion = strings.ToLower(strings.TrimSpace(in.Transport.ProxyProtocolVersion))
	in.HealthCheck.Type = strings.ToLower(strings.TrimSpace(in.HealthCheck.Type))
	in.HealthCheck.Path = strings.TrimSpace(in.HealthCheck.Path)
	return in
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}

func validHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || len(host) > 253 || strings.ContainsAny(host, " \t\r\n/\\\"'") {
		return false
	}
	if net.ParseIP(host) != nil || host == "localhost" {
		return true
	}
	return validDomain(host)
}

func validDomain(domain string) bool {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if domain == "" || len(domain) > 253 || strings.Contains(domain, "..") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') && ch != '-' {
				return false
			}
		}
	}
	return true
}
