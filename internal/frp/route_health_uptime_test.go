package frp

import (
	"testing"
	"time"
)

func findUptimeStat(t *testing.T, stats []RouteUptimeStat, domain string) RouteUptimeStat {
	t.Helper()
	for _, stat := range stats {
		if stat.Domain == domain {
			return stat
		}
	}
	t.Fatalf("结果里没有 %s：%+v", domain, stats)
	return RouteUptimeStat{}
}

func TestComputeRouteUptimeIncidentAndUnknown(t *testing.T) {
	from := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	at := func(offset time.Duration) string { return from.Add(offset).Format(time.RFC3339) }

	events := []RouteHealthEvent{
		// 窗口开始前就是 up：状态延续到窗口开始
		{ServerID: 1, Domain: "a.com", State: RouteHealthUp, At: at(-2 * time.Hour)},
		{ServerID: 1, Domain: "a.com", State: RouteHealthDown, At: at(time.Hour), Reason: "tunnel"},
		{ServerID: 1, Domain: "a.com", State: RouteHealthUp, At: at(2 * time.Hour)},
		// 窗口内才第一次出现
		{ServerID: 1, Domain: "b.com", State: RouteHealthUp, At: at(6 * time.Hour)},
		// 判不出来
		{ServerID: 2, Domain: "c.com", State: RouteHealthUnknown, At: at(-time.Hour)},
	}

	stats := ComputeRouteUptime(events, from, to)
	if len(stats) != 3 {
		t.Fatalf("应有 3 条规则的统计，实际 %d：%+v", len(stats), stats)
	}

	a := findUptimeStat(t, stats, "a.com")
	if a.CoveredMinutes != 24*60 {
		t.Fatalf("窗口开始前的状态应延续，覆盖 24h，实际 %d 分钟", a.CoveredMinutes)
	}
	if a.DownMinutes != 60 {
		t.Fatalf("不可用应为 60 分钟，实际 %d", a.DownMinutes)
	}
	if a.Uptime < 95 || a.Uptime > 96 {
		t.Fatalf("可用率应约 95.8%%，实际 %.2f", a.Uptime)
	}
	if len(a.Incidents) != 1 || a.Incidents[0].Minutes != 60 || a.Incidents[0].Reason != "tunnel" {
		t.Fatalf("不可用区间不对：%+v", a.Incidents)
	}
	if a.Incidents[0].To == "" {
		t.Fatal("已恢复的区间应有结束时间")
	}
	if a.State != RouteHealthUp {
		t.Fatalf("窗口末尾状态应为 up，实际 %s", a.State)
	}

	b := findUptimeStat(t, stats, "b.com")
	if b.CoveredMinutes != 18*60 {
		t.Fatalf("窗口内才出现的规则应从第一条记录起算 18h，实际 %d", b.CoveredMinutes)
	}
	if b.Uptime != 100 || b.DownMinutes != 0 {
		t.Fatalf("没有任何 down 时可用率应为 100：%+v", b)
	}

	c := findUptimeStat(t, stats, "c.com")
	if c.UnknownMinutes != 24*60 || c.DownMinutes != 0 {
		t.Fatalf("全程 unknown 不该算作不可用：%+v", c)
	}
	if c.Uptime != 0 {
		t.Fatalf("全程判不出来时不能编造可用率（不给 100），实际 %.2f", c.Uptime)
	}
}

func TestComputeRouteUptimeOngoingIncident(t *testing.T) {
	from := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)

	events := []RouteHealthEvent{
		{ServerID: 1, Domain: "a.com", State: RouteHealthUp, At: from.Add(0).Format(time.RFC3339)},
		{ServerID: 1, Domain: "a.com", State: RouteHealthDown, At: from.Add(20 * time.Hour).Format(time.RFC3339), Reason: "service"},
	}

	stats := ComputeRouteUptime(events, from, to)
	stat := findUptimeStat(t, stats, "a.com")
	if stat.DownMinutes != 4*60 {
		t.Fatalf("到窗口结束仍在持续应计 240 分钟，实际 %d", stat.DownMinutes)
	}
	if len(stat.Incidents) != 1 {
		t.Fatalf("应有 1 段区间：%+v", stat.Incidents)
	}
	if stat.Incidents[0].To != "" {
		t.Fatalf("仍在持续的区间不应有结束时间，实际 %q", stat.Incidents[0].To)
	}
	if stat.State != RouteHealthDown {
		t.Fatalf("窗口末尾状态应为 down，实际 %s", stat.State)
	}
}

func TestComputeRouteUptimeSkipsRulesWithoutEvents(t *testing.T) {
	from := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	// 窗口内没有任何记录 → 不出现在结果里，而不是给一个 100%
	if stats := ComputeRouteUptime(nil, from, from.Add(time.Hour)); len(stats) != 0 {
		t.Fatalf("没有记录时不该有统计：%+v", stats)
	}
}
