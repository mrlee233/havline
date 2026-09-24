// Package monitor 周期巡检「资源占用」与「隧道连通性」这两类必须持续观察才会出现的问题。
// 事件型通知（DDNS 失败、证书到期）由各自模块在发生时直接发出，不需要巡检。
//
// 判定在这里，通知只走 notify.Service：开关、阈值、冷却都读设置项，
// 所以本包不需要知道通知通道的任何细节。
package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/fonu/fonu/internal/notify"
	"github.com/fonu/fonu/internal/settings"
	"github.com/fonu/fonu/internal/sysinfo"
)

// RouteStatus 是巡检关心的隧道状态；由调用方注入（避免 monitor 依赖 frp 包）。
type RouteStatus struct {
	Domain   string
	TunnelOK bool
	Detail   string
}

// Checker 每轮都重新读设置项：改阈值或开关不需要重启。
type Checker struct {
	settings *settings.Store
	notify   *notify.Service
	logger   *slog.Logger
	routes   func(ctx context.Context) ([]RouteStatus, error)

	mu             sync.Mutex
	cpuPrev        sysinfo.CPUSample
	resourceAlerts map[string]time.Time
	tunnelFailures map[string]int
}

func New(settingsStore *settings.Store, notifySvc *notify.Service, logger *slog.Logger, routes func(context.Context) ([]RouteStatus, error)) *Checker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Checker{
		settings:       settingsStore,
		notify:         notifySvc,
		logger:         logger,
		routes:         routes,
		resourceAlerts: map[string]time.Time{},
		tunnelFailures: map[string]int{},
	}
}

// Run 跑一轮巡检。scheduler 已对任务包了 recover，这里不重复兜底。
func (c *Checker) Run(ctx context.Context) {
	if c == nil || c.settings == nil {
		return
	}
	c.checkResources(ctx)
	c.checkTunnels(ctx)
}

func (c *Checker) checkResources(ctx context.Context) {
	// 开关关闭时连采集都省掉（关掉的功能不该有后台开销）
	if enabled, _ := c.settings.GetBool(ctx, settings.KeyNotifyOnResourceThreshold); !enabled {
		return
	}
	cpuLimit, _ := c.settings.GetInt(ctx, settings.KeyNotifyCPUThreshold)
	memoryLimit, _ := c.settings.GetInt(ctx, settings.KeyNotifyMemoryThreshold)
	diskLimit, _ := c.settings.GetInt(ctx, settings.KeyNotifyDiskThreshold)

	c.mu.Lock()
	stats, sample := sysinfo.Read(c.cpuPrev)
	c.cpuPrev = sample
	c.mu.Unlock()

	limits := []struct {
		metric string
		label  string
		limit  int
		value  float64
	}{
		{"cpu", "CPU", cpuLimit, stats.CPUPercent},
		{"memory", "内存", memoryLimit, percent(stats.MemUsedBytes, stats.MemTotalBytes)},
		{"disk", "磁盘", diskLimit, percent(stats.DiskUsedBytes, stats.DiskTotalBytes)},
	}
	now := time.Now()
	for _, item := range limits {
		// 阈值 <= 0 表示这一项不告警（CPU 首次采样时占用率为 0，也不会误报）
		if item.limit <= 0 {
			continue
		}
		if item.value < float64(item.limit) {
			// 回落到阈值以下就清冷却：再次超标可以立刻提醒
			c.mu.Lock()
			delete(c.resourceAlerts, item.metric)
			c.mu.Unlock()
			continue
		}
		if !c.allowResourceAlert(item.metric, now) {
			continue
		}
		c.notify.Alert(ctx, notify.EventResourceThreshold, "资源占用过高",
			fmt.Sprintf("%s 使用率 %.1f%%，已超过阈值 %d%%", item.label, item.value, item.limit))
	}
}

// allowResourceAlert 按静默期限流：持续高占用每 DefaultResourceAlertCooldown 提醒一次。
func (c *Checker) allowResourceAlert(metric string, now time.Time) bool {
	cooldown := time.Duration(notify.DefaultResourceAlertCooldown) * time.Second
	c.mu.Lock()
	defer c.mu.Unlock()
	if last, ok := c.resourceAlerts[metric]; ok && now.Sub(last) < cooldown {
		return false
	}
	c.resourceAlerts[metric] = now
	return true
}

func (c *Checker) checkTunnels(ctx context.Context) {
	if c.routes == nil {
		return
	}
	if enabled, _ := c.settings.GetBool(ctx, settings.KeyNotifyOnTunnelDown); !enabled {
		return
	}
	limit, _ := c.settings.GetInt(ctx, settings.KeyNotifyTunnelFailThreshold)
	if limit <= 0 {
		return
	}

	routes, err := c.routes(ctx)
	if err != nil {
		c.logger.Warn("巡检隧道状态失败", "module", "MONITOR", "error", err.Error())
		return
	}
	for _, route := range routes {
		if strings.TrimSpace(route.Domain) == "" {
			continue
		}
		c.mu.Lock()
		failures, event := tunnelTransition(c.tunnelFailures[route.Domain], route.TunnelOK, limit)
		c.tunnelFailures[route.Domain] = failures
		c.mu.Unlock()

		switch event {
		case tunnelEventDown:
			c.notify.Alert(ctx, notify.EventTunnelDown, "隧道连续不通",
				fmt.Sprintf("%s 连续 %d 次巡检未在 frps 上注册：%s", route.Domain, limit, detailOr(route.Detail)))
		case tunnelEventRecovered:
			c.notify.Alert(ctx, notify.EventTunnelDown, "隧道已恢复",
				fmt.Sprintf("%s 已重新在 frps 上注册", route.Domain))
		}
	}
}

type tunnelEvent string

const (
	tunnelEventNone      tunnelEvent = ""
	tunnelEventDown      tunnelEvent = "down"
	tunnelEventRecovered tunnelEvent = "recovered"
)

// tunnelTransition 推进某个域名的连续失败计数，返回新计数与本轮要发的事件。
// 只在「刚好达到阈值」与「从失败中恢复」两个时刻发，避免每轮巡检都刷通知。
func tunnelTransition(previous int, tunnelOK bool, limit int) (int, tunnelEvent) {
	if tunnelOK {
		if previous >= limit {
			return 0, tunnelEventRecovered
		}
		return 0, tunnelEventNone
	}
	next := previous + 1
	if next == limit {
		return next, tunnelEventDown
	}
	return next, tunnelEventNone
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

func detailOr(detail string) string {
	if strings.TrimSpace(detail) == "" {
		return "未返回具体原因"
	}
	return detail
}
