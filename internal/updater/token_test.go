package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveTokenPrefersEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "updater.token")
	if err := os.WriteFile(path, []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	token, source, err := ResolveToken("env-token", path)
	if err != nil {
		t.Fatal(err)
	}
	if token != "env-token" || source != "环境变量" {
		t.Fatalf("环境变量应优先，实际 token=%q source=%q", token, source)
	}
}

func TestResolveTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "updater.token")
	if err := os.WriteFile(path, []byte(" file-token \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	token, source, err := ResolveToken("", path)
	if err != nil {
		t.Fatal(err)
	}
	if token != "file-token" || source != path {
		t.Fatalf("文件 Token 解析不正确，实际 token=%q source=%q", token, source)
	}
}

func TestResolveTokenMissingFile(t *testing.T) {
	token, source, err := ResolveToken("", filepath.Join(t.TempDir(), "missing.token"))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" || source != "" {
		t.Fatalf("文件不存在时应返回空 Token，实际 token=%q source=%q", token, source)
	}
}

func TestResolveTokenEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updater.token")
	if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := ResolveToken("", path); err == nil {
		t.Fatal("空 Token 文件应返回错误")
	}
}

func TestEnsureTokenCreatesAndReusesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updater.token")
	token, source, err := EnsureToken("", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 || source != path {
		t.Fatalf("生成的 Token 不正确，实际 token=%q source=%q", token, source)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != token+"\n" {
		t.Fatalf("Token 文件内容不正确: %q", string(data))
	}
	again, _, err := EnsureToken("", path)
	if err != nil {
		t.Fatal(err)
	}
	if again != token {
		t.Fatalf("重复调用应复用文件 Token，实际 %q", again)
	}
}

func TestWaitForTokenReadsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updater.token")
	if err := os.WriteFile(path, []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, _, err := WaitForToken("", path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if token != "file-token" {
		t.Fatalf("等待 Token 结果不正确: %q", token)
	}
}

func TestWaitForTokenTimesOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.token")
	if _, _, err := WaitForToken("", path, 50*time.Millisecond); err == nil {
		t.Fatal("文件不存在时应等待超时")
	}
}

// updater 未启用（socket 为空）时不能去碰 Token 文件：
// 原生版以非 root 运行，写 /run/havline-updater 会失败并拖垮整个启动。
func TestEnsureTokenIfEnabledSkipsWhenSocketDisabled(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "nested", "updater.token")

	token, source, err := EnsureTokenIfEnabled("", "", tokenFile)
	if err != nil {
		t.Fatalf("socket 未启用时不应报错：%v", err)
	}
	if token != "" || source != "" {
		t.Fatalf("socket 未启用时应返回空 Token，实际 token=%q source=%q", token, source)
	}
	if _, statErr := os.Stat(tokenFile); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("socket 未启用时不应创建 Token 文件（statErr=%v）", statErr)
	}
}

// 对照组：updater 启用（socket 非空）时仍照旧生成并落盘。
func TestEnsureTokenIfEnabledCreatesWhenSocketSet(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "nested", "updater.token")

	token, source, err := EnsureTokenIfEnabled("/run/havline-updater/updater.sock", "", tokenFile)
	if err != nil {
		t.Fatalf("socket 启用时应生成 Token：%v", err)
	}
	if len(token) != 64 || source != tokenFile {
		t.Fatalf("生成的 Token 不正确，实际 token=%q source=%q", token, source)
	}
	if _, statErr := os.Stat(tokenFile); statErr != nil {
		t.Fatalf("socket 启用时应写入 Token 文件：%v", statErr)
	}
}
