package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/havline/havline/internal/auth"
	"github.com/havline/havline/internal/frp"
	"github.com/havline/havline/internal/settings"
	"github.com/havline/havline/internal/terminal"
)

// 终端通道的读超时：前端每 30 秒发一次 ping，这里留 3 倍余量。
const terminalReadTimeout = 90 * time.Second

// TerminalHandlerOptions 是终端处理器的装配参数；Manager 为 nil 时相关接口返回不可用。
type TerminalHandlerOptions struct {
	FRP      *frp.Service
	Settings *settings.Store
	Auth     *auth.Service
	Manager  *terminal.Manager
	Audits   *terminal.AuditStore
	// Commands 是「常用命令」存储；为 nil 时相关接口返回空列表
	Commands *terminal.CommandStore
	// Recordings 是会话录制存储；为 nil 时录制接口返回空
	Recordings *terminal.RecordingStore
	// Unlocks 与 AuthHandler 共用同一份解锁票据：改密后要让当前会话立即失效
	Unlocks *terminalUnlockStore
	Logger  *slog.Logger
}

// TerminalHandler 提供内置 SSH 终端：目标列表、会话创建、WebSocket 双向通道与关闭。
type TerminalHandler struct {
	frp        *frp.Service
	settings   *settings.Store
	auth       *auth.Service
	manager    *terminal.Manager
	audits     *terminal.AuditStore
	commands   *terminal.CommandStore
	recordings *terminal.RecordingStore
	unlocks    *terminalUnlockStore
	guard      *loginGuard
	logger     *slog.Logger
	upgrader   websocket.Upgrader
}

// NewTerminalHandler 装配终端处理器。
func NewTerminalHandler(opts TerminalHandlerOptions) *TerminalHandler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	unlocks := opts.Unlocks
	if unlocks == nil {
		unlocks = newTerminalUnlockStore()
	}
	return &TerminalHandler{
		frp: opts.FRP, settings: opts.Settings, auth: opts.Auth, manager: opts.Manager,
		audits: opts.Audits, commands: opts.Commands, recordings: opts.Recordings,
		unlocks: unlocks, guard: newLoginGuard(), logger: logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     allowSameOrigin,
		},
	}
}

// allowSameOrigin 只允许同源页面建立 WebSocket，避免第三方页面借用已登录会话。
func allowSameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

type terminalTarget struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Ready    bool   `json:"ready"`
	Disabled string `json:"disabled_reason,omitempty"`
}

type terminalCreateRequest struct {
	ServerID int64 `json:"server_id"`
	Cols     int   `json:"cols"`
	Rows     int   `json:"rows"`
}

// terminalUnlockRequest 是终端二次认证的请求体。
type terminalUnlockRequest struct {
	Password string `json:"password"`
}

// terminalCommandRequest 是新增快捷命令的请求体。
type terminalCommandRequest struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type terminalClientMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

