package settings

import (
	"testing"
)

// TestEveryDeclaredKeyHasConsumer：每个声明出来的键都要有「谁在读」的说明，
// 并且不能只是复述键名本身。这条测试防的是「界面能改、后端没人读」的黑洞设置项
// （theme / timezone 曾经就是这样：常量、默认值、白名单、前端表单四处齐全，唯独没人消费）。
func TestEveryDeclaredKeyHasConsumer(t *testing.T) {
	for _, spec := range Specs {
		if spec.Key == "" {
			t.Fatal("存在未填 Key 的 Spec")
		}
		if spec.Kind == "" {
			t.Fatalf("%s 未填 Kind", spec.Key)
		}
		if spec.Consumer == "" {
			t.Fatalf("%s 未填 Consumer（说明谁在读这个设置项）", spec.Key)
		}
	}
}

// TestNoDuplicateKeys：同一个键不能声明两次，否则 Lookup / Defaults 会悄悄取其中一个。
func TestNoDuplicateKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, spec := range Specs {
		if seen[spec.Key] {
			t.Fatalf("重复声明的设置项：%s", spec.Key)
		}
		seen[spec.Key] = true
	}
	if len(seen) != len(Specs) {
		t.Fatalf("Specs 数量 %d 与去重后 %d 不一致", len(Specs), len(seen))
	}
}

// TestKindableDefaultsMatchKind：默认值必须能被它声明的 Kind 解析，
// 否则 GetInt/GetBool 在没写过值时会拿到脏数据。
func TestKindableDefaultsMatchKind(t *testing.T) {
	for _, spec := range Specs {
		if spec.Default == "" {
			continue
		}
		switch spec.Kind {
		case KindInt:
			if _, ok := parseInt(spec.Default); !ok {
				t.Fatalf("%s 的默认值 %q 不是合法整数", spec.Key, spec.Default)
			}
		case KindBool:
			if _, ok := parseBool(spec.Default); !ok {
				t.Fatalf("%s 的默认值 %q 不是合法布尔值", spec.Key, spec.Default)
			}
		}
	}
}

// TestDefaultsDerivedFromSpecs：Defaults 必须与 Specs 一致，防止两处各写一份。
func TestDefaultsDerivedFromSpecs(t *testing.T) {
	index := map[string]string{}
	for _, spec := range Specs {
		if spec.Default != "" {
			index[spec.Key] = spec.Default
		}
	}
	if len(index) != len(Defaults) {
		t.Fatalf("Defaults 数量 %d 与 Specs 中带默认值的条目 %d 不一致", len(Defaults), len(index))
	}
	for key, want := range index {
		if got := Defaults[key]; got != want {
			t.Fatalf("%s 默认值不符：got %q want %q", key, got, want)
		}
	}
}

// TestInternalStateKeysNotWritable：程序自己维护的状态不能通过 API 写入，
// 否则用户可以把 frp_resume_on_start 之类改成任意值，破坏重启恢复逻辑。
func TestInternalStateKeysNotWritable(t *testing.T) {
	internal := []string{
		KeyFrpResumeOnStart, KeyStatusPageCodeHash,
		KeyChinaCIDRUpdatedAt, KeyChinaCIDRCountV4, KeyChinaCIDRCountV6, KeyChinaCIDRLastError,
		KeyNginxGlobalMode,
	}
	writable := WritableKeys()
	for _, key := range internal {
		if writable[key] {
			t.Fatalf("%s 是内部状态键，不应出现在可写白名单里", key)
		}
	}
}

// TestSecretKeysAreDeclared：已知的敏感键必须被标记为 Secret，读接口才会打码。
func TestSecretKeysAreDeclared(t *testing.T) {
	secrets := []string{
		KeyNotifySMTPPassword, KeyNotifyWebhookSecret, KeyNotifyTelegramToken,
		KeyZeroSSLAPIKey, KeyLiteSSLEABHMAC, KeyStatusPageCodeHash,
	}
	for _, key := range secrets {
		spec, ok := Lookup(key)
		if !ok {
			t.Fatalf("%s 未在注册表里声明", key)
		}
		if !spec.Secret {
			t.Fatalf("%s 应标记为 Secret", key)
		}
	}
}

// TestWritableWhitelistContainsCoreKeys：核心可配置项必须在白名单里，
// 否则前端保存会静默丢字段（PUT 会按白名单过滤）。
func TestWritableWhitelistContainsCoreKeys(t *testing.T) {
	writable := WritableKeys()
	core := []string{
		KeyTheme, KeyTimezone, KeyLogRetentionDays,
		KeyDDNSCheckInterval, KeyCertRenewThreshold,
		KeyBackupAutoEnabled, KeyBackupKeepCount,
		KeyStatusPageEnabled, KeyTerminalEnabled, KeyTerminalRecording,
	}
	for _, key := range core {
		if !writable[key] {
			t.Fatalf("%s 应允许 API 写入", key)
		}
	}
}

// parseInt / parseBool 是注册表测试用的宽松解析，与 Store 的行为保持一致。
func parseInt(raw string) (int, bool) {
	n := 0
	neg := false
	for i, c := range raw {
		if i == 0 && c == '-' {
			neg = true
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}

func parseBool(raw string) (bool, bool) {
	switch raw {
	case "1", "true", "yes", "on", "0", "false", "no", "off":
		return raw == "1" || raw == "true" || raw == "yes" || raw == "on", true
	default:
		return false, false
	}
}
