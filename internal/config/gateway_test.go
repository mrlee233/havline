package config

import "testing"

func TestNativeGatewayConfig(t *testing.T) {
	t.Setenv("HAVLINE_GATEWAY_SOCKET", "/tmp/havline-test.sock")
	t.Setenv("HAVLINE_GATEWAY_PREFIX", "/app/havline")
	t.Setenv("HAVLINE_UPDATER_SOCKET", "")
	cfg := Load()
	if cfg.GatewaySocket != "/tmp/havline-test.sock" || cfg.GatewayPrefix != "/app/havline" {
		t.Fatal("原生网关配置未生效")
	}
	if cfg.UpdaterSocket != "" {
		t.Fatal("原生应用显式空值应禁用 Docker 升级")
	}
}
