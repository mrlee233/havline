package cloudflared

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// DriftResult 是本地规则派生 ingress 与云端配置的差异。
type DriftResult struct {
	TunnelID int64    `json:"tunnel_id"`
	Managed  bool     `json:"managed"`
	HasDrift bool     `json:"has_drift"`
	Items    []string `json:"items"`
}

// DiagnosticCheck 一项诊断结果。
type DiagnosticCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// DiagnosticsResult 是 Cloudflare 隧道的一键诊断结果。
type DiagnosticsResult struct {
	TunnelID    int64             `json:"tunnel_id"`
	GeneratedAt string            `json:"generated_at"`
	Checks      []DiagnosticCheck `json:"checks"`
}

// PreflightCheck 是接入向导的一项预检结果。
type PreflightCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

// PreflightResult 是 Cloudflare 接入前的整体检查结果。
type PreflightResult struct {
	Ready  bool             `json:"ready"`
	Checks []PreflightCheck `json:"checks"`
}

// Preflight 检查新增 Cloudflare 出口前最容易出错的配置。
func (s *Service) Preflight(ctx context.Context) PreflightResult {
	binary := s.binaryCheck()
	checks := []PreflightCheck{{Name: binary.Name, Status: binary.Status, Message: binary.Message}}
	row, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		return PreflightResult{Ready: false, Checks: append(checks, PreflightCheck{Name: "应用配置", Status: "error", Message: err.Error()})}
	}
	checks = append(checks, PreflightCheck{
		Name: "Account ID", Status: statusBool(strings.TrimSpace(row.AccountID) != ""), Message: firstNonEmpty(row.AccountID, "未填写 Account ID"), Action: "settings",
	})
	checks = append(checks, PreflightCheck{
		Name: "API Token", Status: statusBool(row.APITokenEnc != ""), Message: boolText(row.APITokenEnc != "", "已保存 API Token", "未保存 API Token"), Action: "settings",
	})
	if row.APITokenEnc == "" {
		return PreflightResult{Ready: false, Checks: checks}
	}
	api, err := s.cloudflareAPI(ctx)
	if err != nil {
		return PreflightResult{Ready: false, Checks: append(checks, PreflightCheck{Name: "Cloudflare API", Status: "error", Message: err.Error(), Action: "settings"})}
	}
	zones, err := api.ListZones(ctx)
	if err != nil {
		return PreflightResult{Ready: false, Checks: append(checks, PreflightCheck{Name: "Zone 权限", Status: "error", Message: err.Error(), Action: "settings"})}
	}
	checks = append(checks, PreflightCheck{
		Name: "Zone 权限", Status: statusBool(len(zones) > 0), Message: fmt.Sprintf("可访问 %d 个 Cloudflare Zone", len(zones)), Action: "settings",
	})
	return PreflightResult{Ready: !hasPreflightError(checks), Checks: checks}
}

func statusBool(ok bool) string {
	if ok {
		return "ok"
	}
	return "error"
}

