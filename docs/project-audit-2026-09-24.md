# Fonu 全项目审查报告

> 审查日期：2026-09-24　对象：`f:\Fonu-main\Fonu-1.0.2`（VERSION 1.2.6；Go 后端 `cmd/` + `internal/` 202 个 `.go`，前端 `web/src` 44 个源文件）
> 方法：三路并行静态审查（Go 后端 / 前端 / 工程与部署）+ 高信号 grep 统计。
> 可信度说明：§1.1、§5.1 的高危项已逐条读取原文核对；其余结论来自 grep 与源码阅读，未做运行时验证；行号为 2026-09-24 快照，实施时以标识符为准。
> 用途：把「哪里要改、哪里要补」按优先级摊开，供逐项实施与勾选。

---

## 〇、结论速览

| 级别 | 条数 | 内容 |
| --- | --- | --- |
| **P0 · 安全** | 4 | 备份恢复的任意文件写；agent 安装 frps 的 SSRF + 二进制无校验；SSH 不校验主机密钥；三处凭据明文回传/落盘 |
| **P0 · 用户可见缺陷** | 2 | `/frp-agent` 服务端详情弹窗状态点恒灰；仪表盘 DDNS/证书卡「红灰点 + 正常」文案矛盾 |
| **P1 · 正确性** | 6 | 安装失败仍报「已启动」；备份失败仍覆盖唯一副本；限流状态损坏静默清空；请求体解析错误被忽略等 |
| **P1 · 工程** | 5 | CI 只发版不校验；三套锁文件并存；版本号双源（本地构建显示 1.0.2）；`fonu-agent.exe` 入库；前端零测试设施 |
| **P1 · 前端质量** | 4 | 死样式与重复样式块；231 处硬编码色值；轮询无 in-flight 互斥且只有仪表盘处理页面隐藏 |
| **P2 · 重构/增补** | 若干 | 5 套「安全写文件」、4 套 `.bak`、agent 与宿主机各写一份 nginx 模板；8 个包零测试；i18n 缺失 |

**已确认无问题、不必重复投入**：无命令注入（`exec.Command` 全为固定 argv）；无路由漏挂鉴权（API 97 条受保护 + 4 条公开，agent 17 条全包在 `s.auth`）；无无界 `io.ReadAll`（全配 `io.LimitReader`）；无 ticker 泄漏（两处 `NewTicker` 都有 `defer Stop`）；无 `console.log`/`debugger`；无 `v-html`；密钥落盘权限普遍 0600；前端定时器清理齐全。

---

## 一、安全

### 1.1 P0 高危（已逐条复核原文）

| 位置 | 问题 | 修法 |
| --- | --- | --- |
| `internal/api/backup_handler.go:52-53` | `tmpPath := filepath.Join(os.TempDir(), header.Filename)` 后 `os.Create` —— multipart 文件名完全可控，`../../root/.ssh/authorized_keys` 之类会逃出 `/tmp` 并被截断写入（`filepath.Join` 会 Clean 掉 `..`，不构成防护） | 改 `os.CreateTemp("", "fonu-restore-*.zip")`，或用 `filepath.Base(header.Filename)` |
| `internal/agent/server.go:257-260` | `req.Proxy` 未做任何校验就被拼成 `proxy + "/" + downloadURL` 交给 VPS 下载，随后解压安装为 `frps` 可执行文件 → SSRF + 任意二进制安装；且整条路径**不做 SHA-256 校验**（对比 `internal/frp/runtime.go:203` 主程序下载是校验的） | `req.Proxy` 走 `internal/frp/runtime.go:320 downloadProxyOptions` 白名单；下载后校验 `sha256sums` |
| `internal/frp/ssh_installer.go:42` | `HostKeyCallback: ssh.InsecureIgnoreHostKey()` —— 接受任意主机密钥，MITM 可截获该 VPS 的 root SSH 凭据 | 用 `knownhosts` 校验；或首次连接回显指纹让用户确认后落库 |
| `internal/notify/store.go:400-404` + `internal/api/settings_handler.go:31,50` | `SanitizeSettings` 只删 SMTP 密码 / Webhook Secret / Telegram Token，而 `KeyZeroSSLAPIKey` 在可写白名单里且随 `GET /api/settings` 原样返回 → **ZeroSSL API Key 明文回传浏览器** | 把 `KeyZeroSSLAPIKey` 加入 `SanitizeSettings`，改用掩码（2026-09-24 实施：只回 `MaskedSecret`，未删键——`SettingsView` 与 `CertificatesView` 都用「值非空」判断已配置，删键会让「已配置」状态与 ZeroSSL 申请门槛失效） |

### 1.2 密钥与凭据（中危）

