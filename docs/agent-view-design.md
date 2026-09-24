# 公网 Agent 页（`AgentView.vue`）设计方案

> **时效说明（2026-09-24）**：文中「顶部下拉选服务端」的结构已被 v1.2.6 的**左列表 + 右详情**取代（页面标题也改为「公网服务端」），现状见 `docs/frp-server-list-merge-plan.md`。

> 本文按 `breezell-design` 两遍流程产出：**Pass 1a 检测栈 → Pass 1b 确定设计方向与令牌 → Pass 2 实施规格**，
> 并结合本项目（Fonu）既有页面与设计系统。落地时以本文为准，`docs/agent-view-refactor-plan.md`（结构整改）被本文吸收。
>
> 更新日期：2026-09-23

---

## 一、Pass 1a：栈与设计系统检测（结论先行）

| 检测项 | 实际值 |
| --- | --- |
| 框架 | Vue 3 + `<script setup lang="ts">` + Vite 6（`web/package.json`） |
| 组件库 | **naive-ui**（`n-button` `n-select` `n-tabs` `n-modal` `n-popconfirm` `n-alert` `n-spin` `n-icon` `n-tag` `n-form`） |
| 图标 | `@vicons/ionicons5` |
| 令牌系统 | `web/src/styles/variables.css`：`:root` + `html.dark` / `[data-theme='dark']` 双主题，`--fonu-*` 前缀 |
| 共享组件 | `PageHeader` `FonuCard` `StatCard` `StatusBadge` `EmptyState` `LoadError`（另有 `MiniBarChart` 等图表件） |
| 工具 | `utils/format.ts`（`formatBytes/formatRate/formatDate/formatRelativeTime/…`）、`utils/status.ts`（`StatusKind` 语义） |
| 同族页面 | `DashboardView`（统计卡 + 插槽）、`DdnsView`（提示 + 状态卡）、`CertificatesView`（统计卡规格基准）、`ProxyView`（分组/弹窗）、`SettingsView`（分区） |

**结论（skill 的 "Already has a design system → MATCH it" 规则）**：本页**沿用** naive-ui + `--fonu-*` 令牌 + 既有共享组件，
**不引入** Tailwind / shadcn-vue 等第二套体系，**不新增**第二套令牌或全局样式。

---

## 二、Pass 1b：设计方向

### 方向：机架面板（controlled density）

一句话概念：**像看一排机架指示灯** —— 状态永远是第一眼可见的、紧凑排列的四色状态条，操作被收进分区按钮带，输出以时间线呈现。克制、无装饰，密度优先。

- **Layout**：`状态区(4 列稳定网格) → 状态详情(2 列) → 控制/生命周期(2 列) → 输出与日志(整宽)`；左对齐、无居中大标题、无渐变、无装饰插图。
- **Signature（记忆点）**：**四色状态点行 + 操作输出时间线**（每条输出带来源徽标与相对时间）。

### Color（5 个语义令牌，全部复用现有）

| 语义 | 令牌 | 用在哪 | 判定规则 |
| --- | --- | --- | --- |
| 正常 / 运行 | `--fonu-brand`（`--fonu-brand-soft`、`--fonu-brand-text`） | 在线、运行中、监听中、成功输出 | 无需人工干预 |
| 待处理 | `--fonu-warning` | frps 未运行、端口未监听、frps 未安装、开机自启未启用 | 可一键恢复（引导操作） |
| 需人工干预 | `--fonu-error` | Agent 未连接、操作失败输出、卸载确认 | 需用户排查或不可逆 |
| 中性信息 | `--fonu-text-secondary` / `--fonu-text-muted` | 字段名、说明、时间戳、空态 | — |
| 引导 | `--fonu-info`（`--fonu-info-soft`） | 页面级引导提示（反代入口等） | 非故障类提示 |

规则：**一屏内 error 不超过一处**；warning 不用于"没数据"，只用于"有目标状态但未达成"。亮/暗两套值由 `variables.css` 的 `html.dark` 覆盖，**页面内零硬编码色值**。

### Type（沿用项目字体，显式声明）

- 正文字体 `--fonu-font`（`-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei'`），等宽 `--fonu-mono`。
- **关于 skill 的"禁用 system-ui 系默认栈"**：本项目已存在设计系统，按 "MATCH it, never add a second" 规则，**本页继续使用项目字体**；替换字体属**全局决策**（改 `variables.css`，影响全站并需全站回归），不在本页范围。
- 页内只用 4 档字号 + 3 档字重（与 `CertificatesView` 统计卡规格一致）：

| 用途 | 字号 / 字重 | 令牌 |
| --- | --- | --- |
| 统计卡数值（词条类） | 18px / 700 | `.stat-card__value--sm` |
| 统计卡数值（数字类） | 30px / 700，`letter-spacing:-0.03em` | `.stat-card__value` |
| 字段名 / 卡片标签 | 12–13px / 600 | `.stat-card__label`、`.field-grid label` |
| 说明 / 时间戳 / 空态 | 12–12.5px / 400 | `--fonu-text-muted`、`.form-hint` |
| 路径 / 命令 / 版本 / 日志 | `--fonu-mono` 12.5–13px | `.mono` / `.mono-wrap` / `.code-block` |

