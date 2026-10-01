package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDueForBackup(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

	// 还没有备份 → 立刻做一份
	if !dueForBackup(time.Time{}, now, AutoInterval) {
		t.Fatal("没有备份时应立即备份")
	}
	// 刚备份过 → 跳过：这正是重启不会重复备份的原因
	if dueForBackup(now.Add(-time.Hour), now, AutoInterval) {
		t.Fatal("间隔内的备份不应重复执行")
	}
	// 正好到间隔 → 该做了
	if !dueForBackup(now.Add(-AutoInterval), now, AutoInterval) {
		t.Fatal("到达间隔应再备份一次")
	}
	// 超过间隔 → 该做
	if !dueForBackup(now.Add(-25*time.Hour), now, AutoInterval) {
		t.Fatal("超过间隔应再备份一次")
	}
}

func TestExportVerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "havline.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := New(dir)

	path, err := svc.Export(context.Background())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	entries, err := svc.Verify(context.Background(), path)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if entries < 1 {
		t.Fatalf("条目数应至少为 1，实际 %d", entries)
	}

	archives, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(archives) != 1 {
		t.Fatalf("应列出 1 份备份，实际 %d", len(archives))
	}
	if archives[0].Size == 0 || !archiveNamePattern.MatchString(archives[0].Name) {
		t.Fatalf("备份条目异常：%+v", archives[0])
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	svc := New(t.TempDir())
	path := filepath.Join(svc.backupsDir(), "havline-backup-20260925-000000-1.tar.gz")
	if err := os.MkdirAll(svc.backupsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a gzip stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Verify(context.Background(), path); err == nil {
		t.Fatal("非 gzip 内容应校验失败")
	}
}

func TestListPruneAndPath(t *testing.T) {
	svc := New(t.TempDir())
	if err := os.MkdirAll(svc.backupsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"havline-backup-20260923-100000-1.tar.gz",
		"havline-backup-20260924-100000-2.tar.gz",
		"havline-backup-20260925-100000-3.tar.gz",
	} {
		if err := os.WriteFile(filepath.Join(svc.backupsDir(), name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// 不符合命名格式的文件不参与：备份目录里可能混着手工放进来的东西
	if err := os.WriteFile(filepath.Join(svc.backupsDir(), "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	archives, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(archives) != 3 {
		t.Fatalf("应只列出 3 份备份，实际 %d", len(archives))
	}
	if archives[0].Name != "havline-backup-20260925-100000-3.tar.gz" {
		t.Fatalf("应按时间倒序，实际首条 %s", archives[0].Name)
	}

	removed, err := svc.Prune(2)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 1 {
		t.Fatalf("应删除 1 份，实际 %d", removed)
	}
	if archives, _ = svc.List(); len(archives) != 2 {
		t.Fatalf("剪枝后应剩 2 份，实际 %d", len(archives))
	}

	// 名称校验：不能让备份名把读写带出备份目录
	for _, name := range []string{"../havline.db", "notes.txt", "", "havline-backup-x.tar.gz"} {
		if _, err := svc.Path(name); err == nil {
			t.Fatalf("非法备份名应拒绝：%q", name)
		}
	}
	if err := svc.Remove("havline-backup-20260920-100000-9.tar.gz"); err == nil {
		t.Fatal("删除不存在的备份应报错")
	}
	if _, err := svc.Prune(0); err != nil {
		t.Fatalf("keep<=0 应直接返回：%v", err)
	}
}
