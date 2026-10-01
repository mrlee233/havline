package auth

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/havline/havline/internal/db"
)

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
