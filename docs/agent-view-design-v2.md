# 公网 Agent 页（`AgentView.vue`）设计方案 v2

> **时效说明（2026-09-24）**：文中「顶部下拉选服务端」的结构已被 v1.2.6 的**左列表 + 右详情**取代（页面标题也改为「公网服务端」），现状见 `docs/frp-server-list-merge-plan.md`。

> 本文按 `breezell-design` 两遍流程产出，核心是**先列清功能、再谈设计**：
> 第二节把本页应有的 **27 项能力**逐条落位（含 5 处当前页面看不到的能力），第四节按功能分区。
>
> 取代 `docs/agent-view-design.md`（v1 只做了结构搬家，未解决"功能不可见"问题）；
> 也取代 `docs/agent-view-refactor-plan.md`。更新日期：2026-09-23

---

## 一、Pass 1a：栈与设计系统检测

| 检测项 | 实际值 |
| --- | --- |
| 框架 | Vue 3 + `<script setup lang="ts">` + Vite 6 |
| 组件库 | **naive-ui**（`n-button` `n-select` `n-tabs` `n-modal` `n-popconfirm` `n-alert` `n-spin` `n-icon` `n-tag` `n-tooltip` `n-form`） |
| 图标 | `@vicons/ionicons5` |
| 令牌系统 | `web/src/styles/variables.css`：`:root` + `html.dark` 双主题，`--fonu-*` 前缀 |
| 共享组件 | `PageHeader` `FonuCard` `StatCard` `StatusBadge` `EmptyState` `LoadError` |
| 工具 | `utils/format.ts`（`formatBytes/formatRate/formatDate/formatRelativeTime/…`）、`utils/status.ts`（`StatusKind`） |

**结论**：沿用既有体系（naive-ui + `--fonu-*` + 共享组件），**不引入**第二套框架/令牌。

---

## 二、功能总清单（Capability Inventory）

> 设计原则：**没有隐式功能**。每项能力都要有固定位置、可见状态、明确反馈；
> 能做的显示按钮，不能做的显示**原因**（禁用 + 提示），而不是隐藏。

| # | 能力 | 数据来源 | 当前状态 | 新设计位置 | 可见性形态 |
| --- | --- | --- | --- | --- | --- |
| A | 选择公网服务端 | `listFrpServers`（agent/ssh 已配置） | 已有 | `PageHeader#actions` | 下拉选择 |
| B | Agent 连通性 | `getFrpAgentStatus` 成败 | 已有 | ① 运行状态 | 三态：在线 / 未连接 / 未加载 |
| C | Agent 地址 | `server.agent_url` | 已有 | ① 运行状态卡副标题 | 等宽文本 |
| D | Agent 版本 | `agent_version` | 已有 | ② 环境与依赖 | 等宽文本 |
| E | **Agent 凭据是否已配置** | `server.agent_configured` | **未展示** | ② 环境与依赖 | StatusBadge：已配置 / 未配置 |
| F | frps 运行状态 | `frps_active` | 已有 | ① 运行状态 | 在线 / 未运行 |
| G | frps 已安装版本 | `frps_version` | 已有 | ① + ② | 文本 / 徽标 |
| H | frps 二进制路径 | `frps_binary` | 已有 | ② 环境与依赖 | 等宽 + 换行 |
| I | frps 配置文件路径 | `frps_config_path` | 已有 | ② 环境与依赖 | 等宽 + 换行 |
| J | frps 启动命令 | `frps_unit_exec` | 已有 | ② 环境与依赖 | 等宽 + 换行 |
| K | frps 开机自启 | `frps_enabled` | 已有 | ② 环境与依赖 | StatusBadge |
| L | frps 管理面板入口 | `dashboard_addr/port` → `dashboardURL` | 有（未配置时**按钮隐藏**） | ③ frps 控制 | **常显**；未配置时禁用 + tooltip 指向配置位置 |
| M | 端口监听 80/443/7000/8080 | `ports` | 仅 80/443 卡 + 7000 字段（**8080 未展示**） | ① 运行状态「端口」卡 | 四端口全列：`80✓ 443✓ 7000✗ 8080✗` |
| N | **已注册反代路由数** | `routes[]` | **未展示** | ① 运行状态 | 「反代路由 N 条」+ 折叠列表 |
| O | **最后一次检查时间** | `checked_at` | **未展示** | `PageHeader` 副标题 | 「检查于 x 前」（`formatRelativeTime`） |
| P | **Nginx 可用性（本页完全未展示）** | `nginx_ok/missing/running/version/path/conf_dir/error` | **未展示**（在公网反代页） | ② 环境与依赖「Nginx」组 | 徽标 + 版本 + 路径 + 配置目录 + 错误原文 + 跳转 |
| Q | 启动 / 停止 frps | `frpsServerActionRaw(start/stop)` | 已有 | ③ frps 控制 | 按状态互斥的按钮 |
| R | 重启 frps | `frpsServerActionRaw(restart)` | 已有 | ③ frps 控制 | 按钮 |
| S | 下发 frps 配置 | `pushFrpConfigToAgent` | 已有（散在另一卡） | ③ frps 控制 | 按钮 |
| S2 | 查看即将下发的 frps 配置 | `getFrpFRPSConfig` | 本页未提供 | ③ frps 控制（紧跟下发按钮） | 按钮 + 只读弹窗（等宽代码块 + 复制） |
| T | 安装 / 升级 frps | `installFrpServer` + `listFrpReleases` | 已有 | ③ frps 控制 | 版本下拉 + 按钮（列表空时禁用） |
| U | 探测环境 | `probeFrpAgent` | 已有 | ④ Agent 生命周期 | 按钮 |
| V | 安装 / 升级 Agent | `installFrpAgent` | 已有 | ④ Agent 生命周期 | 按钮 |
| W | 卸载（保留数据 / 含数据） | `uninstallFrpAgent(keepData)` | 已有（危险无确认） | ④ Agent 生命周期 | 两按钮，含数据**带二次确认** |
| X | SSH 诊断 | `sshDiagnoseFrpAgent` | 已有 | ④ Agent 生命周期 | 按钮 |
| Y | frps 最近日志 | `frps_log_tail` | 已有（独立卡） | ⑤ 输出与日志 Tab | 日志块 + 空态 |
| Z | 操作输出（7 类） | 各接口返回 | 已有（单一 `agentOutput`） | ⑤ 输出与日志 Tab | 时间线：来源 + 时间 + 成败 |
| AA | 公网反代管理入口 | `getFrpAgentRouteCandidates` / `RouteHealth` | 有（整卡只放跳转） | 入口行 + ① 反代路由卡 | 「N 条规则 · 健康 X/Y」+ 跳转 `/public-proxy` |

