package cloudflared

import (
	"context"
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
	mirror := findMirror(mirrorID)
	target := s.binaryPath()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	tmp := target + ".download"
	_ = os.Remove(tmp)
	if err := fetchFile(ctx, DownloadURL(mirror, ""), tmp); err != nil {
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
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
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
