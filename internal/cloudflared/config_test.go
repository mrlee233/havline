package cloudflared

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/havline/havline/internal/proxy"
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

func TestDecodeTokenURLSafe(t *testing.T) {
	payload := `{"a":"account-1","t":"tunnel-2","s":"secret-3"}`
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	if _, _, _, err := DecodeToken(encoded); err != nil {
		t.Fatalf("URL-safe token 应可解码: %v", err)
	}
}

func TestDecodeBase64Flexible(t *testing.T) {
	raw := []byte{0xfb, 0xef, 0xbe, 0xad}
	for _, encoded := range []string{
		base64.RawURLEncoding.EncodeToString(raw),
		base64.URLEncoding.EncodeToString(raw),
		base64.RawStdEncoding.EncodeToString(raw),
	} {
		got, err := decodeBase64(encoded)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatalf("编码 %q 解码失败：%v", encoded, err)
		}
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

func TestBuildIngressForRulesUsesRuleUpstream(t *testing.T) {
	rules := []proxy.Rule{
		{
			Enabled:    true,
			Upstream:   "http://192.168.1.10:8080",
			Hosts:      []proxy.Host{{Hostname: "b.example.com"}},
			Exits:      []string{proxy.ExitCloudflare},
			CFTunnelID: 9,
		},
		{
			Enabled:    true,
			Upstream:   "http://192.168.1.11:9090",
			Hosts:      []proxy.Host{{Hostname: "a.example.com"}},
			Exits:      []string{proxy.ExitLocal, proxy.ExitCloudflare},
			CFTunnelID: 9,
		},
	}
	got := BuildIngressForRules(rules)
	if len(got) != 3 {
		t.Fatalf("应生成 2 条域名 + catch-all：%#v", got)
	}
	if got[0].Hostname != "a.example.com" || got[0].Service != "http://192.168.1.11:9090" {
		t.Fatalf("第一条 ingress 不符：%#v", got[0])
	}
	if got[1].Hostname != "b.example.com" || got[1].Service != "http://192.168.1.10:8080" {
		t.Fatalf("第二条 ingress 不符：%#v", got[1])
	}
	if got[2].Hostname != "" || got[2].Service != "http_status:404" {
		t.Fatalf("末条必须是 catch-all：%#v", got[2])
	}
}

func TestBuildIngressForRulesSkipsDisabledAndManualRules(t *testing.T) {
	rules := []proxy.Rule{
		{Enabled: false, Upstream: "http://127.0.0.1:8080", Hosts: []proxy.Host{{Hostname: "off.example.com"}}, Exits: []string{proxy.ExitCloudflare}, CFTunnelID: 1},
		{Enabled: true, Upstream: "http://127.0.0.1:8081", Hosts: []proxy.Host{{Hostname: "local.example.com"}}, Exits: []string{proxy.ExitLocal}},
	}
	got := BuildIngressForRules(rules)
	if len(got) != 1 || got[0].Service != "http_status:404" {
		t.Fatalf("停用规则和仅本机规则不应进入 Cloudflare ingress：%#v", got)
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
