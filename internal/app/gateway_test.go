package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatewayPrefix(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"/app/havline/api/version", "/api/version"},
		{"/app/havline", "/"},
		{"/api/version", "/api/version"},
		{"/app/havline-other/api/version", "/app/havline-other/api/version"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			handler := stripPrefixIfPresent("/app/havline/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.want {
					t.Errorf("路径为 %q，期望 %q", r.URL.Path, tc.want)
				}
			}))
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.input, nil))
		})
	}
}
