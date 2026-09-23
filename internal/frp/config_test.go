package frp

import (
	"strings"
	"testing"
)

func TestRenderTOML(t *testing.T) {
	remotePort := 60022
	content := renderTOML(renderedConfig{
		Server: Server{ServerAddr: "vps.example.com", ServerPort: 7000, TLS: true},
		Token:  "token with \"quotes\"",
		Proxies: []Proxy{
			{Name: "nas-ssh", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &remotePort},
			{Name: "nas-web", Type: "http", LocalIP: "127.0.0.1", LocalPort: 18080, CustomDomains: []string{"nas.example.com"}},
		},
	})

	for _, expected := range []string{
		`serverAddr = "vps.example.com"`,
		`auth.token = "token with \"quotes\""`,
		`remotePort = 60022`,
		`customDomains = ["nas.example.com"]`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("配置缺少 %q:\n%s", expected, content)
		}
	}
}

func TestRenderMaskedTOMLDoesNotExposeToken(t *testing.T) {
	content := renderMaskedTOML(Server{ServerAddr: "vps.example.com", ServerPort: 7000, TLS: true}, nil)
	if !strings.Contains(content, `auth.token = "********"`) {
		t.Fatalf("脱敏配置未隐藏 Token:\n%s", content)
	}
}

func TestRenderProxyOptions(t *testing.T) {
	content := renderTOML(renderedConfig{Server: Server{ServerAddr: "vps.example.com", ServerPort: 7000}, Proxies: []Proxy{
		{Name: "udp", Type: "udp", LocalIP: "127.0.0.1", LocalPort: 53, RemotePort: intPtr(6002), Options: ProxyOptions{Transport: ProxyTransportOptions{UseEncryption: true, ProxyProtocolVersion: "v2"}, HealthCheck: ProxyHealthCheckOptions{Type: "tcp"}}},
		{Name: "secret", Type: "stcp", LocalIP: "127.0.0.1", LocalPort: 22, Options: ProxyOptions{SecretKey: "secret", AllowUsers: []string{"*"}}},
	}})
	for _, expected := range []string{`type = "udp"`, `remotePort = 6002`, `transport.useEncryption = true`, `transport.proxyProtocolVersion = "v2"`, `healthCheck.type = "tcp"`, `secretKey = "secret"`, `allowUsers = ["*"]`} {
		if !strings.Contains(content, expected) {
			t.Fatalf("配置缺少 %q:\n%s", expected, content)
		}
	}
}

func TestRenderNATTraversalOnlyWhenDisabledAssistedAddrs(t *testing.T) {
	omitted := renderTOML(renderedConfig{Proxies: []Proxy{
		{Name: "p2p", Type: "xtcp", Options: ProxyOptions{NATTraversal: &ProxyNATTraversal{}}},
	}})
	if strings.Contains(omitted, "natTraversal") {
		t.Fatalf("disableAssistedAddrs=false 不应渲染 natTraversal（旧版 frpc 校验会失败）:\n%s", omitted)
	}
	enabled := renderTOML(renderedConfig{Proxies: []Proxy{
		{Name: "p2p", Type: "xtcp", Options: ProxyOptions{NATTraversal: &ProxyNATTraversal{DisableAssistedAddrs: true}}},
	}})
	if !strings.Contains(enabled, "natTraversal.disableAssistedAddrs = true") {
		t.Fatalf("disableAssistedAddrs=true 应渲染 natTraversal:\n%s", enabled)
	}
}

func intPtr(value int) *int { return &value }
