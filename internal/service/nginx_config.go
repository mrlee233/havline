package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/havline/havline/internal/nginx"
	"github.com/havline/havline/internal/proxy"
	"github.com/havline/havline/internal/settings"
)

type NginxBackupInfo struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type RuleNginxView struct {
	Mode      string            `json:"mode"`
	Enabled   bool              `json:"enabled"`
	Active    bool              `json:"active"`
	Generated string            `json:"generated"`
	Content   string            `json:"content"`
	Backups   []NginxBackupInfo `json:"backups"`
}

type GlobalNginxView struct {
	Mode               string            `json:"mode"`
	GeneratedFramework string            `json:"generated_framework"`
	GeneratedSnippet   string            `json:"generated_snippet"`
	Content            string            `json:"content"`
	Backups            []NginxBackupInfo `json:"backups"`
}

type SaveRuleNginxInput struct {
	Mode    string
	Content *string
}

type SaveGlobalNginxInput struct {
	Mode    string
	Content *string
}

func (s *ProxyService) GetRuleNginx(ctx context.Context, id int64) (RuleNginxView, error) {
	rule, err := s.Get(ctx, id)
	if err != nil {
		return RuleNginxView{}, err
	}
	certs, err := s.loadCertSources(ctx)
	if err != nil {
		return RuleNginxView{}, err
	}
	opts := s.loadGenerateOptions(ctx)
	generated, err := nginx.GenerateRuleBlocks(s.cfg, rule, certs, opts)
	if err != nil {
		return RuleNginxView{}, err
	}
	mode := rule.NginxMode
	if mode == "" {
		mode = "auto"
	}
	content := generated
	if mode == "custom" {
		custom, err := nginx.ReadRuleCustom(s.cfg, id)
		if err != nil {
			return RuleNginxView{}, err
		}
		if strings.TrimSpace(custom) != "" {
			content = custom
		}
	}
	backups, err := nginx.ListBackups(s.cfg.NginxRuleBackupsDir(id))
	if err != nil {
		return RuleNginxView{}, err
	}
	return RuleNginxView{
		Mode:      mode,
		Enabled:   rule.Enabled,
		Active:    rule.Enabled,
		Generated: generated,
		Content:   content,
		Backups:   mapBackups(backups),
	}, nil
}

func (s *ProxyService) SaveRuleNginx(ctx context.Context, id int64, in SaveRuleNginxInput) (RuleNginxView, error) {
	rule, err := s.Get(ctx, id)
	if err != nil {
		return RuleNginxView{}, err
	}
	mode := strings.TrimSpace(in.Mode)
	if mode == "" {
		mode = rule.NginxMode
	}
	if mode != "auto" && mode != "custom" {
		return RuleNginxView{}, fmt.Errorf("无效的 nginx 模式")
	}

	if mode == "auto" {
		if err := s.store.SetNginxMode(ctx, id, "auto"); err != nil {
			return RuleNginxView{}, err
		}
		// 切回自动模式后，手动配置的占用记录不再适用
		if err := s.store.ClearCustomEndpoints(ctx, id); err != nil {
			return RuleNginxView{}, err
		}
		if err := s.applyNginx(ctx); err != nil {
			return RuleNginxView{}, err
		}
		return s.GetRuleNginx(ctx, id)
	}

	if in.Content == nil {
		return RuleNginxView{}, fmt.Errorf("手动模式需要提供配置内容")
	}
	content := *in.Content
	if strings.TrimSpace(content) == "" {
		return RuleNginxView{}, fmt.Errorf("Nginx 配置不能为空")
	}

	if err := s.validateRuleNginxPending(ctx, rule, content); err != nil {
		return RuleNginxView{}, err
	}
	// 手动配置里的 listen/server_name 不进 proxy_hosts，需单独解析记录并校验占用，
	// 否则它与其它规则的同域名同端口冲突只会在 Nginx reload 时静默失效
	endpoints := nginx.ParseServerEndpoints(content)
	if err := s.store.EnsureEndpointsAvailable(ctx, endpoints, id); err != nil {
		return RuleNginxView{}, err
	}
	if _, err := nginx.SaveRuleCustomWithBackup(s.cfg, id, content); err != nil {
		return RuleNginxView{}, err
	}
	if err := s.store.ReplaceCustomEndpoints(ctx, id, endpoints); err != nil {
		return RuleNginxView{}, err
	}
	if err := s.store.SetNginxMode(ctx, id, "custom"); err != nil {
		return RuleNginxView{}, err
	}
	if err := s.applyNginx(ctx); err != nil {
		return RuleNginxView{}, err
	}
	return s.GetRuleNginx(ctx, id)
}

