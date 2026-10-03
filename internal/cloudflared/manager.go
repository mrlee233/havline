package cloudflared

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/havline/havline/internal/config"
)

var metricsPortPattern = regexp.MustCompile(`(?i)starting metrics server on 127\.0\.0\.1:(\d+)/metrics`)

type processState struct {
	cmd         *exec.Cmd
	status      string
	lastError   string
	metricsPort int
	metrics     RuntimeStatus
	logMu       sync.Mutex
}

// Manager 托管 cloudflared 子进程、日志与 metrics。
type Manager struct {
	cfg       config.Config
	logger    *slog.Logger
	mu        sync.Mutex
	processes map[int64]*processState
}

func NewManager(cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{cfg: cfg, logger: logger, processes: map[int64]*processState{}}
}

func buildRunArgs(tunnel Tunnel, configPath, token string) []string {
	args := []string{"tunnel"}
	if tunnel.Mode == ModeTokenRemote {
		args = append(args, networkArgs(tunnel.Network)...)
		return append(args, "run", "--token", token)
	}
	args = append(args, "--config", configPath)
	args = append(args, networkArgs(tunnel.Network)...)
	return append(args, "run")
}

func networkArgs(network NetworkSettings) []string {
	args := []string{}
	if network.HAConnections > 0 {
		args = append(args, "--ha-connections", strconv.Itoa(network.HAConnections))
	}
	if pool := strings.TrimSpace(network.OriginCAPool); pool != "" {
		args = append(args, "--origin-ca-pool", pool)
	}
	if network.NoTLSVerify {
		args = append(args, "--no-tls-verify")
	}
	if header := strings.TrimSpace(network.HTTPHostHeader); header != "" {
		args = append(args, "--http-host-header", header)
	}
	return args
}

func buildEnv(network NetworkSettings) []string {
	env := os.Environ()
	if protocol := strings.TrimSpace(network.TransportProtocol); protocol != "" {
		env = append(env, "TUNNEL_TRANSPORT_PROTOCOL="+protocol)
	}
	if edge := strings.TrimSpace(network.EdgeIPVersion); edge != "" {
		env = append(env, "TUNNEL_EDGE_IP_VERSION="+edge)
	}
	if network.ProxyMode == "disabled" {
		env = removeEnv(env, []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"})
	}
	return env
}

func removeEnv(env []string, names []string) []string {
	out := make([]string, 0, len(env))
	for _, item := range env {
		remove := false
		for _, name := range names {
			if strings.HasPrefix(item, name+"=") {
				remove = true
				break
			}
		}
		if !remove {
			out = append(out, item)
		}
	}
	return out
}

// Start 启动一条隧道的 cloudflared 子进程。
func (m *Manager) Start(tunnel Tunnel, binaryPath, configPath, token string) error {
	m.mu.Lock()
	if _, exists := m.processes[tunnel.ID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("隧道已在运行中")
	}
	state := &processState{status: "starting"}
	m.processes[tunnel.ID] = state
	m.mu.Unlock()

	logFile, err := os.OpenFile(tunnel.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		m.remove(tunnel.ID)
		return err
	}
	cmd := exec.Command(binaryPath, buildRunArgs(tunnel, configPath, token)...)
	cmd.Env = buildEnv(tunnel.Network)
	cmd.SysProcAttr = hideWindowAttr()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = logFile.Close()
		m.remove(tunnel.ID)
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = logFile.Close()
		m.remove(tunnel.ID)
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		m.remove(tunnel.ID)
		return fmt.Errorf("启动 cloudflared 失败: %w", err)
	}
	state.cmd = cmd
	go m.pipeLogs(tunnel.ID, stdout, logFile)
	go m.pipeLogs(tunnel.ID, stderr, logFile)
	go m.wait(tunnel.ID, cmd, logFile)
	return nil
}

func (m *Manager) pipeLogs(id int64, reader io.Reader, logFile *os.File) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		m.writeLog(id, logFile, line)
		m.detectMetrics(id, line)
	}
}

func (m *Manager) writeLog(id int64, logFile *os.File, line string) {
	m.mu.Lock()
	state := m.processes[id]
	m.mu.Unlock()
	if state == nil {
		return
	}
	state.logMu.Lock()
	defer state.logMu.Unlock()
	_, _ = fmt.Fprintf(logFile, "%s %s\n", time.Now().Format(time.RFC3339), line)
}

func (m *Manager) detectMetrics(id int64, line string) {
	match := metricsPortPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return
	}
	port, err := strconv.Atoi(match[1])
	if err != nil || port <= 0 {
		return
	}
	m.mu.Lock()
	state := m.processes[id]
	if state == nil || state.metricsPort == port {
		m.mu.Unlock()
		return
	}
	state.metricsPort = port
	m.mu.Unlock()
	go m.scrapeMetrics(id, port)
}