| 位置 | 问题 | 修法 |
| --- | --- | --- |
| `internal/frp/agent_installer.go:34` → `internal/frp/service.go:694-700` → `POST /api/frpmulti/servers/{id}/agent/probe` | Probe 用 `sed` 从 `/etc/fonu-agent/agent.env` 读出 `FONU_AGENT_TOKEN` 放进返回体，**明文回传前端**（库里同一字段走 `has_token` 掩码，`internal/frp/model.go:102`） | 只返回 `has_token` + 后 4 位 |
| `internal/agent/server.go:1243` | 自动生成的 Basic Auth 密码被写成 nginx 注释 `# Basic Auth: user=… password=…`，文件 0640 落在 VPS vhost 目录，且被 `GET /api/v1/routes/{domain}/conf` 原样返回 | 只在 `PUT /api/v1/routes` 响应里返回一次密码，不写注释 |
| `internal/auth/auth.go:57-62` | 首启生成的 admin 密码明文写入应用日志（`internal/app/app.go:54-55` 同时进 stdout 与 `app.log`），而 `GET /api/logs/system` 会把该文件内容返回 UI | 仅写 stderr，或改为环境变量 / 一次性文件下发初始密码 |
| `internal/agent/server.go:97,892` | 目录缓存文件用 `0o644` | 统一 `0o600` |

### 1.3 鉴权强度与会话

| 位置 | 级别 | 问题 | 修法 |
| --- | --- | --- | --- |
| `internal/api/auth_handler.go:52-70` | 中 | 登录失败只记录，**无限速/锁定**，可无限爆破 | 按 IP + 用户名做失败计数与退避（`notify` 里已有窗口统计可复用） |
| `internal/api/auth_handler.go:90-99` | 中 | 会话 cookie 缺 `Secure`，而应用默认明文 HTTP（`internal/app/app.go:122`） | 依 `X-Forwarded-Proto` 或配置项决定是否加 `Secure` |
| `internal/api/auth_handler.go:114` | 低 | `clearSessionCookie` 未设 `SameSite`（其余 cookie 是 Lax） | 补齐 `SameSite=Lax` |
| 全局 | 中 | 无 CSRF 令牌，状态变更仅靠 cookie | 校验 `Origin`/`Referer`，或加双提交令牌 |
| `internal/auth/auth.go:22,120-138` | 中 | 会话固定 7 天 TTL，改密码**不失效其他会话** | `ChangePassword` 时 `DELETE FROM sessions WHERE admin_id = ?` |
| `internal/agent/server.go:191` | 低 | token 比较用 `!=`（非常量时间） | `crypto/subtle.ConstantTimeCompare` |
| `internal/agent/server.go:114`、`cmd/fonu-agent/main.go:17` | 中 | agent 默认监听 `:7700` 且为明文 HTTP，Bearer token 在公网裸奔；`-token` 还会进 `ps` 输出 | 默认绑 `127.0.0.1`（Nginx 前置）或加 TLS |
| `internal/api/router.go:180` + `:42` | 低 | `SessionMiddleware` 被套两层，重复解析 cookie | 只在外层套一次 |

### 1.4 路径穿越（除 1.1 外）

| 位置 | 级别 | 问题 | 修法 |
| --- | --- | --- | --- |
| `internal/backup/service.go:88` | 中 | `strings.HasPrefix(cleaned, cleaned(dataDir))` 是**不带分隔符的前缀判断**：`DataDir=/data` 时条目 `../data-other/x` 清理后为 `/data-other/x`，前缀匹配通过 → 写到 DataDir 之外 | 用 `filepath.Rel` + 拒绝 `..`（`internal/acme/download.go:75-89 validateCertFilePath` 已是正确写法，直接复用） |
| `internal/backup/service.go:100` | 中 | `os.FileMode(header.Mode)` 未掩码，构造的归档可造出 0777 / 带执行位文件 | `& 0o777`，目录/文件强制 `0o750/0o640` |
| `internal/api/backup_handler.go:41` | 低 | 单请求 64MB 且该端点无频率限制 | 降到 16MB 并加限速 |

---

## 二、错误处理与正确性

