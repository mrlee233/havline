package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadLimitStateFile(t *testing.T) {
	dir := t.TempDir()

	missing, err := readLimitStateFile(filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatalf("文件不存在应返回空状态: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("文件不存在应返回空 map，实际 %#v", missing)
	}

	validPath := filepath.Join(dir, "valid.json")
	valid := []byte(`{"a.example.com":{"rate":10,"burst":20,"conn":5}}`)
	if err := os.WriteFile(validPath, valid, 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := readLimitStateFile(validPath)
	if err != nil {
		t.Fatal(err)
	}
	got := state["a.example.com"]
	if got.Rate != 10 || got.Burst != 20 || got.Conn != 5 {
		t.Fatalf("解析结果不正确: %#v", got)
	}

	brokenPath := filepath.Join(dir, "broken.json")
	broken := []byte(`{"a.example.com":`)
	if err := os.WriteFile(brokenPath, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readLimitStateFile(brokenPath); err == nil {
		t.Fatal("损坏 JSON 应返回错误")
	}
	after, err := os.ReadFile(brokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(broken) {
		t.Fatalf("读取失败不应修改原文件: %q", after)
	}

	emptyPath := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readLimitStateFile(emptyPath); err == nil {
		t.Fatal("空文件应返回错误")
	}

	nullPath := filepath.Join(dir, "null.json")
	if err := os.WriteFile(nullPath, []byte("null\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readLimitStateFile(nullPath); err == nil {
		t.Fatal("null 内容应返回错误")
	}
}
