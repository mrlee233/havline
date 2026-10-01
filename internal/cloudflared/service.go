package cloudflared

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/fsutil"
	"github.com/havline/havline/internal/proxy"
	"github.com/havline/havline/internal/secret"
)

const defaultOriginService = "http://127.0.0.1:80"

// RuleSource 提供本机反代规则，用于推导 ingress 候选域名。
type RuleSource interface {
	List(ctx context.Context) ([]proxy.Rule, error)
}

// Service 是 Cloudflare 隧道模块的门面。
type Service struct {
	cfg       config.Config
	store     *Store
	secretBox *secret.Box
	logger    *slog.Logger
	manager   *Manager
	rules     RuleSource
}

func NewService(cfg config.Config, store *Store, secretBox *secret.Box, logger *slog.Logger, rules RuleSource) *Service {
	moduleLogger := logger.With("module", "CLOUDFLARED")
	return &Service{
		cfg:       cfg,
		store:     store,
		secretBox: secretBox,
		logger:    moduleLogger,
		manager:   NewManager(cfg, moduleLogger),
		rules:     rules,
	}
}

func (s *Service) EnsureDirs() error {
	return os.MkdirAll(filepath.Join(s.cfg.DataDir, "cloudflared", "bin"), 0o755)
}

func (s *Service) List(ctx context.Context) ([]Tunnel, error) {
	items, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		s.attachConfig(&items[i])
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Tunnel, error) {
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return Tunnel{}, err
	}
	s.attachConfig(&item)
	return item, nil
}

// Create 解析 token、生成 creds.json 与 config.yml，并写入数据库。
func (s *Service) Create(ctx context.Context, in CreateInput) (Tunnel, error) {
	if strings.TrimSpace(in.Mode) == "" {
		in.Mode = ModeAccountLocal
	}
	if err := validateInput(in); err != nil {
		return Tunnel{}, err
	}
	api, err := s.cloudflareAPI(ctx)
	if err != nil {
		return Tunnel{}, err
	}
	in.Network = s.applyDefaultNetwork(ctx, in.Network)
	tunnelID, tunnelToken, err := api.CreateTunnel(ctx, in.Name)
	if err != nil {
		return Tunnel{}, err
	}
	accountTag, decodedID, tunnelSecret, err := DecodeToken(tunnelToken)
	if err != nil {
		_ = api.DeleteTunnel(ctx, tunnelID)
		return Tunnel{}, fmt.Errorf("Cloudflare 返回的隧道凭据无法解析: %w", err)
	}
	dir := s.tunnelDir(in.Name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		_ = api.DeleteTunnel(ctx, tunnelID)
		return Tunnel{}, err
	}
	configPath := filepath.Join(dir, "config.yml")
	credsPath := filepath.Join(dir, "creds.json")
	logPath := filepath.Join(dir, "cloudflared.log")
	origin := strings.TrimSpace(in.OriginService)
	if origin == "" {
		origin = defaultOriginService
	}
	ingress := BuildIngress(in.Hostnames, origin)
	if in.Managed {
		ingress = BuildIngress(nil, origin)
	}
	if err := fsutil.WriteFileAtomic(credsPath, []byte(CredentialsJSON(accountTag, decodedID, tunnelSecret)), 0o600); err != nil {
		_ = api.DeleteTunnel(ctx, tunnelID)
		return Tunnel{}, err
	}
	if err := fsutil.WriteFileAtomic(configPath, []byte(BuildConfig(decodedID, credsPath, in.Network, ingress)), 0o640); err != nil {
		_ = api.DeleteTunnel(ctx, tunnelID)
		return Tunnel{}, err
	}
	encrypted, err := s.secretBox.Encrypt(tunnelToken)
	if err != nil {
		_ = api.DeleteTunnel(ctx, tunnelID)
		return Tunnel{}, err
	}
	tunnel, err := s.store.Create(ctx, in, decodedID, accountTag, encrypted, "", configPath, credsPath, logPath)
	if err != nil {
		_ = api.DeleteTunnel(ctx, tunnelID)
		return Tunnel{}, err
	}
	if tunnel.Managed {
		if err := s.syncManagedTunnel(ctx, tunnel); err != nil {
			s.logger.Warn("创建托管隧道后同步规则失败", "tunnel_id", decodedID, "error", err.Error())
			tunnel.DNSWarning = "隧道已创建，但规则或 DNS 同步失败：" + err.Error()
		}
		return tunnel, nil
	}
	_ = api.PutIngress(ctx, decodedID, ingress)
	if err := s.syncTunnelDNS(ctx, api, tunnel.ID, decodedID, in.Hostnames); err != nil {
		s.logger.Warn("自动创建 Cloudflare DNS 失败", "tunnel_id", decodedID, "error", err.Error())
		tunnel.DNSWarning = "隧道已创建，但 DNS 记录未自动创建：" + err.Error()
	}
	return tunnel, nil
}

