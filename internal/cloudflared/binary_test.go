package cloudflared

import (
	"strings"
	"testing"
)

func TestDownloadURL(t *testing.T) {
	official := DownloadURL(Mirror{ID: "official"}, "")
	if !strings.Contains(official, "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-") {
		t.Fatalf("官方下载地址不正确：%s", official)
	}
	proxied := DownloadURL(Mirror{ID: "gh-proxy-org", Base: "https://gh-proxy.org/"}, "")
	if !strings.HasPrefix(proxied, "https://gh-proxy.org/https://github.com/cloudflare/cloudflared/releases/latest/download/") {
		t.Fatalf("镜像前缀拼接不正确：%s", proxied)
	}
}

func TestFindMirrorFallback(t *testing.T) {
	if got := findMirror("not-exist"); got.ID != "official" {
		t.Fatalf("未知镜像应回退官方源：%#v", got)
	}
}
