package cloudflared

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/fonu/fonu/internal/config"
	"github.com/fonu/fonu/internal/fsutil"
	"github.com/fonu/fonu/internal/proxy"
	"github.com/fonu/fonu/internal/secret"
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
	return s.store.List(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (Tunnel, error) {
	return s.store.Get(ctx, id)
}

// Create 解析 token、生成 creds.json 与 config.yml，并写入数据库。
func (s *Service) Create(ctx context.Context, in CreateInput) (Tunnel, error) {
	if err := validateInput(in); err != nil {
		return Tunnel{}, err
	}
	accountTag, tunnelID, tunnelSecret, err := DecodeToken(in.Token)
	if err != nil {
		return Tunnel{}, err
	}
	dir := s.tunnelDir(in.Name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
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
	configContent := BuildConfig(tunnelID, credsPath, in.Network, ingress)
	if err := fsutil.WriteFileAtomic(credsPath, []byte(CredentialsJSON(accountTag, tunnelID, tunnelSecret)), 0o600); err != nil {
		return Tunnel{}, err
	}
	if err := fsutil.WriteFileAtomic(configPath, []byte(configContent), 0o640); err != nil {
		return Tunnel{}, err
	}
	encrypted, err := s.secretBox.Encrypt(in.Token)
	if err != nil {
		return Tunnel{}, err
	}
	return s.store.Create(ctx, in, tunnelID, accountTag, encrypted, "", configPath, credsPath, logPath)
}

// Update 更新隧道配置；token 留空时保留原凭据。
func (s *Service) Update(ctx context.Context, id int64, in CreateInput) (Tunnel, error) {
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return Tunnel{}, err
	}
	_ = s.manager.Stop(id)
	accountTag, tunnelID := current.AccountTag, current.TunnelID
	encrypted := current.CredentialsEnc
	if token := strings.TrimSpace(in.Token); token != "" {
		accountTag, tunnelID, tunnelSecret, err := DecodeToken(token)
		if err != nil {
			return Tunnel{}, err
		}
		encrypted, err = s.secretBox.Encrypt(token)
		if err != nil {
			return Tunnel{}, err
		}
		if err := fsutil.WriteFileAtomic(current.CredsPath, []byte(CredentialsJSON(accountTag, tunnelID, tunnelSecret)), 0o600); err != nil {
			return Tunnel{}, err
		}
	}
	origin := strings.TrimSpace(in.OriginService)
	if origin == "" {
		origin = defaultOriginService
	}
	configContent := BuildConfig(tunnelID, current.CredsPath, in.Network, BuildIngress(in.Hostnames, origin))
	if err := fsutil.WriteFileAtomic(current.ConfigPath, []byte(configContent), 0o640); err != nil {
		return Tunnel{}, err
	}
	return s.store.Update(ctx, id, in, tunnelID, accountTag, encrypted, current.CertPEMEnc,
		current.ConfigPath, current.CredsPath, current.LogPath)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	_ = s.manager.Stop(id)
	if dir := filepath.Dir(current.ConfigPath); dir != "" && strings.HasPrefix(dir, s.cfg.DataDir) {
		_ = os.RemoveAll(dir)
	}
	return s.store.Delete(ctx, id)
}

func (s *Service) Start(ctx context.Context, id int64) (Tunnel, error) {
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
	return s.store.Get(ctx, id)
}

func (s *Service) Stop(ctx context.Context, id int64) (Tunnel, error) {
	_ = s.manager.Stop(id)
	_ = s.store.SetRuntime(ctx, id, "stopped", "", 0)
	return s.store.Get(ctx, id)
}

func (s *Service) Restart(ctx context.Context, id int64) (Tunnel, error) {
	_, _ = s.Stop(ctx, id)
	return s.Start(ctx, id)
}

func (s *Service) RuntimeStatus(ctx context.Context, id int64) (RuntimeStatus, error) {
	if _, err := s.store.Get(ctx, id); err != nil {
		return RuntimeStatus{}, err
	}
	return s.manager.Status(id), nil
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
	case ModeTokenLocal, ModeTokenRemote:
	default:
		return fmt.Errorf("暂不支持该模式，当前仅支持 token-local / token-remote")
	}
	if strings.TrimSpace(in.Token) == "" {
		return fmt.Errorf("token 不能为空")
	}
	return nil
}
