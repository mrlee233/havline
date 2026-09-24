package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicReplacesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frps.toml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0o640); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("内容未替换：%q", data)
	}
}

// TestWriteFileAtomicLeavesNoTempFiles：临时文件必须被 rename 或清理，不能在配置目录里留垃圾。
func TestWriteFileAtomicLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nginx.conf")
	if err := WriteFileAtomic(path, []byte("worker_processes 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("残留临时文件：%s", entry.Name())
		}
	}
	if len(entries) != 1 {
		t.Fatalf("目录里应只剩目标文件，实际 %d 个", len(entries))
	}
}

func TestBackupSkipsMissingFile(t *testing.T) {
	if err := Backup(filepath.Join(t.TempDir(), "nope.conf")); err != nil {
		t.Fatalf("文件不存在时应视为无需备份，实际 %v", err)
	}
}

func TestBackupCopiesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frpc.toml")
	if err := os.WriteFile(path, []byte("token = \"x\""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Backup(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "token = \"x\"" {
		t.Fatalf("备份内容不符：%q", data)
	}
}
