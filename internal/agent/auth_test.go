package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerAuthToken(t *testing.T) {
	server := &Server{cfg: Config{Token: "secret-token"}}
	handler := server.auth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	cases := []struct {
		name   string
		header string
		status int
	}{
		{name: "正确 Token", header: "Bearer secret-token", status: http.StatusNoContent},
		{name: "Bearer 大小写不敏感", header: "bearer secret-token", status: http.StatusNoContent},
		{name: "多余空白可容忍", header: "  Bearer   secret-token  ", status: http.StatusNoContent},
		{name: "错误 Token", header: "Bearer wrong-token", status: http.StatusUnauthorized},
		{name: "长度不同的 Token", header: "Bearer secret-token-longer", status: http.StatusUnauthorized},
		{name: "缺少 Authorization", status: http.StatusUnauthorized},
		{name: "认证方案错误", header: "Token secret-token", status: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			recorder := httptest.NewRecorder()
			handler(recorder, req)
			if recorder.Code != tc.status {
				t.Fatalf("期望状态 %d，实际 %d", tc.status, recorder.Code)
			}
		})
	}
}

func TestSecureTokenEqual(t *testing.T) {
	if !secureTokenEqual("same-token", "same-token") {
		t.Fatal("相同 Token 应通过")
	}
	if secureTokenEqual("short", "longer-token") {
		t.Fatal("不同 Token 不应通过")
	}
	if secureTokenEqual("", "non-empty") {
		t.Fatal("空 Token 不应通过")
	}
}
