package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/havline/havline/internal/db"
	"github.com/havline/havline/internal/terminal"
)

// TestListAuditsHandler：审计接口返回落库记录；未接存储时返回空数组而不是 null。
func TestListAuditsHandler(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}

	store := terminal.NewAuditStore(conn)
	now := time.Now().UTC()
	entry := terminal.AuditEntry{
		SessionID: "s1", ServerID: 3, Target: "东京 VPS · root@10.0.0.1",
		User: "admin", ClientIP: "192.168.1.9",
		StartedAt: now.Format(time.RFC3339), EndedAt: now.Add(time.Second).Format(time.RFC3339),
		DurationMS: 1_000, Reason: "用户关闭",
	}
	if err := store.WriteAudit(context.Background(), entry); err != nil {
		t.Fatalf("写入审计失败：%v", err)
	}

	handler := NewTerminalHandler(TerminalHandlerOptions{Audits: store})
	rec := httptest.NewRecorder()
	handler.ListAudits(rec, httptest.NewRequest(http.MethodGet, "/api/terminal/audits", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码不符：%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Entries []terminal.AuditEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败：%v", err)
	}
	if len(body.Entries) != 1 {
		t.Fatalf("期望 1 条审计，实际 %d 条", len(body.Entries))
	}
	if body.Entries[0].User != "admin" || body.Entries[0].Reason != "用户关闭" {
		t.Fatalf("审计字段不符：%+v", body.Entries[0])
	}

	// 未接存储时也要给前端一个数组，避免前端拿到 null 崩掉
	empty := NewTerminalHandler(TerminalHandlerOptions{})
	emptyRec := httptest.NewRecorder()
	empty.ListAudits(emptyRec, httptest.NewRequest(http.MethodGet, "/api/terminal/audits", nil))
	if got := emptyRec.Body.String(); got != "{\"entries\":[]}\n" {
		t.Fatalf("空审计响应不符：%s", got)
	}
}
