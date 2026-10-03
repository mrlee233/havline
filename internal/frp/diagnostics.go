package frp

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// 诊断状态常量：ok 已确认可用；failed 已确认失败；unknown 无法确认（区别于失败）。
const (
	diagStateOK      = "ok"
	diagStateFailed  = "failed"
	diagStateUnknown = "unknown"

	diagTCPTimeout       = 3 * time.Second
	diagDNSTimeout       = 2 * time.Second
	diagnosticsCacheTTL  = 15 * time.Second
	diagnosticsMaxInFlight = 4
)

type diagnosticsCacheEntry struct {
	diagnostics ServerDiagnostics
	expiresAt   time.Time
}

// ServerDiagnostics 返回服务端诊断结果。带短 TTL 缓存与并发上限：
// 列表页轮询不应同步等待定位等外部 API，也不能无限叠加探测请求。
func (s *Service) ServerDiagnostics(ctx context.Context, id int64) (ServerDiagnostics, error) {
	server, err := s.store.GetServer(ctx, id)
	if err != nil {
		return ServerDiagnostics{}, err
	}
	return s.runDiagnostics(ctx, server), nil
}

func (s *Service) runDiagnostics(ctx context.Context, server Server) ServerDiagnostics {
	s.diagMu.Lock()
	if entry, ok := s.diagCache[server.ID]; ok && time.Now().Before(entry.expiresAt) {
		cached := entry.diagnostics
		s.diagMu.Unlock()
		return cached
	}
	// 并发上限：超限时返回旧结果或 unknown，不排队等待
	if s.diagInFlight >= diagnosticsMaxInFlight {
		cached, ok := s.diagCache[server.ID]
		s.diagMu.Unlock()
		if ok {
			return cached.diagnostics
		}
		return ServerDiagnostics{ServerID: server.ID, Checks: []DiagnosticCheck{{Name: "tcp_reachable", State: diagStateUnknown, Detail: "探测请求过多，请稍后重试"}}}
	}
	s.diagInFlight++
	s.diagMu.Unlock()
	defer func() {
		s.diagMu.Lock()
		s.diagInFlight--
		s.diagMu.Unlock()
	}()

	result := s.computeDiagnostics(ctx, server)
	s.diagMu.Lock()
	s.diagCache[server.ID] = diagnosticsCacheEntry{diagnostics: result, expiresAt: time.Now().Add(diagnosticsCacheTTL)}
	s.diagMu.Unlock()
	return result
}

// computeDiagnostics 执行各诊断项。定位失败、日志缺失等一律记为 unknown 或非阻断展示，
// 绝不写入 Error（Error 仅表示控制端口探测失败）。
func (s *Service) computeDiagnostics(ctx context.Context, server Server) ServerDiagnostics {
	dnsCheck := diagnoseDNS(ctx, server.ServerAddr)
	tcpCheck := diagnoseTCP(ctx, server, dnsCheck)
	diagnostics := ServerDiagnostics{
		ServerID:  server.ID,
		Available: tcpCheck.State == diagStateOK,
		LatencyMS: tcpCheck.LatencyMS,
		Checks:    []DiagnosticCheck{tcpCheck, s.diagnoseFRPCLogin(server.ID), dnsCheck},
	}
	if tcpCheck.State == diagStateFailed {
		diagnostics.Error = tcpCheck.Detail
	}

	// location：非阻断展示项。缓存命中直接返回；未命中先展示 IP 占位并后台刷新，
	// 不让列表页同步等待外部定位 API。
	location, resolved := s.locationSnapshot(ctx, server.ServerAddr)
	diagnostics.Location = location
	locationCheck := DiagnosticCheck{Name: "location", State: diagStateOK, Detail: location}
	if !resolved {
		locationCheck.State = diagStateUnknown
		locationCheck.Detail = "定位信息后台获取中"
		go s.refreshLocation(server.ServerAddr)
	}
	diagnostics.Checks = append(diagnostics.Checks, locationCheck)
	s.store.AddServerDiagnostic(ctx, diagnostics)
	return diagnostics
}

// diagnoseTCP 控制端口 TCP 探测：只证明端口可达，不涉及认证与代理可用性。
func diagnoseTCP(ctx context.Context, server Server, dnsCheck DiagnosticCheck) DiagnosticCheck {
	check := DiagnosticCheck{Name: "tcp_reachable"}
	if dnsCheck.State == diagStateFailed {
		check.State = diagStateFailed
		check.Detail = "已跳过：" + dnsCheck.Detail
		return check
	}
	dialer := net.Dialer{Timeout: diagTCPTimeout}
	started := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(server.ServerAddr, fmt.Sprint(server.ServerPort)))
	check.LatencyMS = time.Since(started).Milliseconds()
	if check.LatencyMS < 1 {
		check.LatencyMS = 1
	}
	if err != nil {
		check.State = diagStateFailed
		check.Detail = diagnoseDialError(err)
		return check
	}
	_ = conn.Close()
	check.State = diagStateOK
	check.Detail = fmt.Sprintf("控制端口 %d 可达", server.ServerPort)
	return check
}

