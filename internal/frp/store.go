package frp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) ListServers(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx, serverSelect+` ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	servers := []Server{}
	for rows.Next() {
		server, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

func (s *Store) GetServer(ctx context.Context, id int64) (Server, error) {
	server, err := scanServer(s.db.QueryRowContext(ctx, serverSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, fmt.Errorf("FRP 服务端不存在")
	}
	return server, err
}

func (s *Store) GetEnabledServer(ctx context.Context) (Server, error) {
	server, err := scanServer(s.db.QueryRowContext(ctx, serverSelect+` WHERE enabled = 1 ORDER BY id LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, fmt.Errorf("未启用 FRP 服务端")
	}
	return server, err
}

func (s *Store) ListEnabledServers(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx, serverSelect+` WHERE enabled = 1 ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	servers := []Server{}
	for rows.Next() {
		server, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

func (s *Store) CreateServer(ctx context.Context, in ServerInput, tokenEnc, agentTokenEnc, sshSecretEnc string) (Server, error) {
	opts, _ := json.Marshal(in.Options)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO frp_servers(name, server_addr, server_port, auth_token_enc, tls_enabled, tls_server_name, enabled, config_json, agent_url, agent_token_enc, ssh_host, ssh_port, ssh_user, ssh_auth, ssh_secret_enc, mgmt_enabled, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`, in.Name, in.ServerAddr, in.ServerPort, tokenEnc, boolInt(in.TLS), in.TLSServerName, boolInt(in.Enabled), string(opts), in.AgentURL, agentTokenEnc, in.SSHHost, in.SSHPort, in.SSHUser, in.SSHAuth, sshSecretEnc, boolInt(in.MgmtEnabled))
	if err != nil {
		return Server{}, mapUniqueError(err, "FRP 服务端名称已存在")
	}
	id, _ := result.LastInsertId()
	return s.GetServer(ctx, id)
}

func (s *Store) UpdateServer(ctx context.Context, id int64, in ServerInput, tokenEnc string, updateToken bool, agentTokenEnc string, updateAgentToken bool, sshSecretEnc string, updateSSHSecret bool) (Server, error) {
	options, _ := json.Marshal(in.Options)
	query := `UPDATE frp_servers SET name = ?, server_addr = ?, server_port = ?, tls_enabled = ?, tls_server_name = ?, enabled = ?, config_json = ?, agent_url = ?, ssh_host = ?, ssh_port = ?, ssh_user = ?, ssh_auth = ?, mgmt_enabled = ?, updated_at = datetime('now') WHERE id = ?`
	args := []any{in.Name, in.ServerAddr, in.ServerPort, boolInt(in.TLS), in.TLSServerName, boolInt(in.Enabled), string(options), in.AgentURL, in.SSHHost, in.SSHPort, in.SSHUser, in.SSHAuth, boolInt(in.MgmtEnabled), id}
	if updateToken || updateAgentToken || updateSSHSecret {
		query = `UPDATE frp_servers SET name = ?, server_addr = ?, server_port = ?, auth_token_enc = CASE WHEN ? THEN ? ELSE auth_token_enc END, agent_url = ?, agent_token_enc = CASE WHEN ? THEN ? ELSE agent_token_enc END, ssh_host = ?, ssh_port = ?, ssh_user = ?, ssh_auth = ?, ssh_secret_enc = CASE WHEN ? THEN ? ELSE ssh_secret_enc END, tls_enabled = ?, tls_server_name = ?, enabled = ?, config_json = ?, mgmt_enabled = ?, updated_at = datetime('now') WHERE id = ?`
		args = []any{in.Name, in.ServerAddr, in.ServerPort, updateToken, tokenEnc, in.AgentURL, updateAgentToken, agentTokenEnc, in.SSHHost, in.SSHPort, in.SSHUser, in.SSHAuth, updateSSHSecret, sshSecretEnc, boolInt(in.TLS), in.TLSServerName, boolInt(in.Enabled), string(options), boolInt(in.MgmtEnabled), id}
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return Server{}, mapUniqueError(err, "FRP 服务端名称已存在")
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return Server{}, fmt.Errorf("FRP 服务端不存在")
	}
	return s.GetServer(ctx, id)
}

func (s *Store) DeleteServer(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM frp_servers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return fmt.Errorf("FRP 服务端不存在")
	}
	return nil
}

