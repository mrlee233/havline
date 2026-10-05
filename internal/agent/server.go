package agent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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

	"github.com/havline/havline/internal/fsutil"
	"github.com/havline/havline/internal/nginxtmpl"
	"github.com/havline/havline/internal/sysinfo"
)

const AgentVersion = "0.15.1"

// Config 是 havline-agent 的运行配置。agent 只在 VPS 本地管理自己的 Nginx 片段、证书和 frps 配置。
type Config struct {
	ListenAddr     string
	AllowRemote    bool
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
	cfg            Config
	listenLoopback bool
	mu             sync.Mutex
	// metrics 采样缓存：CPU 占用率需要两次采样，间隔内的重复请求直接复用上次结果
	metricsMu     sync.Mutex
	metricsAt     time.Time
	metricsSample sysinfo.CPUSample
	metricsCached map[string]any
	// nginx 可用性探测缓存：status 轮询频繁，避免每次都起 nginx -t 进程
	nginxStatusMu sync.Mutex
	nginxStatusAt time.Time
	nginxStatus   nginxProbe
	// 测试注入钩子；生产环境为 nil，走真实 nginx 校验与重载
	validateNginxFn func() error
	reloadNginxFn   func() error
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
		cfg.ListenAddr = "127.0.0.1:7700"
	}
	loopback, err := validateListenAddr(cfg.ListenAddr, cfg.AllowRemote)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.NginxConfDir) == "" {
		// 目录探测失败不阻断启动：agent 照常提供状态接口，路由写入时重试探测（systemd PATH 下可能找不到 nginx，写入前会补全路径）
		cfg.NginxConfDir = ""
	}
	if strings.TrimSpace(cfg.NginxBinary) == "" {
		cfg.NginxBinary = "nginx"
	}
	if strings.TrimSpace(cfg.CertsDir) == "" {
		cfg.CertsDir = "/var/lib/havline-agent/certs"
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
		cfg.DataDir = "/var/lib/havline-agent"
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
	server := &Server{cfg: cfg, listenLoopback: loopback}
	// 启动后台预解析目录（不阻断监听）；每次启动都会用最新探测逻辑重新解析并覆盖落盘缓存，
	// 历史错误目录的缓存会被自动修正。
	server.preloadNginxConfDir()
	return server, nil
}

func validateListenAddr(addr string, allowRemote bool) (bool, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false, fmt.Errorf("Agent 监听地址无效: %w", err)
	}
	host = strings.Trim(host, "[]")
	loopback := strings.EqualFold(host, "localhost")
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
	}
	if !loopback && !allowRemote {
		return false, fmt.Errorf("检测到 Agent 将监听非回环地址 %s；默认安全模式只监听 127.0.0.1:7700。如确认需要远程监听，请设置 HAVLINE_AGENT_ALLOW_REMOTE=1", addr)
	}
	return loopback, nil
}

func listenScope(loopback bool) string {
	if loopback {
		return "loopback"
	}
	return "public"
}

func transportSecurity(loopback bool) string {
	if loopback {
		return "loopback"
	}
	return "plaintext-http"
}

type agentRoute struct {
	Method  string
	Pattern string
	Handler http.HandlerFunc
}

