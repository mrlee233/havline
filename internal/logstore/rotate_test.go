package logstore

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRotateIfNeededSkipsSmallFile：未超阈值不能产生任何副作用，否则每小时一次的
// 定时任务会把日志切成无数小文件。
func TestRotateIfNeededSkipsSmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	if err := os.WriteFile(path, []byte("small\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	reopened := 0
	r := NewRotator(path, 0, func() error {
		reopened++
		return nil
	})
	rotated, err := r.RotateIfNeeded(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if rotated {
		t.Fatal("未超阈值不应轮转")
	}
	if reopened != 0 {
		t.Fatal("未轮转时不应通知重开日志")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("原文件应保持不动：%v", err)
	}
}

// TestRotateCompressesAndReopens：超阈值时要归档成 .gz、通知进程重开文件、并删掉原文件。
func TestRotateCompressesAndReopens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	content := strings.Repeat("2026-10-24T10:00:00+08:00 a.example.com 18080 GET / 200 0.010 1.2.3.4 10.0.0.2:80 100 200\n", 200)
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}

	reopened := 0
	r := NewRotator(path, 0, func() error {
		reopened++
		return nil
	})
	r.MaxBytes = 1024 // 强制触发
	now := time.Date(2026, 10, 24, 13, 5, 0, 0, time.Local)
	rotated, err := r.RotateIfNeeded(now)
	if err != nil {
		t.Fatal(err)
	}
	if !rotated {
		t.Fatal("超过阈值应轮转")
	}
	if reopened != 1 {
		t.Fatalf("轮转后必须通知重开日志，实际调用 %d 次", reopened)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("轮转后原文件应被移走")
	}

	archive := filepath.Join(dir, "access-20261024-1305.log.gz")
	file, err := os.Open(archive)
	if err != nil {
		t.Fatalf("应生成归档文件 %s：%v", archive, err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	data, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("归档内容应与原文件一致：got %d bytes want %d", len(data), len(content))
	}
}

// TestRotatePrunesOldArchives：归档份数与保留天数都要生效。
func TestRotatePrunesOldArchives(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")

	// 造 3 个既有归档，其中 2 个已经超期
	old := time.Now().Add(-40 * 24 * time.Hour)
	stale := []string{"access-20200101-0000.log.gz", "access-20200102-0000.log.gz"}
	for _, name := range stale {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	fresh := filepath.Join(dir, "access-20261024-1200.log.gz")
	if err := os.WriteFile(fresh, []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(strings.Repeat("line\n", 500)), 0o640); err != nil {
		t.Fatal(err)
	}
	r := NewRotator(path, 7, nil) // 保留 7 天
	r.MaxBytes = 1024
	if _, err := r.RotateIfNeeded(time.Now()); err != nil {
		t.Fatal(err)
	}

	for _, name := range stale {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("超过保留天数的归档应被删除：%s", name)
		}
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("未超期的归档应保留：%v", err)
	}
}

// TestTailLinesReadsFromEnd：尾部读取既要拿对最后几行，也不能因为文件很大而读全量。
func TestTailLinesReadsFromEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	var b strings.Builder
	for i := 0; i < 5000; i++ {
		b.WriteString("line-")
		b.WriteString(strings.Repeat("x", 20))
		b.WriteString("\n")
	}
	b.WriteString("last-line-a\nlast-line-b\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o640); err != nil {
		t.Fatal(err)
	}

	lines, err := tailLines(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{strings.TrimSpace("line-xxxxxxxxxxxxxxxxxxxx"), "last-line-a", "last-line-b"}
	if len(lines) != len(want) {
		t.Fatalf("应返回 3 行，实际 %d：%v", len(lines), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("第 %d 行不符：got %q want %q", i, lines[i], want[i])
		}
	}
}

// TestTailLinesSingleLongLine：单行超过初始块大小时不能被截断成多行。
func TestTailLinesSingleLongLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	long := strings.Repeat("y", 300*1024)
	if err := os.WriteFile(path, []byte(long+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	lines, err := tailLines(path, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != long {
		t.Fatalf("超长单行应完整返回，实际 %d 行（首行长度 %d）", len(lines), len(lines[0]))
	}
}

// TestTailLinesMissingFile：日志还没产生时返回空而不是报错。
func TestTailLinesMissingFile(t *testing.T) {
	lines, err := tailLines(filepath.Join(t.TempDir(), "nope.log"), 10)
	if err != nil {
		t.Fatalf("文件不存在不应报错：%v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("应返回空行列表，实际 %v", lines)
	}
}
