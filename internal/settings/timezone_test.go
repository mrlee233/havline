package settings

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/havline/havline/internal/db"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "havline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	return NewStore(conn)
}

// TestApplyTimezoneSetsLocal：设置里的时区必须真的作用到 time.Local，
// 否则「改了时区但通知时间不变」这个黑洞会一直存在。
func TestApplyTimezoneSetsLocal(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })

	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Set(ctx, KeyTimezone, "Asia/Tokyo"); err != nil {
		t.Fatal(err)
	}

	loc, err := ApplyTimezone(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if loc.String() != "Asia/Tokyo" {
		t.Fatalf("应返回 Asia/Tokyo，实际 %s", loc)
	}
	if time.Local.String() != "Asia/Tokyo" {
		t.Fatalf("time.Local 应被设为 Asia/Tokyo，实际 %s", time.Local)
	}
	// 同一个瞬时在东京比上海早一小时
	_, offset := time.Now().In(time.Local).Zone()
	if offset != 9*3600 {
		t.Fatalf("东京时区偏移应为 +9h，实际 %d 秒", offset)
	}
}

// TestApplyTimezoneFallsBackToDefault：没写过设置时用注册表里的默认值。
func TestApplyTimezoneFallsBackToDefault(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })

	store := newTestStore(t)
	loc, err := ApplyTimezone(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	if loc.String() != Defaults[KeyTimezone] {
		t.Fatalf("应使用默认时区 %q，实际 %s", Defaults[KeyTimezone], loc)
	}
}

// TestLoadTimezoneRejectsInvalid：非法时区要给出可读的中文报错，
// 而不是把 "" 或 UTC 悄悄当成合法值写库。
func TestLoadTimezoneRejectsInvalid(t *testing.T) {
	cases := []string{"", "  ", "Mars/Olympus", "Asia/Shanghai; rm -rf /"}
	for _, input := range cases {
		if _, err := LoadTimezone(input); err == nil {
			t.Fatalf("%q 应被拒绝", input)
		}
	}
	if _, err := LoadTimezone("UTC"); err != nil {
		t.Fatalf("UTC 应合法：%v", err)
	}
}

// TestLoadTimezoneErrorMentionsName：报错里要带上是哪个值不合法，便于前端提示。
func TestLoadTimezoneErrorMentionsName(t *testing.T) {
	_, err := LoadTimezone("Bad/Zone")
	if err == nil {
		t.Fatal("非法时区应报错")
	}
	if !strings.Contains(err.Error(), "Bad/Zone") {
		t.Fatalf("报错应包含原始输入，实际：%v", err)
	}
}
