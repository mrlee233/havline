package frp

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/havline/havline/internal/certificate"
	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/db"
	"github.com/havline/havline/internal/secret"
)

// newVhostPortTestService 建一个带真实 SQLite 的 Service：80/443 的放行是「校验 + 生成 frps.toml」
// 两段链路共同决定的，纯函数测试覆盖不到。
func newVhostPortTestService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "havline.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	box, err := secret.NewBox("vhost-port-test-key")
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(config.Config{DataDir: dir}, NewStore(conn), box, logger, certificate.NewStore(conn))
	if err := svc.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	return svc, context.Background()
}

func baseVhostServerInput() ServerInput {
	return ServerInput{
		Name:       "test-vps",
		ServerAddr: "10.0.0.1",
		ServerPort: 7000,
		Enabled:    true,
		Token:      "test-token",
		Options:    ServerOptions{AuthMethod: "token", LogLevel: "info", Protocol: "tcp"},
	}
}

// 未二次确认时，80/443 必须被拒绝。
func TestCreateServerPrivilegedVhostPortsRequiresConfirmation(t *testing.T) {
	svc, ctx := newVhostPortTestService(t)

	for _, testCase := range []struct {
		name  string
		http  int
		https int
	}{
		{name: "HTTP 80", http: 80, https: 8443},
		{name: "HTTPS 443", http: 8080, https: 443},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			in := baseVhostServerInput()
			in.Options.VhostHTTPPort = testCase.http
			in.Options.VhostHTTPSPort = testCase.https
			if _, err := svc.CreateServer(ctx, in); err == nil {
				t.Fatal("未二次确认时保存 80/443 应被拒绝")
			} else if !strings.Contains(err.Error(), "80/443") {
				t.Fatalf("错误信息应点明 80/443：%v", err)
			}
		})
	}
}

// 二次确认后允许保存，生成的 frps.toml 要带风险注释且不能报错。
func TestCreateServerPrivilegedVhostPortsConfirmed(t *testing.T) {
	svc, ctx := newVhostPortTestService(t)

	in := baseVhostServerInput()
	in.Options.VhostHTTPPort = 80
	in.Options.VhostHTTPSPort = 443
	in.AllowPrivilegedVhostPorts = true
	server, err := svc.CreateServer(ctx, in)
	if err != nil {
		t.Fatalf("二次确认后保存应成功：%v", err)
	}

	// vhost 端口只在存在 http/https/tcpmux 规则时才写入 frps.toml
	if _, err := svc.CreateProxy(ctx, ProxyInput{
		ServerID: server.ID, Name: "nas-web", Type: "http",
		LocalIP: "127.0.0.1", LocalPort: 18080,
		CustomDomains: []string{"nas.example.com"}, Enabled: true,
	}); err != nil {
		t.Fatalf("创建 http 规则失败：%v", err)
	}

	content, err := svc.buildFRPSServerConfig(ctx, server)
	if err != nil {
		t.Fatalf("已放行的 80/443 不应导致 frps.toml 生成失败：%v", err)
	}
	for _, want := range []string{"vhostHTTPPort = 80", "vhostHTTPSPort = 443", "80/443"} {
		if !strings.Contains(content, want) {
			t.Fatalf("frps.toml 缺少 %q：\n%s", want, content)
		}
	}
}

// 反代回源地址必须跟着 80/443 走，否则 VPS 上 Nginx 会指向错端口。
func TestAgentUpstreamFollowsPrivilegedVhostPorts(t *testing.T) {
	svc, ctx := newVhostPortTestService(t)

	in := baseVhostServerInput()
	in.Options.VhostHTTPPort = 80
	in.Options.VhostHTTPSPort = 443
	in.AllowPrivilegedVhostPorts = true
	server, err := svc.CreateServer(ctx, in)
	if err != nil {
		t.Fatalf("保存失败：%v", err)
	}

	httpProxy, err := svc.CreateProxy(ctx, ProxyInput{
		ServerID: server.ID, Name: "nas-web", Type: "http",
		LocalIP: "127.0.0.1", LocalPort: 18080,
		CustomDomains: []string{"nas.example.com"}, Enabled: true,
	})
	if err != nil {
		t.Fatalf("创建 http 规则失败：%v", err)
	}
	if got, err := svc.agentUpstreamFor(ctx, httpProxy); err != nil {
		t.Fatalf("解析回源地址失败：%v", err)
	} else if got != "127.0.0.1:80" {
		t.Fatalf("HTTP 回源地址 = %q，期望 127.0.0.1:80", got)
	}

	httpsProxy, err := svc.CreateProxy(ctx, ProxyInput{
		ServerID: server.ID, Name: "nas-web-tls", Type: "https",
		LocalIP: "127.0.0.1", LocalPort: 18080,
		CustomDomains: []string{"nas-tls.example.com"}, Enabled: true,
	})
	if err != nil {
		t.Fatalf("创建 https 规则失败：%v", err)
	}
	if got, err := svc.agentUpstreamFor(ctx, httpsProxy); err != nil {
		t.Fatalf("解析回源地址失败：%v", err)
	} else if got != "127.0.0.1:443" {
		t.Fatalf("HTTPS 回源地址 = %q，期望 127.0.0.1:443", got)
	}
}