// diagnoseDNS 记录域名解析结果：A/AAAA 地址、解析耗时；TTL 由系统解析器决定，标准库无法读取，不展示。
func diagnoseDNS(ctx context.Context, host string) DiagnosticCheck {
	check := DiagnosticCheck{Name: "dns_resolution"}
	if ip := net.ParseIP(host); ip != nil {
		check.State = diagStateOK
		check.Detail = fmt.Sprintf("配置为 IP 地址 %s，未经过域名解析", ip)
		return check
	}
	resolver := net.DefaultResolver
	ctxTimeout, cancel := context.WithTimeout(ctx, diagDNSTimeout)
	defer cancel()
	started := time.Now()
	addresses, err := resolver.LookupIPAddr(ctxTimeout, host)
	elapsed := time.Since(started).Milliseconds()
	if err != nil || len(addresses) == 0 {
		check.State = diagStateFailed
		check.Detail = "域名解析失败：" + diagnoseResolveError(err)
		return check
	}
	var ipv4, ipv6 []string
	for _, address := range addresses {
		if address.IP.To4() != nil {
			ipv4 = append(ipv4, address.IP.String())
		} else {
			ipv6 = append(ipv6, address.IP.String())
		}
	}
	summary := "系统解析器；A=" + strings.Join(ipv4, ", ")
	if len(ipv6) > 0 {
		summary += "；AAAA=" + strings.Join(ipv6, ", ")
	} else {
		summary += "；AAAA=无"
	}
	check.LatencyMS = elapsed
	check.Detail = fmt.Sprintf("%s（解析耗时 %d ms）", summary, elapsed)
	return check
}

func diagnoseResolveError(err error) string {
	if err == nil {
		return "无解析结果"
	}
	if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
		return "域名不存在"
	}
	return err.Error()
}

// diagnoseFRPCLogin 从 frpc 运行日志推断登录状态：ok 已登录，failed 最近一次登录失败，unknown 无证据。
func (s *Service) diagnoseFRPCLogin(serverID int64) DiagnosticCheck {
	check := DiagnosticCheck{Name: "frpc_login"}
	data, err := os.ReadFile(s.serverLogPath(serverID))
	if os.IsNotExist(err) {
		check.State = diagStateUnknown
		check.Detail = "frpc 尚未运行，无登录记录"
		return check
	}
	if err != nil {
		check.State = diagStateUnknown
		check.Detail = "读取 frpc 日志失败"
		return check
	}
	state, detail := summarizeFRPCLogin(string(data))
	check.State = state
	check.Detail = detail
	return check
}

// summarizeFRPCLogin 从日志尾部向前找最近的登录结论；登录成功优先于更早的失败记录。
func summarizeFRPCLogin(log string) (string, string) {
	lines := strings.Split(strings.TrimRight(log, "\n"), "\n")
	const window = 200
	if len(lines) > window {
		lines = lines[len(lines)-window:]
	}
	failureDetail := ""
	for index := len(lines) - 1; index >= 0; index-- {
		line := lines[index]
		if prefix := strings.Index(line, "] "); prefix >= 0 {
			line = line[prefix+2:]
		}
		if strings.Contains(line, "login to server success") {
			return diagStateOK, "frpc 已登录 frps"
		}
		if failureDetail == "" && isLoginFailureLine(line) {
			failureDetail = line
		}
	}
	if failureDetail != "" {
		if len(failureDetail) > 160 {
			failureDetail = failureDetail[:160] + "…"
		}
		return diagStateFailed, "frpc 登录失败：" + failureDetail
	}
	return diagStateUnknown, "日志中暂无登录结果记录"
}

func isLoginFailureLine(line string) bool {
	if strings.Contains(line, "login to the server failed") || strings.Contains(line, "connect to server error") || strings.Contains(line, "authentication failed") {
		return true
	}
	return strings.Contains(line, "token") && strings.Contains(line, "error")
}

// locationSnapshot 返回定位缓存值；未命中时返回 IP 占位（resolved=false），由调用方后台刷新。
func (s *Service) locationSnapshot(ctx context.Context, host string) (string, bool) {
	ip := resolvePublicIP(ctx, host)
	if ip == "" {
		// 内网地址或解析失败：展示主机本身，不发起外部定位，也不视为错误
		return strings.TrimSpace(host), true
	}
	s.locationMu.Lock()
	cached, ok := s.locations[ip]
	s.locationMu.Unlock()
	if ok && time.Now().Before(cached.expiresAt) {
		return cached.value, true
	}
	return ip, false
}

// refreshLocation 后台补全定位缓存；结果只影响展示，不参与可用性判定。
func (s *Service) refreshLocation(host string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = s.serverLocation(ctx, host)
}
