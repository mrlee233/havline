package acme

import "testing"

func TestParseResolverList(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"带端口", "223.5.5.5:53, 1.1.1.1:53", []string{"223.5.5.5:53", "1.1.1.1:53"}},
		{"缺端口补 53", "8.8.8.8\n9.9.9.9", []string{"8.8.8.8:53", "9.9.9.9:53"}},
		{"分号与空格", "223.5.5.5;8.8.4.4 1.0.0.1", []string{"223.5.5.5:53", "8.8.4.4:53", "1.0.0.1:53"}},
		{"空值", "   ", nil},
		{"只留空白项", " , ; \n", nil},
	}
	for _, item := range cases {
		got := parseResolverList(item.raw)
		if len(got) != len(item.want) {
			t.Fatalf("%s: 期望 %v，实际 %v", item.name, item.want, got)
		}
		for i := range got {
			if got[i] != item.want[i] {
				t.Fatalf("%s: 期望 %v，实际 %v", item.name, item.want, got)
			}
		}
	}
}

func TestParseResolverListCapsEntries(t *testing.T) {
	raw := "1.1.1.1,1.1.1.2,1.1.1.3,1.1.1.4,1.1.1.5,1.1.1.6,1.1.1.7,1.1.1.8,1.1.1.9,1.1.1.10"
	if got := parseResolverList(raw); len(got) != 8 {
		t.Fatalf("最多取前 8 条，实际 %d：%v", len(got), got)
	}
}

// 内置的默认值必须是 lego 能直接用的 host:port 形式
func TestDefaultResolversAreUsable(t *testing.T) {
	got := parseResolverList(defaultACMEDNSResolvers)
	if len(got) == 0 {
		t.Fatalf("默认解析器解析为空：%q", defaultACMEDNSResolvers)
	}
	if got[0] != "223.5.5.5:53" {
		t.Fatalf("默认解析器首项应带端口，实际 %q", got[0])
	}
}
