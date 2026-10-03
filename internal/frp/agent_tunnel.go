package frp

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const agentRemoteAddr = "127.0.0.1:7700"

const (
	agentTunnelKeepalive  = 30 * time.Second
	agentTunnelBackoffMin = time.Second
	agentTunnelBackoffMax = 60 * time.Second
)

type agentTunnel struct {
	serverID  int64
	localPort int
	ssh       SSHInstaller

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	listener  net.Listener
	client    *ssh.Client
	state     string
	lastError string
	wg        sync.WaitGroup
}

func newAgentTunnel(parent context.Context, serverID int64, localPort int, installer SSHInstaller) *agentTunnel {
	return newAgentTunnelWithListener(parent, serverID, localPort, installer, nil)
}

func newAgentTunnelWithListener(parent context.Context, serverID int64, localPort int, installer SSHInstaller, listener net.Listener) *agentTunnel {
	ctx, cancel := context.WithCancel(parent)
	return &agentTunnel{
		serverID:  serverID,
		localPort: localPort,
		ssh:       installer,
		ctx:       ctx,
		cancel:    cancel,
		listener:  listener,
		state:     "connecting",
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
	t.mu.Lock()
	t.state = "connected"
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
		_, err := t.ensureClient()
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
		go func() {
			defer t.wg.Done()
			t.handle(conn)
		}()
	}
}

func (t *agentTunnel) handle(local net.Conn) {
	defer local.Close()
	client, err := t.ensureClient()
	if err != nil {
		t.setState("reconnecting", err)
		return
	}
	remote, err := client.Dial("tcp", agentRemoteAddr)
	if err != nil {
		t.invalidateClient(client)
		t.setState("reconnecting", err)
		return
	}
	defer remote.Close()
	t.setState("connected", nil)
	proxyTunnel(local, remote)
}

func (t *agentTunnel) ensureClient() (*ssh.Client, error) {
	t.mu.Lock()
	if t.client != nil {
		client := t.client
		t.mu.Unlock()
		if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err == nil {
			return client, nil
		}
		t.invalidateClient(client)
	} else {
		t.mu.Unlock()
	}
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
	t.wg.Wait()
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
