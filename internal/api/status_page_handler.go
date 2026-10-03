package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/havline/havline/internal/cloudflared"
	"github.com/havline/havline/internal/frp"
	"github.com/havline/havline/internal/notify"
	"github.com/havline/havline/internal/settings"
)

// statusPageWindow / statusPageLimit 匿名访问的限流窗口与配额：
// 状态页会被扫描器盯上，这里比登录接口更宽松但仍有限度。
const (
	statusPageWindow = time.Minute
	statusPageLimit  = 60
)

// StatusPageHandler 提供**匿名只读**状态页数据，默认关闭。
//
// 刻意只暴露域名与可用率：不带内网地址、端口、证书路径、服务端地址——
// 状态页是要挂到公网上的东西，泄露拓扑就等于把内网结构摊开给别人看。
type StatusPageHandler struct {
	settings   *settings.Store
	frp        *frp.Service
	cloudflare *cloudflared.Service
	limiter    *statusPageLimiter
}

func NewStatusPageHandler(settingsStore *settings.Store, frpSvc *frp.Service, cloudflareSvc *cloudflared.Service) *StatusPageHandler {
	return &StatusPageHandler{
		settings:   settingsStore,
		frp:        frpSvc,
		cloudflare: cloudflareSvc,
		limiter:    newStatusPageLimiter(),
	}
}

type statusPageRoute struct {
	Domain         string               `json:"domain"`
	State          string               `json:"state"`
	Source         string               `json:"source,omitempty"`
	Uptime         float64              `json:"uptime_pct"`
	DownMinutes    int                  `json:"down_minutes"`
	UnknownMinutes int                  `json:"unknown_minutes"`
	Incidents      []statusPageIncident `json:"incidents"`
}

type statusPageIncident struct {
	From    string `json:"from"`
	To      string `json:"to,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Minutes int    `json:"minutes"`
}

type statusPagePayload struct {
	Title        string            `json:"title"`
	CodeRequired bool              `json:"code_required"`
	CheckedAt    string            `json:"checked_at"`
	Up           int               `json:"up"`
	Down         int               `json:"down"`
	Unknown      int               `json:"unknown"`
	Routes       []statusPageRoute `json:"routes"`
}

// Get 是唯一的匿名入口：关闭时一律 404（与「不存在」不可区分），
// 设了访问码时校验 ?code=，校验失败也回 404。
func (h *StatusPageHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.settings == nil {
		writeError(r, w, http.StatusNotFound, "状态页未开启")
		return
	}
	if !h.limiter.allow(clientIPForLimit(r)) {
		writeError(r, w, http.StatusTooManyRequests, "请求过于频繁")
		return
	}

	enabled, _ := h.settings.GetBool(r.Context(), settings.KeyStatusPageEnabled)
	if !enabled || (h.frp == nil && h.cloudflare == nil) {
		writeError(r, w, http.StatusNotFound, "状态页未开启")
		return
	}
	codeHash, err := h.settings.Get(r.Context(), settings.KeyStatusPageCodeHash)
	if err != nil {
		codeHash = ""
	}
	codeRequired := strings.TrimSpace(codeHash) != ""
	if codeRequired && hashStatusPageCode(r.URL.Query().Get("code")) != codeHash {
		writeError(r, w, http.StatusNotFound, "状态页未开启")
		return
	}

	payload := statusPagePayload{
		Title:        "Havline 服务状态",
		CodeRequired: codeRequired,
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
		Routes:       make([]statusPageRoute, 0),
	}
	if err := appendFRPStatus(&payload, h.frp, r.Context()); err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取状态失败", err)
		return
	}
	if err := appendCloudflareStatus(&payload, h.cloudflare, r.Context()); err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取状态失败", err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func appendFRPStatus(payload *statusPagePayload, svc *frp.Service, ctx context.Context) error {
	if svc == nil {
		return nil
	}
	stats, err := svc.RouteUptime(ctx, 24*time.Hour)
	if err != nil {
		return err
	}
	for _, stat := range stats {
		switch stat.State {
		case frp.RouteHealthUp:
			payload.Up++
		case frp.RouteHealthDown:
			payload.Down++
		default:
			payload.Unknown++
		}
		payload.Routes = append(payload.Routes, statusPageRoute{
			Domain:         stat.Domain,
			State:          stat.State,
			Source:         "frp",
			Uptime:         stat.Uptime,
			DownMinutes:    stat.DownMinutes,
			UnknownMinutes: stat.UnknownMinutes,
			Incidents:      mapFRPIncidents(stat.Incidents),
		})
	}
	return nil
}

func appendCloudflareStatus(payload *statusPagePayload, svc *cloudflared.Service, ctx context.Context) error {
	if svc == nil {
		return nil
	}
	stats, err := svc.HealthUptime(ctx, 24*time.Hour)
	if err != nil {
		return err
	}
	for _, stat := range stats {
		switch stat.State {
		case cloudflared.HealthUp:
			payload.Up++
		case cloudflared.HealthDown:
			payload.Down++
		default:
			payload.Unknown++
		}
		payload.Routes = append(payload.Routes, statusPageRoute{
			Domain:         stat.Domain,
			State:          stat.State,
			Source:         "cloudflare",
			Uptime:         stat.Uptime,
			DownMinutes:    stat.DownMinutes,
			UnknownMinutes: stat.UnknownMinutes,
			Incidents:      mapCloudflareIncidents(stat.Incidents),
		})
	}
	return nil
}

func mapFRPIncidents(items []frp.RouteHealthIncident) []statusPageIncident {
	out := make([]statusPageIncident, 0, len(items))
	for _, item := range items {
		out = append(out, statusPageIncident{
			From: item.From, To: item.To, Reason: item.Reason, Detail: item.Detail, Minutes: item.Minutes,
		})
	}
	return out
}

func mapCloudflareIncidents(items []cloudflared.HealthIncident) []statusPageIncident {
	out := make([]statusPageIncident, 0, len(items))
	for _, item := range items {
		out = append(out, statusPageIncident{
			From: item.From, To: item.To, Reason: item.Reason, Detail: item.Detail, Minutes: item.Minutes,
		})
	}
	return out
}

func hashStatusPageCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

// clientIPForLimit 取限流用的客户端 IP
func clientIPForLimit(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// statusPageLimiter 按 IP 的滑动窗口限流；只给匿名状态页用
type statusPageLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
	now  func() time.Time
}

func newStatusPageLimiter() *statusPageLimiter {
	return &statusPageLimiter{hits: map[string][]time.Time{}, now: time.Now}
}

func (l *statusPageLimiter) allow(ip string) bool {
	now := l.now()
	cutoff := now.Add(-statusPageWindow)

	l.mu.Lock()
	defer l.mu.Unlock()
	kept := make([]time.Time, 0, len(l.hits[ip])+1)
	for _, at := range l.hits[ip] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) >= statusPageLimit {
		l.hits[ip] = kept
		return false
	}
	l.hits[ip] = append(kept, now)
	// 顺手清理长期不活跃的 IP，避免 map 无限增长
	if len(l.hits) > 1024 {
		for key, times := range l.hits {
			if len(times) == 0 {
				delete(l.hits, key)
			}
		}
	}
	return true
}

// MaskedStatusPageCode 供设置接口回显用（与通知模块的掩码约定一致）
const MaskedStatusPageCode = notify.MaskedSecret