// agentRoutes 集中声明 Agent API；Handler 统一包 auth，测试据此遍历验证无 Token 必须 401。
func agentRoutes(s *Server) []agentRoute {
	return []agentRoute{
		{http.MethodGet, "/api/v1/ping", s.ping},
		{http.MethodGet, "/api/v1/status", s.status},
		{http.MethodGet, "/api/v1/routes", s.routes},
		{http.MethodPut, "/api/v1/routes", s.putRoute},
		{http.MethodGet, "/api/v1/routes/{domain}/conf", s.routeConf},
		{http.MethodGet, "/api/v1/routes/{domain}/versions", s.routeVersions},
		{http.MethodGet, "/api/v1/routes/{domain}/versions/{name}", s.routeVersion},
		{http.MethodPost, "/api/v1/routes/{domain}/versions/{name}/rollback", s.rollbackRoute},
		{http.MethodPut, "/api/v1/china-cidr", s.putChinaCIDR},
		{http.MethodPost, "/api/v1/nginx/install", s.installNginx},
		{http.MethodPut, "/api/v1/transport/https", s.putHTTPSProxy},
		{http.MethodGet, "/api/v1/transport/https", s.getHTTPSProxy},
		{http.MethodDelete, "/api/v1/transport/https", s.deleteHTTPSProxy},
		{http.MethodGet, "/api/v1/firewall", s.firewallStatus},
		{http.MethodPost, "/api/v1/firewall/allow", s.firewallAllow},
		{http.MethodDelete, "/api/v1/routes/{domain}", s.deleteRoute},
		{http.MethodPut, "/api/v1/frps/config", s.putFRPSConfig},
		{http.MethodPost, "/api/v1/frps/install", s.installFRPS},
		{http.MethodPost, "/api/v1/frps/action", s.frpsAction},
		{http.MethodPost, "/api/v1/certs", s.putCertificate},
		{http.MethodGet, "/api/v1/metrics", s.metrics},
		{http.MethodGet, "/api/v1/logs/nginx", s.nginxLogs},
		{http.MethodGet, "/api/v1/frps/config", s.frpsConfig},
		{http.MethodGet, "/api/v1/certs", s.certs},
		{http.MethodPost, "/api/v1/nginx/reload", s.nginxReload},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, route := range agentRoutes(s) {
		mux.HandleFunc(route.Method+" "+route.Pattern, s.auth(route.Handler))
	}
	return mux
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	server := &http.Server{Addr: s.cfg.ListenAddr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, IdleTimeout: 2 * time.Minute}
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
		token := bearerToken(r)
		if token == "" || !secureTokenEqual(token, s.cfg.Token) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// bearerToken 解析 Authorization: Bearer <token>；格式不符时返回空串。
func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// secureTokenEqual 使用常量时间比较，避免通过响应时间推测 Token 内容。
func secureTokenEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (s *Server) ping(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "havline-agent"})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	ports := map[string]bool{"80": portListening(80), "443": portListening(443), "7000": portListening(7000), "8080": portListening(8080)}
	nginxProbe := s.probeNginx()
	frpsActive := exec.Command("systemctl", "is-active", "--quiet", "frps").Run() == nil
	httpsState, httpsConfigured := readHTTPSProxyState(s.cfg.DataDir)
	writeJSON(w, http.StatusOK, map[string]any{
		"service":                "havline-agent",
		"agent_version":          AgentVersion,
		"listen_addr":            s.cfg.ListenAddr,
		"listen_scope":           listenScope(s.listenLoopback),
		"remote_access_enabled":  s.cfg.AllowRemote,
		"transport_security":     transportSecurity(s.listenLoopback),
		"https_proxy_configured": httpsConfigured,
		"https_proxy_port":       httpsState.Port,
		"nginx_binary":           s.cfg.NginxBinary,
		"nginx_conf_dir":         s.cfg.NginxConfDir,
		"nginx_ok":               nginxProbe.OK,
		"nginx_missing":          nginxProbe.Missing,
		"nginx_running":          nginxProbe.Running,
		"nginx_path":             nginxProbe.Path,
		"nginx_version":          nginxProbe.Version,
		"nginx_error":            nginxProbe.Error,
		"frps_binary":            s.cfg.FrpsBinary,
		"frps_config_path":       s.cfg.FrpsConfigPath,
		"ports":                  ports,
		"frps_active":            frpsActive,
		"frps_vhost_hint":        "curl -H 'Host: 你的域名' http://127.0.0.1:8080 可测试 frps vhost 是否通",
		"frps_unit_exec":         strings.TrimSpace(execCommandOutput("systemctl", "show", "frps", "-p", "ExecStart", "--value")),
		"frps_enabled":           exec.Command("systemctl", "is-enabled", "--quiet", "frps").Run() == nil,
		"frps_version":           strings.TrimSpace(frpsVersion(s.cfg.FrpsBinary)),
		"frps_log_tail":          frpsLogTailDesc(),
		"dashboard_addr":         strings.TrimSpace(dashboardAddr(s.cfg.FrpsConfigPath)),
		"routes":                 s.routeNames(),
		"checked_at":             time.Now().UTC().Format(time.RFC3339),
	})
}

type frpsInstallRequest struct {
	Version string `json:"version"`
	Proxy   string `json:"proxy,omitempty"`
}

// frps 安装包只允许从官方 release 下载，或经这几个加速前缀。白名单与 internal/frp/runtime.go 的
// downloadProxyOptions 同源（agent 不依赖应用侧包，这里保留一份副本，改一处需同步另一处）。
var frpsDownloadProxies = map[string]struct{}{
	"":                          {},
	"https://gh-proxy.org/":     {},
	"https://v4.gh-proxy.org/":  {},
	"https://cdn.gh-proxy.org/": {},
}

const frpsReleaseBaseURL = "https://github.com/fatedier/frp/releases/download/"