| 位置 | 级别 | 问题 | 修法 |
| --- | --- | --- | --- |
| `internal/agent/server.go:318` | 高 | `_ = exec.Command("systemctl","enable","--now","frps").Run()` 后无条件 `notes = append(notes, "frps.service 已启动")` —— 启动失败仍报成功 | 判 `err`，失败写 stderr 到 `notes` 并置 `ok:false` |
| `internal/frp/service.go:1498` | 中 | `_ = os.WriteFile(path+".bak", current, 0o600)` 失败被忽略，紧接着 `os.Rename` 覆盖唯一副本，可能丢配置 | 备份失败即 return，不覆盖 |
| `internal/agent/server.go:1188` 附近 `readLimitState` | 中 | `_ = json.Unmarshal` 失败静默变空 map，`writeLimitState` 随后删掉 zone 文件 → 全部路由限流静默消失 | 反序列化失败返回 error 并保留原文件 |
| `internal/frp/service.go:775-779` | 中 | `InstallFRPSOnAgent` 里 `_ = client.PushFRPSConfig(...)` 忽略下发失败，前端只看到「安装成功」 | 把该错误并入返回结果 |
| `internal/api/cert_handler.go:47,144`、`ddns_handler.go:122`、`frpmulti_handler.go:609` | 中 | `_ = json.NewDecoder(r.Body).Decode(&req)` 忽略解析错误，继续用零值请求体 | `if err := ...; err != nil { 400 }` |
| `internal/backup/service.go:30`、`internal/api/backup_handler.go:53-71` | 中 | 写入方 `defer Close()` 不检查错误：ENOSPC 时半个备份被当有效文件下载；`out.Close()` 失败后仍去 Restore | 写入方改显式 `if err := f.Close(); err != nil` |
| `internal/api/logs_handler.go:85,112` | 低 | `_ = logstore.StreamFile(...)` 丢弃 SSE 写出错误 | `slog.Warn` 记录 |
| `internal/api/auth_handler.go:98` | 低 | `_ = h.auth.Logout(...)` 忽略删会话失败，客户端仍清 cookie | 失败返回 500 |
| `internal/traffic/collector.go:148,151` | 低 | `_ = c.persist(ctx)` 落库失败无痕迹 | 记 warn 并保留脏标记重试 |
| `internal/frp/agent_client.go:58` | 低 | `data, _ := io.ReadAll(...)` 忽略读错误，非 2xx 时错误体可能为空 | 判 err |
| `internal/frp/agent_installer.go:24-25` | 低 | `Probe` 把所有 SSH 失败吞成空串，与「VPS 上确实没有」无法区分 | 收集失败原因，结果里带 `warnings` |
| `internal/frp/ssh_installer.go:125-126` | 低 | `var _ io.Reader = (*os.File)(nil)` / `var _ = filepath.Separator` 是为保留导入的哑声明 | 删两行及两个导入 |
| `internal/agent/server.go:229,422` | 低 | `dashboardAddr` 注释自称已废弃，仍被 `status` 调用，每次读一遍 frps.toml | 删函数与调用 |

**已核查无问题**：`err` 遮蔽（`service.go:1494` 用 `readErr` 是有意为之）；无命令注入（22 处 `exec.Command` 全固定 argv；SSH 侧唯一插值点 `uploadBytes:95` 经 `shellPath:123` 单引号转义，传入常量 `/tmp/fonu-agent`）。

---

## 三、并发与生命周期

| 位置 | 级别 | 问题 | 修法 |
| --- | --- | --- | --- |
| `internal/frp/service.go:166,170-172` | 中 | `go s.runDownload(version)` 用 `context.Background()` + 10 分钟超时，不随 App 关闭取消，`internal/app/app.go:196-215 Shutdown` 也不等待；任务状态只在内存，重启即丢 | 传 App ctx，`sync.WaitGroup` 在 Shutdown 等待 |
| `internal/scheduler/scheduler.go:31,37` | 中 | `job.Run(ctx)` 无 `recover()`，任一 tick panic 会崩掉整个进程；`time.NewTicker(job.Interval)` 在 Interval ≤ 0 时 panic，目前只靠 `internal/app/app.go:135-143` 兜住 | 每个 job 包 `defer recover()`；`New`/`Start` 跳过非正间隔 |
| `internal/traffic/collector.go:176-198` | 中 | `ingestLine` 持 `c.mu` 写锁时调 `notify.RecordAccessHit`（DB 写 + 通知判断），**锁内做 I/O**，阻塞 `/api/proxies/traffic`、`/clients` 读 | 锁内只累积 IP，锁外统一 notify |
| `internal/frp/service.go:1258` | 中 | `agentClient` 每次调用都 `NewAgentClient` → 新建 `http.Client`/`Transport`，批量同步 N 条路由就建 N 个连接池 | 按 serverID 缓存 client |
| `internal/agent/server.go:107 preloadNginxConfDir` | 低 | 启动 `go func()` 一次性任务无 ctx，退出时可能正在跑 `nginx -T` | 加 ctx 或 WaitGroup |
| `internal/logstore/stream.go:65-70` | 低 | 每个 SSE 客户端一个 goroutine + 500ms 轮询（ctx 检查正确、无泄漏） | 可接受；量级上去后换 fsnotify |

**已核查无问题**：全部 `http.Client` 构造点都带 `Timeout`；两处 `time.NewTicker` 都有 `defer Stop()`。

---

## 四、重复逻辑（可收敛点）

