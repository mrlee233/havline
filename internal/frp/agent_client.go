package frp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AgentClient 调用公网 VPS 上的 fonu-agent。凭据由调用方解密后只在内存中使用。
type AgentClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewAgentClient(baseURL, token string) *AgentClient {
	return &AgentClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   token,
		// 路由写入链路含 nginx -T 自省 + -t 校验 + reload（宝塔带 lua/waf 模块时各步可达数秒），15s 会超时
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *AgentClient) request(ctx context.Context, method, path string, payload any, result any) error {
	if c.baseURL == "" || c.token == "" {
		return fmt.Errorf("公网 agent 未配置")
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &statusError{code: resp.StatusCode, body: strings.TrimSpace(string(data))}
	}
	if result != nil && len(data) > 0 {
		if err := json.Unmarshal(data, result); err != nil {
			return fmt.Errorf("agent 响应格式无效: %w", err)
		}
	}
	return nil
}

func (c *AgentClient) Ping(ctx context.Context) error {
	return c.request(ctx, http.MethodGet, "/api/v1/ping", nil, nil)
}

func (c *AgentClient) Status(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.request(ctx, http.MethodGet, "/api/v1/status", nil, &result)
	return result, err
}

// agentMinVersionInsights 是 P1 三个只读端点（metrics / logs / frps-config）要求的最低 agent 版本
const agentMinVersionInsights = "0.10.0"

// ErrAgentEndpointMissing 表示 VPS 上的 agent 版本过旧、没有该端点（HTTP 404/405）
var ErrAgentEndpointMissing = errors.New("VPS 上的 agent 版本过低，不支持该接口")

// statusError 保留 HTTP 状态码，便于上层区分「端点不存在」与真实错误
type statusError struct {
	code int
	body string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("agent 返回 HTTP %d: %s", e.code, e.body)
}

// wrapEndpointMissing 把「端点不存在」统一成带升级提示的错误，其它错误原样返回
func wrapEndpointMissing(err error) error {
	var statusErr *statusError
	if errors.As(err, &statusErr) && (statusErr.code == http.StatusNotFound || statusErr.code == http.StatusMethodNotAllowed) {
		return fmt.Errorf("%w（需 ≥ %s），请在服务端详情里升级 Agent", ErrAgentEndpointMissing, agentMinVersionInsights)
	}
	return err
}

// Metrics 采集 VPS 主机资源（CPU / 内存 / 磁盘 / 负载 / 运行时长）
func (c *AgentClient) Metrics(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.request(ctx, http.MethodGet, "/api/v1/metrics", nil, &result); err != nil {
		return nil, wrapEndpointMissing(err)
	}
	return result, nil
}

// NginxLogs 读取 VPS 上 Nginx 的 access / error 日志尾部
func (c *AgentClient) NginxLogs(ctx context.Context, kind string, lines int) (map[string]any, error) {
	query := url.Values{}
	query.Set("type", kind)
	query.Set("lines", strconv.Itoa(lines))
	var result map[string]any
	if err := c.request(ctx, http.MethodGet, "/api/v1/logs/nginx?"+query.Encode(), nil, &result); err != nil {
		return nil, wrapEndpointMissing(err)
	}
	return result, nil
}

// FRPSConfig 回读 VPS 上的 frps.toml（用于与下发内容比对）
func (c *AgentClient) FRPSConfig(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.request(ctx, http.MethodGet, "/api/v1/frps/config", nil, &result); err != nil {
		return nil, wrapEndpointMissing(err)
	}
	return result, nil
}

func (c *AgentClient) SyncRoute(ctx context.Context, domain, upstream string, tls bool, certDir string) error {
	_, err := c.SyncRouteEx(ctx, domain, upstream, tls, certDir, false, 0, false, nil, "")
	return err
}

// RouteSpec 公网反代单域名部署参数（字段名与 agent routeRequest 的 JSON key 严格一致）。
type RouteSpec struct {
	Domain, Upstream, CertDir                   string
	TLS, WebSocket, RedirectHTTPS               bool
	HTTPSDisabled, SecurityHeaders              bool
	TLS13Only                                   bool
	VhostPort                                   int
	AllowIPs, DenyIPs                           []string
	BasicAuthUser, BasicAuthPassword            string
	ClientMaxBodySize, ProxyReadTimeout         string
	RateLimitRate, RateLimitBurst, ConnLimitMax int
	ChinaOnly                                   bool
}

