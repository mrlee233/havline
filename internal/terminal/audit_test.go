package terminal

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/havline/havline/internal/db"
)

// TestAuditStoreRoundTrip：写入的审计能按时间倒序读回，并按保留期清理。
func TestAuditStoreRoundTrip(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}

	store := NewAuditStore(conn)
	ctx := context.Background()
	now := time.Now().UTC()
	entries := []AuditEntry{
		{
			SessionID: "s1", ServerID: 1, Target: "东京 VPS · root@10.0.0.1",
			User: "admin", ClientIP: "192.168.1.2",
			StartedAt:  now.Add(-2 * time.Minute).Format(time.RFC3339),
			EndedAt:    now.Add(-time.Minute).Format(time.RFC3339),
			DurationMS: 60_000, Reason: "用户关闭", BytesIn: 12, BytesOut: 4096,
		},
		{
			SessionID: "s2", ServerID: 2, Target: "洛杉矶 VPS · root@10.0.0.2",
			User: "admin", ClientIP: "10.1.1.1",
			StartedAt:  now.Format(time.RFC3339),
			EndedAt:    now.Add(time.Second).Format(time.RFC3339),
			DurationMS: 1_000, Reason: "空闲超时",
		},
	}
	for _, entry := range entries {
		if err := store.WriteAudit(ctx, entry); err != nil {
			t.Fatalf("写入审计失败：%v", err)
		}
	}

	got, err := store.ListAudits(ctx, 10)
	if err != nil {
		t.Fatalf("读取审计失败：%v", err)
	}
	if len(got) != 2 {
		t.Fatalf("期望 2 条审计，实际 %d 条", len(got))
	}
	// 按写入顺序倒序：最近结束的 s2 在前
	if got[0].SessionID != "s2" || got[1].SessionID != "s1" {
		t.Fatalf("排序不符：%+v", got)
	}
	if got[1].Reason != "用户关闭" || got[1].BytesOut != 4096 || got[1].ClientIP != "192.168.1.2" {
		t.Fatalf("字段回读不符：%+v", got[1])
	}

	// 保留期外的记录会被清理
	if _, err := conn.ExecContext(ctx,
		`UPDATE terminal_audit_logs SET ended_at = ? WHERE session_id = 's1'`,
		now.Add(-AuditRetention-time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("准备过期数据失败：%v", err)
	}
	deleted, err := store.PruneAudits(ctx)
	if err != nil {
		t.Fatalf("清理失败：%v", err)
	}
	if deleted != 1 {
		t.Fatalf("期望清理 1 条，实际 %d 条", deleted)
	}

	remaining, err := store.ListAudits(ctx, 10)
	if err != nil {
		t.Fatalf("读取审计失败：%v", err)
	}
	if len(remaining) != 1 || remaining[0].SessionID != "s2" {
		t.Fatalf("清理结果不符：%+v", remaining)
	}
	// limit 传 0 时回落到默认值，不应报错
	if _, err := store.ListAudits(ctx, 0); err != nil {
		t.Fatalf("默认 limit 读取失败：%v", err)
	}
}
