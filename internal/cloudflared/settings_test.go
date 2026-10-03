package cloudflared

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/db"
	"github.com/havline/havline/internal/secret"
)

func TestAppSettingsAccountAndAPIToken(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	box, err := secret.NewBox("cloudflared-settings-test")
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(config.Config{DataDir: dir}, NewStore(conn), box, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	settings, err := svc.SaveAppSettings(context.Background(), AppSettingsInput{
		AccountID: "acc-1", APIToken: "cfat_test_token", Mirror: "gh-proxy-org", Network: DefaultNetworkSettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings.AccountID != "acc-1" || !settings.APITokenConfigured || settings.APITokenMasked == "" {
		t.Fatalf("Account ID 与 API Token 应保存并回显掩码：%#v", settings)
	}
	api, err := svc.cloudflareAPI(context.Background())
	if err != nil || api.AccountID != "acc-1" || api.APIToken != "cfat_test_token" {
		t.Fatalf("cloudflareAPI 构造失败：%v %#v", err, api)
	}
}

func TestSetEnabledBlocksStart(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	box, err := secret.NewBox("cloudflared-enabled-test")
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(config.Config{DataDir: dir}, NewStore(conn), box, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	settings, err := svc.SetEnabled(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled {
		t.Fatal("全局托管应为停用")
	}
	if _, err := svc.Start(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "全局已停用") {
		t.Fatalf("全局停用时启动应被拒绝：%v", err)
	}
	settings, err = svc.SetEnabled(context.Background(), true)
	if err != nil || !settings.Enabled {
		t.Fatalf("重新启用失败：%v %#v", err, settings)
	}
}

func TestManagedTunnelPersists(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	store := NewStore(conn)
	item, err := store.Create(context.Background(), CreateInput{
		Name:      "managed",
		Mode:      ModeAccountLocal,
		Network:   DefaultNetworkSettings(),
		Managed:   true,
		AutoStart: true,
	}, "remote-tunnel", "account", "encrypted", "", "/tmp/config.yml", "/tmp/creds.json", "/tmp/cloudflared.log")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Managed || !item.AutoStart || item.TunnelID != "remote-tunnel" {
		t.Fatalf("托管隧道字段未持久化：%#v", item)
	}
}
