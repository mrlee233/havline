package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/havline/havline/internal/config"
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

func TestRunGatewaySocketWithoutTCP(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "app.sock")
	server := &http.Server{
		Addr: "",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("havline-gateway-ok"))
		}),
	}
	app := &App{
		cfg:    config.Config{GatewaySocket: socket},
		server: server,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- app.Run() }()

	response := requestUnixSocket(t, socket)
	if !strings.Contains(response, "havline-gateway-ok") {
		t.Fatalf("unix socket response %q", response)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Run() error: %v", err)
	}
}

func requestUnixSocket(t *testing.T, socket string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", socket)
		if err == nil {
			defer conn.Close()
			_, _ = fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")
			data, _ := io.ReadAll(conn)
			return string(data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("unix socket not ready: %s", socket)
	return ""
}