// Update 更新隧道配置；凭据由 Cloudflare API 创建时生成，编辑不修改。
func (s *Service) Update(ctx context.Context, id int64, in CreateInput) (Tunnel, error) {
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return Tunnel{}, err
	}
	if current.Managed && !in.Managed {
		if count := s.tunnelUsageCount(ctx, id); count > 0 {
			return Tunnel{}, fmt.Errorf("仍有 %d 条反向代理规则使用该托管隧道，请先移除 Cloudflare 出口", count)
		}
	}
	_ = s.manager.Stop(id)
	in.Network = s.applyDefaultNetwork(ctx, in.Network)
	origin := strings.TrimSpace(in.OriginService)
	if origin == "" {
		origin = defaultOriginService
	}
	ingress := BuildIngress(in.Hostnames, origin)
	if in.Managed {
		ingress = BuildIngress(nil, origin)
	}
	if err := fsutil.WriteFileAtomic(current.ConfigPath, []byte(BuildConfig(current.TunnelID, current.CredsPath, in.Network, ingress)), 0o640); err != nil {
		return Tunnel{}, err
	}
	updated, err := s.store.Update(ctx, id, in, current.TunnelID, current.AccountTag, current.CredentialsEnc, current.CertPEMEnc,
		current.ConfigPath, current.CredsPath, current.LogPath)
	if err != nil {
		return Tunnel{}, err
	}
	dnsWarning := ""
	if updated.Managed {
		if err := s.syncManagedTunnel(ctx, updated); err != nil {
			dnsWarning = "托管隧道已保存，但规则或 DNS 同步失败：" + err.Error()
		}
	} else if updated.Mode == ModeAccountLocal && updated.TunnelID != "" {
		if api, err := s.cloudflareAPI(ctx); err == nil {
			_ = api.PutIngress(ctx, updated.TunnelID, ingress)
			if err := s.syncTunnelDNS(ctx, api, updated.ID, updated.TunnelID, in.Hostnames); err != nil {
				s.logger.Warn("同步 Cloudflare DNS 失败", "tunnel_id", updated.TunnelID, "error", err.Error())
				dnsWarning = "DNS 记录同步失败：" + err.Error()
			}
		}
	}
	updated.DNSWarning = dnsWarning
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if count := s.tunnelUsageCount(ctx, id); count > 0 {
		return fmt.Errorf("仍有 %d 条反向代理规则使用该隧道，请先移除这些规则的 Cloudflare 出口", count)
	}
	_ = s.manager.Stop(id)
	if current.TunnelID != "" {
		if api, err := s.cloudflareAPI(ctx); err == nil {
			s.deleteTunnelDNS(ctx, api, current.ID)
			_ = api.DeleteTunnel(ctx, current.TunnelID)
		}
	}
	if dir := filepath.Dir(current.ConfigPath); dir != "" && strings.HasPrefix(dir, s.cfg.DataDir) {
		_ = os.RemoveAll(dir)
	}
	return s.store.Delete(ctx, id)
}