### Spacing & Radius

- 间距只用 `--fonu-space-1…6`（4/8/12/16/24/32），**禁止魔数**；卡片内边距统一 `var(--fonu-space-4) var(--fonu-space-5)`。
- 圆角只用 `--fonu-radius`(12px) / `--fonu-radius-sm`(8px)；阴影只用 `--fonu-shadow`。
- 页面里已有的 `style="width: 220px"` 这类内联尺寸，一律改为类（`.agent-select` / `.frps-version-select`）。

---

## 三、信息架构与线框

```text
┌ PageHeader  公网 Agent · 一句话说明          [服务端选择 ▾] [刷新] ┐
├ n-alert（最多一条状态提示：error 未连接 / warning frps 未运行）
├ 入口引导行（次级链接，不占 alert 位）：反代的部署与访问控制在「公网反代」→
├ 状态区  section[aria-label=服务状态]  4 列稳定网格
│   ● Agent 连接      ● frps 服务      ● 80 端口       ● 443 端口
│     在线/未连接        运行中/未运行     监听中/未监听    监听中/未监听
│     <agent 地址>       <frps 版本>       HTTP 入口       HTTPS 入口
├ 两列网格
│   FonuCard 服务状态详情            FonuCard frps 服务控制
│     field-grid 2 列：              按钮带：启动|停止 · 重启 ·
│     Agent 版本 / frps 版本            frps 管理面板 · 下发配置
│     启动命令 / 配置文件              [版本选择 ▾] 安装/升级
│     安装位置 / 开机自启
│     7000 端口
│   FonuCard Agent 生命周期（SSH）     FonuCard 输出与日志
│     探测环境 · 安装/升级 Agent         n-tabs：操作输出 | frps 日志
│     SSH 诊断 · 卸载(保留)             操作输出 = 时间线（来源+时间+ok）
│     卸载(含数据) ← 二次确认            空态 EmptyState · [清空]
└ ────────────────────────────────────────────────────────────────
```

要点：**状态只读 / 操作只写 / 输出只展示** 三段物理分离；「公网 Nginx 反代」原整卡删除，改为入口引导行。

---

## 四、组件与状态矩阵（Pass 2 规格）

### 交互状态（skill：每个可交互元素都要有全状态）

| 元素 | default | hover | pressed | focused | disabled | loading |
| --- | --- | --- | --- | --- | --- | --- |
| `n-button`（主/次/危险） | 库默认 | 库默认 | 库默认 | 库默认焦点环（回归实测 Tab 可达） | `:disabled="busy !== null && busy !== 本动作"` | `:loading="busy === '动作名'"` |
| 服务端选择器 `n-select` | 库默认 | 库默认 | — | 库默认 | 无服务端时禁用 | 无 |
| `n-tabs`（输出/日志） | 库默认 | 库默认 | 库默认 | 库默认（键盘 ←/→ 可切换） | 无 | 无 |
| `n-popconfirm`（卸载含数据） | 触发器为 warning 按钮 | 库默认 | 库默认 | 库默认 | busy 时禁用 | 确认后走按钮 loading |
| `.stat-card`（只读） | 令牌底色 | **有意不加 hover** | — | 不可聚焦（非交互元素） | — | 骨架/`—` |

### 内容状态（区分"没数据"与"拿不到数据"）

| 场景 | 呈现 |
| --- | --- |
| 尚未拉取（`statusLoaded=false`） | 数值位显示 `—`，`__sub` 显示「读取中…」 |
| 拉取中（首次） | `n-spin` 包裹状态区（仅首次，轮询不显示） |
| 拉取失败 | 数值位「未连接」+ 顶部 `error` alert（含「探测环境」动作） |
| 列表/日志为空 | `EmptyState`（不再渲染「暂无日志」字符串） |
| 操作失败 | 输出时间线中该条 `ok=false` 标红 + `message.error` |

---

## 五、动效与无障碍

**动效（high-impact, restrained）**

- 首屏**一次性**入场：状态区与卡片 `fade-up`（`translateY(4px)→0`，160ms，`--fonu-shadow` 不变），仅 `onMounted` 后触发一次；
- `@media (prefers-reduced-motion: reduce)` 下全部禁用；
- 除入场与按钮库内置反馈外，**不做**散点微动效（不加卡片 hover 位移、不加脉冲动画）。

**无障碍（Web 必做）**

