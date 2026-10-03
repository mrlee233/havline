package frp

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"strings"
	"testing"
	"time"
)

func TestGenerateAgentCertificate(t *testing.T) {
	cert, err := generateAgentCertificate("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePin(cert.Pin); err != nil {
		t.Fatalf("pin 无效: %v", err)
	}
	block, _ := pem.Decode([]byte(cert.CertPEM))
	if block == nil {
		t.Fatal("证书 PEM 解码失败")
	}
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.IPAddresses) != 1 || !parsed.IPAddresses[0].Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("证书 IP SAN 不正确: %#v", parsed.IPAddresses)
	}
}

func TestPinTLSConfigVerification(t *testing.T) {
	cert, err := generateAgentCertificate("agent.example.com")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(cert.CertPEM))
	if block == nil {
		t.Fatal("证书 PEM 解码失败")
	}
	verify := pinTLSConfig("agent.example.com", cert.Pin).VerifyPeerCertificate
	if err := verify([][]byte{block.Bytes}, nil); err != nil {
		t.Fatalf("正确 pin 校验失败: %v", err)
	}
	wrong := pinTLSConfig("agent.example.com", strings.Repeat("0", 64)).VerifyPeerCertificate
	if err := wrong([][]byte{block.Bytes}, nil); err == nil {
		t.Fatal("错误 pin 应校验失败")
	}
}

func TestPinTLSConfigAcceptsGracePins(t *testing.T) {
	cert, err := generateAgentCertificate("agent.example.com")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(cert.CertPEM))
	if block == nil {
		t.Fatal("证书 PEM 解码失败")
	}
	verify := pinTLSConfig("agent.example.com", strings.Repeat("1", 64), cert.Pin).VerifyPeerCertificate
	if err := verify([][]byte{block.Bytes}, nil); err != nil {
		t.Fatalf("轮换过渡期应同时接受旧 pin 与新 pin: %v", err)
	}
}

func TestPinTLSConfigRejectsHostMismatch(t *testing.T) {
	cert, err := generateAgentCertificate("agent.example.com")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(cert.CertPEM))
	if block == nil {
		t.Fatal("证书 PEM 解码失败")
	}
	verify := pinTLSConfig("other.example.com", cert.Pin).VerifyPeerCertificate
	if err := verify([][]byte{block.Bytes}, nil); err == nil {
		t.Fatal("证书 SAN 与连接主机不一致时应拒绝")
	}
}

func TestAgentHTTPSBaseURL(t *testing.T) {
	cases := []struct {
		name   string
		server Server
		want   string
	}{
		{
			name:   "旧隧道地址按监听端口重建",
			server: Server{AgentURL: "http://127.0.0.1:17701", SSHHost: "agent.example.com", AgentListenAddr: "0.0.0.0:7443"},
			want:   "https://agent.example.com:7443",
		},
		{
			name:   "已是 HTTPS 地址保持不变",
			server: Server{AgentURL: "https://agent.example.com:8443", SSHHost: "agent.example.com", AgentListenAddr: "0.0.0.0:7443"},
			want:   "https://agent.example.com:8443",
		},
		{
			name:   "监听地址缺少端口时使用默认端口",
			server: Server{AgentURL: "http://127.0.0.1:17701", SSHHost: "agent.example.com"},
			want:   "https://agent.example.com:7443",
		},
		{
			name:   "缺少 SSH 主机时保留原地址",
			server: Server{AgentURL: "http://127.0.0.1:17701"},
			want:   "http://127.0.0.1:17701",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := agentHTTPSBaseURL(tc.server); got != tc.want {
				t.Fatalf("agentHTTPSBaseURL() = %q, 期望 %q", got, tc.want)
			}
		})
	}
}

func TestAgentTransportPins(t *testing.T) {
	active := Server{
		AgentTLSPin:          "new",
		AgentTLSPinPrev:      "old",
		AgentTLSPinPrevUntil: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	pins := agentTransportPins(active)
	if len(pins) != 2 || pins[0] != "new" || pins[1] != "old" {
		t.Fatalf("过渡期应同时返回新旧 pin：%v", pins)
	}
	expired := active
	expired.AgentTLSPinPrevUntil = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if pins := agentTransportPins(expired); len(pins) != 1 || pins[0] != "new" {
		t.Fatalf("过渡期结束后只应保留新 pin：%v", pins)
	}
}

func TestResultPortAndListenPort(t *testing.T) {
	if got := resultPort(map[string]any{"port": float64(8443)}, 0); got != 8443 {
		t.Fatalf("resultPort() = %d, 期望 8443", got)
	}
	if got := resultPort(nil, 7443); got != 7443 {
		t.Fatalf("resultPort() 回退值 = %d, 期望 7443", got)
	}
	if got := listenPort("0.0.0.0:9443", 7443); got != 9443 {
		t.Fatalf("listenPort() = %d, 期望 9443", got)
	}
	if got := listenPort("", 7443); got != 7443 {
		t.Fatalf("listenPort() 回退值 = %d, 期望 7443", got)
	}
}
