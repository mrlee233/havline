package settings

// Kind 描述一个设置项的值类型，决定校验方式与默认值的解析方式。
type Kind string

const (
	// KindString 任意文本（域名、邮箱、URL、路径等）
	KindString Kind = "string"
	// KindBool 布尔值，接受 1/true/yes/on
	KindBool Kind = "bool"
	// KindInt 整数
	KindInt Kind = "int"
	// KindJSON JSON 文本（对象或数组）
	KindJSON Kind = "json"
)

// Spec 是一条设置项的完整声明。
//
// 引入它的原因：设置项此前分散在四个地方——常量、Defaults 默认值表、
// settings_handler 的白名单、前端表单。新增一项时很容易只改其中两三处，
// 于是出现「界面能改、后端没人读」的黑洞设置项（theme / timezone 就是这样来的）。
// 现在白名单、默认值、敏感标记、校验方式都从这张表派生，漏写会被测试挡住。
type Spec struct {
	// Key 数据库里的键名（用 Key* 常量赋值，避免拼写漂移）
	Key string
	// Kind 值类型
	Kind Kind
	// Default 默认值；空串表示无默认值
	Default string
	// Writable 是否允许通过 PUT /api/settings 写入。
	// 程序内部维护的状态（如 frp_resume_on_start、cidr 更新时间）应为 false。
	Writable bool
	// Secret 是否为敏感值：读接口会以掩码返回，绝不回明文
	Secret bool
	// Consumer 说明「谁在读这个键」，用于人工评审时确认不是黑洞设置。
	// 约定写包名或功能名，不写具体行号（会漂移）。
	Consumer string
}

