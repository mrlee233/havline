package cloudflared

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

const tunnelSelect = `
SELECT id, name, mode, COALESCE(tunnel_id, ''), COALESCE(account_tag, ''),
       COALESCE(credentials_enc, ''), COALESCE(cert_pem_enc, ''),
       COALESCE(config_path, ''), COALESCE(creds_path, ''), COALESCE(log_path, ''),
       COALESCE(network_json, '{}'), COALESCE(status, 'stopped'), COALESCE(last_error, ''),
       COALESCE(metrics_port, 0), COALESCE(auto_start, 0), COALESCE(managed, 0), created_at, updated_at
FROM cf_tunnels`

func (s *Store) List(ctx context.Context) ([]Tunnel, error) {
	rows, err := s.db.QueryContext(ctx, tunnelSelect+` ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tunnel{}
	for rows.Next() {
		item, err := scanTunnel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (Tunnel, error) {
	item, err := scanTunnel(s.db.QueryRowContext(ctx, tunnelSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Tunnel{}, fmt.Errorf("Cloudflare 隧道不存在")
	}
	return item, err
}

func (s *Store) Create(ctx context.Context, in CreateInput, tunnelID, accountTag, credentialsEnc, certPEMEnc, configPath, credsPath, logPath string) (Tunnel, error) {
	network, _ := json.Marshal(in.Network)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO cf_tunnels(name, mode, tunnel_id, account_tag, credentials_enc, cert_pem_enc,
		    config_path, creds_path, log_path, network_json, status, auto_start, managed, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'stopped', ?, ?, datetime('now'))
	`, in.Name, in.Mode, tunnelID, accountTag, credentialsEnc, certPEMEnc,
		configPath, credsPath, logPath, string(network), boolInt(in.AutoStart), boolInt(in.Managed))
	if err != nil {
		return Tunnel{}, mapUniqueName(err)
	}
	id, _ := result.LastInsertId()
	return s.Get(ctx, id)
}

func (s *Store) Update(ctx context.Context, id int64, in CreateInput, tunnelID, accountTag, credentialsEnc, certPEMEnc, configPath, credsPath, logPath string) (Tunnel, error) {
	network, _ := json.Marshal(in.Network)
	_, err := s.db.ExecContext(ctx, `
		UPDATE cf_tunnels
		SET name = ?, mode = ?, tunnel_id = ?, account_tag = ?, credentials_enc = ?,
		    cert_pem_enc = ?, config_path = ?, creds_path = ?, log_path = ?,
		    network_json = ?, auto_start = ?, managed = ?, updated_at = datetime('now')
		WHERE id = ?
	`, in.Name, in.Mode, tunnelID, accountTag, credentialsEnc, certPEMEnc,
		configPath, credsPath, logPath, string(network), boolInt(in.AutoStart), boolInt(in.Managed), id)
	if err != nil {
		return Tunnel{}, mapUniqueName(err)
	}
	return s.Get(ctx, id)
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cf_tunnels WHERE id = ?`, id)
	return err
}

func (s *Store) SetRuntime(ctx context.Context, id int64, status, lastError string, metricsPort int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE cf_tunnels SET status = ?, last_error = ?, metrics_port = ?, updated_at = datetime('now')
		WHERE id = ?
	`, status, lastError, metricsPort, id)
	return err
}

func (s *Store) UpdateCredentials(ctx context.Context, id int64, credentialsEnc string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE cf_tunnels SET credentials_enc = ?, updated_at = datetime('now') WHERE id = ?
	`, credentialsEnc, id)
	return err
}

// settingsRow 是应用配置的原始行；TokenEnc 只在 Service 内部解密，不直接回给前端。
type settingsRow struct {
	AccountID   string
	Enabled     bool
	TokenEnc    string
	APITokenEnc string
	Mirror      string
	Network     NetworkSettings
}

// DNSRecord 是 Havline 自动创建或接管的 Cloudflare DNS 记录。
type DNSRecord struct {
	ID        int64  `json:"id"`
	TunnelID  int64  `json:"tunnel_id"`
	ZoneID    string `json:"zone_id"`
	RecordID  string `json:"record_id"`
	Hostname  string `json:"hostname"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Store) ListDNSRecords(ctx context.Context, tunnelID int64) ([]DNSRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tunnel_id, zone_id, record_id, hostname, created_at, updated_at
		FROM cf_dns_records WHERE tunnel_id = ? ORDER BY hostname ASC
	`, tunnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DNSRecord{}
	for rows.Next() {
		var item DNSRecord
		if err := rows.Scan(&item.ID, &item.TunnelID, &item.ZoneID, &item.RecordID, &item.Hostname, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) FindDNSRecord(ctx context.Context, hostname string) (DNSRecord, error) {
	var item DNSRecord
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tunnel_id, zone_id, record_id, hostname, created_at, updated_at
		FROM cf_dns_records WHERE hostname = ?
	`, strings.ToLower(strings.TrimSpace(hostname))).Scan(
		&item.ID, &item.TunnelID, &item.ZoneID, &item.RecordID, &item.Hostname, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DNSRecord{}, fmt.Errorf("Cloudflare DNS 记录不存在")
	}
	return item, err
}

