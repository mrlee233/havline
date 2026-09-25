package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fonu/fonu/internal/frp"
	"github.com/fonu/fonu/internal/monitor"
)

// monitorSource 把 frp 服务适配成 monitor.Source：monitor 只关心「状态快照」，
// 怎么从 frp / agent 取是这里的事，两边因此互不依赖。
type monitorSource struct {
	frp    *frp.Service
	logger *slog.Logger
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
			routes = append(routes, monitor.RouteStatus{Domain: item.Domain, TunnelOK: item.TunnelOK, Detail: item.TunnelDetail})
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
