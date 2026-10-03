package frp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// frps 管理接口（webServer）返回的代理状态结构，仅取本模块用到的字段。
type frpsProxyStatus struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	TodayTrafficIn  int64  `json:"today_traffic_in"`
	TodayTrafficOut int64  `json:"today_traffic_out"`
	CurrentConns    int    `json:"cur_conns"`
}

type ProxyTraffic struct {
	TrafficIn      int64 `json:"traffic_in"`
	TrafficOut     int64 `json:"traffic_out"`
	CurConns       int   `json:"cur_conns"`
	TrafficInRate  int64 `json:"traffic_in_rate"`
	TrafficOutRate int64 `json:"traffic_out_rate"`
}

type ServerTraffic struct {
	ServerID           int64                   `json:"server_id"`
	OK                 bool                    `json:"ok"`
	Error              string                  `json:"error,omitempty"`
	Proxies            map[string]ProxyTraffic `json:"proxies"`
	ProxyNamesByDomain map[string]string       `json:"proxy_names_by_domain,omitempty"`
}

type ProxiesTraffic struct {
	Items     map[int64]ProxyTraffic `json:"items"`
	Errors    map[int64]string       `json:"errors,omitempty"`
	SampledAt string                 `json:"sampled_at"`
}

type trafficSample struct {
	at  time.Time
	in  int64
	out int64
}

type frpsTrafficClient struct {
	httpClient *http.Client
}

func newFRPSTrafficClient() *frpsTrafficClient {
	return &frpsTrafficClient{httpClient: &http.Client{Timeout: 6 * time.Second}}
}

// fetchServerTraffic 版本自适应采集：
//   - frp >= v0.71.0：GET /api/v2/proxies（一次返回全部类型，data.items[].name 含 user 前缀）；
//   - frp < v0.71.0（如 v0.68.0）：无 V2 API（/api/v2/* 404），用 GET /api/proxies（一次返回全部类型）。
//
// 判定方式：V2 返回 404 page not found 即回退旧端点；其余错误如实返回。
func (c *frpsTrafficClient) fetchServerTraffic(ctx context.Context, server Server, password string) ServerTraffic {
	empty := map[string]ProxyTraffic{}
	if strings.TrimSpace(server.Options.DashboardAddr) == "" || server.Options.DashboardPort == 0 {
		return ServerTraffic{ServerID: server.ID, Error: "未配置 frps 管理接口（webServer）", Proxies: empty}
	}

	// 优先 V2 API（v0.71.0+）
	v2URL := fmt.Sprintf("http://%s:%d/api/v2/proxies", server.Options.DashboardAddr, server.Options.DashboardPort)
	body, status, err := c.getJSON(ctx, v2URL, server, password)
	if err != nil {
		return ServerTraffic{ServerID: server.ID, Error: fmt.Sprintf("frps 管理接口请求失败（GET %s）: %v", v2URL, err), Proxies: empty}
	}
	if status == http.StatusOK {
		return c.parseV2Proxies(server, body)
	}
	if status != http.StatusNotFound {
		return ServerTraffic{ServerID: server.ID, Error: fmt.Sprintf("frps 管理接口返回 %d（GET %s）: %s", status, v2URL, strings.TrimSpace(string(body))), Proxies: empty}
	}

	// 旧版没有全量列表端点，流量采集无法在不知道代理名时继续；公网反代健康检查使用
	// fetchLegacyRouteProxies 按候选规则逐条查询 /api/proxies/{user.name}。
	return ServerTraffic{ServerID: server.ID, Error: "frps 旧版 dashboard 不支持全量代理列表，请通过规则名逐条查询", Proxies: empty}
}

// legacyProxyInfo 是 frp 0.68.0 的单代理响应结构。
type legacyProxyInfo struct {
	Name string `json:"name"`
	User string `json:"user"`
	Conf struct {
		CustomDomains []string `json:"customDomains"`
	} `json:"conf"`
	Status          string `json:"status"`
	TodayTrafficIn  int64  `json:"todayTrafficIn"`
	TodayTrafficOut int64  `json:"todayTrafficOut"`
	CurConns        int    `json:"curConns"`
}