func boolText(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func hasPreflightError(checks []PreflightCheck) bool {
	for _, check := range checks {
		if check.Status == "error" {
			return true
		}
	}
	return false
}

// Drift 比较本地规则派生 ingress 与 Cloudflare 云端配置。
func (s *Service) Drift(ctx context.Context, id int64) (DriftResult, error) {
	tunnel, err := s.store.Get(ctx, id)
	if err != nil {
		return DriftResult{}, err
	}
	result := DriftResult{TunnelID: id, Managed: tunnel.Managed, Items: []string{}}
	if !tunnel.Managed {
		return result, nil
	}
	rules, err := s.rules.List(ctx)
	if err != nil {
		return DriftResult{}, err
	}
	expected := BuildIngressForRules(rulesForTunnel(rules, tunnel.ID))
	api, err := s.cloudflareAPI(ctx)
	if err != nil {
		return DriftResult{}, err
	}
	actual, err := api.GetIngress(ctx, tunnel.TunnelID)
	if err != nil {
		return DriftResult{}, err
	}
	result.Items = compareIngress(expected, actual)
	result.HasDrift = len(result.Items) > 0
	return result, nil
}

// PushTunnel 以本地规则为准重新覆盖云端 ingress。
func (s *Service) PushTunnel(ctx context.Context, id int64) error {
	tunnel, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !tunnel.Managed {
		return fmt.Errorf("手工隧道不支持从规则覆盖，请先在编辑中开启自动托管")
	}
	return s.syncManagedTunnel(ctx, tunnel)
}

func compareIngress(expected, actual []IngressRule) []string {
	expectedMap := ingressMap(expected)
	actualMap := ingressMap(actual)
	items := []string{}
	keys := unionKeys(expectedMap, actualMap)
	for _, key := range keys {
		want, wantOK := expectedMap[key]
		got, gotOK := actualMap[key]
		switch {
		case wantOK && !gotOK:
			items = append(items, fmt.Sprintf("%s 本地存在，云端缺失", key))
		case !wantOK && gotOK:
			items = append(items, fmt.Sprintf("%s 云端存在，本地已移除", key))
		case want != got:
			items = append(items, fmt.Sprintf("%s 回源不一致：本地 %s，云端 %s", key, want, got))
		}
	}
	return items
}

func ingressMap(ingress []IngressRule) map[string]string {
	out := map[string]string{}
	for _, item := range ingress {
		key := strings.ToLower(strings.TrimSpace(item.Hostname))
		if key == "" {
			key = "(catch-all)"
		}
		out[key] = strings.TrimSpace(item.Service)
	}
	return out
}

func unionKeys(a, b map[string]string) []string {
	seen := map[string]struct{}{}
	for key := range a {
		seen[key] = struct{}{}
	}
	for key := range b {
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Diagnostics 返回一组面向用户的 Cloudflare 隧道检查项。
func (s *Service) Diagnostics(ctx context.Context, id int64) (DiagnosticsResult, error) {
	tunnel, err := s.store.Get(ctx, id)
	if err != nil {
		return DiagnosticsResult{}, err
	}
	result := DiagnosticsResult{TunnelID: id, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Checks: []DiagnosticCheck{}}
	result.Checks = append(result.Checks, s.binaryCheck())
	result.Checks = append(result.Checks, configCheck(tunnel.ConfigPath))
	result.Checks = append(result.Checks, processCheck(s.manager.Status(id), tunnel))
	api := s.apiDiagnostic(ctx, tunnel)
	result.Checks = append(result.Checks, api)
	result.Checks = append(result.Checks, s.dnsCheck(ctx, tunnel))
	result.Checks = append(result.Checks, networkCheck("Cloudflare edge region1", "region1.v2.argotunnel.com:7844"))
	result.Checks = append(result.Checks, networkCheck("Cloudflare edge region2", "region2.v2.argotunnel.com:7844"))
	return result, nil
}

func (s *Service) binaryCheck() DiagnosticCheck {
	version := s.BinaryVersion()
	if strings.TrimSpace(version) == "" {
		return DiagnosticCheck{Name: "cloudflared 二进制", Status: "error", Message: "未安装或不可执行"}
	}
	return DiagnosticCheck{Name: "cloudflared 二进制", Status: "ok", Message: version}
}

func configCheck(path string) DiagnosticCheck {
	if _, err := os.Stat(path); err != nil {
		return DiagnosticCheck{Name: "本地配置", Status: "error", Message: "config.yml 不存在或不可读"}
	}
	return DiagnosticCheck{Name: "本地配置", Status: "ok", Message: "config.yml 可读"}
}

func processCheck(status RuntimeStatus, tunnel Tunnel) DiagnosticCheck {
	if status.Running && status.Status == "connected" {
		return DiagnosticCheck{Name: "隧道进程", Status: "ok", Message: fmt.Sprintf("已连接，HA 连接数 %d", status.Connections)}
	}
	if tunnel.AutoStart {
		return DiagnosticCheck{Name: "隧道进程", Status: "error", Message: "未运行或未连接：" + status.LastError}
	}
	return DiagnosticCheck{Name: "隧道进程", Status: "warning", Message: "当前未运行，且未启用自动启动"}
}

func (s *Service) apiDiagnostic(ctx context.Context, tunnel Tunnel) DiagnosticCheck {
	api, err := s.cloudflareAPI(ctx)
	if err != nil {
		return DiagnosticCheck{Name: "Cloudflare API", Status: "error", Message: err.Error()}
	}
	if tunnel.TunnelID == "" {
		return DiagnosticCheck{Name: "Cloudflare API", Status: "error", Message: "隧道 ID 为空"}
	}
	if _, err := api.GetIngress(ctx, tunnel.TunnelID); err != nil {
		return DiagnosticCheck{Name: "Cloudflare API", Status: "error", Message: err.Error()}
	}
	return DiagnosticCheck{Name: "Cloudflare API", Status: "ok", Message: "API Token 与云端配置可访问"}
}

func (s *Service) dnsCheck(ctx context.Context, tunnel Tunnel) DiagnosticCheck {
	records, err := s.store.ListDNSRecords(ctx, tunnel.ID)
	if err != nil {
		return DiagnosticCheck{Name: "DNS 记录", Status: "error", Message: err.Error()}
	}
	if len(records) == 0 {
		return DiagnosticCheck{Name: "DNS 记录", Status: "warning", Message: "没有 Havline 接管的 DNS 记录"}
	}
	return DiagnosticCheck{Name: "DNS 记录", Status: "ok", Message: fmt.Sprintf("已接管 %d 条记录", len(records))}
}

func networkCheck(name, address string) DiagnosticCheck {
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		return DiagnosticCheck{Name: name, Status: "warning", Message: "连接失败：" + err.Error()}
	}
	_ = conn.Close()
	return DiagnosticCheck{Name: name, Status: "ok", Message: "TCP 7844 可连接"}
}

// RouteTestResult 是单条路由源服务的连通性测试结果。
type RouteTestResult struct {
	OK        bool    `json:"ok"`
	Message   string  `json:"message"`
	LatencyMS float64 `json:"latency_ms"`
}

// TestService 测试一条路由 service 的 TCP 连通性。
func (s *Service) TestService(ctx context.Context, service string) (RouteTestResult, error) {
	address, err := serviceAddress(service)
	if err != nil {
		return RouteTestResult{}, err
	}
	start := time.Now()
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return RouteTestResult{OK: false, Message: err.Error()}, nil
	}
	_ = conn.Close()
	return RouteTestResult{OK: true, Message: "源服务 TCP 可连接", LatencyMS: float64(time.Since(start).Milliseconds())}, nil
}

func serviceAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		if !strings.Contains(raw, ":") {
			return "", fmt.Errorf("service 格式无效：%s", raw)
		}
		return raw, nil
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		switch parsed.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		case "ssh":
			port = "22"
		default:
			return "", fmt.Errorf("service 缺少端口：%s", raw)
		}
	}
	return net.JoinHostPort(host, port), nil
}
