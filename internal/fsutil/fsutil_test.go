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

func TestBackupVersionedStoresContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nginx.conf")
	if err := os.WriteFile(path, []byte("worker_processes 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	version, err := BackupVersioned(path, 5)
	if err != nil || version == "" {
		t.Fatalf("应返回版本路径：%q %v", version, err)
	}
	if !strings.Contains(version, VersionsDir(path)) {
		t.Fatalf("版本应落在版本目录内：%s", version)
	}
	data, err := os.ReadFile(version)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "worker_processes 1;" {
		t.Fatalf("版本内容不符：%q", data)
	}
}

func TestBackupVersionedPrunesToKeep(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nginx.conf")
	// 每次写入不同内容：内容相同会被去重，剪枝也就无从检验
	for _, content := range []string{"v1", "v2", "v3", "v4", "v5"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := BackupVersioned(path, 2); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(VersionsDir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("应只保留 2 份版本，实际 %d", len(entries))
	}
}

func TestBackupVersionedSkipsMissingFile(t *testing.T) {
	version, err := BackupVersioned(filepath.Join(t.TempDir(), "nope.conf"), 5)
	if err != nil || version != "" {
		t.Fatalf("文件不存在时应返回空路径且无错误：%q %v", version, err)
	}
}

func TestListVersionsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nginx.conf")
	if err := os.WriteFile(path, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BackupVersioned(path, 5); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BackupVersioned(path, 5); err != nil {
		t.Fatal(err)
	}

	versions, err := ListVersions(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Name <= versions[1].Name {
		t.Fatalf("版本应按名称倒序（新的在前）列出：%v", versions)
	}
	seen := map[string]bool{}
	for _, version := range versions {
		data, err := ReadVersion(path, version.Name)
		if err != nil {
			t.Fatal(err)
		}
		if version.Size != int64(len(data)) {
			t.Fatalf("版本大小不符：%d vs %d", version.Size, len(data))
		}
		seen[string(data)] = true
	}
	if !seen["v1"] || !seen["v2"] {
		t.Fatalf("两个版本的内容都应在：%v", seen)
	}
}

func TestBackupVersionedSkipsIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nginx.conf")
	if err := os.WriteFile(path, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := BackupVersioned(path, 5)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BackupVersioned(path, 5)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("内容未变时不应新增版本：%q vs %q", first, second)
	}
	versions, err := ListVersions(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("内容未变时应只有 1 个版本，实际 %d", len(versions))
	}
}

func TestReadVersionRejectsTraversal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nginx.conf")
	if _, err := ReadVersion(path, "../nginx.conf"); err == nil {
		t.Fatal("带路径上跳的版本名应被拒绝")
	}
	if _, err := ReadVersion(path, "missing.conf"); err == nil {
		t.Fatal("不存在的版本应报错")
	}
	versions, err := ListVersions(path)
	if err != nil || len(versions) != 0 {
		t.Fatalf("版本目录不存在时应返回空列表且无错误：%v %v", versions, err)
	}
}

func TestVersionsDirIsHidden(t *testing.T) {
	path := filepath.Join("/etc/nginx/sites-enabled", "havlineagent-https.conf")
	base := filepath.Base(VersionsDir(path))
	if !strings.HasPrefix(base, ".") {
		t.Fatalf("版本目录应以点号开头，避免被 nginx 的 include * 加载：%s", base)
	}
}

func TestMigrateLegacyVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "havlineagent-https.conf")
	legacy := path + ".versions"
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "20260101T000000.000-abcd.conf"
	if err := os.WriteFile(filepath.Join(legacy, name), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	versions, err := ListVersions(path)
	if err != nil || len(versions) != 1 {
		t.Fatalf("迁移后应保留 1 个版本：%v %v", versions, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("旧版本目录应被移走，避免 nginx include * 命中：%v", err)
	}
	data, err := ReadVersion(path, versions[0].Name)
	if err != nil || string(data) != "old" {
		t.Fatalf("迁移后版本内容应可读：%q %v", data, err)
	}
}
