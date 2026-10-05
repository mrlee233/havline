package frp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AgentClient 调用公网 VPS 上的 havline-agent。凭据由调用方解密后只在内存中使用。
type AgentClient struct {
	baseURL string
	token   string
	pin     string
	http    *http.Client
}

func NewAgentClient(baseURL, token string) *AgentClient {
	return newAgentClient(baseURL, token, "")
}

func NewAgentClientWithPin(baseURL, token, pin string) *AgentClient {
	return newAgentClient(baseURL, token, pin)
}

// NewAgentClientWithPins 支持证书轮换过渡期同时接受旧 pin 与新 pin。
func NewAgentClientWithPins(baseURL, token string, pins ...string) *AgentClient {
	return newAgentClient(baseURL, token, pins...)
}

func newAgentClient(baseURL, token string, pins ...string) *AgentClient {
	normalized := normalizePins(pins)
	client := &AgentClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   token,
		pin:     strings.Join(normalized, ","),
		// 路由写入链路含 nginx -T 自省 + -t 校验 + reload（宝塔带 lua/waf 模块时各步可达数秒），15s 会超时
		http: &http.Client{Timeout: 60 * time.Second},
	}
	if len(normalized) > 0 {
		client.http.Transport = &http.Transport{TLSClientConfig: pinTLSConfig(hostFromBaseURL(client.baseURL), normalized...)}
	}
	return client
}

func normalizePins(pins []string) []string {
	out := make([]string, 0, len(pins))
	for _, pin := range pins {
		pin = strings.ToLower(strings.TrimSpace(pin))
		if pin == "" {
			continue
		}
		out = append(out, pin)
	}
	return out
}

func hostFromBaseURL(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func pinTLSConfig(host string, pins ...string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("服务器未返回证书")
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}
			now := time.Now()
			if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
				return fmt.Errorf("Agent HTTPS 证书不在有效期内")
			}
			if err := verifyCertificateHost(cert, host); err != nil {
				return err
			}
			sum := sha256.Sum256(rawCerts[0])
			got := hex.EncodeToString(sum[:])
			for _, pin := range pins {
				if subtle.ConstantTimeCompare([]byte(got), []byte(pin)) == 1 {
					return nil
				}
			}
			return fmt.Errorf("Agent HTTPS 证书指纹不匹配")
		},
	}
}

// verifyCertificateHost 校验证书 SAN 是否匹配连接主机，避免 pin 正确但证书属于其他主机。
func verifyCertificateHost(cert *x509.Certificate, host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		for _, candidate := range cert.IPAddresses {
			if candidate.Equal(ip) {
				return nil
			}
		}
		return fmt.Errorf("Agent HTTPS 证书不包含 IP %s", host)
	}
	if err := cert.VerifyHostname(host); err != nil {
		return fmt.Errorf("Agent HTTPS 证书 SAN 不匹配 %s", host)
	}
	return nil
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
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

func (c *AgentClient) ConfigureHTTPS(ctx context.Context, certPEM, keyPEM string, port int) (map[string]any, error) {
	var result map[string]any
	err := c.request(ctx, http.MethodPut, "/api/v1/transport/https", map[string]any{
		"cert_pem": certPEM,
		"key_pem":  keyPEM,
		"port":     port,
	}, &result)
	return result, err
}

func (c *AgentClient) DisableHTTPS(ctx context.Context) error {
	var result map[string]any
	return c.request(ctx, http.MethodDelete, "/api/v1/transport/https", nil, &result)
}

// HTTPSState 读取 Agent 当前 HTTPS 反代状态与证书到期时间。
func (c *AgentClient) HTTPSState(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.request(ctx, http.MethodGet, "/api/v1/transport/https", nil, &result)
	return result, err
}

func (c *AgentClient) Status(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.request(ctx, http.MethodGet, "/api/v1/status", nil, &result)
	return result, err
}

// FirewallStatus 读取 VPS 主机防火墙类型与目标端口放行状态。
func (c *AgentClient) FirewallStatus(ctx context.Context, port int) (map[string]any, error) {
	var result map[string]any
	err := c.request(ctx, http.MethodGet, "/api/v1/firewall?port="+strconv.Itoa(port), nil, &result)
	return result, err
}

// AllowFirewallPort 请求 Agent 显式放行主机防火墙端口（仅 ufw / firewalld）。
func (c *AgentClient) AllowFirewallPort(ctx context.Context, port int) (map[string]any, error) {
	var result map[string]any
	err := c.request(ctx, http.MethodPost, "/api/v1/firewall/allow", map[string]any{"port": port}, &result)
	return result, err
}

