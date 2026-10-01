package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/havline/havline/internal/cloudflared"
	"github.com/havline/havline/internal/frp"
	"github.com/havline/havline/internal/monitor"
	"github.com/havline/havline/internal/traffic"
)

// monitorSource 把 frp 服务适配成 monitor.Source：monitor 只关心「状态快照」，
// 怎么从 frp / agent 取是这里的事，两边因此互不依赖。
type monitorSource struct {
	frp        *frp.Service
	cloudflare *cloudflared.Service
	logger     *slog.Logger
	traffic    *traffic.Collector
}

func (s monitorSource) CloudflareRoutes(ctx context.Context) ([]monitor.CloudflareRouteStatus, error) {
	if s.cloudflare == nil {
		return []monitor.CloudflareRouteStatus{}, nil
	}
	statuses, err := s.cloudflare.HealthStatuses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]monitor.CloudflareRouteStatus, 0, len(statuses))
	for _, item := range statuses {
		out = append(out, monitor.CloudflareRouteStatus{
			TunnelID: item.TunnelID,
			Name:     item.Name,
			Domain:   item.Domain,
			State:    item.State,
			Reason:   item.Reason,
			Detail:   item.Detail,
			Known:    item.Known,
		})
	}
	return out, nil
}

func (s monitorSource) RecordCloudflareHealth(ctx context.Context, samples []monitor.CloudflareHealthSample) error {
	if s.cloudflare == nil {
		return nil
	}
	converted := make([]cloudflared.HealthSample, 0, len(samples))
	for _, sample := range samples {
		converted = append(converted, cloudflared.HealthSample{
			TunnelID: sample.TunnelID,
			Name:     sample.Name,
			Domain:   sample.Domain,
			State:    sample.State,
			Reason:   sample.Reason,
			Detail:   sample.Detail,
		})
	}
	return s.cloudflare.RecordHealth(ctx, converted)
}

func (s monitorSource) PruneCloudflareHealth(ctx context.Context) error {
	if s.cloudflare == nil {
		return nil
	}
	_, err := s.cloudflare.PruneHealth(ctx)
	return err
}

func (s monitorSource) Routes(ctx context.Context) ([]monitor.RouteStatus, error) {
	servers, err := s.enabledServers(ctx)
	if err != nil {
		return nil, err
	}
	routes := make([]monitor.RouteStatus, 0, len(servers))
	for _, server := range servers {
		// 单台服务端探测失败不影响其它服务端：它本轮跳过，等下一轮
		health, err := s.frp.RouteHealth(ctx, server.ID)
		if err != nil {
			s.logger.Warn("巡检跳过服务端隧道", "module", "MONITOR", "server_id", server.ID, "error", err.Error())
			continue
		}
		for _, item := range health {
			routes = append(routes, monitor.RouteStatus{
				ServerID:  server.ID,
				Domain:    item.Domain,
				TunnelOK:  item.TunnelOK,
				DNSOK:     item.DNSOK,
				ServiceOK: item.ServiceOK,
				Known:     item.TunnelKnown,
				Detail:    item.TunnelDetail,
			})
		}
	}
	return routes, nil
}

func (s monitorSource) Servers(ctx context.Context) ([]monitor.ServerStatus, error) {
	servers, err := s.enabledServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]monitor.ServerStatus, 0, len(servers))
	for _, server := range servers {
		out = append(out, monitor.ServerStatus{
			ID:              server.ID,
			Name:            server.Name,
			DesiredState:    server.DesiredState,
			ProcessState:    server.ProcessState,
			ConnectionState: server.ConnectionState,
			Detail:          server.LastError,
		})
	}
	return out, nil
}

