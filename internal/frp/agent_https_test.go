package frp

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
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
	verify := pinTLSConfig(cert.Pin).VerifyPeerCertificate
	if err := verify([][]byte{block.Bytes}, nil); err != nil {
		t.Fatalf("正确 pin 校验失败: %v", err)
	}
	wrong := pinTLSConfig("0000000000000000000000000000000000000000000000000000000000000000").VerifyPeerCertificate
	if err := wrong([][]byte{block.Bytes}, nil); err == nil {
		t.Fatal("错误 pin 应校验失败")
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
