package cloudflared

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	HealthUp      = "up"
	HealthDown    = "down"
	HealthUnknown = "unknown"

	HealthRetention = 30 * 24 * time.Hour
)

// HealthStatus 一条域名在当前巡检轮次中的健康快照。
type HealthStatus struct {
	TunnelID int64
	Name     string
	Domain   string
	State    string
	Reason   string
	Detail   string
	Known    bool
}

// HealthSample 交给落库层的状态样本。
type HealthSample struct {
	TunnelID int64
	Name     string
	Domain   string
	State    string
	Reason   string
	Detail   string
}

// HealthEvent 一条状态变更记录。
type HealthEvent struct {
	TunnelID int64  `json:"tunnel_id"`
	Domain   string `json:"domain"`
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
	Detail   string `json:"detail,omitempty"`
	At       string `json:"at"`
}

// HealthIncident 一段 Cloudflare 隧道不可用区间。
type HealthIncident struct {
	From    string `json:"from"`
	To      string `json:"to,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Minutes int    `json:"minutes"`
}

// UptimeStat 一条 Cloudflare 域名的可用率与不可用区间。
type UptimeStat struct {
	TunnelID       int64            `json:"tunnel_id"`
	Domain         string           `json:"domain"`
	State          string           `json:"state"`
	Uptime         float64          `json:"uptime_pct"`
	DownMinutes    int              `json:"down_minutes"`
	UnknownMinutes int              `json:"unknown_minutes"`
	CoveredMinutes int              `json:"covered_minutes"`
	Incidents      []HealthIncident `json:"incidents"`
}

// TunnelSummary 仪表盘使用的 Cloudflare 隧道概览。
type TunnelSummary struct {
	Count     int
	Online    int
	Status    string
	Message   string
	LastError string
}

// HealthStatuses 返回所有 Cloudflare 隧道域名的当前健康状态。
func (s *Service) HealthStatuses(ctx context.Context) ([]HealthStatus, error) {
	settings, err := s.store.GetSettingsRaw(ctx)
	if err != nil {
		return nil, err
	}
	tunnels, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := []HealthStatus{}
	for i := range tunnels {
		tunnel := tunnels[i]
		s.attachConfig(&tunnel)
		status := s.manager.Status(tunnel.ID)
		for _, domain := range tunnel.Hostnames {
			out = append(out, healthStatus(settings.Enabled, tunnel, status, domain))
		}
	}
	return out, nil
}

func healthStatus(globalEnabled bool, tunnel Tunnel, status RuntimeStatus, domain string) HealthStatus {
	item := HealthStatus{TunnelID: tunnel.ID, Name: tunnel.Name, Domain: domain, State: HealthUnknown, Known: false}
	if !globalEnabled {
		item.Reason = "disabled"
		item.Detail = "Cloudflare 隧道全局托管已停用"
		return item
	}
	if status.Running {
		item.Known = true
		if status.Status == "connected" {
			item.State = HealthUp
			return item
		}
		item.State = HealthDown
		item.Reason = "connection"
		item.Detail = strings.TrimSpace(status.LastError)
		return item
	}
	if tunnel.AutoStart {
		item.Known = true
		item.State = HealthDown
		item.Reason = "process"
		item.Detail = strings.TrimSpace(status.LastError)
		return item
	}
	item.Reason = "stopped"
	item.Detail = "隧道未运行，且未启用自动启动"
	return item
}

// RecordHealth 只写状态变化，避免每轮巡检都插入重复记录。
func (s *Service) RecordHealth(ctx context.Context, samples []HealthSample) error {
	if len(samples) == 0 {
		return nil
	}
	current, err := s.latestHealthStates(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, sample := range samples {
		domain := strings.ToLower(strings.TrimSpace(sample.Domain))
		if sample.TunnelID <= 0 || domain == "" || sample.State == "" {
			continue
		}
		key := healthKey(sample.TunnelID, domain)
		if state, ok := current[key]; ok && state == sample.State {
			continue
		}
		if _, err := s.store.db.ExecContext(ctx, `
			INSERT INTO cf_health_events(tunnel_id, domain, state, reason, detail, at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, sample.TunnelID, domain, sample.State, sample.Reason, sample.Detail, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) latestHealthStates(ctx context.Context) (map[string]string, error) {
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT e.tunnel_id, e.domain, e.state
		FROM cf_health_events e
		JOIN (
			SELECT tunnel_id, domain, MAX(id) AS id
			FROM cf_health_events
			GROUP BY tunnel_id, domain
		) m ON m.id = e.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := map[string]string{}
	for rows.Next() {
		var tunnelID int64
		var domain, state string
		if err := rows.Scan(&tunnelID, &domain, &state); err != nil {
			return nil, err
		}
		states[healthKey(tunnelID, domain)] = state
	}
	return states, rows.Err()
}

// HealthUptime 返回窗口内每条 Cloudflare 域名的可用率。
func (s *Service) HealthUptime(ctx context.Context, window time.Duration) ([]UptimeStat, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}
	now := time.Now().UTC()
	since := now.Add(-window)
	events, err := s.healthEventsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	return ComputeHealthUptime(events, since, now), nil
}

