package frp

import (
	"context"
	"fmt"
	"net"
	"testing"
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
	if state != "connected" {
		t.Fatalf("state=%q, want connected", state)
	}
	tunnel.Stop()
	if listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err != nil {
		t.Fatalf("隧道停止后端口未释放: %v", err)
	} else {
		_ = listener.Close()
	}
}
