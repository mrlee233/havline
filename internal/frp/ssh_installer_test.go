package frp

import "testing"

// TestShellPathEscapesSingleQuotes：uploadBytes 把路径拼进远程 shell 命令，
// 单引号必须转义，否则路径里的引号能截断命令。
func TestShellPathEscapesSingleQuotes(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/tmp/havline-agent", "'/tmp/havline-agent'"},
		{"/tmp/it's here", `'/tmp/it'\''s here'`},
		{"", "''"},
	}
	for _, c := range cases {
		if got := shellPath(c.in); got != c.want {
			t.Errorf("shellPath(%q) = %s，期望 %s", c.in, got, c.want)
		}
	}
}

// TestIsELFBinary：内嵌二进制在开发构建里是占位文件，必须识别出来并提前报错，
// 否则 VPS 上 systemd 会反复 Exec format error。
func TestIsELFBinary(t *testing.T) {
	if isELFBinary([]byte("not an elf")) {
		t.Error("普通文本不应被判为 ELF")
	}
	if isELFBinary(nil) {
		t.Error("空内容不应被判为 ELF")
	}
	if !isELFBinary([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0}) {
		t.Error("ELF 魔数应被识别")
	}
}
