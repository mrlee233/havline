package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/havline/havline/internal/auth"
	"github.com/havline/havline/internal/frp"
)

// FRPMultiHandler 承载多服务端 FRP 模块的 HTTP 接口（/api/frpmulti/*），
// 包装 internal/frp 的 Service（0.1.25 核心迁移版）。
type FRPMultiHandler struct {
	svc  *frp.Service
	auth *auth.Service
}

type frpServerRequest struct {
	AgentURL               string            `json:"agent_url"`
	AgentToken             string            `json:"agent_token"`
	ClearAgentToken        bool              `json:"clear_agent_token"`
	SSHHost                string            `json:"ssh_host"`
	SSHPort                int               `json:"ssh_port"`
	SSHUser                string            `json:"ssh_user"`
	SSHAuth                string            `json:"ssh_auth"`
	SSHSecret              string            `json:"ssh_secret"`
	ClearSSHSecret         bool              `json:"clear_ssh_secret"`
	MgmtEnabled            *bool             `json:"mgmt_enabled"`
	Name                   string            `json:"name"`
	ServerAddr             string            `json:"server_addr"`
	ServerPort             int               `json:"server_port"`
	TLSEnabled             *bool             `json:"tls_enabled"`
	TLSServerName          string            `json:"tls_server_name"`
	Enabled                *bool             `json:"enabled"`
	Token                  string            `json:"token"`
	ClearToken             bool              `json:"clear_token"`
	Options                frp.ServerOptions `json:"options"`
	OIDCClientSecret       string            `json:"oidc_client_secret"`
	ClearOIDCClientSecret  bool              `json:"clear_oidc_client_secret"`
	DashboardPassword      string            `json:"dashboard_password"`
	ClearDashboardPassword bool              `json:"clear_dashboard_password"`
	TLSCertificate         string            `json:"tls_certificate"`
	TLSKey                 string            `json:"tls_key"`
	TLSTrustedCA           string            `json:"tls_trusted_ca"`
	ClearTLSCertificate    bool              `json:"clear_tls_certificate"`
	ClearTLSKey            bool              `json:"clear_tls_key"`
	ClearTLSTrustedCA      bool              `json:"clear_tls_trusted_ca"`
}

type frpProxyRequest struct {
	ServerID          int64            `json:"server_id"`
	Name              string           `json:"name"`
	Type              string           `json:"type"`
	LocalIP           string           `json:"local_ip"`
	LocalPort         int              `json:"local_port"`
	RemotePort        *int             `json:"remote_port"`
	CustomDomains     []string         `json:"custom_domains"`
	HostHeaderRewrite string           `json:"host_header_rewrite"`
	Options           frp.ProxyOptions `json:"options"`
	Enabled           *bool            `json:"enabled"`
	Remark            string           `json:"remark"`
}

type frpDownloadProxyRequest struct {
	Proxy string `json:"proxy"`
}

func NewFRPMultiHandler(svc *frp.Service, authSvc *auth.Service) *FRPMultiHandler {
	return &FRPMultiHandler{svc: svc, auth: authSvc}
}

func (h *FRPMultiHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Status(r.Context()))
}

func (h *FRPMultiHandler) Runtime(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Runtime(r.Context()))
}

func (h *FRPMultiHandler) SetDownloadProxy(w http.ResponseWriter, r *http.Request) {
	var request frpDownloadProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	runtime, err := h.svc.SetDownloadProxy(r.Context(), request.Proxy)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runtime)
}

func (h *FRPMultiHandler) Releases(w http.ResponseWriter, r *http.Request) {
	releases, err := h.svc.ListReleases(r.Context())
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, releases)
}

func (h *FRPMultiHandler) Binaries(w http.ResponseWriter, r *http.Request) {
	binaries, err := h.svc.ListBinaries()
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取已下载 FRP 客户端失败")
		return
	}
	writeJSON(w, http.StatusOK, binaries)
}

func (h *FRPMultiHandler) DownloadStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.DownloadStatus())
}

