package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTerminal 模拟后端终端：写入的内容原样回显，方便验证「输入 → 输出 → 订阅者」链路。
type fakeTerminal struct {
	reader io.ReadCloser
	writer *io.PipeWriter

	mu      sync.Mutex
	resizes []string
	closed  bool
}

func newFakeTerminal() *fakeTerminal {
	pr, pw := io.Pipe()
	return &fakeTerminal{reader: pr, writer: pw}
}

func (f *fakeTerminal) Read(p []byte) (int, error) { return f.reader.Read(p) }

func (f *fakeTerminal) Write(p []byte) (int, error) { return f.writer.Write(p) }

func (f *fakeTerminal) Resize(cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resizes = append(f.resizes, fmt.Sprintf("%dx%d", cols, rows))
	return nil
}

func (f *fakeTerminal) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return f.writer.CloseWithError(io.EOF)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// openWith 返回一个总是成功、返回 fakeTerminal 的 Opener。
func openWith(terms ...*fakeTerminal) (Opener, *[]*fakeTerminal) {
	created := &[]*fakeTerminal{}
	open := func(_ context.Context, _ int64, _, _ int) (Terminal, string, error) {
		term := newFakeTerminal()
		if len(terms) > 0 {
			term = terms[0]
		}
		*created = append(*created, term)
		return term, "测试服务端 · root@127.0.0.1", nil
	}
	return open, created
}

// TestSessionInputEchoesToSubscriber：输入写入后端后，订阅者应收到回显；新订阅者能从回放缓冲补齐。
func TestSessionInputEchoesToSubscriber(t *testing.T) {
	open, _ := openWith()
	manager := NewManager(testLogger(), open, Options{})
	session, err := manager.Create(context.Background(), testCreateOptions(1))
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	defer manager.CloseAll("测试结束")

	chunks, cancel := session.Subscribe()
	defer cancel()
	if err := session.Input([]byte("hello")); err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	if got := waitForOutput(t, chunks); !strings.Contains(got, "hello") {
		t.Fatalf("订阅者未收到回显，实际 %q", got)
	}

	// 第二个订阅者应当先拿到回放缓冲
	replay, replayCancel := session.Subscribe()
	defer replayCancel()
	if got := waitForOutput(t, replay); !strings.Contains(got, "hello") {
		t.Fatalf("回放缓冲未包含历史输出，实际 %q", got)
	}
}

// TestManagerRejectsExtraSessions：并发上限生效。
func TestManagerRejectsExtraSessions(t *testing.T) {
	open, _ := openWith()
	manager := NewManager(testLogger(), open, Options{MaxSessions: 1})
	if _, err := manager.Create(context.Background(), testCreateOptions(1)); err != nil {
		t.Fatalf("第一个会话应当成功：%v", err)
	}
	defer manager.CloseAll("测试结束")
	_, err := manager.Create(context.Background(), testCreateOptions(2))
	if !errors.Is(err, ErrTooManySessions) {
		t.Fatalf("期望 ErrTooManySessions，实际 %v", err)
	}
}

// TestSweepClosesIdleSession：空闲超时后会话被回收。
func TestSweepClosesIdleSession(t *testing.T) {
	open, _ := openWith()
	manager := NewManager(testLogger(), open, Options{IdleTimeout: 10 * time.Millisecond, MaxLifetime: time.Hour})
	session, err := manager.Create(context.Background(), testCreateOptions(1))
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	time.Sleep(30 * time.Millisecond)
	manager.sweep()
	if got := manager.List(); len(got) != 0 {
		t.Fatalf("空闲会话未被回收：%+v", got)
	}
	if snap := session.Snapshot(); !snap.Closed || snap.Reason != "空闲超时" {
		t.Fatalf("关闭原因不符：%+v", snap)
	}
}

// TestDisabledManager：未注入 Opener 时视为功能未启用。
func TestDisabledManager(t *testing.T) {
	manager := NewManager(testLogger(), nil, Options{})
	if manager.Enabled() {
		t.Fatal("nil Opener 应当是未启用状态")
	}
	if _, err := manager.Create(context.Background(), testCreateOptions(1)); !errors.Is(err, ErrDisabled) {
		t.Fatalf("期望 ErrDisabled，实际 %v", err)
	}
}

