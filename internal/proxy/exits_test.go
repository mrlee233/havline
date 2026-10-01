package proxy

import "testing"

func TestNormalizeExitsDefaultsToLocal(t *testing.T) {
	exits, tunnelID, err := normalizeExits(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if tunnelID != 0 || len(exits) != 1 || exits[0] != ExitLocal {
		t.Fatalf("默认出口应为 local：%#v tunnel=%d", exits, tunnelID)
	}
}

func TestNormalizeExitsRequiresTunnelForCloudflare(t *testing.T) {
	if _, _, err := normalizeExits([]string{ExitCloudflare}, 0); err == nil {
		t.Fatal("Cloudflare 出口缺少隧道时应报错")
	}
}

func TestNormalizeExitsDeduplicatesAndSorts(t *testing.T) {
	exits, tunnelID, err := normalizeExits([]string{ExitCloudflare, ExitLocal, ExitCloudflare}, 12)
	if err != nil {
		t.Fatal(err)
	}
	if tunnelID != 12 || len(exits) != 2 || exits[0] != ExitCloudflare || exits[1] != ExitLocal {
		t.Fatalf("出口去重排序不符：%#v tunnel=%d", exits, tunnelID)
	}
}

func TestRuleUsesExitTreatsEmptyAsLocal(t *testing.T) {
	rule := Rule{}
	if !rule.UsesExit(ExitLocal) {
		t.Fatal("空出口应兼容为本机出口")
	}
	if rule.UsesExit(ExitCloudflare) {
		t.Fatal("空出口不应被视为 Cloudflare")
	}
}