// Specs 是所有设置项的唯一清单。新增设置项必须在这里登记，
// 否则既不能通过 API 写入，也不会被 Defaults / 白名单识别。
var Specs = []Spec{
	// ---- 常规 ----
	{Key: KeyTheme, Kind: KindString, Default: "system", Writable: true, Consumer: "web:useTheme（启动时按设置初始化，切换时回写）"},
	{Key: KeyTimezone, Kind: KindString, Default: "Asia/Shanghai", Writable: true, Consumer: "app（启动时 time.Local）+ notify（告警时间）"},
	{Key: KeyLogRetentionDays, Kind: KindInt, Default: "30", Writable: true, Consumer: "app.rotateNginxLogs（归档保留天数）"},

	// ---- DDNS / 证书 ----
	{Key: KeyDDNSCheckInterval, Kind: KindInt, Default: "5", Writable: true, Consumer: "app（ddns 任务间隔）"},
	{Key: KeyCertRenewThreshold, Kind: KindInt, Default: "30", Writable: true, Consumer: "acme（续期阈值）"},
	{Key: KeyRootDomain, Kind: KindString, Writable: true, Consumer: "（保留键，当前无读取点）"},
	{Key: KeyACMEEmail, Kind: KindString, Writable: true, Consumer: "acme（注册邮箱）"},
	{Key: KeyACMECA, Kind: KindString, Default: "letsencrypt", Writable: true, Consumer: "acme（CA 选择）"},
	{Key: KeyACMEDNSResolvers, Kind: KindString, Writable: true, Consumer: "acme（DNS-01 传播检查）"},
	{Key: KeyZeroSSLAPIKey, Kind: KindString, Writable: true, Secret: true, Consumer: "acme（ZeroSSL EAB 换发）"},
	{Key: KeyLiteSSLEABKid, Kind: KindString, Writable: true, Consumer: "acme（LiteSSL EAB kid）"},
	{Key: KeyLiteSSLEABHMAC, Kind: KindString, Writable: true, Secret: true, Consumer: "acme（LiteSSL EAB hmac）"},

	// ---- 通知 ----
	{Key: KeyNotifyType, Kind: KindString, Writable: true, Consumer: "notify（渠道选择）"},
	{Key: KeyNotifyEmailJSON, Kind: KindJSON, Writable: true, Consumer: "notify（SMTP 配置）"},
	{Key: KeyNotifySMTPPassword, Kind: KindString, Writable: true, Secret: true, Consumer: "notify（SMTP 密码）"},
	{Key: KeyNotifyWebhookJSON, Kind: KindJSON, Writable: true, Consumer: "notify（Webhook 配置）"},
	{Key: KeyNotifyWebhookSecret, Kind: KindString, Writable: true, Secret: true, Consumer: "notify（Webhook 签名密钥）"},
	{Key: KeyNotifyTelegramJSON, Kind: KindJSON, Writable: true, Consumer: "notify（Telegram 配置）"},
	{Key: KeyNotifyTelegramToken, Kind: KindString, Writable: true, Secret: true, Consumer: "notify（Bot Token）"},
	{Key: KeyNotifyWebhookURL, Kind: KindString, Writable: true, Consumer: "notify（自定义 Webhook 地址）"},

	// ---- 通知事件开关 ----
	{Key: KeyNotifyOnDDNSIPChange, Kind: KindBool, Writable: true, Consumer: "notify（DDNS IP 变化事件）"},
	{Key: KeyNotifyOnDDNSFailure, Kind: KindBool, Writable: true, Consumer: "notify（DDNS 失败事件）"},
	{Key: KeyNotifyOnCertExpiry, Kind: KindBool, Writable: true, Consumer: "notify（证书临期事件）"},
	{Key: KeyNotifyOnCertRenewSuccess, Kind: KindBool, Writable: true, Consumer: "notify（证书续期成功）"},
	{Key: KeyNotifyOnCertRenewFailure, Kind: KindBool, Writable: true, Consumer: "notify（证书续期失败）"},
	{Key: KeyNotifyOnIPFrequentAccess, Kind: KindBool, Writable: true, Consumer: "notify（频繁访问事件）"},
	{Key: KeyNotifyOnLoginFailure, Kind: KindBool, Writable: true, Consumer: "notify（登录失败事件）"},
	{Key: KeyNotifyOnNginxReloadFailure, Kind: KindBool, Writable: true, Consumer: "notify（Nginx 重载失败）"},
	{Key: KeyNotifyOnResourceThreshold, Kind: KindBool, Writable: true, Consumer: "notify（资源超阈值）"},
	{Key: KeyNotifyOnTunnelDown, Kind: KindBool, Writable: true, Consumer: "notify（隧道中断）"},
	{Key: KeyNotifyOnFrpsDown, Kind: KindBool, Writable: true, Consumer: "notify（frps 不可用）"},
	{Key: KeyNotifyOnAgentUnreachable, Kind: KindBool, Writable: true, Consumer: "notify（Agent 不可达）"},
	{Key: KeyNotifyOnRouteDrift, Kind: KindBool, Writable: true, Consumer: "notify（路由漂移）"},
	{Key: KeyNotifyOnBackupFailure, Kind: KindBool, Writable: true, Consumer: "notify（备份失败）"},

	// ---- 通知阈值 ----
	{Key: KeyNotifyIPFrequentThreshold, Kind: KindInt, Writable: true, Consumer: "notify（频繁访问阈值）"},
	{Key: KeyNotifyIPFrequentWindowSec, Kind: KindInt, Writable: true, Consumer: "notify（频繁访问窗口）"},
	{Key: KeyNotifyLoginFailureThreshold, Kind: KindInt, Writable: true, Consumer: "notify（登录失败次数阈值）"},
	{Key: KeyNotifyLoginFailureWindowSec, Kind: KindInt, Writable: true, Consumer: "notify（登录失败窗口）"},
	{Key: KeyNotifyCPUThreshold, Kind: KindInt, Writable: true, Consumer: "monitor（CPU 阈值）"},
	{Key: KeyNotifyMemoryThreshold, Kind: KindInt, Writable: true, Consumer: "monitor（内存阈值）"},
	{Key: KeyNotifyDiskThreshold, Kind: KindInt, Writable: true, Consumer: "monitor（磁盘阈值）"},
	{Key: KeyNotifyTunnelFailThreshold, Kind: KindInt, Writable: true, Consumer: "monitor（隧道失败次数）"},
	{Key: KeyNotifyFrpsFailThreshold, Kind: KindInt, Writable: true, Consumer: "monitor（frps 失败次数）"},
	{Key: KeyNotifyAgentFailThreshold, Kind: KindInt, Writable: true, Consumer: "monitor（Agent 失败次数）"},
	{Key: KeyNotifyDriftCooldownHours, Kind: KindInt, Writable: true, Consumer: "monitor（漂移告警冷却）"},

	// ---- 备份 ----
	{Key: KeyBackupAutoEnabled, Kind: KindBool, Writable: true, Consumer: "backup.AutoRunner（自动备份开关）"},
	{Key: KeyBackupKeepCount, Kind: KindInt, Writable: true, Consumer: "backup.KeepCount（保留份数）"},

	// ---- 功能开关 ----
	{Key: KeyRouteHealthEnabled, Kind: KindBool, Writable: true, Consumer: "monitor（路由健康采集）"},
	{Key: KeyStatusPageEnabled, Kind: KindBool, Writable: true, Consumer: "api.status_page（公开状态页开关）"},
	{Key: KeyStatusPageCode, Kind: KindString, Writable: true, Consumer: "api.status_page（明文只在保存请求里出现，落库为 hash）"},
	{Key: KeyMetricsHistoryEnabled, Kind: KindBool, Writable: true, Consumer: "monitor（指标历史采样）"},
	{Key: KeyTerminalEnabled, Kind: KindBool, Writable: true, Consumer: "api.terminal（SSH 终端总开关）"},
	{Key: KeyTerminalRecording, Kind: KindBool, Writable: true, Consumer: "api.terminal（会话录制开关）"},

	// ---- 安全 ----
	{Key: KeyTrustedProxy, Kind: KindJSON, Writable: true, Consumer: "nginx（real_ip 信任链）"},
	{Key: KeyGlobalIPBlacklist, Kind: KindJSON, Writable: true, Consumer: "nginx（全局黑名单）"},
	{Key: KeyGlobalIPWhitelist, Kind: KindJSON, Writable: true, Consumer: "nginx（全局白名单）"},

	// ---- 中国 CIDR ----
	{Key: KeyChinaCIDRUpdateHours, Kind: KindInt, Default: "24", Writable: true, Consumer: "app（cidr 更新间隔）"},
	{Key: KeyChinaCIDRSourceV4, Kind: KindString, Writable: true, Consumer: "chinacidr（IPv4 源地址）"},
	{Key: KeyChinaCIDRSourceV6, Kind: KindString, Writable: true, Consumer: "chinacidr（IPv6 源地址）"},

	// ---- 内部状态：程序自己维护，不接受 API 写入 ----
	{Key: KeyStatusPageCodeHash, Kind: KindString, Secret: true, Consumer: "api.status_page（访问码哈希）"},
	{Key: KeyNginxGlobalMode, Kind: KindString, Default: "auto", Consumer: "service.nginx_config（auto/custom 模式）"},
	{Key: KeyChinaCIDRUpdatedAt, Kind: KindString, Consumer: "chinacidr（上次更新时间）"},
	{Key: KeyChinaCIDRCountV4, Kind: KindInt, Consumer: "chinacidr（IPv4 条目数）"},
	{Key: KeyChinaCIDRCountV6, Kind: KindInt, Consumer: "chinacidr（IPv6 条目数）"},
	{Key: KeyChinaCIDRLastError, Kind: KindString, Consumer: "chinacidr（最近错误）"},
	{Key: KeyFrpResumeOnStart, Kind: KindBool, Consumer: "frp（重启后是否恢复 frpc）"},

	// ---- 历史键：已废弃，仅为兼容旧库保留声明 ----
	{Key: KeyNotifyOnDDNSError, Kind: KindBool, Consumer: "（历史键，已迁移到 KeyNotifyOnDDNSFailure）"},
	{Key: KeyNotifyOnCertError, Kind: KindBool, Consumer: "（历史键，已迁移到 KeyNotifyOnCertExpiry）"},
	{Key: KeyNotifyOnNginxError, Kind: KindBool, Consumer: "（历史键，已忽略）"},
	{Key: KeyFRPEnabled, Kind: KindBool, Consumer: "（历史键，旧单服务端 FRP，已被 internal/frp 取代）"},
	{Key: KeyFRPServerAddr, Kind: KindString, Consumer: "（历史键，旧单服务端 FRP）"},
	{Key: KeyFRPServerPort, Kind: KindInt, Consumer: "（历史键，旧单服务端 FRP）"},
	{Key: KeyFRPAuthToken, Kind: KindString, Secret: true, Consumer: "（历史键，旧单服务端 FRP）"},
	{Key: KeyFRPTLSEnabled, Kind: KindBool, Consumer: "（历史键，旧单服务端 FRP）"},
	{Key: KeyFRPCustomDomains, Kind: KindString, Consumer: "（历史键，旧单服务端 FRP）"},
	{Key: KeyFRPLastError, Kind: KindString, Consumer: "（历史键，旧单服务端 FRP）"},
	{Key: KeyFRPStartedAt, Kind: KindString, Consumer: "（历史键，旧单服务端 FRP）"},
}

// specIndex 是 Specs 的索引，供 O(1) 查找。
var specIndex = func() map[string]Spec {
	out := make(map[string]Spec, len(Specs))
	for _, spec := range Specs {
		out[spec.Key] = spec
	}
	return out
}()

// Defaults 是从 Specs 派生的默认值表（只含声明了 Default 的项）。
var Defaults = func() map[string]string {
	out := make(map[string]string, len(Specs))
	for _, spec := range Specs {
		if spec.Default != "" {
			out[spec.Key] = spec.Default
		}
	}
	return out
}()

// Lookup 返回某个键的声明。
func Lookup(key string) (Spec, bool) {
	spec, ok := specIndex[key]
	return spec, ok
}

// WritableKeys 返回所有允许 API 写入的键，用于 settings_handler 的白名单。
func WritableKeys() map[string]bool {
	out := make(map[string]bool, len(Specs))
	for _, spec := range Specs {
		if spec.Writable {
			out[spec.Key] = true
		}
	}
	return out
}
