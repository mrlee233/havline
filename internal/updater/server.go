package updater

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var ErrBusy = errors.New("升级任务正在进行中")

var serviceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// Config 是 havline-updater sidecar 的运行配置。
type Config struct {
	SocketPath     string
	ProjectDir     string
	ComposeFile    string
	Service        string
	Mode           string
	RepoURL        string
	Branch         string
	AppURL         string
	HostProjectDir string
	ProjectName    string
	SelfImage      string
	DockerSocket   string
	Token          string
	BuildTimeout   time.Duration
	HealthTimeout  time.Duration
}

type Server struct {
	cfg         Config
	logger      *slog.Logger
	composePath string

	mu          sync.Mutex
	state       Status
	lastChecked time.Time
}

type composePlan struct {
	helper            bool
	projectName       string
	containerCompose  string
	hostProjectDir    string
	hostComposePath   string
	helperComposePath string
}

// NewServer 校验 sidecar 配置；参数不正确时拒绝启动。
func NewServer(cfg Config, logger *slog.Logger) (*Server, error) {
	cfg.SocketPath = strings.TrimSpace(cfg.SocketPath)
	if cfg.SocketPath == "" {
		cfg.SocketPath = "/run/havline-updater/updater.sock"
	}
	if strings.TrimSpace(cfg.ProjectDir) == "" {
		cfg.ProjectDir = "/workspace"
	}
	if strings.TrimSpace(cfg.ComposeFile) == "" {
		cfg.ComposeFile = "docker-compose.yml"
	}
	if strings.TrimSpace(cfg.Service) == "" {
		cfg.Service = "havline"
	}
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = "local"
	}
	if strings.TrimSpace(cfg.Branch) == "" {
		cfg.Branch = "main"
	}
	if strings.TrimSpace(cfg.AppURL) == "" {
		cfg.AppURL = "http://havline:6893"
	}
	if strings.TrimSpace(cfg.DockerSocket) == "" {
		cfg.DockerSocket = "/var/run/docker.sock"
	}
	cfg.HostProjectDir = strings.TrimSpace(cfg.HostProjectDir)
	cfg.ProjectName = strings.TrimSpace(cfg.ProjectName)
	cfg.SelfImage = strings.TrimSpace(cfg.SelfImage)
	cfg.Token = strings.TrimSpace(cfg.Token)
	if cfg.Token == "" {
		return nil, fmt.Errorf("HAVLINE_UPDATER_TOKEN 未设置，拒绝启动未鉴权的升级服务")
	}
	if cfg.BuildTimeout <= 0 {
		cfg.BuildTimeout = 30 * time.Minute
	}
	if cfg.HealthTimeout <= 0 {
		cfg.HealthTimeout = 3 * time.Minute
	}
	if cfg.Mode != "local" && cfg.Mode != "git" {
		return nil, fmt.Errorf("HAVLINE_UPDATE_MODE 只支持 local 或 git")
	}
	if !serviceNamePattern.MatchString(cfg.Service) {
		return nil, fmt.Errorf("HAVLINE_UPDATE_SERVICE 含非法字符")
	}
	composePath, err := resolveComposePath(cfg.ProjectDir, cfg.ComposeFile)
	if err != nil {
		return nil, err
	}
	if cfg.Mode == "git" && strings.TrimSpace(cfg.RepoURL) == "" {
		return nil, fmt.Errorf("git 模式必须设置 HAVLINE_UPDATE_REPO")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:         cfg,
		logger:      logger,
		composePath: composePath,
		state: Status{
			Enabled: true,
			Mode:    cfg.Mode,
			Phase:   PhaseIdle,
			Message: "尚未检查更新",
		},
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("POST /apply", s.handleApply)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimSpace(r.Header.Get(tokenHeader))
		if len(provided) != len(s.cfg.Token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.Token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "updater 鉴权失败"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// Serve 在共享 Unix Socket 上提供服务。
func (s *Server) Serve(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.cfg.SocketPath), 0o750); err != nil {
		return err
	}
	_ = os.Remove(s.cfg.SocketPath)
	listener, err := net.Listen("unix", s.cfg.SocketPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(s.cfg.SocketPath)
	}()
	_ = os.Chmod(s.cfg.SocketPath, 0o660)

	server := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Status(r.Context()))
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	status, err := s.Apply(r.Context())
	if errors.Is(err, ErrBusy) {
		writeJSON(w, http.StatusConflict, status)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}

