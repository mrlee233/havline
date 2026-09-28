// Package nginxtmpl 提供本机 Nginx 与公网 Agent 共用的纯文本模板函数。
//
// 这里只做字符串渲染，不读文件、不执行进程、不依赖业务模型，
// 因此 internal/nginx 与 internal/agent 都可以安全导入而不产生循环依赖。
package nginxtmpl

import (
	"fmt"
	"strings"
)

// DefaultIndent 是 Nginx server 块内指令的默认缩进。
const DefaultIndent = "    "

// TLSConfig 描述证书与 TLS 协议版本偏好。
type TLSConfig struct {
	CertPath  string
	KeyPath   string
	TLS13Only bool
}

// SecurityHeaders 描述安全响应头；HTTPS 为 false 时不输出 HSTS。
type SecurityHeaders struct {
	Enabled           bool
	HTTPS             bool
	HSTSMaxAgeSeconds int
}

// AccessControl 描述 allow / deny 顺序；Allow 非空时末尾自动追加 deny all。
// ChinaDeny 是可选的完整指令片段，插在 deny 黑名单之后、allow 白名单之前，
// 让两端共享顺序结构，同时保留各自的 geo 变量名。
type AccessControl struct {
	Allow     []string
	Deny      []string
	ChinaDeny string
}

// LimitConfig 描述 location 级限流指令；Zone 是已合法化的 zone 前缀。
type LimitConfig struct {
	Zone  string
	Rate  int
	Burst int
	Conn  int
}

// SSLDirectives 生成证书与 TLS 协议指令，缩进固定为 server 块内层级。
func SSLDirectives(cfg TLSConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%sssl_certificate %s;\n", DefaultIndent, cfg.CertPath)
	fmt.Fprintf(&b, "%sssl_certificate_key %s;\n", DefaultIndent, cfg.KeyPath)
	if cfg.TLS13Only {
		fmt.Fprintf(&b, "%sssl_protocols TLSv1.3;\n", DefaultIndent)
		return b.String()
	}
	fmt.Fprintf(&b, "%sssl_protocols TLSv1.2 TLSv1.3;\n", DefaultIndent)
	return b.String()
}

// ServerSecurityHeaders 生成统一的安全响应头；HSTS 只在 HTTPS 且 Enabled 时输出。
func ServerSecurityHeaders(cfg SecurityHeaders) string {
	if !cfg.Enabled || !cfg.HTTPS {
		return ""
	}
	maxAge := cfg.HSTSMaxAgeSeconds
	if maxAge <= 0 {
		maxAge = 31536000
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%sadd_header Strict-Transport-Security \"max-age=%d; includeSubDomains\" always;\n", DefaultIndent, maxAge)
	fmt.Fprintf(&b, "%sadd_header X-Content-Type-Options \"nosniff\" always;\n", DefaultIndent)
	fmt.Fprintf(&b, "%sadd_header X-Frame-Options \"SAMEORIGIN\" always;\n", DefaultIndent)
	fmt.Fprintf(&b, "%sadd_header Referrer-Policy \"strict-origin-when-cross-origin\" always;\n", DefaultIndent)
	return b.String()
}

// AccessDirectives 生成 allow / deny：先 deny 黑名单，再 allow 白名单，最后 deny all。
func AccessDirectives(cfg AccessControl, indent string) string {
	var b strings.Builder
	for _, ip := range cfg.Deny {
		if ip = strings.TrimSpace(ip); ip != "" {
			fmt.Fprintf(&b, "%sdeny %s;\n", indent, ip)
		}
	}
	b.WriteString(strings.TrimRight(cfg.ChinaDeny, "\n"))
	if strings.TrimSpace(cfg.ChinaDeny) != "" {
		b.WriteString("\n")
	}
	allowCount := 0
	for _, ip := range cfg.Allow {
		if ip = strings.TrimSpace(ip); ip != "" {
			fmt.Fprintf(&b, "%sallow %s;\n", indent, ip)
			allowCount++
		}
	}
	if allowCount > 0 {
		fmt.Fprintf(&b, "%sdeny all;\n", indent)
	}
	return b.String()
}

// LimitDirectives 生成 location 级限流指令：连接数在前，请求速率在后。
func LimitDirectives(cfg LimitConfig, indent string) string {
	zone := strings.TrimSpace(cfg.Zone)
	if zone == "" {
		return ""
	}
	var b strings.Builder
	if cfg.Conn > 0 {
		fmt.Fprintf(&b, "%slimit_conn %s_conn %d;\n", indent, zone, cfg.Conn)
	}
	if cfg.Rate > 0 {
		if cfg.Burst > 0 {
			fmt.Fprintf(&b, "%slimit_req zone=%s_req burst=%d nodelay;\n", indent, zone, cfg.Burst)
		} else {
			fmt.Fprintf(&b, "%slimit_req zone=%s_req nodelay;\n", indent, zone)
		}
	}
	return b.String()
}

// UpgradeHeaders 生成 WebSocket 代理头；connectionExpr 由调用方提供（map 变量或字面量）。
// proxy_http_version 由调用方保留在原有位置，避免改变既有输出顺序。
func UpgradeHeaders(indent, connectionExpr string) string {
	connectionExpr = strings.TrimSpace(connectionExpr)
	if connectionExpr == "" {
		connectionExpr = "$connection_upgrade"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%sproxy_set_header Upgrade $http_upgrade;\n", indent)
	fmt.Fprintf(&b, "%sproxy_set_header Connection %s;\n", indent, connectionExpr)
	return b.String()
}

// RateLimitZone 生成 http 上下文的请求速率 zone 定义；zone 需包含完整后缀。
func RateLimitZone(zone string, rate int) string {
	zone = strings.TrimSpace(zone)
	if zone == "" || rate <= 0 {
		return ""
	}
	return fmt.Sprintf("limit_req_zone $binary_remote_addr zone=%s:10m rate=%dr/s;\n", zone, rate)
}

// ConnLimitZone 生成 http 上下文的连接数 zone 定义；zone 需包含完整后缀。
func ConnLimitZone(zone string) string {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return ""
	}
	return fmt.Sprintf("limit_conn_zone $binary_remote_addr zone=%s:10m;\n", zone)
}

// ZoneName 把前缀与片段合法化为 zone 名：只保留小写字母与数字，其余换成下划线。
func ZoneName(prefix string, parts ...string) string {
	var b strings.Builder
	writeZonePart(&b, prefix)
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('_')
		}
		writeZonePart(&b, part)
	}
	return b.String()
}

func writeZonePart(b *strings.Builder, part string) {
	for _, r := range strings.ToLower(part) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
}
