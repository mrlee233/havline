package agent

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

var agentRouteParamPattern = regexp.MustCompile(`\{[^}]+\}`)

// TestAgentRoutesRequireToken 遍历全部 Agent API：匿名请求必须 401，防止新增路由漏挂 auth。
func TestAgentRoutesRequireToken(t *testing.T) {
	server := &Server{cfg: Config{Token: "secret-token"}}
	routes := agentRoutes(server)
	if len(routes) < 20 {
		t.Fatalf("Agent 路由覆盖不足：%d 条", len(routes))
	}
	handler := server.Handler()
	for _, route := range routes {
		path := agentRouteParamPattern.ReplaceAllString(route.Pattern, "1")
		req := httptest.NewRequest(route.Method, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("路由 %s %s 匿名请求应返回 401，实际 %d", route.Method, route.Pattern, rec.Code)
		}
	}
}
