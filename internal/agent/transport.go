package agent

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/havline/havline/internal/fsutil"
	"github.com/havline/havline/internal/nginxtmpl"
)

const (
	httpsProxyConfigName = "havlineagent-https.conf"
	httpsProxyCertPath   = "/etc/havline-agent/tls/agent.crt"
	httpsProxyKeyPath    = "/etc/havline-agent/tls/agent.key"
)

type httpsProxyRequest struct {
	CertPEM string `json:"cert_pem"`
	KeyPEM  string `json:"key_pem"`
	Port    int    `json:"port"`
}

type httpsProxyState struct {
	Port int `json:"port"`
}

func (s *Server) putHTTPSProxy(w http.ResponseWriter, r *http.Request) {
	var req httpsProxyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Port < 0 || req.Port > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "端口需在 0-65535 之间，0 表示自动选择"})
		return
	}
	if _, err := tls.X509KeyPair([]byte(req.CertPEM), []byte(req.KeyPEM)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "证书或私钥格式无效"})
		return
	}
	if probe := s.probeNginx(); probe.Missing {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "VPS 未安装 Nginx，无法启用 HTTPS 直连"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	port, err := s.configureHTTPSProxy(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "port": port, "self_check": "ok"})
}

func (s *Server) deleteHTTPSProxy(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.removeHTTPSProxy(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// getHTTPSProxy 返回当前 HTTPS 反代状态与证书到期时间，供中央端同步轮换计划。
func (s *Server) getHTTPSProxy(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, enabled := readHTTPSProxyState(s.cfg.DataDir)
	notAfter := ""
	if t, err := readCertificateNotAfter(httpsProxyCertPath); err == nil {
		notAfter = t.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":   enabled,
		"port":      state.Port,
		"not_after": notAfter,
	})
}

// configureHTTPSProxy 按候选端口逐个尝试；请求端口为 0 时自动选择 7443 / 8443 / 9443。
func (s *Server) configureHTTPSProxy(req httpsProxyRequest) (int, error) {
	var lastErr error
	for _, port := range candidateHTTPSPorts(req.Port) {
		if err := s.applyHTTPSProxy(req, port); err != nil {
			lastErr = fmt.Errorf("端口 %d 不可用: %w", port, err)
			continue
		}
		return port, nil
	}
	if lastErr == nil {
		lastErr = errors.New("没有可用的 HTTPS 端口")
	}
	return 0, lastErr
}

func candidateHTTPSPorts(requested int) []int {
	if requested > 0 {
		return []int{requested}
	}
	return []int{7443, 8443, 9443}
}

func (s *Server) applyHTTPSProxy(req httpsProxyRequest, port int) error {
	dir, err := s.ensureNginxDir()
	if err != nil {
		return err
	}
	tlsDir := "/etc/havline-agent/tls"
	if err := os.MkdirAll(tlsDir, 0o750); err != nil {
		return err
	}
	if err := atomicWrite(httpsProxyCertPath, []byte(req.CertPEM), 0o640); err != nil {
		return err
	}
	if err := atomicWrite(httpsProxyKeyPath, []byte(req.KeyPEM), 0o600); err != nil {
		return err
	}
	path := filepath.Join(dir, httpsProxyConfigName)
	previous, readErr := os.ReadFile(path)
	if _, err := fsutil.BackupVersioned(path, 10); err != nil {
		return err
	}
	content := renderHTTPSProxy(port, httpsProxyCertPath, httpsProxyKeyPath)
	if err := atomicWrite(path, []byte(content), 0o640); err != nil {
		return err
	}
	if err := s.validateNginx(); err != nil {
		s.restoreHTTPSConfig(path, previous, readErr)
		return fmt.Errorf("nginx config rejected: %w", err)
	}
	if err := s.reloadNginx(); err != nil {
		s.restoreHTTPSConfig(path, previous, readErr)
		return err
	}
	pin, err := certificatePin(req.CertPEM)
	if err != nil {
		return err
	}
	if err := s.verifyHTTPSProxy(port, pin); err != nil {
		s.restoreHTTPSConfig(path, previous, readErr)
		return err
	}
	return writeHTTPSProxyState(s.cfg.DataDir, httpsProxyState{Port: port})
}

func (s *Server) removeHTTPSProxy() error {
	dir, err := s.ensureNginxDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, httpsProxyConfigName)
	previous, readErr := os.ReadFile(path)
	if errors.Is(readErr, os.ErrNotExist) {
		_ = os.Remove(httpsProxyStatePath(s.cfg.DataDir))
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := s.validateNginx(); err != nil {
		s.restoreHTTPSConfig(path, previous, readErr)
		return err
	}
	if err := s.reloadNginx(); err != nil {
		s.restoreHTTPSConfig(path, previous, readErr)
		return err
	}
	_ = os.Remove(httpsProxyStatePath(s.cfg.DataDir))
	return nil
}

func (s *Server) restoreHTTPSConfig(path string, previous []byte, readErr error) {
	if readErr == nil {
		_ = atomicWrite(path, previous, 0o640)
	} else {
		_ = os.Remove(path)
	}
	_ = s.reloadNginx()
}

func (s *Server) ensureNginxDir() (string, error) {
	dir := strings.TrimSpace(s.cfg.NginxConfDir)
	if dir != "" {
		return dir, nil
	}
	dir, err := detectNginxConfDir(s.nginxPath())
	if err != nil {
		return "", err
	}
	s.cfg.NginxConfDir = dir
	return dir, nil
}

func renderHTTPSProxy(port int, certPath, keyPath string) string {
	return fmt.Sprintf(`# Managed by havline-agent. Do not edit.
server {
    listen %d ssl default_server;
    server_name _;
%s

    location /api/v1/ {
        proxy_pass http://127.0.0.1:7700;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Authorization $http_authorization;
    }
}
`, port, nginxtmpl.SSLDirectives(nginxtmpl.TLSConfig{CertPath: certPath, KeyPath: keyPath}))
}

func certificatePin(certPEM string) (string, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return "", errors.New("证书 PEM 解码失败")
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:]), nil
}

