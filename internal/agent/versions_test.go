package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateLegacyVersionDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fonuagent-https.conf")
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
	entries, err := os.ReadDir(filepath.Join(dir, ".fonuagent-https.conf.versions"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("隐藏版本目录应保留 1 个版本：%v %v", entries, err)
	}
}

func TestMigrateVersionDirsFromNginxOutput(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "fonuagent-https.conf.versions")
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
