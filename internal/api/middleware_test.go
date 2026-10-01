package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/havline/havline/internal/auth"
	"github.com/havline/havline/internal/db"
)

func newTokenTestAuth(t *testing.T, scope string) (*auth.Service, string) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "havline.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := auth.New(conn)
	created, err := svc.CreateAPIToken(context.Background(), "test", scope, time.Time{})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return svc, created.Token
}

func okTokenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
}

func TestRequireAuthAcceptsReadTokenForGet(t *testing.T) {
	svc, plain := newTokenTestAuth(t, auth.TokenScopeRead)
	handler := RequireAuth(svc)(okTokenHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("只读令牌应能读，实际 %d", rec.Code)
	}
}

func TestRequireAuthBlocksWriteForReadToken(t *testing.T) {
	svc, plain := newTokenTestAuth(t, auth.TokenScopeRead)
	handler := RequireAuth(svc)(okTokenHandler())

	req := httptest.NewRequest(http.MethodDelete, "/api/proxies/1", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("只读令牌不应能写，实际 %d", rec.Code)
	}
}

func TestRequireAuthAllowsWriteForWriteToken(t *testing.T) {
	svc, plain := newTokenTestAuth(t, auth.TokenScopeWrite)
	handler := RequireAuth(svc)(okTokenHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/proxies", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("写令牌应能写，实际 %d", rec.Code)
	}
}

func TestRequireAuthTokenCannotManageTokens(t *testing.T) {
	// 写令牌也不能管理令牌：否则等于拿令牌给自己提权或无限续期
	svc, plain := newTokenTestAuth(t, auth.TokenScopeWrite)
	handler := RequireAuth(svc)(okTokenHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/settings/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("令牌管理应只允许登录会话，实际 %d", rec.Code)
	}
}

func TestRequireAuthRejectsBadCredentials(t *testing.T) {
	svc, _ := newTokenTestAuth(t, auth.TokenScopeRead)
	handler := RequireAuth(svc)(okTokenHandler())

	cases := map[string]string{
		"未知令牌": "Bearer deadbeef",
		"错误前缀": "Token abc",
		"没有凭据": "",
	}
	for name, header := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s 应返回 401，实际 %d", name, rec.Code)
		}
	}
}

func TestBearerTokenParsing(t *testing.T) {
	cases := map[string]string{
		"Bearer abc":  "abc",
		"bearer abc":  "abc",
		"Bearer  abc": "abc",
		"Basic abc":   "",
		"Bearer ":     "",
		"":            "",
	}
	for header, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		if got := bearerToken(req); got != want {
			t.Fatalf("Authorization %q 解析为 %q，期望 %q", header, got, want)
		}
	}
}