- 语义：状态区用 `<section aria-label="服务状态">`；装饰性图标 `aria-hidden="true"`；按钮均有可读文本（不出现只有图标的裸按钮）；
- 动态内容：操作输出容器 `aria-live="polite"`（异步追加需播报）；`n-popconfirm` 已带键盘与焦点管理；
- 键盘：Tab 顺序 = 选择器 → 刷新 → 状态区（跳过只读）→ 各卡片按钮带 → tabs；`n-tabs` 支持方向键；
- 对比度：实测下列组合达 WCAG AA（正文 4.5:1、大字号 3:1）——`--fonu-brand`/`--fonu-warning`/`--fonu-error`/`--fonu-text-muted` 在 `--fonu-surface` 上，**亮色与暗色各测一遍**；
- 触控：≤760px 时按钮 `min-height: 44px`（或升为 `size="medium"`），满足最小触控目标；
- 响应式：`1200px+` 4 列状态 / `900–1199px` 2 列 / `≤900px` 单列 + 两列网格并一列 / `≤760px` 按钮铺满 / **`320px` 无横向溢出**。

---

## 六、状态与数据流（含前一轮复核发现的 3 处缺陷修复）

```ts
// 1) 切换服务端必须重置，避免串台（缺陷修复）
watch(selectedId, () => {
  opOutput.value = []
  agentStatus.value = null
  statusLoaded.value = false
  agentStatusOk.value = false
  void refreshAll()
})

// 2) 三态区分：未加载 / 加载中 / 失败（缺陷修复）
const statusLoaded = ref(false)     // 是否已成功拉取过
const statusLoading = ref(false)    // 首次加载中

// 3) 操作状态机：按动作粒度 loading，轮询静默（缺陷修复）
type Action = 'probe' | 'agent-install' | 'agent-uninstall' | 'diagnose'
            | 'frps-start' | 'frps-stop' | 'frps-restart' | 'frps-install' | 'push-config'
const busy = ref<Action | null>(null)
async function run<T>(action: Action, fn: () => Promise<T>) {
  if (busy.value) return            // 安全网：按钮已 disabled，这里只是兜底
  busy.value = action
  try { return await fn() } finally { busy.value = null }
}
async function refreshAll(options: { silent?: boolean } = { silent: true }) { /* 轮询不触碰 busy */ }

// 4) 输出时间线：来源 + 时间 + 成功标记，上限 20 条
interface OpOutput { at: string; source: string; text: string; ok: boolean }
const opOutput = ref<OpOutput[]>([])
function pushOutput(source: string, text: string, ok = true) {
  opOutput.value = [{ at: new Date().toISOString(), source, text, ok }, ...opOutput.value].slice(0, 20)
}
```

**提示规则（修掉原方案"互斥"与"常驻"的矛盾）**：状态提示**互斥且最多一条**（优先级 `error` 未连接 > `warning` frps 未运行）；
「公网反代入口」**不占 alert 位**，改为 PageHeader 下方一行次级链接。

**轮询**：10 秒一次；`document.visibilityState === 'hidden'` 时跳过；`onBeforeUnmount` 清理定时器。

---

## 七、本页不做（Never）

- 不引入第二套 UI 框架 / 令牌 / 全局样式（naive-ui + `--fonu-*` 已存在）；
- 不改全局字体与令牌（属全站决策）；
- 不用硬编码色值与魔数间距（一律令牌）；
- 不做渐变、居中大标题、统一灰卡片这类 slop 模式；
- 统计卡不加装饰性 hover / 位移 / 脉冲；
- 不把页面级操作塞进正文卡片（统一进 `PageHeader#actions`）；
- 不改后端与 agent（本次纯前端单文件）。

---

## 八、验收清单（对照 skill self-check）

- [ ] 复用了项目真实栈与令牌（naive-ui + `--fonu-*` + 共享组件），未新增第二套
- [ ] 单一设计方向（机架面板）落地：状态条 + 输出时间线为 signature，无 banned 字体引入，颜色/间距/圆角全部走令牌
- [ ] 交互状态齐全：hover / pressed / **focused（键盘实测）** / disabled / loading / empty / error
- [ ] 无障碍：对比度 AA（亮+暗）、可见焦点环、键盘可完成全部操作、`aria-live` 播报输出、图标 `aria-hidden`
- [ ] 最窄 320px 无横向溢出；≤760px 按钮 ≥44px 触控目标
- [ ] 回归：切换服务端不串台；仅被点按钮 loading；轮询不闪；危险操作有二次确认；`vue-tsc` 0 错误 + `vite build` 成功

---

## 九、与既有文档关系与工作量

| 文档 | 关系 |
| --- | --- |
| `docs/agent-view-refactor-plan.md` | 结构整改清单（12 项问题 + 分步改造），**本文吸收其全部内容**并补上设计方向、状态矩阵、无障碍与动效规格；实施时以本文为准 |
| `web/.breezell/skills/fonu-page-ui`（工作区 skill） | 页面 UI 通用标准；本文是它在「公网 Agent」页的具体化 |

工作量：**单文件改动（`web/src/views/AgentView.vue`）**，约 4–5 小时（比原结构整改多出的部分：设计方向落地、状态矩阵、无障碍与动效、320px 适配）。
验证：`npx vue-tsc --noEmit` + `npx vite build`，再同步 `web/dist → cmd/fonu/web/dist` 后 `go build ./...` 使本地生效。
