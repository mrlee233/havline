# 上游同步方案：Havline 1.0.2 → 1.0.5 对照与可借鉴项

> **时效说明（2026-09-24）**：文中对 `/frp` 页现状的描述基于 2026-09-23，已被 v1.2.6 的重构推翻（现为**左服务端列表 + 右规则表格**，服务端增删改已并入 `/frp-agent`）。

> 本项目基于**上游 v1.0.2**（2026-09-18）开发，上游已迭代到 **v1.0.5**（2026-09-23）。
> 本文逐版本列出上游 1.0.3 / 1.0.4 / 1.0.5 的全部变更，并逐条核对**本项目代码现状**，
> 给出「同步 / 已具备 / 不适用 / 待核对」的判定与落地建议。
>
> 上游来源：`https://github.com/O96u/Fonu`（CHANGELOG.md + 提交记录）
> 核对方式：对本仓库 `f:\Havline-main\Havline-1.0.2` 做 grep / 文件树核对（证据写在每条的「本项目现状」里）
> 更新日期：2026-09-23

---

## 一、结论速览

| # | 上游项 | 版本 | 本项目现状 | 建议 |
| --- | --- | --- | --- | --- |
| 1 | 非 HTTPS 环境复制到剪贴板失败 | 1.0.4 | **同样存在**：6 处 `navigator.clipboard`（5 个文件） | **P0 同步**：抽 `utils/clipboard.ts` 加降级 |
| 2 | 非 HTTPS 添加 TCP 隧道失败（`crypto.randomUUID`） | 1.0.4 | **不适用**：全仓 `randomUUID` = 0 处 | 不适用（附一条 `crypto.subtle` 复核项） |
| 3 | 日志中心新增「FRP 日志」Tab（解析 frpc 日志） | 1.0.3 | **缺口**：`LogsView.vue` 只有 系统 / 访问 / Nginx 三个 Tab | **P1 同步**（后端接口已具备，纯前端） |
| 4 | 通知：单通道密钥解密失败不拖垮其它通道 | 1.0.5 | **确凿缺口**：`store.go:105-108` 解密失败即 return，整份配置加载失败 | **P1 同步修** |
| 5 | 通知时间按系统时区显示 | 1.0.4 | **条件缺口**：`format.go` 用 `time.Now()`，但容器未设 `TZ` → Docker 下显示 UTC | P1 补 `TZ` |
| 6 | frps 参考配置自动生成 `allowPorts` + 防火墙放行提示 | 1.0.3 | **已具备**：`internal/frp/frps_template.go` 已生成 `allowPorts` | 已具备；仅可补「VPS 防火墙放行」UI 提示 |
| 7 | TCP 远程端口手动指定支持 1–65535 | 1.0.5 | **已具备**：`FrpView.vue:336` 已是 `:min="1" :max="65535"` | 已具备；后端随机范围（1024–65535）待核 |
| 8 | FRP 页重构为「状态卡 + Web/TCP/高级/运行日志分栏」 | 1.0.3 | **方向已对齐**：本轮刚改为标准统计区 + 卡片分区 | 不照搬（形态差异大，见 §三.1） |
| 9 | 独立 TCP 隧道列表 / 纯 TCP 模式无需域名 | 1.0.3 | **功能已具备**：规则列表支持 tcp/udp/http/https 等类型 | 可选做「按类型筛选」，不照搬分栏 |
| 10 | 反代：新建入口时校验监听端口占用 | 1.0.5 | **已具备且语义更准**：`ensureHostAvailable` 校验「同域名 + 同端口」；仅缺冲突规则名与 custom 模式覆盖 | P3 收尾（详见 §八） |
| 11 | 反代：复制改为「预填表单、不改域名后缀」 | 1.0.5 | **部分对齐**：已是 `openDuplicate` 打开表单，但 `duplicateName()` 仍加「-复制」 | P3 去后缀 |
| 12 | 反代：子表撑满卡片 / 域名后紧跟复制·打开 / 状态列仅保留开关 | 1.0.5 | 本地布局不同（本轮未涉及该页） | P3 视觉可选对齐 |
| 13 | Telegram 代理支持 socks5 + 自动补 `http://` | 1.0.3 | **代理已具备**（`telegram.go:43-51`，Go 原生支持 socks5）；缺保存校验与自动补前缀 | P3 补校验 |
| 14 | 通知事件开关保存与前端状态同步更可靠 | 1.0.5 | **未定位到缺陷**：`service.go:210` `send()` 为单一出口，未复现上游描述的问题 | 保留观察 |
| 15 | 连接设置：地址与端口合并一行 / 卡片内重启按钮 | 1.0.3 | 本地 FrpView 已有服务端启停按钮 | P3 视觉可选 |

