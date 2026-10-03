package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/havline/havline/internal/auth"
	"github.com/havline/havline/internal/certificate"
	"github.com/havline/havline/internal/cloudflared"
	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/db"
	"github.com/havline/havline/internal/frp"
	"github.com/havline/havline/internal/secret"
	"github.com/havline/havline/internal/settings"
)

var routeParamPattern = regexp.MustCompile(`\{[^}]+\}`)

// TestProtectedRoutesRequireAuth 遍历路由表：受保护路由匿名必须 401，公开路由不能被鉴权误拦。
func TestProtectedRoutesRequireAuth(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "havline.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	authSvc := auth.New(conn)
	box, err := secret.NewBox("router-auth-test-key")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataDir: dir, NginxPIDFile: filepath.Join(dir, "nginx.pid")}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	frpSvc := frp.NewService(cfg, frp.NewStore(conn), box, logger, certificate.NewStore(conn))
	cfSvc := cloudflared.NewService(cfg, cloudflared.NewStore(conn), box, logger, nil)

	handler, specs := newRouterWithSpecs(Deps{
		Config:     cfg,
		Auth:       authSvc,
		FRPMulti:   frpSvc,
		Cloudflare: cfSvc,
		Settings:   settings.NewStore(conn),
		Logger:     logger,
	})

	protected, public := 0, 0
	for _, spec := range specs {
		req := httptest.NewRequest(routeMethod(spec.Pattern), sampleRoutePath(spec.Pattern), nil)
		code := callRouteSafely(handler, req)
		if spec.Public {
			public++
			if code == http.StatusUnauthorized {
				t.Fatalf("公开路由 %s 不应被鉴权拦截", spec.Pattern)
			}
			continue
		}
		protected++
		if code != http.StatusUnauthorized {
			t.Fatalf("受保护路由 %s 匿名请求应返回 401，实际 %d", spec.Pattern, code)
		}
	}
	if protected < 100 {
		t.Fatalf("受保护路由覆盖不足：%d 条", protected)
	}
	if public < 4 {
		t.Fatalf("公开路由覆盖不足：%d 条", public)
	}
}

func routeMethod(pattern string) string {
	method, _, _ := strings.Cut(pattern, " ")
	return method
}

func sampleRoutePath(pattern string) string {
	_, path, _ := strings.Cut(pattern, " ")
	return routeParamPattern.ReplaceAllString(path, "1")
}

// callRouteSafely 在依赖缺失导致 handler panic 时返回 0，避免公开路由的构造依赖影响鉴权断言。
func callRouteSafely(handler http.Handler, req *http.Request) (code int) {
	defer func() {
		if recover() != nil {
			code = 0
		}
	}()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code
}
