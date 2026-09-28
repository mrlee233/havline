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
