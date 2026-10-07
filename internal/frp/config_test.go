package frp

import (
	"encoding/json"
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

// 旧数据（config_json = "{}"）缺省布尔键时必须补回表单默认值，
// 否则会生成 transport.tcpMux = false，让每条新连接都重建 TCP+TLS。
func TestServerOptionsBooleanDefaultsWhenKeysMissing(t *testing.T) {
	var legacy ServerOptions
	if err := json.Unmarshal([]byte("{}"), &legacy); err != nil {
		t.Fatalf("解析旧版空配置失败：%v", err)
	}
	legacy = normalizeServerOptions(legacy)
	if !legacy.TCPMux || !legacy.LoginFailExit {
		t.Fatalf("缺省键应补默认值 true，实际 tcpMux=%v loginFailExit=%v", legacy.TCPMux, legacy.LoginFailExit)
	}
	content := renderTOML(renderedConfig{Server: Server{ServerAddr: "vps.example.com", ServerPort: 7000, Options: legacy}})
	if !strings.Contains(content, "transport.tcpMux = true") {
		t.Fatalf("旧版配置应渲染 transport.tcpMux = true:\n%s", content)
	}

	var explicit ServerOptions
	if err := json.Unmarshal([]byte(`{"tcp_mux":false,"login_fail_exit":false}`), &explicit); err != nil {
		t.Fatalf("解析显式配置失败：%v", err)
	}
	if explicit.TCPMux || explicit.LoginFailExit {
		t.Fatalf("显式 false 不应被默认值覆盖，实际 tcpMux=%v loginFailExit=%v", explicit.TCPMux, explicit.LoginFailExit)
	}
}

func intPtr(value int) *int { return &value }
