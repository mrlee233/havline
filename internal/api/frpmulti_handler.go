package api

import (
	"encoding/json"
	"net/http"

	"github.com/fonu/fonu/internal/frp"
)

// FRPMultiHandler 承载多服务端 FRP 模块的 HTTP 接口（/api/frpmulti/*），
// 包装 internal/frp 的 Service（0.1.25 核心迁移版）。
type FRPMultiHandler struct {
	svc *frp.Service
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

func NewFRPMultiHandler(svc *frp.Service) *FRPMultiHandler {
	return &FRPMultiHandler{svc: svc}
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

// AgentRouteConf 返回指定穿透规则（首个域名）当前反代配置内容。
func (h *FRPMultiHandler) AgentRouteConf(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 服务端 ID 无效")
		return
	}
	proxyID, err := parseID(r.PathValue("proxyID"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "FRP 规则 ID 无效")
		return
	}
	proxy, err := h.svc.GetProxy(r.Context(), proxyID)
	if err != nil || proxy.ServerID != id {
		writeError(r, w, http.StatusNotFound, "FRP 规则不存在")
		return
	}
	if len(proxy.CustomDomains) == 0 {
		writeError(r, w, http.StatusBadRequest, "该规则没有自定义域名")
		return
	}
	client, ok := h.svc.AgentClient(r.Context(), id)
	if !ok {
		writeError(r, w, http.StatusBadGateway, "公网 agent 未配置或 Token 未保存")
		return
	}
	content, err := client.RouteConf(r.Context(), proxy.CustomDomains[0])
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proxy_id": proxyID, "domain": proxy.CustomDomains[0], "content": content})
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
	if err := h.svc.AgentFRPSAction(r.Context(), id, body.Action); err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": body.Action})
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
	_ = json.NewDecoder(r.Body).Decode(&body)
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
