package updater

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteVersionURLs(t *testing.T) {
	urls, err := remoteVersionURLs("http://example.com/mrlee/Havline.git", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) == 0 || !strings.Contains(urls[0], "/api/v1/repos/mrlee/Havline/contents/VERSION") {
		t.Fatalf("缺少 Gitea API 候选地址: %#v", urls)
	}
	if !strings.Contains(urls[1], "/mrlee/Havline/raw/branch/main/VERSION") {
		t.Fatalf("缺少 Gitea raw 候选地址: %#v", urls)
	}
}

func TestParseRemoteVersion(t *testing.T) {
	if got, err := parseRemoteVersion([]byte("1.3.2\n")); err != nil || got != "1.3.2" {
		t.Fatalf("文本版本解析失败: got=%q err=%v", got, err)
	}
	payload, _ := json.Marshal(map[string]string{
		"encoding": "base64",
		"content":  base64.StdEncoding.EncodeToString([]byte("1.3.2\n")),
	})
	if got, err := parseRemoteVersion(payload); err != nil || got != "1.3.2" {
		t.Fatalf("JSON 版本解析失败: got=%q err=%v", got, err)
	}
}

func TestFetchRemoteVersionFromGiteaAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/mrlee/Havline/contents/VERSION" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"encoding": "base64",
			"content":  base64.StdEncoding.EncodeToString([]byte("1.3.2\n")),
		})
	}))
	defer server.Close()

	got, err := fetchRemoteVersion(context.Background(), server.URL+"/mrlee/Havline.git", "main")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.3.2" {
		t.Fatalf("got %q, want 1.3.2", got)
	}
}
