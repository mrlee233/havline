// Package monitor 周期巡检几类必须持续观察才会出现的问题：主机资源占用、隧道连通性、
// 公网服务端掉线、agent 不可达、配置漂移。
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

// RouteStatus 是巡检关心的隧道状态。
type RouteStatus struct {
	Domain   string
	TunnelOK bool
	Detail   string
}

// ServerStatus 是巡检关心的公网服务端状态：只有 DesiredState 表示「用户要它运行」时才判定掉线。
type ServerStatus struct {
	ID              int64
	Name            string
	DesiredState    string
	ProcessState    string
	ConnectionState string
	Detail          string
}

// AgentStatus 是某台服务端上 fonu-agent 的可达性；没配 agent 的服务端不会出现在这里。
type AgentStatus struct {
	ID        int64
	Name      string
	Reachable bool
	Detail    string
}

// DriftStatus 是某台服务端的部署漂移摘要，Count 为 0 表示没有漂移。
type DriftStatus struct {
	ID    int64
	Name  string
	Count int
	Items []string
}

// Source 提供巡检需要的外部状态；由 app 侧把 frp 服务适配过来，monitor 因此不依赖 frp 包。
type Source interface {
	Routes(ctx context.Context) ([]RouteStatus, error)
	Servers(ctx context.Context) ([]ServerStatus, error)
	Agents(ctx context.Context) ([]AgentStatus, error)
	Drift(ctx context.Context) ([]DriftStatus, error)
}

// Checker 每轮都重新读设置项：改阈值或开关不需要重启。
type Checker struct {
	settings *settings.Store
	notify   *notify.Service
	logger   *slog.Logger
	source   Source

	mu             sync.Mutex
	cpuPrev        sysinfo.CPUSample
	resourceAlerts map[string]time.Time
	failures       map[string]int
	driftAlerted   map[int64]bool
	driftNotified  map[int64]time.Time
}

func New(settingsStore *settings.Store, notifySvc *notify.Service, logger *slog.Logger, source Source) *Checker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Checker{
		settings:       settingsStore,
		notify:         notifySvc,
		logger:         logger,
		source:         source,
		resourceAlerts: map[string]time.Time{},
		failures:       map[string]int{},
		driftAlerted:   map[int64]bool{},
		driftNotified:  map[int64]time.Time{},
	}
}

