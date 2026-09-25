# 同类项目对标与增补建议

> 日期：2026-09-24　对象：Fonu 1.0.2（VERSION 1.2.10）
> 方法：检索 GitHub 上与本项目同域的开源项目（隧道/反代管理面板、自托管监控、Nginx 面板、DDNS 面板），逐项对照 Fonu 现状给出「可优化 / 可增补」。
> 可信度：对标项目的能力来自其首页 / README 摘要与检索片段，**未逐个深读源码**；Fonu 现状来自本仓库代码与 2026-09-24 的全项目审查（`docs/project-audit-2026-09-24.md`）。

---

## 一、对标项目

| 项目 | 定位 | 与 Fonu 的关系 |
| --- | --- | --- |
| [fosrl/pangolin](https://github.com/fosrl/pangolin) | 自托管**隧道式反代管理服务**，内置身份与访问控制 + 仪表盘 UI | 架构最接近：VPS 侧服务 + 内网侧隧道客户端 + 统一面板；差别在它主打**身份/访问控制**（SSO、资源级策略） |
| [vaala/frp-panel](https://github.com/vaala/frp-panel) | frp 的**中心化控制面板**（Go + gRPC + protobuf，React/Next + shadcn） | 同为「面板 + 多客户端」模型；它管的是多台内网 frpc，Fonu 管的是本机 frpc + 多台公网 frps |
| [henrygd/beszel](https://github.com/henrygd/beszel) | 轻量服务器监控，**hub + agent**，含历史数据、Docker 统计、告警、多用户、OAuth、API | 与 Fonu 的 app + fonu-agent 架构同型；它在「历史/告警/账号」上明显更成熟 |
| [0xJacky/nginx-ui](https://github.com/0xJacky/nginx-ui) | Nginx 的 Web 管理界面：配置**版本备份/对比/回滚**、集群、证书一键签发与续期、在线编辑、加密配置导出 | Fonu 的反代层与它重叠；它的配置版本管理值得直接借鉴 |
| [jeessy2/ddns-go](https://github.com/jeessy2/ddns-go) / [qdm12/ddns-updater](https://github.com/qdm12/ddns-updater) | DDNS 客户端：多服务商、Web UI、多记录类型、通知（ddns-updater 走 Shoutrrr，支持几十种渠道） | Fonu 的 DDNS 模块与其重叠；差距在**通知渠道数量**与记录类型覆盖 |

---

## 二、能力对照（✔ 已有 / ◐ 部分 / ✘ 无）

| 能力 | pangolin | frp-panel | beszel | nginx-ui | ddns-go | Fonu 现状 |
| --- | --- | --- | --- | --- | --- | --- |
| 面板 + 内网 agent（SSH 引导安装） | ✔ | ✔ | ✔ | ◐ | ✘ | **✔**（SSH 装 agent、HTTP 管理、TOFU 主机密钥） |
| 多台公网服务端 + 多规则类型 | ✘ | ◐ | ✘ | ✘ | ✘ | **✔**（frps 多台、8 种穿透类型、规则按服务端归属） |
| 公网反代下发 + 证书推送 | ✔ | ◐ | ✘ | ✔ | ✘ | **✔**（vhost 模板生成、证书推送、部署漂移检测、四段健康检查） |
| 配置**版本历史 / 对比 / 回滚** | ◐ | ◐ | ✘ | **✔** | ✘ | **✘**（只有单份 `.bak`） |
| 指标**历史趋势** | ◐ | ◐ | **✔** | ◐ | ✘ | **✘**（仅当前值 + 最近 5 分钟采样） |
| **阈值型告警**（CPU/磁盘/连续失败） | ◐ | ✘ | **✔** | ✘ | ◐ | **✘**（只有事件型通知） |
| 通知渠道数量 | ◐ | ◐ | ✔ | ✘ | ✔ | ◐（邮件 / Webhook / Telegram） |
| **API Token**（脚本、HomeAssistant 调用） | ✔ | ✔ | ✔ | ✔ | ✔ | **✘**（仅会话 cookie） |
| 多用户 / OIDC / 只读账号 | **✔** | ◐ | ✔ | ◐ | ✘ | **✘**（单管理员） |
| 定时自动备份 + 保留策略 | ◐ | ◐ | **✔** | ✔ | ✘ | ◐（手动导出 / 恢复） |
| Docker 容器统计 | ✘ | ✘ | **✔** | ✘ | ✘ | **✘** |
| 加密配置导出 / 迁移包 | ◐ | ✘ | ✘ | **✔** | ✘ | ◐（脱敏导出，无导入） |

---

## 三、建议增补（按价值 / 成本排序）

### P0 · 成本低、直接补齐明显短板

1. **配置版本历史 + 对比 + 回滚**（学 nginx-ui）
   - 现状：`internal/fsutil.Backup` 只保留一份 `<path>.bak`；公网反代页已有「漂移检测」能看出「VPS 上的与记录不一致」，但没有历史可比。
   - 做法：把 `.bak` 升级为 `<path>.versions/<时间戳>.conf`（保留 N 份，沿用现有 `pruneBackups` 思路），复用公网反代页「漂移清单」的 UI 模式做「版本列表 + 差异高亮 + 一键回滚」。
   - 落点：`internal/fsutil`（新增 `BackupVersioned`）、`internal/nginx/custom.go`（已有 `backupFile`/`pruneBackups` 可复用）、`web/src/views/ProxyRemoteView.vue`。
   - 价值：误改配置后可一键回到上一个可用版本，比现在的「删掉重部署」可靠得多。

2. **API Token**（学 beszel / nginx-ui）
   - 现状：97 条 API 路由只有会话 cookie 一种认证；脚本、HomeAssistant、手机快捷指令都调不动。
   - 做法：`api_tokens` 表（名称 + 哈希 + 权限范围 + 最后使用时间），`RequireAuth` 中间件同时接受 `Authorization: Bearer`，设置页做管理（新建时只显示一次明文，沿用现有「掩码往返」约定）。
   - 落点：`internal/api/middleware.go`、`internal/auth`、`web/src/views/SettingsView.vue`。
   - 注意：这是新增认证面，必须走一次安全自查（令牌哈希存储、撤销、只读范围）。

3. **通知渠道补齐国内常用通道**（学 ddns-go / ddns-updater）
   - 现状：邮件 / Webhook / Telegram。`web/src/constants/notifyProviders.ts` 里已有 `DEFAULT_BARK_SERVER`、`DEFAULT_NTFY_SERVER` 常量（说明预留了位置）。
   - 做法：补 Bark、ntfy、企业微信、钉钉、飞书（都是「POST 一个 URL + JSON」模式，可复用现有 Webhook 通道的发送路径）。
   - 落点：`internal/notify`、`web/src/constants/notifyProviders.ts`、`SettingsView.vue`。
   - 价值：NAS 用户最常用 Bark / 企业微信，成本极低。

4. **阈值型告警**（学 beszel）
   - 现状：通知全是事件型（DDNS 失败、证书到期、IP 频繁访问、登录失败、Nginx 重载失败），没有「CPU 持续 90%」「磁盘 85%」「隧道连续 3 次不通」这类。
   - 做法：复用 `internal/scheduler`（已有 `recover()` 与间隔校验）加一个指标巡检任务 + 冷却时间，阈值与开关走设置项（现有 `notify_*` 事件开关的模式可直接照搬）。
   - 落点：`internal/notify`、`internal/scheduler`、`internal/agent`（metrics 端点已存在，v1.2.4 加的）。

### P1 · 价值高、工作量中等

5. **指标历史与趋势图**（学 beszel）
   - 现状：VPS 资源、隧道流量都只有「当前值 + 最近 5 分钟内存采样」，重启即丢。
   - 做法：新增 `metrics_samples` 表（按 1 分钟落点 + 保留期设置），前端加 1h / 24h / 7d 切换（ECharts 与 `MiniTrafficChart` 已在用）。
   - 价值：NAS 场景最关心磁盘与带宽趋势，这是当前仪表盘最大的能力缺口。

6. **健康检查持续化 + 只读状态页**（学 uptime-kuma / beszel）
   - 现状：四段健康检查是「打开页面才算一次」，页面关掉就没有数据；`internal/agent/insights.go` 已有采样能力。
   - 做法：后台按间隔对每条反代规则做一轮检查并落库，连续失败 N 次才告警、恢复时再通知一次；可加一个可公开的只读状态页（按规则展示最近 24 小时可用率）。
   - 注意：状态页是**新增的匿名访问面**，必须单独设计（默认关闭、可设访问码）。

7. **定时自动备份 + 保留策略**（学 beszel / nginx-ui）
   - 现状：手动「导出 / 恢复」；恢复路径的校验与权限刚在 v1.2.7 收紧。
   - 做法：`scheduler` 加每日备份任务（写 `<DataDir>/backups`，保留 N 份，可选 WebDAV/S3 上传），设置页给保留份数。

8. **一键迁移包（加密导出 + 导入）**（学 nginx-ui 的加密配置导出）
   - 现状：只有「脱敏配置导出」（给用户看，不含密钥），换机器得手动重填服务端与证书。
   - 做法：导出含服务端 / 规则 / 证书 / 设置 / 密钥的加密包（口令派生密钥，用现有 `internal/secret`），导入时校验版本并落库。
   - 价值：换 NAS、重装系统、备份恢复三类场景都会用到。

9. **Docker 容器统计**（学 beszel）
   - 做法：agent 加只读 `GET /api/v1/docker`（容器列表 + CPU/内存/网络），公网服务端页加一张卡（`internal/sysinfo` 已有采集框架）。
   - 注意：agent 侧要显式限制只读、并处理「VPS 上没装 Docker」的降级。

### P2 · 可选

10. **只读账号 / OIDC**（学 pangolin）：给家人看仪表盘、或接入已有的 SSO。属权限模型改动，建议在 API Token 落地、认证面稳定后再做。
11. **DDNS 记录类型扩展**（学 ddns-go）：显式支持 AAAA / CNAME / 多记录批量，并在 UI 上区分。
12. **日志增强**：按域名 / 规则过滤、导出 CSV（现在有级别筛选、关键词搜索、分页与倒序）。
13. **规则模板与克隆**：常用规则（HTTP 反代 + Basic Auth + 限流）存为模板，新建时一键套用。

---

## 四、现有功能的优化点（对标后回看自己）

| 位置 | 可优化 | 依据 |
| --- | --- | --- |
| 健康检查 | 现在每轮 7 个接口并行（v1.2.8 已改并行）+ 15 秒轮询；可把「健康检查」单独拉长到 60 秒、其余保持高频 | beszel 对采集频率按指标分层 |
| 通知 | 事件型通知缺「冷却时间」，连续事件会连发（登录失败已有窗口统计，可推广到全部事件） | uptime-kuma / beszel 都有冷却与静默期 |
| 证书 | 续期失败后的重试与提醒策略值得复核（到期提醒已有） | nginx-ui 强调「自动续期 + 失败可见」 |
| 备份 | `internal/backup` 的写入方 `defer Close()` 不检查错误（审查报告 §二已列，P1 未做） | 与 nginx-ui 的「备份可用性」要求一致 |
| 配置生成 | 反代配置由模板生成是**优点**（保证同源），不要为了对齐 nginx-ui 而开放全文在线编辑 | 本项目刚在 v1.2.8 收敛模板重复，开放全文编辑会破坏该约束 |
| 前端 | 颜色令牌收敛（231 处）与 i18n 起步仍在待办 | 与对标项目无关，属自身债务 |

---

## 五、明确不建议跟进

1. **多租户 / 组织模型**（pangolin 面向团队）：Fonu 是个人 / 家庭 NAS 工具，引入组织、成员、资源级策略会让设置页与数据模型复杂度翻倍。
2. **Nginx 配置全文在线编辑**（nginx-ui 的路子）：会破坏「反代配置由模板同源生成」这一核心保证；保留现有「自定义片段」入口即可。
3. **集群 / 多节点镜像**（nginx-ui 的集群管理）：Fonu 的 app + agent 已是多节点模型，再叠一层集群属于重复抽象。
4. **MCP / AI 助手接入**（nginx-ui 新功能）：与项目定位不符，且每个新接口都是新的攻击面，收益不明确。
5. **把 frps 面板做成 frp-panel 那样的「多 frpc 集中管理」**：Fonu 的 frpc 就跑在本机 NAS 上（单一客户端 + 多服务端），不需要再管别人的 frpc。

---

## 六、建议实施顺序

1. 配置版本历史 + 回滚（P0-1）——与刚做完的 fsutil 收敛直接衔接，风险最低、收益最直观。
2. 通知渠道补齐 + 阈值告警（P0-3、P0-4）——复用现有通知与调度框架，改动集中在 `internal/notify`。
3. API Token（P0-2）——单独一轮，配一次安全自查。
4. 指标历史与趋势图（P1-5）+ 健康检查持续化（P1-6）——需要新表与后台任务，建议同一轮做（共用落点与保留期设置）。
5. 其余按需。

## 七、第二批方向（第二轮检索补充）

上一节偏「同域直接对标」；这一批来自相邻领域（零信任访问、状态页、加密备份、NAS 硬件监控、SSO/MFA、服务面板），都是 Fonu 已有模块的**能力延伸**，不需要新架构。

| 借鉴点 | 出处 | 落到 Fonu 哪里 | 价值 |
| --- | --- | --- | --- |
| **API Token 的到期 / 刷新 / 新旧密钥宽限期** | Cloudflare Zero Trust 的 Service Token | 补强 §三-P0-2：令牌加 `expires_at`、可刷新延长、轮换时旧密钥保留一段宽限期 | 让脚本 / HomeAssistant 的接入既可撤销又不会一改就断 |
| **状态页 + 事故（incident）记录** | Gatus、Uptime Kuma | 与 §三-P1-6 合并：持续健康检查落库后，除「可用率」再输出「不可用区间」列表（什么时间、哪条规则、多久） | 「昨晚为什么打不开」这类问题有处可查 |
| **Prometheus `/metrics` 导出** | Gatus | 新端点，把已有指标（VPS 资源、隧道流量、健康状态、证书剩余天数）按 Prometheus 文本格式暴露 | 一行配置就能接入 Grafana / HomeAssistant，比自建图表性价比高 |
| **事件订阅（webhook 推事件）** | Cloudflare 的 Event subscriptions | 现有通知是「发给人」；再加「推给系统」（把规则部署 / 证书续期 / 漂移检测事件 POST 到外部 URL） | 对接自动化（如部署后触发 CDN 刷新） |
| **2FA / TOTP + 登录设备与会话列表** | Authelia、Authentik | 补强 §三-P2-10：TOTP 绑定（二维码 + 一次性恢复码）、已登录设备列表与「踢下线」、登录历史 | Fonu 面板暴露在公网时，单密码是最大的残留风险；且与 v1.2.8 的「改密失效其他会话」天然衔接 |
| **审计日志**（谁、何时、改了什么） | Authelia、Authentik | 新增 `audit_logs` 表，在服务端 / 规则 / 证书 / 设置的写入口记一条（含来源 IP） | 多设备 / 多人使用时能回溯误操作 |
| **备份保留策略（GFS）+ 完整性校验** | restic / kopia | 补强 §三-P1-7：保留策略改成 `keep-daily/weekly/monthly`；恢复前先校验包内条目（数量 / 大小 / 时间）再落盘 | 备份不会因「保留太多份」撑爆磁盘，也不会拿半个包去覆盖现有数据 |
| **本机 SMART 磁盘健康 + UPS 状态** | NAS Doctor、Scrutiny、PeaNUT | 扩展 `internal/sysinfo`（本机侧，不经 agent）：磁盘 SMART（温度 / 重分配扇区 / 待处理扇区 / 通电小时）、UPS 电量与剩余时间 | NAS 用户的真痛点：硬盘将坏、市电断了；目前仪表盘只有 CPU / 内存 / 磁盘占用 |
| **服务入口墙** | Homepage、Homarr、Glance | 仪表盘新增一区：把启用中的反代 / 穿透规则按域名聚合成可点击卡片（域名、类型、健康点），复用现有 `openDomain()` | NAS 用户装面板就是为了「一处点开所有服务」 |
| **移动端 PWA + 命令面板** | Homepage、Dashy（kiosk 模式） | `manifest.webmanifest` + 图标，让手机「加到主屏」；再加 Cmd/Ctrl+K 跳页面 / 搜规则 | 手机上用得多，成本很低 |

**这一批里我建议先做的两项**：

1. **本机 SMART + UPS**（上表第 8 行）——Fonu 面向 NAS，而当前仪表盘完全没覆盖硬件健康；`internal/sysinfo` 已有平台分文件（`_linux.go` / `_windows.go`）的框架，加采集与一张卡即可，不碰 agent。
2. **Prometheus 导出**（第 3 行）——把已有指标换个输出格式就能接 Grafana / HomeAssistant，比自建历史图表（§三-P1-5）成本低一个量级，两者可先做这个。

**仍然不建议跟的**（补充）：把 Fonu 变成通用监控平台（Gatus / Uptime Kuma 那种「任意 URL 探活」）——它的健康检查应该围绕自己下发的反代规则，而不是变成一个通用探活器；引入 Redis 会话存储（Authelia / Authentik 的选择）也没必要，单机 SQLite 足够。

---

## 八、待评估：读取 VPS 实际放行端口（只读）

> 背景：现在 Fonu 只能**告诉用户该放行哪些端口**（frps 同源配置弹窗按已启用的 TCP/UDP 规则推导端口列表，`internal/frp/frps_template.go` 也生成 `allowPorts`），但读不到服务器上**实际放行**了什么。隧道连不上时最常见的原因正是端口未放行，而日志不会点明。

### 结论

**主机侧防火墙可以读，云厂商安全组读不到。**

| 来源 | 可行性 | 做法 |
| --- | --- | --- |
| `ufw`（Debian / Ubuntu） | ✅ | `ufw status numbered`（agent 以 root 运行，无需额外提权） |
| `firewalld`（RHEL / CentOS） | ✅ | `firewall-cmd --list-ports` / `--list-services` / `--list-rich-rules` |
| `nftables` / `iptables` 兜底 | ◐ | `nft list ruleset` / `iptables-save`——是 ufw/firewalld 的底层，直接读会看到大量实现细节，只作为「没有前两者时」的兑底 |
| **云厂商安全组**（本机是 RainYun） | ❌ | 安全组在厂商控制台 / API，**主机侧不可见**；要读就得引入 AccessKey（新凭据面，不建议） |
| 上游 NAT / 网络设备 | ❌ | 不在主机视野内 |

### 设计（只读，不提供任何写操作）

1. agent 新增 `GET /api/v1/firewall`（沿用 `s.auth`）：返回 `{backend, rules: [{port, proto, source, action}], missing_tool, checked_at}`；先探测哪个前端存在，命令全部**固定 argv**（与现有 `systemctl` / `ss -tlnp` 的做法一致，不拼用户输入）。
2. 应用侧 `AgentClient.Firewall()` + `GET /api/frpmulti/servers/{id}/agent/firewall`，与现有 agent 路由同构（`internal/api/router.go`）。
3. 前端把「需要放行的端口」（现有规则推导逻辑）与「实际已放行」做**交叉对照**：直接标出「已放行 / 未放行」，并固定一行说明「云安全组需在厂商控制台确认（主机侧读不到）」。落点：`web/src/views/AgentView.vue`（frps.toml 弹窗与详情区）与 `FrpView.vue` 的规则区提示。
4. 与已有数据交叉验证：agent 现在已在采集 `ss -tlnp` 的监听端口，可额外区分「已放行但无服务监听」与「有服务监听但未放行」。

### 边界与风险

- **只读**：不提供任何防火墙写操作，与 agent 能力方案 §六「防火墙规则读写不做（最多只读）」一致。
- **降级**：命令不存在 / 无权限 / 无规则三种情况都要能降级成「未检测到防火墙管理工具」而不是报错。
- **不引入云 API**：接厂商 API 意味着用户要交出 AccessKey，收益（省一次控制台确认）不值这个代价。
- **agent 行为变更**：需递增 `AgentVersion`（0.12.2）并重编双架构内嵌二进制，VPS 上点「升级 Agent」才生效。

---

## 九、P0 实施状态（2026-09-24）

### 已完成：配置版本的后端基础

| 改动 | 位置 | 说明 |
| --- | --- | --- |
| `VersionsDir` / `BackupVersioned` / `pruneVersions` | `internal/fsutil/fsutil.go` | 版本存为 `<path>.versions/<UTC 时间戳.毫秒>-<随机 4 位>.conf`，按文件名倒序即新旧顺序；`keep <= 0` 不剪枝；文件不存在视为无需备份 |
| 接入 Nginx 配置写入 | `internal/nginx/nginx.go` 的 `Apply` | 覆盖前先 `.bak`（保留最近一次，兼容既有读取方），再 `.versions/` 保留最近 10 份 |
| 测试 | `internal/fsutil/fsutil_test.go` | 新增 3 例：内容正确、剪枝到 keep 份、文件不存在返回空且不报错 |

验证：`gofmt` 干净、`go build ./...` / `go vet` / `go test ./internal/fsutil/ ./internal/nginx/` 全绿。

### 已完成：版本列表 / 差异 / 回滚（本机侧与 agent 侧都做了）

| 改动 | 位置 |
| --- | --- |
| `ListVersions` / `ReadVersion`（名称校验拒绍上跳）、`BackupVersioned` 对相同内容去重 | `internal/fsutil/fsutil.go` + 4 个测试 |
| `ReadConfig` / `ListConfigVersions` / `ReadConfigVersion` / `RollbackConfigVersion`（先语法校验 + `nginx -t`，不过关不覆盖）；把 Apply 的「让它生效」抽成 `activate` 供回滚复用 | `internal/nginx/versions.go`、`nginx.go` |
| `GET /api/settings/nginx/config/versions`、`GET …/{name}`、`POST …/{name}/rollback` | `internal/api/nginx_handler.go`、`router.go`、`internal/service/nginx_config.go` |
| 设置页「Nginx 全局配置」卡内新增「主配置版本与回滚」：版本列表 + 逐行差异 + 二次确认回滚 | `web/src/views/SettingsView.vue` |
| agent 侧：写 vhost 前留版本；**校验失败改为恢复上一个可用版本**（此前直接删文件，等于把站点从磁盘上抹掉）；新增 `GET /routes/{domain}/versions`、`GET …/versions/{name}`、`POST …/versions/{name}/rollback`；删路由时一并清版本目录 | `internal/agent/versions.go`、`server.go`（`AgentVersion` → **0.12.2**，双架构二进制已重编） |
| 应用侧：`AgentClient.RouteVersions/RouteVersion/RollbackRouteVersion` + 三个 `/api/frpmulti/servers/{id}/agent/routes/{proxyID}/versions…` 路由（端点不存在时提示「需 ≥ 0.12.2」） | `internal/frp/agent_client.go`、`internal/api/frpmulti_handler.go`、`router.go` |
| 前端：公网反代页「查看配置」弹窗内加版本面板 + 差异高亮 + 回滚二次确认 | `web/src/views/ProxyRemoteView.vue`、`components/ConfigDiffView.vue`、`utils/diff.ts` |

#### 原计划（已按此实现，保留作对照）

1. `internal/fsutil`：加 `ListVersions(path) ([]VersionEntry, error)`（名称 + 大小 + 修改时间，新的在前）与 `ReadVersion(path, name) ([]byte, error)`（拒绝带 `/` 或 `..` 的名称）。
2. `internal/nginx`：`ListConfigVersions()` / `ReadConfigVersion(name)` / `RollbackConfigVersion(name)`——回滚必须先 `ValidateConfigSyntax` + `m.validate(ctx, tmp)` 通过再写回并 reload，失败不覆盖当前配置（与 Apply 同构）。
3. `internal/api`：`GET /api/nginx/versions`、`GET /api/nginx/versions/{name}`、`POST /api/nginx/versions/{name}/rollback`（受保护路由，写入口径同现有 nginx 接口）。
4. 前端：本地反代页 Nginx 配置区 + 公网反代页——版本列表（时间 / 大小）+ 逐行差异高亮（复用漂移清单的样式）+ 回滚二次确认。
5. 边界：本轮只做**本机侧**；VPS 上的 `fonu-<域名>.conf` 版本历史属于 agent 侧，需单独一轮（agent 写路由时也调 `BackupVersioned`，再配一个只读列表端点）。

### 已完成：其余三项 P0（明细见文末「本轮完成明细」）

#### 原计划（已按此实现，保留作对照）

- **API Token**：表 `api_tokens(id, name, token_hash, scopes, last_used_at, expires_at, created_at)`；`RequireAuth` 同时接受 `Authorization: Bearer`；新建时只显示一次明文（沿用现有「掩码往返」约定）；设置页管理；落地前过一遍安全自查（哈希存储、可撤销、只读范围、`last_used_at` 审计）。建议单独一轮。
- **通知渠道**：现有 email / webhook / telegram 三通道的抽象（`SaveInput` / `ParseSaveInputFromMap` / `validateSaveInput`）→ 新增 Bark / ntfy / 企业微信 / 钉钉 / 飞书（都是 POST URL + JSON）；`web/src/constants/notifyProviders.ts` 已有 `DEFAULT_BARK_SERVER` / `DEFAULT_NTFY_SERVER` 常量可用。
- **阈值告警**：`internal/scheduler` 加指标巡检任务（数据源：agent `GET /api/v1/metrics` + 本机 `internal/sysinfo`）→ 阈值 / 开关 / 冷却时间走设置项（照搬 `notify_*` 事件开关模式）；冷却状态可先放内存（重启丢可接受）。

建议顺序：版本回滚 → 通知渠道 → 阈值告警 → API Token（本轮已全部完成）。

### 本轮完成明细

**通知渠道补齐**

- Bark / ntfy / Gotify **本来就有**（上文「只有邮件 / Webhook / Telegram」不准确）；本轮补的是**企业微信机器人、钉钉机器人、飞书机器人**：固定路径 + JSON，`Key` 存 `Webhook.Key`、`Server` 可覆盖默认域名。
- 钉钉正文固定以「Fonu · 」开头，机器人用「自定义关键词 = Fonu」即可命中（加签方式暂不支持，UI 已注明）。
- `providerUsesKey()` 收敛了「凭据放 Key 还是 WebhookSecret」的判定，保存与落库共用一处。
- 前端：预设下拉 + 三家各自的字段与提示；**切换预设时服务地址自动换成该预设默认值**（否则会把 `api.day.app` 带到企业微信上）。
- 测试：`push_test.go` 锁定三家的端点与 payload 形状。

**阈值型告警**

- 新包 `internal/monitor`：`Checker.Run` 每 2 分钟跑一轮（`internal/app/app.go` 注册为 scheduler 任务）。
  - 资源：CPU / 内存 / 磁盘超阈值告警，同一项 30 分钟静默期，回落到阈值以下即清冷却（再次超标可立即再报）。
  - 隧道：连续 N 次巡检未在 frps 上注册才告警（默认 3 次），**只在刚好达到阈值与恢复时各发一次**，不刷屏。
- 事件与设置项：`on_resource_threshold` / `on_tunnel_down`（默认**关闭**，升级后不改变现有用户收到的通知）+ `notify_cpu_threshold`(90) / `notify_memory_threshold`(90) / `notify_disk_threshold`(85) / `notify_tunnel_fail_threshold`(3)；开关关闭时连采集与探测都跳过。
- 前端：告警事件新增「资源占用过高」「隧道连续不通」两项与对应阈值输入。
- 测试：`monitor_test.go` 覆盖「只报一次 / 恢复报一次 / 未达阈值静默」。

**API Token**

- 迁移 `025_api_tokens.sql`：`api_tokens(id, name, token_hash, scope, expires_at, last_used_at, created_at)`，`token_hash` 是明文令牌的 SHA-256（令牌是 `crypto/rand` 的 32 字节随机串，熵足够，不需要 bcrypt）；明文只在创建响应里回一次。
- `RequireAuth` 同时接受会话 cookie 与 `Authorization: Bearer <token>`；`read` 范围只允许 GET/HEAD，`write` 与会话同权。
- **安全自查**：① 令牌管理（`/api/settings/tokens`）与改密 / 退出**只允许登录会话**——否则一个 write 令牌能给自己提权或无限续期；② 撤销即删行、立即失效；③ 可选有效期（最长 3650 天，空为永不过期）；④ `last_used_at` 限频回写（1 分钟一次，单连接 SQLite 上不做每请求写）。
- 接口：`GET/POST /api/settings/tokens`、`DELETE /api/settings/tokens/{id}`；设置页「高级」新增「API Token」卡：列表（名称 / 权限 / 创建 / 最近使用 / 有效期）+ 创建表单 + 明文一次性展示与复制 + 撤销确认。
- 测试：`internal/auth/token_test.go`（生命周期 / 过期 / 非法输入 / 明文不落盘）、`internal/api/middleware_test.go`（只读令牌只读、写令牌可写、令牌不能管理令牌、错误凭据 401）。

**本轮验证**

`gofmt`（改动文件干净）、`go build ./...`、`go vet ./...`、`go test ./... -count=1` 全绿；`npx vue-tsc --noEmit` 0 错误、`npx vite build` 成功；`internal/frp/agentbin/` 双架构二进制已重编（含 `0.12.2`）。

**待用户操作**

1. **重启本机 `bin\fonu.exe`** 才能看到新接口与新 UI（新迁移在启动时自动执行）。
2. **VPS 上点「升级 Agent」** 才会装上 0.12.2，公网反代页的版本历史才会出现（否则提示「需 ≥ 0.12.2」）。
3. 已随本轮提交：`CHANGELOG.md` 新增 v1.2.9 段，`VERSION` 与 `web/package.json` 同步到 1.2.9。

## 十、A 档告警落地（2026-09-25）

> 承接 §三-P0-4 的阈值告警：把「隧道不通」这一族故障的三个常见成因补齐。**纯应用侧改动，agent 无改动**（`AgentVersion` 保持 0.12.2，无需在 VPS 上重新升级）。

| 新事件 | 判定 | 默认 | 限流 |
| --- | --- | --- | --- |
| 公网服务端掉线 `frps_server_down` | `DesiredState` 为 running/starting 时，进程 stopped/error 或连接 disconnected/error | 关闭 · 连续 2 次 | 达标报一次 + 恢复报一次 |
| 公网 agent 不可达 `agent_unreachable` | agent 地址与令牌都可用，但 `GET /api/v1/ping` 连续失败 | 关闭 · 连续 3 次 | 同上 |
| 配置漂移 `route_drift` | `RouteDrift` 报告有 items（未部署 / 孤儿配置 / 未生效 / 内容不一致） | 关闭 · 24 小时 | 首次即报 + 每 24h 重复 + 清空报恢复 |

**避免误报的三条约定**：① 服务端只在「用户要它运行」时判定，手动停掉的不报；② `ConnectionState=unknown` 按健康处理（状态源 `manager+log` 偶尔读不到不代表掉线）；③ 没配 agent 的服务端不参与可达性判定。

**结构变化**：`internal/monitor` 的注入方式从「一个 routes 闭包」改成 `Source` 接口（Routes / Servers / Agents / Drift），frp 侧适配放在新文件 `internal/app/monitor_source.go`——monitor 依旧不依赖 frp 包，新增数据源只需扩接口。三条新检查与原有隧道巡检共用 `failureTransition`（原 `tunnelTransition` 泛化），漂移因不是「连续失败」型而单独用冷却；`notify.Alert` 一律在锁外调用。

---

> 说明：本文件是**建议清单**，未改动任何代码；实施时按本仓库既有约定（每步验证 `go build/vet/test` + `vue-tsc/vite build`、agent 行为变更递增 `AgentVersion`、提交同批更新 `CHANGELOG` 与 `VERSION`）。
