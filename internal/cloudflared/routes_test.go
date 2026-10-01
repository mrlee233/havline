package cloudflared

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManualRoutesFromConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	content := `tunnel: test
ingress:
  - hostname: a.example.com
    service: http://127.0.0.1:8080
  - hostname: b.example.com
    service: http://127.0.0.1:9090
  - service: http_status:404
`
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := manualRoutes(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Routes) != 2 || result.Routes[0].Hostname != "a.example.com" || result.Routes[1].Service != "http://127.0.0.1:9090" {
		t.Fatalf("手工路由解析不符：%#v", result)
	}
	if result.CatchAll != "http_status:404" {
		t.Fatalf("catch-all 不符：%q", result.CatchAll)
	}
}

func TestBuildManualIngress(t *testing.T) {
	ingress, hostnames, err := buildManualIngress([]RouteInput{
		{Hostname: "A.Example.com", Path: "*", Service: "http://127.0.0.1:8080"},
		{Hostname: "b.example.com", Service: "https://127.0.0.1:8443"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ingress) != 3 || ingress[0].Hostname != "a.example.com" || ingress[1].Hostname != "b.example.com" {
		t.Fatalf("ingress 结果不符：%#v", ingress)
	}
	if ingress[0].Path != "" || ingress[1].Path != "" {
		t.Fatalf("通配路径不应写入 Cloudflare ingress：%#v", ingress)
	}
	if len(hostnames) != 2 || hostnames[1] != "b.example.com" {
		t.Fatalf("hostnames 结果不符：%#v", hostnames)
	}
}
