package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/havline/havline/internal/auth"
	"github.com/havline/havline/internal/db"
	"github.com/havline/havline/internal/settings"
	"github.com/havline/havline/internal/terminal"
)

const (
	testAdminPassword = "admin-password-123"
)

// TestTerminalUnlockStore：票据按时失效、可主动撤销，过期条目会被顺带清理。
func TestTerminalUnlockStore(t *testing.T) {
	store := newTerminalUnlockStore()
	const testSessionID = "session-abc"
	if _, ok := store.expiresAt(testSessionID); ok {
		t.Fatal("未解锁的会话不应有效")
	}
	expires := store.grant(testSessionID)
	if !expires.After(time.Now()) {
		t.Fatalf("解锁到期时间应当在未来：%v", expires)
	}
	if _, ok := store.expiresAt(testSessionID); !ok {
		t.Fatal("解锁后应当有效")
	}
	store.revoke(testSessionID)
	if _, ok := store.expiresAt(testSessionID); ok {
		t.Fatal("锁定后应当失效")
	}

	// 手动塞一条过期记录，grant 时应当被清掉
	store.expires["stale"] = time.Now().Add(-time.Minute)
	store.grant("other")
	if _, ok := store.expires["stale"]; ok {
		t.Fatal("过期条目未被清理")
	}
}

