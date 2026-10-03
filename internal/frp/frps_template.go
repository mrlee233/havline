package frp

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// buildFRPSServerConfig 把多服务端模块的 frpc 侧服务端记录渲染为 VPS 侧的 frps.toml，
// bindPort、认证、管理接口与 frpc 配置同源生成，消除两端错配（官方 BuildFRPSConfig 的多服务端版）。
// 密钥只在生成前一刻解密进模板，永不写入日志或其他 API 响应。
func (s *Service) buildFRPSServerConfig(ctx context.Context, server Server) (string, error) {
	// token 认证才需要取同一份 token；none / oidc 不涉及
	token := ""
	if server.Options.AuthMethod == "token" {
		encoded, err := s.store.Token(ctx, server.ID)
		if err != nil {
			return "", err
		}
		if encoded != "" {
			token, err = s.secretBox.Decrypt(encoded)
			if err != nil {
				return "", fmt.Errorf("FRP Token 无法解密，请重新保存服务端配置")
			}
		}
	}

	dashboardOn := server.Options.DashboardPort > 0 && strings.TrimSpace(server.Options.DashboardAddr) != ""
	hasVhost, vhostErr := s.hasVhostProxies(ctx, server.ID)
	// subDomainHost 缺口检测：frpc 侧规则填了 subdomain 时，frps 必须配置 subDomainHost 才生效
	proxies, proxyErr := s.store.ListProxiesByServer(ctx, server.ID)
	hasSubdomain := false
	if proxyErr == nil {
		for _, p := range proxies {
			if strings.TrimSpace(p.Options.Subdomain) != "" {
				hasSubdomain = true
				break
			}
		}
	}

	var b strings.Builder
	b.WriteString("# 由 Havline 生成：与本服务端的 frpc 配置同源（bindPort/auth 与客户端一致）\n")
	b.WriteString("# 格式：frp v0.52.0+ 的 TOML 配置（INI 自 v0.52.0 起已被官方弃用）\n")
	b.WriteString("# 用法：保存为 VPS 上的 frps.toml，执行 ./frps -c frps.toml\n\n")
	// TOML 规范：顶层标量键必须写在任何 [table] 段之前，否则会被归入上一个表
	b.WriteString("bindAddr = \"0.0.0.0\"\n")
	b.WriteString(fmt.Sprintf("bindPort = %d\n", server.ServerPort))
	switch server.Options.Protocol {
	case "kcp":
		b.WriteString(fmt.Sprintf("kcpBindPort = %d # frpc 使用 kcp 协议接入；官方允许与 bindPort 相同（UDP）\n", server.ServerPort))
	case "quic":
		b.WriteString(fmt.Sprintf("quicBindPort = %d # frpc 使用 quic 协议接入；UDP 端口官方允许与 bindPort 相同，如冲突请改为独立端口\n", server.ServerPort))
	}
	if vhostErr == nil && hasVhost {
		// 官方默认 vhost 端口为 0（禁用）。默认 8080/8443：80/443 由 Nginx 占用（Nginx 80/443 →
		// proxy_pass 127.0.0.1:8080 → frps vhost → 隧道），可在服务端表单自定义，与 Nginx upstream 同源
		httpPort := server.Options.VhostHTTPPort
		if httpPort <= 0 {
			httpPort = 8080
		}
		httpsPort := server.Options.VhostHTTPSPort
		if httpsPort <= 0 {
			httpsPort = 8443
		}
		b.WriteString(fmt.Sprintf("vhostHTTPPort = %d\n", httpPort))
		b.WriteString(fmt.Sprintf("vhostHTTPSPort = %d\n", httpsPort))
	}
	// 子域名根：规则填了 subdomain 时必须配置，否则 subdomain 不生效
	if host := strings.TrimSpace(server.Options.SubDomainHost); host != "" {
		b.WriteString(fmt.Sprintf("subDomainHost = %q\n", host))
	}
	// 自定义 404 页面（HTTP vhost 命中不到规则时的响应）
	if page := strings.TrimSpace(server.Options.Custom404Page); page != "" {
		b.WriteString(fmt.Sprintf("custom404Page = %q\n", page))
	}
	// tcpmux 规则的硬依赖：存在 tcpmux 规则时必须配置监听端口
	if port := server.Options.TCPMuxHTTPConnectPort; port > 0 {
		b.WriteString(fmt.Sprintf("tcpmuxHTTPConnectPort = %d\n", port))
	}
	// 强制 TLS：公网 frps 标准安全实践；开启后 frpc 不开 TLS 将无法连接
	if server.Options.TLSForce {
		b.WriteString("transport.tls.force = true # 只接受 TLS 连接（frpc 侧必须启用 transport.tls.enable）\n")
	}
	// 传输参数与 frpc 侧同源：两端不一致会导致连不上或池被截断
	if !server.Options.TCPMux {
		b.WriteString("transport.tcpMux = false # 与 frpc 侧 tcp_mux 同源（官方默认 true，仅在关闭时输出）\n")
	}
	if server.Options.PoolCount > 0 {
		b.WriteString(fmt.Sprintf("transport.maxPoolCount = %d # 不低于 frpc 侧 pool_count，避免连接池被静默截断\n", server.Options.PoolCount))
	}
	// 端口白名单：逗号分隔的单端口或「起-止」范围，如 6000,6005-6010,7000-8000；留空不限制
	if ports := strings.TrimSpace(server.Options.AllowPorts); ports != "" {
		b.WriteString(fmt.Sprintf("allowPorts = [\n%s\n]\n", renderAllowPorts(ports)))
	}
	// 每客户端可绑定端口上限；0 表示不限制
	if maxPorts := server.Options.MaxPortsPerClient; maxPorts > 0 {
		b.WriteString(fmt.Sprintf("maxPortsPerClient = %d\n", maxPorts))
	}
	// frps 文件日志：journalctl 只保留尾部，排障需完整历史时开启；写入 /var/log/frps.log
	if server.Options.FrpsLogToFile {
		maxDays := server.Options.FrpsLogMaxDays
		if maxDays <= 0 {
			maxDays = 3
		}
		b.WriteString("\nlog.to = \"/var/log/frps.log\"\n")
		b.WriteString(fmt.Sprintf("log.maxDays = %d\n", maxDays))
	}
	if !server.TLS {
		b.WriteString("# 安全提示：frpc 侧未启用 TLS，frps 可加 transport.tls.force = true 强制 TLS（开启后 frpc 不开 TLS 将无法连接）\n")
	}
	switch server.Options.AuthMethod {
	case "token":
		if token != "" {
			// 点式键与官方 frps_full_example.toml 同款（TOML 语义上与 [auth] 表等价）
			b.WriteString("\nauth.method = \"token\"\n")
			b.WriteString(fmt.Sprintf("auth.token = %q\n", token))
		} else {
			b.WriteString("\n# 注意：frpc 侧为 Token 认证但尚未保存 Token；保存后在 Havline 重新生成本文件以保持两端一致\n")
		}
	case "none":
		b.WriteString("\n# frpc 侧为无认证（auth.method = \"none\"），frps 无需 [auth] 配置；\n")
		b.WriteString("# 公网暴露的 frps 建议配置认证或用 transport.tls.force = true 强制 TLS，防止任意客户端接入\n")
	case "oidc":
		b.WriteString("\n# frpc 侧使用 OIDC 认证；frps 侧的 oidc.* 校验参数（issuer、client_id 等）依赖你的身份提供商，\n")
		b.WriteString("# 请参考官方 frps_full_example.toml 的 [auth] oidc 段手动配置\n")
	}
	if server.TLS {
		b.WriteString("\n# frpc 侧已启用 transport.tls.enable；frps 的 transport.tls.force 默认为 false，无需额外 TLS 配置即可接受 TLS 连接\n")
	}

	// webServer（frps 管理接口）：与客户端 options.dashboard_* 同源
	if dashboardOn {
		b.WriteString("\nwebServer.addr = \"0.0.0.0\"\n")
		b.WriteString(fmt.Sprintf("webServer.port = %d\n", server.Options.DashboardPort))
		if user := strings.TrimSpace(server.Options.DashboardUser); user != "" {
			b.WriteString(fmt.Sprintf("webServer.user = %q\n", user))
		}
		if encoded, err := s.store.DashboardPassword(ctx, server.ID); err == nil && encoded != "" {
			if password, err := s.secretBox.Decrypt(encoded); err == nil && password != "" {
				b.WriteString(fmt.Sprintf("webServer.password = %q\n", password))
			}
		}
		b.WriteString("\n# 安全提示：webServer 暴露穿透流量与连接信息，建议用防火墙限制仅 Havline 所在机器访问该端口\n")
	}

	if hasSubdomain {
		b.WriteString("\n# 注意：该服务端下存在填写 subdomain 的规则；frps 需配置 subDomainHost（如 frps.example.com）后，\n")
		b.WriteString("# 规则的 subdomain 字段才会生效为 sub.frps.example.com；域名 DNS 也需指向本 VPS\n")
	}
	b.WriteString("\n# 防火墙放行清单（按本服务端当前配置生成）\n")
	b.WriteString(fmt.Sprintf("# %d/tcp  # frps 控制端口\n", server.ServerPort))
	if dashboardOn {
		b.WriteString(fmt.Sprintf("# %d/tcp  # frps 管理接口（webServer）\n", server.Options.DashboardPort))
	}
	if vhostErr == nil && hasVhost {
		httpPort := server.Options.VhostHTTPPort
		if httpPort <= 0 {
			httpPort = 8080
		}
		httpsPort := server.Options.VhostHTTPSPort
		if httpsPort <= 0 {
			httpsPort = 8443
		}
		b.WriteString(fmt.Sprintf("# %d, %d/tcp  # 上方 vhostHTTPPort / vhostHTTPSPort 对应端口（仅本机 Nginx 回源需要，可不对公网开放）\n", httpPort, httpsPort))
	}
	return b.String(), nil
}

