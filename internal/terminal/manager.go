package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

var (
	// ErrDisabled 表示没有注入 Opener（终端功能未启用）。
	ErrDisabled = errors.New("SSH 终端未启用")
	// ErrTooManySessions 表示并发会话已达上限。
	ErrTooManySessions = errors.New("并发终端会话数已达上限")
	// ErrNotFound 表示会话不存在或已被回收。
	ErrNotFound = errors.New("终端会话不存在或已关闭")
)

// Opener 由调用方注入：按服务端 ID 建立一个已认证的终端。
// 实现放在 frp 包，复用它的 SSH 凭据解密与 known_hosts 校验，避免重复实现一套认证。
type Opener func(ctx context.Context, serverID int64, cols, rows int) (Terminal, string, error)

// CreateOptions 是建立会话的入参：连哪台、初始尺寸，以及审计需要的调用方身份。
type CreateOptions struct {
	ServerID int64
	Cols     int
	Rows     int
	// User / ClientIP 只用于写审计记录，不参与连接
	User     string
	ClientIP string
	// Record 为真且注入了 RecorderFactory 时，本次会话输出会写入录制文件
	Record bool
}

// RecorderFactory 按会话创建录制器；录制失败只记日志，不影响会话本身。
type RecorderFactory func(sessionID string, opts CreateOptions) (OutputRecorder, error)

// Options 是并发上限与超时配置。
type Options struct {
	MaxSessions int
	IdleTimeout time.Duration
	MaxLifetime time.Duration
}

// Manager 管理所有终端会话：创建、查找、关闭与超时回收。
type Manager struct {
	logger *slog.Logger
	open   Opener

	mu        sync.Mutex
	sessions  map[string]*Session
	pending   int
	max       int
	idleAfter time.Duration
	maxAge    time.Duration
	audit     AuditSink
	recorder  RecorderFactory
}

// NewManager 创建会话管理器；open 为 nil 时视为功能未启用。
func NewManager(logger *slog.Logger, open Opener, opts Options) *Manager {
	if opts.MaxSessions <= 0 {
		opts.MaxSessions = 5
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = 10 * time.Minute
	}
	if opts.MaxLifetime <= 0 {
		opts.MaxLifetime = 12 * time.Hour
	}
	return &Manager{
		logger: logger, open: open, sessions: map[string]*Session{},
		max: opts.MaxSessions, idleAfter: opts.IdleTimeout, maxAge: opts.MaxLifetime,
	}
}

// Enabled 返回当前是否具备建立会话的能力。
func (m *Manager) Enabled() bool { return m.open != nil }

// SetAuditSink 注入审计存储；未注入时会话审计只落日志（测试与未启用审计的场景）。
func (m *Manager) SetAuditSink(sink AuditSink) {
	m.mu.Lock()
	m.audit = sink
	m.mu.Unlock()
}

// SetRecorderFactory 注入会话录制工厂；未注入时即使请求录制也不会写文件。
func (m *Manager) SetRecorderFactory(factory RecorderFactory) {
	m.mu.Lock()
	m.recorder = factory
	m.mu.Unlock()
}

// Create 建立新会话：超过并发上限或后端连接失败时返回错误。
func (m *Manager) Create(ctx context.Context, opts CreateOptions) (*Session, error) {
	if m.open == nil {
		return nil, ErrDisabled
	}
	if err := m.reserve(); err != nil {
		return nil, err
	}
	term, label, err := m.open(ctx, opts.ServerID, normCols(opts.Cols), normRows(opts.Rows))
	if err != nil {
		m.finishReserve(nil)
		return nil, err
	}
	session := newSession(newSessionID(), opts.ServerID, label, opts.User, opts.ClientIP, term)
	// 会话自行结束（远端退出、读写出错）时也要移出列表并写审计，因此挂在会话上
	session.onClose = m.handleSessionClosed
	if opts.Record {
		m.attachRecorder(session, opts)
	}
	session.startPump(m.logger)
	m.finishReserve(session)
	m.logger.Info("终端会话已建立",
		"session_id", session.ID, "server_id", opts.ServerID, "label", label,
		"user", opts.User, "client_ip", opts.ClientIP)
	return session, nil
}

