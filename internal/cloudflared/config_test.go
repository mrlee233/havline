package cloudflared

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecodeToken(t *testing.T) {
	payload := `{"a":"account-1","t":"tunnel-2","s":"secret-3"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	account, tunnel, secret, err := DecodeToken(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if account != "account-1" || tunnel != "tunnel-2" || secret != "secret-3" {
		t.Fatalf("解码结果不符：%s %s %s", account, tunnel, secret)
	}
}

func TestDecodeTokenAlternativeFields(t *testing.T) {
	payload := `{"AccountTag":"a","TunnelID":"t","TunnelSecret":"s"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	if _, _, _, err := DecodeToken(encoded); err != nil {
		t.Fatalf("兼容字段名解析失败: %v", err)
	}
}

func TestDecodeTokenRejectsInvalid(t *testing.T) {
	if _, _, _, err := DecodeToken("not-base64"); err == nil {
		t.Fatal("非法 token 应报错")
	}
}

func TestBuildIngressInvariants(t *testing.T) {
	got := BuildIngress([]string{"B.example.com", "a.example.com", "a.example.com", " "}, "http://127.0.0.1:80")
	if len(got) != 3 {
		t.Fatalf("应生成 2 条域名 + 1 条 catch-all：%#v", got)
	}
	if got[0].Hostname != "a.example.com" || got[1].Hostname != "b.example.com" {
		t.Fatalf("域名应去重并稳定排序：%#v", got)
	}
	if got[2].Hostname != "" || got[2].Service != "http_status:404" {
		t.Fatalf("末条必须是 catch-all：%#v", got[2])
	}
}

func TestBuildConfig(t *testing.T) {
	config := BuildConfig("tunnel-1", "/data/creds.json", DefaultNetworkSettings(), BuildIngress([]string{"a.example.com"}, "http://127.0.0.1:80"))
	for _, want := range []string{
		"tunnel: tunnel-1",
		"credentials-file: /data/creds.json",
		"protocol: http2",
		`edge-ip-version: "4"`,
		"ha-connections: 2",
		"- hostname: a.example.com",
		"service: http://127.0.0.1:80",
		"- service: http_status:404",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("config.yml 缺少 %q：\n%s", want, config)
		}
	}
}

func TestNormalizeService(t *testing.T) {
	if got := NormalizeService("http", "localhost", 80); got != "http://127.0.0.1:80" {
		t.Fatalf("NormalizeService() = %q", got)
	}
}