// fetchChecksumForCandidates 依次尝试官方校验文件名的候选。
// frp 的校验文件一直叫 frp_sha256_checksums.txt（不带版本号），旧名字保留兜底，
// 避开上游改名后整个安装流程直接 404。
func fetchChecksumForCandidates(releaseURL, version, asset string) (string, error) {
	candidates := []string{
		"frp_sha256_checksums.txt",
		"frp_" + version + "_sha256sums.txt",
	}
	var lastErr error
	for _, name := range candidates {
		sum, err := fetchChecksumFor(releaseURL+name, asset)
		if err == nil {
			return sum, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("%v（已尝试：%s）", lastErr, strings.Join(candidates, "、"))
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
	proxy := strings.TrimSpace(req.Proxy)
	if _, ok := frpsDownloadProxies[proxy]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "不支持的下载加速代理"})
		return
	}
	asset := fmt.Sprintf("frp_%s_linux_%s.tar.gz", version, arch)
	releaseURL := proxy + frpsReleaseBaseURL + "v" + version + "/"
	downloadURL := releaseURL + asset
	workDir := filepath.Join(s.cfg.DataDir, "tmp-frps-"+version)
	_ = os.RemoveAll(workDir)
	defer os.RemoveAll(workDir)
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	// 先取官方校验文件，再下载安装包并按 SHA-256 校验：避免装上来路不明的 frps 可执行文件
	wantSum, err := fetchChecksumForCandidates(releaseURL, version, asset)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "获取 frps 官方校验信息失败: " + err.Error()})
		return
	}
	tarball := filepath.Join(workDir, "frp.tar.gz")
	if err := downloadToFile(downloadURL, tarball); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "下载 frps 失败: " + err.Error()})
		return
	}
	gotSum, err := fileSHA256(tarball)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "校验 frps 安装包失败: " + err.Error()})
		return
	}
	if !strings.EqualFold(gotSum, wantSum) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "frps 安装包 SHA-256 校验失败"})
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
	notes := []string{"frps 二进制已安装: " + installPath, "安装包 SHA-256 校验通过"}
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
		unit := fmt.Sprintf("[Unit]\nDescription=frps service (managed by havline-agent)\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nExecStart=%s -c %s\nRestart=on-failure\nRestartSec=3\nStandardOutput=append:%s\nStandardError=append:%s\n\n[Install]\nWantedBy=multi-user.target\n", installPath, s.cfg.FrpsConfigPath, frpsSystemdLogPath, frpsSystemdLogPath)
		if err := atomicWrite(unitPath, []byte(unit), 0o644); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "写入 frps.service 失败: " + err.Error()})
			return
		}
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}
	// nohup 手工跑的 frps 让位给 systemd；已在 systemd 下运行则重启以加载新二进制
	active := exec.Command("systemctl", "is-active", "--quiet", "frps").Run() == nil
	if !active && exec.Command("pgrep", "-x", "frps").Run() == nil {
		_ = exec.Command("pkill", "-x", "frps").Run()
		time.Sleep(time.Second)
		notes = append(notes, "已停止 nohup 方式运行的旧 frps 进程")
	}
	args := []string{"restart", "frps"}
	label := "已重启以加载新版本"
	if !active {
		args = []string{"enable", "--now", "frps"}
		label = "已启动"
	}
	// 启动失败必须如实回报：之前忽略 err 后无条件写「已启动」，运维会以为服务在跑
	if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "version": version, "binary": installPath, "notes": notes,
			"error": fmt.Sprintf("frps 二进制已安装，但 systemctl %s 失败：%v：%s", strings.Join(args, " "), err, tailText(string(out), 400)),
		})
		return
	}
	notes = append(notes, "frps.service "+label)
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
	notes := make([]string, 0, 2)
	if req.Action == "start" || req.Action == "restart" {
		if note := cleanupOrphanFRPS(); note != "" {
			notes = append(notes, note)
		}
	}
	if output, err := command.CombinedOutput(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("systemctl %s frps 失败: %s", req.Action, strings.TrimSpace(string(output)))})
		return
	}
	time.Sleep(800 * time.Millisecond) // 给 frps 启动留出窗口，失败时才能在 is-active/journal 里看到
	active := exec.Command("systemctl", "is-active", "--quiet", "frps").Run() == nil
	result := map[string]any{"ok": true, "action": req.Action, "frps_active": active, "notes": notes}
	if !active {
		// 启动失败时把 frps 的失败原因带回去，而不是只说“启动成功”
		result["ok"] = false
		result["error"] = s.frpsFailureDetail(req.Action)
	}
	writeJSON(w, http.StatusOK, result)
}

const frpsSystemdLogPath = "/var/log/frps-systemd.log"

// cleanupOrphanFRPS 在 systemd 服务未运行时清理手工启动的 frps，
// 避免旧进程占用 7000/vhost 端口后导致 systemd 启动即退出。
func cleanupOrphanFRPS() string {
	if exec.Command("systemctl", "is-active", "--quiet", "frps").Run() == nil {
		return ""
	}
	if exec.Command("pgrep", "-x", "frps").Run() != nil {
		return ""
	}
	if err := exec.Command("pkill", "-x", "frps").Run(); err != nil {
		return "检测到 systemd 之外运行的 frps 进程，但停止失败"
	}
	time.Sleep(300 * time.Millisecond)
	return "检测到 systemd 之外运行的 frps 进程，已先停止旧进程"
}

