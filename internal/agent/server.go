package agent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AgentVersion 随每次 agent 行为变更手动递增；status/ping 与安装脚本都会回报，用于确认 VPS 上跑的是哪个版本。
// 0.5.0：目录判定改用 nginx -T 自省（dump 完整生效配置，按 "# configuration file" 标记
// 找出真正被加载的文件所在目录，零猜测）；include 链递归与 FONU_AGENT_NGINX_CONF_DIR 保留为回退。
// 0.9.0：Nginx 可用性显式探测（status 暴露 nginx_ok/nginx_missing）、部署前预检、一键安装 Nginx 端点。
// 0.9.1：status 补充 nginx_running / nginx_version，供页面展示 Nginx 状态卡片。
const AgentVersion = "0.9.1"

// Config 是 fonu-agent 的运行配置。agent 只在 VPS 本地管理自己的 Nginx 片段、证书和 frps 配置。
type Config struct {
	ListenAddr     string
	Token          string
	NginxConfDir   string
	NginxBinary    string
	CertsDir       string
	FrpsConfigPath string
	FrpsBinary     string
	DataDir        string
}

// Server 暴露 agent HTTP 服务。
type Server struct {
	cfg Config
	mu  sync.Mutex
	// nginx 可用性探测缓存：status 轮询频繁，避免每次都起 nginx -t 进程
	nginxStatusMu sync.Mutex
	nginxStatusAt time.Time
	nginxStatus   nginxProbe
}