// attachRecorder 按需挂上录制器；失败只记日志，绝不因此拒绝建立会话。
func (m *Manager) attachRecorder(session *Session, opts CreateOptions) {
	m.mu.Lock()
	factory := m.recorder
	m.mu.Unlock()
	if factory == nil {
		return
	}
	recorder, err := factory(session.ID, opts)
	if err != nil {
		m.logger.Warn("终端录制未启用：无法创建录制文件",
			"module", "TERMINAL", "session_id", session.ID, "error", err.Error())
		return
	}
	session.recorder = recorder
	m.logger.Info("终端会话开始录制", "module", "TERMINAL", "session_id", session.ID, "server_id", opts.ServerID)
}

// Get 返回会话；不存在时返回 false。会话可能已关闭，调用方需自行判断原因。
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	return session, ok
}

// Close 关闭并移除会话；会话不存在时幂等返回。
func (m *Manager) Close(id, reason string) {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	m.logger.Info("终端会话已关闭", "session_id", id, "server_id", session.ServerID, "reason", reason)
	// 关闭动作会触发 handleSessionClosed：移出列表 + 写审计
	session.Close(reason)
}

// CloseAll 关闭全部会话（服务停止或用户点「全部关闭」）。
func (m *Manager) CloseAll(reason string) {
	for _, snap := range m.List() {
		m.Close(snap.ID, reason)
	}
}

// List 返回当前所有会话的只读快照。
func (m *Manager) List() []Snapshot {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.Unlock()
	out := make([]Snapshot, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, session.Snapshot())
	}
	return out
}

// StartSweeper 定期回收空闲超时、超长与已结束的会话；ctx 取消时关闭全部会话。
func (m *Manager) StartSweeper(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				m.CloseAll("服务停止")
				return
			case <-ticker.C:
				m.sweep()
			}
		}
	}()
}

// reserve 预占一个会话名额：把「正在建立」的会话也计入上限，避免并发点击绕过限制。
func (m *Manager) reserve() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sessions)+m.pending >= m.max {
		return ErrTooManySessions
	}
	m.pending++
	return nil
}

// finishReserve 结束预占：session 非空时登记为新会话。
func (m *Manager) finishReserve(session *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending--
	// 会话可能在登记前就被远端结束并自行回收，这种情况不再登记，避免列表里留下死会话
	if session != nil && !session.Snapshot().Closed {
		m.sessions[session.ID] = session
	}
}

// handleSessionClosed 是会话的唯一收尾入口：会话无论被谁关闭都会走到这里。
func (m *Manager) handleSessionClosed(session *Session) {
	m.mu.Lock()
	delete(m.sessions, session.ID)
	m.mu.Unlock()
	m.recordAudit(session)
}

// recordAudit 写一条审计：注入了存储就落库，否则退化为结构化日志。
func (m *Manager) recordAudit(session *Session) {
	entry := session.Audit()
	m.mu.Lock()
	sink := m.audit
	m.mu.Unlock()
	if sink == nil {
		m.logger.Info("终端会话审计",
			"module", "TERMINAL",
			"session_id", entry.SessionID,
			"server_id", entry.ServerID,
			"target", entry.Target,
			"user", entry.User,
			"client_ip", entry.ClientIP,
			"duration_ms", entry.DurationMS,
			"reason", entry.Reason)
		return
	}
	// 关闭动作可能来自请求 goroutine（其 ctx 随响应结束而取消），这里用独立超时写库
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sink.WriteAudit(ctx, entry); err != nil {
		m.logger.Warn("终端审计写入失败", "module", "TERMINAL", "session_id", entry.SessionID, "error", err.Error())
	}
}

func (m *Manager) sweep() {
	now := time.Now()
	for _, snap := range m.List() {
		if snap.Closed {
			reason := snap.Reason
			if reason == "" {
				reason = "会话已结束"
			}
			m.Close(snap.ID, reason)
			continue
		}
		if now.Sub(snap.LastSeen) > m.idleAfter {
			m.Close(snap.ID, "空闲超时")
			continue
		}
		if now.Sub(snap.CreatedAt) > m.maxAge {
			m.Close(snap.ID, "超过最长会话时长")
		}
	}
}

func newSessionID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

// normCols 把前端传来的列数夹到合理范围，避免异常值让远端 PTY 申请失败。
func normCols(cols int) int {
	if cols < 20 {
		return 80
	}
	if cols > 500 {
		return 500
	}
	return cols
}

// normRows 把前端传来的行数夹到合理范围。
func normRows(rows int) int {
	if rows < 5 {
		return 24
	}
	if rows > 300 {
		return 300
	}
	return rows
}
