package proxy

import (
	"context"
)

// ReplaceCustomEndpoints 用解析出的占用集合替换该规则在 proxy_custom_endpoints 里的记录。
// 手动（custom）Nginx 规则保存或回滚成功后调用，使其占用对其它规则的保存校验可见。
func (s *Store) ReplaceCustomEndpoints(ctx context.Context, ruleID int64, endpoints []Endpoint) error {
	if err := s.ClearCustomEndpoints(ctx, ruleID); err != nil {
		return err
	}
	for _, endpoint := range endpoints {
		if _, err := s.querier().ExecContext(ctx, `
			INSERT INTO proxy_custom_endpoints(rule_id, hostname, listen_port)
			VALUES (?, ?, ?)
		`, ruleID, endpoint.Hostname, endpoint.Port); err != nil {
			return err
		}
	}
	return nil
}

// ClearCustomEndpoints 清除该规则的手动配置占用（切回自动模式或删除规则时调用）
func (s *Store) ClearCustomEndpoints(ctx context.Context, ruleID int64) error {
	_, err := s.querier().ExecContext(ctx, `DELETE FROM proxy_custom_endpoints WHERE rule_id = ?`, ruleID)
	return err
}

// EnsureEndpointsAvailable 校验一批 (域名, 端口) 是否已被其它规则占用，
// 用于手动 Nginx 配置的保存前检查（与表单规则的校验共用同一条判定）。
func (s *Store) EnsureEndpointsAvailable(ctx context.Context, endpoints []Endpoint, excludeRuleID int64) error {
	for _, endpoint := range endpoints {
		if err := s.ensureHostAvailable(ctx, endpoint.Hostname, endpoint.Port, excludeRuleID); err != nil {
			return err
		}
	}
	return nil
}
