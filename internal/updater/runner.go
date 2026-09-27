package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxCommandOutput = 64 << 10

func (s *Server) runUpdate(ctx context.Context) error {
	s.mu.Lock()
	s.state.Phase = PhaseRunning
	s.state.Message = "正在准备升级"
	s.mu.Unlock()

	latest, err := s.latestVersion(ctx)
	if err != nil {
		return err
	}
	if err := validateSemver(latest); err != nil {
		return err
	}
	current, _ := readCurrentVersion(ctx, s.cfg.AppURL)
	if current != "" && CompareVersions(latest, current) <= 0 {
		return fmt.Errorf("当前已是最新版本 %s", current)
	}
	previousImage, _ := s.currentImage()
	s.logf("当前容器镜像：%s", previousImage)
	s.logf("目标版本：%s", latest)

	if s.cfg.Mode == "local" {
		if err := s.prepareLocalSource(ctx); err != nil {
			return err
		}
	}
	if err := s.build(ctx, latest); err != nil {
		return err
	}
	if err := s.recreate(ctx, latest); err != nil {
		return err
	}
	if err := s.waitHealthy(ctx, latest); err != nil {
		s.rollback(previousImage)
		return fmt.Errorf("升级后健康检查失败：%w", err)
	}
	return nil
}

func (s *Server) build(ctx context.Context, version string) error {
	s.logf("开始构建 fonu:%s，这一步可能持续几分钟", version)
	buildCtx, cancel := context.WithTimeout(ctx, s.cfg.BuildTimeout)
	defer cancel()
	if _, err := s.compose(buildCtx, version, "build", s.cfg.Service); err != nil {
		return fmt.Errorf("docker compose build 失败：%w", err)
	}
	return nil
}

func (s *Server) recreate(ctx context.Context, version string) error {
	s.logf("重建容器 %s", s.cfg.Service)
	upCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := s.compose(upCtx, version, "up", "-d", "--no-deps", "--force-recreate", s.cfg.Service); err != nil {
		return fmt.Errorf("docker compose up 失败：%w", err)
	}
	return nil
}

func (s *Server) waitHealthy(ctx context.Context, expected string) error {
	s.logf("等待服务恢复并返回版本 %s", expected)
	deadline := time.Now().Add(s.cfg.HealthTimeout)
	for time.Now().Before(deadline) {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		version, err := readCurrentVersion(checkCtx, s.cfg.AppURL)
		cancel()
		if err == nil && version == expected {
			s.logf("健康检查通过：%s", version)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf("等待 %s 超时", expected)
}

func (s *Server) rollback(previousImage string) {
	version := imageTag(previousImage)
	if version == "" {
		s.logf("无法确定上一个镜像 tag，跳过自动回滚")
		return
	}
	s.logf("发现升级失败，尝试回滚到 fonu:%s", version)
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.compose(rollbackCtx, version, "up", "-d", "--no-deps", "--force-recreate", s.cfg.Service); err != nil {
		s.logf("自动回滚失败：%v", err)
		return
	}
	s.logf("已切回 fonu:%s", version)
}

func (s *Server) prepareLocalSource(ctx context.Context) error {
	if _, err := os.Stat(filepath.Join(s.cfg.ProjectDir, ".git")); err != nil {
		s.logf("项目目录不是 Git 仓库，跳过源码拉取")
		return nil
	}
	status, err := runCommand(ctx, s.cfg.ProjectDir, nil, "git", "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("检查 Git 工作区失败：%w", err)
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("本地源码存在未提交改动，已拒绝自动拉取")
	}
	s.logf("拉取远端源码")
	if _, err := runCommand(ctx, s.cfg.ProjectDir, nil, "git", "pull", "--ff-only"); err != nil {
		return fmt.Errorf("git pull 失败：%w", err)
	}
	return nil
}

func (s *Server) latestVersion(ctx context.Context) (string, error) {
	if s.cfg.Mode == "git" {
		return s.latestVersionFromGitClone(ctx)
	}
	return s.latestVersionFromLocal(ctx)
}

func (s *Server) latestVersionFromLocal(ctx context.Context) (string, error) {
	localPath := filepath.Join(s.cfg.ProjectDir, "VERSION")
	if _, err := os.Stat(filepath.Join(s.cfg.ProjectDir, ".git")); err != nil {
		return readVersionFile(localPath)
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := runCommand(fetchCtx, s.cfg.ProjectDir, nil, "git", "fetch", "origin", s.cfg.Branch, "--prune"); err != nil {
		return readVersionFile(localPath)
	}
	value, err := runCommand(ctx, s.cfg.ProjectDir, nil, "git", "show", "origin/"+s.cfg.Branch+":VERSION")
	if err != nil {
		return readVersionFile(localPath)
	}
	return strings.TrimSpace(value), nil
}

func (s *Server) latestVersionFromGitClone(ctx context.Context) (string, error) {
	dir, err := os.MkdirTemp("", "fonu-update-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	cloneCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if _, err := runCommand(cloneCtx, "", nil, "git", "clone", "--depth", "1", "--branch", s.cfg.Branch, s.cfg.RepoURL, dir); err != nil {
		return "", fmt.Errorf("读取远端版本失败：%w", err)
	}
	return readVersionFile(filepath.Join(dir, "VERSION"))
}

func readVersionFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(data))
	if err := validateSemver(version); err != nil {
		return "", err
	}
	return version, nil
}

func readCurrentVersion(ctx context.Context, appURL string) (string, error) {
	url := strings.TrimRight(appURL, "/") + "/api/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.Version), nil
}

func (s *Server) currentImage() (string, error) {
	output, err := runCommand(context.Background(), "", nil, "docker", "inspect", "--format", "{{.Config.Image}}", s.cfg.Service)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func imageTag(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

func (s *Server) compose(ctx context.Context, version string, args ...string) (string, error) {
	fullArgs := append([]string{"compose", "-f", s.composePath}, args...)
	env := []string{"FONU_VERSION=" + version}
	return runCommand(ctx, s.cfg.ProjectDir, env, "docker", fullArgs...)
}

func runCommand(ctx context.Context, dir string, extraEnv []string, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	if len(extraEnv) > 0 {
		command.Env = append(os.Environ(), extraEnv...)
	}
	output, err := command.CombinedOutput()
	text := string(output)
	if len(text) > maxCommandOutput {
		text = text[len(text)-maxCommandOutput:]
	}
	if err != nil {
		return text, fmt.Errorf("%s: %w: %s", strings.Join(append([]string{name}, args...), " "), err, strings.TrimSpace(text))
	}
	return text, nil
}