func (s *Service) tunnelUsageCount(ctx context.Context, id int64) int {
	if s.rules == nil {
		return 0
	}
	rules, err := s.rules.List(ctx)
	if err != nil {
		return 0
	}
	count := 0
	for _, rule := range rules {
		if rule.UsesExit(proxy.ExitCloudflare) && rule.CFTunnelID == id {
			count++
		}
	}
	return count
}

func (s *Service) Start(ctx context.Context, id int64) (Tunnel, error) {
	settings, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		return Tunnel{}, err
	}
	if !settings.Enabled {
		return Tunnel{}, fmt.Errorf("Cloudflare 隧道全局已停用，请先在页面顶部重新启用")
	}
	tunnel, err := s.store.Get(ctx, id)
	if err != nil {
		return Tunnel{}, err
	}
	binaryPath := s.binaryPath()
	if _, err := os.Stat(binaryPath); err != nil {
		return Tunnel{}, fmt.Errorf("cloudflared 未安装，请先下载二进制")
	}
	token := ""
	if tunnel.Mode == ModeTokenRemote {
		plain, err := s.secretBox.Decrypt(tunnel.CredentialsEnc)
		if err != nil {
			return Tunnel{}, fmt.Errorf("凭据解密失败: %w", err)
		}
		token = plain
	}
	if err := s.manager.Start(tunnel, binaryPath, tunnel.ConfigPath, token); err != nil {
		_ = s.store.SetRuntime(ctx, id, "failed", err.Error(), 0)
		return Tunnel{}, err
	}
	_ = s.store.SetRuntime(ctx, id, "starting", "", 0)
	return s.Get(ctx, id)
}

func (s *Service) Stop(ctx context.Context, id int64) (Tunnel, error) {
	_ = s.manager.Stop(id)
	_ = s.store.SetRuntime(ctx, id, "stopped", "", 0)
	return s.Get(ctx, id)
}

func (s *Service) Restart(ctx context.Context, id int64) (Tunnel, error) {
	_, _ = s.Stop(ctx, id)
	return s.Start(ctx, id)
}

// RefreshCredentials 重新从 Cloudflare 获取隧道凭据并更新本地 creds.json。
func (s *Service) RefreshCredentials(ctx context.Context, id int64) (Tunnel, error) {
	tunnel, err := s.store.Get(ctx, id)
	if err != nil {
		return Tunnel{}, err
	}
	api, err := s.cloudflareAPI(ctx)
	if err != nil {
		return Tunnel{}, err
	}
	token, err := api.TunnelToken(ctx, tunnel.TunnelID)
	if err != nil {
		return Tunnel{}, err
	}
	accountTag, decodedID, tunnelSecret, err := DecodeToken(token)
	if err != nil {
		return Tunnel{}, err
	}
	if err := fsutil.WriteFileAtomic(tunnel.CredsPath, []byte(CredentialsJSON(accountTag, decodedID, tunnelSecret)), 0o600); err != nil {
		return Tunnel{}, err
	}
	encrypted, err := s.secretBox.Encrypt(token)
	if err != nil {
		return Tunnel{}, err
	}
	if err := s.store.UpdateCredentials(ctx, id, encrypted); err != nil {
		return Tunnel{}, err
	}
	if s.manager.Status(id).Running {
		return s.Restart(ctx, id)
	}
	return s.Get(ctx, id)
}

func (s *Service) RuntimeStatus(ctx context.Context, id int64) (RuntimeStatus, error) {
	if _, err := s.store.Get(ctx, id); err != nil {
		return RuntimeStatus{}, err
	}
	return s.manager.Status(id), nil
}

