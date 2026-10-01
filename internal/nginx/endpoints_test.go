package nginx

import "testing"

func TestParseServerEndpoints(t *testing.T) {
	content := `server {
    listen 8011;
    listen [::]:8011;
    server_name a.com b.com;
    location / {
        proxy_pass http://127.0.0.1:8080;
    }
}

# 注释行不参与解析
server {
    listen 127.0.0.1:8017 ssl;
    server_name *.c.com ~^regex\.com$ _;
}
`
	endpoints := ParseServerEndpoints(content)
	want := []struct {
		host string
		port int
	}{
		{"a.com", 8011},
		{"b.com", 8011},
		{"*.c.com", 8017},
	}
	if len(endpoints) != len(want) {
		t.Fatalf("expected %d endpoints, got %d (%+v)", len(want), len(endpoints), endpoints)
	}
	for i, expect := range want {
		if endpoints[i].Hostname != expect.host || endpoints[i].Port != expect.port {
			t.Fatalf("endpoint %d = %s:%d, want %s:%d", i, endpoints[i].Hostname, endpoints[i].Port, expect.host, expect.port)
		}
	}
}

func TestParseServerEndpointsDefaultsToPort80(t *testing.T) {
	endpoints := ParseServerEndpoints("server {\n    server_name d.com;\n    return 444;\n}\n")
	if len(endpoints) != 1 || endpoints[0].Hostname != "d.com" || endpoints[0].Port != 80 {
		t.Fatalf("expected d.com:80, got %+v", endpoints)
	}
}

func TestParseServerEndpointsIgnoresUnparseable(t *testing.T) {
	content := `include /etc/nginx/snippets/extra.conf;
server {
    listen unix:/run/havline.sock;
    server_name $host;
}
`
	if endpoints := ParseServerEndpoints(content); len(endpoints) != 0 {
		t.Fatalf("expected no endpoints, got %+v", endpoints)
	}
}