### 本次必须补齐的 5 处（数据已返回，页面没显示）

| 项 | 数据字段 | 为什么重要 |
| --- | --- | --- |
| E | `agent_configured` | 决定"能不能连上 agent"，是排障第一问 |
| M(8080) | `ports["8080"]` | frps vhost 端口是否在听，直接对应反代回源 |
| N | `routes[]` | 一眼看出 VPS 上已注册几条反代 |
| O | `checked_at` | 判断状态是不是陈旧的（页面 10 秒轮询，用户需要知道数据新鲜度） |
| P | `nginx_*` 七字段 | 反代的前置依赖；本页不显示就只能跳到另一页才能发现"没装 Nginx" |

---

## 三、设计方向与令牌（Pass 1b）

**方向：机架面板（controlled density）**——像看一排机架指示灯：状态永远第一眼可见，操作收进分区按钮带，输出以时间线呈现；克制、无装饰、密度优先。

- **Layout**：状态区（4 列稳定网格）→ 环境与依赖（两列）→ 控制/生命周期（两列）→ 输出与日志（整宽）；左对齐，无居中大标题、无渐变、无装饰插图。
- **Signature**：**四色状态点行 + 操作输出时间线**。

**Color（5 个语义令牌，全部复用现有）**

| 语义 | 令牌 | 用于 | 判定 |
| --- | --- | --- | --- |
| 正常 | `--fonu-brand`（`-soft`/`-text`） | 在线、运行中、监听中、成功输出 | 无需干预 |
| 待处理 | `--fonu-warning` | frps 未运行、端口未监听、frps 未安装、开机自启未启用 | 可一键恢复 |
| 需人工干预 | `--fonu-error` | Agent 未连接、Nginx 缺失、操作失败、卸载确认 | 需排查/不可逆 |
| 中性 | `--fonu-text-secondary` / `--fonu-text-muted` | 字段名、说明、时间、空态 | — |
| 引导 | `--fonu-info`（`-soft`） | 页面级引导（反代入口等） | 非故障 |

规则：一屏 error ≤ 1 处；warning 只用于"有目标状态但未达成"，不用于"没数据"。亮/暗由 `variables.css` 覆盖，**页面零硬编码色值**。

**Type（沿用项目字体，显式声明）**

- 正文 `--fonu-font`，等宽 `--fonu-mono`。skill 禁用 system-ui 系默认栈，但本项目已有设计系统，按 **"MATCH it, never add a second"** 继续使用；换字体属全局决策（改 `variables.css` + 全站回归），不在本页范围。
- 页内 4 档字号 / 3 档字重：数值词条 18px/700（`.stat-card__value--sm`）、数值数字 30px/700/`-0.03em`、字段名与卡标签 12–13px/600、说明与时间 12–12.5px/400；路径/命令/版本/日志走 `--fonu-mono` 12.5–13px。

