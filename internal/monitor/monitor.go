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

	"github.com/havline/havline/internal/metrics"
	"github.com/havline/havline/internal/notify"
	"github.com/havline/havline/internal/settings"
	"github.com/havline/havline/internal/sysinfo"
)

// RouteStatus 是巡检关心的隧道与健康状态。
type RouteStatus struct {
	ServerID int64
	Domain   string
	// Known 表示这次的隧道状态是判得出来的（frps 管理接口可用）。
	// 判不出来时既不能计入连续失败，也不能当成恢复。
	Known     bool
	TunnelOK  bool
	DNSOK     bool
	ServiceOK bool
	Detail    string

	// 健康结论取值，与 frp 侧的 RouteHealthUp/Down/Unknown 保持一致，
	// 由 app 侧适配时显式翻译一次。
}

const (
	HealthUp      = "up"
	HealthDown    = "down"
	HealthUnknown = "unknown"
)

// HealthSample 是一条规则本轮的健康结论，交给落库层（monitor 不关心它写哪张表）。
type HealthSample struct {
	ServerID int64
	Domain   string
	State    string
	Reason   string
	Detail   string
}

// HealthRecorder 持久化健康结论，由 frp 服务实现。
type HealthRecorder interface {
	RecordRouteHealth(ctx context.Context, samples []HealthSample) error
	PruneRouteHealth(ctx context.Context) error
}

// CloudflareRouteStatus 是 Cloudflare 隧道域名的巡检快照。
type CloudflareRouteStatus struct {
	TunnelID int64
	Name     string
	Domain   string
	State    string
	Reason   string
	Detail   string
	Known    bool
}

// CloudflareHealthSample 是 Cloudflare 隧道的状态变更样本。
type CloudflareHealthSample struct {
	TunnelID int64
	Name     string
	Domain   string
	State    string
	Reason   string
	Detail   string
}

// CloudflareHealthSource 提供 Cloudflare 隧道健康快照。
type CloudflareHealthSource interface {
	CloudflareRoutes(ctx context.Context) ([]CloudflareRouteStatus, error)
}

