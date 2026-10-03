package cloudflared

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/havline/havline/internal/fsutil"
	"github.com/havline/havline/internal/validate"
)

// RouteView 是配置页展示的一条 ingress 路由。
type RouteView struct {
	ID       int64  `json:"id,omitempty"`
	Hostname string `json:"hostname"`
	Path     string `json:"path"`
	Service  string `json:"service"`
	Source   string `json:"source"`
}

// TunnelRoutes 是配置页需要的路由列表。
type TunnelRoutes struct {
	Managed  bool        `json:"managed"`
	Routes   []RouteView `json:"routes"`
	CatchAll string      `json:"catch_all"`
}

// RouteInput 是手工隧道路由保存入参。
type RouteInput struct {
	Hostname string `json:"hostname"`
	Path     string `json:"path"`
	Service  string `json:"service"`
}

// Routes 返回托管规则的派生路由，或手工 config.yml 中的 ingress 路由。
func (s *Service) Routes(ctx context.Context, id int64) (TunnelRoutes, error) {
	tunnel, err := s.store.Get(ctx, id)
	if err != nil {
		return TunnelRoutes{}, err
	}
	if tunnel.Managed {
		return s.managedRoutes(ctx, tunnel)
	}
	return manualRoutes(tunnel.ConfigPath)
}

func (s *Service) managedRoutes(ctx context.Context, tunnel Tunnel) (TunnelRoutes, error) {
	if s.rules == nil {
		return TunnelRoutes{Managed: true, Routes: []RouteView{}, CatchAll: "http_status:404"}, nil
	}
	rules, err := s.rules.List(ctx)
	if err != nil {
		return TunnelRoutes{}, err
	}
	out := TunnelRoutes{Managed: true, Routes: []RouteView{}, CatchAll: "http_status:404"}
	for _, rule := range rulesForTunnel(rules, tunnel.ID) {
		for _, host := range rule.Hostnames() {
			out.Routes = append(out.Routes, RouteView{
				ID: rule.ID, Hostname: host, Path: "*", Service: rule.Upstream, Source: "rule",
			})
		}
	}
	return out, nil
}

func manualRoutes(configPath string) (TunnelRoutes, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return TunnelRoutes{Routes: []RouteView{}, CatchAll: "http_status:404"}, nil
		}
		return TunnelRoutes{}, err
	}
	out := TunnelRoutes{Routes: []RouteView{}, CatchAll: "http_status:404"}
	lines := strings.Split(string(data), "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		host, found := strings.CutPrefix(line, "- hostname:")
		if !found {
			continue
		}
		hostname := strings.TrimSpace(host)
		if hostname == "" || i+1 >= len(lines) {
			continue
		}
		path := "*"
		next := i + 1
		if value, ok := strings.CutPrefix(strings.TrimSpace(lines[next]), "path:"); ok {
			path = strings.TrimSpace(value)
			next++
		}
		if next >= len(lines) {
			continue
		}
		service, ok := strings.CutPrefix(strings.TrimSpace(lines[next]), "service:")
		if !ok {
			continue
		}
		out.Routes = append(out.Routes, RouteView{
			Hostname: hostname, Path: path, Service: strings.TrimSpace(service), Source: "config",
		})
	}
	if len(out.Routes) > 0 {
		catchAll, err := catchAllService(data)
		if err == nil && catchAll != "" {
			out.CatchAll = catchAll
		}
	}
	return out, nil
}

func catchAllService(data []byte) (string, error) {
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "- service:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "- service:")), nil
		}
	}
	return "", fmt.Errorf("未找到 catch-all")
}

// ReplaceRoutes 保存手工隧道的路由列表，并重新生成本地与云端 ingress。
func (s *Service) ReplaceRoutes(ctx context.Context, id int64, routes []RouteInput) (TunnelRoutes, error) {
	tunnel, err := s.store.Get(ctx, id)
	if err != nil {
		return TunnelRoutes{}, err
	}
	if tunnel.Managed {
		return TunnelRoutes{}, fmt.Errorf("托管隧道的路由由反向代理规则管理")
	}
	ingress, hostnames, err := buildManualIngress(routes)
	if err != nil {
		return TunnelRoutes{}, err
	}
	if err := fsutil.WriteFileAtomic(tunnel.ConfigPath, []byte(BuildConfig(tunnel.TunnelID, tunnel.CredsPath, tunnel.Network, ingress)), 0o640); err != nil {
		return TunnelRoutes{}, err
	}
	if tunnel.Mode == ModeAccountLocal && tunnel.TunnelID != "" {
		api, err := s.cloudflareAPI(ctx)
		if err != nil {
			return TunnelRoutes{}, err
		}
		if err := api.PutIngress(ctx, tunnel.TunnelID, ingress); err != nil {
			return TunnelRoutes{}, err
		}
		if err := s.syncTunnelDNS(ctx, api, tunnel.ID, tunnel.TunnelID, hostnames); err != nil {
			return TunnelRoutes{}, err
		}
	}
	if err := s.restartIfRunning(ctx, tunnel.ID); err != nil {
		return TunnelRoutes{}, err
	}
	return s.Routes(ctx, id)
}

func buildManualIngress(routes []RouteInput) ([]IngressRule, []string, error) {
	seen := map[string]struct{}{}
	ingress := make([]IngressRule, 0, len(routes)+1)
	hostnames := make([]string, 0, len(routes))
	for _, route := range routes {
		hostname := strings.ToLower(strings.TrimSpace(route.Hostname))
		if err := validate.Domain(hostname); err != nil {
			return nil, nil, fmt.Errorf("域名 %s 无效：%w", hostname, err)
		}
		if _, ok := seen[hostname]; ok {
			return nil, nil, fmt.Errorf("域名 %s 重复", hostname)
		}
		seen[hostname] = struct{}{}
		service := strings.TrimSpace(route.Service)
		if service == "" || !strings.Contains(service, "://") {
			return nil, nil, fmt.Errorf("域名 %s 的源服务格式无效", hostname)
		}
		path := strings.TrimSpace(route.Path)
		// Cloudflare ingress 的 path 是正则；通配路由必须省略 path，发送 "*" 会被判为非法正则。
		if path == "" || path == "*" {
			path = ""
		}
		ingress = append(ingress, IngressRule{Hostname: hostname, Path: path, Service: service})
		hostnames = append(hostnames, hostname)
	}
	ingress = append(ingress, IngressRule{Service: "http_status:404"})
	return ingress, hostnames, nil
}
