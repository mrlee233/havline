package api

import (
	"net/http"

	"github.com/havline/havline/internal/updater"
)

type UpdateHandler struct {
	client *updater.Client
}

func NewUpdateHandler(client *updater.Client) *UpdateHandler {
	return &UpdateHandler{client: client}
}

func (h *UpdateHandler) Status(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || !h.client.Enabled() {
		writeJSON(w, http.StatusOK, updater.Status{
			Enabled: false,
			Phase:   updater.PhaseIdle,
			Message: "当前部署未启用一键升级",
		})
		return
	}
	status, err := h.client.Status(r.Context())
	if err != nil {
		writeError(r, w, http.StatusBadGateway, "读取升级状态失败", err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *UpdateHandler) Apply(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || !h.client.Enabled() {
		writeError(r, w, http.StatusServiceUnavailable, "当前部署未启用一键升级")
		return
	}
	status, err := h.client.Apply(r.Context())
	if err != nil {
		writeError(r, w, http.StatusBadGateway, "启动升级失败", err)
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}