// nginxProbe Nginx 可用性探测结果。
type nginxProbe struct {
	OK      bool   `json:"ok"`      // nginx -t 通过
	Missing bool   `json:"missing"` // 未找到可执行文件
	Running bool   `json:"running"` // 有 nginx 进程在运行
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// resolveNginxConfDir 预先解析并缓存 vhost 目录：探测结果落盘到 DataDir/nginx_conf_dir，
// 启动时后台执行一次（不阻断监听），路由写入时优先读缓存。
// 返回目录与错误；缓存命中时直接返回。
func (s *Server) resolveNginxConfDir() (string, error) {
	s.mu.Lock()
	cached := strings.TrimSpace(s.cfg.NginxConfDir)
	s.mu.Unlock()
	if cached != "" {
		return cached, nil
	}
	dir, err := detectNginxConfDir(s.nginxPath())
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.cfg.NginxConfDir = dir
	s.mu.Unlock()
	_ = os.MkdirAll(dir, 0o755)
	// 结果落盘，下次启动优先复用（避免每次启动都跑 -T）
	if s.cfg.DataDir != "" {
		_ = os.WriteFile(filepath.Join(s.cfg.DataDir, "nginx_conf_dir"), []byte(dir), 0o644)
	}
	return dir, nil
}

// preloadNginxConfDir 启动后台预解析：成功与否不影响监听，结果进缓存供状态与路由写入使用。
func (s *Server) preloadNginxConfDir() {
	go func() {
		if _, err := s.resolveNginxConfDir(); err != nil {
			log.Printf("nginx conf dir 预解析失败（路由写入时会重试）: %v", err)
		}
	}()
}

var domainPattern = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))+$`)

func NewServer(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		cfg.ListenAddr = ":7700"
	}
	if strings.TrimSpace(cfg.NginxConfDir) == "" {
		// 目录探测失败不阻断启动：agent 照常提供状态接口，路由写入时重试探测（systemd PATH 下可能找不到 nginx，写入前会补全路径）
		cfg.NginxConfDir = ""
	}
	if strings.TrimSpace(cfg.NginxBinary) == "" {
		cfg.NginxBinary = "nginx"
	}
	if strings.TrimSpace(cfg.CertsDir) == "" {
		cfg.CertsDir = "/var/lib/fonu-agent/certs"
	}
	if strings.TrimSpace(cfg.FrpsConfigPath) == "" {
		cfg.FrpsConfigPath = "/etc/frp/frps.toml"
	}
	if strings.TrimSpace(cfg.FrpsBinary) == "" {
		cfg.FrpsBinary = "/usr/local/bin/frps"
	} else if !filepath.IsAbs(cfg.FrpsBinary) {
		// 安装器固定把 frps 装到 /usr/local/bin；状态展示与实际安装位置保持一致
		cfg.FrpsBinary = "/usr/local/bin/" + filepath.Base(cfg.FrpsBinary)
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		cfg.DataDir = "/var/lib/fonu-agent"
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("agent token is required")
	}
	for _, dir := range []string{cfg.NginxConfDir, cfg.CertsDir, cfg.DataDir} {
		// NginxConfDir 可能为空（探测延迟到路由写入时重试），跳过而不是报错
		if strings.TrimSpace(dir) == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create agent directory: %w", err)
		}
	}
	server := &Server{cfg: cfg}
	// 启动后台预解析目录（不阻断监听）；每次启动都会用最新探测逻辑重新解析并覆盖落盘缓存，
	// 历史错误目录的缓存会被自动修正。
	server.preloadNginxConfDir()
	return server, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/ping", s.auth(s.ping))
	mux.HandleFunc("GET /api/v1/status", s.auth(s.status))
	mux.HandleFunc("GET /api/v1/routes", s.auth(s.routes))
	mux.HandleFunc("PUT /api/v1/routes", s.auth(s.putRoute))
	mux.HandleFunc("GET /api/v1/routes/{domain}/conf", s.auth(s.routeConf))
	mux.HandleFunc("PUT /api/v1/china-cidr", s.auth(s.putChinaCIDR))
	mux.HandleFunc("POST /api/v1/nginx/install", s.auth(s.installNginx))
	mux.HandleFunc("DELETE /api/v1/routes/{domain}", s.auth(s.deleteRoute))
	mux.HandleFunc("PUT /api/v1/frps/config", s.auth(s.putFRPSConfig))
	mux.HandleFunc("POST /api/v1/frps/install", s.auth(s.installFRPS))
	mux.HandleFunc("POST /api/v1/frps/action", s.auth(s.frpsAction))
	mux.HandleFunc("POST /api/v1/certs", s.auth(s.putCertificate))
	return mux
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	server := &http.Server{Addr: s.cfg.ListenAddr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	return server.ListenAndServe()
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.cfg.Token {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) ping(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "fonu-agent"})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	ports := map[string]bool{"80": portListening(80), "443": portListening(443), "7000": portListening(7000), "8080": portListening(8080)}
	nginxProbe := s.probeNginx()
	frpsActive := exec.Command("systemctl", "is-active", "--quiet", "frps").Run() == nil
	writeJSON(w, http.StatusOK, map[string]any{
		"service":          "fonu-agent",
		"agent_version":    AgentVersion,
		"nginx_binary":     s.cfg.NginxBinary,
		"nginx_conf_dir":   s.cfg.NginxConfDir,
		"nginx_ok":         nginxProbe.OK,
		"nginx_missing":    nginxProbe.Missing,
		"nginx_running":    nginxProbe.Running,
		"nginx_path":       nginxProbe.Path,
		"nginx_version":    nginxProbe.Version,
		"nginx_error":      nginxProbe.Error,
		"frps_binary":      s.cfg.FrpsBinary,
		"frps_config_path": s.cfg.FrpsConfigPath,
		"ports":            ports,
		"frps_active":      frpsActive,
		"frps_vhost_hint":  "curl -H 'Host: 你的域名' http://127.0.0.1:8080 可测试 frps vhost 是否通",
		"frps_unit_exec":   strings.TrimSpace(execCommandOutput("systemctl", "show", "frps", "-p", "ExecStart", "--value")),
		"frps_enabled":     exec.Command("systemctl", "is-enabled", "--quiet", "frps").Run() == nil,
		"frps_version":     strings.TrimSpace(frpsVersion(s.cfg.FrpsBinary)),
		"frps_log_tail":    strings.TrimSpace(journalTail("frps", 6)),
		"dashboard_addr":   strings.TrimSpace(dashboardAddr(s.cfg.FrpsConfigPath)),
		"routes":           s.routeNames(),
		"checked_at":       time.Now().UTC().Format(time.RFC3339),
	})
}

type frpsInstallRequest struct {
	Version string `json:"version"`
	Proxy   string `json:"proxy,omitempty"`
}

// installFRPS 下载指定版本的 frp release、安装 frps 二进制并注册 systemd 服务（幂等，重复调用=升级）。
func (s *Server) installFRPS(w http.ResponseWriter, r *http.Request) {
	var req frpsInstallRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	version := strings.TrimPrefix(strings.TrimSpace(req.Version), "v")
	if matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+$`, version); !matched {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version 需形如 0.71.0"})
		return
	}
	arch := map[string]string{"amd64": "amd64", "arm64": "arm64"}[runtime.GOARCH]
	if arch == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported agent arch: " + runtime.GOARCH})
		return
	}
	downloadURL := fmt.Sprintf("https://github.com/fatedier/frp/releases/download/v%s/frp_%s_linux_%s.tar.gz", version, version, arch)
	if proxy := strings.TrimSpace(req.Proxy); proxy != "" {
		downloadURL = strings.TrimRight(proxy, "/") + "/" + downloadURL
	}
	workDir := filepath.Join(s.cfg.DataDir, "tmp-frps-"+version)
	_ = os.RemoveAll(workDir)
	defer os.RemoveAll(workDir)
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	tarball := filepath.Join(workDir, "frp.tar.gz")
	if err := downloadToFile(downloadURL, tarball); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "下载 frps 失败: " + err.Error()})
		return
	}
	installPath := s.cfg.FrpsBinary
	if !filepath.IsAbs(installPath) {
		installPath = "/usr/local/bin/frps"
	}
	if err := extractFile(tarball, "frp_"+version+"_linux_"+arch+"/frps", installPath, 0o755); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "解压 frps 失败: " + err.Error()})
		return
	}
	notes := []string{"frps 二进制已安装: " + installPath}
	unitPath := "/etc/systemd/system/frps.service"
	writeUnit := false
	if _, err := os.Stat(unitPath); errors.Is(err, os.ErrNotExist) {
		writeUnit = true
		notes = append(notes, "已创建 systemd 单位 frps.service")
	} else if existing, err := os.ReadFile(unitPath); err == nil {
		// 旧 unit（如 frps-admin 面板遗留）的 ExecStart 可能指向已不存在的二进制；检测到失效路径时覆写接管
		for _, line := range strings.Split(string(existing), "\n") {
			if execPath, found := strings.CutPrefix(strings.TrimSpace(line), "ExecStart="); found {
				ref := strings.Fields(execPath)
				if len(ref) > 0 {
					if _, statErr := os.Stat(ref[0]); statErr != nil {
						writeUnit = true
						notes = append(notes, fmt.Sprintf("检测到现有 frps.service 的 ExecStart 指向不存在的 %s，已覆写接管", ref[0]))
					}
				}
				break
			}
		}
	}
	if writeUnit {
		unit := fmt.Sprintf("[Unit]\nDescription=frps service (managed by fonu-agent)\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nExecStart=%s -c %s\nRestart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=multi-user.target\n", installPath, s.cfg.FrpsConfigPath)
		if err := atomicWrite(unitPath, []byte(unit), 0o644); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "写入 frps.service 失败: " + err.Error()})
			return
		}
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}
	// nohup 手工跑的 frps 让位给 systemd；已在 systemd 下运行则重启以加载新二进制
	if exec.Command("systemctl", "is-active", "--quiet", "frps").Run() != nil {
		if exec.Command("pgrep", "-x", "frps").Run() == nil {
			_ = exec.Command("pkill", "-x", "frps").Run()
			time.Sleep(time.Second)
			notes = append(notes, "已停止 nohup 方式运行的旧 frps 进程")
		}
		_ = exec.Command("systemctl", "enable", "--now", "frps").Run()
		notes = append(notes, "frps.service 已启动")
	} else {
		_ = exec.Command("systemctl", "restart", "frps").Run()
		notes = append(notes, "frps.service 已重启以加载新版本")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version, "binary": installPath, "notes": notes})
}

type frpsActionRequest struct {
	Action string `json:"action"` // start | stop | restart
}

