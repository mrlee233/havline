package agent

import "testing"

func TestValidateListenAddr(t *testing.T) {
	cases := []struct {
		addr        string
		allowRemote bool
		loopback    bool
		wantErr     bool
	}{
		{"127.0.0.1:7700", false, true, false},
		{"localhost:7700", false, true, false},
		{"[::1]:7700", false, true, false},
		{":7700", false, false, true},
		{"0.0.0.0:7700", false, false, true},
		{"0.0.0.0:7700", true, false, false},
		{"bad-address", false, false, true},
	}
	for _, tc := range cases {
		loopback, err := validateListenAddr(tc.addr, tc.allowRemote)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s 应拒绝", tc.addr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s 不应报错: %v", tc.addr, err)
		}
		if loopback != tc.loopback {
			t.Fatalf("%s loopback=%v, want %v", tc.addr, loopback, tc.loopback)
		}
	}
}
