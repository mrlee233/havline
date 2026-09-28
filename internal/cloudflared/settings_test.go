package cloudflared

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/fonu/fonu/internal/config"
	"github.com/fonu/fonu/internal/db"
	"github.com/fonu/fonu/internal/secret"
)

func TestAppSettingsDefaultToken(t *testing.T) {
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
	token := base64.StdEncoding.EncodeToString([]byte(`{"a":"acc-1","t":"tun-1","s":"sec-1"}`))

	settings, err := svc.SaveAppSettings(context.Background(), AppSettingsInput{
		Token: token, Mirror: "gh-proxy-org", Network: DefaultNetworkSettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !settings.DefaultTokenConfigured || settings.DefaultTokenMasked == "" {
		t.Fatalf("默认 token 应标记为已配置并返回掩码：%#v", settings)
	}

	tunnel, err := svc.Create(context.Background(), CreateInput{
		Name: "demo", Mode: ModeTokenLocal, Hostnames: []string{"a.example.com"}, Network: DefaultNetworkSettings(),
	})
	if err != nil {
		t.Fatalf("应能使用默认 token 创建隧道: %v", err)
	}
	if tunnel.TunnelID != "tun-1" || tunnel.AccountTag != "acc-1" {
		t.Fatalf("默认 token 解析结果不符：%#v", tunnel)
	}
}