// frpsAction 对 VPS 本机 frps 服务执行启停（仅白名单动作，经 systemctl）。
func (s *Server) frpsAction(w http.ResponseWriter, r *http.Request) {
	var req frpsActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	var command *exec.Cmd
	switch req.Action {
	case "start":
		command = exec.Command("systemctl", "start", "frps")
	case "stop":
		command = exec.Command("systemctl", "stop", "frps")
	case "restart":
		command = exec.Command("systemctl", "restart", "frps")
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action 需为 start/stop/restart"})
		return
	}
	if output, err := command.CombinedOutput(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("systemctl %s frps 失败: %s", req.Action, strings.TrimSpace(string(output)))})
		return
	}
	time.Sleep(800 * time.Millisecond) // 给 frps 启动留出窗口，失败时才能在 is-active/journal 里看到
	active := exec.Command("systemctl", "is-active", "--quiet", "frps").Run() == nil
	result := map[string]any{"ok": true, "action": req.Action, "frps_active": active}
	if !active {
		// 启动失败时把 frps 的失败原因带回去，而不是只说“启动成功”
		journal, _ := exec.Command("journalctl", "-u", "frps", "-n", "8", "--no-pager").CombinedOutput()
		result["ok"] = false
		result["error"] = fmt.Sprintf("systemctl %s 已执行但 frps 未进入运行状态，最近日志：\n%s", req.Action, strings.TrimSpace(string(journal)))
	}
	writeJSON(w, http.StatusOK, result)
}

func downloadToFile(url, path string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(resp.Body, 256<<20))
	return err
}

// extractFile 从 tar.gz 中取出 name 对应的单个文件写入 dest。
func extractFile(tarball, name, dest string, mode os.FileMode) error {
	file, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("压缩包内未找到 %s", name)
		}
		if err != nil {
			return err
		}
		if strings.TrimSpace(filepath.Clean(header.Name)) != name {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(reader, 256<<20))
		if err != nil {
			return err
		}
		return atomicWrite(dest, data, mode)
	}
}

func execCommandOutput(name string, args ...string) string {
	output, _ := exec.Command(name, args...).CombinedOutput()
	return string(output)
}

// dashboardAddr 已废弃：面板地址改由 Fonu 前端从服务端表单的「frps 管理接口」字段拼出，agent 不再解析 frps.toml。
func dashboardAddr(configPath string) string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	var addr, port string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if value, found := strings.CutPrefix(line, "webServer.addr"); found {
			addr = strings.Trim(strings.TrimSpace(strings.TrimPrefix(value, "=")), "\"")
		} else if value, found := strings.CutPrefix(line, "webServer.port"); found {
			port = strings.TrimSpace(strings.TrimPrefix(value, "="))
		}
	}
	if addr == "" || port == "" || port == "0" {
		return ""
	}
	if addr == "0.0.0.0" || addr == "::" {
		addr = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%s", addr, port)
}

func frpsVersion(binary string) string {
	output, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return string(output)
}

func journalTail(unit string, lines int) string {
	output, err := exec.Command("journalctl", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager").CombinedOutput()
	if err != nil {
		return ""
	}
	return string(output)
}

// nginxBinaryPath 解析实际可用的 nginx 可执行文件：优先传入名称（PATH/绝对路径），
// 否则从运行中的 nginx 主进程反查（/proc/<pid>/exe），零硬编码安装位置。
func nginxBinaryPath(configured string) string {
	if p := strings.TrimSpace(configured); p != "" {
		if filepath.IsAbs(p) {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		} else if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	// 从运行中的 nginx master 进程反查二进制（不依赖安装位置）
	if entries, err := os.ReadDir("/proc"); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if data, err := os.ReadFile("/proc/" + entry.Name() + "/comm"); err != nil || strings.TrimSpace(string(data)) != "nginx" {
				continue
			}
			if link, err := os.Readlink("/proc/" + entry.Name() + "/exe"); err == nil && strings.Contains(strings.ToLower(link), "nginx") {
				return link
			}
		}
	}
	return ""
}

// detectNginxConfDir 自动发现 Nginx 实际加载完整 server 配置的目录（零硬编码路径）：
// 1) nginx -T dump 完整生效配置，收集所有被加载的 .conf 所在目录（Nginx 亲口说的事实）；
// 2) 按内容启发排序：目录下已有含 server_name 的 .conf（HTTP vhost 语义）优先，片段目录（location-only）靠后；
// 3) 逐个试探：写入 → nginx -t → 通过即选定并缓存；失败则清理该次写入并换下一个。
// Nginx 自己是唯一裁判，agent 不猜测、不评分、不写死任何安装路径。
func detectNginxConfDir(binary string) (string, error) {
	nginx := nginxBinaryPath(binary)
	if nginx == "" {
		return "", fmt.Errorf("未找到可用的 nginx 可执行文件；请先在 VPS 安装 Nginx（apt install nginx / yum install nginx），或用 FONU_AGENT_NGINX_BIN 显式指定路径")
	}
	out, err := exec.Command(nginx, "-T").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("nginx -T 执行失败: %w", err)
	}
	loadedDirs := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if value, found := strings.CutPrefix(line, "# configuration file "); found {
			p := strings.TrimSuffix(strings.TrimSpace(value), ":")
			if strings.HasSuffix(p, ".conf") {
				loadedDirs[filepath.Dir(p)] = true
			}
		}
	}
	delete(loadedDirs, ".")
	if len(loadedDirs) == 0 {
		return "", fmt.Errorf("nginx -T 输出中未发现任何被加载的 .conf 目录")
	}
	// 内容启发：目录下已有含 server_name 的 .conf 视为 HTTP vhost 目录，优先尝试。
	dirs := make([]string, 0, len(loadedDirs))
	for dir := range loadedDirs {
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool {
		si, sj := dirHasVhostConf(dirs[i]), dirHasVhostConf(dirs[j])
		if si != sj {
			return si // vhost 语义目录在前
		}
		return dirs[i] < dirs[j]
	})
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		probe := filepath.Join(dir, "fonu-probe.conf")
		probeBody := "# Managed by fonu-agent. Do not edit.\nserver {\n    listen 127.0.0.1:1;\n    server_name fonu-probe.invalid;\n}\n"
		if err := atomicWrite(probe, []byte(probeBody), 0o644); err != nil {
			continue
		}
		// 两级验证：-t 仅验证语法，不保证文件被 include 加载（主配置目录等无 include 的目录也能通过）。
		// 必须再执行 -T 确认探针文件真的出现在生效配置清单里，才算“会被加载”的目录。
		if err := runCommand(nginx, "-t"); err == nil {
			if tOut, tErr := exec.Command(nginx, "-T").CombinedOutput(); tErr == nil && strings.Contains(string(tOut), probe) {
				_ = os.Remove(probe)
				_ = runCommand(nginx, "-s", "reload") // 清除 probe 后恢复原状
				return dir, nil
			}
			// -t 通过但 -T 不含探针：该目录不被 include 加载，删除探针换下一个
			_ = os.Remove(probe)
			continue
		}
		// 该目录不接受完整 server 块（片段目录）：删除 probe 文件。
		// 若 -t 失败信息指向历史残留的 fonu-*.conf，也一并清理（仅限 Agent 生成的文件）。
		_ = os.Remove(probe)
		if tOut, tErr := exec.Command(nginx, "-t").CombinedOutput(); tErr != nil {
			cleanupLegacyFonuConf(string(tOut))
			if err := runCommand(nginx, "-t"); err == nil {
				continue // 清理残留后配置恢复可用，换下一个目录
			}
		}
	}
	return "", fmt.Errorf("在 %d 个被加载目录中均无法放置完整 server 配置（已尝试: %s）；可用 FONU_AGENT_NGINX_CONF_DIR 显式指定", len(dirs), strings.Join(dirs, ", "))
}