**Spacing & Radius**：只用 `--fonu-space-1…6` 与 `--fonu-radius(-sm)`、`--fonu-shadow`；禁止魔数；内联尺寸一律改类（`.agent-select` / `.frps-version-select`）。

---

## 四、页面分区与线框（按功能分区）

```text
┌ PageHeader  公网 Agent · 检查于 12 秒前            [服务端 ▾] [刷新] ┐
├ n-alert   状态提示（互斥一条：error 未连接 / warning frps 未运行）
├ 入口行    公网反代：3 条规则 · 健康 2/3   →  /public-proxy
├ ① 运行状态   section[aria-label=运行状态]  4 列稳定网格
│   ● Agent 连接      ● frps 服务       ● 反代路由        ● 端口监听
│     在线/未连接          运行中/未运行      3 条              80✓ 443✓
│     <agent 地址>        <frps 版本>      健康 2/3 已生效     7000✓ 8080✗
├ ② 环境与依赖（Fonu Agent | Nginx | frps 运行时，三列并排）
│   Fonu Agent                Nginx（本页新增）          frps 运行时
│     版本 / 地址               可用性徽标 / 版本            二进制 / 版本
│     凭据：已配置 ✓            可执行路径 / 配置目录          配置路径
│     [探测环境]                错误原文（红）               启动命令 / 开机自启
│                             → 去「公网反代」处理
├ ③+④ 两列并排（左：Agent 生命周期（SSH） / 右：frps 控制）
│   左 [探测环境] [安装/升级 Agent] [SSH 诊断]          右 [启动|停止] [重启] [管理面板]
│      [卸载(保留数据)] [卸载(含数据) ← 二次确认]          [下发配置] [版本 ▾] [安装/升级]
└ ⑤ 输出与日志   n-tabs：操作时间线（来源+时间+成败，aria-live） | frps 日志
                  空态 EmptyState · [清空]
```

分区原则：

1. **每项能力都有固定位置**（对照第二节清单）；按钮常显，不可用时**禁用 + 说明原因**，绝不隐藏；
2. 状态区只读 / 控制区只写 / 输出区只展示，三段物理分离；
3. 新增的「环境与依赖」把三件前置条件（Agent 凭据、Nginx、frps 运行时）集中可见——排障时最先要看的就是这三项。

---

## 五、组件与状态矩阵（Pass 2 规格）

| 元素 | default | hover | pressed | focused | disabled | loading |
| --- | --- | --- | --- | --- | --- | --- |
| `n-button`（主/次/危险） | 库默认 | 库默认 | 库默认 | 库默认焦点环（键盘实测） | `busy !== null && busy !== 本动作` | `busy === '动作名'` |
| 服务端选择器 | 库默认 | 库默认 | — | 库默认 | 无服务端时禁用 | — |
| `n-tabs`（输出/日志） | 库默认 | 库默认 | 库默认 | 库默认（←/→ 可切换） | — | — |
| `n-popconfirm`（卸载含数据） | warning 触发器 | 库默认 | 库默认 | 库默认 | busy 时禁用 | 确认后走按钮 loading |
| `.stat-card`（只读） | 令牌底色 | **有意不加 hover** | — | 非交互不可聚焦 | — | 骨架 / `—` |
| 「管理面板」（未配管理接口） | **常显但禁用** | — | — | — | 是（tooltip 指向配置位置） | — |
| 「安装/升级 frps」（版本列表未加载） | **常显但禁用** | — | — | — | 是（提示「正在获取官方版本列表…」） | 列表加载中 |

**内容状态（区分"没数据"与"拿不到数据"）**

| 场景 | 呈现 |
| --- | --- |
| 尚未拉取 | 数值位 `—`，副标题「读取中…」 |
| 首次拉取中 | `n-spin` 包裹状态区（仅首次，轮询不显示） |
| 拉取失败 | 数值位「未连接」+ 顶部 `error` alert（含「探测环境」动作） |
| 无已注册路由 / 无日志 | `EmptyState`（不再渲染「暂无日志」字符串） |
| 操作失败 | 时间线中该条 `ok=false` 标红 + `message.error` |

---

## 六、动效与无障碍

**动效**：首屏一次性 `fade-up`（`translateY(4px)→0`，160ms），仅 `onMounted` 触发一次；`prefers-reduced-motion: reduce` 全禁用；除此之外不做散点微动效（不加卡片 hover 位移/脉冲）。

