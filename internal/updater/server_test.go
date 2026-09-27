package updater

import (
	"path/filepath"
	"testing"
)

func TestResolveComposePath(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveComposePath(dir, "docker-compose.hub.yml")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "docker-compose.hub.yml")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := resolveComposePath(dir, "../docker-compose.yml"); err == nil {
		t.Fatal("路径上跳应被拒绝")
	}
	if _, err := resolveComposePath(dir, "compose.yml"); err == nil {
		t.Fatal("不在白名单中的 compose 文件应被拒绝")
	}
}

func TestNewServerDefaultsAndValidation(t *testing.T) {
	server, err := NewServer(Config{
		ProjectDir:  t.TempDir(),
		ComposeFile: "docker-compose.yml",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !server.state.Enabled || server.cfg.Mode != "local" {
		t.Fatalf("默认状态不正确: %#v", server.state)
	}
	if _, err := NewServer(Config{ProjectDir: t.TempDir(), Mode: "invalid"}, nil); err == nil {
		t.Fatal("非法 mode 应被拒绝")
	}
	if _, err := NewServer(Config{ProjectDir: t.TempDir(), Mode: "git"}, nil); err == nil {
		t.Fatal("git 模式缺少 RepoURL 应被拒绝")
	}
}
