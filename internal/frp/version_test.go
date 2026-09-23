package frp

import (
	"path/filepath"
	"testing"
)

func TestVersionFromBinaryPath(t *testing.T) {
	frpDir := filepath.Join("data", "frp")
	cases := []struct {
		name string
		path string
		want string
	}{
		{"带 v 前缀的版本目录", filepath.Join(frpDir, "bin", "v0.71.0", "frpc.exe"), "v0.71.0"},
		{"纯数字版本目录", filepath.Join(frpDir, "bin", "0.62.1", "frpc"), "0.62.1"},
		{"不在 bin/<版本>/ 下", filepath.Join(frpDir, "frpc.exe"), ""},
		{"不在 FrpDir 下", filepath.Join("other", "bin", "v1.0.0", "frpc"), ""},
		{"多一层子目录", filepath.Join(frpDir, "bin", "v1.0.0", "extra", "frpc"), ""},
		{"目录名不是版本", filepath.Join(frpDir, "bin", "latest", "frpc"), ""},
	}
	for _, tc := range cases {
		if got := versionFromBinaryPath(frpDir, tc.path); got != tc.want {
			t.Fatalf("%s：versionFromBinaryPath(%q) = %q，期望 %q", tc.name, tc.path, got, tc.want)
		}
	}
}
