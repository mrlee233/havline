package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\nline4\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rows, truncated, err := tailFile(path, 2, maxLogLineBytes)
	if err != nil {
		t.Fatalf("tailFile: %v", err)
	}
	// 文件里还有更早的行没返回，因此应当标记为截断
	if !truncated {
		t.Fatal("只取两行时应标记为截断（还有更早的日志未返回）")
	}
	if len(rows) != 2 || rows[0] != "line3" || rows[1] != "line4" {
		t.Fatalf("unexpected rows: %#v", rows)
	}

	// 请求行数超过文件内容：返回全部且不标记截断
	rows, truncated, err = tailFile(path, 100, maxLogLineBytes)
	if err != nil || truncated || len(rows) != 4 {
		t.Fatalf("unexpected full read: rows=%d truncated=%v err=%v", len(rows), truncated, err)
	}

	// 单行超长时截断
	long := strings.Repeat("x", maxLogLineBytes+10)
	if err := os.WriteFile(path, []byte(long+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows, _, err = tailFile(path, 5, maxLogLineBytes)
	if err != nil {
		t.Fatalf("tailFile: %v", err)
	}
	if len(rows) != 1 || !strings.HasSuffix(rows[0], "…") || len(rows[0]) > maxLogLineBytes+3 {
		t.Fatalf("超长行未被截断：len=%d", len(rows[0]))
	}

	// 文件不存在返回错误，由调用方转成「200 + 空列表」
	if _, _, err := tailFile(filepath.Join(t.TempDir(), "missing.log"), 10, maxLogLineBytes); err == nil {
		t.Fatal("文件不存在应返回错误")
	}
}

func TestLogPathFromDump(t *testing.T) {
	dump := "# configuration file /etc/nginx/nginx.conf:\naccess_log /var/log/nginx/access.log main;\nerror_log /var/log/nginx/error.log warn;\n"
	if got := logPathFromDump(dump, "access_log"); got != "/var/log/nginx/access.log" {
		t.Fatalf("unexpected access log: %q", got)
	}
	if got := logPathFromDump(dump, "error_log"); got != "/var/log/nginx/error.log" {
		t.Fatalf("unexpected error log: %q", got)
	}

	// off / stderr / 相对路径 / 含变量 都不采用
	skip := "access_log off;\naccess_log stderr;\naccess_log logs/access.log;\naccess_log /var/log/nginx/$host.log;\n"
	if got := logPathFromDump(skip, "access_log"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
