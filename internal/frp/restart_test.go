package frp

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/havline/havline/internal/certificate"
	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/db"
	"github.com/havline/havline/internal/secret"
	"github.com/havline/havline/internal/settings"
)

func newRestartTestService(t *testing.T) (*Service, *settings.Store, context.Context) {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "havline.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	box, err := secret.NewBox("restart-test-key")
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(config.Config{DataDir: dir}, NewStore(conn), box, logger, certificate.NewStore(conn))
	if err := svc.EnsureDirs(); err != nil {
		t.Fatalf("创建目录失败：%v", err)
	}
	settingsStore := settings.NewStore(conn)
	svc.SetSettingsStore(settingsStore)
	return svc, settingsStore, context.Background()
}

// 用户往往是从隧道那头点「重启」：Stop 一杀 frpc，连接就断、请求上下文随即取消。
// 重启流程必须无视这个取消走完，否则就是「停掉了却再也起不来」。
func TestRestartIgnoresCancelledRequestContext(t *testing.T) {
	svc, _, _ := newRestartTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.Restart(ctx)
	if err != nil && strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("重启不应被请求取消打断：%v", err)
	}
}

// frpc 的启停状态要能被记录与读回：应用停用再启用后据此自动恢复。
func TestFrpResumeStateRoundTrip(t *testing.T) {
	svc, _, ctx := newRestartTestService(t)

	if svc.lastSessionWasRunning(ctx) {
		t.Fatal("默认应为「上次未运行」")
	}
	svc.rememberResumeState(ctx, true)
	if !svc.lastSessionWasRunning(ctx) {
		t.Fatal("记录运行后应能读回，供下次启动自动恢复")
	}
	svc.rememberResumeState(ctx, false)
	if svc.lastSessionWasRunning(ctx) {
		t.Fatal("手动停止后不应再自动恢复")
	}
}

// 应用退出（StopForShutdown）不能把「上次在运行」标记抹掉，
// 否则「停用应用 → 重新启用」永远触发不了自动恢复。
func TestStopForShutdownKeepsResumeFlag(t *testing.T) {
	svc, _, ctx := newRestartTestService(t)

	// 模拟退出前 frpc 正在运行（真实环境里由 Status 判断，测试里直接给结论）
	svc.runningProbe = func(context.Context) bool { return true }
	svc.rememberResumeState(ctx, true)
	if err := svc.StopForShutdown(ctx); err != nil {
		t.Fatalf("退出时停止失败：%v", err)
	}
	if !svc.lastSessionWasRunning(ctx) {
		t.Fatal("退出后仍应保留「上次在运行」标记，供下次启用时自动恢复")
	}

	// 退出前本来就是停止的：不能凭空写成「在运行」
	svc.runningProbe = func(context.Context) bool { return false }
	if err := svc.StopForShutdown(ctx); err != nil {
		t.Fatalf("退出时停止失败：%v", err)
	}
	if svc.lastSessionWasRunning(ctx) {
		t.Fatal("退出前没在运行就不该标记为自动恢复")
	}
}
