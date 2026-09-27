package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// 未配置时：生成一个 64 字符密钥、以 0600 落盘，并且下次启动复用同一个
func TestResolveSessionSecretGeneratesAndReuses(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DataDir: dir}

	source, err := cfg.ResolveSessionSecret()
	if err != nil {
		t.Fatalf("生成密钥失败：%v", err)
	}
	if len(cfg.SessionSecret) != 64 {
		t.Fatalf("应生成 64 字符密钥，实际 %q", cfg.SessionSecret)
	}
	if source != cfg.sessionSecretPath() {
		t.Fatalf("来源应指向密钥文件，实际 %q", source)
	}
	info, err := os.Stat(cfg.sessionSecretPath())
	if err != nil {
		t.Fatalf("密钥文件没写出来：%v", err)
	}
	// Windows 没有 POSIX 权限位（Perm() 返回合成值），只在类 Unix 上校验 0600
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("密钥文件权限应为 0600，实际 %v", info.Mode().Perm())
	}

	// 模拟下一次启动：必须复用同一个密钥，否则所有人都会掉线
	again := Config{DataDir: dir}
	if _, err := again.ResolveSessionSecret(); err != nil {
		t.Fatalf("二次读取失败：%v", err)
	}
	if again.SessionSecret != cfg.SessionSecret {
		t.Fatalf("重复启动应复用同一个密钥：%q vs %q", again.SessionSecret, cfg.SessionSecret)
	}
}

// 显式配置优先级最高，且不应该再写密钥文件
func TestResolveSessionSecretPrefersExplicitValue(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), SessionSecret: "explicit-value-1234567890"}
	source, err := cfg.ResolveSessionSecret()
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	if cfg.SessionSecret != "explicit-value-1234567890" {
		t.Fatalf("显式配置应原样保留，实际 %q", cfg.SessionSecret)
	}
	if source != "环境变量" {
		t.Fatalf("来源应标为环境变量，实际 %q", source)
	}
	if _, err := os.Stat(cfg.sessionSecretPath()); err == nil {
		t.Fatal("显式配置时不应生成密钥文件")
	}
}

// 历史默认值必须原样保留（它同时是 secret box 的密钥），但来源要提示出来
func TestResolveSessionSecretKeepsLegacyValueWithWarning(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), SessionSecret: LegacySessionSecret}
	source, err := cfg.ResolveSessionSecret()
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	if cfg.SessionSecret != LegacySessionSecret {
		t.Fatalf("历史默认值必须保留，实际 %q", cfg.SessionSecret)
	}
	if source == "" || source == "环境变量" {
		t.Fatalf("应提示这仍是历史默认值，实际 %q", source)
	}
}

// 密钥文件被写坏时重新生成，而不是带着坏密钥启动
func TestResolveSessionSecretReplacesCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "session_secret"), []byte("short\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{DataDir: dir}
	if _, err := cfg.ResolveSessionSecret(); err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	if len(cfg.SessionSecret) != 64 {
		t.Fatalf("损坏的密钥文件应被替换，实际 %q", cfg.SessionSecret)
	}
}
