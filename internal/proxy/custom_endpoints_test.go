package proxy

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/havline/havline/internal/db"
)

func newTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(1)", name))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	conn.SetMaxOpenConns(1)
	if err := db.Migrate(ctx, conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewStore(conn), conn
}

func createTestRule(t *testing.T, store *Store, name, host string, port int) Rule {
	t.Helper()
	rule, err := store.Create(context.Background(), CreateInput{
		Upstream:   "http://192.168.1.10:8080",
		ListenPort: port,
		ListenIPv4: true,
		Hosts:      []string{host},
		Enabled:    true,
		Name:       name,
	})
	if err != nil {
		t.Fatalf("create rule %s: %v", name, err)
	}
	return rule
}

func TestEnsureEndpointsAvailableNamesOwningRule(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	owner := createTestRule(t, store, "官网", "a.com", 8011)

	err := store.EnsureEndpointsAvailable(ctx, []Endpoint{{Hostname: "a.com", Port: 8011}}, 0)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "规则「官网」") {
		t.Fatalf("expected owning rule name in error, got %q", err.Error())
	}

	// 编辑自身不应被判为冲突
	if err := store.EnsureEndpointsAvailable(ctx, []Endpoint{{Hostname: "a.com", Port: 8011}}, owner.ID); err != nil {
		t.Fatalf("exclude self should pass, got %v", err)
	}
	// 同域名不同端口是合法虚拟主机
	if err := store.EnsureEndpointsAvailable(ctx, []Endpoint{{Hostname: "a.com", Port: 8012}}, 0); err != nil {
		t.Fatalf("different port should pass, got %v", err)
	}
}

func TestEnsureEndpointsAvailableSeesCustomEndpoints(t *testing.T) {
	store, conn := newTestStore(t)
	ctx := context.Background()
	custom := createTestRule(t, store, "手动规则", "b.com", 8011)
	if err := store.ReplaceCustomEndpoints(ctx, custom.ID, []Endpoint{{Hostname: "c.com", Port: 8013}}); err != nil {
		t.Fatalf("replace custom endpoints: %v", err)
	}

	err := store.EnsureEndpointsAvailable(ctx, []Endpoint{{Hostname: "c.com", Port: 8013}}, 0)
	if err == nil {
		t.Fatal("expected conflict with custom endpoints")
	}
	if !strings.Contains(err.Error(), "手动 Nginx 配置") {
		t.Fatalf("expected custom-rule hint in error, got %q", err.Error())
	}

	// 删除规则后占用记录随外键级联清除
	if _, err := conn.ExecContext(ctx, `DELETE FROM proxy_rules WHERE id = ?`, custom.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	if err := store.EnsureEndpointsAvailable(ctx, []Endpoint{{Hostname: "c.com", Port: 8013}}, 0); err != nil {
		t.Fatalf("cascade should free the binding, got %v", err)
	}
}

func TestClearCustomEndpoints(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, store, "手动规则", "b.com", 8011)
	if err := store.ReplaceCustomEndpoints(ctx, rule.ID, []Endpoint{{Hostname: "c.com", Port: 8013}}); err != nil {
		t.Fatalf("replace custom endpoints: %v", err)
	}
	if err := store.ClearCustomEndpoints(ctx, rule.ID); err != nil {
		t.Fatalf("clear custom endpoints: %v", err)
	}
	if err := store.EnsureEndpointsAvailable(ctx, []Endpoint{{Hostname: "c.com", Port: 8013}}, 0); err != nil {
		t.Fatalf("cleared binding should be free, got %v", err)
	}
}