// AppSettings 返回模块级配置；默认 token 只回显掩码。
func (s *Service) AppSettings(ctx context.Context) (AppSettings, error) {
	row, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		return AppSettings{}, err
	}
	out := AppSettings{AccountID: row.AccountID, Enabled: row.Enabled, Mirror: row.Mirror, Network: row.Network}
	if row.TokenEnc != "" {
		out.DefaultTokenConfigured = true
		if plain, err := s.secretBox.Decrypt(row.TokenEnc); err == nil {
			out.DefaultTokenMasked = MaskSecret(plain)
		}
	}
	if row.APITokenEnc != "" {
		out.APITokenConfigured = true
		if plain, err := s.secretBox.Decrypt(row.APITokenEnc); err == nil {
			out.APITokenMasked = MaskSecret(plain)
		}
	}
	return out, nil
}

// SaveAppSettings 保存默认 token、下载镜像与默认网络设置；Token 留空表示保留原值。
func (s *Service) SaveAppSettings(ctx context.Context, in AppSettingsInput) (AppSettings, error) {
	row, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		return AppSettings{}, err
	}
	accountID := strings.TrimSpace(in.AccountID)
	if accountID == "" {
		accountID = row.AccountID
	}
	enabled := row.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	tokenEnc := row.TokenEnc
	apiTokenEnc := row.APITokenEnc
	if apiToken := strings.TrimSpace(in.APIToken); apiToken != "" {
		apiTokenEnc, err = s.secretBox.Encrypt(apiToken)
		if err != nil {
			return AppSettings{}, err
		}
	}
	mirror := strings.TrimSpace(in.Mirror)
	if mirror == "" {
		mirror = row.Mirror
	}
	if mirror == "" {
		mirror = "official"
	}
	network := in.Network
	if strings.TrimSpace(network.TransportProtocol) == "" {
		network = row.Network
	}
	if err := s.store.SaveSettings(ctx, accountID, enabled, tokenEnc, apiTokenEnc, mirror, network); err != nil {
		return AppSettings{}, err
	}
	return s.AppSettings(ctx)
}

// SetEnabled 切换全局托管开关；关闭时停止所有隧道，开启时恢复 auto_start 的隧道。
func (s *Service) SetEnabled(ctx context.Context, enabled bool) (AppSettings, error) {
	row, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		return AppSettings{}, err
	}
	if err := s.store.SaveSettings(ctx, row.AccountID, enabled, row.TokenEnc, row.APITokenEnc, row.Mirror, row.Network); err != nil {
		return AppSettings{}, err
	}
	if enabled {
		s.StartAll(ctx)
	} else {
		s.StopAll(ctx)
	}
	return s.AppSettings(ctx)
}

// StopAll 停止所有隧道进程，但保留配置、凭据与云端隧道。
func (s *Service) StopAll(ctx context.Context) {
	tunnels, err := s.store.List(ctx)
	if err != nil {
		s.logger.Warn("读取 Cloudflare 隧道失败", "error", err.Error())
		return
	}
	for _, tunnel := range tunnels {
		if err := s.manager.Stop(tunnel.ID); err != nil {
			s.logger.Warn("停止 Cloudflare 隧道失败", "tunnel_id", tunnel.ID, "error", err.Error())
		}
		_ = s.store.SetRuntime(ctx, tunnel.ID, "stopped", "", 0)
	}
}

func (s *Service) applyDefaultNetwork(ctx context.Context, network NetworkSettings) NetworkSettings {
	if strings.TrimSpace(network.TransportProtocol) != "" {
		return network
	}
	row, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		return DefaultNetworkSettings()
	}
	return row.Network
}

func (s *Service) cloudflareAPI(ctx context.Context) (*CloudflareAPI, error) {
	row, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		return nil, err
	}
	accountID := strings.TrimSpace(row.AccountID)
	if accountID == "" {
		return nil, fmt.Errorf("请先在「应用配置」中填写 Account ID")
	}
	if row.APITokenEnc == "" {
		return nil, fmt.Errorf("请先在「应用配置」中保存 API Token")
	}
	token, err := s.secretBox.Decrypt(row.APITokenEnc)
	if err != nil {
		return nil, fmt.Errorf("API Token 解密失败: %w", err)
	}
	return NewCloudflareAPI(accountID, token), nil
}

