package frp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const agentRemoteAddr = "127.0.0.1:7700"

const (
	agentTunnelKeepalive        = 30 * time.Second
	agentTunnelKeepaliveTimeout = 5 * time.Second
	agentTunnelBackoffMin       = time.Second
	agentTunnelBackoffMax       = 60 * time.Second
)

type agentTunnel struct {
	serverID   int64
	localPort  int
	remoteAddr string
	ssh        SSHInstaller

	ctx    context.Context
	cancel context.CancelFunc

	connectMu sync.Mutex
	mu        sync.Mutex
	listener  net.Listener
	client    *ssh.Client
	state     string
	lastError string
	wg        sync.WaitGroup

	connsMu sync.Mutex
	conns   map[net.Conn]struct{}
}

func newAgentTunnel(parent context.Context, serverID int64, localPort int, installer SSHInstaller) *agentTunnel {
	return newAgentTunnelWithListener(parent, serverID, localPort, installer, nil)
}

func newAgentTunnelWithListener(parent context.Context, serverID int64, localPort int, installer SSHInstaller, listener net.Listener) *agentTunnel {
	ctx, cancel := context.WithCancel(parent)
	return &agentTunnel{
		serverID:   serverID,
		localPort:  localPort,
		remoteAddr: agentRemoteAddr,
		ssh:        installer,
		ctx:        ctx,
		cancel:     cancel,
		listener:   listener,
		state:      "connecting",
		conns:      map[net.Conn]struct{}{},
	}
}

func (t *agentTunnel) Start() error {
	if t.listener == nil {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", t.localPort))
		if err != nil {
			t.setState("failed", err)
			return fmt.Errorf("监听隧道本地端口失败: %w", err)
		}
		t.listener = listener
	}
	if t.localPort == 0 {
		if addr, ok := t.listener.Addr().(*net.TCPAddr); ok {
			t.localPort = addr.Port
		}
	}
	t.mu.Lock()
	t.state = "connecting"
	t.lastError = ""
	t.mu.Unlock()
	t.wg.Add(2)
	go t.acceptLoop()
	go t.monitorLoop()
	return nil
}

// monitorLoop 主动保活并在断开后按指数退避重建 SSH 连接，避免只在有流量时才尝试重连。
func (t *agentTunnel) monitorLoop() {
	defer t.wg.Done()
	backoff := agentTunnelBackoffMin
	for {
		err := t.check()
		wait := agentTunnelKeepalive
		if err != nil {
			t.setState("reconnecting", err)
			backoff = nextTunnelBackoff(backoff)
			wait = backoff
		} else {
			t.setState("connected", nil)
			backoff = agentTunnelBackoffMin
		}
		if !t.sleep(wait) {
			return
		}
	}
}

// check 同时验证 SSH 连接和远端 Agent 端口，避免只连上 SSH 就误判隧道可用。
func (t *agentTunnel) check() error {
	client, err := t.ensureClient()
	if err != nil {
		return err
	}
	if err := t.keepalive(client); err != nil {
		t.invalidateClient(client)
		return fmt.Errorf("SSH keepalive 失败: %w", err)
	}
	remote, err := client.Dial("tcp", t.remoteAddr)
	if err != nil {
		return fmt.Errorf("连接远端 Agent 端口失败: %w", err)
	}
	_ = remote.Close()
	return nil
}

func (t *agentTunnel) sleep(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-t.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextTunnelBackoff(current time.Duration) time.Duration {
	if current < agentTunnelBackoffMin {
		return agentTunnelBackoffMin
	}
	next := current * 2
	if next > agentTunnelBackoffMax {
		return agentTunnelBackoffMax
	}
	return next
}

func (t *agentTunnel) acceptLoop() {
	defer t.wg.Done()
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			if t.ctx.Err() != nil {
				return
			}
			t.setState("failed", err)
			return
		}
		t.wg.Add(1)
		if !t.trackConn(conn) {
			t.wg.Done()
			return
		}
		go func() {
			defer t.wg.Done()
			defer t.untrackConn(conn)
			t.handle(conn)
		}()
	}
}