// fetchLegacyProxyByName 兼容 frp 0.68.0：旧 dashboard 只能按实际代理名查询，
// 代理名遵循 user.proxyName 规则（例如 admin.ssssss）。
func (c *frpsTrafficClient) fetchLegacyProxyByName(ctx context.Context, server Server, password, proxyName string) (legacyProxyInfo, int, error) {
	url := fmt.Sprintf("http://%s:%d/api/proxies/%s", server.Options.DashboardAddr, server.Options.DashboardPort, proxyName)
	body, status, err := c.getJSON(ctx, url, server, password)
	if err != nil {
		return legacyProxyInfo{}, 0, err
	}
	if status != http.StatusOK {
		return legacyProxyInfo{}, status, fmt.Errorf("GET %s 返回 %d: %s", url, status, strings.TrimSpace(string(body)))
	}
	var info legacyProxyInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return legacyProxyInfo{}, status, fmt.Errorf("解析 GET %s 响应失败: %v", url, err)
	}
	return info, status, nil
}

// fetchLegacyRouteProxies 按候选规则逐条查询旧版 frps，并保留域名到真实代理名的映射。
func (c *frpsTrafficClient) fetchLegacyRouteProxies(ctx context.Context, server Server, password string, names []string) ServerTraffic {
	empty := map[string]ProxyTraffic{}
	proxies := make(map[string]ProxyTraffic)
	proxyNamesByDomain := make(map[string]string)
	var lastErr error
	for _, name := range names {
		proxyName := name
		if user := strings.TrimSpace(server.Options.User); user != "" && !strings.HasPrefix(proxyName, user+".") {
			proxyName = user + "." + proxyName
		}
		info, _, err := c.fetchLegacyProxyByName(ctx, server, password, proxyName)
		if err != nil {
			lastErr = err
			continue
		}
		traffic := ProxyTraffic{TrafficIn: info.TodayTrafficIn, TrafficOut: info.TodayTrafficOut, CurConns: info.CurConns}
		proxies[info.Name] = traffic
		for _, domain := range info.Conf.CustomDomains {
			if domain = strings.ToLower(strings.TrimSpace(domain)); domain != "" {
				proxies["domain:"+domain] = traffic
				proxyNamesByDomain[domain] = info.Name
			}
		}
	}
	if len(proxies) == 0 && lastErr != nil {
		return ServerTraffic{ServerID: server.ID, Error: lastErr.Error(), Proxies: empty}
	}
	return ServerTraffic{ServerID: server.ID, OK: true, Proxies: proxies, ProxyNamesByDomain: proxyNamesByDomain}
}

// getJSON 执行带 Basic Auth 的 GET 请求并返回响应体与状态码。
func (c *frpsTrafficClient) getJSON(ctx context.Context, url string, server Server, password string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if user := strings.TrimSpace(server.Options.DashboardUser); user != "" {
		req.SetBasicAuth(user, password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// parseV2Proxies 解析 V2 API 响应（实测结构）：
// {"code":200,"data":{"items":[{"name":"admin.ssssss","user":"admin","spec":{"type":"http","http":{"customDomains":[...]}},"status":{"phase":"online",...}}]}}
func (c *frpsTrafficClient) parseV2Proxies(server Server, body []byte) ServerTraffic {
	empty := map[string]ProxyTraffic{}
	var payload struct {
		Data struct {
			Items []struct {
				Name string `json:"name"`
				User string `json:"user"`
				Spec struct {
					Type string `json:"type"`
					HTTP struct {
						CustomDomains []string `json:"customDomains"`
					} `json:"http"`
				} `json:"spec"`
				Status struct {
					Phase           string `json:"phase"`
					TodayTrafficIn  int64  `json:"todayTrafficIn"`
					TodayTrafficOut int64  `json:"todayTrafficOut"`
					CurConns        int    `json:"curConns"`
				} `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ServerTraffic{ServerID: server.ID, Error: fmt.Sprintf("解析 V2 响应失败: %v", err), Proxies: empty}
	}
	proxies := make(map[string]ProxyTraffic)
	proxyNamesByDomain := make(map[string]string)
	for _, item := range payload.Data.Items {
		traffic := ProxyTraffic{
			TrafficIn:  item.Status.TodayTrafficIn,
			TrafficOut: item.Status.TodayTrafficOut,
			CurConns:   item.Status.CurConns,
		}
		proxies[item.Name] = traffic
		// 域名索引：健康检查按域名直接匹配（比代理名猜测可靠）
		for _, d := range item.Spec.HTTP.CustomDomains {
			domain := strings.ToLower(strings.TrimSpace(d))
			if domain != "" {
				proxies["domain:"+domain] = traffic
				proxyNamesByDomain[domain] = item.Name
			}
		}
	}
	return ServerTraffic{ServerID: server.ID, OK: true, Proxies: proxies, ProxyNamesByDomain: proxyNamesByDomain}
}

// validDashboardAddr 宽松校验管理接口地址（IP 或域名，不带协议与端口）。
func validDashboardAddr(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return true
	}
	return validHost(addr)
}