**优先级**：P0 = 剪贴板降级（真实功能缺陷，且本轮新增的复制按钮也在其中）；P1 = FRP 日志 Tab、通知健壮性两条；P2/P3 = 核对项与视觉对齐。

---

## 二、逐版本明细

### v1.0.3（2026-09-21）

**新增**

1. **FRP：支持独立 TCP 隧道列表（如 SSH、数据库），可与 Web 网关同时使用；纯 TCP 模式无需配置穿透域名**
   - 本项目现状：`FrpView.vue` 的规则表单已支持 `tcp / udp / http / https / tcpmux / stcp / sudp / xtcp`（`proxyForm.type`），且 `remote_port` 仅对 tcp/udp 显示（`FrpView.vue:336`）→ **能力已具备**。
   - 差异：上游是「Web / TCP 分栏」的信息架构；本项目是「统一规则列表 + 类型字段」。本项目的公网反代链路（`ProxyRemoteView` → agent → Nginx）依赖 http 规则 + 自定义域名，改成上游式分栏会牵动两页交互。
   - 建议：**不照搬分栏**；可选做「规则列表按类型筛选」的小改进（纯前端，低风险）。

2. **FRP：允许先保存连接设置（地址/端口/Token），待配置域名或 TCP 隧道后再启动 frpc**
   - 本项目现状：服务端保存与 frpc 启动是分离的（`createFrpServer` 与 `startFrpServer` 独立），但未核对「无规则时是否允许启动」。
   - 建议：列入 §四 待核对；若本地已有拦截，则对齐为「允许保存、启动前给提示」。

3. **FRP：frps 参考配置自动生成 `allowPorts`，并提示在 VPS 防火墙放行 TCP 远程端口**
   - 本项目现状：`internal/frp/frps_template.go` **已生成 `allowPorts`**（grep 命中）。
   - 缺口：缺少「请在 VPS 防火墙放行 TCP 远程端口」的 UI 提示。
   - 建议：**P2**，在 frps 配置弹窗（`showFRPSConfig`）与规则保存后各加一句提示（纯文案 + 1 个列表推导）。

4. **日志中心：新增 FRP 日志 Tab，解析 frpc 运行日志（时间、级别、来源、消息）**
   - 本项目现状：`LogsView.vue` 现有 Tab = `系统日志 / 访问日志 / Nginx 日志`（第 11 / 40 / 72 行）→ **无 FRP Tab**。
   - 后端：已有 `getFrpLogs` / `getFrpProxyLogs`（`api` 命中），日志内容可得。
   - 建议：**P1 同步**。落地方式：`LogsView.vue` 增加第 4 个 Tab + 解析器（时间/级别/来源/消息四列，复用 `formatLogTime` / `formatLogLine` 与现有筛选控件），后端若已有 `/api/frp/logs` 则纯前端。

**改进**

5. **FRP：内网穿透页重构为「状态卡 + 连接设置 + Web/TCP/高级/运行日志分栏」**
   - 本项目现状：本轮（见提交 `70796e5`）刚把 4 张自造状态卡改为标准 `.stats-row/.stat-card`，并把 6 个弹窗统一到 `.frp-card-modal` 外壳 → **视觉方向已对齐**。
   - 建议：不照搬分栏（理由同 §2.1），但可借鉴其「运行日志就近展示」的思路——本地运行日志已在弹窗与日志中心可查。

6. **FRP：状态区展示已配置 Web 域名与 TCP 隧道，标签单行展示，超出可跳转对应 Tab**
   - 本项目现状：`stats-row` 第三张卡是「frpc 版本」，未展示「已配置域名/隧道」。上游把这两类信息提到状态区。
   - 建议：**P2 可选**。若要做，建议把第 4 张卡「最近应用」换成「已配置入口（N 个 Web / M 个 TCP）」，点击跳规则列表并筛选。

