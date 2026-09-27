package updater

import (
	"context"
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

// Config 是 fonu-updater sidecar 的运行配置。
type Config struct {
	SocketPath    string
	ProjectDir    string
	ComposeFile   string
	Service       string
	Mode          string
	RepoURL       string
	Branch        string
	AppURL        string
	BuildTimeout  time.Duration
	HealthTimeout time.Duration
}

type Server struct {
	cfg         Config
	logger      *slog.Logger
	composePath string

	mu          sync.Mutex
	state       Status
	lastChecked time.Time
}

// NewServer 校验 sidecar 配置；参数不正确时拒绝启动。
func NewServer(cfg Config, logger *slog.Logger) (*Server, error) {
	cfg.SocketPath = strings.TrimSpace(cfg.SocketPath)
	if cfg.SocketPath == "" {
		cfg.SocketPath = "/run/fonu-updater/updater.sock"
	}
	if strings.TrimSpace(cfg.ProjectDir) == "" {
		cfg.ProjectDir = "/workspace"
	}
	if strings.TrimSpace(cfg.ComposeFile) == "" {
		cfg.ComposeFile = "docker-compose.yml"
	}
	if strings.TrimSpace(cfg.Service) == "" {
		cfg.Service = "fonu"
	}
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = "local"
	}
	if strings.TrimSpace(cfg.Branch) == "" {
		cfg.Branch = "main"
	}
	if strings.TrimSpace(cfg.AppURL) == "" {
		cfg.AppURL = "http://fonu:6893"
	}
	if cfg.BuildTimeout <= 0 {
		cfg.BuildTimeout = 30 * time.Minute
	}
	if cfg.HealthTimeout <= 0 {
		cfg.HealthTimeout = 3 * time.Minute
	}
	if cfg.Mode != "local" && cfg.Mode != "git" {
		return nil, fmt.Errorf("FONU_UPDATE_MODE 只支持 local 或 git")
	}
	if !serviceNamePattern.MatchString(cfg.Service) {
		return nil, fmt.Errorf("FONU_UPDATE_SERVICE 含非法字符")
	}
	composePath, err := resolveComposePath(cfg.ProjectDir, cfg.ComposeFile)
	if err != nil {
		return nil, err
	}
	if cfg.Mode == "git" && strings.TrimSpace(cfg.RepoURL) == "" {
		return nil, fmt.Errorf("git 模式必须设置 FONU_UPDATE_REPO")
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
	return mux
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

// Status 返回当前版本、远端版本和任务状态。
func (s *Server) Status(ctx context.Context) Status {
	s.mu.Lock()
	if busyPhase(s.state.Phase) {
		status := cloneStatus(s.state)
		s.mu.Unlock()
		return status
	}
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
	s.state.Message = "升级任务已排队，即将重启 Fonu"
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
