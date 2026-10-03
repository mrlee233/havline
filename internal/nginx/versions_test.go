package nginx

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/fsutil"
)

func newVersionTestManager(t *testing.T) (*Manager, config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:        dir,
		NginxBin:       "havline-nginx-not-installed",
		NginxPIDFile:   filepath.Join(dir, "nginx", "nginx.pid"),
		NginxMimeTypes: filepath.Join(dir, "mime.types"),
	}
	mgr := NewManager(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := mgr.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	return mgr, cfg
}

func TestConfigVersionsListAndRead(t *testing.T) {
	mgr, cfg := newVersionTestManager(t)
	path := cfg.NginxConfigPath()
	if err := os.WriteFile(path, []byte("worker_processes 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := fsutil.BackupVersioned(path, maxConfigVersions); err != nil {
		t.Fatal(err)
	}
	versions, err := mgr.ListConfigVersions()
	if err != nil || len(versions) != 1 {
		t.Fatalf("应列出 1 个版本：%v %v", versions, err)
	}
	content, err := mgr.ReadConfigVersion(versions[0].Name)
	if err != nil || content != "worker_processes 1;\n" {
		t.Fatalf("版本内容不符：%q %v", content, err)
	}
}

func TestRollbackConfigVersionRejectsTraversal(t *testing.T) {
	mgr, _ := newVersionTestManager(t)
	if _, err := mgr.RollbackConfigVersion(context.Background(), "../nginx.conf"); err == nil {
		t.Fatal("带路径上跳的版本名应被拒绝")
	}
}

func TestRollbackConfigVersionKeepsCurrentOnSyntaxError(t *testing.T) {
	mgr, cfg := newVersionTestManager(t)
	path := cfg.NginxConfigPath()
	if err := os.WriteFile(path, []byte("worker_processes 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := fsutil.VersionsDir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	badName := "20260101T000000.000-bad.conf"
	if err := os.WriteFile(filepath.Join(dir, badName), []byte("server {\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.RollbackConfigVersion(context.Background(), badName); err == nil {
		t.Fatal("语法错误的版本应拒绝回滚")
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != "worker_processes 1;\n" {
		t.Fatalf("校验失败时不应覆盖当前配置：%q %v", current, err)
	}
}

func TestRollbackConfigVersionRestoresAndKeepsCurrent(t *testing.T) {
	mgr, cfg := newVersionTestManager(t)
	path := cfg.NginxConfigPath()
	if err := os.WriteFile(path, []byte("worker_processes 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := fsutil.BackupVersioned(path, maxConfigVersions); err != nil {
		t.Fatal(err)
	}
	versions, err := mgr.ListConfigVersions()
	if err != nil || len(versions) != 1 {
		t.Fatalf("准备版本失败：%v %v", versions, err)
	}
	if err := os.WriteFile(path, []byte("worker_processes 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := mgr.RollbackConfigVersion(context.Background(), versions[0].Name)
	if err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	if !strings.Contains(result.Message, "已回滚") {
		t.Fatalf("回滚结果缺少提示：%q", result.Message)
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != "worker_processes 1;\n" {
		t.Fatalf("回滚后内容不符：%q %v", current, err)
	}
	after, err := mgr.ListConfigVersions()
	if err != nil || len(after) < 2 {
		t.Fatalf("回滚前的当前配置也应留存为版本：%v %v", after, err)
	}
}
