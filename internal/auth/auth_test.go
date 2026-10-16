package auth

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/havline/havline/internal/db"
)

// TestVerifyPassword：终端二次认证依赖的密码校验——对的放行，错的与不存在的账号都拒绝。
func TestVerifyPassword(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "havline.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := context.Background()
	svc := New(conn)
	if err := svc.Setup(ctx, "admin", "admin-password-123"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	sessionID, err := svc.Login(ctx, "admin", "admin-password-123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	adminID, err := svc.AdminIDForSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("admin id: %v", err)
	}

	if err := svc.VerifyPassword(ctx, adminID, "admin-password-123"); err != nil {
		t.Fatalf("正确密码应当通过：%v", err)
	}
	if err := svc.VerifyPassword(ctx, adminID, "admin-password-124"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("错误密码应当拒绝，实际 %v", err)
	}
	if err := svc.VerifyPassword(ctx, adminID, ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("空密码应当拒绝，实际 %v", err)
	}
	if err := svc.VerifyPassword(ctx, 9999, "admin-password-123"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("不存在的账号应当拒绝，实际 %v", err)
	}
}

func TestEnsureDefaultAdminCreatesOnce(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "havline.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	svc := New(conn)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if err := svc.EnsureDefaultAdmin(context.Background(), logger); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	initialized, err := svc.IsInitialized(context.Background())
	if err != nil || !initialized {
		t.Fatalf("expected initialized admin, got initialized=%v err=%v", initialized, err)
	}

	if err := svc.EnsureDefaultAdmin(context.Background(), logger); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	if _, err := svc.Login(context.Background(), "admin", "wrong-password"); err != ErrUnauthorized {
		t.Fatalf("expected unauthorized for wrong password, got %v", err)
	}
}

func TestEnsureDefaultAdminUsesEnvPassword(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "havline.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	t.Setenv("HAVLINE_INITIAL_ADMIN_PASSWORD", "env-password-123")
	svc := New(conn)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if err := svc.EnsureDefaultAdmin(context.Background(), logger); err != nil {
		t.Fatalf("bootstrap with env password: %v", err)
	}
	if _, err := svc.Login(context.Background(), "admin", "env-password-123"); err != nil {
		t.Fatalf("expected env password login to succeed, got %v", err)
	}
}