func (s *Store) ServerRuntimeStatus(ctx context.Context, serverID int64) (ServerRuntimeStatus, error) {
	status := ServerRuntimeStatus{
		ServerID: serverID, Status: "stopped", DesiredState: "stopped",
		ProcessState: "stopped", ConnectionState: "unknown", StateSource: "legacy",
	}
	var pid, exitCode sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT status, COALESCE(desired_state, status), COALESCE(process_state, 'stopped'),
		       COALESCE(connection_state, 'unknown'), COALESCE(state_source, 'legacy'),
		       process_pid, exit_code, COALESCE(last_error, ''), COALESCE(last_started_at, ''),
		       COALESCE(last_connected_at, ''), COALESCE(last_disconnected_at, '')
		FROM frp_runtime_status WHERE server_id = ?
	`, serverID).Scan(
		&status.Status, &status.DesiredState, &status.ProcessState, &status.ConnectionState,
		&status.StateSource, &pid, &exitCode, &status.LastError, &status.LastStartedAt,
		&status.LastConnectedAt, &status.LastDisconnectedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if pid.Valid {
		status.ProcessPID = int(pid.Int64)
	}
	if exitCode.Valid {
		status.ExitCode = int(exitCode.Int64)
	}
	return status, err
}

func (s *Store) SetServerDesiredState(ctx context.Context, status ServerRuntimeStatus) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO frp_runtime_status(server_id, status, desired_state, state_source, last_started_at, last_error, updated_at)
		VALUES (?, ?, ?, 'manual', NULLIF(?, ''), ?, datetime('now'))
		ON CONFLICT(server_id) DO UPDATE SET
			status = excluded.status,
			desired_state = excluded.desired_state,
			state_source = 'manual',
			last_started_at = COALESCE(excluded.last_started_at, frp_runtime_status.last_started_at),
			last_error = excluded.last_error,
			updated_at = datetime('now')
	`, status.ServerID, status.Status, status.DesiredState, status.LastStartedAt, status.LastError)
	return err
}

func (s *Store) SetServerObservedState(ctx context.Context, status ServerRuntimeStatus) error {
	desiredState := "stopped"
	if status.ProcessState == "running" {
		desiredState = "running"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO frp_runtime_status(
			server_id, status, desired_state, process_state, connection_state, state_source,
			process_pid, exit_code, last_error, last_started_at, last_connected_at, last_disconnected_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, 0), NULLIF(?, 0), ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), datetime('now'))
		ON CONFLICT(server_id) DO UPDATE SET
			process_state = excluded.process_state,
			connection_state = excluded.connection_state,
			state_source = excluded.state_source,
			process_pid = excluded.process_pid,
			exit_code = excluded.exit_code,
			last_error = excluded.last_error,
			last_started_at = COALESCE(NULLIF(excluded.last_started_at, ''), frp_runtime_status.last_started_at),
			last_connected_at = COALESCE(NULLIF(excluded.last_connected_at, ''), frp_runtime_status.last_connected_at),
			last_disconnected_at = COALESCE(NULLIF(excluded.last_disconnected_at, ''), frp_runtime_status.last_disconnected_at),
			updated_at = datetime('now')
	`, status.ServerID, desiredState, desiredState, status.ProcessState, status.ConnectionState, status.StateSource,
		status.ProcessPID, status.ExitCode, status.LastError, status.LastStartedAt,
		status.LastConnectedAt, status.LastDisconnectedAt)
	return err
}

func (s *Store) AddServerDiagnostic(ctx context.Context, diagnostic ServerDiagnostics) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO frp_server_diagnostics(server_id, checked_at, available, latency_ms, error)
		VALUES (?, datetime('now'), ?, NULLIF(?, 0), ?)
	`, diagnostic.ServerID, boolInt(diagnostic.Available), diagnostic.LatencyMS, diagnostic.Error); err != nil {
		return err
	}
	// 只保留每个服务端最近 60 条采样，30 分钟曲线窗口远用不到这么多
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM frp_server_diagnostics
		WHERE server_id = ? AND id NOT IN (
			SELECT id FROM frp_server_diagnostics WHERE server_id = ? ORDER BY id DESC LIMIT 60
		)
	`, diagnostic.ServerID, diagnostic.ServerID)
	return err
}

func (s *Store) ServerDiagnosticDetail(ctx context.Context, serverID int64) (ServerStatusDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT checked_at, available, COALESCE(latency_ms, 0), COALESCE(error, '')
		FROM frp_server_diagnostics
		WHERE server_id = ? AND checked_at >= datetime('now', '-30 minutes')
		ORDER BY checked_at ASC, id ASC
	`, serverID)
	if err != nil {
		return ServerStatusDetail{}, err
	}
	defer rows.Close()
	detail := ServerStatusDetail{Samples: []ServerSample{}}
	for rows.Next() {
		var sample ServerSample
		var available int
		if err := rows.Scan(&sample.CheckedAt, &available, &sample.LatencyMS, &sample.Error); err != nil {
			return ServerStatusDetail{}, err
		}
		sample.Available = available == 1
		detail.Samples = append(detail.Samples, sample)
	}
	if err := rows.Err(); err != nil {
		return ServerStatusDetail{}, err
	}
	return summarizeServerSamples(detail.Samples), nil
}