// dirHasVhostConf 判断目录下是否已有含 server_name 的 .conf（HTTP vhost 语义）。
func dirHasVhostConf(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(dir, entry.Name())); err == nil && strings.Contains(string(data), "server_name") {
			return true
		}
	}
	return false
}

// cleanupLegacyFonuConf 根据 nginx -t 的报错文本清理 Agent 历史生成的坏文件（仅限 fonu- 前缀）。
func cleanupLegacyFonuConf(nginxTestOutput string) {
	for _, line := range strings.Split(nginxTestOutput, "\n") {
		idx := strings.Index(line, "fonu-")
		if idx < 0 {
			continue
		}
		start, end := -1, -1
		for i := idx; i >= 0 && line[i] != ' '; i-- {
			start = i
		}
		for i := idx; i < len(line) && line[i] != ' ' && line[i] != ':'; i++ {
			end = i + 1
		}
		if start < 0 || end <= start {
			continue
		}
		path := line[start:end]
		if !strings.HasSuffix(path, ".conf") {
			continue
		}
		if data, err := os.ReadFile(path); err == nil && strings.HasPrefix(string(data), "# Managed by fonu-agent.") {
			_ = os.Remove(path)
		}
	}
}

func portListening(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (s *Server) routes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.routeDetails())
}

// routeDetails 返回每条路由的真实状态：文件存在/语法通过/被 Nginx 实际加载。
func (s *Server) routeDetails() []map[string]any {
	entries, err := os.ReadDir(s.cfg.NginxConfDir)
	if err != nil {
		return []map[string]any{}
	}
	details := []map[string]any{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "fonu-") || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		domain := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "fonu-"), ".conf")
		path := filepath.Join(s.cfg.NginxConfDir, entry.Name())
		item := map[string]any{
			"domain":      domain,
			"file_exists": true,
			"syntax_ok":   false,
			"loaded":      false,
		}
		// 语法与加载判定合并在一次 -T 中：-T 成功且输出含本文件 = 语法通过且被加载
		if nginx := s.nginxPath(); nginx != "" {
			if tOut, tErr := exec.Command(nginx, "-T").CombinedOutput(); tErr == nil {
				item["syntax_ok"] = true
				item["loaded"] = strings.Contains(string(tOut), path)
			}
		}
		details = append(details, item)
	}
	sort.Slice(details, func(i, j int) bool { return details[i]["domain"].(string) < details[j]["domain"].(string) })
	return details
}

type routeRequest struct {
	Domain        string   `json:"domain"`
	Upstream      string   `json:"upstream"`
	TLS           bool     `json:"tls"`
	CertDir       string   `json:"cert_dir,omitempty"`
	WebSocket     bool     `json:"websocket,omitempty"`
	VhostPort     int      `json:"vhost_port,omitempty"`      // 回源自检用：Fonu 传入本次部署的 frps vhost 端口
	RedirectTLS   bool     `json:"redirect_https,omitempty"`  // HTTP 80 强制跳转 HTTPS
	AllowIPs      []string `json:"allow_ips,omitempty"`       // IP 白名单；空则不限制
	BasicAuthUser string   `json:"basic_auth_user,omitempty"` // Basic Auth 用户名；空则不启用（密码自动生成）
	// 第一梯队部署设置（仅作用于公网 Nginx vhost，不写入 frpc.toml）
	DenyIPs           []string `json:"deny_ips,omitempty"`             // IP 黑名单；与白名单可同时使用
	BasicAuthPassword string   `json:"basic_auth_password,omitempty"`  // Basic Auth 自定义密码；空则首次自动生成
	HTTPSDisabled     bool     `json:"https_disabled,omitempty"`       // 显式关闭 HTTPS（证书已推送也不监听 443）
	ClientMaxBodySize string   `json:"client_max_body_size,omitempty"` // 上传大小限制，如 50m
	ProxyReadTimeout  string   `json:"proxy_read_timeout,omitempty"`   // 代理读超时，如 3600s
	SecurityHeaders   bool     `json:"security_headers,omitempty"`     // 安全响应头（HSTS 等）
	TLS13Only         bool     `json:"tls13_only,omitempty"`           // 仅 TLS 1.3
	// 第二梯队部署设置：限流 / 连接数 / 仅中国大陆 IP
	RateLimitRate  int  `json:"rate_limit_rate,omitempty"`  // 请求限流：每秒请求数
	RateLimitBurst int  `json:"rate_limit_burst,omitempty"` // 请求限流：突发允许量
	ConnLimitMax   int  `json:"conn_limit_max,omitempty"`   // 每 IP 并发连接数上限
	ChinaOnly      bool `json:"china_only,omitempty"`       // 仅允许中国大陆 IP（需先下发 IP 段）
}