func (s *ProxyService) RollbackRuleNginx(ctx context.Context, id int64, backupName string) (RuleNginxView, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return RuleNginxView{}, err
	}
	backups, err := nginx.ListBackups(s.cfg.NginxRuleBackupsDir(id))
	if err != nil {
		return RuleNginxView{}, err
	}
	if len(backups) == 0 {
		return RuleNginxView{}, fmt.Errorf("没有可回滚的备份")
	}
	if backupName == "" {
		backupName = backups[0].Name
	}
	if err := nginx.RestoreBackup(s.cfg.NginxRuleBackupsDir(id), backupName, s.cfg.NginxRuleCustomPath(id)); err != nil {
		return RuleNginxView{}, err
	}
	custom, err := nginx.ReadRuleCustom(s.cfg, id)
	if err != nil {
		return RuleNginxView{}, err
	}
	rule, err := s.Get(ctx, id)
	if err != nil {
		return RuleNginxView{}, err
	}
	if err := s.validateRuleNginxPending(ctx, rule, custom); err != nil {
		return RuleNginxView{}, err
	}
	endpoints := nginx.ParseServerEndpoints(custom)
	if err := s.store.EnsureEndpointsAvailable(ctx, endpoints, id); err != nil {
		// 备份文件已写回（与原有行为一致），占用记录需跟磁盘内容保持一致，避免留下过期占用
		_ = s.store.ReplaceCustomEndpoints(ctx, id, endpoints)
		return RuleNginxView{}, err
	}
	if err := s.store.ReplaceCustomEndpoints(ctx, id, endpoints); err != nil {
		return RuleNginxView{}, err
	}
	if err := s.store.SetNginxMode(ctx, id, "custom"); err != nil {
		return RuleNginxView{}, err
	}
	if err := s.applyNginx(ctx); err != nil {
		return RuleNginxView{}, err
	}
	return s.GetRuleNginx(ctx, id)
}

func (s *ProxyService) GetGlobalNginx(ctx context.Context) (GlobalNginxView, error) {
	mode, err := s.settings.Get(ctx, settings.KeyNginxGlobalMode)
	if err != nil {
		return GlobalNginxView{}, err
	}
	if mode == "" {
		mode = "auto"
	}
	content, err := nginx.ReadGlobalCustom(s.cfg)
	if err != nil {
		return GlobalNginxView{}, err
	}
	backups, err := nginx.ListBackups(s.cfg.NginxGlobalBackupsDir())
	if err != nil {
		return GlobalNginxView{}, err
	}
	return GlobalNginxView{
		Mode:               mode,
		GeneratedFramework: nginx.GenerateGlobalFramework(s.cfg),
		GeneratedSnippet:   nginx.DefaultGlobalHTTPSnippet(),
		Content:            content,
		Backups:            mapBackups(backups),
	}, nil
}

func (s *ProxyService) SaveGlobalNginx(ctx context.Context, in SaveGlobalNginxInput) (GlobalNginxView, error) {
	mode := strings.TrimSpace(in.Mode)
	if mode == "" {
		var err error
		mode, err = s.settings.Get(ctx, settings.KeyNginxGlobalMode)
		if err != nil {
			return GlobalNginxView{}, err
		}
	}
	if mode != "auto" && mode != "custom" {
		return GlobalNginxView{}, fmt.Errorf("无效的 nginx 模式")
	}

	if mode == "auto" {
		if err := s.settings.Set(ctx, settings.KeyNginxGlobalMode, "auto"); err != nil {
			return GlobalNginxView{}, err
		}
		if err := s.applyNginx(ctx); err != nil {
			return GlobalNginxView{}, err
		}
		return s.GetGlobalNginx(ctx)
	}

	if in.Content == nil {
		return GlobalNginxView{}, fmt.Errorf("手动模式需要提供配置内容")
	}
	content := *in.Content
	if err := s.validateGlobalNginxPending(ctx, content); err != nil {
		return GlobalNginxView{}, err
	}
	if _, err := nginx.SaveGlobalCustomWithBackup(s.cfg, content); err != nil {
		return GlobalNginxView{}, err
	}
	if err := s.settings.Set(ctx, settings.KeyNginxGlobalMode, "custom"); err != nil {
		return GlobalNginxView{}, err
	}
	if err := s.applyNginx(ctx); err != nil {
		return GlobalNginxView{}, err
	}
	return s.GetGlobalNginx(ctx)
}

func (s *ProxyService) RollbackGlobalNginx(ctx context.Context, backupName string) (GlobalNginxView, error) {
	backups, err := nginx.ListBackups(s.cfg.NginxGlobalBackupsDir())
	if err != nil {
		return GlobalNginxView{}, err
	}
	if len(backups) == 0 {
		return GlobalNginxView{}, fmt.Errorf("没有可回滚的备份")
	}
	if backupName == "" {
		backupName = backups[0].Name
	}
	if err := nginx.RestoreBackup(s.cfg.NginxGlobalBackupsDir(), backupName, s.cfg.NginxGlobalCustomPath()); err != nil {
		return GlobalNginxView{}, err
	}
	content, err := nginx.ReadGlobalCustom(s.cfg)
	if err != nil {
		return GlobalNginxView{}, err
	}
	if err := s.validateGlobalNginxPending(ctx, content); err != nil {
		return GlobalNginxView{}, err
	}
	if err := s.settings.Set(ctx, settings.KeyNginxGlobalMode, "custom"); err != nil {
		return GlobalNginxView{}, err
	}
	if err := s.applyNginx(ctx); err != nil {
		return GlobalNginxView{}, err
	}
	return s.GetGlobalNginx(ctx)
}

