package frp

import "testing"

func TestExtractAgentVersion(t *testing.T) {
	output := "agent version: 0.15.1\nhavline-agent installed\nToken: abc123"
	if got := extractAgentVersion(output); got != "0.15.1" {
		t.Fatalf("版本解析失败，got %q", got)
	}
	if got := extractAgentVersion("havline-agent installed\nToken: abc123"); got != "" {
		t.Fatalf("无版本行时应返回空串，got %q", got)
	}
}