func (s *Server) putRoute(w http.ResponseWriter, r *http.Request) {
	var req routeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := validateRoute(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// 部署预检：未装 Nginx 时给出人话提示，而不是后续的 nginx config rejected
	if probe := s.probeNginx(); probe.Missing {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "VPS 未安装 Nginx（或不在常见路径）：请先安装（apt install nginx / yum install nginx），或在「公网反代」页点「一键安装 Nginx」后再部署"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeRoute(req); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	result := map[string]any{"ok": true, "domain": req.Domain, "websocket": req.WebSocket}
	// 部署后回源自检：模拟 Nginx 的回源请求，区分三种状态：
	// 2xx/3xx = 隧道通且服务响应；404 = 反代/frps 正常但无此域名的隧道注册；0/5xx = 回源失败或内网服务异常
	if vhost := req.VhostPort; vhost > 0 {
		code, body := s.probeVhost(vhost, req.Domain)
		result["probe_status"] = code
		result["probe_ok"] = code >= 200 && code < 400
		result["probe_notfound"] = code == 404
		result["probe_body_snip"] = body
	}
	writeJSON(w, http.StatusOK, result)
}

// probeVhost 从 VPS 本机模拟 Nginx 回源：返回 HTTP 状态码与响应片段（限长）。
func (s *Server) probeVhost(port int, host string) (int, string) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	if err != nil {
		return 0, err.Error()
	}
	req.Host = host
	resp, err := client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	return resp.StatusCode, strings.TrimSpace(string(snippet))
}

// routeConf 返回指定域名的当前反代配置内容（查看生成的 nginx 配置）。
func (s *Server) routeConf(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if !validDomain(domain) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid domain"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.routePath(domain))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "该域名尚未部署反代"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domain": domain, "content": string(data)})
}

func (s *Server) deleteRoute(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if !validDomain(domain) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid domain"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.routePath(domain)
	if err := s.clearLimitState(domain); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if err := s.reloadNginx(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "domain": domain})
}

type frpsConfigRequest struct {
	Content string `json:"content"`
}

func (s *Server) putFRPSConfig(w http.ResponseWriter, r *http.Request) {
	var req frpsConfigRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10)).Decode(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "frps config content is required"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	backup := s.cfg.FrpsConfigPath + ".bak"
	if old, err := os.ReadFile(s.cfg.FrpsConfigPath); err == nil {
		if err := atomicWrite(backup, old, 0o600); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := atomicWrite(s.cfg.FrpsConfigPath, []byte(req.Content), 0o600); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": s.cfg.FrpsConfigPath, "restart_required": true})
}

type certificateRequest struct {
	Domain     string `json:"domain"`
	Fullchain  string `json:"fullchain_pem"`
	PrivateKey string `json:"key_pem"`
}

func (s *Server) putCertificate(w http.ResponseWriter, r *http.Request) {
	var req certificateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	if !validDomain(req.Domain) || strings.TrimSpace(req.Fullchain) == "" || strings.TrimSpace(req.PrivateKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "domain and PEM values are required"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.cfg.CertsDir, req.Domain)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if err := atomicWrite(filepath.Join(dir, "fullchain.pem"), []byte(req.Fullchain), 0o600); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if err := atomicWrite(filepath.Join(dir, "privkey.pem"), []byte(req.PrivateKey), 0o600); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "domain": req.Domain, "directory": dir})
}