// SyncRouteSpec 部署路由并要求回源自检；位置参数已到上限，新增选项一律走本方法。
func (c *AgentClient) SyncRouteSpec(ctx context.Context, spec RouteSpec) (map[string]any, error) {
	var result map[string]any
	err := c.request(ctx, http.MethodPut, "/api/v1/routes", map[string]any{
		"domain": spec.Domain, "upstream": spec.Upstream, "tls": spec.TLS, "cert_dir": spec.CertDir,
		"websocket": spec.WebSocket, "vhost_port": spec.VhostPort,
		"redirect_https": spec.RedirectHTTPS, "allow_ips": spec.AllowIPs, "basic_auth_user": spec.BasicAuthUser,
		"deny_ips": spec.DenyIPs, "basic_auth_password": spec.BasicAuthPassword,
		"https_disabled": spec.HTTPSDisabled, "client_max_body_size": spec.ClientMaxBodySize,
		"proxy_read_timeout": spec.ProxyReadTimeout, "security_headers": spec.SecurityHeaders,
		"tls13_only":      spec.TLS13Only,
		"rate_limit_rate": spec.RateLimitRate, "rate_limit_burst": spec.RateLimitBurst,
		"conn_limit_max": spec.ConnLimitMax, "china_only": spec.ChinaOnly,
	}, &result)
	return result, err
}

// InstallNginx 触发 VPS 上一键安装 Nginx（需 agent 以 root 运行）。
func (c *AgentClient) InstallNginx(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.request(ctx, http.MethodPost, "/api/v1/nginx/install", nil, &result)
	return result, err
}

// SyncChinaCIDR 把中国 IP 段（geo include 格式）下发到公网 agent。
func (c *AgentClient) SyncChinaCIDR(ctx context.Context, content string) error {
	var result map[string]any
	return c.request(ctx, http.MethodPut, "/api/v1/china-cidr", map[string]any{"content": content}, &result)
}

// SyncRouteEx 保留的位置参数兼容包装（新代码请用 SyncRouteSpec）。
func (c *AgentClient) SyncRouteEx(ctx context.Context, domain, upstream string, tls bool, certDir string, websocket bool, vhostPort int, redirectHTTPS bool, allowIPs []string, basicAuthUser string) (map[string]any, error) {
	return c.SyncRouteSpec(ctx, RouteSpec{
		Domain: domain, Upstream: upstream, TLS: tls, CertDir: certDir,
		WebSocket: websocket, VhostPort: vhostPort, RedirectHTTPS: redirectHTTPS,
		AllowIPs: allowIPs, BasicAuthUser: basicAuthUser,
	})
}

// RouteConf 返回指定域名当前反代配置内容。
func (c *AgentClient) RouteConf(ctx context.Context, domain string) (string, error) {
	var result struct {
		Domain  string `json:"domain"`
		Content string `json:"content"`
	}
	if err := c.request(ctx, http.MethodGet, "/api/v1/routes/"+domain+"/conf", nil, &result); err != nil {
		return "", err
	}
	return result.Content, nil
}

// RouteDetail 单条公网反代路由的真实状态。
type RouteDetail struct {
	Domain     string `json:"domain"`
	FileExists bool   `json:"file_exists"`
	SyntaxOK   bool   `json:"syntax_ok"`
	Loaded     bool   `json:"loaded"`
}

// RoutesDetail 返回全部公网反代路由的状态明细（文件/语法/被 Nginx 加载）。
func (c *AgentClient) RoutesDetail(ctx context.Context) ([]RouteDetail, error) {
	var result []RouteDetail
	err := c.request(ctx, http.MethodGet, "/api/v1/routes", nil, &result)
	return result, err
}

func (c *AgentClient) DeleteRoute(ctx context.Context, domain string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/routes/"+domain, nil, nil)
}

func (c *AgentClient) FRPSAction(ctx context.Context, action string) error {
	return c.request(ctx, http.MethodPost, "/api/v1/frps/action", map[string]string{"action": action}, nil)
}

func (c *AgentClient) InstallFRPS(ctx context.Context, version, proxy string) (map[string]any, error) {
	var result map[string]any
	err := c.request(ctx, http.MethodPost, "/api/v1/frps/install", map[string]string{"version": version, "proxy": proxy}, &result)
	return result, err
}

func (c *AgentClient) PushFRPSConfig(ctx context.Context, content string) error {
	return c.request(ctx, http.MethodPut, "/api/v1/frps/config", map[string]string{"content": content}, nil)
}

func (c *AgentClient) PushCertificate(ctx context.Context, domain, fullchain, privateKey string) error {
	return c.request(ctx, http.MethodPost, "/api/v1/certs", map[string]string{
		"domain": domain, "fullchain_pem": fullchain, "key_pem": privateKey,
	}, nil)
}