| 重复项 | 证据 | 建议 |
| --- | --- | --- |
| **「安全写文件」5 套实现** | `internal/agent/server.go:1428 atomicWrite`、`internal/frp/service.go:1816 writeConfigAtomically`、`internal/nginx/nginx.go:114-141`（固定 `.tmp` 名，0644，且同一段在 `:194`、`:394` 再抄两遍）、`internal/chinacidr/service.go:99-117`、`internal/frp/service.go:1488-1501` | 抽 `internal/fsutil.WriteFileAtomic(path, data, mode)`；`nginx.go` 三处合并为 `applyConfigFile` |
| **非原子直写** | `internal/nginx/custom.go:98,144,151,158`、`internal/nginx/errors.go:32,78` | 同上 |
| **`.bak` 备份 4 套** | `internal/frp/service.go:1496-1498`、`:1821-1825`、`internal/nginx/nginx.go:132-136`、`internal/nginx/custom.go:190-197` | 统一 `fsutil.Backup(path)` |
| **nginx 模板双份** | `internal/agent/server.go`（`nginxRoute:1155`、`sslDirectives:1420`、`zoneKey:1390`、限流/geo 生成）与 `internal/nginx/generate.go`（有 30 个测试文件覆盖）各写一遍 vhost/TLS/限流模板 | 抽到共享包，agent 与宿主机同源；否则改一处必须记得改两处 |
| **agent HTTP 客户端双份** | `internal/frp/agent_client.go` 与 `internal/frp/agent_insights.go`、`internal/agent/insights.go` 对同一批端点各自解包 | insights 也走 `AgentClient.request` |
| **规则自定义配置两处入口** | `internal/service/nginx_config.go` 与 `internal/nginx/custom.go` | 只保留 `internal/nginx/custom.go` |

---

## 五、前端

### 5.1 用户可见缺陷（已复核）

| 位置 | 级别 | 问题 | 修法 |
| --- | --- | --- | --- |
| `web/src/views/AgentView.vue:582` | **P0** | `<StatusBadge :value="serverStartupType(viewingServer)" :text="serverStartupText(viewingServer)" />` —— `serverStartupType()`（`:1382-1387`）返回的是 `'success'\|'warning'\|'error'\|'default'`（`n-tag` 的 type 值），却被当状态值传给 `:value`；`statusKind('success')` 与 `statusKind('default')` 都落到 unknown → **服务端详情弹窗「运行中」和未知态都是灰点**（`FrpView.vue:777` 注释记录过同一个坑，那边修了、这边没修） | 改 `:kind="serverStartupType(viewingServer)"`，函数标注返回 `StatusKind` 并把 `'default'` 换成 `'unknown'`；`:49` 的 `n-tag :type` 另拆 `serverStartupTagType()` |
| `web/src/views/DashboardView.vue:35,46` | **P1** | `:value="status?.ddns_status"` / `:value="status?.certificate_status"` 配静态 `text="正常"` —— 后端返回 `error`/`none` 时出现「红/灰点 + 正常」自相矛盾 | `text` 改由 `statusLabel(value)` 推导 |
| `DdnsView.vue:170,213`、`ProxyView.vue:139`、`DashboardView.vue:22,59,70,162,238,263` | P2 | 目前值恰好是 `statusKind()` 的合法输入，但语义错，后端一改值就静默变灰 | 统一改 `:kind` |
| `web/src/views/ProxyRemoteView.vue:489-514` | P1 | `refreshAll` 里 7 个独立接口用 `await track(...)` 串行发，每次刷新耗时是 7 个往返相加；15 秒轮询时同样串行 | 改 `Promise.all`（`track` 已把异常收进 `failures`，并行不影响「单点失败不中断整轮」）；健康检查可拉长周期或只在手动刷新时跑 |

### 5.2 死代码与重复样式（重构遗留）

| 位置 | 级别 | 问题 |
| --- | --- | --- |
| `web/src/views/FrpView.vue:1229-1236`、`:1396-1398`、`:1468-1474`、`:1487`、`:1524` | P1 | `frp-server-table`、`frp-detail-card`、`server-detail*`、`detail-stats`、`server-status-summary`、`status-bars` 等类名在模板与脚本 **0 引用**（活体已迁到 `AgentView.vue:578-610` / `1818-1844`），整块可删 |
| `web/src/views/AgentView.vue:1853-1855` | P1 | `.detail-subgroup`/`__title`/`.detail-muted` 与 `:1850-1852` **逐字重复两遍**，删后一组 |
| 弹窗外壳 `.proxy-modal__*` | P1 | 在 `FrpView.vue`（`.frp-proxy-modal` 与 `.frp-guide-modal` 各一份）、`ProxyRemoteView.vue:942-958`、`AgentView.vue`、`ProxyView.vue` **四份**，差异仅 1–2 个属性 | 抽 `ProxyModalShell.vue`（header/tabbar/scroll/help 四插槽）或非 scoped 的 `styles/proxy-modal.css` |
| 左列表骨架 | P1 | `DdnsView`（`.ddns-layout`/`.task-sidebar`/`.task-item`）与 `AgentView`/`FrpView`/`ProxyRemoteView`（`.server-sidebar`/`.server-item`）两套命名同构重复；`.server-item__name`/`__endpoint` 三条规则在三个文件逐字相同 | 抽 `SidebarList.vue`（items + activeKey + 行插槽） |

> 说明：`FrpView.vue` 里 `.server-sidebar` / `.server-item`、`.frp-proxy-modal`、`.frp-guide-modal` **仍在模板中使用，不是死代码**，不要一起删。

### 5.3 硬编码颜色（违反「零硬编码色值」约定）

