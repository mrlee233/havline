package cloudflared

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

const cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

// CloudflareAPI 用 Account ID + API Token 调用 Cloudflare API。
type CloudflareAPI struct {
	AccountID string
	APIToken  string
	client    *http.Client
}

func NewCloudflareAPI(accountID, apiToken string) *CloudflareAPI {
	return &CloudflareAPI{
		AccountID: strings.TrimSpace(accountID),
		APIToken:  strings.TrimSpace(apiToken),
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfEnvelope struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Errors  []cfAPIError    `json:"errors"`
}

func (a *CloudflareAPI) do(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, cloudflareAPIBase+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.APIToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求 Cloudflare API 失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	var envelope cfEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("Cloudflare API 响应格式无效（HTTP %d）", resp.StatusCode)
	}
	if !envelope.Success {
		message := "Cloudflare API 调用失败"
		if len(envelope.Errors) > 0 && strings.TrimSpace(envelope.Errors[0].Message) != "" {
			message = envelope.Errors[0].Message
		}
		return fmt.Errorf("%s（HTTP %d）", message, resp.StatusCode)
	}
	if out != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("Cloudflare API 结果解析失败: %w", err)
		}
	}
	return nil
}

// VerifyToken 调用账号级 tokens/verify 接口验证 API Token。
func (a *CloudflareAPI) VerifyToken(ctx context.Context) (string, error) {
	if a.AccountID == "" {
		return "", fmt.Errorf("请先填写 Account ID")
	}
	if a.APIToken == "" {
		return "", fmt.Errorf("请先填写 API Token（cfat_...）")
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := a.do(ctx, http.MethodGet, "/accounts/"+a.AccountID+"/tokens/verify", nil, &result); err != nil {
		return "", err
	}
	if result.Status != "active" {
		return "", fmt.Errorf("API Token 状态为 %s，不是 active", result.Status)
	}
	return "API Token 有效（状态：active）", nil
}

// CreateTunnel 创建一条 config_src=local 的命名隧道，并返回隧道 ID 与凭据 token。
func (a *CloudflareAPI) CreateTunnel(ctx context.Context, name string) (tunnelID, token string, err error) {
	var result struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	payload := map[string]any{"name": name, "config_src": "local"}
	if err := a.do(ctx, http.MethodPost, "/accounts/"+a.AccountID+"/cfd_tunnel", payload, &result); err != nil {
		return "", "", err
	}
	if result.ID == "" {
		return "", "", fmt.Errorf("Cloudflare 未返回隧道 ID")
	}
	if strings.TrimSpace(result.Token) != "" {
		return result.ID, result.Token, nil
	}
	token, err = a.TunnelToken(ctx, result.ID)
	return result.ID, token, err
}

// TunnelToken 读取指定隧道的凭据 token。
func (a *CloudflareAPI) TunnelToken(ctx context.Context, tunnelID string) (string, error) {
	var token string
	if err := a.do(ctx, http.MethodGet, "/accounts/"+a.AccountID+"/cfd_tunnel/"+tunnelID+"/token", nil, &token); err != nil {
		return "", err
	}
	if strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("Cloudflare 未返回隧道凭据")
	}
	return token, nil
}

// PutIngress 把本地 ingress 推送到 Cloudflare 云端配置。
func (a *CloudflareAPI) PutIngress(ctx context.Context, tunnelID string, ingress []IngressRule) error {
	return a.do(ctx, http.MethodPut, "/accounts/"+a.AccountID+"/cfd_tunnel/"+tunnelID+"/configurations", map[string]any{
		"config": map[string]any{"ingress": ingress},
	}, nil)
}

// GetIngress 读取云端隧道的 ingress 配置。
func (a *CloudflareAPI) GetIngress(ctx context.Context, tunnelID string) ([]IngressRule, error) {
	var result struct {
		Config struct {
			Ingress []IngressRule `json:"ingress"`
		} `json:"config"`
	}
	if err := a.do(ctx, http.MethodGet, "/accounts/"+a.AccountID+"/cfd_tunnel/"+tunnelID+"/configurations", nil, &result); err != nil {
		return nil, err
	}
	return result.Config.Ingress, nil
}

