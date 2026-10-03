package agent

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/havline/havline/internal/sysinfo"
)

const (
	// metricsInterval 两次 CPU 采样的最小间隔：间隔内重复调用直接返回上次结果，避免数值抖动
	metricsInterval = time.Second
	// maxLogLineBytes 单行日志截断长度，避免一行超长把响应撑大
	maxLogLineBytes = 4 << 10
	// maxLogLines 日志尾部行数上限
	maxLogLines = 2000
	// maxFRPSConfigBytes frps.toml 回读上限
	maxFRPSConfigBytes = 256 << 10
)

// metrics 采集 VPS 主机资源（CPU / 内存 / 磁盘 / 负载 / 运行时长）。
// 复用 internal/sysinfo：agent 跑在 VPS 上，采到的天然是 VPS 自身数值。
func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()

	now := time.Now()
	if s.metricsCached != nil && now.Sub(s.metricsAt) < metricsInterval {
		writeJSON(w, http.StatusOK, s.metricsCached)
		return
	}

	stats, sample := sysinfo.Read(s.metricsSample)
	s.metricsSample = sample
	payload := map[string]any{
		"cpu_percent":      math.Round(stats.CPUPercent*10) / 10,
		"mem_used_bytes":   stats.MemUsedBytes,
		"mem_total_bytes":  stats.MemTotalBytes,
		"disk_used_bytes":  stats.DiskUsedBytes,
		"disk_total_bytes": stats.DiskTotalBytes,
		// 当前只统计根文件系统（与 internal/sysinfo 的 Linux 实现一致）
		"disk_path":  "/",
		"checked_at": now.UTC().Format(time.RFC3339),
	}
	if load := sysinfo.LoadAvg(); len(load) == 3 {
		payload["load_avg"] = load
	}
	if uptime := sysinfo.UptimeSeconds(); uptime > 0 {
		payload["uptime_seconds"] = uptime
	}

	s.metricsAt = now
	s.metricsCached = payload
	writeJSON(w, http.StatusOK, payload)
}

// nginxLogs 读取 VPS 上 Nginx 的 access / error 日志尾部。
// 路径不接受请求参数：只从 `nginx -T` 的实际生效配置、默认目录或环境变量目录里取，避免变成任意文件读取。
func (s *Server) nginxLogs(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("type"))
	if kind != "access" && kind != "error" {
		kind = "access"
	}
	lines := maxLogLines
	if raw := strings.TrimSpace(r.URL.Query().Get("lines")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			lines = parsed
		}
	}
	if lines <= 0 || lines > maxLogLines {
		lines = maxLogLines
	}

	path := s.nginxLogPath(kind)
	if path == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"type": kind, "path": "", "lines": []string{}, "truncated": false,
			"checked_at": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	content, truncated, err := tailFile(path, lines, maxLogLineBytes)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在不是错误：返回空列表 + 路径，由前端提示「未找到日志文件」
			writeJSON(w, http.StatusOK, map[string]any{
				"type": kind, "path": path, "lines": []string{}, "truncated": false,
				"checked_at": time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		status := http.StatusInternalServerError
		if os.IsPermission(err) {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]any{"error": fmt.Sprintf("读取日志失败：%v", err), "path": path})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"type": kind, "path": path, "lines": content, "truncated": truncated,
		"checked_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// nginxLogPath 解析日志文件路径：优先 `nginx -T` 实际生效配置里的指令，其次环境变量目录，最后默认目录
func (s *Server) nginxLogPath(kind string) string {
	directive := "access_log"
	if kind == "error" {
		directive = "error_log"
	}
	if nginx := s.nginxPath(); nginx != "" {
		if out, err := exec.Command(nginx, "-T").CombinedOutput(); err == nil {
			if path := logPathFromDump(string(out), directive); path != "" {
				return path
			}
		}
	}
	dir := strings.TrimSpace(os.Getenv("HAVLINE_AGENT_NGINX_LOG_DIR"))
	if dir == "" {
		dir = "/var/log/nginx"
	}
	return filepath.Join(dir, kind+".log")
}

// logPathFromDump 从 `nginx -T` 输出里取第一条绝对路径的日志指令（跳过 off / stderr / 含变量的写法）
func logPathFromDump(dump, directive string) string {
	pattern := regexp.MustCompile(`(?m)^\s*` + directive + `\s+([^;\s]+)`)
	for _, match := range pattern.FindAllStringSubmatch(dump, -1) {
		candidate := strings.TrimSpace(match[1])
		if candidate == "" || candidate == "off" || candidate == "stderr" || strings.Contains(candidate, "$") {
			continue
		}
		// 只接受绝对路径：agent 只跑在 Linux 上，用前导斜杠判定（filepath.IsAbs 在 Windows 上对 /var/... 返回 false）
		if strings.HasPrefix(candidate, "/") {
			return candidate
		}
	}
	return ""
}

// tailFile 从文件末尾倒读最多 lines 行：按块回溯，避免把整个日志读进内存；单行超长时截断
func tailFile(path string, lines int, maxLine int) ([]string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}

	const chunkSize = 32 * 1024
	var (
		offset int64 = info.Size()
		buffer []byte
	)
	for offset > 0 && bytes.Count(buffer, []byte("\n")) <= lines {
		readSize := int64(chunkSize)
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize
		block := make([]byte, readSize)
		if _, err := file.ReadAt(block, offset); err != nil && err != io.EOF {
			return nil, false, err
		}
		buffer = append(block, buffer...)
	}

	rows := strings.Split(strings.TrimRight(string(buffer), "\n"), "\n")
	truncated := false
	if len(rows) > lines {
		rows = rows[len(rows)-lines:]
		truncated = true
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if len(row) > maxLine {
			row = row[:maxLine] + "…"
		}
		out = append(out, row)
	}
	return out, truncated, nil
}

