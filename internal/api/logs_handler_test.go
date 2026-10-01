package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/havline/havline/internal/auth"
	"github.com/havline/havline/internal/backup"
	"github.com/havline/havline/internal/certificate"
	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/db"
	"github.com/havline/havline/internal/discovery"
	"github.com/havline/havline/internal/proxy"
	"github.com/havline/havline/internal/service"
	"github.com/havline/havline/internal/settings"
)

func TestLogsAccessRoute(t *testing.T) {
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
	if err := authSvc.Setup(context.Background(), "admin", "password123"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	sessionID, err := authSvc.Login(context.Background(), "admin", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	handler := NewRouter(Deps{
		Config:    config.Config{DataDir: dir, NginxPIDFile: filepath.Join(dir, "nginx.pid")},
		Auth:      authSvc,
		Proxy:     service.NewProxyService(config.Config{DataDir: dir, NginxPIDFile: filepath.Join(dir, "nginx.pid")}, conn, proxy.NewStore(conn), certificate.NewStore(conn), settings.NewStore(conn), nil),
		Backup:    backup.New(dir),
		Discovery: discovery.New(),
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/logs/access?limit=100", nil)
	req.AddCookie(&http.Cookie{Name: "havline_session", Value: sessionID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var body []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
}