func TestPrivilegedVhostPortsDetection(t *testing.T) {
	cases := []struct {
		name    string
		options ServerOptions
		want    bool
	}{
		{name: "默认 8080/8443", options: ServerOptions{}, want: false},
		{name: "留空但显式 0", options: ServerOptions{VhostHTTPPort: 0, VhostHTTPSPort: 0}, want: false},
		{name: "HTTP 80", options: ServerOptions{VhostHTTPPort: 80}, want: true},
		{name: "HTTPS 443", options: ServerOptions{VhostHTTPSPort: 443}, want: true},
		{name: "自定义高位端口", options: ServerOptions{VhostHTTPPort: 18080, VhostHTTPSPort: 18443}, want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := privilegedVhostPorts(testCase.options); got != testCase.want {
				t.Fatalf("privilegedVhostPorts() = %v, 期望 %v", got, testCase.want)
			}
		})
	}
}

// 80/443 回源端口下公网反代不适用：单条部署与全量重部署（含漂移一键修复）都必须被拒绝。
func TestPublicReverseProxyRejectedForPrivilegedVhostPorts(t *testing.T) {
	svc, ctx := newVhostPortTestService(t)

	in := baseVhostServerInput()
	in.Options.VhostHTTPPort = 80
	in.Options.VhostHTTPSPort = 443
	in.AllowPrivilegedVhostPorts = true
	server, err := svc.CreateServer(ctx, in)
	if err != nil {
		t.Fatalf("保存失败：%v", err)
	}
	proxy, err := svc.CreateProxy(ctx, ProxyInput{
		ServerID: server.ID, Name: "nas-web", Type: "http",
		LocalIP: "127.0.0.1", LocalPort: 18080,
		CustomDomains: []string{"nas.example.com"}, Enabled: true,
	})
	if err != nil {
		t.Fatalf("创建 http 规则失败：%v", err)
	}

	if err := svc.DeployAgentRoute(ctx, proxy.ID); err == nil {
		t.Fatal("80/443 回源端口下部署公网反代应被拒绝")
	} else if !strings.Contains(err.Error(), "80/443") {
		t.Fatalf("错误信息应说明 80/443 不适用公网反代：%v", err)
	}

	if _, err := svc.RedeployAllRoutes(ctx, server.ID); err == nil {
		t.Fatal("80/443 回源端口下全量重部署应被拒绝")
	} else if !strings.Contains(err.Error(), "80/443") {
		t.Fatalf("错误信息应说明 80/443 不适用公网反代：%v", err)
	}
}

// 对照组：默认 8080/8443 时不应被 80/443 规则拦下（此时失败是「没配 agent」，属于另一条链路）。
func TestPublicReverseProxyNotBlockedForDefaultVhostPorts(t *testing.T) {
	svc, ctx := newVhostPortTestService(t)

	server, err := svc.CreateServer(ctx, baseVhostServerInput())
	if err != nil {
		t.Fatalf("保存失败：%v", err)
	}
	proxy, err := svc.CreateProxy(ctx, ProxyInput{
		ServerID: server.ID, Name: "nas-web", Type: "http",
		LocalIP: "127.0.0.1", LocalPort: 18080,
		CustomDomains: []string{"nas.example.com"}, Enabled: true,
	})
	if err != nil {
		t.Fatalf("创建 http 规则失败：%v", err)
	}

	err = svc.DeployAgentRoute(ctx, proxy.ID)
	if err == nil {
		t.Fatal("未配置 agent 时部署应失败")
	}
	if strings.Contains(err.Error(), "不适用也无需部署") {
		t.Fatalf("默认端口不应被 80/443 规则拦下，实际：%v", err)
	}
}