7. **FRP：连接设置中服务器地址与端口合并为一行；左侧卡片内提供重启 frp 服务按钮**
   - 本项目现状：服务端行操作列已含「启动/停止」（`serverColumns` 操作列），但无「重启」。
   - 建议：**P3**。加一个「重启」按钮（`stopServer` → `startServer` 串联或后端 action），与上游一致。

8. **通知：Telegram 代理地址支持 socks5，保存时校验格式并自动补全 `http://` 前缀**
   - 本项目现状：`internal/notify/telegram.go` 存在；是否支持 socks5 未核对。
   - 建议：列入 §四 待核对。

### v1.0.4（2026-09-21）

9. **修复：非 HTTPS 访问时添加 TCP 隧道失败（`crypto.randomUUID` 不可用）**
   - 本项目现状：**全仓 `randomUUID` 命中 0 处**（`*.vue` 与全仓两次核对均为 0）→ **不适用**。
   - 附注：仍需确认无 `crypto.subtle`（同样要求安全上下文）的使用；`crypto.getRandomValues` 不受限。

10. **修复：非 HTTPS 环境下复制到剪贴板失败（内网穿透、DDNS、反向代理）** ← **本项目同样存在**
    - 本项目现状：`navigator.clipboard` 共 **6 处 / 5 个文件**：
      `ProxyRemoteView.vue`（2 处，本轮新增的「复制域名」「复制反代配置」）、
      `AgentView.vue`（1 处，「配置查看」弹窗的复制）、
      `DdnsView.vue`（1 处）、`ProxyView.vue`（1 处）、
      归档文件 `FrpView-1.0.2.vue`（1 处，已弃用代码，可不动）。
    - 影响：Havline 常用 `http://内网IP:6893` 访问，属**非安全上下文**，此时 `navigator.clipboard` 为 `undefined` → 复制功能直接失败（本项目现在只弹「复制失败」提示，用户无从下手）。
    - 建议：**P0 同步**。做法：新增 `web/src/utils/clipboard.ts`：
      ```ts
      export async function copyText(text: string): Promise<boolean> {
        // 非安全上下文（http:// 内网 IP）下 navigator.clipboard 不存在，回退到 execCommand
        if (navigator.clipboard?.writeText) {
          try { await navigator.clipboard.writeText(text); return true } catch { /* 落到回退 */ }
        }
        try {
          const el = document.createElement('textarea')
          el.value = text; el.setAttribute('readonly', ''); el.style.position = 'fixed'; el.style.opacity = '0'
          document.body.appendChild(el); el.select(); document.execCommand('copy'); document.body.removeChild(el)
          return true
        } catch { return false }
      }
      ```
      然后 5 个调用点统一改为 `copyText(...)`，失败提示保留。

11. **修复：通知消息时间未按系统设置时区显示（如 Telegram 测试通知）**
    - 本项目现状：`internal/notify/format.go` 存在；是否用 `time.Local` / 配置时区未核对。
    - 建议：列入 §四 待核对；修法一般是 `time.Now()` → 按系统时区格式化（或统一走一个 `nowInSystemZone()` 帮助函数）。

### v1.0.5（2026-09-23）

**新增**

12. **反代：新建入口时校验监听端口占用，避免重复端口静默冲突**
    - 本项目现状（已核实）：**已具备**。`internal/proxy/proxy.go:499 ensureHostAvailable` 在保存前查 `proxy_hosts` 的「同域名 + 同端口」，`proxy_hosts` 表另有 UNIQUE 约束兜底。
    - 语义差异：上游按「端口」判冲突，本项目按「端口 + 域名」判冲突——后者才是 Nginx 的真实冲突（同端口不同域名是合法的虚拟主机，按端口判会误报）。
    - 残留缺口：错误提示不含冲突规则名；`nginx_mode = custom` 的规则不参与校验。详见 §八。

**改进**

13. **反代：入口/规则「复制」改为打开表单预填原内容，由用户确认后再保存（不再自动创建、不改域名后缀）**
    - 本项目现状：`ProxyView.vue` 已有复制相关实现（grep 命中 9 处），行为是否「直接创建」未核对。
    - 建议：**P2 核对**；若本地是「点击即创建副本」，按上游改为「预填表单」，避免误建规则。

