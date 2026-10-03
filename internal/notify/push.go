package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func SendWebhook(ctx context.Context, client *http.Client, cfg RuntimeConfig, content AlertContent) error {
	switch cfg.Webhook.Provider {
	case WebhookBark:
		return sendBark(ctx, client, cfg, content.PushTitle, content.barkMessage())
	case WebhookNtfy:
		return sendNtfy(ctx, client, cfg, content.PushTitle, content.PlainBody)
	case WebhookGotify:
		return sendGotify(ctx, client, cfg, content.PushTitle, content.PlainBody)
	case WebhookWecom:
		return sendWecom(ctx, client, cfg, content.PushTitle, content.PlainBody)
	case WebhookDingtalk:
		return sendDingtalk(ctx, client, cfg, content.PushTitle, content.PlainBody)
	case WebhookFeishu:
		return sendFeishu(ctx, client, cfg, content.PushTitle, content.PlainBody)
	case WebhookCustom:
		return sendCustomWebhook(ctx, client, cfg, content)
	default:
		return fmt.Errorf("不支持的 Webhook 预设")
	}
}

func webhookSecret(cfg RuntimeConfig) string {
	if strings.TrimSpace(cfg.WebhookSecret) != "" {
		return strings.TrimSpace(cfg.WebhookSecret)
	}
	return strings.TrimSpace(cfg.Webhook.Key)
}

func sendBark(ctx context.Context, client *http.Client, cfg RuntimeConfig, title, message string) error {
	key := webhookSecret(cfg)
	if key == "" {
		return fmt.Errorf("Bark Device Key 未配置")
	}
	server := normalizeServerURL(cfg.Webhook.Server)
	if server == "" {
		server = DefaultBarkServer
	}
	endpoint := fmt.Sprintf("%s/%s/%s/%s", server, url.PathEscape(key), url.PathEscape(title), url.PathEscape(message))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	return doRequest(client, req)
}

func sendNtfy(ctx context.Context, client *http.Client, cfg RuntimeConfig, title, message string) error {
	topic := strings.TrimSpace(cfg.Webhook.Topic)
	if topic == "" {
		return fmt.Errorf("ntfy Topic 未配置")
	}
	server := normalizeServerURL(cfg.Webhook.Server)
	if server == "" {
		server = DefaultNtfyServer
	}
	endpoint := joinURL(server, topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if token := webhookSecret(cfg); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doRequest(client, req)
}

func sendGotify(ctx context.Context, client *http.Client, cfg RuntimeConfig, title, message string) error {
	token := webhookSecret(cfg)
	if token == "" {
		return fmt.Errorf("Gotify App Token 未配置")
	}
	server := normalizeServerURL(cfg.Webhook.Server)
	if server == "" {
		return fmt.Errorf("Gotify 服务地址未配置")
	}
	endpoint := joinURL(server, "message") + "?token=" + url.QueryEscape(token)
	return postJSON(ctx, client, endpoint, map[string]any{
		"title":    title,
		"message":  message,
		"priority": 5,
	})
}

// 企业微信群机器人：POST /cgi-bin/webhook/send?key=<key>，markdown 正文
func sendWecom(ctx context.Context, client *http.Client, cfg RuntimeConfig, title, message string) error {
	key := webhookSecret(cfg)
	if key == "" {
		return fmt.Errorf("企业微信机器人 Key 未配置")
	}
	server := normalizeServerURL(cfg.Webhook.Server)
	if server == "" {
		server = DefaultWecomServer
	}
	endpoint := joinURL(server, "cgi-bin", "webhook", "send") + "?key=" + url.QueryEscape(key)
	return postJSON(ctx, client, endpoint, map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"content": markdownBody(title, message)},
	})
}

// 钉钉群机器人：POST /robot/send?access_token=<key>。
// 正文固定带 “Havline · ” 前缀，机器人用「自定义关键词 = Havline」的安全设置即可命中。
func sendDingtalk(ctx context.Context, client *http.Client, cfg RuntimeConfig, title, message string) error {
	key := webhookSecret(cfg)
	if key == "" {
		return fmt.Errorf("钉钉机器人 Access Token 未配置")
	}
	server := normalizeServerURL(cfg.Webhook.Server)
	if server == "" {
		server = DefaultDingtalkServer
	}
	titled := "Havline · " + title
	endpoint := joinURL(server, "robot", "send") + "?access_token=" + url.QueryEscape(key)
	return postJSON(ctx, client, endpoint, map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"title": titled, "text": markdownBody(titled, message)},
	})
}

// 飞书自定义机器人：POST /open-apis/bot/v2/hook/<key>，纯文本消息
func sendFeishu(ctx context.Context, client *http.Client, cfg RuntimeConfig, title, message string) error {
	key := webhookSecret(cfg)
	if key == "" {
		return fmt.Errorf("飞书机器人 Key 未配置")
	}
	server := normalizeServerURL(cfg.Webhook.Server)
	if server == "" {
		server = DefaultFeishuServer
	}
	endpoint := joinURL(server, "open-apis", "bot", "v2", "hook", key)
	return postJSON(ctx, client, endpoint, map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": title + "\n" + message},
	})
}

// markdownBody 拼出国内 IM 机器人都能渲染的 markdown（企业微信 / 钉钉都认 ## 标题 + 正文）
func markdownBody(title, message string) string {
	return "## " + title + "\n" + message
}

func sendCustomWebhook(ctx context.Context, client *http.Client, cfg RuntimeConfig, content AlertContent) error {
	target := strings.TrimSpace(cfg.Webhook.URL)
	if target == "" {
		return fmt.Errorf("Webhook URL 未配置")
	}
	return postJSON(ctx, client, target, content.webhookPayload())
}

// postJSON 发一个 JSON POST：各国内 IM 机器人都是这个形状，只是 URL 与 payload 不同。
func postJSON(ctx context.Context, client *http.Client, endpoint string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doRequest(client, req)
}

func doRequest(client *http.Client, req *http.Request) error {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if len(body) > 0 {
			return fmt.Errorf("请求失败 (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("请求失败 (%d)", resp.StatusCode)
	}
	return nil
}

func normalizeServerURL(server string) string {
	server = strings.TrimSpace(server)
	server = strings.TrimRight(server, "/")
	if server == "" {
		return ""
	}
	if !strings.Contains(server, "://") {
		server = "https://" + server
	}
	return server
}

func joinURL(base string, parts ...string) string {
	base = strings.TrimRight(normalizeServerURL(base), "/")
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			base += "/" + part
		}
	}
	return base
}