func summarizeServerSamples(samples []ServerSample) ServerStatusDetail {
	detail := ServerStatusDetail{Samples: samples}
	if len(samples) == 0 {
		return detail
	}
	detail.Available = samples[len(samples)-1].Available
	detail.LastCheckedAt = samples[len(samples)-1].CheckedAt
	detail.LastError = samples[len(samples)-1].Error
	var total int64
	var available int
	for _, sample := range samples {
		if !sample.Available {
			continue
		}
		available++
		total += sample.LatencyMS
		if sample.LatencyMS > detail.PeakLatency {
			detail.PeakLatency = sample.LatencyMS
		}
	}
	detail.Availability = float64(available) / float64(len(samples)) * 100
	if available > 0 {
		detail.AverageLatency = total / int64(available)
	}
	return detail
}

func (s *Store) CountOtherEnabledServers(ctx context.Context, id int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM frp_servers WHERE enabled = 1 AND id != ?`, id).Scan(&count)
	return count, err
}

func (s *Store) SSHSecret(ctx context.Context, id int64) (string, error) {
	var secret string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(ssh_secret_enc, '') FROM frp_servers WHERE id = ?`, id).Scan(&secret)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("FRP 服务端不存在")
	}
	return secret, err
}

func (s *Store) AgentToken(ctx context.Context, id int64) (string, error) {
	var token string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(agent_token_enc, '') FROM frp_servers WHERE id = ?`, id).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("FRP 服务端不存在")
	}
	return token, err
}

func (s *Store) SetAgentTransport(ctx context.Context, id int64, agentURL, transport string, localPort int, tlsPin, listenAddr, state, lastError string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE frp_servers
		SET agent_url = ?, agent_transport = ?, agent_local_port = ?, agent_tls_pin = ?,
		    agent_listen_addr = ?, agent_transport_state = ?, agent_transport_error = ?,
		    updated_at = datetime('now')
		WHERE id = ?
	`, agentURL, transport, localPort, tlsPin, listenAddr, state, lastError, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return fmt.Errorf("FRP 服务端不存在")
	}
	return nil
}

// SetAgentTLSPinRotation 更新当前证书 pin、过渡期旧 pin 与证书到期时间。
func (s *Store) SetAgentTLSPinRotation(ctx context.Context, id int64, pin, prevPin, prevUntil, notAfter string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE frp_servers
		SET agent_tls_pin = ?, agent_tls_pin_prev = ?, agent_tls_pin_prev_until = ?, agent_tls_not_after = ?,
		    updated_at = datetime('now')
		WHERE id = ?
	`, pin, prevPin, prevUntil, notAfter, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return fmt.Errorf("FRP 服务端不存在")
	}
	return nil
}

// SetAgentFirewallState 记录最近一次主机防火墙检测 / 放行结果，供前端展示。
func (s *Store) SetAgentFirewallState(ctx context.Context, id int64, state string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE frp_servers SET agent_firewall_state = ?, updated_at = datetime('now') WHERE id = ?
	`, state, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return fmt.Errorf("FRP 服务端不存在")
	}
	return nil
}

// SetAgentTLSNotAfter 只同步证书到期时间，不改动 pin（用于旧记录补齐）。
func (s *Store) SetAgentTLSNotAfter(ctx context.Context, id int64, notAfter string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE frp_servers SET agent_tls_not_after = ?, updated_at = datetime('now') WHERE id = ?
	`, notAfter, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return fmt.Errorf("FRP 服务端不存在")
	}
	return nil
}

func (s *Store) Token(ctx context.Context, id int64) (string, error) {
	var token string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(auth_token_enc, '') FROM frp_servers WHERE id = ?`, id).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("FRP 服务端不存在")
	}
	return token, err
}