// agentMinVersionInsights 是 P1 三个只读端点（metrics / logs / frps-config）要求的最低 agent 版本
const agentMinVersionInsights = "0.11.0"

// agentMinVersionRouteVersions 是域名 vhost 版本列表 / 对比 / 回滚端点要求的最低 agent 版本
const agentMinVersionRouteVersions = "0.12.2"

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

// wrapEndpointMissing 把「端点不存在」统一成带升级提示的错误（按端点给出最低版本），其它错误原样返回
func wrapEndpointMissing(err error, minVersion string) error {
	var statusErr *statusError
	if errors.As(err, &statusErr) && (statusErr.code == http.StatusNotFound || statusErr.code == http.StatusMethodNotAllowed) {
		return fmt.Errorf("%w（需 ≥ %s），请在服务端详情里升级 Agent", ErrAgentEndpointMissing, minVersion)
	}
	return err
}

// Metrics 采集 VPS 主机资源（CPU / 内存 / 磁盘 / 负载 / 运行时长）
func (c *AgentClient) Metrics(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.request(ctx, http.MethodGet, "/api/v1/metrics", nil, &result); err != nil {
		return nil, wrapEndpointMissing(err, agentMinVersionInsights)
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
		return nil, wrapEndpointMissing(err, agentMinVersionInsights)
	}
	return result, nil
}

// Certs 列出 VPS 证书目录下的域名与到期时间
func (c *AgentClient) Certs(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.request(ctx, http.MethodGet, "/api/v1/certs", nil, &result); err != nil {
		return nil, wrapEndpointMissing(err, agentMinVersionInsights)
	}
	return result, nil
}

// NginxReload 显式校验并重载 Nginx，返回校验 / 重载结果与失败原因
func (c *AgentClient) NginxReload(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.request(ctx, http.MethodPost, "/api/v1/nginx/reload", nil, &result); err != nil {
		return nil, wrapEndpointMissing(err, agentMinVersionInsights)
	}
	return result, nil
}

// FRPSConfig 回读 VPS 上的 frps.toml（用于与下发内容比对）
func (c *AgentClient) FRPSConfig(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.request(ctx, http.MethodGet, "/api/v1/frps/config", nil, &result); err != nil {
		return nil, wrapEndpointMissing(err, agentMinVersionInsights)
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

func (c *AgentClient) FRPSAction(ctx context.Context, action string) (map[string]any, error) {
	var result map[string]any
	if err := c.request(ctx, http.MethodPost, "/api/v1/frps/action", map[string]string{"action": action}, &result); err != nil {
		return nil, err
	}
	return normalizeFRPSActionResult(result)
}

// normalizeFRPSActionResult 保留 agent 的 ok=false 与日志，避免后端把它误判为成功。
func normalizeFRPSActionResult(result map[string]any) (map[string]any, error) {
	if result == nil {
		return nil, errors.New("agent 未返回 frps 操作结果")
	}
	ok, exists := result["ok"].(bool)
	if !exists {
		return nil, errors.New("agent 未返回 frps 操作结果")
	}
	if ok {
		return result, nil
	}
	if msg, _ := result["error"].(string); strings.TrimSpace(msg) != "" {
		return result, nil
	}
	result["error"] = "frps 未进入运行状态，请查看 frps 日志"
	return result, nil
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

// RouteVersions 返回域名当前 vhost 内容与历史版本列表（需 agent ≥ 0.12.2）。
func (c *AgentClient) RouteVersions(ctx context.Context, domain string) (map[string]any, error) {
	var result map[string]any
	if err := c.request(ctx, http.MethodGet, "/api/v1/routes/"+domain+"/versions", nil, &result); err != nil {
		return nil, wrapEndpointMissing(err, agentMinVersionRouteVersions)
	}
	return result, nil
}

// RouteVersion 读取某个历史版本的内容，供与当前 vhost 做差异对比。
func (c *AgentClient) RouteVersion(ctx context.Context, domain, name string) (map[string]any, error) {
	var result map[string]any
	path := "/api/v1/routes/" + domain + "/versions/" + url.PathEscape(name)
	if err := c.request(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, wrapEndpointMissing(err, agentMinVersionRouteVersions)
	}
	return result, nil
}

// RollbackRouteVersion 让 VPS 上的 vhost 回退到指定版本。
func (c *AgentClient) RollbackRouteVersion(ctx context.Context, domain, name string) (map[string]any, error) {
	var result map[string]any
	path := "/api/v1/routes/" + domain + "/versions/" + url.PathEscape(name) + "/rollback"
	if err := c.request(ctx, http.MethodPost, path, nil, &result); err != nil {
		return nil, wrapEndpointMissing(err, agentMinVersionRouteVersions)
	}
	return result, nil
}
