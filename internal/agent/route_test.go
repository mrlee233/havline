package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRoute(t *testing.T) {
	cases := []struct {
		name string
		req  routeRequest
		ok   bool
	}{
		{"正常", routeRequest{Domain: "a.example.com", Upstream: "127.0.0.1:8080"}, true},
		{"非法域名", routeRequest{Domain: "bad domain", Upstream: "127.0.0.1:8080"}, false},
		{"空上游", routeRequest{Domain: "a.example.com"}, false},
		{"上游带分号", routeRequest{Domain: "a.example.com", Upstream: "127.0.0.1:80; rm -rf /"}, false},
		{"相对证书目录", routeRequest{Domain: "a.example.com", Upstream: "127.0.0.1:80", TLS: true, CertDir: "certs"}, false},
	}
	for _, c := range cases {
		err := validateRoute(c.req)
		if c.ok && err != nil {
			t.Errorf("%s：期望通过，实际 %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s：期望报错", c.name)
		}
	}
}

// TestNginxRouteRendersUpstream：vhost 模板的关键行（标记、server_name、proxy_pass）。
// 不构造 Basic Auth 场景——那条路径会往 /var/lib/havline-agent 写 htpasswd，不适合在单测里触发。
func TestNginxRouteRendersUpstream(t *testing.T) {
	out := nginxRoute(routeRequest{Domain: "a.example.com", Upstream: "127.0.0.1:8080"}, "")
	if !strings.HasPrefix(out, "# Managed by havline-agent.") {
		t.Errorf("缺少托管标记，其它工具无法识别该文件归属：\n%s", out)
	}
	if !strings.Contains(out, "server_name a.example.com;") {
		t.Errorf("缺少 server_name：\n%s", out)
	}
	if !strings.Contains(out, "proxy_pass http://127.0.0.1:8080;") {
		t.Errorf("缺少 proxy_pass 回源：\n%s", out)
	}
	if strings.Contains(out, "listen 443") {
		t.Errorf("未启用 TLS 时不应监听 443：\n%s", out)
	}
}

// TestNginxRouteSharedTemplateSemantics 锁定共享模板迁出后的关键语义与顺序。
func TestNginxRouteSharedTemplateSemantics(t *testing.T) {
	out := nginxRoute(routeRequest{
		Domain:          "a.example.com",
		Upstream:        "127.0.0.1:8080",
		TLS:             true,
		RedirectTLS:     true,
		SecurityHeaders: true,
		WebSocket:       true,
		AllowIPs:        []string{"192.168.1.0/24"},
		DenyIPs:         []string{"8.8.8.8"},
		RateLimitRate:   10,
		RateLimitBurst:  20,
		ConnLimitMax:    5,
		ChinaOnly:       true,
	}, "/etc/havline-agent/certs/a.example.com")
	for _, want := range []string{
		"ssl_protocols TLSv1.2 TLSv1.3;",
		`add_header Referrer-Policy "strict-origin-when-cross-origin" always;`,
		"deny 8.8.8.8;",
		"if ($havline_china_client = 0) { return 403; }",
		"allow 192.168.1.0/24;",
		"deny all;",
		"limit_conn havline_a_example_com_conn 5;",
		"limit_req zone=havline_a_example_com_req burst=20 nodelay;",
		"proxy_set_header Upgrade $http_upgrade;",
		`proxy_set_header Connection "upgrade";`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("缺少 %q：\n%s", want, out)
		}
	}
	denyIdx := strings.Index(out, "deny 8.8.8.8;")
	allowIdx := strings.Index(out, "allow 192.168.1.0/24;")
	denyAllIdx := strings.Index(out, "deny all;")
	if denyIdx < 0 || allowIdx < 0 || denyAllIdx < 0 || !(denyIdx < allowIdx && allowIdx < denyAllIdx) {
		t.Fatalf("allow / deny 顺序不正确：\n%s", out)
	}
}

// TestFileSHA256 覆盖 frps 安装包的校验路径。
func TestFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Fatalf("sha256 不符：got %s", got)
	}
}
