package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/havline/havline/internal/db"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "havline.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(conn)
}

func TestAPITokenLifecycle(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	created, err := svc.CreateAPIToken(ctx, "homeassistant", TokenScopeRead, time.Time{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Token == "" || created.Scope != TokenScopeRead || created.ID == 0 {
		t.Fatalf("unexpected created token: %+v", created)
	}

	// 库里只存 SHA-256：明文不落盘
	var stored string
	if err := svc.db.QueryRowContext(ctx, `SELECT token_hash FROM api_tokens WHERE id = ?`, created.ID).Scan(&stored); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	if stored == created.Token || len(stored) != 64 {
		t.Fatalf("令牌应以 SHA-256 存储，实际 %q", stored)
	}

	scope, err := svc.ValidateAPIToken(ctx, created.Token)
	if err != nil || scope != TokenScopeRead {
		t.Fatalf("validate: scope=%q err=%v", scope, err)
	}

	var lastUsed string
	if err := svc.db.QueryRowContext(ctx, `SELECT COALESCE(last_used_at, '') FROM api_tokens WHERE id = ?`, created.ID).Scan(&lastUsed); err != nil {
		t.Fatalf("read last_used_at: %v", err)
	}
	if lastUsed == "" {
		t.Fatal("校验通过后应回写 last_used_at")
	}

	if _, err := svc.ValidateAPIToken(ctx, "not-a-real-token"); err != ErrUnauthorized {
		t.Fatalf("未知令牌应拒绝，实际 %v", err)
	}
	if err := svc.DeleteAPIToken(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.ValidateAPIToken(ctx, created.Token); err != ErrUnauthorized {
		t.Fatalf("删除后令牌应失效，实际 %v", err)
	}
	if err := svc.DeleteAPIToken(ctx, created.ID); err == nil {
		t.Fatal("重复删除应报错")
	}
}

func TestAPITokenExpiry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	expired, err := svc.CreateAPIToken(ctx, "old-script", TokenScopeWrite, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.ValidateAPIToken(ctx, expired.Token); err != ErrUnauthorized {
		t.Fatalf("过期令牌应拒绝，实际 %v", err)
	}
}

func TestAPITokenRejectsBadInput(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateAPIToken(ctx, "   ", TokenScopeRead, time.Time{}); err == nil {
		t.Fatal("空名称应报错")
	}
	if _, err := svc.CreateAPIToken(ctx, "bad-scope", "admin", time.Time{}); err == nil {
		t.Fatal("未知权限范围应报错")
	}
	if _, err := svc.ValidateAPIToken(ctx, "  "); err != ErrUnauthorized {
		t.Fatalf("空令牌应拒绝，实际 %v", err)
	}
}

func TestListAPITokensReturnsNewestFirst(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateAPIToken(ctx, "first", TokenScopeRead, time.Time{}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := svc.CreateAPIToken(ctx, "second", TokenScopeWrite, time.Time{}); err != nil {
		t.Fatalf("create second: %v", err)
	}
	tokens, err := svc.ListAPITokens(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tokens) != 2 || tokens[0].Name != "second" {
		t.Fatalf("应按 id 倒序返回：%+v", tokens)
	}
}
