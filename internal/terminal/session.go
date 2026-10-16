package terminal

import (
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"
)

// ErrSessionClosed 表示会话已经关闭，不能再写入。
var ErrSessionClosed = errors.New("终端会话已关闭")

// Terminal 是终端后端的统一抽象：SSH PTY 与本地 PTY 都实现它。
// 会话只依赖这个接口，因此「服务端 SSH」和后续的「本地 Shell」可以共用同一套会话管理。
type Terminal interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// backlogLimit 是每个会话保留的回放缓冲字节数：前端重连或刷新后能补回最近的输出。
const backlogLimit = 64 << 10

// Snapshot 是会话的只读快照，供 HTTP 层展示与清理判断使用。
type Snapshot struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	ServerID  int64     `json:"server_id"`
	Closed    bool      `json:"closed"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
}

// Session 是一个已连接的终端会话：把后端输出分发给订阅者，并保留一小段回放缓冲。
type Session struct {
	ID       string
	Label    string
	ServerID int64

	term Terminal
	// recorder 非空时把输出同步写一份到磁盘（默认关闭，见设置里的会话录制开关）
	recorder OutputRecorder
	// user / clientIP 只在建立会话时写入，供审计记录「谁、从哪来」
	user     string
	clientIP string
	// onClose 由管理器注入：会话自行结束（远端退出、超时）时也要走同一套清理与审计
	onClose func(*Session)

	mu        sync.Mutex
	backlog   []byte
	subs      map[chan []byte]struct{}
	closed    bool
	reason    string
	createdAt time.Time
	lastSeen  time.Time
	endedAt   time.Time
	bytesIn   int64
	bytesOut  int64
}

func newSession(id string, serverID int64, label, user, clientIP string, term Terminal) *Session {
	now := time.Now()
	return &Session{
		ID: id, Label: label, ServerID: serverID, term: term, user: user, clientIP: clientIP,
		subs: map[chan []byte]struct{}{}, createdAt: now, lastSeen: now,
	}
}

// Subscribe 订阅输出：先回放已有缓冲，再接收后续数据。
// cancel 可重复调用；会话关闭时订阅通道会被关闭，读取方据此得知会话结束。
func (s *Session) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 64)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	s.subs[ch] = struct{}{}
	backlog := append([]byte(nil), s.backlog...)
	s.mu.Unlock()
	if len(backlog) > 0 {
		ch <- backlog
	}
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			s.mu.Lock()
			if _, ok := s.subs[ch]; ok {
				delete(s.subs, ch)
				close(ch)
			}
			s.mu.Unlock()
		})
	}
	return ch, cancel
}

// Input 把前端按键写入后端终端。
func (s *Session) Input(p []byte) error {
	s.mu.Lock()
	closed := s.closed
	if !closed {
		s.lastSeen = time.Now()
		s.bytesIn += int64(len(p))
	}
	s.mu.Unlock()
	if closed {
		return ErrSessionClosed
	}
	_, err := s.term.Write(p)
	return err
}

// Resize 同步终端窗口大小。
func (s *Session) Resize(cols, rows int) error {
	if s.Snapshot().Closed {
		return ErrSessionClosed
	}
	return s.term.Resize(cols, rows)
}

// Close 关闭会话：结束后端终端，并让所有订阅者收到关闭。
func (s *Session) Close(reason string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.reason = reason
	s.endedAt = time.Now()
	subs := make([]chan []byte, 0, len(s.subs))
	for ch := range s.subs {
		delete(s.subs, ch)
		subs = append(subs, ch)
	}
	hook := s.onClose
	recorder := s.recorder
	s.recorder = nil
	s.mu.Unlock()
	_ = s.term.Close()
	for _, ch := range subs {
		close(ch)
	}
	if recorder != nil {
		recorder.Close()
	}
	// 只在「第一次关闭」时触发：管理器据此把会话移出列表并写审计
	if hook != nil {
		hook(s)
	}
}

// Snapshot 返回会话的只读快照。
func (s *Session) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{
		ID: s.ID, Label: s.Label, ServerID: s.ServerID,
		Closed: s.closed, Reason: s.reason,
		CreatedAt: s.createdAt, LastSeen: s.lastSeen,
	}
}

// Audit 汇总一条审计记录：会话结束时间、时长、断开原因与收发字节数。
func (s *Session) Audit() AuditEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	ended := s.endedAt
	if ended.IsZero() {
		ended = time.Now()
	}
	reason := s.reason
	if reason == "" {
		reason = "会话已结束"
	}
	return AuditEntry{
		SessionID:  s.ID,
		ServerID:   s.ServerID,
		Target:     s.Label,
		User:       s.user,
		ClientIP:   s.clientIP,
		StartedAt:  s.createdAt.UTC().Format(time.RFC3339),
		EndedAt:    ended.UTC().Format(time.RFC3339),
		DurationMS: ended.Sub(s.createdAt).Milliseconds(),
		Reason:     reason,
		BytesIn:    s.bytesIn,
		BytesOut:   s.bytesOut,
	}
}

// write 把后端输出写入回放缓冲并分发给订阅者。
func (s *Session) write(p []byte) {
	if len(p) == 0 {
		return
	}
	s.mu.Lock()
	s.lastSeen = time.Now()
	s.bytesOut += int64(len(p))
	s.backlog = appendBacklog(s.backlog, p)
	recorder := s.recorder
	subs := make([]chan []byte, 0, len(s.subs))
	for ch := range s.subs {
		subs = append(subs, ch)
	}
	s.mu.Unlock()
	chunk := append([]byte(nil), p...)
	if recorder != nil {
		recorder.Write(chunk)
	}
	for _, ch := range subs {
		select {
		case ch <- chunk:
		default:
			// 订阅者读得慢就丢这一帧：终端是「最新优先」的流，卡住整个会话更糟
		}
	}
}

// startPump 持续读取后端输出；读到 EOF 说明远端结束，主动关闭会话。
func (s *Session) startPump(logger *slog.Logger) {
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := s.term.Read(buf)
			if n > 0 {
				s.write(buf[:n])
			}
			if err == nil {
				continue
			}
			if !errors.Is(err, io.EOF) {
				logger.Debug("终端读取结束", "session_id", s.ID, "error", err.Error())
			}
			s.Close("会话已结束")
			return
		}
	}()
}

// appendBacklog 只保留末尾 backlogLimit 字节，避免长会话把内存吃满。
func appendBacklog(buf, p []byte) []byte {
	buf = append(buf, p...)
	if len(buf) <= backlogLimit {
		return buf
	}
	return append([]byte(nil), buf[len(buf)-backlogLimit:]...)
}