func (s *ProxyService) validateRuleNginxPending(ctx context.Context, rule proxy.Rule, content string) error {
	rules, err := s.store.ListEnabledLocal(ctx)
	if err != nil {
		return err
	}
	merged := make([]proxy.Rule, 0, len(rules))
	found := false
	for _, r := range rules {
		if r.ID == rule.ID {
			ruleCopy := rule
			ruleCopy.NginxMode = "custom"
			ruleCopy.Enabled = true
			merged = append(merged, ruleCopy)
			found = true
			continue
		}
		merged = append(merged, r)
	}
	if !found && rule.Enabled {
		ruleCopy := rule
		ruleCopy.NginxMode = "custom"
		merged = append(merged, ruleCopy)
	}
	return s.validateWithOverrides(ctx, merged, nginx.GenerateOptions{
		RuleCustomOverrides: map[int64]string{rule.ID: content},
	})
}

func (s *ProxyService) validateGlobalNginxPending(ctx context.Context, content string) error {
	rules, err := s.store.ListEnabledLocal(ctx)
	if err != nil {
		return err
	}
	return s.validateWithOverrides(ctx, rules, nginx.GenerateOptions{
		GlobalCustomOverride: content,
	})
}

func (s *ProxyService) validateWithOverrides(ctx context.Context, rules []proxy.Rule, extra nginx.GenerateOptions) error {
	certs, err := s.loadCertSources(ctx)
	if err != nil {
		return err
	}
	opts := s.loadGenerateOptions(ctx)
	opts.GlobalCustomOverride = extra.GlobalCustomOverride
	opts.GlobalCustomPathOverride = extra.GlobalCustomPathOverride
	opts.RuleCustomOverrides = extra.RuleCustomOverrides
	opts.ChinaCIDRAvailable = nginx.ChinaCIDRExists(s.cfg)
	content, err := nginx.Generate(s.cfg, rules, certs, opts)
	if err != nil {
		return err
	}
	return s.nginx.ValidateContent(ctx, content)
}

func mapBackups(entries []nginx.BackupEntry) []NginxBackupInfo {
	out := make([]NginxBackupInfo, 0, len(entries))
	for _, entry := range entries {
		out = append(out, NginxBackupInfo{
			Name:      entry.Name,
			CreatedAt: entry.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	return out
}

// NginxConfigVersionInfo 描述主配置（nginx.conf）的一个历史版本。
type NginxConfigVersionInfo struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

type NginxConfigVersionsView struct {
	Message  string                   `json:"message,omitempty"`
	Content  string                   `json:"content"`
	Versions []NginxConfigVersionInfo `json:"versions"`
}

type NginxConfigVersionView struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// GetNginxConfigVersions 返回当前主配置与其历史版本，前端拿两者做差异对比。
func (s *ProxyService) GetNginxConfigVersions() (NginxConfigVersionsView, error) {
	content, err := s.nginx.ReadConfig()
	if err != nil {
		return NginxConfigVersionsView{}, err
	}
	versions, err := s.nginx.ListConfigVersions()
	if err != nil {
		return NginxConfigVersionsView{}, err
	}
	return NginxConfigVersionsView{Content: content, Versions: mapConfigVersions(versions)}, nil
}

func (s *ProxyService) ReadNginxConfigVersion(name string) (NginxConfigVersionView, error) {
	content, err := s.nginx.ReadConfigVersion(name)
	if err != nil {
		return NginxConfigVersionView{}, err
	}
	return NginxConfigVersionView{Name: name, Content: content}, nil
}

// RollbackNginxConfigVersion 回滚主配置；校验不过时 nginx 层不会动磁盘上的当前配置。
func (s *ProxyService) RollbackNginxConfigVersion(ctx context.Context, name string) (NginxConfigVersionsView, error) {
	result, err := s.nginx.RollbackConfigVersion(ctx, name)
	if err != nil {
		return NginxConfigVersionsView{}, err
	}
	view, err := s.GetNginxConfigVersions()
	if err != nil {
		return NginxConfigVersionsView{}, err
	}
	view.Message = result.Message
	return view, nil
}

func mapConfigVersions(entries []nginx.ConfigVersion) []NginxConfigVersionInfo {
	out := make([]NginxConfigVersionInfo, 0, len(entries))
	for _, entry := range entries {
		out = append(out, NginxConfigVersionInfo{
			Name:      entry.Name,
			Size:      entry.Size,
			CreatedAt: entry.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	return out
}
