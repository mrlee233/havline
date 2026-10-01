package app

import (
	"testing"

	"github.com/havline/havline/internal/frp"
	"github.com/havline/havline/internal/monitor"
)

// monitor 与 frp 两侧的状态字面量必须一致，落库前由 healthStateValue 翻译一次；
// 这个测试把翻译钉住，避免以后改一侧的常量而另一侧悄悄写错值。
func TestHealthStateValue(t *testing.T) {
	cases := map[string]string{
		monitor.HealthUp:      frp.RouteHealthUp,
		monitor.HealthDown:    frp.RouteHealthDown,
		monitor.HealthUnknown: frp.RouteHealthUnknown,
		"其他":                  frp.RouteHealthUnknown,
	}
	for input, want := range cases {
		if got := healthStateValue(input); got != want {
			t.Fatalf("%q 应翻译为 %q，实际 %q", input, want, got)
		}
	}
}