// TestTerminalUnlockFlow：未解锁不能建会话 → 密码错误被拒并计数 → 解锁后可建会话 → 可主动锁定。
func TestTerminalUnlockFlow(t *testing.T) {
	handler, openerCalled, sessionID := newUnlockTestHandler(t)

	// 1) 未解锁：建会话被拒，并给出稳定错误码
	rec := callTerminal(t, handler.CreateSession, http.MethodPost, `{"server_id":1,"cols":80,"rows":24}`, sessionID)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("未解锁建会话应当 403，实际 %d %s", rec.Code, rec.Body.String())
	}
	if code := errorCode(t, rec); code != "terminal_locked" {
		t.Fatalf("错误码应当是 terminal_locked，实际 %q", code)
	}
	if *openerCalled {
		t.Fatal("未解锁就触发了后端连接")
	}

	// 2) 密码错误：401，且不发放票据
	rec = callTerminal(t, handler.Unlock, http.MethodPost, `{"password":"wrong-password"}`, sessionID)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误密码应当 401，实际 %d %s", rec.Code, rec.Body.String())
	}
	if code := errorCode(t, rec); code != "bad_password" {
		t.Fatalf("错误码应当是 bad_password，实际 %q", code)
	}
	if _, ok := handler.unlocks.expiresAt(sessionID); ok {
		t.Fatal("密码错误却发放了解锁票据")
	}

	// 3) 密码正确：解锁成功
	rec = callTerminal(t, handler.Unlock, http.MethodPost, `{"password":"`+testAdminPassword+`"}`, sessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("正确密码应当 200，实际 %d %s", rec.Code, rec.Body.String())
	}
	var unlocked struct {
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &unlocked); err != nil || unlocked.ExpiresAt == "" {
		t.Fatalf("解锁响应缺少 expires_at：%s", rec.Body.String())
	}

	// 状态接口能读到剩余时间
	rec = callTerminal(t, handler.UnlockStatus, http.MethodGet, "", sessionID)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"unlocked":true`) {
		t.Fatalf("解锁状态不符：%d %s", rec.Code, rec.Body.String())
	}

	// 4) 已解锁：放行到后端连接（测试用 Opener 固定失败，所以这里是 502 而不是 403）
	rec = callTerminal(t, handler.CreateSession, http.MethodPost, `{"server_id":1,"cols":80,"rows":24}`, sessionID)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("解锁后仍被拦：%s", rec.Body.String())
	}
	if !*openerCalled {
		t.Fatal("解锁后应当尝试连接后端")
	}

	// 5) 主动锁定
	rec = callTerminal(t, handler.Lock, http.MethodDelete, "", sessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("锁定失败：%d %s", rec.Code, rec.Body.String())
	}
	if _, ok := handler.unlocks.expiresAt(sessionID); ok {
		t.Fatal("锁定后票据仍有效")
	}
}

// TestTerminalUnlockRateLimit：同一 IP 连续失败到阈值后被锁定。
func TestTerminalUnlockRateLimit(t *testing.T) {
	handler, _, sessionID := newUnlockTestHandler(t)
	for i := 0; i < loginFailureLimit; i++ {
		rec := callTerminal(t, handler.Unlock, http.MethodPost, `{"password":"wrong-password"}`, sessionID)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次失败应当 401，实际 %d", i+1, rec.Code)
		}
	}
	rec := callTerminal(t, handler.Unlock, http.MethodPost, `{"password":"wrong-password"}`, sessionID)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("超过阈值应当 429，实际 %d %s", rec.Code, rec.Body.String())
	}
	if code := errorCode(t, rec); code != "too_many_attempts" {
		t.Fatalf("错误码应当是 too_many_attempts，实际 %q", code)
	}
	// 锁定期间即使密码正确也不放行
	rec = callTerminal(t, handler.Unlock, http.MethodPost, `{"password":"`+testAdminPassword+`"}`, sessionID)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("锁定期间应当继续 429，实际 %d", rec.Code)
	}
}

// TestTerminalCommandsAPI：常用命令的新增、列表与删除。
func TestTerminalCommandsAPI(t *testing.T) {
	handler, _, sessionID := newUnlockTestHandler(t)

	rec := callTerminal(t, handler.CreateCommand, http.MethodPost, `{"name":"磁盘占用","command":"df -h"}`, sessionID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("新增命令应当 201，实际 %d %s", rec.Code, rec.Body.String())
	}
	var created terminal.TerminalCommand
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("新增响应缺少 ID：%s", rec.Body.String())
	}

	rec = callTerminal(t, handler.ListCommands, http.MethodGet, "", sessionID)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "df -h") {
		t.Fatalf("列表未包含新增命令：%d %s", rec.Code, rec.Body.String())
	}

	// 空名称应当被拒
	rec = callTerminal(t, handler.CreateCommand, http.MethodPost, `{"name":"  ","command":"ls"}`, sessionID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("空名称应当 400，实际 %d", rec.Code)
	}

	// 删除要走带路径参数的分支，这里手工塞 PathValue
	if code := deleteCommand(t, handler, sessionID, created.ID); code != http.StatusOK {
		t.Fatalf("删除应当 200，实际 %d", code)
	}
	if code := deleteCommand(t, handler, sessionID, created.ID); code != http.StatusNotFound {
		t.Fatalf("重复删除应当 404，实际 %d", code)
	}
}

// deleteCommand 按路由形态调用删除接口（PathValue 由 ServeMux 注入，测试里手工设置）。
func deleteCommand(t *testing.T, handler *TerminalHandler, sessionID string, id int64) int {
	t.Helper()
	raw := strconv.FormatInt(id, 10)
	req := httptest.NewRequest(http.MethodDelete, "/api/terminal/commands/"+raw, nil)
	req.AddCookie(&http.Cookie{Name: "havline_session", Value: sessionID})
	req.SetPathValue("id", raw)
	rec := httptest.NewRecorder()
	SessionMiddleware(nil)(http.HandlerFunc(handler.DeleteCommand)).ServeHTTP(rec, req)
	return rec.Code
}

// TestTokenDeniedPathCoversTerminal：终端不接受 API Token，否则二次认证可以被绕过。
func TestTokenDeniedPathCoversTerminal(t *testing.T) {
	for _, path := range []string{
		"/api/terminal/targets",
		"/api/terminal/unlock",
		"/api/terminal/sessions",
	} {
		if !tokenDeniedPath(path) {
			t.Fatalf("%s 应当只允许浏览器会话", path)
		}
	}
	if tokenDeniedPath("/api/proxies") {
		t.Fatal("普通接口不应被限制")
	}
}

// newUnlockTestHandler 造一套「终端已开启 + 已初始化管理员」的处理器，并返回一个真实会话 ID。
func newUnlockTestHandler(t *testing.T) (*TerminalHandler, *bool, string) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "havline.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}

	ctx := context.Background()
	authSvc := auth.New(conn)
	if err := authSvc.Setup(ctx, "admin", testAdminPassword); err != nil {
		t.Fatalf("初始化管理员失败：%v", err)
	}
	settingsStore := settings.NewStore(conn)
	if err := settingsStore.SetBool(ctx, settings.KeyTerminalEnabled, true); err != nil {
		t.Fatalf("打开终端开关失败：%v", err)
	}
	// 解锁票据绑定真实会话：没有这条会话记录，AdminIDForSession 会先拒绝
	sessionID, err := authSvc.Login(ctx, "admin", testAdminPassword)
	if err != nil {
		t.Fatalf("登录失败：%v", err)
	}

	openerCalled := false
	manager := terminal.NewManager(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(context.Context, int64, int, int) (terminal.Terminal, string, error) {
			openerCalled = true
			return nil, "", errors.New("测试环境不建立真实连接")
		},
		terminal.Options{},
	)

	handler := NewTerminalHandler(TerminalHandlerOptions{
		Auth:     authSvc,
		Settings: settingsStore,
		Manager:  manager,
		Unlocks:  newTerminalUnlockStore(),
		Commands: terminal.NewCommandStore(conn),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return handler, &openerCalled, sessionID
}

// callTerminal 带上会话 Cookie 调用处理器；SessionMiddleware 只读 Cookie，这里复用它注入会话。
func callTerminal(t *testing.T, next http.HandlerFunc, method, body, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/api/terminal", reader)
	req.AddCookie(&http.Cookie{Name: "havline_session", Value: sessionID})
	rec := httptest.NewRecorder()
	SessionMiddleware(nil)(next).ServeHTTP(rec, req)
	return rec
}

// errorCode 读出响应体里的稳定错误码。
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析错误响应失败：%v", err)
	}
	return body.Code
}