grep 计 `#hex`/`rgb()`/`rgba()` 共 267 处 / 19 文件；扣除 `styles/variables.css`（36 处，令牌定义本体）后 231 处。按文件：`useTheme.ts` 45、`CertificatesView.vue` 44、`ProxyView.vue` 34、`ProxyAccessLogBox.vue` 22、`DdnsView.vue` 18、`DashboardView.vue` 15、`MiniTrafficChart.vue` 11、`MiniBarChart.vue` 9、`SettingsView.vue` 8、`StatCard.vue` 6、`MiniDonutChart.vue` 5、`LogsView.vue` 4、`AgentView.vue` 3、`StatusBadge.vue` 3，其余各 1。

| 优先级 | 位置 | 问题 |
| --- | --- | --- |
| P1 | `DashboardView.vue:1124-1131` | 横幅写死 `#f4fffc/#f1fbff/#eef9ff/#0f172a/#134e4a` 与 `rgba(16,185,129,…)`，而 `variables.css:17,58` 已有 `--fonu-banner-bg`/`--fonu-banner-border` |
| P1 | `CertificatesView.vue:1256-1272`、`DdnsView.vue:1263-1279`、`ProxyView.vue:2225-2228` | 同一组状态色 `#3b82f6/#10b981/#f59e0b/#ef4444` 复制三处 | 收敛到 `StatCard` 的 tone 类或 `--fonu-tone-*` 令牌 |
| P1 | `CertificatesView.vue:1182-1211`、`ProxyAccessLogBox.vue:149-150` | 同一暗色终端色板写两遍 | 抽 `--fonu-log-*` |
| P1 | `StatusBadge.vue:43,48,53`、`StatCard.vue:84-95` | 组件内 `rgba(16,185,129,.12)` 绕过 `variables.css:4` 已有的 `--fonu-brand-soft` | 改用令牌并补 warning/error 的 soft 令牌 |
| P2 | `ProxyView.vue`、`DdnsView.vue`、`CertificatesView.vue`、`SettingsView.vue:1251-1282,1601-1613`、`LogsView.vue:854-898` 等 | 零散违规，含 `color-mix(in srgb, #10b981 40%, …)` 与 `var(--n-error-color, #e11d48)` 直接引用 Naive UI 变量 | 逐条换语义令牌 |
| P2 | `MiniBarChart`/`MiniDonutChart`/`MiniTrafficChart`/`useTheme.ts` | 图表色板三份重复拷贝，且 `AgentView.vue:1257` 用 `#18a058` 与品牌绿不一致 | `useTheme.ts` 导出 `CHART_COLORS` 供各图表复用 |

**有意保留**：`FrpView.vue:1385`、`AgentView.vue:1808` 的 `color: #fff`（品牌底色上的文字）；ECharts 系列色（按既有约定保留，但建议收敛为常量）。

### 5.4 类型

- `api/client.ts:288,331` 的 `Record<string, any>` 是全仓仅有的 2 处 `any` → 定义 `FrpAgentProbeResult`。
- `FrpView.vue:778` 用 `as 'success' | 'disabled' | 'warning'` 硬转 → 改 `: StatusKind`。
- `DdnsView.vue:669-673` 的 `taskStatusKind()` 名为 Kind 实为状态值，与 `StatusBadge` 的 `kind` 语义相反 → 改名 `taskStatusValue`。
- `AgentView.vue:1382-1387` 的 `serverStartupType()` 无返回类型注解，是 §5.1 那条 P0 的根因。

### 5.5 轮询与页面可见性

- 清理齐全（9 处 `clearInterval` 都在 `onUnmounted`/`onBeforeUnmount`）。
- **只有仪表盘处理了页面隐藏**：`DashboardView.vue:846-856` 只停 traffic(2s)/status(15s)，同文件的 `cidrPollTimer`（`:946-974`）与 `loadRouteHealth` 不在暂停范围；`ProxyView.vue:1799,1825`、`FrpView.vue:1207,914`、`LogsView.vue:782`、`ProxyRemoteView.vue:865` 完全没有隐藏页处理 → 抽 `useVisibilityPolling()`。
- **轮询无 in-flight 互斥**：`DashboardView.vue:808-812` 每 2s 调 `refreshTraffic()`，体内只有 try/catch，慢链路会堆积并发请求 → 加 `if (loading.value) return` 或改递归 `setTimeout`。
- 频率偏高：2s 类（Dashboard/ProxyView）建议降到 5s 或按激活状态降频。

### 5.6 超大组件

`ProxyView.vue` 3480、`AgentView.vue` 1966、`CertificatesView.vue` 1875、`DashboardView.vue` 1758、`DdnsView.vue` 1745、`SettingsView.vue` 1672、`FrpView.vue` 1557。

- `ProxyView.vue` 优先拆 `RuleFormModal.vue`、`NginxSettingsPanel.vue`、`RuleClientsPanel.vue`。
- `AgentView.vue`：模板 `440-559` → `ServerFormModal.vue`；`561-577` → `ServerLogsModal.vue`；`578-640` → `ServerDetailModal.vue` + `LatencyChart.vue`。
- 搬迁硬约束：**scoped 样式不随模板走，必须整份复制**（`AgentView.vue:1850-1855` 的重复块就是漏搬/重搬的产物）。

