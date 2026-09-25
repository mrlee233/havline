package monitor

import "testing"

func TestFailureTransitionAlertsOncePerOutage(t *testing.T) {
	const limit = 3
	failures := 0
	var events []failureEvent

	// 连续三轮不通：只在第 3 轮（刚好达到阈值）发一次
	for i := 0; i < 5; i++ {
		var event failureEvent
		failures, event = failureTransition(failures, false, limit)
		if event != failureEventNone {
			events = append(events, event)
		}
	}
	if len(events) != 1 || events[0] != failureEventDown {
		t.Fatalf("连续不通应只告警一次，实际 %v", events)
	}
	if failures != 5 {
		t.Fatalf("失败计数应继续累加，实际 %d", failures)
	}

	// 恢复：发一次恢复通知并清零
	failures, event := failureTransition(failures, true, limit)
	if event != failureEventRecovered || failures != 0 {
		t.Fatalf("恢复应通知一次并清零：event=%v failures=%d", event, failures)
	}

	// 恢复后再次不通：还能再报一次
	failures, event = failureTransition(failures, false, limit)
	if event != failureEventNone || failures != 1 {
		t.Fatalf("恢复后重新计数：event=%v failures=%d", event, failures)
	}
	failures, event = failureTransition(failures, false, limit)
	if event != failureEventNone {
		t.Fatalf("未达阈值不应告警：%v", event)
	}
	if _, event = failureTransition(failures, false, limit); event != failureEventDown {
		t.Fatalf("第二次中断达到阈值应再告警一次：%v", event)
	}
}

func TestFailureTransitionBelowThresholdIsSilent(t *testing.T) {
	// 单次抖动（未达阈值）不打扰用户，也不算作恢复
	if failures, event := failureTransition(0, true, 3); failures != 0 || event != failureEventNone {
		t.Fatalf("健康状态不应产生事件：%d %v", failures, event)
	}
	if failures, event := failureTransition(1, true, 3); failures != 0 || event != failureEventNone {
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

func TestServerDownOnlyWhenUserWantsItRunning(t *testing.T) {
	// 用户手动停掉的不算掉线
	stopped := ServerStatus{DesiredState: "stopped", ProcessState: "stopped", ConnectionState: "disconnected"}
	if serverDown(stopped) {
		t.Fatal("用户手动停止的服务端不应判为掉线")
	}
	// 期望运行但进程没在跑
	if !serverDown(ServerStatus{DesiredState: "running", ProcessState: "stopped"}) {
		t.Fatal("期望运行但进程未运行应判为掉线")
	}
	// 进程在跑但连不上 frps
	if !serverDown(ServerStatus{DesiredState: "running", ProcessState: "running", ConnectionState: "disconnected"}) {
		t.Fatal("连接断开应判为掉线")
	}
	// 状态源读不到连接状态时不报警：manager+log 偶尔读不到不代表掉线
	if serverDown(ServerStatus{DesiredState: "running", ProcessState: "running", ConnectionState: "unknown"}) {
		t.Fatal("连接状态未知时不应误报")
	}
}

func TestServerLabelFallsBackToID(t *testing.T) {
	if got := serverLabel(3, "   "); got != "服务端 #3" {
		t.Fatalf("无名服务端应回落到编号，实际 %q", got)
	}
	if got := serverLabel(3, "VPS-香港"); got != "VPS-香港" {
		t.Fatalf("有名字应直接用名字，实际 %q", got)
	}
}

func TestStateOrFallsBackToPlaceholder(t *testing.T) {
	if got := stateOr(""); got != "未知" {
		t.Fatalf("空状态应回落到占位文案，实际 %q", got)
	}
	if got := stateOr("error"); got != "error" {
		t.Fatalf("非空状态应原样返回，实际 %q", got)
	}
}
