package frp

import "testing"

func TestSummarizeRouteHealth(t *testing.T) {
	items := []RouteHealthCandidate{
		{DNSOK: true, CertOK: true, TunnelOK: true, TunnelKnown: true, ServiceOK: true},
		{DNSOK: true, CertOK: false, TunnelOK: true, TunnelKnown: true, ServiceOK: false},
		{DNSOK: false, CertOK: true, TunnelOK: false, TunnelKnown: true, ServiceOK: true},
		// frps 管理接口不可用：隧道状态判不出来，既不算通也不是不通
		{DNSOK: true, CertOK: true, ServiceOK: true},
	}
	summary := summarizeRouteHealth(2, []string{"vps-b"}, items)

	if summary.Servers != 2 || summary.Routes != 4 || summary.Healthy != 1 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	if summary.DNSFailed != 1 || summary.CertMissing != 1 || summary.TunnelDown != 1 ||
		summary.TunnelUnknown != 1 || summary.ServiceDown != 1 {
		t.Fatalf("unexpected segment counts: %+v", summary)
	}
	if len(summary.FailedServers) != 1 || summary.FailedServers[0] != "vps-b" {
		t.Fatalf("unexpected failed servers: %+v", summary.FailedServers)
	}
}

func TestSummarizeRouteHealthEmpty(t *testing.T) {
	summary := summarizeRouteHealth(0, nil, nil)
	if summary.Servers != 0 || summary.Routes != 0 || summary.Healthy != 0 {
		t.Fatalf("expected empty summary, got %+v", summary)
	}
	if len(summary.FailedServers) != 0 {
		t.Fatalf("expected no failed servers, got %+v", summary.FailedServers)
	}
}