// frpsFailureDetail 汇总 systemd 状态、输出日志与直接启动结果，
// 避免 journald 不可用时只剩 “No journal files were found”。
func (s *Server) frpsFailureDetail(action string) string {
	lines := []string{fmt.Sprintf("systemctl %s 已执行但 frps 未进入运行状态", action)}
	if out := strings.TrimSpace(execCommandOutput("systemctl", "show", "frps",
		"-p", "ActiveState", "-p", "SubState", "-p", "Result", "-p", "ExecMainCode", "-p", "ExecMainStatus")); out != "" {
		lines = append(lines, "systemd 状态:\n"+out)
	}
	if out := strings.TrimSpace(execCommandOutput("systemctl", "status", "frps", "--no-pager", "-l")); out != "" {
		lines = append(lines, "systemctl status:\n"+tailText(out, 2000))
	}
	if out := strings.TrimSpace(journalTail("frps", frpsLogTailLines)); out != "" && !strings.Contains(out, "No journal files were found") {
		lines = append(lines, "journal 日志:\n"+out)
	}
	if out := strings.TrimSpace(readFRPSSystemdLog()); out != "" {
		lines = append(lines, "frps 输出:\n"+out)
	}
	if out := s.probeFRPSStartup(); out != "" {
		lines = append(lines, "直接启动诊断:\n"+out)
	}
	return strings.Join(lines, "\n\n")
}

// probeFRPSStartup 在无 systemd 日志时直接运行 frps 2 秒：
// 配置错误会立即返回；配置正常时会打印启动成功后被 timeout 结束。
func (s *Server) probeFRPSStartup() string {
	if exec.Command("pgrep", "-x", "frps").Run() == nil {
		return "已有 frps 进程在运行，跳过直接启动诊断"
	}
	out, err := exec.Command("timeout", "2s", s.cfg.FrpsBinary, "-c", s.cfg.FrpsConfigPath).CombinedOutput()
	text := strings.TrimSpace(string(out))
	hint := frpsStartupHint(text)
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return tailText(text+hint, 2400)
	}
	if text == "" {
		return "frps 直接运行 2 秒未报错，可能已正常启动后被超时结束；请检查 systemd 单元和端口占用"
	}
	return tailText(text+hint, 2400)
}

func frpsStartupHint(output string) string {
	if !strings.Contains(output, "address already in use") {
		return ""
	}
	return "\n\n建议：80/443 通常由 VPS 上的 Nginx 占用；请把 frps 的 vhostHTTPPort/vhostHTTPSPort 改为 8080/8443，保存后重新下发 frps 配置并重启 frps。"
}

func frpsLogTailDesc() string {
	if out := strings.TrimSpace(journalTailDesc("frps", frpsLogTailLines)); out != "" && !strings.Contains(out, "No journal files were found") {
		return out
	}
	return readFRPSSystemdLog()
}

