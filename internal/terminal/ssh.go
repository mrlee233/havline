package terminal

import (
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

// sshTerminal 把 x/crypto/ssh 的交互式会话包装成 Terminal。
// 申请了 PTY 之后，远端 shell 的 stdout/stderr 会合并到同一条输出流，无需分别读取。
type sshTerminal struct {
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader

	closeOnce sync.Once
	closeErr  error
}

// NewSSHTerminal 在已认证的 SSH 连接上申请 PTY 并启动登录 shell。
func NewSSHTerminal(client *ssh.Client, cols, rows int) (Terminal, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("申请 PTY 失败: %w", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("打开 SSH 输入流失败: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("打开 SSH 输出流失败: %w", err)
	}
	if err := session.Shell(); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("启动远端 shell 失败: %w", err)
	}
	return &sshTerminal{client: client, session: session, stdin: stdin, stdout: stdout}, nil
}

// Read 读取远端输出。
func (t *sshTerminal) Read(p []byte) (int, error) { return t.stdout.Read(p) }

// Write 把按键写入远端 PTY。
func (t *sshTerminal) Write(p []byte) (int, error) { return t.stdin.Write(p) }

// Resize 通知远端窗口尺寸变化。
func (t *sshTerminal) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return t.session.WindowChange(rows, cols)
}

// Close 关闭 SSH 会话与底层连接；重复调用安全。
func (t *sshTerminal) Close() error {
	t.closeOnce.Do(func() {
		_ = t.session.Close()
		t.closeErr = t.client.Close()
	})
	return t.closeErr
}