// TestResizeForwarded：窗口尺寸会转发给后端终端。
func TestResizeForwarded(t *testing.T) {
	term := newFakeTerminal()
	open, _ := openWith(term)
	manager := NewManager(testLogger(), open, Options{})
	session, err := manager.Create(context.Background(), testCreateOptions(1))
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	defer manager.CloseAll("测试结束")
	if err := session.Resize(120, 40); err != nil {
		t.Fatalf("resize 失败：%v", err)
	}
	term.mu.Lock()
	defer term.mu.Unlock()
	if len(term.resizes) != 1 || term.resizes[0] != "120x40" {
		t.Fatalf("resize 未转发到后端：%v", term.resizes)
	}
}

// testCreateOptions 是测试用的建会话入参，带上审计需要的调用方身份。
func testCreateOptions(serverID int64) CreateOptions {
	return CreateOptions{ServerID: serverID, Cols: 80, Rows: 24, User: "admin", ClientIP: "127.0.0.1"}
}

// captureAudit 收集审计写入，供断言「谁、何时、连了哪台、多久、为什么断开」。
type captureAudit struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (c *captureAudit) WriteAudit(_ context.Context, entry AuditEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entry)
	return nil
}

func (c *captureAudit) list() []AuditEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]AuditEntry(nil), c.entries...)
}

// TestCloseWritesAudit：主动关闭时落一条审计，且重复关闭不会写第二条。
func TestCloseWritesAudit(t *testing.T) {
	open, _ := openWith()
	manager := NewManager(testLogger(), open, Options{})
	sink := &captureAudit{}
	manager.SetAuditSink(sink)

	session, err := manager.Create(context.Background(), CreateOptions{
		ServerID: 7, Cols: 120, Rows: 40, User: "admin", ClientIP: "192.168.1.9",
	})
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	if err := session.Input([]byte("whoami\n")); err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	manager.Close(session.ID, "用户关闭")

	entries := sink.list()
	if len(entries) != 1 {
		t.Fatalf("期望 1 条审计，实际 %d 条", len(entries))
	}
	entry := entries[0]
	if entry.SessionID != session.ID || entry.ServerID != 7 || entry.Target == "" {
		t.Fatalf("审计目标字段不符：%+v", entry)
	}
	if entry.User != "admin" || entry.ClientIP != "192.168.1.9" {
		t.Fatalf("审计来源字段不符：%+v", entry)
	}
	if entry.Reason != "用户关闭" {
		t.Fatalf("审计断开原因不符：%+v", entry)
	}
	if entry.StartedAt == "" || entry.EndedAt == "" || entry.DurationMS < 0 {
		t.Fatalf("审计时间字段不符：%+v", entry)
	}
	if entry.BytesIn != int64(len("whoami\n")) {
		t.Fatalf("审计入流量不符：%+v", entry)
	}

	// 幂等：重复关闭不应写第二条
	manager.Close(session.ID, "用户关闭")
	if got := sink.list(); len(got) != 1 {
		t.Fatalf("重复关闭写入了重复审计：%d 条", len(got))
	}
}

// TestRemoteCloseWritesAudit：远端自行退出时同样落审计，并从会话列表里移除。
func TestRemoteCloseWritesAudit(t *testing.T) {
	term := newFakeTerminal()
	open, _ := openWith(term)
	manager := NewManager(testLogger(), open, Options{})
	sink := &captureAudit{}
	manager.SetAuditSink(sink)

	if _, err := manager.Create(context.Background(), testCreateOptions(1)); err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	// 远端断开：pump 读到 EOF 后应当自行收尾
	_ = term.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(sink.list()) == 1 && len(manager.List()) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	entries := sink.list()
	if len(entries) != 1 {
		t.Fatalf("远端断开未写审计：%d 条", len(entries))
	}
	if entries[0].Reason != "会话已结束" {
		t.Fatalf("远端断开原因不符：%+v", entries[0])
	}
	if got := manager.List(); len(got) != 0 {
		t.Fatalf("已结束的会话仍留在列表：%+v", got)
	}
}

// waitForOutput 读取一帧输出，2 秒内没到就返回空串。
func waitForOutput(t *testing.T, chunks <-chan []byte) string {
	t.Helper()
	select {
	case chunk, ok := <-chunks:
		if !ok {
			return ""
		}
		return string(chunk)
	case <-time.After(2 * time.Second):
		return ""
	}
}
