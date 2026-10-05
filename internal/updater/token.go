package updater

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultTokenPath 是主服务与 updater sidecar 共享 Token 的默认位置。
const DefaultTokenPath = "/run/havline-updater/updater.token"

const tokenWaitInterval = 200 * time.Millisecond

// ResolveToken 解析 updater Token：显式环境变量优先，其次读取共享文件。
// 文件不存在时返回空 Token，不视为错误，便于不含 updater 的部署直接启动。
func ResolveToken(token, tokenFile string) (string, string, error) {
	if value := strings.TrimSpace(token); value != "" {
		return value, "环境变量", nil
	}
	path := strings.TrimSpace(tokenFile)
	if path == "" {
		return "", "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("读取 updater Token 文件失败: %w", err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", "", fmt.Errorf("updater Token 文件为空: %s", path)
	}
	return value, path, nil
}

// EnsureToken 确保共享 Token 文件存在。环境变量优先；未配置时由主程序生成
// 并原子写入共享卷，updater 随后读取同一文件。
func EnsureToken(token, tokenFile string) (string, string, error) {
	if value := strings.TrimSpace(token); value != "" {
		return value, "环境变量", nil
	}
	path := strings.TrimSpace(tokenFile)
	if path == "" {
		return "", "", nil
	}
	if data, err := os.ReadFile(path); err == nil {
		if value := strings.TrimSpace(string(data)); value != "" {
			return value, path, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("读取 updater Token 文件失败: %w", err)
	}
	value, err := randomToken()
	if err != nil {
		return "", "", err
	}
	if err := writeTokenFile(path, value); err != nil {
		return "", "", err
	}
	return value, path, nil
}

// WaitForToken 等待主程序在共享卷中创建 Token 文件；环境变量已设置时立即返回。
func WaitForToken(token, tokenFile string, timeout time.Duration) (string, string, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		value, source, err := ResolveToken(token, tokenFile)
		if err != nil {
			return "", "", err
		}
		if value != "" {
			return value, source, nil
		}
		if time.Now().After(deadline) {
			return "", "", fmt.Errorf("等待 updater Token 文件超时: %s", tokenFile)
		}
		time.Sleep(tokenWaitInterval)
	}
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成 updater Token 失败: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func writeTokenFile(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("创建 updater Token 目录失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(token+"\n"), 0o600); err != nil {
		return fmt.Errorf("写入 updater Token 文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("保存 updater Token 文件失败: %w", err)
	}
	return os.Chmod(path, 0o600)
}
