package acme

import (
	"strings"
	"testing"
)

// LiteSSL 接替 Buypass 的位置：目录地址实测可用，且强制 EAB。
func TestLiteSSLDirectoryAndLabel(t *testing.T) {
	url, err := DirectoryURL(CALiteSSL)
	if err != nil {
		t.Fatalf("LiteSSL 目录地址解析失败：%v", err)
	}
	if url != "https://acme.litessl.com/acme/v2/directory" {
		t.Fatalf("LiteSSL 目录地址不对：%q", url)
	}
	if label := CALabel(CALiteSSL); label != "LiteSSL" {
		t.Fatalf("LiteSSL 展示名不对：%q", label)
	}
	// 别名与归一化
	if got := NormalizeCA("trustasia"); got != CALiteSSL {
		t.Fatalf("别名 trustasia 应归一化为 %s，实际 %q", CALiteSSL, got)
	}
	if reason := CAUnavailableReason(CALiteSSL); reason != "" {
		t.Fatalf("LiteSSL 是可用的 CA，不该有停用原因：%q", reason)
	}
}

// LiteSSL 支持通配符与多域名，因此 ValidateCADomains 不应拦它。
func TestValidateCADomainsLiteSSLAllowsWildcard(t *testing.T) {
	domains := []string{"*.36800.cc", "a.com", "b.com", "c.com", "d.com", "e.com", "f.com"}
	if err := ValidateCADomains(CALiteSSL, domains); err != nil {
		t.Fatalf("LiteSSL 应允许通配符与多域名：%v", err)
	}
}

// Buypass 保留在停用表里（历史记录要能展示），但提示里应给出可用的替代。
func TestBuypassIsRetiredWithAlternatives(t *testing.T) {
	reason := CAUnavailableReason(CABuypass)
	if reason == "" {
		t.Fatal("Buypass 应仍被标记为已停用")
	}
	if !strings.Contains(reason, "LiteSSL") {
		t.Fatalf("停用提示里应包含 LiteSSL 这个替代项：%q", reason)
	}
	// 旧记录仍能正常取到目录地址（不报「不支持的颁发机构」）
	if _, err := DirectoryURL(CABuypass); err != nil {
		t.Fatalf("历史 Buypass 记录的目录地址仍应可解析：%v", err)
	}
}