14. **反代：入口分组子表布局撑满卡片宽度；域名后紧跟复制/打开链接；状态列仅保留开关**
    - 本项目现状：`ProxyView.vue` 无「入口分组」概念（grep `分组` = 0）→ **子表撑满**项不适用；「域名后紧跟复制/打开链接」与「状态列仅保留开关」是纯视觉项。
    - 建议：**P3 可选**。注意：本轮刚在 `ProxyRemoteView` 的域名列做了「域名 + 复制 + 打开」的同类交互（见提交 `ca309db`），若要对齐，可复用同一套 `copyText` + 图标按钮样式。

15. **反代：同端口监听配置冲突提示仅在配置确实不一致时显示**
    - 本项目现状：未核对本地是否有该提示（`ProxyView.vue` 的冲突提示逻辑）。
    - 建议：**P2 核对**（误报会干扰用户）。

16. **FRP：TCP 隧道远程端口手动指定支持 1–65535（0 仍为自动分配，随机范围 1024–65535）**
    - 本项目现状：**前端已具备**（`FrpView.vue:336` `:min="1" :max="65535"`）。
    - 待核：后端「0 = 自动分配」的随机范围是否为 1024–65535（`internal/frp` 内）。
    - 建议：核对后如有差异，改一行常量。

17. **通知：各通道密钥解密失败不再拖垮其它通道；事件开关保存与前端状态同步更可靠**
    - 本项目现状：`internal/notify/store.go` 有 10 处解密相关逻辑 → 需要看「单通道解密失败是否被上层吞掉/中断整轮发送」。
    - 建议：**P1 核对后修**。典型修法：每个通道独立 `try/catch`（Go 里是错误收集而非 return），把失败通道记入日志与结果，继续发其它通道。

**修复**

18. **通知：旧 SMTP 等密文无法解密时，Telegram/Webhook 等已配置通道仍无法告警的问题**
    - 与第 17 条同源，**同一处修改可一并解决**。

---

## 三、不照搬项及原因

1. **FRP 页「Web / TCP / 高级 / 运行日志」分栏**：本项目的公网反代链路（`ProxyRemoteView` → agent → VPS Nginx）依赖「http 规则 + 自定义域名 + 部署态」这一套语义；照搬分栏会让「规则 → 部署 → 健康」的入口被打散。当前「统计区 + 服务端表 + 规则表」的形态更贴合本项目已有能力。
2. **`crypto.randomUUID` 修复**：本项目未使用该 API（全仓 0 处），无需移植。
3. **入口分组子表撑满**：本项目 `ProxyView` 没有「入口分组」结构，不适用。

---

## 四、逐项核实结果（A–H，全部已读代码确认，无推测）

