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
// 不构造 Basic Auth 场景——那条路径会往 /var/lib/fonu-agent 写 htpasswd，不适合在单测里触发。
func TestNginxRouteRendersUpstream(t *testing.T) {
	out := nginxRoute(routeRequest{Domain: "a.example.com", Upstream: "127.0.0.1:8080"}, "")
	if !strings.HasPrefix(out, "# Managed by fonu-agent.") {
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