type terminalServerMessage struct {
	Type    string `json:"type"`
	Data    string `json:"data,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// ListTargets 返回可选目标：只列出已配置 SSH 的服务端，未配置的给出原因。
func (h *TerminalHandler) ListTargets(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(r, w) {
		return
	}
	servers, err := h.frp.ListServers(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取服务端列表失败", err)
		return
	}
	targets := make([]terminalTarget, 0, len(servers))
	for _, server := range servers {
		item := terminalTarget{
			Value:  strconv.FormatInt(server.ID, 10),
			Label:  server.Name,
			Detail: fmt.Sprintf("%s@%s:%d", server.SSHUser, server.SSHHost, server.SSHPort),
			Ready:  server.SSHConfigured,
		}
		if !server.SSHConfigured {
			item.Disabled = "未配置 SSH，请到「公网服务端」填写"
		}
		targets = append(targets, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": targets})
}

// ListRecordings 返回磁盘上的会话录制，供「录像审计」列表使用。
func (h *TerminalHandler) ListRecordings(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(r, w) {
		return
	}
	if h.recordings == nil {
		writeJSON(w, http.StatusOK, map[string]any{"recordings": []terminal.Recording{}})
		return
	}
	items, err := h.recordings.List()
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取录制列表失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recordings": items})
}

// GetRecording 读取一段录制（asciinema v2 文本），供前端回放。
func (h *TerminalHandler) GetRecording(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(r, w) {
		return
	}
	if h.recordings == nil {
		writeError(r, w, http.StatusNotFound, "录制不存在或已被清理")
		return
	}
	id := r.PathValue("id")
	path, ok := h.recordings.Path(id)
	if !ok {
		writeError(r, w, http.StatusBadRequest, "录制 ID 不合法")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeError(r, w, http.StatusNotFound, "录制不存在或已被清理")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取录制失败", err)
		return
	}
	// 录制内容按原样返回：前端按 asciinema v2 解析后逐帧回放
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeContent(w, r, id+".cast", info.ModTime(), file)
}

// ListCommands 返回已保存的快捷命令（右侧「常用命令」面板）。
func (h *TerminalHandler) ListCommands(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(r, w) {
		return
	}
	if h.commands == nil {
		writeJSON(w, http.StatusOK, map[string]any{"commands": []terminal.TerminalCommand{}})
		return
	}
	commands, err := h.commands.ListCommands(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取常用命令失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"commands": commands})
}

// CreateCommand 保存一条快捷命令。
func (h *TerminalHandler) CreateCommand(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(r, w) {
		return
	}
	if h.commands == nil {
		writeError(r, w, http.StatusServiceUnavailable, "常用命令当前不可用")
		return
	}
	var req terminalCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	command, err := h.commands.CreateCommand(r.Context(), req.Name, req.Command)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error(), err)
		return
	}
	writeJSON(w, http.StatusCreated, command)
}

// DeleteCommand 删除一条快捷命令。
func (h *TerminalHandler) DeleteCommand(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(r, w) {
		return
	}
	if h.commands == nil {
		writeError(r, w, http.StatusServiceUnavailable, "常用命令当前不可用")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(r, w, http.StatusBadRequest, "命令 ID 无效")
		return
	}
	if err := h.commands.DeleteCommand(r.Context(), id); err != nil {
		if errors.Is(err, terminal.ErrCommandNotFound) {
			writeError(r, w, http.StatusNotFound, err.Error())
			return
		}
		writeError(r, w, http.StatusInternalServerError, "删除常用命令失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// UnlockStatus 返回当前登录会话的终端解锁状态，供前端显示剩余时间。
func (h *TerminalHandler) UnlockStatus(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(r, w) {
		return
	}
	expires, ok := h.unlocks.expiresAt(sessionIDFromContext(r.Context()))
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"unlocked": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"unlocked":   true,
		"expires_at": expires.UTC().Format(time.RFC3339),
	})
}

// Unlock 校验管理员密码，换取终端短时解锁。
// 这是「登录态点一下就能拿到 root shell」与「还需要知道一个密码」之间的那道门。
func (h *TerminalHandler) Unlock(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(r, w) {
		return
	}
	sessionID := sessionIDFromContext(r.Context())
	if sessionID == "" {
		// 终端不接受 API Token，二次认证也必须走浏览器会话
		writeErrorCode(r, w, http.StatusForbidden, "terminal_session_required", "终端二次认证需要使用浏览器登录会话")
		return
	}
	ip := clientIP(r)
	if h.guard.blocked(ip) {
		writeErrorCode(r, w, http.StatusTooManyRequests, "too_many_attempts", "密码错误次数过多，请 5 分钟后再试")
		return
	}
	var req terminalUnlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	adminID, err := h.auth.AdminIDForSession(r.Context(), sessionID)
	if err != nil {
		writeError(r, w, http.StatusUnauthorized, "未登录或会话已过期")
		return
	}
	// 密码原样比对：不做 TrimSpace，避免把首尾空格当作无关字符
	if err := h.auth.VerifyPassword(r.Context(), adminID, req.Password); err != nil {
		h.guard.fail(ip)
		h.logger.Warn("终端二次认证失败", "module", "TERMINAL", "client_ip", ip, "admin_id", adminID)
		writeErrorCode(r, w, http.StatusUnauthorized, "bad_password", "管理员密码不正确")
		return
	}
	h.guard.reset(ip)
	expires := h.unlocks.grant(sessionID)
	h.logger.Info("终端二次认证通过",
		"module", "TERMINAL", "client_ip", ip, "admin_id", adminID,
		"expires_at", expires.UTC().Format(time.RFC3339))
	writeJSON(w, http.StatusOK, map[string]any{"expires_at": expires.UTC().Format(time.RFC3339)})
}

// Lock 主动锁定当前会话的终端解锁，之后新建会话需要重新输入密码。
func (h *TerminalHandler) Lock(w http.ResponseWriter, r *http.Request) {
	h.unlocks.revoke(sessionIDFromContext(r.Context()))
	h.logger.Info("终端已锁定", "module", "TERMINAL", "client_ip", clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// CreateSession 建立一条新的 SSH 终端会话。
func (h *TerminalHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(r, w) {
		return
	}
	// 二次认证：没有有效解锁票据（或已过期）就不许建会话
	if _, unlocked := h.unlocks.expiresAt(sessionIDFromContext(r.Context())); !unlocked {
		writeErrorCode(r, w, http.StatusForbidden, "terminal_locked", "终端需要二次认证：请先输入管理员密码解锁")
		return
	}
	var req terminalCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	if req.ServerID <= 0 {
		writeError(r, w, http.StatusBadRequest, "请选择要连接的服务端")
		return
	}
	// SSH 认证 + PTY 申请需要时间，单独给 30 秒上限
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	user, clientIP := h.terminalActor(r)
	// 录制开关默认关闭：开启后本次会话输出会写入 logs/terminal
	record, err := h.settings.GetBool(r.Context(), settings.KeyTerminalRecording)
	if err != nil {
		record = false
	}
	session, err := h.manager.Create(ctx, terminal.CreateOptions{
		ServerID: req.ServerID,
		Cols:     req.Cols,
		Rows:     req.Rows,
		User:     user,
		ClientIP: clientIP,
		Record:   record,
	})
	if err != nil {
		writeTerminalError(r, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": session.ID, "label": session.Label})
}

// ListAudits 返回最近的终端会话审计：谁、何时、连了哪台、多久、为什么断开。
func (h *TerminalHandler) ListAudits(w http.ResponseWriter, r *http.Request) {
	if h.audits == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []terminal.AuditEntry{}})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := h.audits.ListAudits(r.Context(), limit)
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取终端审计记录失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// terminalActor 解析审计需要的调用方身份：登录用户名与来源 IP。
// 用 API Token 调用时没有会话 Cookie，这里如实记成「API Token」而不是伪造用户名。
func (h *TerminalHandler) terminalActor(r *http.Request) (string, string) {
	ip := clientIP(r)
	if bearerToken(r) != "" {
		return "API Token", ip
	}
	sessionID := sessionIDFromContext(r.Context())
	if sessionID == "" || h.auth == nil {
		return "未知", ip
	}
	adminID, err := h.auth.AdminIDForSession(r.Context(), sessionID)
	if err != nil {
		return "未知", ip
	}
	username, err := h.auth.AdminUsername(r.Context(), adminID)
	if err != nil || strings.TrimSpace(username) == "" {
		return "未知", ip
	}
	return username, ip
}

// ListSessions 返回当前会话快照（前端刷新后清理与排查用）。
func (h *TerminalHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": h.manager.List()})
}

// CloseSession 主动关闭会话。用户点「关闭」或页面卸载时调用。
func (h *TerminalHandler) CloseSession(w http.ResponseWriter, r *http.Request) {
	h.manager.Close(r.PathValue("id"), "用户关闭")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Stream 是终端双向通道：浏览器发送按键/尺寸，服务端推送输出与结束原因。
func (h *TerminalHandler) Stream(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(r, w) {
		return
	}
	id := r.PathValue("id")
	session, ok := h.manager.Get(id)
	if !ok {
		writeError(r, w, http.StatusNotFound, "终端会话不存在或已关闭")
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	go h.readClient(conn, session, id)
	h.writeClient(conn, session)
}

// writeClient 把会话输出推给浏览器；订阅通道关闭表示会话结束。
func (h *TerminalHandler) writeClient(conn *websocket.Conn, session *terminal.Session) {
	chunks, cancel := session.Subscribe()
	defer cancel()
	for chunk := range chunks {
		message := terminalServerMessage{Type: "output", Data: base64.StdEncoding.EncodeToString(chunk)}
		if err := writeSocketMessage(conn, message); err != nil {
			return
		}
	}
	_ = writeSocketMessage(conn, terminalServerMessage{Type: "exit", Reason: session.Snapshot().Reason})
}

// readClient 处理按键与窗口尺寸；连接断开时立刻回收会话。
func (h *TerminalHandler) readClient(conn *websocket.Conn, session *terminal.Session, id string) {
	conn.SetReadLimit(1 << 20)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(terminalReadTimeout))
		_, payload, err := conn.ReadMessage()
		if err != nil {
			h.manager.Close(id, "前端断开")
			return
		}
		var message terminalClientMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			continue
		}
		h.dispatchClientMessage(session, message)
	}
}

func (h *TerminalHandler) dispatchClientMessage(session *terminal.Session, message terminalClientMessage) {
	switch message.Type {
	case "input":
		data, err := base64.StdEncoding.DecodeString(message.Data)
		if err != nil {
			return
		}
		_ = session.Input(data)
	case "resize":
		_ = session.Resize(message.Cols, message.Rows)
	}
}

func writeSocketMessage(conn *websocket.Conn, message terminalServerMessage) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, payload)
}

// terminalEnabled 读设置开关：缺省视为关闭（shell 入口必须显式打开）。
func (h *TerminalHandler) terminalEnabled(r *http.Request) (bool, error) {
	// 用 GetBool 而不是只认 "1"：设置页写 "1"/"0"，但 true/on 这类写法也应被认作开启，
	// 否则开关看着是开的、接口却按未启用拒绝，排查起来很费劲
	return h.settings.GetBool(r.Context(), settings.KeyTerminalEnabled)
}

// requireEnabled 检查设置开关与运行能力；未通过时直接写出错误并返回 false。
func (h *TerminalHandler) requireEnabled(r *http.Request, w http.ResponseWriter) bool {
	enabled, err := h.terminalEnabled(r)
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取设置失败", err)
		return false
	}
	if !enabled {
		writeError(r, w, http.StatusForbidden, "SSH 终端未启用：请到「系统设置」打开后再使用")
		return false
	}
	if h.manager == nil || !h.manager.Enabled() {
		writeError(r, w, http.StatusServiceUnavailable, "SSH 终端当前不可用")
		return false
	}
	return true
}

func writeTerminalError(r *http.Request, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, terminal.ErrDisabled):
		writeError(r, w, http.StatusForbidden, "SSH 终端未启用")
	case errors.Is(err, terminal.ErrTooManySessions):
		writeError(r, w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, terminal.ErrNotFound):
		writeError(r, w, http.StatusNotFound, err.Error())
	default:
		writeError(r, w, http.StatusBadGateway, err.Error())
	}
}
