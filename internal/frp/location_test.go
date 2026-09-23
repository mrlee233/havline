package frp

import (
	"net"
	"testing"
)

func TestFormatLocation(t *testing.T) {
	value := formatLocation(ipLocationResponse{City: "Hong Kong", Region: "Hong Kong", Country: "China"})
	if value != "Hong Kong · China" {
		t.Fatalf("位置格式错误：%s", value)
	}
}

func TestIsPublicIP(t *testing.T) {
	if !isPublicIP(net.ParseIP("38.95.74.87")) {
		t.Fatal("公网 IP 应被识别")
	}
	if isPublicIP(net.ParseIP("192.168.1.1")) {
		t.Fatal("私网 IP 不应被定位")
	}
}
