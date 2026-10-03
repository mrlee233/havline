package cloudflared

import (
	"strings"
	"testing"
)

func TestBuildRunArgs(t *testing.T) {
	local := strings.Join(buildRunArgs(Tunnel{Mode: ModeTokenLocal, Network: DefaultNetworkSettings()}, "/data/config.yml", ""), " ")
	for _, want := range []string{"tunnel", "--config /data/config.yml", "--ha-connections 2", "run"} {
		if !strings.Contains(local, want) {
			t.Fatalf("本地规则启动参数缺少 %q：%s", want, local)
		}
	}
	remote := strings.Join(buildRunArgs(Tunnel{Mode: ModeTokenRemote, Network: DefaultNetworkSettings()}, "", "token-123"), " ")
	if strings.Contains(remote, "--config") || !strings.Contains(remote, "run --token token-123") {
		t.Fatalf("云端规则模式不应使用 --config：%s", remote)
	}
}

func TestParseMetrics(t *testing.T) {
	text := strings.Join([]string{
		"cloudflared_tunnel_ha_connections 2",
		"quic_client_receive_bytes 1000",
		"quic_client_sent_bytes 500",
		"quic_client_smoothed_rtt 0.02",
	}, "\n")
	snapshot := parseMetrics(text)
	if snapshot.connections != 2 || !snapshot.hasQUIC || snapshot.transport != "quic" {
		t.Fatalf("QUIC metrics 解析不符：%#v", snapshot)
	}
	if snapshot.latencyMS <= 0 {
		t.Fatalf("RTT 解析失败：%#v", snapshot)
	}
}

func TestParseMetricsHTTP2(t *testing.T) {
	snapshot := parseMetrics("cloudflared_tunnel_ha_connections 1")
	if snapshot.hasQUIC || snapshot.transport != "http2" || snapshot.connections != 1 {
		t.Fatalf("HTTP/2 metrics 解析不符：%#v", snapshot)
	}
}

func TestMetricsPortPatternCaseInsensitive(t *testing.T) {
	match := metricsPortPattern.FindStringSubmatch("2026-09-30T18:00:00Z INF Starting metrics server on 127.0.0.1:20241/metrics")
	if len(match) != 2 || match[1] != "20241" {
		t.Fatalf("应识别大写 Starting 的 metrics 端口：%#v", match)
	}
}

func TestBuildEnvDisablesProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:8080")
	env := buildEnv(NetworkSettings{ProxyMode: "disabled"})
	for _, item := range env {
		if strings.HasPrefix(item, "HTTP_PROXY=") {
			t.Fatalf("禁用代理时不应保留 HTTP_PROXY：%v", item)
		}
	}
}
