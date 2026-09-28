package agent

import "testing"

func TestParseUFWStatus(t *testing.T) {
	output := `Status: active

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW       Anywhere
7443/tcp                   ALLOW       Anywhere
`
	active, open := parseUFWStatus(output, 7443)
	if !active || !open {
		t.Fatalf("应识别出 ufw 已启用且 7443 已放行：active=%v open=%v", active, open)
	}
	_, open = parseUFWStatus(output, 9443)
	if open {
		t.Fatal("未放行的端口不应判定为已放行")
	}
}

func TestParseUFWStatusInactive(t *testing.T) {
	active, open := parseUFWStatus("Status: inactive\n", 7443)
	if active || open {
		t.Fatalf("inactive 时不应判定为启用：active=%v open=%v", active, open)
	}
}

func TestCandidateHTTPSPorts(t *testing.T) {
	if got := candidateHTTPSPorts(9443); len(got) != 1 || got[0] != 9443 {
		t.Fatalf("显式端口应只返回该端口：%v", got)
	}
	got := candidateHTTPSPorts(0)
	if len(got) != 3 || got[0] != 7443 || got[1] != 8443 || got[2] != 9443 {
		t.Fatalf("自动模式应返回 7443/8443/9443：%v", got)
	}
}
