package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/havline/havline/internal/auth"
)

type contextKey string

const sessionContextKey contextKey = "session_id"

func SessionMiddleware(authSvc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("havline_session")
			if err == nil && cookie.Value != "" {
				ctx := context.WithValue(r.Context(), sessionContextKey, cookie.Value)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth 接受两种凭据：浏览器的会话 cookie，以及 Authorization: Bearer <api token>。
// 令牌分读写两种范围：read 只允许 GET/HEAD，write 与会话同权。
func RequireAuth(authSvc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token := bearerToken(r); token != "" {
				scope, err := authSvc.ValidateAPIToken(r.Context(), token)
				if err != nil {
					writeError(r, w, http.StatusUnauthorized, "API Token 无效或已过期")
					return
				}
				// 账号与令牌管理只允许浏览器会话：否则一个 write 令牌可以自己给自己提权
				if tokenDeniedPath(r.URL.Path) {
					writeError(r, w, http.StatusForbidden, "该操作需要使用登录会话")
					return
				}
				if scope != auth.TokenScopeWrite && r.Method != http.MethodGet && r.Method != http.MethodHead {
					writeError(r, w, http.StatusForbidden, "该 Token 只读，不能执行写操作")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			sessionID, _ := r.Context().Value(sessionContextKey).(string)
			if err := authSvc.ValidateSession(r.Context(), sessionID); err != nil {
				writeError(r, w, http.StatusUnauthorized, "未登录或会话已过期")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken 取出 Authorization: Bearer 里的明文令牌；没有或格式不对返回空串。
func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// tokenDeniedPath 是只允许浏览器会话的路径（账号与令牌管理）。
func tokenDeniedPath(path string) bool {
	if path == "/api/auth/password" || path == "/api/auth/logout" {
		return true
	}
	return strings.HasPrefix(path, "/api/settings/tokens") ||
		strings.HasPrefix(path, "/api/system/update")
}

func sessionIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(sessionContextKey).(string)
	return v
}
