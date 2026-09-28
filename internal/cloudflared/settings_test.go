package cloudflared

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/fonu/fonu/internal/config"
	"github.com/fonu/fonu/internal/db"
	"github.com/fonu/fonu/internal/secret"
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
