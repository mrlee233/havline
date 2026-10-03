package agent

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/havline/havline/internal/fsutil"
)

// maxRouteVersions 每个域名 vhost 保留的版本份数。内容相同的写入会被 fsutil 去重，
// 因此这里对应的是最近 10 次「配置真的变了」的改动。
const maxRouteVersions = 10

type routeVersionInfo struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// routeVersions 返回某域名的当前配置与历史版本，供应用侧把「漂移」与「历史」放在一起看。
func (s *Server) routeVersions(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if !validDomain(domain) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid domain"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.routePath(domain)
	content, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "该域名尚未部署反代"})
		return
	}
	versions, err := fsutil.ListVersions(path)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"domain":   domain,
		"content":  string(content),
		"versions": mapRouteVersions(versions),
	})
}

// routeVersion 返回某个历史版本的内容（前端拿它与当前配置做差异对比）。
func (s *Server) routeVersion(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if !validDomain(domain) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid domain"})
		return
	}
	name := r.PathValue("name")
	s.mu.Lock()
	defer s.mu.Unlock()

	content, err := fsutil.ReadVersion(s.routePath(domain), name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domain": domain, "name": name, "content": string(content)})
}

// rollbackRoute 把某域名的 vhost 回退到指定版本。
func (s *Server) rollbackRoute(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if !validDomain(domain) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid domain"})
		return
	}
	name := r.PathValue("name")
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.routePath(domain)
	content, err := fsutil.ReadVersion(path, name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if err := s.applyRouteContent(path, content); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if err := s.reloadRouteConfig(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "domain": domain, "name": name})
}

// applyRouteContent 写入 vhost 内容并做 nginx -t 校验。
//
// 写之前先把当前内容留成一个版本；校验失败时恢复回上一个可用版本（此前没有文件才删除）。
// 旧行为是校验失败直接删文件——那等于把已经在跑的站点从磁盘上抹掉，只能靠重新部署救回来。
func (s *Server) applyRouteContent(path string, content []byte) error {
	previous, readErr := os.ReadFile(path)
	if _, err := fsutil.BackupVersioned(path, maxRouteVersions); err != nil {
		return err
	}
	if err := atomicWrite(path, content, 0o640); err != nil {
		return err
	}
	if err := s.validateRouteConfig(); err != nil {
		if readErr == nil {
			_ = atomicWrite(path, previous, 0o640)
		} else {
			_ = os.Remove(path)
		}
		return fmt.Errorf("nginx config rejected: %w", err)
	}
	return nil
}

func mapRouteVersions(entries []fsutil.VersionEntry) []routeVersionInfo {
	out := make([]routeVersionInfo, 0, len(entries))
	for _, entry := range entries {
		out = append(out, routeVersionInfo{Name: entry.Name, Size: entry.Size, CreatedAt: entry.CreatedAt})
	}
	return out
}