func (s monitorSource) Agents(ctx context.Context) ([]monitor.AgentStatus, error) {
	servers, err := s.enabledServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]monitor.AgentStatus, 0, len(servers))
	for _, server := range servers {
		// AgentClient 只在「地址与令牌都可用」时才返回 ok：没配 agent 的服务端不参与可达性判定
		client, ok := s.frp.AgentClient(ctx, server.ID)
		if !ok {
			continue
		}
		status := monitor.AgentStatus{ID: server.ID, Name: server.Name, Reachable: true}
		if err := client.Ping(ctx); err != nil {
			status.Reachable = false
			status.Detail = err.Error()
		}
		out = append(out, status)
	}
	return out, nil
}

func (s monitorSource) Drift(ctx context.Context) ([]monitor.DriftStatus, error) {
	servers, err := s.enabledServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]monitor.DriftStatus, 0, len(servers))
	for _, server := range servers {
		report, err := s.frp.RouteDrift(ctx, server.ID)
		if err != nil {
			s.logger.Warn("巡检跳过漂移检测", "module", "MONITOR", "server_id", server.ID, "error", err.Error())
			continue
		}
		status := monitor.DriftStatus{ID: server.ID, Name: server.Name, Count: len(report.Items)}
		for _, item := range report.Items {
			status.Items = append(status.Items, fmt.Sprintf("%s %s", item.Domain, driftKindText(item.Kind)))
		}
		out = append(out, status)
	}
	return out, nil
}

// Traffic 返回若干域名的当前上下行速率：流量采集器内部按 host:port 累计，这里按域名合并。
func (s monitorSource) Traffic(ctx context.Context, domains []string) (map[string]monitor.TrafficRate, error) {
	if s.traffic == nil {
		return nil, nil
	}
	snapshot := s.traffic.SnapshotByHost(domains)
	rates := make(map[string]monitor.TrafficRate, len(snapshot))
	for host, item := range snapshot {
		rates[host] = monitor.TrafficRate{Upload: item.UploadRate, Download: item.DownloadRate}
	}
	return rates, nil
}

func (s monitorSource) enabledServers(ctx context.Context) ([]frp.Server, error) {
	servers, err := s.frp.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	enabled := make([]frp.Server, 0, len(servers))
	for _, server := range servers {
		// 停用中的服务端是用户主动关掉的，不该参与任何告警判定
		if server.Enabled {
			enabled = append(enabled, server)
		}
	}
	return enabled, nil
}

// driftKindText 与公网反代页漂移清单的文案保持一致，通知里才不用回页面去对照。
func driftKindText(kind string) string {
	switch kind {
	case frp.RouteDriftMissing:
		return "未部署"
	case frp.RouteDriftOrphan:
		return "孤儿配置"
	case frp.RouteDriftNotLoaded:
		return "未生效"
	case frp.RouteDriftContentMismatch:
		return "内容不一致"
	default:
		return kind
	}
}

// healthRecorder 把 monitor 的健康结论翻成 frp 侧样本并落库（
// 两侧的状态字面量由 healthStateValue 显式翻译，改动一侧会在这里编译期报错）。
type healthRecorder struct {
	frp *frp.Service
}

func (r healthRecorder) RecordRouteHealth(ctx context.Context, samples []monitor.HealthSample) error {
	converted := make([]frp.RouteHealthSample, 0, len(samples))
	for _, sample := range samples {
		converted = append(converted, frp.RouteHealthSample{
			ServerID: sample.ServerID,
			Domain:   sample.Domain,
			State:    healthStateValue(sample.State),
			Reason:   sample.Reason,
			Detail:   sample.Detail,
		})
	}
	_, err := r.frp.RecordRouteHealth(ctx, converted)
	return err
}

func (r healthRecorder) PruneRouteHealth(ctx context.Context) error {
	_, err := r.frp.PruneRouteHealth(ctx)
	return err
}

func healthStateValue(state string) string {
	switch state {
	case monitor.HealthUp:
		return frp.RouteHealthUp
	case monitor.HealthDown:
		return frp.RouteHealthDown
	default:
		return frp.RouteHealthUnknown
	}
}
