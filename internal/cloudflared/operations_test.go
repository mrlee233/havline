package cloudflared

import "testing"

func TestCompareIngressDetectsMissingAndMismatch(t *testing.T) {
	expected := BuildIngress([]string{"a.example.com", "b.example.com"}, "http://127.0.0.1:8080")
	actual := []IngressRule{
		{Hostname: "a.example.com", Service: "http://127.0.0.1:9090"},
		{Hostname: "c.example.com", Service: "http://127.0.0.1:8080"},
		{Service: "http_status:404"},
	}
	items := compareIngress(expected, actual)
	if len(items) != 3 {
		t.Fatalf("应检测出 3 项漂移，实际 %#v", items)
	}
}

func TestCompareIngressIgnoresOrder(t *testing.T) {
	expected := BuildIngress([]string{"b.example.com", "a.example.com"}, "http://127.0.0.1:8080")
	actual := []IngressRule{
		{Service: "http_status:404"},
		{Hostname: "a.example.com", Service: "http://127.0.0.1:8080"},
		{Hostname: "b.example.com", Service: "http://127.0.0.1:8080"},
	}
	if items := compareIngress(expected, actual); len(items) != 0 {
		t.Fatalf("顺序不同不应算漂移：%#v", items)
	}
}
