package frp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type directTCPIPPayload struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}

func TestAgentTunnelReturnsBadGatewayWhenRemoteAgentIsDown(t *testing.T) {
	sshAddr := startFakeSSHServer(t)
	tunnel := startTestAgentTunnel(t, sshAddr, "127.0.0.1:1")

	response, err := (&http.Client{Timeout: 5 * time.Second}).Get(
		fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", tunnel.localPort),
	)
	if err != nil {
		t.Fatalf("应返回 HTTP 502，而不是 EOF 或超时: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("状态码 = %d，期望 502", response.StatusCode)
	}
	body, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(body), "SSH 隧道不可用") {
		t.Fatalf("响应应包含隧道失败原因，实际为 %q", string(body))
	}
}

func TestAgentTunnelForwardsHTTPToRemoteAgent(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer remote.Close()

	sshAddr := startFakeSSHServer(t)
	tunnel := startTestAgentTunnel(t, sshAddr, strings.TrimPrefix(remote.URL, "http://"))
	response, err := (&http.Client{Timeout: 5 * time.Second}).Get(
		fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", tunnel.localPort),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || string(body) != `{"ok":true}` {
		t.Fatalf("隧道转发失败: status=%d body=%q", response.StatusCode, string(body))
	}
}

func TestAgentTunnelSerializesConcurrentSSHConnections(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer remote.Close()

	sshAddr, connectionCount := startFakeSSHServerWithCounter(t, 300*time.Millisecond)
	tunnel := startTestAgentTunnel(t, sshAddr, strings.TrimPrefix(remote.URL, "http://"))
	client := &http.Client{Timeout: 5 * time.Second}
	var wait sync.WaitGroup
	for index := 0; index < 4; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", tunnel.localPort))
			if err == nil {
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
			}
		}()
	}
	wait.Wait()
	if got := connectionCount.Load(); got != 1 {
		t.Fatalf("并发请求建立了 %d 条 SSH 连接，期望单飞复用 1 条", got)
	}
}

func TestAgentTunnelDoesNotBlockRequestsOnSlowKeepalive(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer remote.Close()

	sshAddr, _ := startFakeSSHServerWithDelays(t, 0, 3*time.Second)
	tunnel := startTestAgentTunnel(t, sshAddr, strings.TrimPrefix(remote.URL, "http://"))
	started := time.Now()
	response, err := (&http.Client{Timeout: 5 * time.Second}).Get(
		fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", tunnel.localPort),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("正常请求被慢 keepalive 阻塞了 %v", elapsed)
	}
}

func startTestAgentTunnel(t *testing.T, sshAddr, remoteAddr string) *agentTunnel {
	t.Helper()
	host, portText, err := net.SplitHostPort(sshAddr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	tunnel := newAgentTunnel(context.Background(), 1, 0, SSHInstaller{
		Host: host, Port: port, User: "test", Password: "test",
		Timeout: 2 * time.Second, KnownHostsPath: filepath.Join(t.TempDir(), "known_hosts"),
	})
	tunnel.remoteAddr = remoteAddr
	if err := tunnel.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tunnel.Stop)
	return tunnel
}

func startFakeSSHServer(t *testing.T) string {
	t.Helper()
	addr, _ := startFakeSSHServerWithCounter(t, 0)
	return addr
}

func startFakeSSHServerWithCounter(t *testing.T, handshakeDelay time.Duration) (string, *atomic.Int32) {
	t.Helper()
	return startFakeSSHServerWithDelays(t, handshakeDelay, 0)
}

func startFakeSSHServerWithDelays(t *testing.T, handshakeDelay, keepaliveDelay time.Duration) (string, *atomic.Int32) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	var connectionCount atomic.Int32
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveFakeSSHConn(conn, config, &connectionCount, handshakeDelay, keepaliveDelay)
		}
	}()
	return listener.Addr().String(), &connectionCount
}

func serveFakeSSHConn(
	conn net.Conn,
	config *ssh.ServerConfig,
	connectionCount *atomic.Int32,
	handshakeDelay, keepaliveDelay time.Duration,
) {
	if handshakeDelay > 0 {
		time.Sleep(handshakeDelay)
	}
	connectionCount.Add(1)
	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer serverConn.Close()
	go func() {
		for request := range requests {
			if request.Type == "keepalive@openssh.com" && keepaliveDelay > 0 {
				time.Sleep(keepaliveDelay)
			}
			if request.WantReply {
				_ = request.Reply(true, nil)
			}
		}
	}()
	for newChannel := range channels {
		if newChannel.ChannelType() != "direct-tcpip" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		var payload directTCPIPPayload
		if err := ssh.Unmarshal(newChannel.ExtraData(), &payload); err != nil {
			_ = newChannel.Reject(ssh.ConnectionFailed, "invalid direct-tcpip payload")
			continue
		}
		target := net.JoinHostPort(payload.DestAddr, strconv.Itoa(int(payload.DestPort)))
		remote, err := net.DialTimeout("tcp", target, 2*time.Second)
		if err != nil {
			_ = newChannel.Reject(ssh.ConnectionFailed, err.Error())
			continue
		}
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			_ = remote.Close()
			continue
		}
		go ssh.DiscardRequests(channelRequests)
		go func() {
			_, _ = io.Copy(remote, channel)
			_ = remote.Close()
		}()
		go func() {
			_, _ = io.Copy(channel, remote)
			_ = channel.Close()
		}()
	}
}