func (m *Manager) scrapeMetrics(id int64, port int) {
	client := &http.Client{Timeout: 5 * time.Second}
	metricsURL := fmt.Sprintf("http://127.0.0.1:%d/metrics", port)
	readyURL := fmt.Sprintf("http://127.0.0.1:%d/ready", port)
	lastIn, lastOut := 0.0, 0.0
	for {
		time.Sleep(2 * time.Second)
		if !m.running(id) {
			return
		}
		text, err := fetchText(client, metricsURL)
		if err != nil {
			continue
		}
		snapshot := parseMetrics(text)
		status := RuntimeStatus{
			Running: true, Status: "connecting", MetricsPort: port,
			Connections: snapshot.connections, LatencyMS: snapshot.latencyMS,
			Transport: snapshot.transport,
		}
		if snapshot.hasQUIC {
			status.BandwidthIn = math.Max(0, snapshot.bytesIn-lastIn) / 2
			status.BandwidthOut = math.Max(0, snapshot.bytesOut-lastOut) / 2
			lastIn, lastOut = snapshot.bytesIn, snapshot.bytesOut
		} else {
			status.BandwidthNotice = "HTTP/2 官方 metrics 不含字节指标"
		}
		if snapshot.connections > 0 {
			status.Status = "connected"
		} else if isReady(client, readyURL) {
			status.Status = "reconnecting"
		}
		m.mu.Lock()
		if state := m.processes[id]; state != nil {
			state.metrics = status
			state.status = status.Status
		}
		m.mu.Unlock()
	}
}

func (m *Manager) running(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.processes[id] != nil
}

func (m *Manager) wait(id int64, cmd *exec.Cmd, logFile *os.File) {
	err := cmd.Wait()
	_ = logFile.Close()
	m.mu.Lock()
	state := m.processes[id]
	if state != nil {
		if err != nil {
			state.status = "failed"
			state.lastError = err.Error()
		} else {
			state.status = "stopped"
		}
	}
	m.mu.Unlock()
}

func (m *Manager) remove(id int64) {
	m.mu.Lock()
	delete(m.processes, id)
	m.mu.Unlock()
}

// Stop 停止隧道子进程。
func (m *Manager) Stop(id int64) error {
	m.mu.Lock()
	state := m.processes[id]
	delete(m.processes, id)
	m.mu.Unlock()
	if state == nil || state.cmd == nil || state.cmd.Process == nil {
		return nil
	}
	_ = state.cmd.Process.Kill()
	return nil
}

// Status 返回运行状态与最近一次 metrics 快照。
func (m *Manager) Status(id int64) RuntimeStatus {
	m.mu.Lock()
	state := m.processes[id]
	m.mu.Unlock()
	if state == nil {
		return RuntimeStatus{Status: "stopped"}
	}
	out := state.metrics
	out.Status = state.status
	out.LastError = state.lastError
	out.MetricsPort = state.metricsPort
	out.Running = state.status != "stopped" && state.status != "failed"
	return out
}

// Logs 读取日志文件末尾若干行。
func (m *Manager) Logs(logPath string, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n"), nil
}

type metricsSnapshot struct {
	connections int
	latencyMS   float64
	bytesIn     float64
	bytesOut    float64
	transport   string
	hasQUIC     bool
}

func parseMetrics(text string) metricsSnapshot {
	snapshot := metricsSnapshot{transport: "http2"}
	var rttSum, rttCount float64
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "cloudflared_tunnel_ha_connections"):
			snapshot.connections = int(parseMetricValue(line))
		case strings.HasPrefix(line, "quic_client_receive_bytes"):
			snapshot.bytesIn += parseMetricValue(line)
			snapshot.hasQUIC = true
		case strings.HasPrefix(line, "quic_client_sent_bytes"):
			snapshot.bytesOut += parseMetricValue(line)
			snapshot.hasQUIC = true
		case strings.HasPrefix(line, "quic_client_smoothed_rtt"):
			rttSum += parseMetricValue(line)
			rttCount++
			snapshot.hasQUIC = true
		}
	}
	if snapshot.hasQUIC {
		snapshot.transport = "quic"
		if rttCount > 0 {
			snapshot.latencyMS = rttSum / rttCount
		}
	}
	return snapshot
}

func parseMetricValue(line string) float64 {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return 0
	}
	value, _ := strconv.ParseFloat(fields[len(fields)-1], 64)
	return value
}

func fetchText(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isReady(client *http.Client, url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}