// SyncRules 让所有托管隧道从反向代理规则重新派生 ingress 与 DNS。
func (s *Service) SyncRules(ctx context.Context) error {
	if s.rules == nil {
		return nil
	}
	rules, err := s.rules.List(ctx)
	if err != nil {
		return err
	}
	tunnels, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, tunnel := range tunnels {
		if !tunnel.Managed {
			continue
		}
		assigned := rulesForTunnel(rules, tunnel.ID)
		dnsRecords, err := s.store.ListDNSRecords(ctx, tunnel.ID)
		if err != nil && firstErr == nil {
			firstErr = err
			continue
		}
		if len(assigned) == 0 && len(dnsRecords) == 0 {
			continue
		}
		if err := s.syncManagedTunnel(ctx, tunnel); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func rulesForTunnel(rules []proxy.Rule, tunnelID int64) []proxy.Rule {
	out := make([]proxy.Rule, 0)
	for _, rule := range rules {
		if !rule.CloudflareEnabled() || rule.CFTunnelID != tunnelID {
			continue
		}
		out = append(out, rule)
	}
	return out
}

func (s *Service) syncManagedTunnel(ctx context.Context, tunnel Tunnel) error {
	if !tunnel.Managed {
		return nil
	}
	if s.rules == nil {
		return nil
	}
	rules, err := s.rules.List(ctx)
	if err != nil {
		return err
	}
	assigned := rulesForTunnel(rules, tunnel.ID)
	ingress := BuildIngressForRules(assigned)
	content := BuildConfig(tunnel.TunnelID, tunnel.CredsPath, tunnel.Network, ingress)
	if err := fsutil.WriteFileAtomic(tunnel.ConfigPath, []byte(content), 0o640); err != nil {
		return err
	}
	api, err := s.cloudflareAPI(ctx)
	if err != nil {
		return err
	}
	if err := api.PutIngress(ctx, tunnel.TunnelID, ingress); err != nil {
		return err
	}
	if err := s.syncTunnelDNS(ctx, api, tunnel.ID, tunnel.TunnelID, hostnamesFromIngress(ingress)); err != nil {
		return err
	}
	return s.restartIfRunning(ctx, tunnel.ID)
}

func hostnamesFromIngress(ingress []IngressRule) []string {
	out := make([]string, 0, len(ingress))
	for _, item := range ingress {
		if strings.TrimSpace(item.Hostname) != "" {
			out = append(out, item.Hostname)
		}
	}
	return out
}

func (s *Service) restartIfRunning(ctx context.Context, id int64) error {
	if !s.manager.Status(id).Running {
		return nil
	}
	if _, err := s.Restart(ctx, id); err != nil {
		return fmt.Errorf("配置已同步，但隧道重启失败：%w", err)
	}
	return nil
}

// syncTunnelDNS 让 Cloudflare DNS 与目标 hostname 保持一致，并只清理 Havline 自己接管的记录。
func (s *Service) syncTunnelDNS(ctx context.Context, api *CloudflareAPI, localTunnelID int64, remoteTunnelID string, hostnames []string) error {
	desired := make(map[string]struct{}, len(hostnames))
	for _, raw := range hostnames {
		hostname := strings.ToLower(strings.TrimSpace(raw))
		if hostname != "" {
			desired[hostname] = struct{}{}
		}
	}
	records, err := s.store.ListDNSRecords(ctx, localTunnelID)
	if err != nil {
		return err
	}
	var firstErr error
	for _, record := range records {
		if _, ok := desired[record.Hostname]; ok {
			continue
		}
		if err := api.DeleteDNSRecord(ctx, record.ZoneID, record.RecordID); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := s.store.DeleteDNSRecord(ctx, record.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	target := remoteTunnelID + ".cfargotunnel.com"
	for hostname := range desired {
		zoneID, _, err := api.FindZone(ctx, hostname)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		recordID, err := api.EnsureCNAME(ctx, zoneID, hostname, target)
		if err != nil {
			if transferredID, transferErr := s.transferTrackedDNS(ctx, api, localTunnelID, zoneID, hostname, target); transferErr == nil {
				recordID = transferredID
				err = nil
			}
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if strings.TrimSpace(recordID) == "" {
			if firstErr == nil {
				firstErr = fmt.Errorf("Cloudflare 未返回 %s 的 DNS 记录 ID", hostname)
			}
			continue
		}
		if err := s.store.UpsertDNSRecord(ctx, localTunnelID, zoneID, recordID, hostname); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Service) transferTrackedDNS(ctx context.Context, api *CloudflareAPI, localTunnelID int64, zoneID, hostname, target string) (string, error) {
	record, err := s.store.FindDNSRecord(ctx, hostname)
	if err != nil {
		return "", err
	}
	if record.TunnelID == localTunnelID {
		if err := api.DeleteDNSRecord(ctx, record.ZoneID, record.RecordID); err != nil {
			return "", err
		}
		if err := s.store.DeleteDNSRecord(ctx, record.ID); err != nil {
			return "", err
		}
		return api.EnsureCNAME(ctx, zoneID, hostname, target)
	}
	if err := api.DeleteDNSRecord(ctx, record.ZoneID, record.RecordID); err != nil {
		return "", err
	}
	if err := s.store.DeleteDNSRecord(ctx, record.ID); err != nil {
		return "", err
	}
	return api.EnsureCNAME(ctx, zoneID, hostname, target)
}

func (s *Service) deleteTunnelDNS(ctx context.Context, api *CloudflareAPI, localTunnelID int64) {
	records, err := s.store.ListDNSRecords(ctx, localTunnelID)
	if err != nil {
		s.logger.Warn("读取 Cloudflare DNS 记录失败", "tunnel_id", localTunnelID, "error", err.Error())
		return
	}
	for _, record := range records {
		if err := api.DeleteDNSRecord(ctx, record.ZoneID, record.RecordID); err != nil {
			s.logger.Warn("删除 Cloudflare DNS 记录失败", "hostname", record.Hostname, "error", err.Error())
			continue
		}
		_ = s.store.DeleteDNSRecord(ctx, record.ID)
	}
	_ = s.store.DeleteDNSRecordsForTunnel(ctx, localTunnelID)
}

// VerifyAPIToken 用 Account ID + API Token 调用 Cloudflare 账号级 tokens/verify 接口。
// accountID / input 为空时回退到应用配置中已保存的值。
func (s *Service) VerifyAPIToken(ctx context.Context, accountID, input string) (string, error) {
	row, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		return "", err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		accountID = strings.TrimSpace(row.AccountID)
	}
	if accountID == "" {
		return "", fmt.Errorf("请先填写 Account ID")
	}
	token := strings.TrimSpace(input)
	if token == "" {
		if row.APITokenEnc == "" {
			return "", fmt.Errorf("请填写 API Token（cfat_...），或先在应用配置中保存")
		}
		token, err = s.secretBox.Decrypt(row.APITokenEnc)
		if err != nil {
			return "", fmt.Errorf("API Token 解密失败: %w", err)
		}
	}
	return NewCloudflareAPI(accountID, token).VerifyToken(ctx)
}

// TestToken 验证 Account ID + API Token 是否可用。
func (s *Service) TestToken(ctx context.Context, accountID, input, _ string) (string, error) {
	return s.VerifyAPIToken(ctx, accountID, input)
}

// Zones 返回当前 API Token 可访问的 Cloudflare Zone。
func (s *Service) Zones(ctx context.Context) ([]Zone, error) {
	api, err := s.cloudflareAPI(ctx)
	if err != nil {
		return nil, err
	}
	return api.ListZones(ctx)
}

// AccessStatus 只读检测域名是否启用了 Cloudflare Access。
func (s *Service) AccessStatus(ctx context.Context, hostname string) (bool, string, error) {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" {
		return false, "", fmt.Errorf("域名为空")
	}
	api, err := s.cloudflareAPI(ctx)
	if err != nil {
		return false, "", err
	}
	return api.HasAccessForHostname(ctx, hostname)
}

// attachConfig 从 config.yml 回填域名与回源 service，供编辑弹窗使用。
func (s *Service) attachConfig(item *Tunnel) {
	data, err := os.ReadFile(item.ConfigPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		host, found := strings.CutPrefix(line, "- hostname:")
		if !found {
			continue
		}
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		item.Hostnames = append(item.Hostnames, host)
		if i+1 >= len(lines) {
			continue
		}
		if service, ok := strings.CutPrefix(strings.TrimSpace(lines[i+1]), "service:"); ok && item.OriginService == "" {
			item.OriginService = strings.TrimSpace(service)
		}
	}
}

func (s *Service) Logs(ctx context.Context, id int64, lines int) (string, error) {
	tunnel, err := s.store.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return s.manager.Logs(tunnel.LogPath, lines)
}

func (s *Service) PreviewConfig(ctx context.Context, id int64) (string, error) {
	tunnel, err := s.store.Get(ctx, id)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(tunnel.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// IngressCandidates 从本机反代规则推导可暴露的域名候选。
func (s *Service) IngressCandidates(ctx context.Context) ([]string, error) {
	if s.rules == nil {
		return []string{}, nil
	}
	rules, err := s.rules.List(ctx)
	if err != nil {
		return nil, err
	}
	hosts := []string{}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		for _, group := range rule.PortGroups() {
			hosts = append(hosts, group.Hostnames...)
		}
	}
	return hosts, nil
}

// StartAuto 在应用启动时拉起标记了 auto_start 的隧道。
func (s *Service) StartAuto(ctx context.Context) {
	settings, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		s.logger.Warn("读取 Cloudflare 配置失败", "error", err.Error())
		return
	}
	if !settings.Enabled {
		return
	}
	tunnels, err := s.store.List(ctx)
	if err != nil {
		s.logger.Warn("读取 Cloudflare 隧道失败", "error", err.Error())
		return
	}
	for _, tunnel := range tunnels {
		if !tunnel.AutoStart {
			continue
		}
		if _, err := s.Start(ctx, tunnel.ID); err != nil {
			s.logger.Warn("自动启动 Cloudflare 隧道失败", "tunnel_id", tunnel.ID, "error", err.Error())
		}
	}
}

// StartAll 启动全部隧道；用于全局总开关重新开启时立即恢复所有出口。
func (s *Service) StartAll(ctx context.Context) {
	settings, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		s.logger.Warn("读取 Cloudflare 配置失败", "error", err.Error())
		return
	}
	if !settings.Enabled {
		return
	}
	tunnels, err := s.store.List(ctx)
	if err != nil {
		s.logger.Warn("读取 Cloudflare 隧道失败", "error", err.Error())
		return
	}
	for _, tunnel := range tunnels {
		if _, err := s.Start(ctx, tunnel.ID); err != nil {
			s.logger.Warn("启动 Cloudflare 隧道失败", "tunnel_id", tunnel.ID, "error", err.Error())
		}
	}
}

func (s *Service) tunnelDir(name string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, strings.TrimSpace(name))
	return filepath.Join(s.cfg.DataDir, "cloudflared", "tunnels", safe)
}

func validateInput(in CreateInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("隧道名称不能为空")
	}
	switch in.Mode {
	case ModeAccountLocal, ModeTokenLocal, ModeTokenRemote:
	default:
		return fmt.Errorf("不支持的隧道模式: %s", in.Mode)
	}
	if !in.Managed && len(in.Hostnames) == 0 {
		return fmt.Errorf("手动隧道至少需要一个暴露域名")
	}
	if in.Managed && in.Mode != ModeAccountLocal {
		return fmt.Errorf("仅 API 管理隧道支持自动托管")
	}
	return nil
}
