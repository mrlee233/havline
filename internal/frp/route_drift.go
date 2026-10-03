package frp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 漂移类型：库里的期望（A）与 VPS 上的实际（B/C）之间的差异分类
const (
	RouteDriftMissing         = "missing"          // 库里要部署但 VPS 上没有
	RouteDriftOrphan          = "orphan"           // VPS 有但库里没有（手工添加或已删规则的残留）
	RouteDriftNotLoaded       = "not_loaded"       // 文件在但未被 Nginx 加载
	RouteDriftContentMismatch = "content_mismatch" // 上流地址与库里的期望不一致
)

// RouteDriftItem 一条差异
type RouteDriftItem struct {
	Domain   string `json:"domain"`
	Kind     string `json:"kind"`
	Detail   string `json:"detail"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

// RouteDriftReport 部署漂移报告：Expected = 库里启用中的规则域名数，Deployed = VPS 上 havline-*.conf 数，Loaded = 其中被加载的数
type RouteDriftReport struct {
	ServerID  int64            `json:"server_id"`
	Expected  int              `json:"expected"`
	Deployed  int              `json:"deployed"`
	Loaded    int              `json:"loaded"`
	Items     []RouteDriftItem `json:"items"`
	CheckedAt string           `json:"checked_at"`
}

// RouteDrift 比对「数据库里启用中的公网反代规则」与「VPS 上 havline-*.conf 的实际状态」。
// 内容比对只做上流地址（proxy_pass）：配置文件由 agent 渲染，应用侧不重复实现一套渲染器，
// 因此不做全文 diff，避免两套渲染逻辑漂移带来的误报。
func (s *Service) RouteDrift(ctx context.Context, serverID int64) (RouteDriftReport, error) {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return RouteDriftReport{}, fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	proxies, err := s.store.ListEnabledProxies(ctx, serverID)
	if err != nil {
		return RouteDriftReport{}, err
	}

	// A：库里的期望域名 → 期望上流
	expected := map[string]string{}
	for _, proxy := range proxies {
		if !deployableProxy(proxy) {
			continue
		}
		upstream, uerr := s.agentUpstreamFor(ctx, proxy)
		if uerr != nil {
			upstream = ""
		}
		for _, domain := range proxy.CustomDomains {
			expected[domain] = upstream
		}
	}

	details, err := client.RoutesDetail(ctx)
	if err != nil {
		return RouteDriftReport{}, err
	}
	// B / C：VPS 上的实际
	actual := make(map[string]RouteDetail, len(details))
	loaded := 0
	for _, detail := range details {
		actual[detail.Domain] = detail
		if detail.Loaded {
			loaded++
		}
	}

	report := RouteDriftReport{
		ServerID:  serverID,
		Expected:  len(expected),
		Deployed:  len(details),
		Loaded:    loaded,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}

	for domain, want := range expected {
		detail, ok := actual[domain]
		if !ok {
			report.Items = append(report.Items, RouteDriftItem{
				Domain: domain, Kind: RouteDriftMissing,
				Detail:   "VPS 上没有该域名的配置，重新部署即可写入",
				Expected: want,
			})
			continue
		}
		if !detail.Loaded {
			report.Items = append(report.Items, RouteDriftItem{
				Domain: domain, Kind: RouteDriftNotLoaded,
				Detail: "配置文件存在但未被 Nginx 加载（语法未通过，或 include 未覆盖该文件）",
			})
			continue
		}
		if want == "" {
			continue
		}
		content, cerr := client.RouteConf(ctx, domain)
		if cerr != nil {
			continue
		}
		got := normalizeUpstream(proxyPassTarget(content))
		if got != "" && got != normalizeUpstream(want) {
			report.Items = append(report.Items, RouteDriftItem{
				Domain: domain, Kind: RouteDriftContentMismatch,
				Detail:   "VPS 上的上流地址与 Havline 记录不一致（可能被手工修改过）",
				Expected: want, Actual: got,
			})
		}
	}

	for domain := range actual {
		if _, ok := expected[domain]; ok {
			continue
		}
		report.Items = append(report.Items, RouteDriftItem{
			Domain: domain, Kind: RouteDriftOrphan,
			Detail: "VPS 上存在该域名配置，但 Havline 里没有对应规则（手工添加，或已删除规则的残留）",
		})
	}

	sort.Slice(report.Items, func(i, j int) bool {
		if report.Items[i].Kind != report.Items[j].Kind {
			return report.Items[i].Kind < report.Items[j].Kind
		}
		return report.Items[i].Domain < report.Items[j].Domain
	})
	return report, nil
}

// RedeployRouteResult 单条规则的重新部署结果
type RedeployRouteResult struct {
	ProxyID int64  `json:"proxy_id"`
	Name    string `json:"name"`
	Domains string `json:"domains"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// RedeployAllResult 全量重部署汇总
type RedeployAllResult struct {
	ServerID  int64                 `json:"server_id"`
	Total     int                   `json:"total"`
	Succeeded int                   `json:"succeeded"`
	Failed    int                   `json:"failed"`
	Results   []RedeployRouteResult `json:"results"`
}

// RedeployAllRoutes 把该服务端上所有启用中的公网反代规则重新下发一遍。
// 逐条执行、单条失败不中断（沿用 Reload 的隔离报错风格），返回逐条结果供前端汇总展示。
func (s *Service) RedeployAllRoutes(ctx context.Context, serverID int64) (RedeployAllResult, error) {
	proxies, err := s.store.ListEnabledProxies(ctx, serverID)
	if err != nil {
		return RedeployAllResult{}, err
	}
	result := RedeployAllResult{ServerID: serverID, Results: []RedeployRouteResult{}}
	for _, proxy := range proxies {
		if !deployableProxy(proxy) {
			continue
		}
		result.Total++
		item := RedeployRouteResult{
			ProxyID: proxy.ID,
			Name:    proxy.Name,
			Domains: strings.Join(proxy.CustomDomains, "、"),
		}
		if err := s.DeployAgentRoute(ctx, proxy.ID); err != nil {
			item.Error = err.Error()
			result.Failed++
		} else {
			item.OK = true
			result.Succeeded++
		}
		result.Results = append(result.Results, item)
	}
	return result, nil
}

// deployableProxy 与 DeployAgentRoute 的准入条件保持一致：启用的 http/https 规则且填了自定义域名
func deployableProxy(proxy Proxy) bool {
	if !proxy.Enabled || len(proxy.CustomDomains) == 0 {
		return false
	}
	return proxy.Type == "http" || proxy.Type == "https"
}

// proxyPassTarget 从 Nginx 配置文本里取出第一条 proxy_pass 的目标地址
func proxyPassTarget(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "proxy_pass ") {
			continue
		}
		value := strings.TrimPrefix(trimmed, "proxy_pass ")
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), ";"))
	}
	return ""
}

// normalizeUpstream 去掉协议前缀与结尾斜杠，便于比较（agent 渲染的是 http://127.0.0.1:8080，应用侧期望的是 127.0.0.1:8080）
func normalizeUpstream(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "https://")
	return strings.TrimSuffix(value, "/")
}
