package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/havline/havline/internal/fsutil"
)

func TestMigrateLegacyVersionDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "havlineagent-https.conf")
	legacy := path + ".versions"
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "20260101T000000.000-abcd.conf"
	if err := os.WriteFile(filepath.Join(legacy, name), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	migrateLegacyVersionDirs(dir)

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("旧版本目录应被移出 nginx include 范围：%v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".havlineagent-https.conf.versions"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("隐藏版本目录应保留 1 个版本：%v %v", entries, err)
	}
}

func TestMigrateVersionDirsFromNginxOutput(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "havlineagent-https.conf.versions")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	output := "2026/09/28 14:38:16 [crit] 6053#6053: pread() \"" + legacy + "\" failed (21: Is a directory)"
	if !migrateVersionDirsFromNginxOutput(output) {
		t.Fatal("应从 nginx 报错文本中识别并迁移旧版本目录")
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("旧版本目录应被移出 include 范围：%v", err)
	}
}

func TestCertificatePin(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	got, err := certificatePin(string(block))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	if want := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("certificatePin() = %s, 期望 %s", got, want)
	}
}

func TestApplyRouteContentSavesPreviousVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "havline-a.example.com.conf")
	if err := os.WriteFile(path, []byte("old config"), 0o640); err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: Config{NginxConfDir: dir}, validateNginxFn: func() error { return nil }}
	if err := server.applyRouteContent(path, []byte("new config")); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != "new config" {
		t.Fatalf("当前配置未更新：%q %v", current, err)
	}
	versions, err := fsutil.ListVersions(path)
	if err != nil || len(versions) != 1 {
		t.Fatalf("写 vhost 前应保存上一版本：%v %v", versions, err)
	}
	previous, err := fsutil.ReadVersion(path, versions[0].Name)
	if err != nil || string(previous) != "old config" {
		t.Fatalf("历史版本内容不符：%q %v", previous, err)
	}
}

func TestApplyRouteContentRestoresOnValidationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "havline-a.example.com.conf")
	if err := os.WriteFile(path, []byte("old config"), 0o640); err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg:             Config{NginxConfDir: dir},
		validateNginxFn: func() error { return errors.New("nginx rejected") },
	}
	if err := server.applyRouteContent(path, []byte("new config")); err == nil {
		t.Fatal("校验失败应返回错误")
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != "old config" {
		t.Fatalf("校验失败后应恢复上一可用版本：%q %v", current, err)
	}
}

func TestApplyRouteContentRemovesNewFileOnValidationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "havline-new.example.com.conf")
	server := &Server{
		cfg:             Config{NginxConfDir: dir},
		validateNginxFn: func() error { return errors.New("nginx rejected") },
	}
	if err := server.applyRouteContent(path, []byte("new config")); err == nil {
		t.Fatal("校验失败应返回错误")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("首次部署失败应删除新文件，避免留下坏配置：%v", err)
	}
}

func TestRollbackRouteReloadsAfterApply(t *testing.T) {
	dir := t.TempDir()
	reloaded := false
	server := &Server{
		cfg:             Config{NginxConfDir: dir},
		validateNginxFn: func() error { return nil },
		reloadNginxFn:   func() error { reloaded = true; return nil },
	}
	path := server.routePath("a.example.com")
	if err := os.WriteFile(path, []byte("healthy"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := fsutil.BackupVersioned(path, maxRouteVersions); err != nil {
		t.Fatal(err)
	}
	versions, err := fsutil.ListVersions(path)
	if err != nil || len(versions) != 1 {
		t.Fatalf("准备版本失败：%v %v", versions, err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0o640); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/routes/{domain}/versions/{name}/rollback", server.rollbackRoute)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes/a.example.com/versions/"+versions[0].Name+"/rollback", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("回滚应成功：%d %s", rec.Code, rec.Body.String())
	}
	if !reloaded {
		t.Fatal("回滚后应执行 nginx reload")
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != "healthy" {
		t.Fatalf("回滚后内容不符：%q %v", current, err)
	}
}