func (s *Store) UpsertDNSRecord(ctx context.Context, tunnelID int64, zoneID, recordID, hostname string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cf_dns_records(tunnel_id, zone_id, record_id, hostname, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(hostname) DO UPDATE SET
		    tunnel_id = excluded.tunnel_id,
		    zone_id = excluded.zone_id,
		    record_id = excluded.record_id,
		    updated_at = datetime('now')
	`, tunnelID, zoneID, recordID, strings.ToLower(strings.TrimSpace(hostname)))
	return err
}

func (s *Store) DeleteDNSRecord(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cf_dns_records WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteDNSRecordsForTunnel(ctx context.Context, tunnelID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cf_dns_records WHERE tunnel_id = ?`, tunnelID)
	return err
}

func (s *Store) GetSettingsRaw(ctx context.Context) (settingsRow, error) {
	var accountID, tokenEnc, apiTokenEnc, mirror, networkJSON string
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(account_id, ''), COALESCE(enabled, 1), COALESCE(default_token_enc, ''), COALESCE(api_token_enc, ''),
		       COALESCE(mirror, 'official'), COALESCE(network_json, '{}')
		FROM cf_settings WHERE id = 1
	`).Scan(&accountID, &enabled, &tokenEnc, &apiTokenEnc, &mirror, &networkJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return settingsRow{Enabled: true, Mirror: "official", Network: DefaultNetworkSettings()}, nil
	}
	if err != nil {
		return settingsRow{}, err
	}
	row := settingsRow{AccountID: accountID, Enabled: enabled == 1, TokenEnc: tokenEnc, APITokenEnc: apiTokenEnc, Mirror: mirror, Network: DefaultNetworkSettings()}
	_ = json.Unmarshal([]byte(networkJSON), &row.Network)
	return row, nil
}

func (s *Store) SaveSettings(ctx context.Context, accountID string, enabled bool, tokenEnc, apiTokenEnc, mirror string, network NetworkSettings) error {
	networkJSON, _ := json.Marshal(network)
	_, err := s.db.ExecContext(ctx, `
		UPDATE cf_settings
		SET account_id = ?, enabled = ?, default_token_enc = ?, api_token_enc = ?, mirror = ?, network_json = ?, updated_at = datetime('now')
		WHERE id = 1
	`, accountID, boolInt(enabled), tokenEnc, apiTokenEnc, mirror, string(networkJSON))
	return err
}

func scanTunnel(row interface{ Scan(dest ...any) error }) (Tunnel, error) {
	var item Tunnel
	var networkJSON, created, updated string
	var autoStart, managed int
	err := row.Scan(&item.ID, &item.Name, &item.Mode, &item.TunnelID, &item.AccountTag,
		&item.CredentialsEnc, &item.CertPEMEnc, &item.ConfigPath, &item.CredsPath, &item.LogPath,
		&networkJSON, &item.Status, &item.LastError, &item.MetricsPort, &autoStart, &managed, &created, &updated)
	if err != nil {
		return Tunnel{}, err
	}
	item.AutoStart = autoStart == 1
	item.Managed = managed == 1
	item.Network = DefaultNetworkSettings()
	_ = json.Unmarshal([]byte(networkJSON), &item.Network)
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func parseTime(raw string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func mapUniqueName(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return fmt.Errorf("Cloudflare 隧道名称已存在")
	}
	return err
}