| # | 上游项 | 核实结论 | 证据 |
| --- | --- | --- | --- |
| A | 允许先保存连接设置、待有域名/TCP 隧道再启动 frpc | **已具备**：本项目没有任何「必须有规则」的拦截 | `internal/frp/service.go:385` `StartServer` 仅做 GetServer → setServerRuntime("running") → enrich 返回；`FrpView.vue:1118` 启动按钮无 disabled 条件 |
| B | Telegram 代理支持 socks5 + 保存时校验格式、自动补 `http://` | **代理已具备（含 socks5）**；**缺保存校验与自动补全** | `internal/notify/telegram.go:43-51` 已有 `cfg.Telegram.ProxyURL` + `http.Transport{Proxy: http.ProxyURL(parsed)}`（Go 原生支持 `socks5://`） |
| C | 通知时间按系统时区显示 | **条件缺口**：代码用 `time.Now()` 格式化（跟随进程 TZ），但**容器未设 TZ** → Docker 部署下显示 UTC | `internal/notify/format.go:9/30`；`Dockerfile` 与 compose 无 `TZ` 环境变量 |
| D | 单通道密钥解密失败不拖垮其它通道 | **确凿缺口，与上游同一 bug** | `internal/notify/store.go:105-108`：SMTP 密文解密失败即 `return Config{}, runtimeSecrets{}, secret.DecryptHint(err)` → **整份配置加载失败**，Telegram/Webhook 也拿不到配置 → 无法告警；`store.go:117` 的 webhook 同理 |
| E | TCP 远程端口 0=自动分配（随机 1024–65535） | **本项目无此功能**：前端强制手填 1–65535，后端只做范围校验 | `FrpView.vue:336` `:min="1"`；`internal/frp/validate.go:331` `port > 0 && port <= 65535`；全仓未找到随机分配逻辑 |
| F | 反代：新建入口时校验监听端口占用 | **已具备（原判为「缺口」是关键词 grep 误判）**：校验存在但文案为「已被其他规则使用」，故搜 `占用`/`in use` 无命中 | `internal/proxy/proxy.go:499-524 ensureHostAvailable/ensureHostsAvailable`（`Create` 调用于 :225、`Update` 调用于 :345/:367）；表约束见 `replaceHosts:526` 的 UNIQUE 分支；错误经 `api/proxy_handler.go:87 writeError(400, err.Error())` 直达前端 |
| G | 反代：复制改为「预填表单、不改域名后缀」 | **部分对齐**：已是打开表单预填，但仍自动加「-复制」后缀 | `ProxyView.vue:148` `@click="openDuplicate(selectedRule)"`（打开而非直接创建）；`ProxyView.vue:1364-1366` `duplicateName()` 追加 `'-复制'` |
| H | 是否存在 `crypto.subtle`（同样要求安全上下文） | **不存在**（0 处） | 全仓扫描 0 命中 |

### 核实后的关键结论

1. **真正要修的只有两处**：剪贴板降级（P0，6 处隐患）、通知单通道解密失败（P1，`store.go` 直接 return 导致整份配置不可用）；
2. **两处是「条件性」的**：通知时区（仅容器未设 TZ 时出现）、反代复制后缀（行为差异，非缺陷）；
3. **四处本项目不缺**：无规则启动、Telegram 代理（含 socks5）、远程端口范围校验、`crypto.subtle`；**一处无对应功能**：远程端口 0=自动分配。

### 原待核对清单（索引保留）
## 五、建议的分批实施

| 批次 | 内容 | 影响面 | 预计 |
| --- | --- | --- | --- |
| **P0** | 剪贴板降级：新增 `utils/clipboard.ts` + 改 5 处调用点 | 前端 6 文件（含本轮新增的复制按钮） | 0.5h |
| **P1-a** | 日志中心「FRP 日志」Tab（四列解析 + 复用筛选） | `LogsView.vue`（+ 可能 1 个后端接口确认） | 1.5h |
| **P1-b** | 通知健壮性：单通道解密失败隔离 + 时区（第 17/18/11 条） | `internal/notify/*`（后端，需 `go test`） | 1.5h |
| **P2** | 待核对 A–H 逐条核实并按结论修（含 frps 防火墙提示、端口占用校验、复制行为、socks5） | 前后端零散 | 2h |
| **P3** | 视觉可选对齐（状态卡补「已配置入口」、规则列表类型筛选、反代视觉三项、重启按钮） | 前端 2 页 | 2h |

**验收**：每批以 `npx vue-tsc --noEmit` + `npx vite build`（前端）与 `go build ./...` + `go test ./internal/...`（后端）为准；涉及通知的后端改动需按现有测试补一条「单通道解密失败仍能发送其它通道」的用例。

---

## 六、附：上游 1.0.2 及更早（本项目基线之前）

本项目基线是上游 **v1.0.2（2026-09-18，新增「证书：火山引擎 DNS 的 ACME DNS-01 验证」）**，其上游历史（v1.0.0 通知体系、v1.0.1 DDNS 火山引擎与多层子域）在本项目 `CHANGELOG.md` 中已有对应条目（v1.0.0 / v1.0.1 / v1.0.2 段），本轮无需回溯。

---

## 七、可增加项清单（本项目确实没有的，按价值排序）

