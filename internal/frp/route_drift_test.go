package frp

import "testing"

func TestProxyPassTarget(t *testing.T) {
	content := "server {\n    listen 443 ssl;\n    location / {\n        proxy_pass http://127.0.0.1:8080;\n    }\n}\n"
	if got := proxyPassTarget(content); got != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected target: %q", got)
	}
	if got := proxyPassTarget("server {\n    listen 80;\n}\n"); got != "" {
		t.Fatalf("expected empty target, got %q", got)
	}
}

func TestNormalizeUpstream(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:8080":   "127.0.0.1:8080",
		"https://127.0.0.1:8443/": "127.0.0.1:8443",
		"127.0.0.1:8080":          "127.0.0.1:8080",
	}
	for input, want := range cases {
		if got := normalizeUpstream(input); got != want {
			t.Fatalf("normalizeUpstream(%q) = %q，期望 %q", input, got, want)
		}
	}
}

func TestDeployableProxy(t *testing.T) {
	deployable := Proxy{Enabled: true, Type: "http", CustomDomains: []string{"a.com"}}
	if !deployableProxy(deployable) {
		t.Fatal("启用的 http 规则且有自定义域名，应可部署")
	}
	for _, proxy := range []Proxy{
		{Enabled: false, Type: "http", CustomDomains: []string{"a.com"}},
		{Enabled: true, Type: "tcp", CustomDomains: []string{"a.com"}},
		{Enabled: true, Type: "http"},
	} {
		if deployableProxy(proxy) {
			t.Fatalf("不应判定为可部署：%+v", proxy)
		}
	}
}
