package settings

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ApplyTimezone 读取「设置 → 常规 → 时区」并设为进程默认时区（time.Local）。
//
// 为什么必须在这里接一下：Go 的 time.Local 只由 TZ 环境变量决定，而设置页的
// timezone 字段此前完全没人读——用户在界面上改成 Asia/Tokyo，通知里的时间戳
// 依旧是容器 TZ。两者取其一都可以，但必须真的生效。
//
// 返回值是最终生效的时区，供启动日志与调用方确认。
func ApplyTimezone(ctx context.Context, store *Store) (*time.Location, error) {
	raw, err := store.Get(ctx, KeyTimezone)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(raw)
	if name == "" {
		name = Defaults[KeyTimezone]
	}
	loc, err := LoadTimezone(name)
	if err != nil {
		return nil, err
	}
	time.Local = loc
	return loc, nil
}

// LoadTimezone 解析时区名，并对常见拼写提供明确的中文报错。
func LoadTimezone(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("时区不能为空")
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("无效的时区 %q（应形如 Asia/Shanghai）", name)
	}
	return loc, nil
}
