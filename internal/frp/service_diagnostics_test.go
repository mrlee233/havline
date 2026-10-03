package frp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDiagnoseDialError(t *testing.T) {
	err := errors.New("connectex: An attempt was made to access a socket in a way forbidden by its access permissions.")
	message := diagnoseDialError(err)
	if !strings.HasPrefix(message, "本机网络策略阻止连接：") {
		t.Fatalf("unexpected message: %s", message)
	}
}

// 23.3：诊断拆分为独立检查项,登录状态从 frpc 日志推断,unknown 不等于失败。
func TestSummarizeFRPCLogin(t *testing.T) {
	logLines := strings.Join([]string{
		"2026/09/17 10:00:00 [I] [root.go:139] frpc service start",
		"2026/09/17 10:00:01 [W] [control.go:167] login to the server failed: token error",
		"2026/09/17 10:01:00 [I] [control.go:164] login to server success, get run id [abc]",
	}, "\n")
	state, detail := summarizeFRPCLogin(logLines)
	if state != diagStateOK {
		t.Fatalf("login success should win over earlier failure, got %s (%s)", state, detail)
	}
	failedLog := "2026/09/17 10:00:01 [W] [control.go:167] login to the server failed: token [wrong] error: authentication failed"
	state, detail = summarizeFRPCLogin(failedLog)
	if state != diagStateFailed || !strings.Contains(detail, "登录失败") {
		t.Fatalf("login failure should be reported, got %s (%s)", state, detail)
	}
	state, _ = summarizeFRPCLogin("2026/09/17 10:00:00 [I] [root.go:139] frpc service start")
	if state != diagStateUnknown {
		t.Fatalf("no login evidence should be unknown, got %s", state)
	}
}

func TestDiagnoseDNSFormatsRecords(t *testing.T) {
	check := diagnoseDNS(context.Background(), "192.168.1.10")
	if check.State != diagStateOK || !strings.Contains(check.Detail, "IP 地址") {
		t.Fatalf("literal IP should skip resolution: %+v", check)
	}
}

func TestSummarizeServerSamplesUsesLatestStatus(t *testing.T) {
	detail := summarizeServerSamples([]ServerSample{
		{CheckedAt: "2026-09-15 18:00:00", Available: true, LatencyMS: 20},
		{CheckedAt: "2026-09-15 18:01:00", Available: false, Error: "连接失败"},
	})
	if detail.Available {
		t.Fatal("latest failed sample should make current status unavailable")
	}
	if detail.Availability != 50 {
		t.Fatalf("unexpected availability: %v", detail.Availability)
	}
	if detail.AverageLatency != 20 || detail.PeakLatency != 20 {
		t.Fatalf("unexpected latency summary: average=%d peak=%d", detail.AverageLatency, detail.PeakLatency)
	}
	if detail.LastError != "连接失败" {
		t.Fatalf("unexpected last error: %s", detail.LastError)
	}
}