| 序 | 可增加项 | 依据（上游版本） | 落地要点 | 工作量 |
| --- | --- | --- | --- | --- |
| 1 | **剪贴板降级**：`utils/clipboard.ts` + 5 处调用点 | 1.0.4 修复 | `navigator.clipboard` 优先、`execCommand` 回退；非 HTTPS 内网访问下才真正生效 | 0.5h |
| 2 | **通知通道解密失败隔离**：单通道失败不拖垮整轮 | 1.0.5 修复 | `internal/notify/store.go` 的 `load()` 改为逐通道收集错误（失败通道标不可用 + 记原因），继续解析其它通道；补一条「SMTP 密文损坏时 Telegram 仍可发送」的测试 | 1.5h |
| 3 | **日志中心「FRP 日志」Tab**（解析时间/级别/来源/消息） | 1.0.3 新增 | `LogsView.vue` 增第 4 个 Tab；后端 `getFrpLogs` 已可用 | 1.5h |
| 4 | **通知时区**：容器设 `TZ`（并让通知时间走系统时区） | 1.0.4 修复 | `docker-compose*.yml` 加 `TZ: Asia/Shanghai`；`Dockerfile` 可加 `ENV TZ` | 0.5h |
| 5 | ~~反代监听端口占用校验~~ → 已具备，降级为「补冲突规则名 + custom 模式覆盖」 | 1.0.5 新增 | 见 §八：错误提示带冲突规则名；custom 规则解析其 `listen`/`server_name` 后纳入校验 | 0.5h + 2h |
| 6 | **frps 配置弹窗补「VPS 防火墙放行 TCP 远程端口」提示** | 1.0.3 改进 | 纯文案 + 由规则列表推导端口集合（`allowPorts` 已生成） | 0.5h |
| 7 | **Telegram 代理保存校验 + 自动补 `http://`** | 1.0.3 改进 | 保存时校验 URL 格式（允许 `http/https/socks5`），缺 scheme 时补 `http://` | 0.5h |
| 8 | **反代复制去掉「-复制」后缀** | 1.0.5 改进 | `duplicateName()` 改为保留原名 + 提示用户改域名 | 0.5h |
| 9 | **FRP 状态卡补「已配置入口」**（N 个 Web / M 个 TCP，点击筛选） | 1.0.3 改进 | 替换第 4 张卡「最近应用」或新增一张 | 1h |
| 10 | **服务端行加「重启」按钮** | 1.0.3 改进 | 复用 stop/start 串联或后端 action | 0.5h |
| 11 | **远程端口 0=自动分配（随机 1024–65535）** | 1.0.5 改进 | 后端随机分配 + 前端允许 0；本项目当前强制手填，属新增能力 | 1h |

> 前 4 项是「本项目确实缺少且有实际影响」的；第 5–8 项是上游同期新增/改进的缺口；第 9–11 项属可选增强。

---

## 八、第 5 项详细整理：反代监听端口占用校验

### 8.1 结论：本项目已有等价实现，且判定语义比上游更准

上游 1.0.5 的「新建入口时校验监听端口占用，避免重复端口静默冲突」，在本项目**已经具备**。
§四.F 当时判为「缺口」，是因为用关键词 `占用` / `in use` 做 grep，而本项目代码里的措辞是
「已被其他规则使用」，所以没搜到——这是**关键词误判**，已在上文更正。

更关键的是**判定语义**：上游按「端口」判冲突，本项目按「端口 + 域名」判冲突。后者才是 Nginx 的真实冲突，
前者会把「同一端口挂多个站点」这种最常见的用法误报成冲突。

### 8.2 Nginx 语义：一条规则到底占用哪些「端口 + 域名」

端口占用的判定必须与 `internal/nginx/generate.go` 的生成逻辑对齐，否则会误报或漏报。
`writeRuleBlocks`（`generate.go:120`）按 `rule.PortGroups()`（`proxy.go:101`）逐组生成 server 块：

| 规则字段 | 生成的 server 块 | 监听端口来源 |
| --- | --- | --- |
| 每个 PortGroup | 至少 1 个 | `host.ListenPort`（host 级覆盖）否则 `rule.ListenPort` |
| `https_enabled = true` 且有可用证书 | +1 个（`listen <port> ssl`） | **同组端口**，不额外占端口 |
| `http_redirect = true` 且 `useHTTPS` | +1 个（同端口、无 ssl、`return 301`） | **同组端口**，不额外占端口 |

**关键推论**：一条规则占用的端口集合就是它的 `PortGroups()` 端口集合；
`https_enabled` / `http_redirect` **不会**额外占用 80 / 443，因此不必按这两个开关推算端口。

