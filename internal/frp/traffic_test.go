package frp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFetchLegacyRouteProxiesUsesBoundedConcurrency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		name := strings.TrimPrefix(r.URL.Path, "/api/proxies/")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"name":%q,"todayTrafficIn":1,"todayTrafficOut":2,"curConns":3}`, name)
	}))
	defer server.Close()

	host, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	client := newFRPSTrafficClient()
	names := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	started := time.Now()
	result := client.fetchLegacyRouteProxies(context.Background(), Server{
		ID: 1,
		Options: ServerOptions{
			DashboardAddr: host,
			DashboardPort: port,
		},
	}, "", names)
	elapsed := time.Since(started)

	if !result.OK || len(result.Proxies) != len(names) {
		t.Fatalf("逐条查询结果不完整: ok=%v proxies=%d", result.OK, len(result.Proxies))
	}
	if elapsed >= time.Second {
		t.Fatalf("并发查询耗时 %v，仍接近串行等待", elapsed)
	}
}