func (h *FRPMultiHandler) DownloadRelease(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	task, err := h.svc.StartDownload(version)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (h *FRPMultiHandler) ActivateBinary(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	status, err := h.svc.ActivateBinary(r.Context(), version)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *FRPMultiHandler) DeleteBinary(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	if err := h.svc.DeleteBinary(version); err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *FRPMultiHandler) ListServers(w http.ResponseWriter, r *http.Request) {
	servers, err := h.svc.ListServers(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取 FRP 服务端失败")
		return
	}
	writeJSON(w, http.StatusOK, servers)
}

func (h *FRPMultiHandler) ServerDiagnostics(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	diagnostics, err := h.svc.ServerDiagnostics(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, diagnostics)
}

func (h *FRPMultiHandler) ServerDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	detail, err := h.svc.ServerDetail(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *FRPMultiHandler) StartServer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	server, err := h.svc.StartServer(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (h *FRPMultiHandler) StopServer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	server, err := h.svc.StopServer(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (h *FRPMultiHandler) CreateServer(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeFRPServerRequest(w, r)
	if !ok {
		return
	}
	server, err := h.svc.CreateServer(r.Context(), request.serverInput())
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (h *FRPMultiHandler) UpdateServer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	request, ok := decodeFRPServerRequest(w, r)
	if !ok {
		return
	}
	server, err := h.svc.UpdateServer(r.Context(), id, request.serverInput())
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (h *FRPMultiHandler) DeleteServer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	if err := h.svc.DeleteServer(r.Context(), id); err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *FRPMultiHandler) AgentProbe(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	result, err := h.svc.AgentProbe(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FRPMultiHandler) AgentInstall(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	server, err := h.svc.AgentInstall(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (h *FRPMultiHandler) AgentUninstall(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	keepData := r.URL.Query().Get("keep_data") == "true"
	output, err := h.svc.AgentUninstall(r.Context(), id, keepData)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": output})
}

func (h *FRPMultiHandler) AgentSSHDiagnose(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	output, err := h.svc.AgentSSHDiagnose(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"output": output})
}

func (h *FRPMultiHandler) AgentTest(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	if err := h.svc.AgentTest(r.Context(), id); err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *FRPMultiHandler) AgentTunnelRestart(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	server, err := h.svc.RestartAgentTunnel(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (h *FRPMultiHandler) SetAgentTransport(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	var request struct {
		Transport string `json:"transport"`
		Port      int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	var server frp.Server
	switch strings.TrimSpace(request.Transport) {
	case "tunnel":
		server, err = h.svc.SwitchAgentToTunnel(r.Context(), id)
	case "https-pin":
		server, err = h.svc.EnableAgentHTTPS(r.Context(), id, request.Port)
	default:
		writeError(r, w, http.StatusBadRequest, "不支持的传输方式")
		return
	}
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	h.audit(r, id, "transport", "切换为 "+server.AgentTransport)
	writeJSON(w, http.StatusOK, server)
}

func (h *FRPMultiHandler) AgentTransportStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	status, err := h.svc.AgentTransportStatus(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *FRPMultiHandler) AgentTransportRotateCert(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	server, err := h.svc.RotateAgentCertificate(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	h.audit(r, id, "rotate-tls-cert", server.AgentTLSPin)
	writeJSON(w, http.StatusOK, server)
}

func (h *FRPMultiHandler) AgentFirewallStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	port, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("port")))
	status, err := h.svc.AgentFirewallStatus(r.Context(), id, port)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *FRPMultiHandler) AgentFirewallAllow(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	var request struct {
		Port int `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	result, err := h.svc.AllowAgentFirewallPort(r.Context(), id, request.Port)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	h.audit(r, id, "firewall-allow", strconv.Itoa(request.Port))
	writeJSON(w, http.StatusOK, result)
}

// audit 记录 Agent 传输配置变更；admin_id 取自登录会话，API Token 调用没有会话时记为 0。
func (h *FRPMultiHandler) audit(r *http.Request, serverID int64, action, detail string) {
	adminID := 0
	if sessionID, ok := r.Context().Value(sessionContextKey).(string); ok && sessionID != "" && h.auth != nil {
		if id, err := h.auth.AdminIDForSession(r.Context(), sessionID); err == nil {
			adminID = id
		}
	}
	slog.Default().Info("审计：Agent 配置变更",
		"module", "AUDIT",
		"action", action,
		"server_id", serverID,
		"admin_id", adminID,
		"detail", detail,
		"remote_addr", r.RemoteAddr,
	)
}

func (h *FRPMultiHandler) AgentStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	status, err := h.svc.AgentStatus(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *FRPMultiHandler) AgentRouteCandidates(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	proxies, err := h.svc.ListAgentRouteCandidates(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, proxies)
}

func (h *FRPMultiHandler) DeployAgentRoute(w http.ResponseWriter, r *http.Request) {
	proxyID, err := parseID(r.PathValue("proxyID"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 规则 ID 无效")
		return
	}
	if err := h.svc.DeployAgentRoute(r.Context(), proxyID); err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "proxy_id": proxyID})
}

func (h *FRPMultiHandler) RemoveAgentRoute(w http.ResponseWriter, r *http.Request) {
	proxyID, err := parseID(r.PathValue("proxyID"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 规则 ID 无效")
		return
	}
	if err := h.svc.RemoveAgentRoute(r.Context(), proxyID); err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "proxy_id": proxyID})
}

func (h *FRPMultiHandler) AgentRoutes(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	// 路由状态明细在独立的 /api/v1/routes 端点（status 里的 routes 仍是字符串数组），
	// 从 agentClient 直取明细并透传。
	client, ok := h.svc.AgentClient(r.Context(), id)
	if !ok {
		writeError(r, w, http.StatusBadGateway, "公网 agent 未配置或 Token 未保存")
		return
	}
	details, err := client.RoutesDetail(r.Context())
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, details)
}

// agentRouteTarget 解析服务端与规则归属，返回该规则用于公网反代的首个域名。
// status 非 0 表示校验失败，调用方直接把 status 与 errMsg 写回响应。
func (h *FRPMultiHandler) agentRouteTarget(r *http.Request, serverIDRaw, proxyIDRaw string) (serverID, proxyID int64, domain string, status int, errMsg string) {
	serverID, err := parseID(serverIDRaw)
	if err != nil {
		return 0, 0, "", http.StatusBadRequest, "FRP 服务端 ID 无效"
	}
	proxyID, err = parseID(proxyIDRaw)
	if err != nil {
		return 0, 0, "", http.StatusBadRequest, "FRP 规则 ID 无效"
	}
	proxy, err := h.svc.GetProxy(r.Context(), proxyID)
	if err != nil || proxy.ServerID != serverID {
		return 0, 0, "", http.StatusNotFound, "FRP 规则不存在"
	}
	if len(proxy.CustomDomains) == 0 {
		return 0, 0, "", http.StatusBadRequest, "该规则没有自定义域名"
	}
	return serverID, proxyID, proxy.CustomDomains[0], 0, ""
}

// AgentRouteConf 返回指定穿透规则（首个域名）当前反代配置内容。
func (h *FRPMultiHandler) AgentRouteConf(w http.ResponseWriter, r *http.Request) {
	serverID, proxyID, domain, status, msg := h.agentRouteTarget(r, r.PathValue("id"), r.PathValue("proxyID"))
	if status != 0 {
		writeError(r, w, status, msg)
		return
	}
	client, ok := h.svc.AgentClient(r.Context(), serverID)
	if !ok {
		writeError(r, w, http.StatusBadGateway, "公网 agent 未配置或 Token 未保存")
		return
	}
	content, err := client.RouteConf(r.Context(), domain)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proxy_id": proxyID, "domain": domain, "content": content})
}

// RouteUptime 返回每条反代规则在最近窗口内的可用率与不可用区间。
// 数据来自后台巡检落库的「状态变更」序列，不做现场探测。
func (h *FRPMultiHandler) RouteUptime(w http.ResponseWriter, r *http.Request) {
	hours := 24
	if raw := strings.TrimSpace(r.URL.Query().Get("hours")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 24*7 {
			hours = parsed
		}
	}
	stats, err := h.svc.RouteUptime(r.Context(), time.Duration(hours)*time.Hour)
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取可用率失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hours": hours, "items": stats})
}

// AgentRouteVersions 返回规则首个域名在 VPS 上的当前 vhost 与历史版本，供差异对比与回滚。
func (h *FRPMultiHandler) AgentRouteVersions(w http.ResponseWriter, r *http.Request) {
	serverID, _, domain, status, msg := h.agentRouteTarget(r, r.PathValue("id"), r.PathValue("proxyID"))
	if status != 0 {
		writeError(r, w, status, msg)
		return
	}
	client, ok := h.svc.AgentClient(r.Context(), serverID)
	if !ok {
		writeError(r, w, http.StatusBadGateway, "公网 agent 未配置或 Token 未保存")
		return
	}
	result, err := client.RouteVersions(r.Context(), domain)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// AgentRouteVersion 读取某个历史版本的内容（与当前 vhost 对比用）。
func (h *FRPMultiHandler) AgentRouteVersion(w http.ResponseWriter, r *http.Request) {
	serverID, _, domain, status, msg := h.agentRouteTarget(r, r.PathValue("id"), r.PathValue("proxyID"))
	if status != 0 {
		writeError(r, w, status, msg)
		return
	}
	client, ok := h.svc.AgentClient(r.Context(), serverID)
	if !ok {
		writeError(r, w, http.StatusBadGateway, "公网 agent 未配置或 Token 未保存")
		return
	}
	result, err := client.RouteVersion(r.Context(), domain, r.PathValue("name"))
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// AgentRollbackRouteVersion 让 VPS 上的 vhost 回退到指定版本（agent 侧校验不过会保持当前配置）。
func (h *FRPMultiHandler) AgentRollbackRouteVersion(w http.ResponseWriter, r *http.Request) {
	serverID, _, domain, status, msg := h.agentRouteTarget(r, r.PathValue("id"), r.PathValue("proxyID"))
	if status != 0 {
		writeError(r, w, status, msg)
		return
	}
	client, ok := h.svc.AgentClient(r.Context(), serverID)
	if !ok {
		writeError(r, w, http.StatusBadGateway, "公网 agent 未配置或 Token 未保存")
		return
	}
	result, err := client.RollbackRouteVersion(r.Context(), domain, r.PathValue("name"))
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// AgentInstallNginx 触发 VPS 上一键安装 Nginx（需 agent 以 root 运行）。
func (h *FRPMultiHandler) AgentInstallNginx(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	result, err := h.svc.InstallAgentNginx(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// AgentRouteHealth 返回指定服务端公网反代规则的四段健康状态（DNS/证书/隧道/内网服务）。
func (h *FRPMultiHandler) AgentRouteHealth(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	health, err := h.svc.RouteHealth(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, health)
}

// AgentRouteHealthSummary 汇总所有已启用服务端的公网反代健康状态（仪表盘用，避免前端逐台请求）
func (h *FRPMultiHandler) AgentRouteHealthSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.svc.SummarizeRouteHealth(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "汇总公网反代健康失败")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// AgentRouteDrift 比对库里的期望与 VPS 上的实际配置，返回部署漂移清单
func (h *FRPMultiHandler) AgentRouteDrift(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	report, err := h.svc.RouteDrift(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// AgentRedeployRoutes 把该服务端上所有启用中的公网反代规则重新下发一遍
func (h *FRPMultiHandler) AgentRedeployRoutes(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	result, err := h.svc.RedeployAllRoutes(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// AgentMetrics 返回 VPS 主机资源（需 agent ≥ 0.10.0）
func (h *FRPMultiHandler) AgentMetrics(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	metrics, err := h.svc.AgentMetrics(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

// AgentNginxLogs 读取 VPS 上 Nginx 的 access / error 日志尾部（需 agent ≥ 0.10.0）
func (h *FRPMultiHandler) AgentNginxLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	lines, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("lines")))
	logs, err := h.svc.AgentNginxLogs(r.Context(), id, strings.TrimSpace(r.URL.Query().Get("type")), lines)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

// AgentFRPSConfig 回读 VPS 上的 frps.toml（需 agent ≥ 0.10.0）
func (h *FRPMultiHandler) AgentFRPSConfig(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	config, err := h.svc.AgentFRPSConfig(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, config)
}

// AgentCerts 列出 VPS 证书目录下的域名与到期时间（需 agent ≥ 0.11.0）
func (h *FRPMultiHandler) AgentCerts(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	certs, err := h.svc.AgentCerts(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, certs)
}

// AgentNginxReload 显式校验并重载 VPS 上的 Nginx（需 agent ≥ 0.11.0）
func (h *FRPMultiHandler) AgentNginxReload(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	result, err := h.svc.AgentNginxReload(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FRPMultiHandler) AgentFRPSAction(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	result, err := h.svc.AgentFRPSAction(r.Context(), id, body.Action)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FRPMultiHandler) InstallFRPSOnAgent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	var body struct {
		Version string `json:"version"`
		Proxy   string `json:"proxy"`
	}
	// 允许空请求体（走默认版本），但 JSON 语法错误要报 400
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	if body.Version == "" {
		body.Version = "0.71.0"
	}
	result, err := h.svc.InstallFRPSOnAgent(r.Context(), id, body.Version, body.Proxy)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FRPMultiHandler) PushFRPSConfigToAgent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	if err := h.svc.PushFRPSConfigToAgent(r.Context(), id); err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *FRPMultiHandler) TestServer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	if err := h.svc.Test(r.Context(), id); err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "FRP 配置校验通过"})
}

func (h *FRPMultiHandler) ListProxies(w http.ResponseWriter, r *http.Request) {
	proxies, err := h.svc.ListProxies(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取 FRP 规则失败")
		return
	}
	writeJSON(w, http.StatusOK, proxies)
}

// ListProxiesTraffic 返回各 frps 管理接口采集到的穿透规则流量（含速率）。
func (h *FRPMultiHandler) ListProxiesTraffic(w http.ResponseWriter, r *http.Request) {
	traffic, err := h.svc.ListProxiesTraffic(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取 FRP 规则流量失败")
		return
	}
	writeJSON(w, http.StatusOK, traffic)
}

func (h *FRPMultiHandler) CreateProxy(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeFRPProxyRequest(w, r)
	if !ok {
		return
	}
	proxy, err := h.svc.CreateProxy(r.Context(), request.proxyInput())
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, proxy)
}

func (h *FRPMultiHandler) UpdateProxy(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 规则 ID 无效")
		return
	}
	request, ok := decodeFRPProxyRequest(w, r)
	if !ok {
		return
	}
	proxy, err := h.svc.UpdateProxy(r.Context(), id, request.proxyInput())
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, proxy)
}

func (h *FRPMultiHandler) DeleteProxy(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 规则 ID 无效")
		return
	}
	if err := h.svc.DeleteProxy(r.Context(), id); err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *FRPMultiHandler) Start(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Start(r.Context()); err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.svc.Status(r.Context()))
}

func (h *FRPMultiHandler) Stop(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Stop(r.Context()); err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.svc.Status(r.Context()))
}

func (h *FRPMultiHandler) Reload(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Reload(r.Context()); err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.svc.Status(r.Context()))
}

func (h *FRPMultiHandler) Logs(w http.ResponseWriter, r *http.Request) {
	lines, err := h.svc.ReadLogs(queryInt(r, "limit", 200), int64(queryInt(r, "server_id", 0)))
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取 FRP 日志失败")
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

// ProxyLogs 返回单条穿透规则的日志（按规则名过滤所属服务端 frpc 日志）。
func (h *FRPMultiHandler) ProxyLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 规则 ID 无效")
		return
	}
	lines, err := h.svc.ReadProxyLogs(r.Context(), id, queryInt(r, "limit", 200))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (h *FRPMultiHandler) ExportConfig(w http.ResponseWriter, r *http.Request) {
	content, err := h.svc.ExportMaskedConfig(r.Context())
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

// FRPSServerConfig 返回指定服务端的 frps.toml 同源配置（bindPort/auth/webServer 与 frpc 侧一致）。
func (h *FRPMultiHandler) FRPSServerConfig(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	content, err := h.svc.FRPSServerConfig(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

func decodeFRPServerRequest(w http.ResponseWriter, r *http.Request) (frpServerRequest, bool) {
	var request frpServerRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return frpServerRequest{}, false
	}
	return request, true
}

func decodeFRPProxyRequest(w http.ResponseWriter, r *http.Request) (frpProxyRequest, bool) {
	var request frpProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return frpProxyRequest{}, false
	}
	return request, true
}

func (request frpServerRequest) serverInput() frp.ServerInput {
	return frp.ServerInput{
		AgentURL: request.AgentURL, AgentToken: request.AgentToken, ClearAgentToken: request.ClearAgentToken,
		SSHHost: request.SSHHost, SSHPort: request.SSHPort, SSHUser: request.SSHUser, SSHAuth: request.SSHAuth,
		SSHSecret: request.SSHSecret, ClearSSHSecret: request.ClearSSHSecret, MgmtEnabled: boolDefault(request.MgmtEnabled, false),
		Name: request.Name, ServerAddr: request.ServerAddr, ServerPort: request.ServerPort,
		TLS: boolDefault(request.TLSEnabled, true), TLSServerName: request.TLSServerName,
		Enabled: boolDefault(request.Enabled, true), Token: request.Token, ClearToken: request.ClearToken, Options: request.Options,
		OIDCClientSecret: request.OIDCClientSecret, ClearOIDCClientSecret: request.ClearOIDCClientSecret,
		DashboardPassword: request.DashboardPassword, ClearDashboardPassword: request.ClearDashboardPassword,
		TLSCertificate: request.TLSCertificate, TLSKey: request.TLSKey, TLSTrustedCA: request.TLSTrustedCA,
		ClearTLSCertificate: request.ClearTLSCertificate, ClearTLSKey: request.ClearTLSKey, ClearTLSTrustedCA: request.ClearTLSTrustedCA,
	}
}

func (request frpProxyRequest) proxyInput() frp.ProxyInput {
	return frp.ProxyInput{
		ServerID: request.ServerID, Name: request.Name, Type: request.Type, LocalIP: request.LocalIP,
		LocalPort: request.LocalPort, RemotePort: request.RemotePort, CustomDomains: request.CustomDomains,
		HostHeaderRewrite: request.HostHeaderRewrite, Options: request.Options, Enabled: boolDefault(request.Enabled, true), Remark: request.Remark,
	}
}
