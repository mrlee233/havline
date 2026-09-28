package frp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAgentEndpointMissingDetection 锁定旧 Agent 的 404 / 405 兼容提示，且不把其它错误误判为版本过低。
func TestAgentEndpointMissingDetection(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		minVersion  string
		route       bool
		wantMissing bool
	}{
		{"404 判定版本过低", http.StatusNotFound, agentMinVersionInsights, false, true},
		{"405 判定版本过低", http.StatusMethodNotAllowed, agentMinVersionRouteVersions, true, true},
		{"500 不判定版本过低", http.StatusInternalServerError, agentMinVersionInsights, false, false},
		{"400 不判定版本过低", http.StatusBadRequest, agentMinVersionInsights, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":"test"}`))
			}))
			defer server.Close()
			client := NewAgentClient(server.URL, "token")
			var err error
			if tc.route {
				_, err = client.RouteVersions(context.Background(), "example.com")
			} else {
				_, err = client.Metrics(context.Background())
			}
			if tc.wantMissing {
				if !errors.Is(err, ErrAgentEndpointMissing) {
					t.Fatalf("应判定为端点缺失：%v", err)
				}
				msg := err.Error()
				if !strings.Contains(msg, tc.minVersion) || !strings.Contains(msg, "升级 Agent") {
					t.Fatalf("错误信息应包含最低版本与升级提示：%q", msg)
				}
				return
			}
			if errors.Is(err, ErrAgentEndpointMissing) {
				t.Fatalf("不应误判为版本过低：%v", err)
			}
			if err == nil {
				t.Fatal("应返回错误")
			}
		})
	}
}

func TestAgentStatusVersionField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"agent_version":"0.10.0","listen_addr":"127.0.0.1:7700"}`))
	}))
	defer server.Close()
	client := NewAgentClient(server.URL, "token")
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status["agent_version"] != "0.10.0" {
		t.Fatalf("agent_version 解析失败：%#v", status)
	}
}