### 5.7 空态 / 可访问性 / i18n

- `v-html` 0 处；空态覆盖良好（`EmptyState`/`LoadError` 各页 2–11 处）；未发现纯图标按钮缺 `aria-label`。
- 缺口：`AgentView.vue:609`（代理表）与 `:610`（连接日志）用裸文本兜底，未走 `EmptyState`；`DashboardView.vue:244` 的 `health-item__icon--${tone}` 仅靠颜色传达状态，建议补文字或 `aria-label`。
- i18n：无 i18n 设施，用户可见文案全部中文硬编码（含中文的行 1882 行 / 17 文件，含注释故为上限；最多 `AgentView` 388、`FrpView` 352、`ProxyView` 227）。建议先建 `src/locales/zh-CN.ts` 按 views 命名空间搬迁，或至少把高频重复串提为常量（样板：`utils/status.ts:3-11` 的 `LABELS`）。

---

## 六、测试缺口

**Go：71 个 `*_test.go`，但 8 个包零测试** —— `internal/app`、`internal/service`、`internal/scheduler`、`internal/settings`、`internal/backup`、`internal/secret`、`internal/discovery`、`internal/version`，以及 `cmd/fonu`、`cmd/fonu-agent`。

关键文件零覆盖（按风险排序）：

1. `internal/agent/server.go`（1465 行，VPS 侧所有文件写入/nginx 操作/exec）—— 包内仅 `apr1_test.go`、`insights_test.go`；`validateRoute`、`nginxRoute`、`atomicWrite`、`detectNginxConfDir`、`installFRPS` 全无测试。
2. `internal/api/router.go` + `middleware.go` —— 没有任何测试断言「每条路由都在 `RequireAuth` 之后」，新增路由漏 `protect()` 不会有信号。
3. `internal/backup/service.go` —— §1.4 的路径校验 medium 无测试；`backup_handler.go` 的临时文件命名也无测试。
4. `internal/frp/service.go`（1847 行）—— 仅 `service_diagnostics_test.go` 间接触及。
5. `internal/frp/ssh_installer.go`、`agent_installer.go` —— `extractToken`、`isELFBinary`、`shellPath`（含单引号转义）都是纯函数，测试成本极低。

建议补的最小集：① 路由鉴权覆盖测试（遍历 `NewRouter`，非公开路径断言 401）；② `backup.Restore` 的 `../` 与 `../data-other/` 两组用例；③ `agent.nginxRoute` + `validateRoute` 表驱动测试；④ `shellPath`/`extractToken`/`isELFBinary` 纯函数测试；⑤ `scheduler` panic 恢复测试；⑥ `internal/notify` 补「SMTP 密文损坏时 Telegram 仍可发送」。

**前端：完全没有测试设施** —— `web/package.json` 无 `test` 脚本，devDependencies 无 vitest/jest/@vue/test-utils，`web/src` 无 `*.spec.ts`/`*.test.ts`；唯一质量门是 `npm run build` 内部的 `vue-tsc --noEmit`。建议加 vitest 骨架，从 `src/api` 与格式化纯函数起步。

---

## 七、工程化 / CI / Docker / 版本

