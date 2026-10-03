package backup

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/havline/havline/internal/notify"
	"github.com/havline/havline/internal/settings"
)

// AutoInterval 自动备份间隔。scheduler 启动时会先跑一轮，靠「最新备份是否还在间隔内」
// 判断该不该备份，因此频繁重启（NAS 上很常见）不会重复备份。
const AutoInterval = 24 * time.Hour

// AutoRunner 定期导出一份归档、回读校验、再按保留份数剪枝。
type AutoRunner struct {
	svc      *Service
	settings *settings.Store
	notify   *notify.Service
	logger   *slog.Logger
	now      func() time.Time
}

func NewAutoRunner(svc *Service, settingsStore *settings.Store, notifySvc *notify.Service, logger *slog.Logger) *AutoRunner {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoRunner{svc: svc, settings: settingsStore, notify: notifySvc, logger: logger, now: time.Now}
}

// Run 跑一轮。失败只通知、不返回错误：scheduler 的任务没有重试机制，通知才是可见性来源。
func (r *AutoRunner) Run(ctx context.Context) {
	if r == nil || r.svc == nil || r.settings == nil {
		return
	}
	enabled, err := r.settings.GetBool(ctx, settings.KeyBackupAutoEnabled)
	if err != nil || !enabled {
		return
	}

	archives, err := r.svc.List()
	if err != nil {
		r.logger.Warn("读取备份列表失败", "module", "BACKUP", "error", err.Error())
		return
	}
	if len(archives) > 0 && !dueForBackup(archives[0].ModTime, r.now(), AutoInterval) {
		return
	}

	path, err := r.svc.Export(ctx)
	if err != nil {
		r.fail(ctx, fmt.Sprintf("导出备份失败：%s", err.Error()))
		return
	}
	entries, err := r.svc.Verify(ctx, path)
	if err != nil {
		// 校验不过的归档不算一份可用备份：删掉并如实告警，别让用户以为已经有备份
		if removeErr := r.svc.Remove(filepath.Base(path)); removeErr != nil {
			r.logger.Warn("删除校验失败的备份失败", "module", "BACKUP", "error", removeErr.Error())
		}
		r.fail(ctx, fmt.Sprintf("备份校验失败：%s", err.Error()))
		return
	}

	removed, err := r.svc.Prune(KeepCount(ctx, r.settings))
	if err != nil {
		r.logger.Warn("剪枝旧备份失败", "module", "BACKUP", "error", err.Error())
	}
	r.logger.Info("自动备份完成", "module", "BACKUP",
		"file", filepath.Base(path), "entries", entries, "removed", removed)
}

// dueForBackup 判断该不该做一次备份：没有备份，或最新一份已超过间隔。
func dueForBackup(newest, now time.Time, interval time.Duration) bool {
	if newest.IsZero() {
		return true
	}
	return now.Sub(newest) >= interval
}

func (r *AutoRunner) fail(ctx context.Context, message string) {
	r.logger.Error("自动备份失败", "module", "BACKUP", "error", message)
	if r.notify != nil {
		r.notify.Alert(ctx, notify.EventBackupFailure, "自动备份失败", message)
	}
}
