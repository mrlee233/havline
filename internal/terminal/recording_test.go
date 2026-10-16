package terminal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRecorder 收集写进来的输出，用于验证会话是否把输出接到录制器上。
type fakeRecorder struct {
	mu     sync.Mutex
	buf    strings.Builder
	closed bool
}

func (r *fakeRecorder) Write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf.Write(p)
}

func (r *fakeRecorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
}

func (r *fakeRecorder) text() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

func (r *fakeRecorder) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// TestCastRecorderAndStore：录制写出合法的 asciinema v2 文件，列表能读到元信息，非法 ID 被拒。
func TestCastRecorderAndStore(t *testing.T) {
	dir := t.TempDir()
	store := NewRecordingStore(dir)
	started := time.Now().Add(-3 * time.Second)
	recorder, err := NewCastRecorder(store, "abcd1234", "服务端 #1", 120, 40, started)
	if err != nil {
		t.Fatalf("创建录制失败：%v", err)
	}
	recorder.Write([]byte("hello "))
	// 无效 UTF-8 必须被替换，否则会写出非法 JSON
	recorder.Write([]byte{0xff, 0xfe})
	recorder.Write([]byte("world\r\n"))
	recorder.Close()

	list, err := store.List()
	if err != nil {
		t.Fatalf("列表失败：%v", err)
	}
	if len(list) != 1 {
		t.Fatalf("期望 1 条录制，实际 %d 条", len(list))
	}
	if list[0].SessionID != "abcd1234" || list[0].Size == 0 || list[0].StartedAt == "" || list[0].EndedAt == "" {
		t.Fatalf("录制元信息不完整：%+v", list[0])
	}

	raw, err := os.ReadFile(filepath.Join(dir, "abcd1234.cast"))
	if err != nil {
		t.Fatalf("读取录制失败：%v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 4 {
		t.Fatalf("期望 4 行（头 + 3 帧），实际 %d 行", len(lines))
	}
	var header struct {
		Version int `json:"version"`
		Width   int `json:"width"`
		Height  int `json:"height"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil || header.Version != 2 {
		t.Fatalf("cast 头不合法：%s", lines[0])
	}
	if header.Width != 120 || header.Height != 40 {
		t.Fatalf("cast 头宽高不符：%+v", header)
	}
	var frame []any
	if err := json.Unmarshal([]byte(lines[1]), &frame); err != nil {
		t.Fatalf("帧不是合法 JSON：%s", lines[1])
	}
	if len(frame) != 3 || frame[1] != "o" {
		t.Fatalf("帧结构不符：%+v", frame)
	}

	// 路径穿越与畸形 ID 必须被拒
	for _, id := range []string{"../etc/passwd", "ab", "a/b", ""} {
		if _, ok := store.Path(id); ok {
			t.Fatalf("非法会话 ID %q 不应通过校验", id)
		}
	}

	// 过期录制会被清理
	stale := filepath.Join(dir, "stale1234.cast")
	if err := os.WriteFile(stale, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("准备过期录制失败：%v", err)
	}
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, past, past); err != nil {
		t.Fatalf("调整时间失败：%v", err)
	}
	removed, err := store.Prune(time.Now().Add(-24 * time.Hour))
	if err != nil || removed != 1 {
		t.Fatalf("期望清理 1 条，实际 %d 条 err=%v", removed, err)
	}
}

// TestManagerRecordsOutput：开启录制时输出会流进录制器，会话结束时录制器被关闭。
func TestManagerRecordsOutput(t *testing.T) {
	open, created := openWith()
	manager := NewManager(testLogger(), open, Options{})
	recorder := &fakeRecorder{}
	manager.SetRecorderFactory(func(string, CreateOptions) (OutputRecorder, error) {
		return recorder, nil
	})

	opts := testCreateOptions(1)
	opts.Record = true
	if _, err := manager.Create(context.Background(), opts); err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	if _, err := (*created)[0].writer.Write([]byte("record-me")); err != nil {
		t.Fatalf("向后端写入失败：%v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(recorder.text(), "record-me") {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(recorder.text(), "record-me") {
		t.Fatalf("输出没有进入录制器：%q", recorder.text())
	}

	manager.CloseAll("测试结束")
	if !recorder.isClosed() {
		t.Fatal("会话结束后录制器应当被关闭")
	}
}

// TestManagerSkipsRecordingWhenOff：未开启录制时不创建录制器。
func TestManagerSkipsRecordingWhenOff(t *testing.T) {
	open, _ := openWith()
	manager := NewManager(testLogger(), open, Options{})
	called := false
	manager.SetRecorderFactory(func(string, CreateOptions) (OutputRecorder, error) {
		called = true
		return &fakeRecorder{}, nil
	})
	if _, err := manager.Create(context.Background(), testCreateOptions(1)); err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	manager.CloseAll("测试结束")
	if called {
		t.Fatal("未开启录制时不应创建录制器")
	}
}
