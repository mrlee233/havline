package proxy

import (
	"context"
	"testing"
)

func TestStorePersistsExitsAndFiltersLocal(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	rule, err := store.Create(ctx, CreateInput{
		Upstream:   "http://192.168.1.10:8080",
		ListenPort: 8443,
		ListenIPv4: true,
		Hosts:      []string{"cf.example.com"},
		Enabled:    true,
		Exits:      []string{ExitCloudflare},
		CFTunnelID: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rule.CFTunnelID != 7 || len(rule.Exits) != 1 || rule.Exits[0] != ExitCloudflare {
		t.Fatalf("Cloudflare 出口未持久化：%#v", rule)
	}
	localRules, err := store.ListEnabledLocal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(localRules) != 0 {
		t.Fatalf("仅 Cloudflare 规则不应生成本机 Nginx：%#v", localRules)
	}

	enabled := true
	updated, err := store.Update(ctx, rule.ID, UpdateInput{
		Exits:      &[]string{ExitLocal, ExitCloudflare},
		Enabled:    &enabled,
		CFTunnelID: int64Ptr(7),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Exits) != 2 {
		t.Fatalf("本机 + Cloudflare 出口未保存：%#v", updated.Exits)
	}
	localRules, err = store.ListEnabledLocal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(localRules) != 1 || localRules[0].ID != rule.ID {
		t.Fatalf("启用本机出口后应进入 Nginx 规则：%#v", localRules)
	}
}

func int64Ptr(v int64) *int64 { return &v }
