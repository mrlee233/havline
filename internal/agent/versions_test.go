package agent

import (
	"os"
	"path/filepath"
	"testing"
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
