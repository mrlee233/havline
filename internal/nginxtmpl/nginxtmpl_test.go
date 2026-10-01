package nginxtmpl

import (
	"strings"
	"testing"
)

func TestSSLDirectives(t *testing.T) {
	base := SSLDirectives(TLSConfig{CertPath: "/certs/fullchain.pem", KeyPath: "/certs/privkey.pem"})
	for _, want := range []string{
		"ssl_certificate /certs/fullchain.pem;",
		"ssl_certificate_key /certs/privkey.pem;",
		"ssl_protocols TLSv1.2 TLSv1.3;",
	} {
		if !strings.Contains(base, want) {
			t.Fatalf("默认 TLS 缺少 %q：%s", want, base)
		}
	}
	only13 := SSLDirectives(TLSConfig{CertPath: "/c", KeyPath: "/k", TLS13Only: true})
	if !strings.Contains(only13, "ssl_protocols TLSv1.3;") || strings.Contains(only13, "TLSv1.2") {
		t.Fatalf("TLS 1.3 only 输出不正确：%s", only13)
	}
}

func TestServerSecurityHeaders(t *testing.T) {
	if got := ServerSecurityHeaders(SecurityHeaders{Enabled: true, HTTPS: false}); got != "" {
		t.Fatalf("HTTP 不应输出 HSTS：%s", got)
	}
	if got := ServerSecurityHeaders(SecurityHeaders{Enabled: false, HTTPS: true}); got != "" {
		t.Fatalf("关闭安全头时不应输出：%s", got)
	}
	got := ServerSecurityHeaders(SecurityHeaders{Enabled: true, HTTPS: true})
	for _, want := range []string{
		`Strict-Transport-Security "max-age=31536000; includeSubDomains"`,
		`X-Content-Type-Options "nosniff"`,
		`X-Frame-Options "SAMEORIGIN"`,
		`Referrer-Policy "strict-origin-when-cross-origin"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("安全头缺少 %q：%s", want, got)
		}
	}
}

func TestAccessDirectives(t *testing.T) {
	got := AccessDirectives(AccessControl{
		Deny:      []string{"8.8.8.8", " "},
		ChinaDeny: "        if ($is_china = 0) { return 403; }\n",
		Allow:     []string{"192.168.1.0/24"},
	}, "        ")
	want := "        deny 8.8.8.8;\n        if ($is_china = 0) { return 403; }\n        allow 192.168.1.0/24;\n        deny all;\n"
	if got != want {
		t.Fatalf("AccessDirectives() = %q, 期望 %q", got, want)
	}
	if got := AccessDirectives(AccessControl{}, "        "); got != "" {
		t.Fatalf("空参数不应输出指令：%q", got)
	}
}

func TestLimitDirectives(t *testing.T) {
	got := LimitDirectives(LimitConfig{Zone: "havline_rule_7", Rate: 10, Burst: 20, Conn: 30}, "        ")
	for _, want := range []string{
		"limit_conn havline_rule_7_conn 30;",
		"limit_req zone=havline_rule_7_req burst=20 nodelay;",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("限流指令缺少 %q：%s", want, got)
		}
	}
	noBurst := LimitDirectives(LimitConfig{Zone: "z", Rate: 5}, "    ")
	if !strings.Contains(noBurst, "limit_req zone=z_req nodelay;") {
		t.Fatalf("无 burst 时输出不正确：%s", noBurst)
	}
}

func TestZoneDirectives(t *testing.T) {
	if got := RateLimitZone("havline_app_req", 10); got != "limit_req_zone $binary_remote_addr zone=havline_app_req:10m rate=10r/s;\n" {
		t.Fatalf("RateLimitZone() = %q", got)
	}
	if got := ConnLimitZone("havline_app_conn"); got != "limit_conn_zone $binary_remote_addr zone=havline_app_conn:10m;\n" {
		t.Fatalf("ConnLimitZone() = %q", got)
	}
	if got := RateLimitZone("", 10); got != "" {
		t.Fatalf("空 zone 不应输出：%q", got)
	}
}

func TestZoneName(t *testing.T) {
	if got := ZoneName("havline", "App.Example.com"); got != "havline_app_example_com" {
		t.Fatalf("ZoneName() = %q, 期望 havline_app_example_com", got)
	}
}

func TestUpgradeHeaders(t *testing.T) {
	got := UpgradeHeaders("        ", "$connection_upgrade")
	for _, want := range []string{
		"proxy_set_header Upgrade $http_upgrade;",
		"proxy_set_header Connection $connection_upgrade;",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("WebSocket 头缺少 %q：%s", want, got)
		}
	}
}
