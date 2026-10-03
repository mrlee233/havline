package frp

import (
	"strings"
	"testing"
)

// frps.toml 的预览（FRPSServerConfig 返回的内容）绝不能带明文密钥：
// 拉到的内容会直接渲染在浏览器的「即将下发的内容」里。
func TestMaskFRPSSecrets(t *testing.T) {
	content := strings.Join([]string{
		`bindPort = 7000`,
		`auth.method = "token"`,
		`auth.token = "s3cr3t-token"`,
		`auth.oidc.clientSecret = "s3cr3t-oidc"`,
		`# auth.token = "注释里的示例不应受影响"`,
		`webServer.addr = "0.0.0.0"`,
	}, "\n")

	masked := maskFRPSSecrets(content)
	if strings.Contains(masked, "s3cr3t-token") || strings.Contains(masked, "s3cr3t-oidc") {
		t.Fatalf("预览里仍能看到明文密钥：\n%s", masked)
	}
	for _, expected := range []string{
		`auth.token = "********"`,
		`auth.oidc.clientSecret = "********"`,
		`bindPort = 7000`,
		`auth.method = "token"`,
		`webServer.addr = "0.0.0.0"`,
	} {
		if !strings.Contains(masked, expected) {
			t.Fatalf("脱敏后缺少 %q：\n%s", expected, masked)
		}
	}
}

// 注释行不是生效配置，不参与脱敏（保持模板原样）
func TestMaskFRPSSecretsKeepsComments(t *testing.T) {
	content := `# auth.token = "说明文字"`
	if got := maskFRPSSecrets(content); got != content {
		t.Fatalf("注释行被改写：%q", got)
	}
}