// Run 跑一轮巡检。scheduler 已对任务包了 recover，这里不重复兜底。
func (c *Checker) Run(ctx context.Context) {
	if c == nil || c.settings == nil {
		return
	}
	c.checkResources(ctx)
	c.checkRoutes(ctx)
	c.checkServers(ctx)
	c.checkAgents(ctx)
	c.checkDrift(ctx)
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

func (c *Checker) checkRoutes(ctx context.Context) {
	if c.source == nil {
		return
	}
	if enabled, _ := c.settings.GetBool(ctx, settings.KeyNotifyOnTunnelDown); !enabled {
		return
	}
	limit, _ := c.settings.GetInt(ctx, settings.KeyNotifyTunnelFailThreshold)
	if limit <= 0 {
		return
	}

	routes, err := c.source.Routes(ctx)
	if err != nil {
		c.logger.Warn("巡检隧道状态失败", "module", "MONITOR", "error", err.Error())
		return
	}
	for _, route := range routes {
		if strings.TrimSpace(route.Domain) == "" {
			continue
		}
		_, event := c.track("route:"+route.Domain, route.TunnelOK, limit)
		switch event {
		case failureEventDown:
			c.notify.Alert(ctx, notify.EventTunnelDown, "隧道连续不通",
				fmt.Sprintf("%s 连续 %d 次巡检未在 frps 上注册：%s", route.Domain, limit, detailOr(route.Detail)))
		case failureEventRecovered:
			c.notify.Alert(ctx, notify.EventTunnelDown, "隧道已恢复",
				fmt.Sprintf("%s 已重新在 frps 上注册", route.Domain))
		}
	}
}

// checkServers 公网服务端掉线：进程没在跑，或隧道客户端连不上 frps。
// 只对「用户要它运行」的服务端判定，手动停掉的不报。
func (c *Checker) checkServers(ctx context.Context) {
	if c.source == nil {
		return
	}
	if enabled, _ := c.settings.GetBool(ctx, settings.KeyNotifyOnFrpsDown); !enabled {
		return
	}
	limit, _ := c.settings.GetInt(ctx, settings.KeyNotifyFrpsFailThreshold)
	if limit <= 0 {
		return
	}

	servers, err := c.source.Servers(ctx)
	if err != nil {
		c.logger.Warn("巡检服务端状态失败", "module", "MONITOR", "error", err.Error())
		return
	}
	for _, server := range servers {
		label := serverLabel(server.ID, server.Name)
		_, event := c.track(fmt.Sprintf("server:%d", server.ID), !serverDown(server), limit)
		switch event {
		case failureEventDown:
			c.notify.Alert(ctx, notify.EventFrpsServerDown, "公网服务端掉线",
				fmt.Sprintf("%s 连续 %d 次巡检未运行或未连接（进程 %s / 连接 %s）：%s",
					label, limit, stateOr(server.ProcessState), stateOr(server.ConnectionState), detailOr(server.Detail)))
		case failureEventRecovered:
			c.notify.Alert(ctx, notify.EventFrpsServerDown, "公网服务端已恢复",
				fmt.Sprintf("%s 已重新运行并连接", label))
		}
	}
}

// checkAgents agent 不可达：VPS 重启后 agent 没起来是最常见的成因。
func (c *Checker) checkAgents(ctx context.Context) {
	if c.source == nil {
		return
	}
	if enabled, _ := c.settings.GetBool(ctx, settings.KeyNotifyOnAgentUnreachable); !enabled {
		return
	}
	limit, _ := c.settings.GetInt(ctx, settings.KeyNotifyAgentFailThreshold)
	if limit <= 0 {
		return
	}

	agents, err := c.source.Agents(ctx)
	if err != nil {
		c.logger.Warn("巡检 agent 可达性失败", "module", "MONITOR", "error", err.Error())
		return
	}
	for _, agent := range agents {
		label := serverLabel(agent.ID, agent.Name)
		_, event := c.track(fmt.Sprintf("agent:%d", agent.ID), agent.Reachable, limit)
		switch event {
		case failureEventDown:
			c.notify.Alert(ctx, notify.EventAgentUnreachable, "公网 agent 不可达",
				fmt.Sprintf("%s 的 fonu-agent 连续 %d 次无响应：%s", label, limit, detailOr(agent.Detail)))
		case failureEventRecovered:
			c.notify.Alert(ctx, notify.EventAgentUnreachable, "公网 agent 已恢复",
				fmt.Sprintf("%s 的 fonu-agent 已恢复响应", label))
		}
	}
}

// checkDrift 配置漂移：VPS 上的 vhost 与 Fonu 记录不一致（被手工改过、或 agent 重装后没重部署）。
// 它不是「连续 N 次」型故障，因此用冷却时间限流：首次发现即报，之后每 cooldown 提醒一次，漂移清空时补一条恢复。
func (c *Checker) checkDrift(ctx context.Context) {
	if c.source == nil {
		return
	}
	if enabled, _ := c.settings.GetBool(ctx, settings.KeyNotifyOnRouteDrift); !enabled {
		return
	}
	cooldownHours, _ := c.settings.GetInt(ctx, settings.KeyNotifyDriftCooldownHours)
	if cooldownHours <= 0 {
		cooldownHours = notify.DefaultDriftCooldownHours
	}
	cooldown := time.Duration(cooldownHours) * time.Hour

	reports, err := c.source.Drift(ctx)
	if err != nil {
		c.logger.Warn("巡检配置漂移失败", "module", "MONITOR", "error", err.Error())
		return
	}
	now := time.Now()
	for _, report := range reports {
		c.mu.Lock()
		alerted := c.driftAlerted[report.ID]
		last := c.driftNotified[report.ID]
		if report.Count == 0 {
			delete(c.driftAlerted, report.ID)
			delete(c.driftNotified, report.ID)
		}
		c.mu.Unlock()

		label := serverLabel(report.ID, report.Name)
		if report.Count == 0 {
			if alerted {
				c.notify.Alert(ctx, notify.EventRouteDrift, "配置漂移已消除",
					fmt.Sprintf("%s 上的 vhost 已与 Fonu 记录一致", label))
			}
			continue
		}
		if alerted && !last.IsZero() && now.Sub(last) < cooldown {
			continue
		}
		c.mu.Lock()
		c.driftAlerted[report.ID] = true
		c.driftNotified[report.ID] = now
		c.mu.Unlock()
		c.notify.Alert(ctx, notify.EventRouteDrift, "公网反代配置漂移",
			fmt.Sprintf("%s 有 %d 项与 Fonu 记录不一致：%s", label, report.Count, strings.Join(report.Items, "、")))
	}
}

// track 推进某个实体的连续失败计数，返回本轮要发的事件。
// 锁只包住 map 读写：notify 一律在锁外调用。
func (c *Checker) track(key string, healthy bool, limit int) (int, failureEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	failures, event := failureTransition(c.failures[key], healthy, limit)
	c.failures[key] = failures
	return failures, event
}

type failureEvent string

const (
	failureEventNone      failureEvent = ""
	failureEventDown      failureEvent = "down"
	failureEventRecovered failureEvent = "recovered"
)

// failureTransition 推进某个实体的连续失败计数，返回新计数与本轮要发的事件。
// 只在「刚好达到阈值」与「从失败中恢复」两个时刻发，避免每轮巡检都刷通知。
func failureTransition(previous int, healthy bool, limit int) (int, failureEvent) {
	if healthy {
		if previous >= limit {
			return 0, failureEventRecovered
		}
		return 0, failureEventNone
	}
	next := previous + 1
	if next == limit {
		return next, failureEventDown
	}
	return next, failureEventNone
}

// serverDown 判定服务端是否处于「用户要它运行、但它没在跑或没连上」。
// ConnectionState 为 unknown 时按健康处理：状态源（manager+log）偶尔读不到不代表掉线，避免误报。
func serverDown(status ServerStatus) bool {
	if status.DesiredState != "running" && status.DesiredState != "starting" {
		return false
	}
	if status.ProcessState == "stopped" || status.ProcessState == "error" {
		return true
	}
	return status.ConnectionState == "disconnected" || status.ConnectionState == "error"
}

func serverLabel(id int64, name string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return fmt.Sprintf("服务端 #%d", id)
}

// stateOr 让通知里的状态字段不会因为空串而变成「（进程  / 连接 ）」。
func stateOr(state string) string {
	if strings.TrimSpace(state) == "" {
		return "未知"
	}
	return state
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
