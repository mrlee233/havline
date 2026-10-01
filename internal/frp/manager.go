package frp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	bin        string
	configPath string
	logPath    string
	logger     *slog.Logger

	mu        sync.RWMutex
	cmd       *exec.Cmd
	orphan    *os.Process
	orphanPID int
	pid       int
	version   string
	started   time.Time
	reloaded  time.Time
	lastErr   string
}

func NewManager(bin, configPath, logPath string, logger *slog.Logger) *Manager {
	return &Manager{bin: bin, configPath: configPath, logPath: logPath, logger: logger.With("module", "FRP")}
}

func (m *Manager) Verify(ctx context.Context, configPath string) error {
	bin, err := m.BinaryPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, "verify", "-c", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("frpc 配置校验失败：%s", safeCommandOutput(output, err))
	}
	return nil
}

func (m *Manager) VerifyBinary(ctx context.Context) error {
	bin, err := m.BinaryPath()
	if err != nil {
		return err
	}
	output, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("frpc 二进制校验失败：%s", safeCommandOutput(output, err))
	}
	return nil
}

func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runningLocked() {
		return nil
	}
	bin, err := m.binaryPathLocked()
	if err != nil {
		m.lastErr = err.Error()
		return err
	}
	logFile, err := os.OpenFile(m.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, "-c", m.configPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		m.lastErr = err.Error()
		return fmt.Errorf("启动 frpc 失败：%w", err)
	}
	m.cmd = cmd
	m.pid = cmd.Process.Pid
	m.started = time.Now().UTC()
	m.reloaded = m.started
	m.lastErr = ""
	go m.wait(cmd, logFile)
	return nil
}

func (m *Manager) Restart(ctx context.Context) error {
	if err := m.Stop(ctx); err != nil {
		return err
	}
	return m.Start()
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	cmd := m.cmd
	orphan := m.orphan
	m.mu.Unlock()
	if cmd == nil && orphan == nil {
		return nil
	}
	process := orphan
	if cmd != nil {
		process = cmd.Process
	}
	if process == nil {
		return nil
	}
	if err := process.Kill(); err != nil {
		return fmt.Errorf("停止 frpc 失败：%w", err)
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		if !m.running() {
			m.mu.Lock()
			m.lastErr = ""
			m.orphan = nil
			m.orphanPID = 0
			m.mu.Unlock()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("等待 frpc 停止超时")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (m *Manager) Status() RuntimeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.statusLocked()
}

func (m *Manager) statusLocked() RuntimeStatus {
	status := RuntimeStatus{
		Status:    "stopped",
		PID:       m.pid,
		Version:   m.version,
		LastError: m.lastErr,
	}
	if m.runningLocked() {
		status.Status = "running"
		if status.PID == 0 {
			status.PID = m.orphanPID
		}
	} else if m.orphan != nil {
		status.PID = 0
	}
	if !m.started.IsZero() {
		status.LastStartedAt = m.started.Format(time.RFC3339)
	}
	if !m.reloaded.IsZero() {
		status.LastReloadedAt = m.reloaded.Format(time.RFC3339)
	}
	return status
}

func (m *Manager) RefreshVersion(ctx context.Context) {
	bin, err := m.BinaryPath()
	if err != nil {
		return
	}
	output, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return
	}
	m.mu.Lock()
	m.version = strings.TrimSpace(string(output))
	m.mu.Unlock()
}

func (m *Manager) wait(cmd *exec.Cmd, logFile *os.File) {
	err := cmd.Wait()
	logFile.Close()
	m.mu.Lock()
	if m.cmd != cmd {
		m.mu.Unlock()
		return
	}
	m.cmd = nil
	m.pid = 0
	if err != nil {
		// 退出原因在 frpc 自身日志的尾部（stderr 也写在那里），直接携带进应用日志，避免只看到 exit status 1
		m.lastErr = fmt.Sprintf("frpc 已退出：%v（详见 %s 末尾）", err, m.logPath)
		m.logger.Warn("frpc exited", "error", err.Error(), "log_tail", tailFile(m.logPath, 12))
	}
	m.mu.Unlock()
}

// tailFile 返回文件最后 maxLines 行文本；读不到时返回空串（不阻断退出处理）。
func tailFile(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, " | ")
}

func (m *Manager) running() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runningLocked()
}

func (m *Manager) runningLocked() bool {
	if m.cmd != nil && m.cmd.Process != nil {
		return true
	}
	return m.orphan != nil && processAlive(m.orphanPID)
}

func (m *Manager) Attach(pid int) bool {
	bin, err := m.BinaryPath()
	if err != nil || !processMatches(pid, bin, m.configPath) {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	m.mu.Lock()
	m.orphan = process
	m.orphanPID = pid
	m.pid = pid
	m.lastErr = ""
	m.mu.Unlock()
	return true
}

func processAlive(pid int) bool {
	alive, _ := probeProcess(pid)
	return alive
}

func processMatches(pid int, binary, configPath string) bool {
	alive, commandLine := probeProcess(pid)
	if !alive {
		return false
	}
	commandLine = strings.ToLower(filepath.ToSlash(commandLine))
	return strings.Contains(commandLine, strings.ToLower(filepath.ToSlash(binary))) &&
		strings.Contains(commandLine, strings.ToLower(filepath.ToSlash(configPath)))
}

func (m *Manager) BinaryPath() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.binaryPathLocked()
}

func (m *Manager) binaryPathLocked() (string, error) {
	if strings.ContainsAny(m.bin, `/\\`) {
		if _, err := os.Stat(m.bin); err != nil {
			return "", fmt.Errorf("未找到 frpc 二进制：%s", m.bin)
		}
		return m.bin, nil
	}
	path, err := exec.LookPath(m.bin)
	if err != nil {
		return "", fmt.Errorf("未找到 frpc 二进制，请设置 HAVLINE_FRPC_BIN")
	}
	return path, nil
}

func (m *Manager) SetBinary(bin string) {
	m.mu.Lock()
	m.bin = bin
	m.version = ""
	m.mu.Unlock()
}

func (m *Manager) LogPath() string {
	return filepath.Clean(m.logPath)
}

func safeCommandOutput(output []byte, err error) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return err.Error()
	}
	if len(text) > 500 {
		return text[:500]
	}
	return text
}
