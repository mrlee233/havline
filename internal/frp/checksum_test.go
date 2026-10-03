package frp

import (
	"strings"
	"testing"
)

// 官方校验文件名不带版本号；这里钉住候选顺序，避免以后又被改回不存在的名字。
func TestChecksumAssetCandidates(t *testing.T) {
	got := checksumAssetCandidates("0.71.0")
	if len(got) < 2 {
		t.Fatalf("应至少有一个候选与一个兜底，实际 %v", got)
	}
	if got[0] != "frp_sha256_checksums.txt" {
		t.Fatalf("首选应为官方现行名字，实际 %q", got[0])
	}
	if got[1] != "frp_0.71.0_sha256sums.txt" {
		t.Fatalf("兜底名字应带版本号，实际 %q", got[1])
	}
}

// 解析真实格式：<hash>  <文件名>（也兼容 * 前缀与多余空白）
func TestFindChecksumLine(t *testing.T) {
	content := []byte(strings.Join([]string{
		"845b486c63686990e671f13cc5e3bd130ce7be659ab3c6f043008353451858cb  frp_0.71.0_android_arm64.tar.gz",
		"84f27e39f11169f7adcef8e8b70c9329de17747b1f14dad9fb95eef5682ea716  frp_0.71.0_linux_amd64.tar.gz",
		"96b8a4a09d7cd6a5ea0e1f9f0b1e4a35b0e0f0b4a4f7c0b0e0f0b4a4f7c0b0e0  *frp_0.71.0_linux_arm64.tar.gz",
		"",
	}, "\n"))

	sum, ok := findChecksumLine(content, "frp_0.71.0_linux_amd64.tar.gz")
	if !ok || sum != "84f27e39f11169f7adcef8e8b70c9329de17747b1f14dad9fb95eef5682ea716" {
		t.Fatalf("未解析出 linux_amd64 的哈希：%q ok=%v", sum, ok)
	}
	sum, ok = findChecksumLine(content, "frp_0.71.0_linux_arm64.tar.gz")
	if !ok || sum != "96b8a4a09d7cd6a5ea0e1f9f0b1e4a35b0e0f0b4a4f7c0b0e0f0b4a4f7c0b0e0" {
		t.Fatalf("带 * 前缀的文件名应能匹配：%q ok=%v", sum, ok)
	}
	if _, ok := findChecksumLine(content, "frp_0.71.0_windows_amd64.zip"); ok {
		t.Fatal("不存在的文件不应匹配到哈希")
	}
}
