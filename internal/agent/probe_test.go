package agent

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// 用临时参数跑探测，避免单测真的等满默认的 1.5 秒间隔
func stubProbeRetry(t *testing.T, attempts int) {
	t.Helper()
	oldAttempts, oldInterval := routeProbeAttempts, routeProbeInterval
	routeProbeAttempts, routeProbeInterval = attempts, time.Millisecond
	t.Cleanup(func() { routeProbeAttempts, routeProbeInterval = oldAttempts, oldInterval })
}

func probePort(t *testing.T, server *httptest.Server) int {
	t.Helper()
	addr, ok := server.Listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("无法取到测试服务端口：%v", server.Listener.Addr())
	}
	return addr.Port
}

// TestProbeVhostReadyWaitsForRegistration：前两次 404（隧道尚未注册）后应继续重试并拿到 200，
// 这正是「部署后立刻探测误报失败、刷新又正常」的场景。
func TestProbeVhostReadyWaitsForRegistration(t *testing.T) {
	stubProbeRetry(t, 4)
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) <= 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	code, _ := (&Server{}).probeVhostReady(probePort(t, server), "a.example.com")
	if code != http.StatusOK {
		t.Fatalf("期望重试后拿到 200，实际 %d", code)
	}
	if hits != 3 {
		t.Fatalf("期望探测 3 次后成功，实际 %d 次", hits)
	}
}

// TestProbeVhostReadyStopsOnSuccess：非 404 立即返回，不做多余重试。
func TestProbeVhostReadyStopsOnSuccess(t *testing.T) {
	stubProbeRetry(t, 4)
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	code, _ := (&Server{}).probeVhostReady(probePort(t, server), "a.example.com")
	if code != http.StatusBadGateway {
		t.Fatalf("期望原样返回 502，实际 %d", code)
	}
	if hits != 1 {
		t.Fatalf("502 不应重试，实际探测 %d 次", hits)
	}
}

// TestProbeVhostReadyGivesUpAfterAttempts：一直 404 时用尽重试次数并返回最后一次结果。
func TestProbeVhostReadyGivesUpAfterAttempts(t *testing.T) {
	stubProbeRetry(t, 3)
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	code, _ := (&Server{}).probeVhostReady(probePort(t, server), "a.example.com")
	if code != http.StatusNotFound {
		t.Fatalf("期望最终仍为 404，实际 %d", code)
	}
	if hits != 3 {
		t.Fatalf("期望探测 3 次后放弃，实际 %d 次", hits)
	}
}