// verifyHTTPSProxy 在 reload 后从本机回环地址握手，确认 7443 上真正返回的就是刚写入的证书。
// nginx reload 是异步的，这里留出短暂重试窗口，避免旧 worker 造成的瞬时误判。
func (s *Server) verifyHTTPSProxy(port int, want string) error {
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for {
		lastErr = probeHTTPSPin(port, want)
		if lastErr == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func probeHTTPSPin(port int, want string) error {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("本机 https://%s 自检失败：%w；%s", addr, err, portListenerHint(port))
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return fmt.Errorf("本机 https://%s 未返回证书；%s", addr, portListenerHint(port))
	}
	sum := sha256.Sum256(certs[0].Raw)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("本机 https://%s 证书指纹不匹配：期望 %s，实际 %s；%s", addr, want, got, portListenerHint(port))
	}
	return nil
}

// portListenerHint 把监听该端口的进程信息带进错误里，便于区分 nginx 未生效与端口被其他服务占用。
func portListenerHint(port int) string {
	out, err := exec.Command("ss", "-tlnp").CombinedOutput()
	if err != nil {
		return "无法读取端口监听信息"
	}
	suffix := ":" + strconv.Itoa(port)
	lines := make([]string, 0, 2)
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, suffix+" ") || strings.HasSuffix(strings.TrimSpace(line), suffix) {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	if len(lines) == 0 {
		return "未发现监听 " + suffix + " 的进程"
	}
	return "监听 " + suffix + "：" + strings.Join(lines, " | ")
}

func httpsProxyStatePath(dataDir string) string {
	return filepath.Join(dataDir, "https_transport.json")
}

func writeHTTPSProxyState(dataDir string, state httpsProxyState) error {
	if strings.TrimSpace(dataDir) == "" {
		return nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return atomicWrite(httpsProxyStatePath(dataDir), data, 0o640)
}

func readHTTPSProxyState(dataDir string) (httpsProxyState, bool) {
	data, err := os.ReadFile(httpsProxyStatePath(dataDir))
	if err != nil {
		return httpsProxyState{}, false
	}
	var state httpsProxyState
	if err := json.Unmarshal(data, &state); err != nil || state.Port <= 0 {
		return httpsProxyState{}, false
	}
	return state, true
}
