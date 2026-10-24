package traffic

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/havline/havline/internal/db"
	"github.com/havline/havline/internal/proxy"
)

// TestForgetEndpointsClearsMemoryAndStore：删除规则时要同时清掉内存累计与落库累计，
// 否则同名规则重建后会直接继承上一轮的流量读数。
func TestForgetEndpointsClearsMemoryAndStore(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "havline.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()
	if err := db.Migrate(ctx, conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	c := NewCollector(conn, "", nil)
	c.ingestLine("2026-09-17T08:30:00+08:00 a.example.com 18080 GET / 200 0.010 1.2.3.4 192.168.8.3:80 100 200", true)
	c.ingestLine("2026-09-17T08:30:01+08:00 b.example.com 18080 GET / 200 0.010 1.2.3.4 192.168.8.3:80 100 200", true)
	if err := c.persist(ctx); err != nil {
		t.Fatalf("persist: %v", err)
	}

	endpoints := []proxy.Endpoint{{Hostname: "a.example.com", Port: 18080}}
	if err := c.ForgetEndpoints(ctx, endpoints); err != nil {
		t.Fatalf("forget: %v", err)
	}

	count := func(host string) int64 {
		var n int64
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(1) FROM proxy_traffic WHERE host = ?`, host).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", host, err)
		}
		return n
	}
	if got := count("a.example.com:18080"); got != 0 {
		t.Fatalf("被删规则对应的流量行应清空，实际还有 %d 行", got)
	}
	if got := count("b.example.com:18080"); got != 1 {
		t.Fatalf("其他规则的流量行不应受影响，实际 %d 行", got)
	}

	// 内存态也要一起清掉：被删端点的累计读数应归零
	up, down, _, _, conns := c.aggregateEndpoints(endpoints, time.Now())
	if up != 0 || down != 0 || conns != 0 {
		t.Fatalf("内存累计应归零：up=%d down=%d conns=%d", up, down, conns)
	}

	// 空入参是空操作，不能报错（删除路径会无差别调用）
	if err := c.ForgetEndpoints(ctx, nil); err != nil {
		t.Fatalf("空端点列表不应报错：%v", err)
	}
}
