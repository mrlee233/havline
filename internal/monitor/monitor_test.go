package monitor

import "testing"

func TestTunnelTransitionAlertsOncePerOutage(t *testing.T) {
	const limit = 3
	failures := 0
	var events []tunnelEvent

	// 连续三轮不通：只在第 3 轮（刚好达到阈值）发一次
	for i := 0; i < 5; i++ {
		var event tunnelEvent
		failures, event = tunnelTransition(failures, false, limit)
		if event != tunnelEventNone {
			events = append(events, event)
		}
	}
	if len(events) != 1 || events[0] != tunnelEventDown {
		t.Fatalf("连续不通应只告警一次，实际 %v", events)
	}
	if failures != 5 {
		t.Fatalf("失败计数应继续累加，实际 %d", failures)
	}

	// 恢复：发一次恢复通知并清零
	failures, event := tunnelTransition(failures, true, limit)
	if event != tunnelEventRecovered || failures != 0 {
		t.Fatalf("恢复应通知一次并清零：event=%v failures=%d", event, failures)
	}

	// 恢复后再次不通：还能再报一次
	failures, event = tunnelTransition(failures, false, limit)
	if event != tunnelEventNone || failures != 1 {
		t.Fatalf("恢复后重新计数：event=%v failures=%d", event, failures)
	}
	failures, event = tunnelTransition(failures, false, limit)
	if event != tunnelEventNone {
		t.Fatalf("未达阈值不应告警：%v", event)
	}
	if _, event = tunnelTransition(failures, false, limit); event != tunnelEventDown {
		t.Fatalf("第二次中断达到阈值应再告警一次：%v", event)
	}
}

func TestTunnelTransitionBelowThresholdIsSilent(t *testing.T) {
	// 单次抖动（未达阈值）不打扰用户，也不算作恢复
	if failures, event := tunnelTransition(0, true, 3); failures != 0 || event != tunnelEventNone {
		t.Fatalf("健康状态不应产生事件：%d %v", failures, event)
	}
	if failures, event := tunnelTransition(1, true, 3); failures != 0 || event != tunnelEventNone {
		t.Fatalf("未达阈值的恢复不应通知：%d %v", failures, event)
	}
}

func TestPercent(t *testing.T) {
	if got := percent(0, 0); got != 0 {
		t.Fatalf("总数为 0 时应返回 0，实际 %v", got)
	}
	if got := percent(50, 200); got != 25 {
		t.Fatalf("期望 25，实际 %v", got)
	}
}

func TestDetailOrFallsBack(t *testing.T) {
	if got := detailOr("  "); got != "未返回具体原因" {
		t.Fatalf("空详情应给出兜底文案，实际 %q", got)
	}
	if got := detailOr("dashboard 超时"); got != "dashboard 超时" {
		t.Fatalf("非空详情应原样返回，实际 %q", got)
	}
}
