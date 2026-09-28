package agent

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fonu/fonu/internal/fsutil"
)

const httpsProxyConfigName = "fonuagent-https.conf"

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
	if req.Port <= 0 || req.Port > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "端口需在 1-65535 之间"})
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
	if err := s.configureHTTPSProxy(req); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "port": req.Port})
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

func (s *Server) configureHTTPSProxy(req httpsProxyRequest) error {
	dir, err := s.ensureNginxDir()
	if err != nil {
		return err
	}
	tlsDir := "/etc/fonu-agent/tls"
	if err := os.MkdirAll(tlsDir, 0o750); err != nil {
		return err
	}
	certPath := filepath.Join(tlsDir, "agent.crt")
	keyPath := filepath.Join(tlsDir, "agent.key")
	if err := atomicWrite(certPath, []byte(req.CertPEM), 0o640); err != nil {
		return err
	}
	if err := atomicWrite(keyPath, []byte(req.KeyPEM), 0o600); err != nil {
		return err
	}
	path := filepath.Join(dir, httpsProxyConfigName)
	previous, readErr := os.ReadFile(path)
	if _, err := fsutil.BackupVersioned(path, 10); err != nil {
		return err
	}
	content := renderHTTPSProxy(req.Port, certPath, keyPath)
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
	return writeHTTPSProxyState(s.cfg.DataDir, httpsProxyState{Port: req.Port})
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
	return fmt.Sprintf(`# Managed by fonu-agent. Do not edit.
server {
    listen %d ssl;
    server_name _;
    ssl_certificate %s;
    ssl_certificate_key %s;
    ssl_protocols TLSv1.2 TLSv1.3;

    location /api/v1/ {
        proxy_pass http://127.0.0.1:7700;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Authorization $http_authorization;
    }
}
`, port, certPath, keyPath)
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
