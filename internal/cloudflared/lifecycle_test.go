package cloudflared

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/db"
	"github.com/havline/havline/internal/secret"
)

// newTestService 装配一个只依赖 SQLite 的 Service，cloudflared 二进制不存在时 Start 会失败，
// 正好用来验证「哪些隧道会被尝试拉起」而不真的启动进程。
func newTestService(t *testing.T) (*Service, *Store) {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	box, err := secret.NewBox("cloudflared-lifecycle-test")
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(conn)
	svc := NewService(config.Config{DataDir: dir}, store, box, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err := svc.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	return svc, store
}

// createTunnel 直接落库造一条隧道，跳过需要 Cloudflare API 的 Create。
func createTunnel(t *testing.T, store *Store, name string, autoStart bool) Tunnel {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	tunnel, err := store.Create(ctx, CreateInput{
		Name:      name,
		Mode:      ModeAccountLocal,
		Network:   DefaultNetworkSettings(),
		AutoStart: autoStart,
	}, "11111111-1111-1111-1111-111111111111", "acc-tag", "", "",
		filepath.Join(dir, "config.yml"), filepath.Join(dir, "creds.json"), filepath.Join(dir, "cloudflared.log"))
	if err != nil {
		t.Fatal(err)
	}
	return tunnel
}

// TestStartAutoSkipsNonAutoStart：应用启动只应拉起 auto_start 的隧道。
// 之前启动路径错用了 StartAll，用户手动停掉的隧道会在重启后自己回来。
func TestStartAutoSkipsNonAutoStart(t *testing.T) {
	tunnels := []Tunnel{
		{ID: 1, Name: "自动", AutoStart: true},
		{ID: 2, Name: "手动", AutoStart: false},
		{ID: 3, Name: "也自动", AutoStart: true},
	}
	targets := autoStartTargets(tunnels)
	if len(targets) != 2 {
		t.Fatalf("应只挑出 2 条 auto_start 隧道，实际 %d：%#v", len(targets), targets)
	}
	for _, tunnel := range targets {
		if !tunnel.AutoStart {
			t.Fatalf("挑出了未勾选 auto_start 的隧道：%#v", tunnel)
		}
	}
}

// TestStartAutoRespectsGlobalSwitch：全局总开关关闭时一条都不启动。
func TestStartAutoRespectsGlobalSwitch(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()
	tunnel := createTunnel(t, store, "自动", true)
	if _, err := svc.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}

	svc.StartAuto(ctx)

	got, err := store.Get(ctx, tunnel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == "failed" {
		t.Fatalf("全局停用时不应尝试启动隧道，实际 %q", got.Status)
	}
}

// TestStopAllMarksStopped：StopAll 应把所有隧道的运行态改写为 stopped，
// 供应用退出与「全局停用」两条路径复用。
func TestStopAllMarksStopped(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()
	first := createTunnel(t, store, "一", true)
	second := createTunnel(t, store, "二", true)
	for _, id := range []int64{first.ID, second.ID} {
		if err := store.SetRuntime(ctx, id, "connected", "", 0); err != nil {
			t.Fatal(err)
		}
	}

	svc.StopAll(ctx)

	for _, id := range []int64{first.ID, second.ID} {
		got, err := store.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "stopped" {
			t.Fatalf("隧道 %d 应为 stopped，实际 %q", id, got.Status)
		}
	}
}

// TestManagerStopIsIdempotent：没有子进程时 Stop 不应报错（退出路径会无差别调用）。
func TestManagerStopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(config.Config{DataDir: dir}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := m.Stop(42); err != nil {
		t.Fatalf("停止不存在的隧道不应报错：%v", err)
	}
	if status := m.Status(42); status.Status != "stopped" || status.Running {
		t.Fatalf("未运行的隧道状态应为 stopped：%#v", status)
	}
}