func (s *Store) OIDCSecret(ctx context.Context, id int64) (string, error) {
	var secret string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(oidc_client_secret_enc, '') FROM frp_servers WHERE id = ?`, id).Scan(&secret)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("FRP 服务端不存在")
	}
	return secret, err
}

func (s *Store) SetOIDCSecret(ctx context.Context, id int64, value string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE frp_servers SET oidc_client_secret_enc = ?, updated_at = datetime('now') WHERE id = ?`, value, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return fmt.Errorf("FRP 服务端不存在")
	}
	return nil
}

func (s *Store) DashboardPassword(ctx context.Context, id int64) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(dashboard_password_enc, '') FROM frp_servers WHERE id = ?`, id).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("FRP 服务端不存在")
	}
	return value, err
}

func (s *Store) SetDashboardPassword(ctx context.Context, id int64, value string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE frp_servers SET dashboard_password_enc = ?, updated_at = datetime('now') WHERE id = ?`, value, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return fmt.Errorf("FRP 服务端不存在")
	}
	return nil
}

func (s *Store) ListProxies(ctx context.Context) ([]Proxy, error) {
	rows, err := s.db.QueryContext(ctx, proxySelect+` ORDER BY server_id ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProxies(rows)
}

func (s *Store) ListProxiesByServer(ctx context.Context, serverID int64) ([]Proxy, error) {
	rows, err := s.db.QueryContext(ctx, proxySelect+` WHERE server_id = ? ORDER BY id ASC`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProxies(rows)
}

func (s *Store) ListEnabledProxies(ctx context.Context, serverID int64) ([]Proxy, error) {
	rows, err := s.db.QueryContext(ctx, proxySelect+` WHERE server_id = ? AND enabled = 1 ORDER BY id ASC`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProxies(rows)
}

func (s *Store) GetProxy(ctx context.Context, id int64) (Proxy, error) {
	proxy, err := scanProxy(s.db.QueryRowContext(ctx, proxySelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Proxy{}, fmt.Errorf("FRP 规则不存在")
	}
	return proxy, err
}

func (s *Store) CreateProxy(ctx context.Context, in ProxyInput) (Proxy, error) {
	domains, _ := json.Marshal(in.CustomDomains)
	options, _ := json.Marshal(in.Options)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO frp_proxies(server_id, name, type, local_ip, local_port, remote_port, custom_domains_json, host_header_rewrite, config_json, enabled, remark, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`, in.ServerID, in.Name, in.Type, in.LocalIP, in.LocalPort, in.RemotePort, string(domains), in.HostHeaderRewrite, string(options), boolInt(in.Enabled), in.Remark)
	if err != nil {
		return Proxy{}, mapUniqueError(err, "FRP 规则名称或 TCP 远程端口已存在")
	}
	id, _ := result.LastInsertId()
	return s.GetProxy(ctx, id)
}

func (s *Store) UpdateProxy(ctx context.Context, id int64, in ProxyInput) (Proxy, error) {
	domains, _ := json.Marshal(in.CustomDomains)
	options, _ := json.Marshal(in.Options)
	result, err := s.db.ExecContext(ctx, `
		UPDATE frp_proxies SET server_id = ?, name = ?, type = ?, local_ip = ?, local_port = ?, remote_port = ?, custom_domains_json = ?, host_header_rewrite = ?, config_json = ?, enabled = ?, remark = ?, updated_at = datetime('now')
		WHERE id = ?
	`, in.ServerID, in.Name, in.Type, in.LocalIP, in.LocalPort, in.RemotePort, string(domains), in.HostHeaderRewrite, string(options), boolInt(in.Enabled), in.Remark, id)
	if err != nil {
		return Proxy{}, mapUniqueError(err, "FRP 规则名称或 TCP 远程端口已存在")
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return Proxy{}, fmt.Errorf("FRP 规则不存在")
	}
	return s.GetProxy(ctx, id)
}

func (s *Store) DeleteProxy(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM frp_proxies WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return fmt.Errorf("FRP 规则不存在")
	}
	return nil
}

func scanProxies(rows *sql.Rows) ([]Proxy, error) {
	proxies := []Proxy{}
	for rows.Next() {
		proxy, err := scanProxy(rows)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, proxy)
	}
	return proxies, rows.Err()
}

