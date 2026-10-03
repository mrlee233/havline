package nginx

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/havline/havline/internal/fsutil"
)

// maxConfigVersions 主配置保留的版本份数。Apply 每次保存都会留一份，fsutil 会跳过内容相同的版本，
// 因此这里对应的是最近 10 次「配置真的变了」的改动。
const maxConfigVersions = 10

// ConfigVersion 是主配置（nginx.conf）的一个历史版本。
type ConfigVersion struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// ReadConfig 读取当前主配置内容，供前端与历史版本做差异对比。
func (m *Manager) ReadConfig() (string, error) {
	data, err := os.ReadFile(m.cfg.NginxConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// ListConfigVersions 列出主配置的历史版本（新的在前）；从未写过配置时返回空列表。
func (m *Manager) ListConfigVersions() ([]ConfigVersion, error) {
	versions, err := fsutil.ListVersions(m.cfg.NginxConfigPath())
	if err != nil {
		return nil, err
	}
	out := make([]ConfigVersion, 0, len(versions))
	for _, version := range versions {
		out = append(out, ConfigVersion{Name: version.Name, Size: version.Size, CreatedAt: version.CreatedAt})
	}
	return out, nil
}

// ReadConfigVersion 读取某个历史版本的内容。
func (m *Manager) ReadConfigVersion(name string) (string, error) {
	data, err := fsutil.ReadVersion(m.cfg.NginxConfigPath(), name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RollbackConfigVersion 把主配置回退到指定版本。
//
// 与 Apply 同构：先语法校验、再 nginx -t 校验，两关都过才写回并重载；校验失败保持当前配置不动。
// 写回前把当前内容也留成一个版本，所以回退本身同样可以再回退。
func (m *Manager) RollbackConfigVersion(ctx context.Context, name string) (ApplyResult, error) {
	if name == "" {
		return ApplyResult{}, fmt.Errorf("请选择要回滚的版本")
	}
	content, err := fsutil.ReadVersion(m.cfg.NginxConfigPath(), name)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := m.EnsureDirs(); err != nil {
		return ApplyResult{}, err
	}
	if err := ValidateConfigSyntax(string(content)); err != nil {
		return ApplyResult{}, fmt.Errorf("该版本无法通过语法校验，已保持当前配置：%s", err)
	}
	if m.available() {
		tmpPath := m.cfg.NginxConfigPath() + ".rollback.tmp"
		if err := fsutil.WriteFileAtomic(tmpPath, content, 0o644); err != nil {
			return ApplyResult{}, err
		}
		validateErr := m.validate(ctx, tmpPath)
		_ = os.Remove(tmpPath)
		if validateErr != nil {
			return ApplyResult{}, fmt.Errorf("该版本无法通过 nginx 校验，已保持当前配置：%s", validateErr)
		}
	}

	currentPath := m.cfg.NginxConfigPath()
	if err := fsutil.Backup(currentPath); err != nil {
		return ApplyResult{}, err
	}
	if _, err := fsutil.BackupVersioned(currentPath, maxConfigVersions); err != nil {
		return ApplyResult{}, err
	}
	if err := fsutil.WriteFileAtomic(currentPath, content, 0o644); err != nil {
		return ApplyResult{}, err
	}

	result, err := m.activate(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	result.Message = "已回滚到所选版本，" + result.Message
	return result, nil
}
