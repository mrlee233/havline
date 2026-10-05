package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveComposePath(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveComposePath(dir, "docker-compose.hub.yml")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "docker-compose.hub.yml")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := resolveComposePath(dir, "../docker-compose.yml"); err == nil {
		t.Fatal("路径上跳应被拒绝")
	}
	if _, err := resolveComposePath(dir, "compose.yml"); err == nil {
		t.Fatal("不在白名单中的 compose 文件应被拒绝")
	}
}

func TestNewServerDefaultsAndValidation(t *testing.T) {
	server, err := NewServer(Config{
		ProjectDir:  t.TempDir(),
		ComposeFile: "docker-compose.yml",
		Token:       "test-token",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !server.state.Enabled || server.cfg.Mode != "local" {
		t.Fatalf("默认状态不正确: %#v", server.state)
	}
	if _, err := NewServer(Config{ProjectDir: t.TempDir(), Mode: "invalid", Token: "test-token"}, nil); err == nil {
		t.Fatal("非法 mode 应被拒绝")
	}
	if _, err := NewServer(Config{ProjectDir: t.TempDir(), Mode: "git", Token: "test-token"}, nil); err == nil {
		t.Fatal("git 模式缺少 RepoURL 应被拒绝")
	}
}

func TestEnsureComposeFile(t *testing.T) {
	dir := t.TempDir()
	server, err := NewServer(Config{ProjectDir: dir, ComposeFile: "docker-compose.yml", ProjectName: "havline", Token: "test-token"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.ensureComposeFile(context.Background()); err == nil {
		t.Fatal("缺少 compose 文件时应返回错误")
	}
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := server.ensureComposeFile(context.Background()); err != nil {
		t.Fatalf("存在 compose 文件时不应报错: %v", err)
	}
}

func TestResolveComposePlanUsesHostProjectDir(t *testing.T) {
	containerDir := t.TempDir()
	hostDir := t.TempDir()
	server, err := NewServer(Config{
		ProjectDir:     containerDir,
		ComposeFile:    "docker-compose.hub.yml",
		HostProjectDir: hostDir,
		ProjectName:    "havline",
		Token:          "test-token",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := server.resolveComposePlan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !plan.helper {
		t.Fatal("容器内看不到 Compose 文件时应使用 helper 模式")
	}
	if plan.helperComposePath != filepath.Join("/workspace", "docker-compose.hub.yml") {
		t.Fatalf("helper compose 路径不正确: %s", plan.helperComposePath)
	}
}

func TestNewServerRequiresToken(t *testing.T) {
	if _, err := NewServer(Config{ProjectDir: t.TempDir()}, nil); err == nil {
		t.Fatal("缺少 Token 时应拒绝启动")
	}
}

func TestHandlerRequiresToken(t *testing.T) {
	server, err := NewServer(Config{ProjectDir: t.TempDir(), Token: "test-token"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, token := range []string{"", "wrong-token"} {
		req := httptest.NewRequest(http.MethodGet, "http://unix/status", nil)
		if token != "" {
			req.Header.Set(tokenHeader, token)
		}
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("Token %q 应返回 401，实际为 %d", token, res.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "http://unix/status", nil)
	req.Header.Set(tokenHeader, "test-token")
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("正确 Token 应返回 200，实际为 %d", res.Code)
	}
}

func TestIsContainerPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/workspace", true},
		{"/workspace/docker-compose.yml", true},
		{"/vol1/1000/Docker/mafrp", false},
		{"/vol1/1000/Docker/mafrp/docker-compose.yml", false},
	}
	for _, tc := range cases {
		if got := isContainerPath(tc.path, "/workspace"); got != tc.want {
			t.Fatalf("isContainerPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
