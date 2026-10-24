package auth

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/havline/havline/internal/db"
)

// TestPruneExpiredSessions 过期会话应被日任务清掉，未过期的必须留着。
func TestPruneExpiredSessions(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "havline.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()
	if err := db.Migrate(ctx, conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	svc := New(conn)
	if err := svc.Setup(ctx, "admin", "admin-password-123"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	adminID, err := svc.AdminIDForSession(ctx, mustLogin(t, svc))
	if err != nil {
		t.Fatalf("admin id: %v", err)
	}

	now := time.Now().UTC()
	expired := []string{"expired-1", "expired-2"}
	for _, id := range expired {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO sessions(id, admin_id, expires_at) VALUES (?, ?, ?)`,
			id, adminID, now.Add(-time.Hour).Format(time.RFC3339)); err != nil {
			t.Fatalf("insert expired session: %v", err)
		}
	}
	future := "still-valid"
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO sessions(id, admin_id, expires_at) VALUES (?, ?, ?)`,
		future, adminID, now.Add(time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("insert valid session: %v", err)
	}

	before := countSessions(t, conn)
	removed, err := svc.PruneExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != int64(len(expired)) {
		t.Fatalf("应删除 %d 条过期会话，实际 %d", len(expired), removed)
	}
	if after := countSessions(t, conn); after != before-int64(len(expired)) {
		t.Fatalf("会话数不符：before=%d after=%d", before, after)
	}
	if err := svc.ValidateSession(ctx, future); err != nil {
		t.Fatalf("未过期会话不应被删：%v", err)
	}

	// 再跑一次应当没有可删的（幂等）
	if removed, err = svc.PruneExpiredSessions(ctx); err != nil || removed != 0 {
		t.Fatalf("重复清理应为空操作：removed=%d err=%v", removed, err)
	}
}

func mustLogin(t *testing.T, svc *Service) string {
	t.Helper()
	sessionID, err := svc.Login(context.Background(), "admin", "admin-password-123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return sessionID
}

func countSessions(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	var count int64
	if err := conn.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM sessions`).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return count
}