// hasVhostProxies 判断该服务端下是否存在依赖 vhost 端口的规则（http/https/tcpmux）。
func (s *Service) hasVhostProxies(ctx context.Context, serverID int64) (bool, error) {
	proxies, err := s.store.ListProxiesByServer(ctx, serverID)
	if err != nil {
		return false, err
	}
	for _, p := range proxies {
		if p.Type == "http" || p.Type == "https" || p.Type == "tcpmux" {
			return true, nil
		}
	}
	return false, nil
}

// renderAllowPorts 把表单端口白名单文本（如 "6000,6005-6010,7000-8000"）渲染为 TOML 数组行；
// 非法片段直接忽略，不中断生成。
func renderAllowPorts(ports string) string {
	var lines []string
	for _, part := range strings.Split(ports, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if start, end, ok := strings.Cut(part, "-"); ok {
			startN, err1 := strconv.Atoi(strings.TrimSpace(start))
			endN, err2 := strconv.Atoi(strings.TrimSpace(end))
			if err1 == nil && err2 == nil && startN > 0 && endN >= startN && endN <= 65535 {
				lines = append(lines, fmt.Sprintf("{ start = %d, end = %d },", startN, endN))
			}
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n > 0 && n <= 65535 {
			lines = append(lines, fmt.Sprintf("{ single = %d },", n))
		}
	}
	return strings.Join(lines, "\n")
}

// frpsSecretLinePattern 匹配带着敏感值的 frps.toml 行
var frpsSecretLinePattern = regexp.MustCompile(`(?i)^\s*(auth\.token|auth\.oidc\.clientsecret)\s*=`)

// maskFRPSSecrets 把 frps.toml 里的敏感值换成掩码：保留键名与行结构，
// 让用户在预览里仍能看清「配了哪些认证项」，但不下发明文密钥。
// 只用于预览；下发路径（PushFRPSConfigToAgent）必须用未脱敏的内容。
func maskFRPSSecrets(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !frpsSecretLinePattern.MatchString(line) {
			continue
		}
		key := strings.TrimSpace(strings.SplitN(line, "=", 2)[0])
		lines[i] = fmt.Sprintf("%s = %q", key, maskedToken)
	}
	return strings.Join(lines, "\n")
}

// FRPSServerConfig 返回指定服务端的 frps.toml 同源配置（对外 API 入口）。
// 这是**预览**入口：密钥一律脱敏（明文只在 PushFRPSConfigToAgent 的下发路径里出现）。
func (s *Service) FRPSServerConfig(ctx context.Context, id int64) (string, error) {
	server, err := s.store.GetServer(ctx, id)
	if err != nil {
		return "", err
	}
	content, err := s.buildFRPSServerConfig(ctx, server)
	if err != nil {
		return "", err
	}
	return maskFRPSSecrets(content), nil
}