| 位置 | 级别 | 问题 | 修法 |
| --- | --- | --- | --- |
| `.github/workflows/`（`docker-release.yml` 106 行、`github-release.yml` 52 行） | **高** | CI 存在但**只发版不校验**：仅 tag / `workflow_dispatch` 触发，零 `go build`/`vet`/`test`、零 `vue-tsc`/`vite build` | 新增 `.github/workflows/ci.yml`，`on: [push, pull_request]` |
| 同上 + `cmd/fonu/main.go:19` | **高** | 步骤顺序不可颠倒：`//go:embed all:web/dist`，而 `web/dist/` 被 `.gitignore` 忽略 → 干净检出下 `go build ./...` 直接失败。CI 必须「前端构建 → 复制 dist 到 `cmd/fonu/web/dist` → `go vet` → `go test ./... -count=1` → `go build`」 | 按此顺序写 `ci.yml` |
| `web/package-lock.json` + `pnpm-lock.yaml` + `pnpm-workspace.yaml` | **高** | 三套依赖描述并存，`package.json` 无 `packageManager`，全仓无 pnpm 调用，Dockerfile 用 `npm ci` | 删 `pnpm-lock.yaml` 与 `pnpm-workspace.yaml`，加 `"packageManager": "npm@10"`（2026-09-24 实施：仓库里实际只有 `web/pnpm-workspace.yaml`，无 `pnpm-lock.yaml`；已删该文件并声明 `npm@10.9.4`） |
| `VERSION`(1.2.6) vs `web/package.json:4`(1.0.2) vs `web/vite.config.ts:10` | **高** | 版本号双源：Docker 链正确注入 `VERSION`，但本地 `npm run dev/build` 回落 `pkg.version` → UI（`AppLayout.vue:140`）显示 1.0.2 | 同步 `package.json` 版本，或让 `vite.config.ts` 直接读 `../VERSION`；CI 加一致性校验 |
| `web/package-lock.json`（如 `:1360`） | 中 | `resolved` 指向 `registry.npmmirror.com`，海外 runner 会从镜像拉包 | 加 `web/.npmrc` 或 CI 用 `npm ci --registry=https://registry.npmjs.org` |
| `fonu-agent.exe`（项目根） | 高 | 未被 `.gitignore` 覆盖（规则里没有 `*.exe`） | 加 `*.exe` 或精确到 `/fonu-agent.exe` |
| `internal/frp/agentbin/*` | 中 | 是 `//go:embed` 编译必需资源（须入库），但无二进制标记 | `.gitattributes` 加 `internal/frp/agentbin/* binary` |
| `docker-release.yml` 的 `build-args: VERSION=…` | 中 | 死配置：Dockerfile 只有 `ARG VERSION_FILE`，从不读 `ARG VERSION`，tag 与文件不一致时静默 | workflow 加一步校验 tag == `VERSION` 文件，或 Dockerfile 支持 `ARG VERSION` 优先 |
| `.dockerignore:15` | 中 | ` docs` 带前导空格；`:12-13` 仍排除已不存在的 `internal/frp-1.0.2`、`FrpView-1.0.2.vue` | 去掉空格与两行过期排除项 |
| `Dockerfile:21` | 低 | `COPY web/package.json web/package-lock.json* ./` 通配符会在锁文件缺失时静默退化 | 去掉 `*` |
| `Dockerfile:53` | 低 | `COPY web/assets/image` 与 `internal/nginx/error_assets_embed.go` 的 embed 可能重复 | 确认运行时用途，若已被 embed 覆盖则移除 |
| `README.md:156`、`docs/DEPLOYMENT.md:42,53`、`internal/frp/frps_template.go:45` | 中 | 文档称「镜像内置 frpc 0.71.0」，而 `Dockerfile:46` 明确「frpc 不预装」 | 三处改为「按需下载」 |

**已确认正确**：镜像链路自洽（`npm ci` → `vite build` → dist 拷入 `cmd/fonu/web/dist` 正好匹配 embed；`web/assets/image` 与 `cmd/fonu/web/dist` 均真实存在，COPY 不会失败）；`.data/`、`*.db` 已被忽略。

---

## 八、文档与仓库卫生

