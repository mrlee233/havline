package acme

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withEndpoint 把 ZeroSSL 端点指向本地测试服务（用完恢复）。
func withEndpoint(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	previous := zeroSSLEABEndpoint
	zeroSSLEABEndpoint = server.URL
	t.Cleanup(func() {
		zeroSSLEABEndpoint = previous
		server.Close()
	})
}

func TestZeroSSLEABCredentialsSuccess(t *testing.T) {
	withEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("access_key"); got != "good-key" {
			t.Fatalf("access_key 应原样传出，实际 %q", got)
		}
		_, _ = w.Write([]byte(`{"success":1,"eab_kid":"kid-1","eab_hmac_key":"hmac-1"}`))
	})

	kid, hmac, err := zerosslEABCredentials(context.Background(), "good-key")
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	if kid != "kid-1" || hmac != "hmac-1" {
		t.Fatalf("凭据解析错误：kid=%q hmac=%q", kid, hmac)
	}
}

// 422 时要把 ZeroSSL 给的错误类型带出来，而不是只回一个状态码让用户猜
func TestZeroSSLEABCredentialsSurfacesErrorType(t *testing.T) {
	withEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":2902,"type":"invalid_api_key"}}`))
	})

	_, _, err := zerosslEABCredentials(context.Background(), "bad-key")
	if err == nil {
		t.Fatal("期望报错")
	}
	if !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "invalid_api_key") {
		t.Fatalf("错误里应含状态码与错误类型，实际：%v", err)
	}
}

// 非 JSON 的失败响应：退回截断正文，便于定位（例如网关返回的 HTML）
func TestZeroSSLEABCredentialsFallsBackToBodySnippet(t *testing.T) {
	withEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>\n  502 Bad Gateway\n</body></html>"))
	})

	_, _, err := zerosslEABCredentials(context.Background(), "any")
	if err == nil || !strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("应带上响应正文摘要，实际：%v", err)
	}
}

// 网络错误必须脱敏：url.Error 的字符串里带 access_key，泄漏进日志等于泄密
func TestZeroSSLEABCredentialsDoesNotLeakKey(t *testing.T) {
	const key = "SUPERSECRETACCESSKEY"
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close() // 立刻关掉，制造连接失败

	previous := zeroSSLEABEndpoint
	zeroSSLEABEndpoint = server.URL
	t.Cleanup(func() { zeroSSLEABEndpoint = previous })

	_, _, err := zerosslEABCredentials(context.Background(), key)
	if err == nil {
		t.Fatal("期望请求失败")
	}
	if strings.Contains(err.Error(), key) {
		t.Fatalf("错误信息里泄露了 API Key：%v", err)
	}
}