// DeleteTunnel 删除云端隧道。
func (a *CloudflareAPI) DeleteTunnel(ctx context.Context, tunnelID string) error {
	return a.do(ctx, http.MethodDelete, "/accounts/"+a.AccountID+"/cfd_tunnel/"+tunnelID+"?cascade=true", nil, nil)
}

// Zone 是 Cloudflare 的域名区域。
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListZones 返回当前 API Token 可访问的 Zone。
func (a *CloudflareAPI) ListZones(ctx context.Context) ([]Zone, error) {
	var zones []Zone
	if err := a.do(ctx, http.MethodGet, "/zones?per_page=50", nil, &zones); err != nil {
		return nil, err
	}
	return zones, nil
}

// FindZone 从 hostname 逐级向上查找所属 Zone。
func (a *CloudflareAPI) FindZone(ctx context.Context, hostname string) (string, string, error) {
	labels := strings.Split(strings.Trim(hostname, "."), ".")
	for i := 0; i < len(labels)-1; i++ {
		candidate := strings.Join(labels[i:], ".")
		var zones []Zone
		if err := a.do(ctx, http.MethodGet, "/zones?name="+url.QueryEscape(candidate), nil, &zones); err != nil {
			return "", "", err
		}
		if len(zones) > 0 {
			return zones[0].ID, zones[0].Name, nil
		}
	}
	return "", "", fmt.Errorf("未找到 %s 对应的 Cloudflare Zone；请确认该域名的根域名已加入当前 Account ID，且 API Token 的 Zone Resources 包含它，并具备 Zone:Read 权限", hostname)
}

// EnsureCNAME 幂等创建指向隧道的 CNAME 记录，并开启代理（橙云）。
// 已存在且目标一致的记录会被接管；指向其他目标的记录不会被覆盖。
func (a *CloudflareAPI) EnsureCNAME(ctx context.Context, zoneID, hostname, target string) (string, error) {
	var records []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	}
	path := "/zones/" + zoneID + "/dns_records?type=CNAME&name=" + url.QueryEscape(hostname)
	if err := a.do(ctx, http.MethodGet, path, nil, &records); err != nil {
		return "", err
	}
	payload := map[string]any{
		"type": "CNAME", "name": hostname, "content": target, "proxied": true, "ttl": 1,
	}
	if len(records) > 0 {
		if !strings.EqualFold(strings.TrimSpace(records[0].Content), strings.TrimSpace(target)) {
			return "", fmt.Errorf("DNS 记录 %s 已指向其他目标，未自动覆盖", hostname)
		}
		if err := a.do(ctx, http.MethodPut, "/zones/"+zoneID+"/dns_records/"+records[0].ID, payload, nil); err != nil {
			return "", err
		}
		return records[0].ID, nil
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := a.do(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", payload, &created); err != nil {
		return "", err
	}
	if strings.TrimSpace(created.ID) == "" {
		return "", fmt.Errorf("Cloudflare 未返回 DNS 记录 ID")
	}
	return created.ID, nil
}

// DeleteDNSRecord 删除一条明确由 Havline 接管的 DNS 记录。
func (a *CloudflareAPI) DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	return a.do(ctx, http.MethodDelete, "/zones/"+zoneID+"/dns_records/"+recordID, nil, nil)
}

type accessApp struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// HasAccessForHostname 只读检测域名是否已有 Access 应用；Token 无权限时返回错误，不修改任何配置。
func (a *CloudflareAPI) HasAccessForHostname(ctx context.Context, hostname string) (bool, string, error) {
	var apps []accessApp
	if err := a.do(ctx, http.MethodGet, "/accounts/"+a.AccountID+"/access/apps?per_page=100", nil, &apps); err != nil {
		return false, "", err
	}
	wanted := strings.ToLower(strings.TrimSpace(hostname))
	for _, app := range apps {
		if strings.EqualFold(strings.TrimSpace(app.Domain), wanted) {
			return true, app.Name, nil
		}
	}
	return false, "", nil
}