func readFRPSSystemdLog() string {
	data, err := os.ReadFile(frpsSystemdLogPath)
	if err != nil {
		return ""
	}
	return tailText(string(data), 8000)
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

// fetchChecksumFor 取官方 sha256sums.txt 中 asset 对应的校验值（格式：<sha256>  <文件名>）。
func fetchChecksumFor(url, asset string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("校验文件中未找到 %s", asset)
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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

// dashboardAddr 已废弃：面板地址改由 Havline 前端从服务端表单的「frps 管理接口」字段拼出，agent 不再解析 frps.toml。
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

// frpsLogTailLines 服务端详情里展示的 frps 日志行数
const frpsLogTailLines = 20

// journalLinePattern 匹配 short-iso 的日志首段：时间戳 + 主机名 + 其余（进程与消息）
var journalLinePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}:\d{2})(?:[+-]\d{2}:?\d{2}|Z)\s+\S+\s+(.*)$`)

// formatJournalLine 把 journalctl 的行重写成「2026-09-22 18:43:34 进程: 消息」。
// short-iso 的 T 分隔与 +08:00 偏移看着费力，主机名在单机视图里每行都一样（纯噪声），
// 认不出来格式的行原样返回，不丢内容。
func formatJournalLine(line string) string {
	matched := journalLinePattern.FindStringSubmatch(line)
	if matched == nil {
		return line
	}
	return matched[1] + " " + matched[2] + " " + matched[3]
}

// formatJournalLog 逐行重写整段日志
func formatJournalLog(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	rows := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, row := range rows {
		rows[i] = formatJournalLine(row)
	}
	return strings.Join(rows, "\n")
}

// journalTailDesc 取 unit 的最近 lines 行系统日志，按「最新在前」返回。
// 用 short-iso 时间戳：journalctl 默认格式只有 "Sep 25 13:20:04"，跨天排查时分不清是哪一天。
func journalTailDesc(unit string, lines int) string {
	if lines <= 0 {
		return ""
	}
	output, err := exec.Command("journalctl", "-u", unit, "-n", strconv.Itoa(lines), "-o", "short-iso", "--no-pager").CombinedOutput()
	if err != nil {
		return ""
	}
	if strings.TrimSpace(string(output)) == "" {
		return ""
	}
	rows := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	// journalctl 输出是「旧 → 新」，倒过来让最先看到的是最新一条
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	for i, row := range rows {
		rows[i] = formatJournalLine(row)
	}
	return strings.Join(rows, "\n")
}

func journalTail(unit string, lines int) string {
	output, err := exec.Command("journalctl", "-u", unit, "-n", strconv.Itoa(lines), "-o", "short-iso", "--no-pager").CombinedOutput()
	if err != nil {
		return ""
	}
	return formatJournalLog(string(output))
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
		return "", fmt.Errorf("未找到可用的 nginx 可执行文件；请先在 VPS 安装 Nginx（apt install nginx / yum install nginx），或用 HAVLINE_AGENT_NGINX_BIN 显式指定路径")
	}
	// 旧版 Agent 可能把 <配置>.versions 目录留在 include 范围内，先按常见目录清理，避免 nginx -T 直接失败。
	for _, dir := range []string{"/etc/nginx/sites-enabled", "/etc/nginx/conf.d", "/etc/nginx"} {
		migrateLegacyVersionDirs(dir)
	}
	out, err := exec.Command(nginx, "-T").CombinedOutput()
	if err != nil && migrateVersionDirsFromNginxOutput(string(out)) {
		out, err = exec.Command(nginx, "-T").CombinedOutput()
	}
	if err != nil {
		return "", fmt.Errorf("nginx -T 执行失败: %w", err)
	}
	loadedDirs := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if value, found := strings.CutPrefix(line, "# configuration file "); found {
			p := strings.TrimSuffix(strings.TrimSpace(value), ":")
			// 不要求 .conf 后缀：Debian/Ubuntu 的 sites-enabled/default 本身不带后缀，
			// 而 nginx.conf 里是 include /etc/nginx/sites-enabled/*（也无后缀限制）；
			// 只认 .conf 会把这个真正该放 vhost 的目录漏掉，只剩主配置目录可试，
			// 而主配置目录里的新文件并不被 include（表现为「均无法放置完整 server 配置」）。
			loadedDirs[filepath.Dir(p)] = true
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
	var lastReason string
	for _, dir := range dirs {
		migrateLegacyVersionDirs(dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			lastReason = fmt.Sprintf("%s: %v", dir, err)
			continue
		}
		probe := filepath.Join(dir, "havline-probe.conf")
		probeBody := "# Managed by havline-agent. Do not edit.\nserver {\n    listen 127.0.0.1:1;\n    server_name havline-probe.invalid;\n}\n"
		if err := atomicWrite(probe, []byte(probeBody), 0o644); err != nil {
			lastReason = fmt.Sprintf("%s: %v", dir, err)
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
			lastReason = fmt.Sprintf("%s: 目录未被 include 加载（nginx -T 输出中无探针文件）", dir)
			_ = os.Remove(probe)
			continue
		}
		// 该目录不接受完整 server 块（片段目录）：删除 probe 文件。
		// 若 -t 失败信息指向历史残留的 havline-*.conf，也一并清理（仅限 Agent 生成的文件）。
		_ = os.Remove(probe)
		if tOut, tErr := exec.Command(nginx, "-t").CombinedOutput(); tErr != nil {
			lastReason = fmt.Sprintf("%s: nginx -t 失败：%s", dir, tailText(string(tOut), 300))
			cleanupLegacyHavlineConf(string(tOut))
			if err := runCommand(nginx, "-t"); err == nil {
				continue // 清理残留后配置恢复可用，换下一个目录
			}
		} else {
			lastReason = fmt.Sprintf("%s: 探针写入后 nginx -t 未通过", dir)
		}
	}
	if lastReason == "" {
		lastReason = "没有可尝试的目录"
	}
	return "", fmt.Errorf("在 %d 个被加载目录中均无法放置完整 server 配置（已尝试: %s）；最后一次失败原因：%s；也可用 HAVLINE_AGENT_NGINX_CONF_DIR 显式指定（Debian/Ubuntu 通常是 /etc/nginx/conf.d 或 /etc/nginx/sites-enabled）", len(dirs), strings.Join(dirs, ", "), lastReason)
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

// cleanupLegacyHavlineConf 根据 nginx -t 的报错文本清理 Agent 历史生成的坏文件（仅限 havline- 前缀）。
func cleanupLegacyHavlineConf(nginxTestOutput string) {
	for _, line := range strings.Split(nginxTestOutput, "\n") {
		idx := strings.Index(line, "havline-")
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
		if data, err := os.ReadFile(path); err == nil && strings.HasPrefix(string(data), "# Managed by havline-agent.") {
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
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "havline-") || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		domain := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "havline-"), ".conf")
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
	VhostPort     int      `json:"vhost_port,omitempty"`      // 回源自检用：Havline 传入本次部署的 frps vhost 端口
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
	// Basic Auth 密码留空时首次自动生成：生成的明文只在本次响应里回一次，
	// 不再写进配置文件注释（否则「查看配置」会永久暴露密码）。已有凭据时返回空串。
	generatedPassword := ""
	if user := strings.TrimSpace(req.BasicAuthUser); user != "" && strings.TrimSpace(req.BasicAuthPassword) == "" {
		if generated, err := ensureBasicAuthSecret(req.Domain, user); err == nil {
			generatedPassword = generated
		}
	}
	if err := s.writeRoute(req); err != nil {
		if generatedPassword != "" {
			// 部署失败就丢弃刚生成的凭据，否则重试时文件已存在、密码再也拿不到
			_ = os.Remove(basicAuthFilePath(req.Domain))
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	result := map[string]any{"ok": true, "domain": req.Domain, "websocket": req.WebSocket}
	if generatedPassword != "" {
		result["basic_auth_user"] = strings.TrimSpace(req.BasicAuthUser)
		result["basic_auth_password"] = generatedPassword
	}
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
	// 版本历史跟着 vhost 一起清掉，避免同域名重新部署后看到上一轮部署的旧版本
	_ = os.RemoveAll(fsutil.VersionsDir(path))
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
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "安装 Nginx 需要 root 权限：请以 root 运行 havline-agent"})
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

// putChinaCIDR 接收 Havline 下发的中国 IP 段（geo include 格式），写入 geo 变量文件并重载。
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
	geo := "# Managed by havline-agent. Do not edit.\ngeo $havline_china_client {\n    default 0;\n    include " + chinaListPath() + ";\n}\n"
	if err := atomicWrite(s.chinaGeoFilePath(), []byte(geo), 0o640); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if err := s.validateRouteConfig(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "nginx config rejected: " + err.Error()})
		return
	}
	if err := s.reloadRouteConfig(); err != nil {
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
	// 仅中国大陆 IP 依赖已下发的 IP 段（geo 变量在 havlineagent-china.conf 中定义）
	if req.ChinaOnly {
		if _, err := os.Stat(chinaListPath()); err != nil {
			return fmt.Errorf("启用「仅中国大陆 IP」前需先同步中国 IP 段（请在公网反代页触发同步后重试）")
		}
	}
	content := nginxRoute(req, certDir)
	path := s.routePath(req.Domain)
	// 清理历史版本写到其他目录的同域名 Agent 文件：从 nginx -T 的生效清单反向比对，
	// 非当前目录但内容带 Agent 标记的 havline-<domain>.conf 一律删除（不碰宝塔自己的配置）。
	if nginx := s.nginxPath(); nginx != "" {
		if tOut, tErr := exec.Command(nginx, "-T").CombinedOutput(); tErr == nil {
			for _, line := range strings.Split(string(tOut), "\n") {
				line = strings.TrimSpace(line)
				if value, found := strings.CutPrefix(line, "# configuration file "); found {
					p := strings.TrimSuffix(strings.TrimSpace(value), ":")
					base := filepath.Base(p)
					if p != path && base == "havline-"+req.Domain+".conf" {
						if data, readErr := os.ReadFile(p); readErr == nil && strings.HasPrefix(string(data), "# Managed by havline-agent.") {
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
	if err := s.applyRouteContent(path, []byte(content)); err != nil {
		return err
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
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "havline-") || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		result = append(result, strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "havline-"), ".conf"))
	}
	sort.Strings(result)
	return result
}

func (s *Server) routePath(domain string) string {
	return filepath.Join(s.cfg.NginxConfDir, "havline-"+domain+".conf")
}

func (s *Server) validateNginx() error {
	migrateLegacyVersionDirs(s.cfg.NginxConfDir)
	return runCommand(s.nginxPath(), "-t")
}

func (s *Server) validateRouteConfig() error {
	if s.validateNginxFn != nil {
		return s.validateNginxFn()
	}
	return s.validateNginx()
}

func (s *Server) reloadRouteConfig() error {
	if s.reloadNginxFn != nil {
		return s.reloadNginxFn()
	}
	return s.reloadNginx()
}

// migrateLegacyVersionDirs 把 conf 目录下的 <name>.versions 目录迁移为隐藏目录。
// 旧布局会被 nginx 的 include * 当成配置文件读取，报 pread() "Is a directory" 导致 nginx -t 失败。
func migrateLegacyVersionDirs(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".versions" || !strings.HasSuffix(entry.Name(), ".versions") {
			continue
		}
		base := strings.TrimSuffix(filepath.Join(dir, entry.Name()), ".versions")
		_ = fsutil.MigrateLegacyVersions(base)
	}
}

// migrateVersionDirsFromNginxOutput 从 nginx 报错文本里解析被误当配置文件读取的 .versions 目录并迁移。
// nginx -T 在探测目录前就失败时，这是拿到具体路径的唯一线索。
func migrateVersionDirsFromNginxOutput(output string) bool {
	changed := false
	for _, line := range strings.Split(output, "\n") {
		idx := strings.Index(line, ".versions")
		if idx < 0 {
			continue
		}
		start := strings.LastIndex(line[:idx], "\"")
		if start < 0 {
			start = strings.LastIndexAny(line[:idx], " '")
		}
		path := line[start+1 : idx+len(".versions")]
		if !filepath.IsAbs(path) {
			continue
		}
		base := strings.TrimSuffix(path, ".versions")
		if err := fsutil.MigrateLegacyVersions(base); err != nil {
			continue
		}
		changed = true
	}
	return changed
}

func (s *Server) reloadNginx() error {
	nginx := s.nginxPath()
	if nginx == "" {
		return errors.New("未找到 nginx 可执行文件：请先在「公网反代」页安装 Nginx，或用 HAVLINE_AGENT_NGINX_BIN 指定路径")
	}
	return runCommand(nginx, "-s", "reload")
}

// nginxPath 返回**可用的** nginx 可执行路径：配置的路径确实可用才用它，否则探测常见安装位置
// （宝塔的 nginx 不在 systemd PATH 里，直接用名字会 exec 失败）。
// 探测不到时返回空串。注意：不要回落到配置里的裸名字（如 "nginx"），
// 那会让 probeNginx 的 Missing 判定永远不成立——「未安装」会被当成「配置校验失败」
// （页面显示 nginx -t 执行失败），且一键安装入口永远不出现。
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
	return ""
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
	b.WriteString("# Managed by havline-agent. Do not edit.\n")
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
	b.WriteString(nginxtmpl.ServerSecurityHeaders(nginxtmpl.SecurityHeaders{
		Enabled: req.SecurityHeaders,
		HTTPS:   tlsEnabled,
	}))
	// 限流/连接数的 zone 定义在 havlineagent-limits.conf（http 上下文），此处只声明状态码
	if req.RateLimitRate > 0 {
		b.WriteString("    limit_req_status 429;\n")
	}
	if req.ConnLimitMax > 0 {
		b.WriteString("    limit_conn_status 503;\n")
	}
	if req.WebSocket {
		b.WriteString("    proxy_http_version 1.1;\n")
		b.WriteString(nginxtmpl.UpgradeHeaders("    ", "\"upgrade\""))
	}
	// IP 访问控制：白名单（allow + deny all）与黑名单（deny）可同时生效；
	// 顺序由共享模板统一：deny → 仅大陆规则 → allow → deny all。
	chinaDeny := ""
	if req.ChinaOnly {
		chinaDeny = "        if ($havline_china_client = 0) { return 403; }\n"
	}
	aclText := nginxtmpl.AccessDirectives(nginxtmpl.AccessControl{
		Allow:     req.AllowIPs,
		Deny:      req.DenyIPs,
		ChinaDeny: chinaDeny,
	}, "        ")
	// Basic Auth：密码优先用规则表单值（自定义），留空则首次自动生成并复用（htpasswd 格式）；
	// 自动生成的明文由 putRoute 在响应里回一次，这里只写 auth_basic 指令，不写密码注释
	authLine := ""
	if user := strings.TrimSpace(req.BasicAuthUser); user != "" {
		if password := strings.TrimSpace(req.BasicAuthPassword); password != "" {
			if err := writeBasicAuthSecret(req.Domain, user, password); err == nil {
				authLine = fmt.Sprintf("        auth_basic \"Havline Protected\";\n        auth_basic_user_file %s;\n", basicAuthFilePath(req.Domain))
			}
		} else if _, err := ensureBasicAuthSecret(req.Domain, user); err == nil {
			authLine = fmt.Sprintf("        auth_basic \"Havline Protected\";\n        auth_basic_user_file %s;\n", basicAuthFilePath(req.Domain))
		}
	}
	// 限流 / 连接数（zone 在 http 上下文定义）
	limitText := nginxtmpl.LimitDirectives(nginxtmpl.LimitConfig{
		Zone:  nginxtmpl.ZoneName("havline", req.Domain),
		Rate:  req.RateLimitRate,
		Burst: req.RateLimitBurst,
		Conn:  req.ConnLimitMax,
	}, "        ")
	fmt.Fprintf(&b, "    location / {\n%s%s%s        proxy_pass http://%s;\n        proxy_set_header Host $host;\n        proxy_set_header X-Real-IP $remote_addr;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n        proxy_http_version 1.1;\n    }\n}\n", aclText, authLine, limitText, req.Upstream)
	return b.String()
}

// agentStateDir 存放 Agent 的持久状态（htpasswd、geo IP 段、限流参数）。
const agentStateDir = "/var/lib/havline-agent"

// zoneKey 把域名转成合法的 nginx zone 名（仅保留小写字母与数字，其余换下划线）。
func zoneKey(domain string) string {
	return nginxtmpl.ZoneName("havline", domain)
}

// limitsFilePath 返回 zone 定义文件（放在 vhost 目录，顶层指令即 http 上下文）。
func (s *Server) limitsFilePath() string {
	return filepath.Join(s.cfg.NginxConfDir, "havlineagent-limits.conf")
}

// chinaGeoFilePath 返回 geo 变量定义文件（同样位于 http 上下文）。
func (s *Server) chinaGeoFilePath() string {
	return filepath.Join(s.cfg.NginxConfDir, "havlineagent-china.conf")
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

func readLimitState() (map[string]routeLimit, error) {
	return readLimitStateFile(limitsStatePath())
}

func readLimitStateFile(path string) (map[string]routeLimit, error) {
	state := map[string]routeLimit{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("限流状态文件为空: %s", path)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("解析限流状态失败: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("限流状态文件内容为空对象: %s", path)
	}
	return state, nil
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
	b.WriteString("# Managed by havline-agent. Do not edit.\n")
	domains := make([]string, 0, len(state))
	for domain := range state {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	for _, domain := range domains {
		item := state[domain]
		key := zoneKey(domain)
		if item.Rate > 0 {
			b.WriteString(nginxtmpl.RateLimitZone(key+"_req", item.Rate))
		}
		if item.Conn > 0 {
			b.WriteString(nginxtmpl.ConnLimitZone(key + "_conn"))
		}
	}
	return atomicWrite(path, []byte(b.String()), 0o640)
}

// applyLimitState 记录本次路由的限流参数并刷新 zone 文件（必须在写 vhost 之前完成）。
func (s *Server) applyLimitState(req routeRequest) error {
	state, err := readLimitState()
	if err != nil {
		return fmt.Errorf("读取限流状态失败，已保留原文件: %w", err)
	}
	if req.RateLimitRate > 0 || req.ConnLimitMax > 0 {
		state[req.Domain] = routeLimit{Rate: req.RateLimitRate, Burst: req.RateLimitBurst, Conn: req.ConnLimitMax}
	} else {
		delete(state, req.Domain)
	}
	return s.writeLimitState(state)
}

// clearLimitState 删除该域名的限流参数并刷新 zone 文件。
func (s *Server) clearLimitState(domain string) error {
	state, err := readLimitState()
	if err != nil {
		return fmt.Errorf("读取限流状态失败，已保留原文件: %w", err)
	}
	if _, ok := state[domain]; !ok {
		return nil
	}
	delete(state, domain)
	return s.writeLimitState(state)
}

// sslDirectives 生成证书与 TLS 协议指令；tls13Only 时只允许 TLS 1.3。
func sslDirectives(certDir string, tls13Only bool) string {
	return nginxtmpl.SSLDirectives(nginxtmpl.TLSConfig{
		CertPath:  filepath.Join(certDir, "fullchain.pem"),
		KeyPath:   filepath.Join(certDir, "privkey.pem"),
		TLS13Only: tls13Only,
	})
}

// writeBasicAuthSecret 用规则表单提供的密码写入 htpasswd（自定义密码场景）。
func writeBasicAuthSecret(domain, user, password string) error {
	hash, err := apr1Crypt(password)
	if err != nil {
		return fmt.Errorf("生成 htpasswd 失败: %w", err)
	}
	return atomicWrite(basicAuthFilePath(domain), []byte(user+":"+hash+"\n"), 0o640)
}

// basicAuthFilePath 返回指定域名的 htpasswd 文件路径（与反代配置同目录管理）。
func basicAuthFilePath(domain string) string {
	return filepath.Join("/var/lib/havline-agent", "basicauth", domain+".htpasswd")
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
	// 纯 Go 生成 apr1 哈希（等价 openssl passwd -apr1，不依赖 VPS 上的外部命令）
	hash, err := apr1Crypt(password)
	if err != nil {
		return "", fmt.Errorf("生成 htpasswd 失败: %w", err)
	}
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
	return fsutil.WriteFileAtomic(path, data, mode)
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