func (s *Server) ensureComposeFile(ctx context.Context) error {
	_, err := s.resolveComposePlan(ctx)
	return err
}

func (s *Server) resolveComposePlan(ctx context.Context) (composePlan, error) {
	projectName, err := s.composeProjectName(ctx)
	if err != nil {
		return composePlan{}, err
	}
	info, err := os.Stat(s.composePath)
	if err == nil && !info.IsDir() {
		return composePlan{projectName: projectName, containerCompose: s.composePath}, nil
	}
	if err == nil && info.IsDir() {
		return composePlan{}, fmt.Errorf("Compose 路径是目录而不是文件：%s", s.composePath)
	}
	hostDir, hostComposePath, discoverErr := s.discoverHostCompose(ctx)
	if discoverErr == nil {
		helperPath, pathErr := helperComposePath(hostDir, hostComposePath)
		if pathErr == nil {
			return composePlan{
				helper:            true,
				projectName:       projectName,
				hostProjectDir:    hostDir,
				hostComposePath:   hostComposePath,
				helperComposePath: helperPath,
			}, nil
		}
	}
	return composePlan{}, fmt.Errorf("找不到 Compose 文件 %s，且无法自动定位宿主机目录；请设置 HAVLINE_UPDATE_HOST_DIR", s.composePath)
}

func (s *Server) composeProjectName(ctx context.Context) (string, error) {
	projectName, err := s.dockerLabel(ctx, "com.docker.compose.project")
	projectName = strings.TrimSpace(projectName)
	if projectName != "" {
		return projectName, nil
	}
	if s.cfg.ProjectName != "" {
		return s.cfg.ProjectName, nil
	}
	if err != nil {
		return "", fmt.Errorf("无法读取现有容器的 Compose 项目标签：%w", err)
	}
	return "", fmt.Errorf("现有 havline 容器不是由 Compose 管理，无法安全一键升级；请按 Compose 方式重新部署")
}

func (s *Server) discoverHostCompose(ctx context.Context) (string, string, error) {
	if s.cfg.HostProjectDir != "" && !isContainerPath(s.cfg.HostProjectDir, s.cfg.ProjectDir) {
		if !filepath.IsAbs(s.cfg.HostProjectDir) {
			return "", "", fmt.Errorf("HAVLINE_UPDATE_HOST_DIR 必须是绝对路径")
		}
		return s.cfg.HostProjectDir, filepath.Join(s.cfg.HostProjectDir, filepath.Base(s.cfg.ComposeFile)), nil
	}
	workingDir, _ := s.dockerLabel(ctx, "com.docker.compose.project.working_dir")
	configFiles, _ := s.dockerLabel(ctx, "com.docker.compose.project.config_files")
	hostDir := strings.TrimSpace(workingDir)
	hostCompose := chooseComposeFile(configFiles, filepath.Base(s.cfg.ComposeFile))
	if hostCompose == "" {
		if hostDir == "" || isContainerPath(hostDir, s.cfg.ProjectDir) {
			return "", "", fmt.Errorf("无法从容器标签读取宿主机 Compose 目录")
		}
		hostCompose = filepath.Join(hostDir, filepath.Base(s.cfg.ComposeFile))
	}
	if hostDir == "" || isContainerPath(hostDir, s.cfg.ProjectDir) {
		hostDir = filepath.Dir(hostCompose)
	}
	if !filepath.IsAbs(hostDir) || !filepath.IsAbs(hostCompose) {
		return "", "", fmt.Errorf("容器标签中的 Compose 路径不是绝对路径")
	}
	if isContainerPath(hostDir, s.cfg.ProjectDir) {
		return "", "", fmt.Errorf("容器标签只提供了容器内路径 %s，无法定位宿主机 Compose 目录", hostDir)
	}
	return filepath.Clean(hostDir), filepath.Clean(hostCompose), nil
}

func isContainerPath(path, containerDir string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	containerDir = filepath.Clean(strings.TrimSpace(containerDir))
	if path == "" || containerDir == "" || path == "." {
		return false
	}
	return path == containerDir || strings.HasPrefix(path, containerDir+string(filepath.Separator))
}