// vpsCertificate VPS 证书目录里的一张证书
// 注意：证书按 <CertsDir>/<域名>/fullchain.pem + privkey.pem 存放（与 putCertificate 一致）。
type vpsCertificate struct {
	Domain   string `json:"domain"`
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
	NotAfter string `json:"not_after,omitempty"`
	DaysLeft int    `json:"days_left"`
}

// certs 列出 VPS 证书目录下的域名与到期时间：用于与 Havline 证书库交叉核对，也能看到宝塔自行申请的证书
func (s *Server) certs(w http.ResponseWriter, _ *http.Request) {
	dir := strings.TrimSpace(s.cfg.CertsDir)
	payload := map[string]any{
		"certs": []vpsCertificate{}, "warnings": []string{}, "dir": dir,
		"checked_at": time.Now().UTC().Format(time.RFC3339),
	}
	if dir == "" {
		writeJSON(w, http.StatusOK, payload)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, payload)
			return
		}
		status := http.StatusInternalServerError
		if os.IsPermission(err) {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": fmt.Sprintf("读取证书目录失败：%v", err)})
		return
	}

	now := time.Now()
	certs := make([]vpsCertificate, 0, len(entries))
	warnings := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		domain := entry.Name()
		item := vpsCertificate{
			Domain:   domain,
			CertPath: filepath.Join(dir, domain, "fullchain.pem"),
			KeyPath:  filepath.Join(dir, domain, "privkey.pem"),
		}
		notAfter, err := readCertificateNotAfter(item.CertPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s：%v", domain, err))
		} else {
			item.NotAfter = notAfter.UTC().Format(time.RFC3339)
			item.DaysLeft = int(notAfter.Sub(now).Hours() / 24)
		}
		certs = append(certs, item)
	}
	// 到期最快的排在前面，便于直接看出哪张该续了
	sort.Slice(certs, func(i, j int) bool { return certs[i].DaysLeft < certs[j].DaysLeft })

	payload["certs"] = certs
	payload["warnings"] = warnings
	writeJSON(w, http.StatusOK, payload)
}

// readCertificateNotAfter 解析 PEM 证书的到期时间
func readCertificateNotAfter(path string) (time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return time.Time{}, fmt.Errorf("不是有效的 PEM 证书")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}
	return cert.NotAfter, nil
}

// nginxReload 显式校验并重载 Nginx，把校验/重载失败的原因回传。
// 这里不加 s.mu：validateNginx / reloadNginx 的调用方约定（见 writeRoute 注释）是外部持锁，
// 而本端点是用户显式触发的排查动作，不加锁可避免与路由写入相互阻塞。
func (s *Server) nginxReload(w http.ResponseWriter, _ *http.Request) {
	payload := map[string]any{"checked_at": time.Now().UTC().Format(time.RFC3339)}
	if err := s.validateNginx(); err != nil {
		payload["ok"] = false
		payload["test_ok"] = false
		payload["test_error"] = err.Error()
		writeJSON(w, http.StatusOK, payload)
		return
	}
	payload["test_ok"] = true
	if err := s.reloadNginx(); err != nil {
		payload["ok"] = false
		payload["reload_ok"] = false
		payload["reload_error"] = err.Error()
		writeJSON(w, http.StatusOK, payload)
		return
	}
	// 重载成功后让 Nginx 可用性探测的缓存失效，避免页面继续显示旧结论
	s.invalidateNginxProbe()
	payload["ok"] = true
	payload["reload_ok"] = true
	writeJSON(w, http.StatusOK, payload)
}

// frpsConfig 回读 VPS 上的 frps.toml，供应用侧与「即将下发的内容」比对，检出被手工修改的情况
func (s *Server) frpsConfig(w http.ResponseWriter, _ *http.Request) {
	path := strings.TrimSpace(s.cfg.FrpsConfigPath)
	payload := map[string]any{
		"path": path, "exists": false, "content": "", "truncated": false,
		"checked_at": time.Now().UTC().Format(time.RFC3339),
	}
	if path == "" {
		writeJSON(w, http.StatusOK, payload)
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		writeJSON(w, http.StatusOK, payload)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		status := http.StatusInternalServerError
		if os.IsPermission(err) {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]any{"error": fmt.Sprintf("读取 frps 配置失败：%v", err), "path": path})
		return
	}
	if len(data) > maxFRPSConfigBytes {
		data = data[:maxFRPSConfigBytes]
		payload["truncated"] = true
	}
	payload["exists"] = true
	payload["content"] = string(data)
	payload["mod_time"] = info.ModTime().UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, payload)
}
