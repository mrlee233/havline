package cloudflared

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/havline/havline/internal/proxy"
)

type tokenPayload struct {
	AccountTag   string `json:"a"`
	TunnelID     string `json:"t"`
	TunnelSecret string `json:"s"`
}

// DecodeToken 解析 Cloudflare Tunnel token；兼容 a/t/s 与 AccountTag/TunnelID/TunnelSecret 两种字段名。
func DecodeToken(token string) (accountTag, tunnelID, tunnelSecret string, err error) {
	clean := strings.NewReplacer("\n", "", "\r", "", " ", "", "\t", "").Replace(strings.TrimSpace(token))
	if clean == "" {
		return "", "", "", fmt.Errorf("token 为空")
	}
	raw, err := decodeBase64(clean)
	if err != nil {
		return "", "", "", err
	}
	var payload tokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.TunnelID == "" {
		var alt struct {
			AccountTag   string `json:"AccountTag"`
			TunnelID     string `json:"TunnelID"`
			TunnelSecret string `json:"TunnelSecret"`
		}
		if err2 := json.Unmarshal(raw, &alt); err2 != nil {
			return "", "", "", fmt.Errorf("token JSON 解析失败: %w", err)
		}
		payload.AccountTag, payload.TunnelID, payload.TunnelSecret = alt.AccountTag, alt.TunnelID, alt.TunnelSecret
	}
	if payload.AccountTag == "" || payload.TunnelID == "" || payload.TunnelSecret == "" {
		return "", "", "", fmt.Errorf("token 缺少 AccountTag / TunnelID / TunnelSecret")
	}
	return payload.AccountTag, payload.TunnelID, payload.TunnelSecret, nil
}

// decodeBase64 同时兼容标准 base64 与 URL-safe base64，并兼容有无填充两种形式。
// Cloudflare 面板生成的 Tunnel Token 使用 URL-safe 编码，直接用标准 base64 会在 '-' / '_' 处失败。
func decodeBase64(input string) ([]byte, error) {
	encodings := []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	}
	for _, encoding := range encodings {
		if data, err := encoding.DecodeString(input); err == nil {
			return data, nil
		}
	}
	padded := input
	if remainder := len(padded) % 4; remainder != 0 {
		padded += strings.Repeat("=", 4-remainder)
	}
	for _, encoding := range encodings {
		if data, err := encoding.DecodeString(padded); err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("token base64 解码失败：请确认粘贴的是完整的 Cloudflare 隧道 token")
}

// CredentialsJSON 生成 cloudflared 需要的 creds.json 内容。
func CredentialsJSON(accountTag, tunnelID, tunnelSecret string) string {
	data, _ := json.MarshalIndent(map[string]string{
		"AccountTag":   accountTag,
		"TunnelID":     tunnelID,
		"TunnelSecret": tunnelSecret,
	}, "", "  ")
	return string(data)
}

// BuildConfig 生成 locally-managed 的 config.yml；ingress 末条由调用方保证是 catch-all。
func BuildConfig(tunnelID, credsPath string, network NetworkSettings, ingress []IngressRule) string {
	var b strings.Builder
	fmt.Fprintf(&b, "tunnel: %s\n", tunnelID)
	fmt.Fprintf(&b, "credentials-file: %s\n\n", credsPath)
	writeNetworkSection(&b, network)
	b.WriteString("ingress:\n")
	for _, rule := range ingress {
		if rule.Hostname != "" {
			fmt.Fprintf(&b, "  - hostname: %s\n", rule.Hostname)
			if path := strings.TrimSpace(rule.Path); path != "" && path != "*" {
				fmt.Fprintf(&b, "    path: %s\n", path)
			}
			fmt.Fprintf(&b, "    service: %s\n", rule.Service)
			continue
		}
		fmt.Fprintf(&b, "  - service: %s\n", rule.Service)
	}
	return b.String()
}

func writeNetworkSection(b *strings.Builder, network NetworkSettings) {
	protocol := strings.TrimSpace(network.TransportProtocol)
	edge := strings.TrimSpace(network.EdgeIPVersion)
	hasOrigin := strings.TrimSpace(network.OriginCAPool) != "" || network.NoTLSVerify || strings.TrimSpace(network.HTTPHostHeader) != ""
	if protocol != "" && protocol != "auto" {
		fmt.Fprintf(b, "protocol: %s\n", protocol)
	}
	if edge != "" && edge != "auto" {
		fmt.Fprintf(b, "edge-ip-version: \"%s\"\n", edge)
	}
	if network.HAConnections > 0 {
		fmt.Fprintf(b, "ha-connections: %d\n", network.HAConnections)
	}
	if hasOrigin {
		b.WriteString("originRequest:\n")
		if pool := strings.TrimSpace(network.OriginCAPool); pool != "" {
			fmt.Fprintf(b, "  caPool: %s\n", pool)
		}
		if network.NoTLSVerify {
			b.WriteString("  noTLSVerify: true\n")
		}
		if header := strings.TrimSpace(network.HTTPHostHeader); header != "" {
			fmt.Fprintf(b, "  httpHostHeader: %s\n", header)
		}
	}
	if protocol != "" || edge != "" || network.HAConnections > 0 || hasOrigin {
		b.WriteString("\n")
	}
}

// BuildIngress 从域名列表生成 ingress，锁定三条不变式：末条 catch-all、去重、稳定排序。
func BuildIngress(hostnames []string, service string) []IngressRule {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(hostnames))
	for _, host := range hostnames {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		cleaned = append(cleaned, host)
	}
	sort.Strings(cleaned)
	out := make([]IngressRule, 0, len(cleaned)+1)
	for _, host := range cleaned {
		out = append(out, IngressRule{Hostname: host, Service: service})
	}
	return append(out, IngressRule{Service: "http_status:404"})
}

// BuildIngressForRules 从反向代理规则派生 ingress，每条域名使用自己的回源地址。
func BuildIngressForRules(rules []proxy.Rule) []IngressRule {
	serviceByHost := map[string]string{}
	for _, rule := range rules {
		if !rule.CloudflareEnabled() {
			continue
		}
		service := strings.TrimSpace(rule.Upstream)
		if service == "" {
			continue
		}
		for _, host := range rule.Hostnames() {
			serviceByHost[host] = service
		}
	}
	hosts := make([]string, 0, len(serviceByHost))
	for host := range serviceByHost {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	out := make([]IngressRule, 0, len(hosts)+1)
	for _, host := range hosts {
		out = append(out, IngressRule{Hostname: host, Service: serviceByHost[host]})
	}
	return append(out, IngressRule{Service: "http_status:404"})
}

// NormalizeService 生成回源 service；localhost 归一化为 127.0.0.1。
func NormalizeService(protocol, host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "localhost" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s://%s:%d", protocol, host, port)
}

// MaskSecret 只保留首尾少量字符，用于 API 回显与日志。
func MaskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + "********" + value[len(value)-4:]
}