func (s *Server) dockerLabel(ctx context.Context, label string) (string, error) {
	format := fmt.Sprintf(`{{ index .Config.Labels %q }}`, label)
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := runCommand(checkCtx, "", nil, "docker", "inspect", "--format", format, s.cfg.Service)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func chooseComposeFile(configFiles, expectedBase string) string {
	for _, item := range strings.Split(configFiles, ",") {
		path := strings.TrimSpace(item)
		if path == "" {
			continue
		}
		if filepath.Base(path) == expectedBase {
			return path
		}
	}
	for _, item := range strings.Split(configFiles, ",") {
		if path := strings.TrimSpace(item); path != "" {
			return path
		}
	}
	return ""
}

func helperComposePath(hostDir, hostCompose string) (string, error) {
	rel, err := filepath.Rel(hostDir, hostCompose)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Compose 文件不在宿主机项目目录内")
	}
	return filepath.Join("/workspace", rel), nil
}

// Status 返回当前版本、远端版本和任务状态。
func (s *Server) Status(ctx context.Context) Status {
	s.mu.Lock()
	if busyPhase(s.state.Phase) {
		status := cloneStatus(s.state)
		s.mu.Unlock()
		return status
	}
	s.mu.Unlock()
	if err := s.ensureComposeFile(ctx); err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state.UpdateAvailable = false
		s.state.Message = err.Error()
		s.state.CheckedAt = time.Now().UTC().Format(time.RFC3339)
		return cloneStatus(s.state)
	}
	s.mu.Lock()
	if !s.lastChecked.IsZero() && time.Since(s.lastChecked) < time.Minute {
		status := cloneStatus(s.state)
		s.mu.Unlock()
		return status
	}
	s.mu.Unlock()

	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	current, currentErr := readCurrentVersion(checkCtx, s.cfg.AppURL)
	latest, latestErr := s.latestVersion(checkCtx)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastChecked = time.Now()
	s.state.CurrentVersion = current
	s.state.LatestVersion = latest
	s.state.UpdateAvailable = latest != "" && current != "" && CompareVersions(latest, current) > 0
	s.state.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	switch {
	case currentErr != nil:
		s.state.Message = "无法读取当前运行版本：" + currentErr.Error()
	case latestErr != nil:
		s.state.Message = "无法读取远端最新版本：" + latestErr.Error()
	case s.state.UpdateAvailable:
		s.state.Message = fmt.Sprintf("发现新版本 %s，当前 %s", latest, current)
	default:
		s.state.Message = "当前已是最新版本"
	}
	return cloneStatus(s.state)
}

// Apply 立即返回排队状态，后台完成构建、重建、健康检查和必要回滚。
func (s *Server) Apply(_ context.Context) (Status, error) {
	s.mu.Lock()
	if busyPhase(s.state.Phase) {
		status := cloneStatus(s.state)
		s.mu.Unlock()
		return status, ErrBusy
	}
	s.state.Phase = PhaseScheduled
	s.state.Busy = true
	s.state.Message = "升级任务已排队，即将重启 Havline"
	s.state.Log = []string{"升级任务已排队"}
	status := cloneStatus(s.state)
	s.mu.Unlock()
	go s.runScheduled()
	return status, nil
}

func (s *Server) runScheduled() {
	time.Sleep(2 * time.Second)
	if err := s.runUpdate(context.Background()); err != nil {
		s.logf("升级失败：%v", err)
		s.finish(PhaseFailed, err.Error())
		return
	}
	s.finish(PhaseSuccess, "升级完成")
}

func (s *Server) finish(phase Phase, message string) {
	s.mu.Lock()
	s.state.Phase = phase
	s.state.Busy = false
	s.state.Message = message
	s.lastChecked = time.Time{}
	s.mu.Unlock()
}

func (s *Server) logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	s.logger.Info(line, "module", "UPDATER")
	s.mu.Lock()
	s.state.Log = append(s.state.Log, line)
	if len(s.state.Log) > 200 {
		s.state.Log = s.state.Log[len(s.state.Log)-200:]
	}
	s.mu.Unlock()
}

func cloneStatus(status Status) Status {
	status.Log = append([]string(nil), status.Log...)
	return status
}

func resolveComposePath(projectDir, composeFile string) (string, error) {
	dir, err := filepath.Abs(strings.TrimSpace(projectDir))
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(composeFile)
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	path = filepath.Clean(path)
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("compose 文件必须位于项目目录内")
	}
	switch filepath.Base(path) {
	case "docker-compose.yml", "docker-compose.hub.yml", "docker-compose.host.yml":
		return path, nil
	default:
		return "", fmt.Errorf("不支持的 compose 文件：%s", filepath.Base(path))
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
