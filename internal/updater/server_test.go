package updater

import (
	"context"
	"os"
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

func TestEnsureComposeFile(t *testing.T) {
	dir := t.TempDir()
	server, err := NewServer(Config{ProjectDir: dir, ComposeFile: "docker-compose.yml"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.ensureComposeFile(context.Background()); err == nil {
		t.Fatal("缺少 compose 文件时应返回错误")
	}
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := server.ensureComposeFile(context.Background()); err != nil {
		t.Fatalf("存在 compose 文件时不应报错: %v", err)
	}
}

func TestResolveComposePlanUsesHostProjectDir(t *testing.T) {
	containerDir := t.TempDir()
	hostDir := t.TempDir()
	server, err := NewServer(Config{
		ProjectDir:     containerDir,
		ComposeFile:    "docker-compose.hub.yml",
		HostProjectDir: hostDir,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := server.resolveComposePlan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !plan.helper {
		t.Fatal("容器内看不到 Compose 文件时应使用 helper 模式")
	}
	if plan.helperComposePath != filepath.Join("/workspace", "docker-compose.hub.yml") {
		t.Fatalf("helper compose 路径不正确: %s", plan.helperComposePath)
	}
}
