package frp

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestAgentTunnelBindsLoopbackAndReleasesPort(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()

	tunnel := newAgentTunnel(context.Background(), 1, port, SSHInstaller{})
	if err := tunnel.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
		t.Fatal("隧道启动后本地端口应已被占用")
	}
	state, _ := tunnel.State()
	if state != "connecting" && state != "connected" && state != "reconnecting" {
		t.Fatalf("state=%q, want connecting、connected 或 reconnecting", state)
	}
	tunnel.Stop()
	if listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err != nil {
		t.Fatalf("隧道停止后端口未释放: %v", err)
	} else {
		_ = listener.Close()
	}
}

func TestNextTunnelBackoff(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want time.Duration
	}{
		{0, agentTunnelBackoffMin},
		{time.Second, 2 * time.Second},
		{40 * time.Second, agentTunnelBackoffMax},
		{agentTunnelBackoffMax, agentTunnelBackoffMax},
	}
	for _, tc := range cases {
		if got := nextTunnelBackoff(tc.in); got != tc.want {
			t.Fatalf("nextTunnelBackoff(%v) = %v, 期望 %v", tc.in, got, tc.want)
		}
	}
}

func TestAgentTunnelStopClosesActiveConnections(t *testing.T) {
	tunnel := newAgentTunnel(context.Background(), 1, 0, SSHInstaller{})
	server, client := net.Pipe()
	defer client.Close()
	if !tunnel.trackConn(server) {
		t.Fatal("隧道运行时应能登记活动连接")
	}
	tunnel.Stop()
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("隧道停止后活动连接应被关闭")
	}
}