| 位置 | 级别 | 问题 | 修法 |
| --- | --- | --- | --- |
| git 根 `f:\Fonu-main` | **高** | 仓库根在此且 `No commits yet on main`，`Fonu-1.0.2/` 整目录未跟踪，根无 `.gitignore`；源码与文档全在版本控制之外 | 明确仓库边界：在 `Fonu-1.0.2/` 内独立 `git init`，或根补 `.gitignore` 并首次提交 |
| `f:\Fonu-main` 根目录 | **高** | 散落 `fonu-1.1.0-image.tar.gz`、`fonu-1.1.0-linux-amd64.tar.gz`、`fonu-1.1.0-linux-amd64`、`fonu-1.1.0-src.zip`、`agent-tsc.log`、`frps-m1-tsc.log`、`fonu-image-build4.log` | 移出仓库或删除；根 `.gitignore` 加 `*.tar.gz`/`*.zip`/`*.log`/`fonu-*` |
| `.breezell/`、`.gocache-agent/` | 中 | 未被 gitignore（`.dockerignore:20` 只挡了前者进镜像） | 根 `.gitignore` 加两行 |
| `internal/frp-1.0.2/`、`web/src/views/FrpView-1.0.2.vue` | 中 | 工作树已删除但**尚未提交**（`git status` 显示为未暂存删除），误 `git checkout` 即回滚清理 | 随下次提交一并提交，并在 CHANGELOG 记一条「移除 1.0.2 归档副本」 |
| `Fonu-1.0.2/docs/`（14 份 md） | 中 | 无索引；`upstream-sync-plan.md` 对 `/frp` 现状的描述基于 2026-09-23（已被 v1.2.6 重构推翻）；`agent-view-design.md` / `-v2.md` 的「顶部下拉」结构已过期；`agent-capability-plan.md` 的 P0/P1/P2 已实施但仍以「方案」语气；`frps管理方案.md` 疑似被 `frps服务端管理方案.md` 取代 | 加 `docs/README.md` 索引表（文件 / 主题 / 状态 / 更新日期）；给三份文档加时效或实施状态标注 |
| `f:\Fonu-main\docs\` 与 `Fonu-1.0.2\docs\` | 中 | 两套文档根并存（根下 `111.md`、`FRP迁移方案-0.1.25到1.0.2.md`、`FRP版本对比-1.0.2-vs-0.1.25.md`） | 合并，或声明根 docs 为临时区 |
| 仓库根 | 低 | 无 README（`Fonu-1.0.2/README.md` 本身 189 行、质量良好） | 上移或放最短指路 README |
| `gofmt` | 低 | `internal/api/auth_handler.go:29-32`、`internal/api/settings_handler.go:44-79`、`internal/traffic/collector.go:18-24,31-38` 从列对齐看确定未格式化（**待 `gofmt -l internal cmd` 确认全量**） | 跑 `gofmt -w` 并在 CI 加卡口 |

---

## 九、增补清单（按优先级，可勾选）

**P0（先做）**

> **实施状态（2026-09-24）**：以下 8 项已全部完成并随 v1.2.7 提交；agent 侧同时把 `AgentVersion` 递增到 0.12.0 并重编了内嵌双架构二进制（VPS 上需点「升级 Agent」才生效）。两处与本文写法不同的取舍见 §1.1 末行与 §七 pnpm 行。

- [ ] `internal/api/backup_handler.go:52` 改 `os.CreateTemp`；`internal/backup/service.go:88` 改 `filepath.Rel` 校验；`:100` 掩码 mode
- [ ] `internal/agent/server.go:258` 的 `req.Proxy` 走白名单 + 下载后校验 SHA-256
- [ ] `internal/frp/ssh_installer.go:42` 换 `knownhosts`（或指纹确认）
- [ ] `internal/notify/store.go:400` 增删 `KeyZeroSSLAPIKey`；`agent_installer.go:34` 只回 `has_token`；`agent/server.go:1243` 不写密码注释
- [ ] `web/src/views/AgentView.vue:582` 改 `:kind` + 返回类型标注；`DashboardView.vue:35,46` 文案随状态推导
- [ ] `.github/workflows/ci.yml`（前端构建 → dist 同步 → vet → test → build）
- [ ] 删 `web/pnpm-lock.yaml` + `pnpm-workspace.yaml`；`web/package.json` 加 `packageManager` 并与 `VERSION` 对齐
- [ ] 根目录 `.gitignore`（含 `*.exe`、`.breezell/`、`.gocache-agent/`）并清理散落产物

**P1**

- [ ] `internal/agent/server.go:318` 假成功、`internal/frp/service.go:1498` 备份失败仍覆盖、`:775-779` 忽略下发失败
- [ ] 4 处请求体解析忽略错误改 400（`cert_handler.go:47,144`、`ddns_handler.go:122`、`frpmulti_handler.go:609`）
- [ ] 两个 `http.Server` 补 `ReadTimeout`/`IdleTimeout`；`runDownload` 挂 App ctx；`scheduler` 加 `recover()`
- [ ] 登录失败退避 + 改密失效旧会话 + cookie `Secure`/`SameSite`
- [ ] `ProxyRemoteView.vue:489-514` 改 `Promise.all`
- [ ] 清 `FrpView.vue` 四组死样式 + `AgentView.vue:1853-1855` 重复块（即 merge 方案的步 4）
- [ ] 抽 `ProxyModalShell.vue` / `SidebarList.vue`；抽 `useVisibilityPolling()`；轮询加 in-flight 互斥
- [ ] 修 README/DEPLOYMENT/`frps_template.go:45` 的「内置 frpc」说法；删 `.dockerignore:12-13,15` 问题行
- [ ] `docs/README.md` 索引 + 三份文档时效标注

**P2**

- [ ] 抽 `internal/fsutil`（原子写 + 备份），合并 5 套写文件 / 4 套 `.bak`
- [ ] nginx 模板抽共享包，agent 与宿主机同源
- [ ] `traffic/collector.go:176` 锁外 notify；`frp/service.go:1258` 缓存 agent client
- [ ] 补测试：路由鉴权覆盖、`backup.Restore` 路径、`agent.nginxRoute`、纯函数组、`scheduler` panic、notify 隔离
- [ ] 前端加 vitest；颜色令牌收敛（231 处）；i18n 起步
- [ ] 拆 `ProxyView.vue`（3480 行）等超大组件

---

## 十、需用户决策

1. **仓库边界**：`f:\Fonu-main`（根，未跟踪全部源码）还是在 `Fonu-1.0.2/` 内独立 `git init`？这决定 `.gitignore` 与 README 放哪。
2. **包管理器**：确定 npm（删 pnpm 两个文件）还是保留 pnpm（需同步改 Dockerfile/README）。
3. **SSH 主机密钥策略**：`knownhosts` 落库（严格、需用户首次确认指纹）还是仅做指纹变更告警（宽松）。
4. **agent 监听地址**：改绑 `127.0.0.1` 需确认 VPS 上 Nginx/SSH 隧道的前置方式，属于会改变部署方式的改动。
5. **版本号粒度**：v1.2.4/1.2.5/1.2.6 同日三段，是否改为「同日合并一段」。
6. **前端 i18n**：是否要真正引入（工作量大），还是只做文案常量集中化。

## 十一、需真机验证（静态审查无法覆盖）

- agent 升到 0.11.0 后：VPS 资源数值随负载变化、公网 Nginx 日志、frps.toml 手工改动比对、VPS 证书清单、故意写错配置后点「重载 Nginx」看失败原因。
- `/frp-agent`、`/frp`、`/public-proxy` 三页新分栏结构的实际渲染（本地登录墙无凭据，未做视觉验证）。
- `bin\fonu.exe` 需重建并重启才能看到本轮前端改动。