// CloudflareHealthRecorder 持久化 Cloudflare 隧道健康状态。
type CloudflareHealthRecorder interface {
	RecordCloudflareHealth(ctx context.Context, samples []CloudflareHealthSample) error
	PruneCloudflareHealth(ctx context.Context) error
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

// AgentStatus 是某台服务端上 havline-agent 的可达性；没配 agent 的服务端不会出现在这里。
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

// TrafficRate 某个域名的当前速率（字节/秒）
type TrafficRate struct {
	Upload   float64
	Download float64
}

// Source 提供巡检需要的外部状态；由 app 侧把 frp 服务与流量采集器适配过来。
type Source interface {
	Routes(ctx context.Context) ([]RouteStatus, error)
	Servers(ctx context.Context) ([]ServerStatus, error)
	Agents(ctx context.Context) ([]AgentStatus, error)
	Drift(ctx context.Context) ([]DriftStatus, error)
	Traffic(ctx context.Context, domains []string) (map[string]TrafficRate, error)
}

// MetricsRecorder 落库指标历史；*metrics.Store 直接实现它。
type MetricsRecorder interface {
	Record(ctx context.Context, samples []metrics.Sample) error
	Prune(ctx context.Context) (int64, error)
}

// Checker 每轮都重新读设置项：改阈值或开关不需要重启。
type Checker struct {
	settings *settings.Store
	notify   *notify.Service
	logger   *slog.Logger
	source   Source
	health   HealthRecorder
	metrics  MetricsRecorder

	mu                sync.Mutex
	cpuPrev           sysinfo.CPUSample
	resourceAlerts    map[string]time.Time
	failures          map[string]int
	driftAlerted      map[int64]bool
	driftNotified     map[int64]time.Time
	lastHealthPrune   time.Time
	lastCFHealthPrune time.Time
	lastMetricsPrune  time.Time
}

func New(settingsStore *settings.Store, notifySvc *notify.Service, logger *slog.Logger, source Source, health HealthRecorder, metricsRecorder MetricsRecorder) *Checker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Checker{
		settings:       settingsStore,
		notify:         notifySvc,
		logger:         logger,
		source:         source,
		health:         health,
		metrics:        metricsRecorder,
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
	c.runHostChecks(ctx)
	c.checkRoutes(ctx)
	c.checkServers(ctx)
	c.checkAgents(ctx)
	c.checkDrift(ctx)
	c.checkCloudflare(ctx)
}

// runHostChecks 一次采样同时喂给「资源阈值告警」与「指标历史」。
// CPU 占用率靠两次采样的差值得出，采两次会互相干扰，所以采样点只留这一个。
func (c *Checker) runHostChecks(ctx context.Context) {
	alertEnabled, _ := c.settings.GetBool(ctx, settings.KeyNotifyOnResourceThreshold)
	// 指标历史默认开启：没配置过时按开处理（存了 "0"/"false" 才关）
	recordEnabled := c.metricsHistoryEnabled(ctx)
	// 两件事都关着就连采样都省掉（关掉的功能不该有后台开销）
	if !alertEnabled && !recordEnabled {
		return
	}

	c.mu.Lock()
	stats, sample := sysinfo.Read(c.cpuPrev)
	c.cpuPrev = sample
	c.mu.Unlock()

	if alertEnabled {
		c.checkResourceLimits(ctx, stats)
	}
	if recordEnabled {
		c.recordHostMetrics(ctx, stats)
	}
}

// metricsHistoryEnabled 读指标历史开关：未配置按开启处理（只有显式存了 0/false 才关）
func (c *Checker) metricsHistoryEnabled(ctx context.Context) bool {
	if c.metrics == nil {
		return false
	}
	raw, err := c.settings.Get(ctx, settings.KeyMetricsHistoryEnabled)
	if err != nil {
		return true
	}
	return raw != "0" && !strings.EqualFold(raw, "false")
}

// recordHostMetrics 记本机序列；采样间隔就是巡检轮次（2 分钟），行量很小
func (c *Checker) recordHostMetrics(ctx context.Context, stats sysinfo.Stats) {
	samples := []metrics.Sample{
		{Scope: metrics.ScopeHost, Metric: metrics.MetricCPU, Value: stats.CPUPercent},
		{Scope: metrics.ScopeHost, Metric: metrics.MetricMemUsed, Value: float64(stats.MemUsedBytes)},
		{Scope: metrics.ScopeHost, Metric: metrics.MetricMemTotal, Value: float64(stats.MemTotalBytes)},
		{Scope: metrics.ScopeHost, Metric: metrics.MetricDiskUsed, Value: float64(stats.DiskUsedBytes)},
		{Scope: metrics.ScopeHost, Metric: metrics.MetricDiskTotal, Value: float64(stats.DiskTotalBytes)},
	}
	if err := c.metrics.Record(ctx, samples); err != nil {
		c.logger.Warn("记录指标历史失败", "module", "MONITOR", "error", err.Error())
	}
	c.pruneMetricsIfDue(ctx)
}

// pruneMetricsIfDue 每天清理一次超期样本
func (c *Checker) pruneMetricsIfDue(ctx context.Context) {
	c.mu.Lock()
	due := c.lastMetricsPrune.IsZero() || time.Since(c.lastMetricsPrune) >= 24*time.Hour
	if due {
		c.lastMetricsPrune = time.Now()
	}
	c.mu.Unlock()
	if !due {
		return
	}
	if _, err := c.metrics.Prune(ctx); err != nil {
		c.logger.Warn("清理指标历史失败", "module", "MONITOR", "error", err.Error())
	}
}

// checkResourceLimits 只做阈值判定：采样已由 runHostChecks 统一完成
func (c *Checker) checkResourceLimits(ctx context.Context, stats sysinfo.Stats) {
	cpuLimit, _ := c.settings.GetInt(ctx, settings.KeyNotifyCPUThreshold)
	memoryLimit, _ := c.settings.GetInt(ctx, settings.KeyNotifyMemoryThreshold)
	diskLimit, _ := c.settings.GetInt(ctx, settings.KeyNotifyDiskThreshold)

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
	recordHealth, _ := c.settings.GetBool(ctx, settings.KeyRouteHealthEnabled)
	if c.health == nil {
		recordHealth = false
	}
	alertEnabled, _ := c.settings.GetBool(ctx, settings.KeyNotifyOnTunnelDown)
	recordMetrics := c.metricsHistoryEnabled(ctx)
	// 三件事都关着就别去探测：RouteHealth 是这一轮里最贵的调用
	if !recordHealth && !alertEnabled && !recordMetrics {
		return
	}

	routes, err := c.source.Routes(ctx)
	if err != nil {
		c.logger.Warn("巡检隧道状态失败", "module", "MONITOR", "error", err.Error())
		return
	}

	// 落库先于告警：判不出来的那一档也要记（unknown 与 down 必须区分）
	if recordHealth {
		samples := make([]HealthSample, 0, len(routes))
		for _, route := range routes {
			if strings.TrimSpace(route.Domain) == "" {
				continue
			}
			state, reason := healthStateOf(route)
			samples = append(samples, HealthSample{
				ServerID: route.ServerID,
				Domain:   route.Domain,
				State:    state,
				Reason:   reason,
				Detail:   route.Detail,
			})
		}
		if err := c.health.RecordRouteHealth(ctx, samples); err != nil {
			c.logger.Warn("记录规则健康状态失败", "module", "MONITOR", "error", err.Error())
		}
		c.pruneHealthIfDue(ctx)
	}

	// 流量序列用同一轮的规则列表落库，不再单独探测一轮
	if recordMetrics {
		c.recordRouteTraffic(ctx, routes)
	}

	if !alertEnabled {
		return
	}
	limit, _ := c.settings.GetInt(ctx, settings.KeyNotifyTunnelFailThreshold)
	if limit <= 0 {
		return
	}
	for _, route := range routes {
		if strings.TrimSpace(route.Domain) == "" {
			continue
		}
		if !route.Known {
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

// checkCloudflare 把 Cloudflare 隧道纳入现有巡检、通知与健康记录。
func (c *Checker) checkCloudflare(ctx context.Context) {
	source, ok := c.source.(CloudflareHealthSource)
	if !ok {
		return
	}
	recordHealth, _ := c.settings.GetBool(ctx, settings.KeyRouteHealthEnabled)
	recorder, canRecord := c.source.(CloudflareHealthRecorder)
	if !canRecord {
		recordHealth = false
	}
	alertEnabled, _ := c.settings.GetBool(ctx, settings.KeyNotifyOnTunnelDown)
	if !recordHealth && !alertEnabled {
		return
	}
	routes, err := source.CloudflareRoutes(ctx)
	if err != nil {
		c.logger.Warn("巡检 Cloudflare 隧道状态失败", "module", "MONITOR", "error", err.Error())
		return
	}
	if recordHealth {
		samples := make([]CloudflareHealthSample, 0, len(routes))
		for _, route := range routes {
			if strings.TrimSpace(route.Domain) == "" {
				continue
			}
			samples = append(samples, CloudflareHealthSample{
				TunnelID: route.TunnelID,
				Name:     route.Name,
				Domain:   route.Domain,
				State:    route.State,
				Reason:   route.Reason,
				Detail:   route.Detail,
			})
		}
		if err := recorder.RecordCloudflareHealth(ctx, samples); err != nil {
			c.logger.Warn("记录 Cloudflare 隧道健康失败", "module", "MONITOR", "error", err.Error())
		}
		c.pruneCloudflareHealthIfDue(ctx, recorder)
	}
	if !alertEnabled {
		return
	}
	limit, _ := c.settings.GetInt(ctx, settings.KeyNotifyTunnelFailThreshold)
	if limit <= 0 {
		return
	}
	for _, route := range routes {
		if !route.Known || strings.TrimSpace(route.Domain) == "" {
			continue
		}
		_, event := c.track(fmt.Sprintf("cloudflare:%d:%s", route.TunnelID, route.Domain), route.State == HealthUp, limit)
		switch event {
		case failureEventDown:
			c.notify.Alert(ctx, notify.EventTunnelDown, "Cloudflare 隧道连续不通",
				fmt.Sprintf("%s 连续 %d 次巡检未连接：%s", route.Domain, limit, detailOr(route.Detail)))
		case failureEventRecovered:
			c.notify.Alert(ctx, notify.EventTunnelDown, "Cloudflare 隧道已恢复",
				fmt.Sprintf("%s 已重新连接 Cloudflare 边缘", route.Domain))
		}
	}
}

func (c *Checker) pruneCloudflareHealthIfDue(ctx context.Context, recorder CloudflareHealthRecorder) {
	c.mu.Lock()
	due := c.lastCFHealthPrune.IsZero() || time.Since(c.lastCFHealthPrune) >= 24*time.Hour
	if due {
		c.lastCFHealthPrune = time.Now()
	}
	c.mu.Unlock()
	if !due {
		return
	}
	if err := recorder.PruneCloudflareHealth(ctx); err != nil {
		c.logger.Warn("清理 Cloudflare 隧道健康记录失败", "module", "MONITOR", "error", err.Error())
	}
}

// healthStateOf 把一段状态翻成与 frp 侧一致的结论 + 失败环节
func healthStateOf(route RouteStatus) (string, string) {
	if !route.Known {
		return HealthUnknown, "dashboard"
	}
	switch {
	case !route.DNSOK:
		return HealthDown, "dns"
	case !route.TunnelOK:
		return HealthDown, "tunnel"
	case !route.ServiceOK:
		return HealthDown, "service"
	default:
		return HealthUp, ""
	}
}

// recordRouteTraffic 记每条规则的上下行速率（scope 按服务端与域名区分）
func (c *Checker) recordRouteTraffic(ctx context.Context, routes []RouteStatus) {
	if c.source == nil || c.metrics == nil {
		return
	}
	domains := make([]string, 0, len(routes))
	for _, route := range routes {
		if domain := strings.TrimSpace(route.Domain); domain != "" {
			domains = append(domains, domain)
		}
	}
	if len(domains) == 0 {
		return
	}
	rates, err := c.source.Traffic(ctx, domains)
	if err != nil {
		c.logger.Warn("读取流量快照失败", "module", "MONITOR", "error", err.Error())
		return
	}
	samples := make([]metrics.Sample, 0, len(routes)*2)
	for _, route := range routes {
		domain := strings.ToLower(strings.TrimSpace(route.Domain))
		rate, ok := rates[domain]
		if !ok {
			continue
		}
		scope := fmt.Sprintf("route:%d:%s", route.ServerID, domain)
		samples = append(samples,
			metrics.Sample{Scope: scope, Metric: metrics.MetricTrafficIn, Value: rate.Upload},
			metrics.Sample{Scope: scope, Metric: metrics.MetricTrafficOut, Value: rate.Download},
		)
	}
	if len(samples) == 0 {
		return
	}
	if err := c.metrics.Record(ctx, samples); err != nil {
		c.logger.Warn("记录流量历史失败", "module", "MONITOR", "error", err.Error())
	}
}

// pruneHealthIfDue 每天清理一次超期记录：不在每轮都删，省一次写事务
func (c *Checker) pruneHealthIfDue(ctx context.Context) {
	c.mu.Lock()
	due := c.lastHealthPrune.IsZero() || time.Since(c.lastHealthPrune) >= 24*time.Hour
	if due {
		c.lastHealthPrune = time.Now()
	}
	c.mu.Unlock()
	if !due {
		return
	}
	if err := c.health.PruneRouteHealth(ctx); err != nil {
		c.logger.Warn("清理健康记录失败", "module", "MONITOR", "error", err.Error())
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
				fmt.Sprintf("%s 的 havline-agent 连续 %d 次无响应：%s", label, limit, detailOr(agent.Detail)))
		case failureEventRecovered:
			c.notify.Alert(ctx, notify.EventAgentUnreachable, "公网 agent 已恢复",
				fmt.Sprintf("%s 的 havline-agent 已恢复响应", label))
		}
	}
}

// checkDrift 配置漂移：VPS 上的 vhost 与 Havline 记录不一致（被手工改过、或 agent 重装后没重部署）。
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
					fmt.Sprintf("%s 上的 vhost 已与 Havline 记录一致", label))
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
			fmt.Sprintf("%s 有 %d 项与 Havline 记录不一致：%s", label, report.Count, strings.Join(report.Items, "、")))
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