// nginxInstallResult 一键安装 Nginx 的执行结果。
type nginxInstallResult struct {
	OK      bool   `json:"ok"`
	Manager string `json:"manager,omitempty"`
	Output  string `json:"output,omitempty"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
}

// installNginx 一键安装 Nginx：识别 apt-get/dnf/yum/apk → 非交互安装 → 启动 → 回写探测结果。
// 需要 agent 以 root 运行（systemd 单元默认如此）。
func (s *Server) installNginx(w http.ResponseWriter, r *http.Request) {
	if os.Geteuid() != 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "安装 Nginx 需要 root 权限：请以 root 运行 fonu-agent"})
		return
	}
	if probe := s.probeNginx(); probe.OK {
		writeJSON(w, http.StatusOK, nginxInstallResult{OK: true, Path: probe.Path, Output: "Nginx 已可用，无需安装"})
		return
	}
	manager, installArgs := detectPackageManager()
	if manager == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "未识别到支持的包管理器（apt-get/dnf/yum/apk），请在 VPS 上手动安装 Nginx"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if manager == "apt-get" {
		update := exec.CommandContext(ctx, "apt-get", "update", "-y")
		update.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		_, _ = update.CombinedOutput() // 索引更新失败不阻断安装尝试
	}
	install := exec.CommandContext(ctx, manager, installArgs...)
	install.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := install.CombinedOutput()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, nginxInstallResult{Manager: manager, Output: tailText(string(out), 4000), Error: err.Error()})
		return
	}
	// 启动并设置开机自启：systemd 优先，回退 service 命令（两者失败都忽略，最终以探测结果为准）
	_ = exec.Command("systemctl", "enable", "--now", "nginx").Run()
	_ = exec.Command("service", "nginx", "start").Run()
	s.invalidateNginxProbe()
	probe := s.probeNginx()
	result := nginxInstallResult{OK: probe.OK, Manager: manager, Output: tailText(string(out), 4000), Path: probe.Path, Error: probe.Error}
	if !probe.OK {
		writeJSON(w, http.StatusBadGateway, result)
		return
	}
	// 安装成功后解析并缓存 vhost 目录，省去后续首次部署的探测
	if dir, err := detectNginxConfDir(s.nginxPath()); err == nil {
		s.cfg.NginxConfDir = dir
		_ = os.MkdirAll(dir, 0o755)
		if s.cfg.DataDir != "" {
			_ = os.WriteFile(filepath.Join(s.cfg.DataDir, "nginx_conf_dir"), []byte(dir), 0o644)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

type chinaCIDRRequest struct {
	Content string `json:"content"`
}

// putChinaCIDR 接收 Fonu 下发的中国 IP 段（geo include 格式），写入 geo 变量文件并重载。
func (s *Server) putChinaCIDR(w http.ResponseWriter, r *http.Request) {
	var req chinaCIDRRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "中国 IP 段内容为空"})
		return
	}
	if probe := s.probeNginx(); probe.Missing {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "VPS 未安装 Nginx：请先安装 Nginx 再同步中国 IP 段"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.cfg.NginxConfDir) == "" {
		dir, err := detectNginxConfDir(s.cfg.NginxBinary)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		s.cfg.NginxConfDir = dir
	}
	if err := atomicWrite(chinaListPath(), []byte(content+"\n"), 0o640); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	geo := "# Managed by fonu-agent. Do not edit.\ngeo $fonu_china_client {\n    default 0;\n    include " + chinaListPath() + ";\n}\n"
	if err := atomicWrite(s.chinaGeoFilePath(), []byte(geo), 0o640); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if err := s.validateNginx(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "nginx config rejected: " + err.Error()})
		return
	}
	if err := s.reloadNginx(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": len(content)})
}

func (s *Server) writeRoute(req routeRequest) error {
	// 目录为空时惰性探测（启动时探测失败不阻断，这里重试；systemd 下补 PATH）
	if strings.TrimSpace(s.cfg.NginxConfDir) == "" {
		dir, err := detectNginxConfDir(s.cfg.NginxBinary)
		if err != nil {
			return err
		}
		// putRoute 已持有 s.mu；这里不能再次加锁，否则会发生自锁死。
		s.cfg.NginxConfDir = dir
	}
	certDir := req.CertDir
	if certDir == "" {
		certDir = filepath.Join(s.cfg.CertsDir, req.Domain)
	}
	// 仅中国大陆 IP 依赖已下发的 IP 段（geo 变量在 fonuagent-china.conf 中定义）
	if req.ChinaOnly {
		if _, err := os.Stat(chinaListPath()); err != nil {
			return fmt.Errorf("启用「仅中国大陆 IP」前需先同步中国 IP 段（请在公网反代页触发同步后重试）")
		}
	}
	content := nginxRoute(req, certDir)
	path := s.routePath(req.Domain)
	// 清理历史版本写到其他目录的同域名 Agent 文件：从 nginx -T 的生效清单反向比对，
	// 非当前目录但内容带 Agent 标记的 fonu-<domain>.conf 一律删除（不碰宝塔自己的配置）。
	if nginx := s.nginxPath(); nginx != "" {
		if tOut, tErr := exec.Command(nginx, "-T").CombinedOutput(); tErr == nil {
			for _, line := range strings.Split(string(tOut), "\n") {
				line = strings.TrimSpace(line)
				if value, found := strings.CutPrefix(line, "# configuration file "); found {
					p := strings.TrimSuffix(strings.TrimSpace(value), ":")
					base := filepath.Base(p)
					if p != path && base == "fonu-"+req.Domain+".conf" {
						if data, readErr := os.ReadFile(p); readErr == nil && strings.HasPrefix(string(data), "# Managed by fonu-agent.") {
							_ = os.Remove(p)
						}
					}
				}
			}
		}
	}
	// 先落 zone 文件再写 vhost：nginx -t 需要 zone 已存在
	if err := s.applyLimitState(req); err != nil {
		return err
	}
	if err := atomicWrite(path, []byte(content), 0o640); err != nil {
		return err
	}
	if err := s.validateNginx(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("nginx config rejected: %w", err)
	}
	return s.reloadNginx()
}

func (s *Server) routeNames() []string {
	entries, err := os.ReadDir(s.cfg.NginxConfDir)
	if err != nil {
		return []string{}
	}
	var result []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "fonu-") || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		result = append(result, strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "fonu-"), ".conf"))
	}
	sort.Strings(result)
	return result
}

func (s *Server) routePath(domain string) string {
	return filepath.Join(s.cfg.NginxConfDir, "fonu-"+domain+".conf")
}

func (s *Server) validateNginx() error {
	return runCommand(s.nginxPath(), "-t")
}

func (s *Server) reloadNginx() error {
	return runCommand(s.nginxPath(), "-s", "reload")
}

// nginxPath 返回可用的 nginx 绝对路径：配置的路径可用则用之；否则探测常见安装位置
// （宝塔的 nginx 不在 systemd PATH 里，直接用名字会 exec 失败）。
func (s *Server) nginxPath() string {
	if p := strings.TrimSpace(s.cfg.NginxBinary); p != "" {
		if filepath.IsAbs(p) {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		} else if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	for _, candidate := range []string{"/www/server/nginx/sbin/nginx", "/usr/local/nginx/sbin/nginx", "/usr/sbin/nginx"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return s.cfg.NginxBinary
}

// probeNginx 真实探测 Nginx 可用性：解析可执行文件并执行 nginx -t；结果缓存 30 秒。
func (s *Server) probeNginx() nginxProbe {
	s.nginxStatusMu.Lock()
	defer s.nginxStatusMu.Unlock()
	if !s.nginxStatusAt.IsZero() && time.Since(s.nginxStatusAt) < 30*time.Second {
		return s.nginxStatus
	}
	probe := nginxProbe{}
	path := strings.TrimSpace(s.nginxPath())
	if path == "" {
		probe.Missing = true
		probe.Error = "未找到 nginx 可执行文件（已尝试配置值、常见安装路径与运行中进程）"
	} else {
		probe.Path = path
		probe.Version = nginxVersion(path)
		probe.Running = nginxRunning()
		if out, err := exec.Command(path, "-t").CombinedOutput(); err != nil {
			probe.Error = strings.TrimSpace(string(out))
		} else {
			probe.OK = true
		}
	}
	s.nginxStatus = probe
	s.nginxStatusAt = time.Now()
	return probe
}

// invalidateNginxProbe 清除探测缓存（安装/升级 Nginx 后强制重新探测）。
func (s *Server) invalidateNginxProbe() {
	s.nginxStatusMu.Lock()
	s.nginxStatusAt = time.Time{}
	s.nginxStatusMu.Unlock()
}

// nginxRunning 判断是否有 nginx 进程在运行（扫描 /proc，不依赖端口占用）。
func nginxRunning() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		if data, err := os.ReadFile("/proc/" + entry.Name() + "/comm"); err == nil && strings.TrimSpace(string(data)) == "nginx" {
			return true
		}
	}
	return false
}

// nginxVersion 返回 nginx 版本字符串（如 nginx/1.24.0）；-v 输出走 stderr。
func nginxVersion(path string) string {
	out, _ := exec.Command(path, "-v").CombinedOutput()
	text := strings.TrimSpace(string(out))
	if idx := strings.Index(text, "nginx version: "); idx >= 0 {
		return strings.TrimSpace(text[idx+len("nginx version: "):])
	}
	return text
}

// detectPackageManager 识别可用的包管理器与非交互安装参数。
func detectPackageManager() (string, []string) {
	candidates := []struct {
		bin  string
		args []string
	}{
		{"apt-get", []string{"install", "-y", "nginx"}},
		{"dnf", []string{"install", "-y", "nginx"}},
		{"yum", []string{"install", "-y", "nginx"}},
		{"apk", []string{"add", "--no-cache", "nginx"}},
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate.bin); err == nil && path != "" {
			return candidate.bin, candidate.args
		}
	}
	return "", nil
}

// tailText 截取文本尾部，避免把整个安装日志塞进响应。
func tailText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return "…（已截断）" + value[len(value)-limit:]
}

func runCommand(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func validateRoute(req routeRequest) error {
	if !validDomain(strings.ToLower(strings.TrimSpace(req.Domain))) {
		return errors.New("invalid domain")
	}
	if strings.TrimSpace(req.Upstream) == "" || strings.ContainsAny(req.Upstream, " \t\r\n;'\"") {
		return errors.New("invalid upstream")
	}
	if req.TLS && strings.TrimSpace(req.CertDir) != "" && !filepath.IsAbs(req.CertDir) {
		return errors.New("certificate directory must be absolute")
	}
	return nil
}

func validDomain(value string) bool {
	if len(value) == 0 || len(value) > 253 || strings.Contains(value, "..") {
		return false
	}
	return domainPattern.MatchString(value)
}

func nginxRoute(req routeRequest, certDir string) string {
	var b strings.Builder
	b.WriteString("# Managed by fonu-agent. Do not edit.\n")
	// HTTPSDisabled 显式关闭 HTTPS：即使证书已推送也不监听 443（字段缺省 = 沿用现有行为）
	tlsEnabled := req.TLS && !req.HTTPSDisabled
	if req.RedirectTLS && tlsEnabled {
		// 80 端口只做 301 跳转，正常代理由 443 TLS server 承担
		fmt.Fprintf(&b, "server {\n    listen 80;\n    server_name %s;\n    return 301 https://$host$request_uri;\n}\n\n", req.Domain)
		fmt.Fprintf(&b, "server {\n    listen 443 ssl;\n    server_name %s;\n", req.Domain)
		b.WriteString(sslDirectives(certDir, req.TLS13Only))
	} else {
		fmt.Fprintf(&b, "server {\n    listen 80;\n    server_name %s;\n", req.Domain)
		if tlsEnabled {
			b.WriteString("    listen 443 ssl;\n")
			b.WriteString(sslDirectives(certDir, req.TLS13Only))
		}
	}
	// 上传大小与代理读超时：按规则覆盖 Nginx 默认值（大文件上传 / 长轮询 / 流媒体必需）
	if size := strings.TrimSpace(req.ClientMaxBodySize); size != "" {
		fmt.Fprintf(&b, "    client_max_body_size %s;\n", size)
	}
	if timeout := strings.TrimSpace(req.ProxyReadTimeout); timeout != "" {
		fmt.Fprintf(&b, "    proxy_read_timeout %s;\n", timeout)
	}
	// 安全响应头：HSTS 仅在 HTTPS 实际启用时输出
	if req.SecurityHeaders {
		b.WriteString("    add_header X-Content-Type-Options \"nosniff\" always;\n")
		b.WriteString("    add_header X-Frame-Options \"SAMEORIGIN\" always;\n")
		b.WriteString("    add_header Referrer-Policy \"strict-origin-when-cross-origin\" always;\n")
		if tlsEnabled {
			b.WriteString("    add_header Strict-Transport-Security \"max-age=31536000; includeSubDomains\" always;\n")
		}
	}
	// 限流/连接数的 zone 定义在 fonuagent-limits.conf（http 上下文），此处只声明状态码
	if req.RateLimitRate > 0 {
		b.WriteString("    limit_req_status 429;\n")
	}
	if req.ConnLimitMax > 0 {
		b.WriteString("    limit_conn_status 503;\n")
	}
	if req.WebSocket {
		b.WriteString("    proxy_http_version 1.1;\n")
		b.WriteString("    proxy_set_header Upgrade $http_upgrade;\n")
		b.WriteString("    proxy_set_header Connection \"upgrade\";\n")
	}
	// IP 访问控制：白名单（allow + deny all）与黑名单（deny）可同时生效；
	// Nginx 按出现顺序匹配，因此 allow 在前、deny 在后。
	var acl strings.Builder
	for _, ip := range req.AllowIPs {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			fmt.Fprintf(&acl, "        allow %s;\n", ip)
		}
	}
	for _, ip := range req.DenyIPs {
		if ip = strings.TrimSpace(ip); ip != "" {
			fmt.Fprintf(&acl, "        deny %s;\n", ip)
		}
	}
	if len(req.AllowIPs) > 0 {
		acl.WriteString("        deny all;\n")
	}
	// Basic Auth：密码优先用规则表单值（自定义），留空则首次自动生成并复用（htpasswd 格式）
	authLine := ""
	var authNote string
	if user := strings.TrimSpace(req.BasicAuthUser); user != "" {
		if password := strings.TrimSpace(req.BasicAuthPassword); password != "" {
			if err := writeBasicAuthSecret(req.Domain, user, password); err == nil {
				authLine = fmt.Sprintf("        auth_basic \"Fonu Protected\";\n        auth_basic_user_file %s;\n", basicAuthFilePath(req.Domain))
			}
		} else if generated, err := ensureBasicAuthSecret(req.Domain, user); err == nil {
			authLine = fmt.Sprintf("        auth_basic \"Fonu Protected\";\n        auth_basic_user_file %s;\n", basicAuthFilePath(req.Domain))
			if generated != "" {
				authNote = fmt.Sprintf("# Basic Auth: user=%s password=%s\n", user, generated)
			}
		}
	}
	b.WriteString(authNote)
	// 限流 / 连接数 / 仅中国大陆 IP 的 location 级指令（zone 与 geo 变量在 http 上下文定义）
	var limitText strings.Builder
	if req.RateLimitRate > 0 {
		zone := zoneKey(req.Domain)
		if req.RateLimitBurst > 0 {
			fmt.Fprintf(&limitText, "        limit_req zone=%s_req burst=%d nodelay;\n", zone, req.RateLimitBurst)
		} else {
			fmt.Fprintf(&limitText, "        limit_req zone=%s_req nodelay;\n", zone)
		}
	}
	if req.ConnLimitMax > 0 {
		fmt.Fprintf(&limitText, "        limit_conn %s_conn %d;\n", zoneKey(req.Domain), req.ConnLimitMax)
	}
	if req.ChinaOnly {
		limitText.WriteString("        if ($fonu_china_client = 0) { return 403; }\n")
	}
	aclText := acl.String()
	fmt.Fprintf(&b, "    location / {\n%s%s%s        proxy_pass http://%s;\n        proxy_set_header Host $host;\n        proxy_set_header X-Real-IP $remote_addr;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n        proxy_http_version 1.1;\n    }\n}\n", aclText, authLine, limitText.String(), req.Upstream)
	return b.String()
}

// agentStateDir 存放 Agent 的持久状态（htpasswd、geo IP 段、限流参数）。
const agentStateDir = "/var/lib/fonu-agent"

// zoneKey 把域名转成合法的 nginx zone 名（仅保留小写字母与数字，其余换下划线）。
func zoneKey(domain string) string {
	var b strings.Builder
	b.WriteString("fonu_")
	for _, r := range strings.ToLower(domain) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// limitsFilePath 返回 zone 定义文件（放在 vhost 目录，顶层指令即 http 上下文）。
func (s *Server) limitsFilePath() string {
	return filepath.Join(s.cfg.NginxConfDir, "fonuagent-limits.conf")
}

// chinaGeoFilePath 返回 geo 变量定义文件（同样位于 http 上下文）。
func (s *Server) chinaGeoFilePath() string {
	return filepath.Join(s.cfg.NginxConfDir, "fonuagent-china.conf")
}

// chinaListPath 返回中国 IP 段文件（geo include 格式：<cidr> 1;）。
func chinaListPath() string {
	return filepath.Join(agentStateDir, "geo", "china.conf")
}

func limitsStatePath() string {
	return filepath.Join(agentStateDir, "route-limits.json")
}

// routeLimit 单条路由的限流参数。
type routeLimit struct {
	Rate  int `json:"rate,omitempty"`
	Burst int `json:"burst,omitempty"`
	Conn  int `json:"conn,omitempty"`
}

func readLimitState() map[string]routeLimit {
	state := map[string]routeLimit{}
	data, err := os.ReadFile(limitsStatePath())
	if err != nil {
		return state
	}
	_ = json.Unmarshal(data, &state)
	return state
}

// writeLimitState 落盘限流参数并重生成 zone 定义文件；无限流时删除该文件。
func (s *Server) writeLimitState(state map[string]routeLimit) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := atomicWrite(limitsStatePath(), data, 0o640); err != nil {
		return err
	}
	path := s.limitsFilePath()
	if len(state) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	var b strings.Builder
	b.WriteString("# Managed by fonu-agent. Do not edit.\n")
	domains := make([]string, 0, len(state))
	for domain := range state {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	for _, domain := range domains {
		item := state[domain]
		key := zoneKey(domain)
		if item.Rate > 0 {
			fmt.Fprintf(&b, "limit_req_zone $binary_remote_addr zone=%s_req:10m rate=%dr/s;\n", key, item.Rate)
		}
		if item.Conn > 0 {
			fmt.Fprintf(&b, "limit_conn_zone $binary_remote_addr zone=%s_conn:10m;\n", key)
		}
	}
	return atomicWrite(path, []byte(b.String()), 0o640)
}

// applyLimitState 记录本次路由的限流参数并刷新 zone 文件（必须在写 vhost 之前完成）。
func (s *Server) applyLimitState(req routeRequest) error {
	state := readLimitState()
	if req.RateLimitRate > 0 || req.ConnLimitMax > 0 {
		state[req.Domain] = routeLimit{Rate: req.RateLimitRate, Burst: req.RateLimitBurst, Conn: req.ConnLimitMax}
	} else {
		delete(state, req.Domain)
	}
	return s.writeLimitState(state)
}

// clearLimitState 删除该域名的限流参数并刷新 zone 文件。
func (s *Server) clearLimitState(domain string) error {
	state := readLimitState()
	if _, ok := state[domain]; !ok {
		return nil
	}
	delete(state, domain)
	return s.writeLimitState(state)
}

// sslDirectives 生成证书与 TLS 协议指令；tls13Only 时只允许 TLS 1.3。
func sslDirectives(certDir string, tls13Only bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "    ssl_certificate %s;\n    ssl_certificate_key %s;\n", filepath.Join(certDir, "fullchain.pem"), filepath.Join(certDir, "privkey.pem"))
	if tls13Only {
		b.WriteString("    ssl_protocols TLSv1.3;\n")
	}
	return b.String()
}

// writeBasicAuthSecret 用规则表单提供的密码写入 htpasswd（自定义密码场景）。
func writeBasicAuthSecret(domain, user, password string) error {
	out, err := exec.Command("openssl", "passwd", "-apr1", password).CombinedOutput()
	if err != nil {
		return fmt.Errorf("生成 htpasswd 失败: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return atomicWrite(basicAuthFilePath(domain), []byte(user+":"+strings.TrimSpace(string(out))+"\n"), 0o640)
}

// basicAuthFilePath 返回指定域名的 htpasswd 文件路径（与反代配置同目录管理）。
func basicAuthFilePath(domain string) string {
	return filepath.Join("/var/lib/fonu-agent", "basicauth", domain+".htpasswd")
}

// ensureBasicAuthSecret 读取已有 Basic Auth 密码；不存在则生成随机密码并写入 htpasswd 文件。
// 返回明文密码（只在生成配置时写进注释，供用户查看一次）。
func ensureBasicAuthSecret(domain, user string) (string, error) {
	path := basicAuthFilePath(domain)
	if data, err := os.ReadFile(path); err == nil {
		// 已有凭据：格式 user:$apr1$...，无法反解；返回空表示沿用旧密码
		if len(data) > 0 {
			return "", nil
		}
	}
	password := randomToken()[:16]
	// 用 openssl 生成 apr1 哈希（Nginx 原生支持）；系统无 openssl 时退化为 crypt 失败报错
	out, err := exec.Command("openssl", "passwd", "-apr1", password).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("生成 htpasswd 失败: %w: %s", err, strings.TrimSpace(string(out)))
	}
	hash := strings.TrimSpace(string(out))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	if err := atomicWrite(path, []byte(user+":"+hash+"\n"), 0o640); err != nil {
		return "", err
	}
	return password, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fonu-agent-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func randomToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(buf)
}
