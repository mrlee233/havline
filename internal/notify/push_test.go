package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendCustomWebhook(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := RuntimeConfig{
		Config: Config{
			Type: NotifyTypeWebhook,
			Webhook: WebhookConfig{
				Provider: WebhookCustom,
				URL:      server.URL,
			},
		},
	}
	content := FormatAlert("test", "title", "message")
	if err := SendWebhook(context.Background(), server.Client(), cfg, content); err != nil {
		t.Fatalf("send custom webhook: %v", err)
	}
	if !strings.Contains(body, `"title":"title"`) || !strings.Contains(body, `"message":"message"`) {
		t.Fatalf("unexpected body: %s", body)
	}
	if !strings.Contains(body, `"formatted":`) {
		t.Fatalf("missing formatted body: %s", body)
	}
}

func TestSendNtfyUsesTopic(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := RuntimeConfig{
		Config: Config{
			Type: NotifyTypeWebhook,
			Webhook: WebhookConfig{
				Provider: WebhookNtfy,
				Server:   server.URL,
				Topic:    "alerts",
			},
		},
	}
	if err := SendWebhook(context.Background(), server.Client(), cfg, FormatAlert("test", "title", "message")); err != nil {
		t.Fatalf("send ntfy: %v", err)
	}
	if gotPath != "/alerts" {
		t.Fatalf("expected /alerts got %s", gotPath)
	}
}

func TestJoinURL(t *testing.T) {
	got := joinURL("https://ntfy.sh", "topic")
	if got != "https://ntfy.sh/topic" {
		t.Fatalf("unexpected url: %s", got)
	}
}

// 国内 IM 机器人：固定路径 + 固定 payload 形状，这里把三家的端点与请求体锁住
func TestSendWecomUsesKeyAndMarkdown(t *testing.T) {
	var path, query, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		query = r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := RuntimeConfig{Config: Config{
		Type:    NotifyTypeWebhook,
		Webhook: WebhookConfig{Provider: WebhookWecom, Server: server.URL, Key: "abc"},
	}}
	if err := SendWebhook(context.Background(), server.Client(), cfg, FormatAlert("test", "标题", "内容")); err != nil {
		t.Fatalf("send wecom: %v", err)
	}
	if path != "/cgi-bin/webhook/send" || query != "key=abc" {
		t.Fatalf("unexpected endpoint: %s?%s", path, query)
	}
	if !strings.Contains(body, `"msgtype":"markdown"`) || !strings.Contains(body, "标题") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestSendDingtalkUsesAccessTokenAndKeyword(t *testing.T) {
	var path, query, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		query = r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := RuntimeConfig{Config: Config{
		Type:    NotifyTypeWebhook,
		Webhook: WebhookConfig{Provider: WebhookDingtalk, Server: server.URL, Key: "tok"},
	}}
	if err := SendWebhook(context.Background(), server.Client(), cfg, FormatAlert("test", "标题", "内容")); err != nil {
		t.Fatalf("send dingtalk: %v", err)
	}
	if path != "/robot/send" || query != "access_token=tok" {
		t.Fatalf("unexpected endpoint: %s?%s", path, query)
	}
	// 「自定义关键词」安全设置靠正文里的 Havline 命中（PushTitle 前面还有事件图标）
	if !strings.Contains(body, "Havline · ") || !strings.Contains(body, "标题") {
		t.Fatalf("missing keyword prefix: %s", body)
	}
}

func TestSendFeishuUsesHookPath(t *testing.T) {
	var path, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := RuntimeConfig{Config: Config{
		Type:    NotifyTypeWebhook,
		Webhook: WebhookConfig{Provider: WebhookFeishu, Server: server.URL, Key: "hookkey"},
	}}
	if err := SendWebhook(context.Background(), server.Client(), cfg, FormatAlert("test", "标题", "内容")); err != nil {
		t.Fatalf("send feishu: %v", err)
	}
	if path != "/open-apis/bot/v2/hook/hookkey" {
		t.Fatalf("unexpected path: %s", path)
	}
	if !strings.Contains(body, `"msg_type":"text"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestProviderUsesKeyCoversKeyBasedPresets(t *testing.T) {
	for _, provider := range []WebhookProvider{WebhookBark, WebhookNtfy, WebhookGotify, WebhookWecom, WebhookDingtalk, WebhookFeishu} {
		if !providerUsesKey(provider) {
			t.Fatalf("%s 的凭据存在 Webhook.Key，密钥落库路径必须覆盖它", provider)
		}
	}
	if providerUsesKey(WebhookCustom) {
		t.Fatal("自定义 Webhook 的凭据来自 URL，不应走 Key 分支")
	}
}
