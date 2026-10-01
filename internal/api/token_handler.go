package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/havline/havline/internal/auth"
)

// 令牌管理挂在 AuthHandler 上：它已经持有 auth.Service，不必再加一层 handler。
const maxTokenExpiryDays = 3650

type createTokenRequest struct {
	Name          string `json:"name"`
	Scope         string `json:"scope"`
	ExpiresInDays int    `json:"expires_in_days"`
}

func (h *AuthHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.auth.ListAPITokens(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取令牌失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
}

// CreateToken 新建令牌；明文只在这一个响应里返回，之后无法再取回。
func (h *AuthHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = auth.TokenScopeRead
	}
	var expiresAt time.Time
	if req.ExpiresInDays > 0 {
		if req.ExpiresInDays > maxTokenExpiryDays {
			writeError(r, w, http.StatusBadRequest, "有效期最长 3650 天")
			return
		}
		expiresAt = time.Now().AddDate(0, 0, req.ExpiresInDays)
	}
	token, err := h.auth.CreateAPIToken(r.Context(), req.Name, scope, expiresAt)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, token)
}

func (h *AuthHandler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "无效的令牌 ID")
		return
	}
	if err := h.auth.DeleteAPIToken(r.Context(), id); err != nil {
		if err.Error() == "令牌不存在" {
			writeError(r, w, http.StatusNotFound, err.Error())
			return
		}
		writeError(r, w, http.StatusInternalServerError, "删除令牌失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