**无障碍**：状态区 `<section aria-label="运行状态">`；装饰图标 `aria-hidden`；按钮均有可读文本；输出区 `aria-live="polite"`；Tab 顺序 = 选择器 → 刷新 → 状态区（跳过只读）→ 各分区按钮带 → tabs；对比度实测 AA（`--fonu-brand/warning/error/text-muted` 在 `--fonu-surface` 上，亮+暗各一遍）；≤760px 按钮 `min-height: 44px`；响应式 `1200+ / 900–1199 / ≤900 / ≤760`（运行状态 4→2→1 列；环境与依赖三列 →≤1199 降 2 列 →≤900 单列，且三列时卡片内短字段（版本/徽标）双列并排、长路径与命令整行，控制卡片高度），**320px 无横向溢出**。

---

## 七、状态与数据流（含前一轮复核发现的 3 处缺陷修复）

```ts
// 1) 切换服务端必须重置，避免串台
watch(selectedId, () => {
  opOutput.value = []
  agentStatus.value = null
  statusLoaded.value = false
  agentStatusOk.value = false
  void refreshAll()
})

// 2) 三态区分：未加载 / 加载中 / 失败
const statusLoaded = ref(false)     // 是否成功拉取过
const statusLoading = ref(false)    // 首次加载中

// 3) 按动作粒度 loading，轮询静默
type Action = 'probe' | 'agent-install' | 'agent-uninstall' | 'diagnose'
            | 'frps-start' | 'frps-stop' | 'frps-restart' | 'frps-install' | 'push-config'
const busy = ref<Action | null>(null)
async function run<T>(action: Action, fn: () => Promise<T>) {
  if (busy.value) return            // 安全网：按钮已 disabled
  busy.value = action
  try { return await fn() } finally { busy.value = null }
}
async function refreshAll(options: { silent?: boolean } = { silent: true }) { /* 轮询不触碰 busy */ }

// 4) 输出时间线：来源 + 时间 + 成败，上限 20 条
interface OpOutput { at: string; source: string; text: string; ok: boolean }
const opOutput = ref<OpOutput[]>([])
function pushOutput(source: string, text: string, ok = true) {
  opOutput.value = [{ at: new Date().toISOString(), source, text, ok }, ...opOutput.value].slice(0, 20)
}
```

**提示规则**：状态提示互斥且最多一条（`error` 未连接 > `warning` frps 未运行）；「公网反代入口」不占 alert 位，独立成入口行。
**轮询**：10 秒；`document.visibilityState === 'hidden'` 时跳过；`onBeforeUnmount` 清理定时器。

---

## 八、本页不做（Never）

- 不引入第二套 UI 框架 / 令牌 / 全局样式；不改全局字体与令牌；
- 不用硬编码色值与魔数间距；不做渐变、居中大标题、统一灰卡片等 slop 模式；
- 统计卡不加装饰性 hover / 位移 / 脉冲；
- 不把页面级操作塞进正文卡片（统一进 `PageHeader#actions`）；
- **不隐藏功能**：任何能力要么可用，要么可见地禁用并给出原因；
- 不改后端与 agent（纯前端单文件）。

---

## 九、验收清单

- [ ] 复用项目真实栈与令牌（naive-ui + `--fonu-*` + 共享组件），未新增第二套
- [ ] 单一设计方向（机架面板）落地，颜色/间距/圆角全走令牌，无 banned 字体引入
- [ ] 交互状态齐全：hover / pressed / **focused（键盘实测）** / disabled / loading / empty / error
- [ ] 无障碍：对比度 AA（亮+暗）、可见焦点环、键盘可完成全部操作、`aria-live` 播报、图标 `aria-hidden`
- [ ] 最窄 320px 无横向溢出；≤760px 按钮 ≥44px
- [ ] **功能可见性：逐条对照第二节 A–AA，每项都有明确位置；E / M(8080) / N / O / P 五项新增展示已落地；不存在隐藏功能**
- [ ] 回归：切换服务端不串台；仅被点按钮 loading；轮询不闪；危险操作有二次确认；`vue-tsc` 0 错误 + `vite build` 成功

---

## 十、与既有文档关系与工作量

| 文档 | 关系 |
| --- | --- |
| `docs/agent-view-design.md`（v1） | 被本文取代（v1 只是结构搬家，未解决"功能不可见"）；确认后可删除 |
| `docs/agent-view-refactor-plan.md` | 被本文取代 |
| 工作区 skill `fonu-page-ui` | 页面 UI 通用标准；本文是它在「公网 Agent」页的具体化 |

工作量：**单文件改动（`web/src/views/AgentView.vue`）**，约 5–6 小时（含 5 处能力补齐、状态机重构、无障碍与响应式）。
验证：`npx vue-tsc --noEmit` + `npx vite build`，再同步 `web/dist → cmd/fonu/web/dist` 后 `go build ./...`。
