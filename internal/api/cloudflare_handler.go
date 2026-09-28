package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/fonu/fonu/internal/cloudflared"
)

type CloudflareHandler struct {
	svc *cloudflared.Service
}

func NewCloudflareHandler(svc *cloudflared.Service) *CloudflareHandler {
	return &CloudflareHandler{svc: svc}
}

func (h *CloudflareHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *CloudflareHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "隧道 ID 无效")
		return
	}
	item, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *CloudflareHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input cloudflared.CreateInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	input.Network = normalizeCloudflareNetwork(input.Network)
	item, err := h.svc.Create(r.Context(), input)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *CloudflareHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "隧道 ID 无效")
		return
	}
	var input cloudflared.CreateInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	input.Network = normalizeCloudflareNetwork(input.Network)
	item, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *CloudflareHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "隧道 ID 无效")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *CloudflareHandler) Start(w http.ResponseWriter, r *http.Request) {
	h.tunnelAction(w, r, h.svc.Start)
}

func (h *CloudflareHandler) Stop(w http.ResponseWriter, r *http.Request) {
	h.tunnelAction(w, r, h.svc.Stop)
}

func (h *CloudflareHandler) Restart(w http.ResponseWriter, r *http.Request) {
	h.tunnelAction(w, r, h.svc.Restart)
}

func (h *CloudflareHandler) tunnelAction(w http.ResponseWriter, r *http.Request, fn func(context.Context, int64) (cloudflared.Tunnel, error)) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "隧道 ID 无效")
		return
	}
	item, err := fn(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *CloudflareHandler) Status(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "隧道 ID 无效")
		return
	}
	status, err := h.svc.RuntimeStatus(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *CloudflareHandler) Logs(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "隧道 ID 无效")
		return
	}
	lines, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("lines")))
	output, err := h.svc.Logs(r.Context(), id, lines)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"output": output})
}

func (h *CloudflareHandler) Config(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "隧道 ID 无效")
		return
	}
	content, err := h.svc.PreviewConfig(r.Context(), id)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": content})
}

func (h *CloudflareHandler) BinaryInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version": h.svc.BinaryVersion(),
		"mirrors": cloudflared.DefaultMirrors(),
	})
}

func (h *CloudflareHandler) AppSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.svc.AppSettings(r.Context())
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *CloudflareHandler) SaveAppSettings(w http.ResponseWriter, r *http.Request) {
	var input cloudflared.AppSettingsInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	input.Network = normalizeCloudflareNetwork(input.Network)
	settings, err := h.svc.SaveAppSettings(r.Context(), input)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *CloudflareHandler) TestToken(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AccountID string `json:"account_id"`
		Token     string `json:"token"`
		Kind      string `json:"kind"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	message, err := h.svc.TestToken(r.Context(), request.AccountID, request.Token, request.Kind)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": message})
}

func (h *CloudflareHandler) DownloadBinary(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Mirror string `json:"mirror"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&request); err != nil {
		writeError(r, w, http.StatusBadRequest, "请求格式无效")
		return
	}
	version, err := h.svc.DownloadBinary(r.Context(), request.Mirror)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version})
}

func (h *CloudflareHandler) IngressCandidates(w http.ResponseWriter, r *http.Request) {
	hosts, err := h.svc.IngressCandidates(r.Context())
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hostnames": hosts})
}

func normalizeCloudflareNetwork(network cloudflared.NetworkSettings) cloudflared.NetworkSettings {
	defaults := cloudflared.DefaultNetworkSettings()
	if strings.TrimSpace(network.TransportProtocol) == "" {
		network.TransportProtocol = defaults.TransportProtocol
	}
	if strings.TrimSpace(network.EdgeIPVersion) == "" {
		network.EdgeIPVersion = defaults.EdgeIPVersion
	}
	if network.HAConnections <= 0 {
		network.HAConnections = defaults.HAConnections
	}
	if strings.TrimSpace(network.ProxyMode) == "" {
		network.ProxyMode = defaults.ProxyMode
	}
	return network
}