func (t *agentTunnel) handle(local net.Conn) {
	defer local.Close()
	client, err := t.ensureClient()
	if err != nil {
		t.setState("reconnecting", err)
		writeTunnelError(local, err)
		return
	}
	remote, err := client.Dial("tcp", t.remoteAddr)
	if err != nil {
		remote, err = t.retryRemoteDial(client, err)
		if err != nil {
			t.setState("reconnecting", err)
			writeTunnelError(local, err)
			return
		}
	}
	defer remote.Close()
	t.setState("connected", nil)
	proxyTunnel(local, remote)
}

// retryRemoteDial 仅在 SSH 连接本身失效时重连，远端 Agent 端口暂时不可用不会关闭共享连接。
func (t *agentTunnel) retryRemoteDial(client *ssh.Client, firstErr error) (net.Conn, error) {
	if err := t.keepalive(client); err == nil {
		return nil, fmt.Errorf("连接远端 Agent 端口失败: %w", firstErr)
	}
	t.invalidateClient(client)
	reconnected, err := t.ensureClient()
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失效，重连失败: %w", err)
	}
	remote, err := reconnected.Dial("tcp", t.remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("SSH 已重连，但远端 Agent 端口仍不可用: %w", err)
	}
	return remote, nil
}

func (t *agentTunnel) ensureClient() (*ssh.Client, error) {
	// 单飞：监控循环和请求处理可能同时触发建连，串行化后只允许一条 SSH 握手。
	// 正常请求只复用连接，不在热路径做 keepalive，避免多个接口互相排队。
	t.connectMu.Lock()
	defer t.connectMu.Unlock()

	t.mu.Lock()
	if t.client != nil {
		client := t.client
		t.mu.Unlock()
		return client, nil
	}
	t.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.ctx, 15*time.Second)
	defer cancel()
	client, err := t.ssh.client(ctx)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	if t.client != nil {
		existing := t.client
		t.mu.Unlock()
		_ = client.Close()
		return existing, nil
	}
	t.client = client
	t.mu.Unlock()
	return client, nil
}

func (t *agentTunnel) keepalive(client *ssh.Client) error {
	done := make(chan error, 1)
	go func() {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(agentTunnelKeepaliveTimeout):
		return fmt.Errorf("SSH keepalive 超时")
	case <-t.ctx.Done():
		return t.ctx.Err()
	}
}

// writeTunnelError 让本地 HTTP 客户端立即得到明确错误，而不是一直等到 context deadline。
func writeTunnelError(conn net.Conn, err error) {
	body, _ := json.Marshal(map[string]string{"error": "SSH 隧道不可用: " + err.Error()})
	_, _ = fmt.Fprintf(conn,
		"HTTP/1.1 502 Bad Gateway\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		len(body), body)
}

func (t *agentTunnel) invalidateClient(client *ssh.Client) {
	t.mu.Lock()
	if t.client == client {
		t.client = nil
	}
	t.mu.Unlock()
	_ = client.Close()
}

func (t *agentTunnel) State() (string, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state, t.lastError
}

func (t *agentTunnel) setState(state string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = state
	if err == nil {
		t.lastError = ""
		return
	}
	t.lastError = err.Error()
}

func (t *agentTunnel) Stop() {
	t.cancel()
	t.mu.Lock()
	if t.listener != nil {
		_ = t.listener.Close()
	}
	if t.client != nil {
		_ = t.client.Close()
	}
	t.mu.Unlock()
	t.closeConns()
	t.wg.Wait()
}

func (t *agentTunnel) trackConn(conn net.Conn) bool {
	t.connsMu.Lock()
	defer t.connsMu.Unlock()
	if t.ctx.Err() != nil {
		_ = conn.Close()
		return false
	}
	t.conns[conn] = struct{}{}
	return true
}

func (t *agentTunnel) untrackConn(conn net.Conn) {
	t.connsMu.Lock()
	delete(t.conns, conn)
	t.connsMu.Unlock()
}

func (t *agentTunnel) closeConns() {
	t.connsMu.Lock()
	conns := make([]net.Conn, 0, len(t.conns))
	for conn := range t.conns {
		conns = append(conns, conn)
	}
	t.connsMu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func proxyTunnel(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(a, b)
		_ = a.Close()
		_ = b.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(b, a)
		_ = a.Close()
		_ = b.Close()
	}()
	wg.Wait()
}