func (s *Service) healthEventsSince(ctx context.Context, since time.Time) ([]HealthEvent, error) {
	stamp := since.UTC().Format(time.RFC3339)
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT tunnel_id, domain, state, COALESCE(reason, ''), COALESCE(detail, ''), at
		FROM cf_health_events WHERE at >= ?
		UNION ALL
		SELECT e.tunnel_id, e.domain, e.state, COALESCE(e.reason, ''), COALESCE(e.detail, ''), e.at
		FROM cf_health_events e
		JOIN (
			SELECT tunnel_id, domain, MAX(id) AS id
			FROM cf_health_events WHERE at < ?
			GROUP BY tunnel_id, domain
		) m ON m.id = e.id
	`, stamp, stamp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []HealthEvent{}
	for rows.Next() {
		var event HealthEvent
		if err := rows.Scan(&event.TunnelID, &event.Domain, &event.State, &event.Reason, &event.Detail, &event.At); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// PruneHealth 清理超过保留期的状态变更。
func (s *Service) PruneHealth(ctx context.Context) (int64, error) {
	res, err := s.store.db.ExecContext(ctx, `DELETE FROM cf_health_events WHERE at < ?`,
		time.Now().UTC().Add(-HealthRetention).Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Summary 返回隧道数量、在线数与最近错误。
func (s *Service) Summary(ctx context.Context) TunnelSummary {
	tunnels, err := s.store.List(ctx)
	if err != nil {
		return TunnelSummary{Status: "error", Message: err.Error()}
	}
	summary := TunnelSummary{Count: len(tunnels), Status: "ok"}
	if len(tunnels) == 0 {
		summary.Status = "none"
		summary.Message = "未创建隧道"
		return summary
	}
	for _, tunnel := range tunnels {
		status := s.manager.Status(tunnel.ID)
		if status.Running && status.Status == "connected" {
			summary.Online++
		}
		if strings.TrimSpace(tunnel.LastError) != "" {
			summary.LastError = tunnel.LastError
		}
	}
	if summary.Online == 0 {
		summary.Status = "error"
		summary.Message = "没有在线隧道"
	} else if summary.Online < summary.Count {
		summary.Status = "warning"
		summary.Message = fmt.Sprintf("%d/%d 条在线", summary.Online, summary.Count)
	} else {
		summary.Message = fmt.Sprintf("%d 条在线", summary.Online)
	}
	return summary
}

func healthKey(tunnelID int64, domain string) string {
	return fmt.Sprintf("%d|%s", tunnelID, strings.ToLower(strings.TrimSpace(domain)))
}

// ComputeHealthUptime 与 FRP 侧同样的口径：unknown 不计入可用率分母。
type healthPoint struct {
	at             time.Time
	tunnelID       int64
	domain         string
	state          string
	reason, detail string
}

func ComputeHealthUptime(events []HealthEvent, from, to time.Time) []UptimeStat {
	byRule := map[string][]healthPoint{}
	order := make([]string, 0, len(events))
	for _, event := range events {
		at, err := time.Parse(time.RFC3339, event.At)
		if err != nil {
			continue
		}
		key := healthKey(event.TunnelID, event.Domain)
		if _, ok := byRule[key]; !ok {
			order = append(order, key)
		}
		byRule[key] = append(byRule[key], healthPoint{
			at: at, tunnelID: event.TunnelID, domain: event.Domain,
			state: event.State, reason: event.Reason, detail: event.Detail,
		})
	}
	sort.Strings(order)
	stats := make([]UptimeStat, 0, len(order))
	for _, key := range order {
		points := byRule[key]
		sort.Slice(points, func(i, j int) bool { return points[i].at.Before(points[j].at) })
		stat := computeHealthPoints(points, from, to)
		if stat.Domain != "" {
			stats = append(stats, stat)
		}
	}
	return stats
}

func computeHealthPoints(points []healthPoint, from, to time.Time) UptimeStat {
	start := from
	index := 0
	state, reason, detail := "", "", ""
	for index < len(points) && !points[index].at.After(from) {
		state, reason, detail = points[index].state, points[index].reason, points[index].detail
		index++
	}
	if state == "" {
		if index >= len(points) {
			return UptimeStat{}
		}
		start = points[index].at
		state, reason, detail = points[index].state, points[index].reason, points[index].detail
		index++
	}
	stat := UptimeStat{TunnelID: points[0].tunnelID, Domain: points[0].domain, State: state, Incidents: []HealthIncident{}}
	upMinutes := 0
	for {
		end := to
		ongoing := true
		if index < len(points) {
			end = points[index].at
			ongoing = false
		}
		minutes := int(end.Sub(start).Minutes())
		if minutes < 0 {
			minutes = 0
		}
		switch state {
		case HealthUp:
			upMinutes += minutes
		case HealthDown:
			stat.DownMinutes += minutes
			incident := HealthIncident{From: start.UTC().Format(time.RFC3339), Reason: reason, Detail: detail, Minutes: minutes}
			if !ongoing {
				incident.To = end.UTC().Format(time.RFC3339)
			}
			stat.Incidents = append(stat.Incidents, incident)
		default:
			stat.UnknownMinutes += minutes
		}
		stat.CoveredMinutes += minutes
		stat.State = state
		if index >= len(points) {
			break
		}
		start = points[index].at
		state, reason, detail = points[index].state, points[index].reason, points[index].detail
		index++
	}
	observed := upMinutes + stat.DownMinutes
	if observed > 0 {
		stat.Uptime = float64(upMinutes) / float64(observed) * 100
	} else {
		stat.Uptime = 100
		if stat.UnknownMinutes > 0 {
			stat.Uptime = 0
		}
	}
	return stat
}