const serverSelect = `
SELECT id, name, server_addr, server_port, tls_enabled, tls_server_name, enabled,
       CASE WHEN auth_token_enc IS NOT NULL AND auth_token_enc != '' THEN 1 ELSE 0 END,
       CASE WHEN oidc_client_secret_enc IS NOT NULL AND oidc_client_secret_enc != '' THEN 1 ELSE 0 END,
       CASE WHEN dashboard_password_enc IS NOT NULL AND dashboard_password_enc != '' THEN 1 ELSE 0 END,
       COALESCE(agent_url, ''), CASE WHEN COALESCE(agent_token_enc, '') != '' THEN 1 ELSE 0 END,
       COALESCE(ssh_host, ''), COALESCE(ssh_port, 22), COALESCE(ssh_user, ''), COALESCE(ssh_auth, 'key'),
       CASE WHEN COALESCE(ssh_secret_enc, '') != '' THEN 1 ELSE 0 END, COALESCE(mgmt_enabled, 0),
       COALESCE(agent_transport, 'http'), COALESCE(agent_local_port, 0), COALESCE(agent_tls_pin, ''),
       COALESCE(agent_tls_pin_prev, ''), COALESCE(agent_tls_pin_prev_until, ''), COALESCE(agent_tls_not_after, ''),
       COALESCE(agent_firewall_state, ''),
       COALESCE(agent_listen_addr, ''), COALESCE(agent_transport_state, 'unknown'), COALESCE(agent_transport_error, ''),
       COALESCE(config_json, '{}'), created_at, updated_at
FROM frp_servers`

const proxySelect = `
SELECT id, server_id, name, type, local_ip, local_port, remote_port, custom_domains_json, host_header_rewrite, COALESCE(config_json, '{}'), enabled, remark, created_at, updated_at
FROM frp_proxies`

func scanServer(row interface{ Scan(dest ...any) error }) (Server, error) {
	var server Server
	var tls, enabled, token, oidcSecret, dashboardPwd, agentConfigured, sshConfigured, mgmtEnabled int
	var configJSON, created, updated string
	err := row.Scan(&server.ID, &server.Name, &server.ServerAddr, &server.ServerPort, &tls, &server.TLSServerName, &enabled, &token, &oidcSecret, &dashboardPwd, &server.AgentURL, &agentConfigured, &server.SSHHost, &server.SSHPort, &server.SSHUser, &server.SSHAuth, &sshConfigured, &mgmtEnabled, &server.AgentTransport, &server.AgentLocalPort, &server.AgentTLSPin, &server.AgentTLSPinPrev, &server.AgentTLSPinPrevUntil, &server.AgentTLSNotAfter, &server.AgentFirewallState, &server.AgentListenAddr, &server.AgentTransportState, &server.AgentTransportError, &configJSON, &created, &updated)
	if err != nil {
		return Server{}, err
	}
	server.TLS = tls == 1
	server.Enabled = enabled == 1
	server.HasToken = token == 1
	server.HasOIDCSecret = oidcSecret == 1
	server.DashboardHasPwd = dashboardPwd == 1
	server.AgentConfigured = agentConfigured == 1
	server.SSHConfigured = sshConfigured == 1
	server.MgmtEnabled = mgmtEnabled == 1
	_ = json.Unmarshal([]byte(configJSON), &server.Options)
	server.Options = normalizeServerOptions(server.Options)
	server.CreatedAt = parseTime(created)
	server.UpdatedAt = parseTime(updated)
	return server, nil
}

func scanProxy(row interface{ Scan(dest ...any) error }) (Proxy, error) {
	var proxy Proxy
	var domains, configJSON, created, updated string
	var enabled int
	if err := row.Scan(&proxy.ID, &proxy.ServerID, &proxy.Name, &proxy.Type, &proxy.LocalIP, &proxy.LocalPort, &proxy.RemotePort, &domains, &proxy.HostHeaderRewrite, &configJSON, &enabled, &proxy.Remark, &created, &updated); err != nil {
		return Proxy{}, err
	}
	_ = json.Unmarshal([]byte(domains), &proxy.CustomDomains)
	_ = json.Unmarshal([]byte(configJSON), &proxy.Options)
	proxy.Enabled = enabled == 1
	proxy.CreatedAt = parseTime(created)
	proxy.UpdatedAt = parseTime(updated)
	return proxy, nil
}

func mapUniqueError(err error, message string) error {
	if strings.Contains(err.Error(), "UNIQUE") {
		return fmt.Errorf(message)
	}
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse("2006-01-02 15:04:05", value)
	if err != nil {
		parsed, _ = time.Parse(time.RFC3339, value)
	}
	return parsed.UTC()
}
