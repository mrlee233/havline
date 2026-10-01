package cloudflared

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const minBinarySize = 10 << 20

// Mirror 是 cloudflared 二进制下载镜像。
type Mirror struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Base string `json:"base"`
}

// DefaultMirrors 返回内置镜像表；Base 为空表示 GitHub 官方源。
func DefaultMirrors() []Mirror {
	return []Mirror{
		{ID: "official", Name: "GitHub 官方源"},
		{ID: "gh-proxy-org", Name: "gh-proxy.org", Base: "https://gh-proxy.org/"},
		{ID: "gh-proxy-hk", Name: "gh-proxy 香港", Base: "https://hk.gh-proxy.org/"},
		{ID: "gh-proxy-cdn", Name: "gh-proxy CDN", Base: "https://cdn.gh-proxy.org/"},
		{ID: "gh-proxy-edgeone", Name: "gh-proxy EdgeOne", Base: "https://edgeone.gh-proxy.org/"},
	}
}

func findMirror(id string) Mirror {
	id = strings.TrimSpace(id)
	for _, mirror := range DefaultMirrors() {
		if mirror.ID == id {
			return mirror
		}
	}
	return DefaultMirrors()[0]
}

func binaryFileName() string {
	name := "cloudflared-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// DownloadURL 生成指定镜像的下载地址；version 为空表示 latest。
func DownloadURL(mirror Mirror, version string) string {
	official := "https://github.com/cloudflare/cloudflared/releases/latest/download/" + binaryFileName()
	if strings.TrimSpace(version) != "" {
		official = "https://github.com/cloudflare/cloudflared/releases/download/" + strings.TrimSpace(version) + "/" + binaryFileName()
	}
	if strings.TrimSpace(mirror.Base) == "" {
		return official
	}
	return mirror.Base + official
}

func (s *Service) binaryPath() string {
	name := "cloudflared"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(s.cfg.DataDir, "cloudflared", "bin", name)
}

// BinaryVersion 返回本机 cloudflared 版本；未安装返回空串。
func (s *Service) BinaryVersion() string {
	path := s.binaryPath()
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// DownloadBinary 下载并替换本机 cloudflared 二进制。
func (s *Service) DownloadBinary(ctx context.Context, mirrorID string) (string, error) {
	return s.DownloadBinaryVersion(ctx, mirrorID, "")
}

// DownloadBinaryVersion 下载指定版本；version 为空表示最新版。
func (s *Service) DownloadBinaryVersion(ctx context.Context, mirrorID, version string) (string, error) {
	mirror := findMirror(mirrorID)
	target := s.binaryPath()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	tmp := target + ".download"
	_ = os.Remove(tmp)
	if err := fetchFile(ctx, DownloadURL(mirror, version), tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	info, err := os.Stat(tmp)
	if err != nil || info.Size() < minBinarySize {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("下载文件过小，可能被镜像源截断")
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmp, 0o755)
	}
	if out, err := exec.Command(tmp, "--version").CombinedOutput(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("cloudflared 二进制不可执行: %s", strings.TrimSpace(string(out)))
	}
	previous := target + ".previous"
	if _, err := os.Stat(target); err == nil {
		_ = os.Remove(previous)
		if err := os.Rename(target, previous); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Rename(previous, target)
		_ = os.Remove(tmp)
		return "", err
	}
	return s.BinaryVersion(), nil
}

// LatestBinaryVersion 查询 Cloudflare 官方 GitHub Release 的最新版本。
func (s *Service) LatestBinaryVersion(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/cloudflare/cloudflared/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("读取最新版本失败：HTTP %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	version := strings.TrimSpace(payload.TagName)
	if version == "" {
		return "", fmt.Errorf("GitHub 未返回 cloudflared 版本")
	}
	return version, nil
}

// RollbackBinary 恢复上一版 cloudflared 二进制。
func (s *Service) RollbackBinary() (string, error) {
	target := s.binaryPath()
	previous := target + ".previous"
	if _, err := os.Stat(previous); err != nil {
		return "", fmt.Errorf("没有可回滚的 cloudflared 版本")
	}
	failed := target + ".failed"
	_ = os.Remove(failed)
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, failed); err != nil {
			return "", err
		}
	}
	if err := os.Rename(previous, target); err != nil {
		_ = os.Rename(failed, target)
		return "", err
	}
	_ = os.Remove(failed)
	return s.BinaryVersion(), nil
}

func fetchFile(ctx context.Context, url, path string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载 cloudflared 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 cloudflared 失败: HTTP %d", resp.StatusCode)
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("写入 cloudflared 失败: %w", err)
	}
	return nil
}
