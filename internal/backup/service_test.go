package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeArchive 造一个含指定条目的 tar.gz，用于验证还原侧的路径校验与权限处理。
func writeArchive(t *testing.T, entries map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		header := &tar.Header{Name: name, Mode: 0o777, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRestoreRejectsEscapingEntries 覆盖「同前缀绕过」：旧实现用 strings.HasPrefix 判断，
// DataDir=/data 时 ../data-other/x 会通过前缀匹配写到 DataDir 之外。
func TestRestoreRejectsEscapingEntries(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		t.Fatal(err)
	}
	archive := writeArchive(t, map[string]string{"../data-other/evil.txt": "x"})
	if err := New(dataDir).Restore(context.Background(), archive); err == nil {
		t.Fatal("应当拒绝逃出 DataDir 的条目")
	}
	if _, err := os.Stat(filepath.Join(root, "data-other", "evil.txt")); err == nil {
		t.Fatal("逃逸条目被写到了 DataDir 之外")
	}
}

func TestRestoreRejectsParentEscape(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		t.Fatal(err)
	}
	archive := writeArchive(t, map[string]string{"../../evil.txt": "x"})
	if err := New(dataDir).Restore(context.Background(), archive); err == nil {
		t.Fatal("应当拒绝带 .. 的条目")
	}
}

// TestRestoreAppliesFixedMode：归档里的权限位不照搬（构造的归档可带 0777 / 执行位）。
func TestRestoreAppliesFixedMode(t *testing.T) {
	dataDir := t.TempDir()
	archive := writeArchive(t, map[string]string{"nginx/nginx.conf": "worker_processes 1;"})
	if err := New(dataDir).Restore(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dataDir, "nginx", "nginx.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS == "windows" {
		// Windows 不保留 Unix 权限位（可写文件统一报 0666）：只断言没带上执行位
		if got&0o111 != 0 {
			t.Fatalf("还原后的文件不应带执行位，实际 %o", got)
		}
	} else if got != restoreFileMode {
		t.Fatalf("还原后的权限应为 %o，实际 %o", restoreFileMode, got)
	}
}

func TestExportIncludesCloudflareTunnelConfig(t *testing.T) {
	dataDir := t.TempDir()
	tunnelDir := filepath.Join(dataDir, "cloudflared", "tunnels", "home")
	if err := os.MkdirAll(tunnelDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tunnelDir, "config.yml"), []byte("tunnel: test\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	path, err := New(dataDir).Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	found := false
	for {
		header, err := tr.Next()
		if err != nil {
			break
		}
		if header.Name == "cloudflared/tunnels/home/config.yml" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("备份应包含 Cloudflare 隧道配置")
	}
}
