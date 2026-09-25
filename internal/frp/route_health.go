package frp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 健康状态取值。unknown 表示「判不出来」（frps 管理接口未配或不可用），
// 不能当成 down —— 这正是 tunnel_known 存在的理由。
const (
	RouteHealthUp      = "up"
	RouteHealthDown    = "down"
	RouteHealthUnknown = "unknown"

	// RouteHealthRetention 状态变更记录的保留期
	RouteHealthRetention = 30 * 24 * time.Hour
)

// RouteHealthSample 是一轮巡检得到的一条规则健康结论
type RouteHealthSample struct {
	ServerID int64
	Domain   string
	State    string
	Reason   string
	Detail   string
}

// RouteHealthEvent 一条状态变更记录
type RouteHealthEvent struct {
	ServerID int64  `json:"server_id"`
	Domain   string `json:"domain"`
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
	Detail   string `json:"detail,omitempty"`
	At       string `json:"at"`
}

// RouteHealthIncident 一段不可用区间；To 为空表示到窗口结束仍在持续
type RouteHealthIncident struct {
	From    string `json:"from"`
	To      string `json:"to,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Minutes int    `json:"minutes"`
}

// RouteUptimeStat 一条规则在窗口内的可用率与不可用区间
type RouteUptimeStat struct {
	ServerID       int64                 `json:"server_id"`
	Domain         string                `json:"domain"`
	State          string                `json:"state"`
	Uptime         float64               `json:"uptime_pct"`
	DownMinutes    int                   `json:"down_minutes"`
	UnknownMinutes int                   `json:"unknown_minutes"`
	CoveredMinutes int                   `json:"covered_minutes"`
	Incidents      []RouteHealthIncident `json:"incidents"`
}

// RecordRouteHealth 只写「状态变更」：与上一条同状态就不写，避免表被每 2 分钟的采样撑大。
// 返回写入条数。
func (s *Service) RecordRouteHealth(ctx context.Context, samples []RouteHealthSample) (int, error) {
	if len(samples) == 0 {
		return 0, nil
	}
	current, err := s.store.latestRouteHealthStates(ctx)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	written := 0
	for _, sample := range samples {
		domain := strings.ToLower(strings.TrimSpace(sample.Domain))
		if domain == "" || sample.State == "" {
			continue
		}
		key := routeHealthKey(sample.ServerID, domain)
		if state, ok := current[key]; ok && state == sample.State {
			continue
		}
		if _, err := s.store.db.ExecContext(ctx,
			`INSERT INTO route_health_events(server_id, domain, state, reason, detail, at) VALUES (?, ?, ?, ?, ?, ?)`,
			sample.ServerID, domain, sample.State, sample.Reason, sample.Detail, now); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// RouteUptime 返回窗口内每条规则的可用率与不可用区间
func (s *Service) RouteUptime(ctx context.Context, window time.Duration) ([]RouteUptimeStat, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}
	now := time.Now().UTC()
	since := now.Add(-window)
	events, err := s.store.routeHealthEventsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	return ComputeRouteUptime(events, since, now), nil
}

// PruneRouteHealth 清理超过保留期的状态变更
func (s *Service) PruneRouteHealth(ctx context.Context) (int64, error) {
	return s.store.pruneRouteHealthEvents(ctx, time.Now().UTC().Add(-RouteHealthRetention))
}

func routeHealthKey(serverID int64, domain string) string {
	return fmt.Sprintf("%d|%s", serverID, domain)
}

// ComputeRouteUptime 由状态变更序列推导 [from, to] 窗口内的可用率。
//
// 覆盖时长只算「有记录可依」的部分：窗口开始前有记录时，其状态延续到窗口开始；
// 窗口内才第一次出现的规则，从它第一条记录开始算。窗口内完全没有记录的规则不出现在结果里。
// 可用率按 up/(up+down) 计，unknown 不计入分母（判不出来不能算不可用），但单独给出时长。
func ComputeRouteUptime(events []RouteHealthEvent, from, to time.Time) []RouteUptimeStat {
	type point struct {
		at             time.Time
		serverID       int64
		domain         string
		state          string
		reason, detail string
	}
	byRule := map[string][]point{}
	order := make([]string, 0, len(events))
	for _, event := range events {
		at, err := time.Parse(time.RFC3339, event.At)
		if err != nil {
			continue
		}
		key := routeHealthKey(event.ServerID, event.Domain)
		if _, ok := byRule[key]; !ok {
			order = append(order, key)
		}
		byRule[key] = append(byRule[key], point{
			at: at, serverID: event.ServerID, domain: event.Domain,
			state: event.State, reason: event.Reason, detail: event.Detail,
		})
	}
	sort.Strings(order)

	stats := make([]RouteUptimeStat, 0, len(order))
	for _, key := range order {
		points := byRule[key]
		sort.Slice(points, func(i, j int) bool { return points[i].at.Before(points[j].at) })

		// 窗口起点的生效状态：取 from 之前最后一条；没有则从第一条记录开始覆盖
		start := from
		index := 0
		state := ""
		reason, detail := "", ""
		for index < len(points) && !points[index].at.After(from) {
			state, reason, detail = points[index].state, points[index].reason, points[index].detail
			index++
		}
		if state == "" {
			if index >= len(points) {
				continue
			}
			start = points[index].at
			state, reason, detail = points[index].state, points[index].reason, points[index].detail
			index++
		}

		stat := RouteUptimeStat{
			ServerID:  points[0].serverID,
			Domain:    points[0].domain,
			State:     state,
			Incidents: make([]RouteHealthIncident, 0, 2),
		}
		upMinutes := 0
		// stat.State 要是「窗口末尾的状态」，不是窗口开始时的状态
		lastState := state
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
			case RouteHealthUp:
				upMinutes += minutes
			case RouteHealthDown:
				stat.DownMinutes += minutes
				incident := RouteHealthIncident{
					From:    start.UTC().Format(time.RFC3339),
					Reason:  reason,
					Detail:  detail,
					Minutes: minutes,
				}
				if !ongoing {
					incident.To = end.UTC().Format(time.RFC3339)
				}
				stat.Incidents = append(stat.Incidents, incident)
			default:
				stat.UnknownMinutes += minutes
			}
			stat.CoveredMinutes += minutes
			lastState = state

			if index >= len(points) {
				break
			}
			start = points[index].at
			state, reason, detail = points[index].state, points[index].reason, points[index].detail
			index++
		}

		stat.State = lastState
		observed := upMinutes + stat.DownMinutes
		if observed > 0 {
			stat.Uptime = float64(upMinutes) / float64(observed) * 100
		} else {
			// 窗口内既没有 up 也没有 down（只有 unknown）时不编造可用率
			stat.Uptime = 100
			if stat.UnknownMinutes > 0 {
				stat.Uptime = 0
			}
		}
		stats = append(stats, stat)
	}
	return stats
}
