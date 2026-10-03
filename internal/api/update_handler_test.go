package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateHandlerDisabled(t *testing.T) {
	handler := NewUpdateHandler(nil)

	statusRecorder := httptest.NewRecorder()
	statusReq := httptest.NewRequest(http.MethodGet, "/api/system/update/status", nil)
	handler.Status(statusRecorder, statusReq)
	if statusRecorder.Code != http.StatusOK {
		t.Fatalf("状态接口期望 200，实际 %d", statusRecorder.Code)
	}
	if body := statusRecorder.Body.String(); !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("未返回 enabled=false: %s", body)
	}

	applyRecorder := httptest.NewRecorder()
	applyReq := httptest.NewRequest(http.MethodPost, "/api/system/update/apply", nil)
	handler.Apply(applyRecorder, applyReq)
	if applyRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("升级接口期望 503，实际 %d", applyRecorder.Code)
	}
}
