package frp

import (
	"context"
	"fmt"
)

// 以下三个方法对应 agent 0.10.0 新增的只读端点；老版本 agent 会返回
// 「agent 版本过低」的提示（见 AgentClient 的 wrapEndpointMissing），而不是原始 HTTP 错误。

// AgentMetrics 返回 VPS 主机资源（CPU / 内存 / 磁盘 / 负载 / 运行时长）
func (s *Service) AgentMetrics(ctx context.Context, serverID int64) (map[string]any, error) {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return nil, fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	return client.Metrics(ctx)
}

// AgentNginxLogs 读取 VPS 上 Nginx 的 access / error 日志尾部
func (s *Service) AgentNginxLogs(ctx context.Context, serverID int64, kind string, lines int) (map[string]any, error) {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return nil, fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	if kind != "access" && kind != "error" {
		kind = "access"
	}
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	return client.NginxLogs(ctx, kind, lines)
}

// AgentCerts 列出 VPS 证书目录下的域名与到期时间，供与 Havline 证书库交叉核对
func (s *Service) AgentCerts(ctx context.Context, serverID int64) (map[string]any, error) {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return nil, fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	return client.Certs(ctx)
}

// AgentNginxReload 显式校验并重载 VPS 上的 Nginx，返回失败原因
func (s *Service) AgentNginxReload(ctx context.Context, serverID int64) (map[string]any, error) {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return nil, fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	return client.NginxReload(ctx)
}

// AgentFRPSConfig 回读 VPS 上的 frps.toml，供与「即将下发的内容」比对
func (s *Service) AgentFRPSConfig(ctx context.Context, serverID int64) (map[string]any, error) {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return nil, fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	return client.FRPSConfig(ctx)
}