Nginx 的冲突规则是「**同端口 + 同 `server_name`**」：此时只在 reload 时打一条
`conflicting server name ... ignored` 警告，**第一个 server 块生效、后面的静默失效**——
这正是上游所说的「重复端口静默冲突」。反过来，「同端口 + 不同域名」是**合法的虚拟主机**，不能报错。

### 8.3 本项目已有的校验链（逐跳）

| 跳 | 位置 | 行为 |
| --- | --- | --- |
| 1 | `web/src/views/ProxyView.vue` 保存表单 | `POST/PUT /api/proxy/rules` |
| 2 | `internal/api/proxy_handler.go:54` / `:93` | `svc.Create` / `svc.Update`；出错走 `writeError(400, err.Error())` |
| 3 | `internal/proxy/proxy.go:225` / `:345` / `:367` | `ensureHostsAvailable(hosts, listenPort, excludeRuleID)` |
| 4 | `internal/proxy/proxy.go:499` | `SELECT rule_id FROM proxy_hosts WHERE hostname = ? COLLATE NOCASE AND listen_port = ?` |
| 5 | `internal/proxy/proxy.go:511` | 返回 `前端域名 %s:%d 已被其他规则使用` |
| 6 | `internal/proxy/proxy.go:526` `replaceHosts` | 撞 `UNIQUE(hostname, listen_port)` 时给出同一文案（兜底） |

**三条路径都已覆盖**：新建（`:225`）、改端口（`:345`，仅校验跟随规则端口的 host，host 级独立端口不受影响）、
改域名（`:367`）。`excludeRuleID` 保证编辑自身时不误判。前端 `message.error(error.message)` 直接展示后端文案，
因此**只改后端文案即可，前端无需改动**。

### 8.4 三处真实缺口（按价值排序）

> **实施状态（2026-09-23）：缺口 1、2（方案 C）、3 已完成**，落地位置与测试见 §8.7。

**缺口 1 · 冲突提示不含冲突规则名**（上游明确要求「错误提示写明冲突规则名」）

- 现状：只说「前端域名 a.com:80 已被其他规则使用」，用户还得自己去列表里找是哪一条。
- 改法：`ensureHostAvailable` 的查询 JOIN `proxy_rules` 取 `name`：

  ```sql
  SELECT h.rule_id, COALESCE(r.name, '') FROM proxy_hosts h
  LEFT JOIN proxy_rules r ON r.id = h.rule_id
  WHERE h.hostname = ? COLLATE NOCASE AND h.listen_port = ? LIMIT 1
  ```

  错误串改为 `前端域名 a.com:80 已被规则「名称」占用`；`name` 为空时回落为 `规则 #id`。
- 影响面：`proxy.go` 单函数 + `proxy_hosts_test.go` 中断言该文案的用例需同步。
- 风险低，0.5h。

**缺口 2 · `nginx_mode = custom` 的规则完全不参与校验**（真正的盲区）

- 现状：`Generate`（`generate.go:52`）对 custom 规则走 `writeCustomRuleBlocks`（`generate.go:108`），
  把 `ruleCustomContent` 原样写入；其自定义配置里的 `listen` / `server_name` **不写入 `proxy_hosts`**，
  因此与已有规则的「同端口 + 同域名」冲突**保存时无任何提示**，只在 Nginx reload 时打一条 warning。
- 改法（先轻后重，二选一）：
  1. **保存 custom 配置时解析**其 `listen <port>` 与 `server_name`，与 `ListEnabled` 的其它规则做
     「同端口 + 同域名」比对，冲突即报错并写明冲突规则名。可复用 `internal/nginx/custom.go` 中
     已有的 `listen ` 前缀处理与 `syntax.go` 的行解析思路。
  2. **把解析结果落库到 `proxy_hosts`**（`hostname` 取 `server_name`、`listen_port` 取端口），
     这样 8.3 的校验链天然覆盖 custom 规则，不必维护两套逻辑（更推荐，但要处理多域名/通配写法）。
- 需处理的写法：`listen 80 default_server;`、`server_name a.com b.com;`、通配 `*.a.com`、
  `include` 片段、注释行。
- 风险中，2h。

