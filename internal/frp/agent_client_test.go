package frp

import "testing"

func TestNormalizeFRPSActionResult(t *testing.T) {
	if _, err := normalizeFRPSActionResult(nil); err == nil {
		t.Fatal("空响应应返回错误")
	}
	if _, err := normalizeFRPSActionResult(map[string]any{}); err == nil {
		t.Fatal("缺少 ok 字段应返回错误")
	}

	failed, err := normalizeFRPSActionResult(map[string]any{
		"ok":    false,
		"error": "systemctl start 已执行但 frps 未进入运行状态",
	})
	if err != nil {
		t.Fatalf("ok=false 应保留给前端展示，got %v", err)
	}
	if failed["ok"] != false || failed["error"] == "" {
		t.Fatalf("失败结果未保留：%+v", failed)
	}

	failedWithoutMessage, err := normalizeFRPSActionResult(map[string]any{"ok": false})
	if err != nil {
		t.Fatalf("ok=false 应补齐默认错误，got %v", err)
	}
	if failedWithoutMessage["error"] != "frps 未进入运行状态，请查看 frps 日志" {
		t.Fatalf("默认错误不正确：%+v", failedWithoutMessage)
	}

	success, err := normalizeFRPSActionResult(map[string]any{"ok": true, "action": "start"})
	if err != nil || success["ok"] != true {
		t.Fatalf("成功结果解析失败：result=%+v err=%v", success, err)
	}
}
