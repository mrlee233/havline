package updater

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	if err := s.ensureComposeFile(ctx); err != nil {
		return err
	}

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
	s.logf("开始构建 havline:%s，这一步可能持续几分钟", version)
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
	s.logf("发现升级失败，尝试回滚到 havline:%s", version)
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.compose(rollbackCtx, version, "up", "-d", "--no-deps", "--force-recreate", s.cfg.Service); err != nil {
		s.logf("自动回滚失败：%v", err)
		return
	}
	s.logf("已切回 havline:%s", version)
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
	if _, err := runCommand(ctx, s.cfg.ProjectDir, nil, "git", "pull", "--ff-only", "origin", s.cfg.Branch); err != nil {
		return fmt.Errorf("git pull 失败：%w", err)
	}
	return nil
}

func (s *Server) latestVersion(ctx context.Context) (string, error) {
	repoURL := strings.TrimSpace(s.cfg.RepoURL)
	if s.cfg.Mode == "local" {
		if remote, err := s.localRemoteURL(ctx); err == nil && strings.TrimSpace(remote) != "" {
			repoURL = remote
		}
	}
	localPath := filepath.Join(s.cfg.ProjectDir, "VERSION")
	if repoURL != "" {
		version, err := fetchRemoteVersion(ctx, repoURL, s.cfg.Branch)
		if err == nil {
			return version, nil
		}
		if s.cfg.Mode == "git" {
			return "", err
		}
		s.logf("读取远端版本失败，改用本地 VERSION：%v", err)
	}
	return readVersionFile(localPath)
}

func (s *Server) localRemoteURL(ctx context.Context) (string, error) {
	if _, err := os.Stat(filepath.Join(s.cfg.ProjectDir, ".git")); err != nil {
		return "", err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := runCommand(checkCtx, s.cfg.ProjectDir, nil, "git", "config", "--get", "remote.origin.url")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// fetchRemoteVersion 通过仓库 raw / API 地址读取 VERSION，避免为了检查版本克隆整个仓库。
func fetchRemoteVersion(ctx context.Context, repoURL, branch string) (string, error) {
	urls, err := remoteVersionURLs(repoURL, branch)
	if err != nil {
		return "", err
	}
	var lastErr error
	for _, candidate := range urls {
		version, err := fetchVersionURL(ctx, candidate)
		if err == nil {
			return version, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("读取远端版本失败：%w", lastErr)
}

func remoteVersionURLs(repoURL, branch string) ([]string, error) {
	parsed, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("仓库地址无效：%s", repoURL)
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	repoPath := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
	if repoPath == "" {
		return nil, fmt.Errorf("仓库地址缺少路径：%s", repoURL)
	}
	base := parsed.Scheme + "://" + parsed.Host
	urls := []string{
		fmt.Sprintf("%s/api/v1/repos/%s/contents/VERSION?ref=%s", base, repoPath, url.QueryEscape(branch)),
		fmt.Sprintf("%s/%s/raw/branch/%s/VERSION", base, repoPath, url.PathEscape(branch)),
		fmt.Sprintf("%s/%s/raw/%s/VERSION", base, repoPath, url.PathEscape(branch)),
	}
	if strings.EqualFold(parsed.Hostname(), "github.com") {
		urls = append([]string{fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/VERSION", repoPath, url.PathEscape(branch))}, urls...)
	}
	return urls, nil
}

func fetchVersionURL(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: HTTP %d", rawURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	version, err := parseRemoteVersion(data)
	if err != nil {
		return "", fmt.Errorf("%s: %w", rawURL, err)
	}
	return version, nil
}

func parseRemoteVersion(data []byte) (string, error) {
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, "{") {
		var payload struct {
			Content     string `json:"content"`
			Encoding    string `json:"encoding"`
			DownloadURL string `json:"download_url"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return "", err
		}
		if payload.Encoding == "base64" && payload.Content != "" {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload.Content))
			if err != nil {
				return "", err
			}
			text = strings.TrimSpace(string(decoded))
		}
	}
	if err := validateSemver(text); err != nil {
		return "", err
	}
	return text, nil
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
	plan, err := s.resolveComposePlan(ctx)
	if err != nil {
		return "", err
	}
	if plan.helper {
		return s.composeViaHelper(ctx, plan, version, args...)
	}
	fullArgs := append([]string{"compose", "--project-name", plan.projectName, "-f", plan.containerCompose}, args...)
	env := []string{"HAVLINE_VERSION=" + version}
	return runCommand(ctx, s.cfg.ProjectDir, env, "docker", fullArgs...)
}

func (s *Server) composeViaHelper(ctx context.Context, plan composePlan, version string, args ...string) (string, error) {
	image, err := s.selfImage(ctx)
	if err != nil {
		return "", err
	}
	helperArgs := []string{
		"run", "--rm", "--entrypoint", "docker",
		"-v", s.cfg.DockerSocket + ":/var/run/docker.sock",
		"-v", plan.hostProjectDir + ":/workspace",
		"-w", "/workspace",
		"-e", "HAVLINE_VERSION=" + version,
		image,
		"compose", "--project-name", plan.projectName, "-f", plan.helperComposePath,
	}
	helperArgs = append(helperArgs, args...)
	return runCommand(ctx, "", nil, "docker", helperArgs...)
}

func (s *Server) selfImage(ctx context.Context) (string, error) {
	if s.cfg.SelfImage != "" {
		return s.cfg.SelfImage, nil
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	output, err := runCommand(ctx, "", nil, "docker", "inspect", "--format", "{{.Config.Image}}", hostname)
	if err != nil {
		return "", fmt.Errorf("无法识别 updater 镜像：%w", err)
	}
	image := strings.TrimSpace(output)
	if image == "" {
		return "", fmt.Errorf("无法识别 updater 镜像")
	}
	return image, nil
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
