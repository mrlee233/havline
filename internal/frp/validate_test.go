package frp

import "testing"

func TestValidateProxyInput(t *testing.T) {
	port := 60022
	cases := []struct {
		name    string
		input   ProxyInput
		wantErr bool
	}{
		{name: "tcp", input: ProxyInput{Name: "nas_ssh", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &port}},
		{name: "http", input: ProxyInput{Name: "nas-web", Type: "http", LocalIP: "127.0.0.1", LocalPort: 18080, CustomDomains: []string{"nas.example.com"}}},
		{name: "missing remote port", input: ProxyInput{Name: "nas", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22}, wantErr: true},
		{name: "invalid name", input: ProxyInput{Name: "bad name", Type: "http", LocalIP: "127.0.0.1", LocalPort: 80, CustomDomains: []string{"nas.example.com"}}, wantErr: true},
		{name: "invalid domain", input: ProxyInput{Name: "nas", Type: "http", LocalIP: "127.0.0.1", LocalPort: 80, CustomDomains: []string{"bad domain"}}, wantErr: true},
		{name: "udp", input: ProxyInput{Name: "dns", Type: "udp", LocalIP: "127.0.0.1", LocalPort: 53, RemotePort: &port}},
		{name: "stcp", input: ProxyInput{Name: "secret", Type: "stcp", LocalIP: "127.0.0.1", LocalPort: 22, Options: ProxyOptions{SecretKey: "abc"}}},
		{name: "tcpmux missing multiplexer", input: ProxyInput{Name: "mux", Type: "tcpmux", LocalIP: "127.0.0.1", LocalPort: 80}, wantErr: true},
		{name: "invalid bandwidth mode", input: ProxyInput{Name: "nas", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &port, Options: ProxyOptions{Transport: ProxyTransportOptions{BandwidthLimitMode: "wrong"}}}, wantErr: true},
		{name: "udp remote port", input: ProxyInput{Name: "dns2", Type: "udp", LocalIP: "127.0.0.1", LocalPort: 53, RemotePort: &port}},
		{name: "xtcp missing secret key", input: ProxyInput{Name: "p2p", Type: "xtcp", LocalIP: "127.0.0.1", LocalPort: 22}, wantErr: true},
		{name: "plugin http2https missing local addr", input: ProxyInput{Name: "p1", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &port, Options: ProxyOptions{Plugin: &ProxyPluginOptions{Type: "https2http", CRTPath: "cert.pem", KeyPath: "key.pem"}}}, wantErr: true},
		{name: "plugin https2http missing cert", input: ProxyInput{Name: "p2", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &port, Options: ProxyOptions{Plugin: &ProxyPluginOptions{Type: "https2http", LocalAddr: "127.0.0.1:80"}}}, wantErr: true},
		{name: "plugin socks5 optional auth", input: ProxyInput{Name: "p3", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &port, Options: ProxyOptions{Plugin: &ProxyPluginOptions{Type: "socks5"}}}},
		{name: "plugin unix_domain_socket missing path", input: ProxyInput{Name: "p4", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &port, Options: ProxyOptions{Plugin: &ProxyPluginOptions{Type: "unix_domain_socket"}}}, wantErr: true},
		{name: "plugin static_file missing local path", input: ProxyInput{Name: "p5", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &port, Options: ProxyOptions{Plugin: &ProxyPluginOptions{Type: "static_file"}}}, wantErr: true},
		{name: "plugin unknown type", input: ProxyInput{Name: "p6", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &port, Options: ProxyOptions{Plugin: &ProxyPluginOptions{Type: "whatever"}}}, wantErr: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateProxyInput(testCase.input)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("validateProxyInput() error = %v, wantErr %v", err, testCase.wantErr)
			}
			// 对外导出的校验入口应与内部入口行为一致（关系表模式校验复用此函数）
			if (ValidateProxy(testCase.input) != nil) != testCase.wantErr {
				t.Fatalf("ValidateProxy() error = %v, wantErr %v", ValidateProxy(testCase.input), testCase.wantErr)
			}
		})
	}
}
