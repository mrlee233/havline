package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/havline/havline/internal/auth"
	"github.com/havline/havline/internal/notify"
)

type AuthHandler struct {
	auth   *auth.Service
	notify *notify.Service
	guard  *loginGuard
}

func NewAuthHandler(authSvc *auth.Service, notifySvc *notify.Service) *AuthHandler {
	return &AuthHandler{auth: authSvc, notify: notifySvc, guard: newLoginGuard()}
}

const (
	loginFailureLimit  = 5
	loginLockoutWindow = 5 * time.Minute
)

// loginGuard 是进程内的登录失败退避：同一 IP 在窗口内失败到阈值后直接拒绝，
// 避免明文 HTTP 部署下被无限爆破；与用于通知的失败统计相互独立。
type loginGuard struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

func newLoginGuard() *loginGuard {
	return &loginGuard{failures: make(map[string][]time.Time)}
}

// blocked 返回该 IP 是否仍在锁定窗口内（顺带清理过期记录）。
func (g *loginGuard) blocked(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	kept := g.recentLocked(ip)
	g.failures[ip] = kept
	return len(kept) >= loginFailureLimit
}

func (g *loginGuard) fail(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.failures) > 1024 {
		for key := range g.failures {
			if len(g.recentLocked(key)) == 0 {
				delete(g.failures, key)
			}
		}
	}
	g.failures[ip] = append(g.recentLocked(ip), time.Now())
}

func (g *loginGuard) reset(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.failures, ip)
}

// recentLocked 返回窗口内的失败时间（调用方需持有锁）。
func (g *loginGuard) recentLocked(ip string) []time.Time {
	now := time.Now()
	kept := make([]time.Time, 0, len(g.failures[ip]))
	for _, at := range g.failures[ip] {
		if now.Sub(at) < loginLockoutWindow {
			kept = append(kept, at)
		}
	}
	return kept
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authStatusResponse struct {
	Initialized   bool `json:"initialized"`
	Authenticated bool `json:"authenticated"`
}

func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	initialized, err := h.auth.IsInitialized(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取初始化状态失败")
		return
	}
	authenticated := false
	if sessionID := sessionIDFromContext(r.Context()); sessionID != "" {
		if err := h.auth.ValidateSession(r.Context(), sessionID); err == nil {
			authenticated = true
		}
	}
	writeJSON(w, http.StatusOK, authStatusResponse{
		Initialized:   initialized,
		Authenticated: authenticated,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	ip := clientIP(r)
	if h.guard.blocked(ip) {
		writeError(r, w, http.StatusTooManyRequests, "登录失败次数过多，请稍后再试")
		return
	}
	sessionID, err := h.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			h.guard.fail(ip)
			if h.notify != nil {
				h.notify.RecordLoginFailure(r.Context(), ip)
			}
			writeError(r, w, http.StatusUnauthorized, "用户名或密码错误")
			return
		}
		writeError(r, w, http.StatusInternalServerError, "登录失败")
		return
	}
	h.guard.reset(ip)
	setSessionCookie(w, r, sessionID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "登录成功"})
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	sessionID := sessionIDFromContext(r.Context())
	adminID, err := h.auth.AdminIDForSession(r.Context(), sessionID)
	if err != nil {
		writeError(r, w, http.StatusUnauthorized, "未登录或会话已过期")
		return
	}
	if err := h.auth.ChangePassword(r.Context(), adminID, req.OldPassword, req.NewPassword, sessionID); err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "密码已更新"})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if sessionID := sessionIDFromContext(r.Context()); sessionID != "" {
		_ = h.auth.Logout(r.Context(), sessionID)
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"message": "已退出登录"})
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "havline_session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
	})
}

// requestIsHTTPS：直连 TLS 或经反代（X-Forwarded-Proto）时给会话 cookie 加 Secure；
// 默认明文 HTTP 部署下不加，否则浏览器会直接丢弃 cookie 导致登录不了。
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "havline_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