**缺口 3 · 并发下端口变更的 TOCTOU**（低危）

- 现状：`Update` 的 `:359` 分支裸执行 `UPDATE proxy_hosts SET listen_port = ? ...`，未捕获 UNIQUE；
  校验（`:345`）与写入之间存在窗口。单用户场景几乎不会触发。
- 改法：把该 `UPDATE` 的 UNIQUE 错误映射成与 `replaceHosts` 相同的中文文案，或把校验与写入并入同一事务。
- 风险低，0.5h（可随缺口 1 一起做）。

### 8.5 明确**不做**的两件事

1. **不按「同端口」判冲突**：同端口不同域名是合法虚拟主机，按端口报错会让「一个端口挂多个站点」
   这种最常见的用法无法保存。
2. **不探测系统层面的端口占用**（`net.Listen` 试占、`ss -ltn`）：Havline 自己就是占用方（Nginx 监听 80/443），
   且容器与宿主机视角不同，误报率极高。上游所指也是 Nginx 配置内的冲突，而非系统端口。

### 8.6 验收标准

- 「同端口 + 同域名」与其它规则冲突 → 400，且提示中**含冲突规则名**；
- 「同端口 + 不同域名」→ 正常保存，不误报；
- host 级独立端口（`a.com:8081` 与 `b.com:8080`）互不干扰；
- 编辑规则自身（不改域名）→ 不误判为与自身冲突（`excludeRuleID` 已保证，需用例固化）；
- custom 模式规则与其 `listen`/`server_name` 冲突 → 保存时报错（缺口 2 完成后）。

### 8.7 实施记录（2026-09-23）

缺口 1、2（方案 C）、3 已落地：

| 缺口 | 落地位置 |
| --- | --- |
| 1 · 冲突提示带规则名 | `internal/proxy/proxy.go` 的 `ensureHostAvailable` + 新增 `endpointOwner`（描述优先「备注名 → 该规则首个域名 → `规则 #id`」）；文案统一为「域名 a.com:80 已被规则「官网」使用，请编辑现有规则或更换域名/端口」 |
| 2 · 方案 C 独立占用表 | 迁移 `migrations/024_proxy_custom_endpoints.sql`（`UNIQUE(rule_id, hostname, listen_port)` + `ON DELETE CASCADE`）；解析器 `internal/nginx/endpoints.go` 的 `ParseServerEndpoints`；读写 `internal/proxy/custom_endpoints.go`（`ReplaceCustomEndpoints` / `ClearCustomEndpoints` / `EnsureEndpointsAvailable`）；`ensureHostAvailable` 现在合并查两张表 |
| 2 · 落库时机 | `internal/service/nginx_config.go`：`SaveRuleNginx` 手动分支（校验占用 → 写文件 → 落库）、切回 `auto` 分支（清理占用）、`RollbackRuleNginx`（回滚后同步占用）；删除规则由外键级联清理 |
| 3 · TOCTOU | `Update` 的 `UPDATE proxy_hosts SET listen_port` 捕获 UNIQUE 后按域名复检并给出带规则名的原因；`replaceHosts` 同样改为复检；新增 `isUniqueViolation` 供两处复用 |

**测试**：`internal/nginx/endpoints_test.go`（多 server 块、`[::]` 写法、通配与正则、无 `listen` 默认 80、`include`/变量忽略）；
`internal/proxy/custom_endpoints_test.go`（走真实迁移建库：冲突含规则名、排除自身、同域名不同端口放行、custom 占用对表单规则可见、级联清理、清理后释放）。

### 8.8 未覆盖的边界（有意保留）

1. **全局手动 Nginx 片段**（`SaveGlobalNginx` → `writeGlobalCustomInclude`）里的 `server` 块不参与校验：
   它没有 rule_id，纳入校验需要额外的归属语义，本轮未做。
2. **前端预检**（`ProxyView.vue` 的 `findHostConflict`）只覆盖 `proxy_hosts`（表单规则的 `rule.hosts`），看不到 custom 占用；
   这类冲突由后端保存时报错兜底，错误消息直接展示给用户。
3. 解析器对手写配置里的 `include`、变量、`listen unix:` 一律忽略——宁可漏报也不误报（拦不住保存比误报更可接受）。


