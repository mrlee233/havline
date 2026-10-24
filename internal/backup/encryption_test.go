package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// listEntries 读出归档里的条目名，用于断言包含关系。
func listEntries(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	names := []string{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
}

// TestExportIncludesSessionSecretAndFrpConfig：备份必须带上凭据加密密钥与 frpc 配置。
// 少了 session_secret，恢复后库里的 DDNS / FRP / Cloudflare / 通知凭据全部解不开。
func TestExportIncludesSessionSecretAndFrpConfig(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "havline.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "session_secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	frpDir := filepath.Join(dataDir, "frp")
	if err := os.MkdirAll(frpDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frpDir, "frpc-server-1.toml"), []byte("token = \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := New(dataDir).Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entries := listEntries(t, path)
	has := map[string]bool{}
	for _, name := range entries {
		has[name] = true
	}
	for _, want := range []string{"havline.db", "session_secret", "frp/frpc-server-1.toml"} {
		if !has[want] {
			t.Fatalf("备份应包含 %q，实际条目：%v", want, entries)
		}
	}
}

// TestExportSkipsFrpBinariesAndLogs：frp/bin、frp/downloads 与 *.log 体积大且可重新获取，不进归档。
func TestExportSkipsFrpBinariesAndLogs(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "havline.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	frpDir := filepath.Join(dataDir, "frp")
	for _, dir := range []string{filepath.Join(frpDir, "bin", "v0.70.1"), filepath.Join(frpDir, "downloads")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(frpDir, "bin", "v0.70.1", "frpc"): "binary",
		filepath.Join(frpDir, "downloads", "frpc.zip"):  "zip",
		filepath.Join(frpDir, "frpc-server-1.log"):      "log",
		filepath.Join(frpDir, "frpc-server-1.toml"):     "token = \"x\"\n",
		filepath.Join(frpDir, "active.json"):            "{}",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	path, err := New(dataDir).Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entries := listEntries(t, path)
	has := map[string]bool{}
	for _, name := range entries {
		has[name] = true
	}
	for _, skip := range []string{"frp/bin/v0.70.1/frpc", "frp/downloads/frpc.zip", "frp/frpc-server-1.log"} {
		if has[skip] {
			t.Fatalf("归档不应包含 %q，实际条目：%v", skip, entries)
		}
	}
	for _, want := range []string{"frp/frpc-server-1.toml", "frp/active.json"} {
		if !has[want] {
			t.Fatalf("归档应保留 %q，实际条目：%v", want, entries)
		}
	}
}

// TestExportWithPassphraseIsEncrypted：给了口令就必须是加密容器，且不带口令读不出来。
func TestExportWithPassphraseIsEncrypted(t *testing.T) {
	dataDir := seedDataDir(t)
	path, err := New(dataDir).ExportWith(context.Background(), Options{Passphrase: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := IsEncrypted(path)
	if err != nil {
		t.Fatal(err)
	}
	if !encrypted {
		t.Fatal("给了口令导出的归档应被标记为加密")
	}

	// 无口令恢复：必须明确报「需要口令」，而不是含糊的解压失败
	target := t.TempDir()
	if err := New(target).Restore(context.Background(), path); !errors.Is(err, ErrPassphraseRequired) {
		t.Fatalf("无口令恢复应报 ErrPassphraseRequired，实际 %v", err)
	}

	// 错口令：GCM 校验失败
	if err := New(target).RestoreWith(context.Background(), path, Options{Passphrase: "wrong"}); err == nil {
		t.Fatal("错误口令不应恢复成功")
	}

	// 正确口令：能恢复出原来的文件内容
	if err := New(target).RestoreWith(context.Background(), path, Options{Passphrase: "correct horse"}); err != nil {
		t.Fatalf("正确口令应恢复成功：%v", err)
	}
	data, err := os.ReadFile(filepath.Join(target, "havline.db"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "db" {
		t.Fatalf("恢复内容不符：%q", data)
	}
}

// TestExportWithoutPassphraseStaysCompatible：不给口令时仍产出未加密归档，
// 老版本与现有客户端可以直接解压。
func TestExportWithoutPassphraseStaysCompatible(t *testing.T) {
	dataDir := seedDataDir(t)
	path, err := New(dataDir).Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := IsEncrypted(path)
	if err != nil {
		t.Fatal(err)
	}
	if encrypted {
		t.Fatal("未给口令时的归档不应加密")
	}
	if _, err := New(t.TempDir()).Verify(context.Background(), path); err != nil {
		t.Fatalf("未加密归档应能通过校验：%v", err)
	}
}

// TestVerifyWithEncryptedArchiveNeedsPassphrase：加密归档校验同样需要口令，
// 否则自动备份会把一份好备份判成坏包删掉。
func TestVerifyWithEncryptedArchiveNeedsPassphrase(t *testing.T) {
	dataDir := seedDataDir(t)
	path, err := New(dataDir).ExportWith(context.Background(), Options{Passphrase: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(dataDir).Verify(context.Background(), path); !errors.Is(err, ErrPassphraseRequired) {
		t.Fatalf("无口令校验应报 ErrPassphraseRequired，实际 %v", err)
	}
	count, err := New(dataDir).VerifyWith(context.Background(), path, Options{Passphrase: "pw"})
	if err != nil {
		t.Fatalf("带口令校验应通过：%v", err)
	}
	if count == 0 {
		t.Fatal("校验应返回条目数")
	}
}

// TestMissingSessionSecretDetectsLegacyArchive：旧归档没有 session_secret，
// 能恢复但凭据解不开，要能被识别出来提示用户。
func TestMissingSessionSecretDetectsLegacyArchive(t *testing.T) {
	dataDir := t.TempDir()
	// 手工造一个「只有 havline.db」的旧式归档
	legacy := writeArchive(t, map[string]string{"havline.db": "db"})
	svc := New(dataDir)

	missing, err := svc.MissingSessionSecret(context.Background(), legacy, Options{})
	if err != nil {
		t.Fatalf("识别旧归档不应报错：%v", err)
	}
	if !missing {
		t.Fatal("旧归档应被判定为缺少 session_secret")
	}

	fresh := seedExport(t, seedDataDir(t))
	missing, err = svc.MissingSessionSecret(context.Background(), fresh, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if missing {
		t.Fatal("新归档包含 session_secret，不应判定为缺失")
	}
}

// TestRestoreKeepsSecretPermissions：session_secret 还原后必须是 0600，
// 否则同机其他用户就能读到解密凭据的钥匙。
func TestRestoreKeepsSecretPermissions(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows 不保留 Unix 权限位")
	}
	dataDir := seedDataDir(t)
	path, err := New(dataDir).Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := New(target).Restore(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(target, "session_secret"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != restoreSecretMode {
		t.Fatalf("session_secret 还原后权限应为 %o，实际 %o", restoreSecretMode, got)
	}
}

func seedDataDir(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "havline.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "session_secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

func seedExport(t *testing.T, dataDir string) string {
	t.Helper()
	path, err := New(dataDir).Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
