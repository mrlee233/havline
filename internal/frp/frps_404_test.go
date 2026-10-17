package frp

import (
	"strings"
	"testing"
)

// 自定义 404 页面：填了「页面内容」就用 Havline 托管的那份（用户不用自己去 VPS 放文件），
// 只有手填路径时才用路径，两者都没填就不输出 custom404Page。
func TestBuildFRPSServerConfigCustom404Page(t *testing.T) {
	svc, ctx := newVhostPortTestService(t)

	base := func() ServerOptions {
		return ServerOptions{AuthMethod: "token", LogLevel: "info", Protocol: "tcp"}
	}
	manualPath := base()
	manualPath.Custom404Page = "/var/www/404.html"
	managed := base()
	managed.Custom404HTML = "<html><body>404</body></html>"
	both := base()
	both.Custom404Page = "/var/www/404.html"
	both.Custom404HTML = "<html><body>404</body></html>"

	cases := []struct {
		name    string
		key     string
		options ServerOptions
		want    string
		notWant string
	}{
		{name: "都没填", key: "none", options: base(), notWant: "custom404Page"},
		{name: "只填路径", key: "path", options: manualPath, want: `custom404Page = "/var/www/404.html"`},
		{
			name: "填了页面内容", key: "html", options: managed,
			want:    `custom404Page = "/etc/frp/havline-404.html"`,
			notWant: "/var/www/404.html",
		},
		{
			name: "两者都填时以内容为准", key: "both", options: both,
			want:    `custom404Page = "/etc/frp/havline-404.html"`,
			notWant: `custom404Page = "/var/www/404.html"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := baseVhostServerInput()
			// 每个子用例用独立名字，避免同一份测试库里重名
			in.Name = "vps-" + tc.key
			in.Options = tc.options
			server, err := svc.CreateServer(ctx, in)
			if err != nil {
				t.Fatalf("创建服务端失败：%v", err)
			}
			content, err := svc.buildFRPSServerConfig(ctx, server)
			if err != nil {
				t.Fatalf("生成 frps.toml 失败：%v", err)
			}
			if tc.want != "" && !strings.Contains(content, tc.want) {
				t.Fatalf("期望包含 %q：\n%s", tc.want, content)
			}
			if tc.notWant != "" && strings.Contains(content, tc.notWant) {
				t.Fatalf("不应包含 %q：\n%s", tc.notWant, content)
			}
		})
	}
}
