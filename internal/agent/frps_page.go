package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// frps 的 custom404Page 只认 VPS 上的文件路径，所以「自定义 404 页面内容」必须先把内容
// 落到 VPS 上。这里给 Havline 一个受控的写文件入口：只写绝对路径、只写 .html，避免变成
// 任意文件写入。

type frpsPageRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (s *Server) putFRPS404Page(w http.ResponseWriter, r *http.Request) {
	var req frpsPageRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	path, err := validatePagePath(req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "页面内容为空"})
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := atomicWrite(path, []byte(req.Content), 0o644); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path, "bytes": len(req.Content)})
}

func (s *Server) deleteFRPS404Page(w http.ResponseWriter, r *http.Request) {
	path, err := validatePagePath(r.URL.Query().Get("path"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// validatePagePath 只接受绝对路径 + .html/.htm 结尾，防止这个接口被当成任意文件写入。
func validatePagePath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	// agent 只跑在 Linux 上，这里按 POSIX 绝对路径判断（用 filepath.IsAbs 会在别的平台判错）
	if path == "" || !strings.HasPrefix(path, "/") {
		return "", errors.New("页面路径必须是绝对路径")
	}
	if strings.Contains(path, "..") {
		return "", errors.New("页面路径不允许包含 ..")
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm":
		return filepath.Clean(path), nil
	default:
		return "", errors.New("页面文件必须以 .html 或 .htm 结尾")
	}
}
