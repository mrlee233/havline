# 公网反代页（`ProxyRemoteView.vue`）设计方案

> 目标：以**本地反向代理页 `ProxyView.vue` 为交互参照**，结合公网反代特有语境（部署到 VPS、Nginx 由 agent 管理、证书由 agent 推送、frps 提供隧道），
> 把公网反代页从"能用的列表页"整理成**功能齐全、状态可见、与本地页同族**的页面。
>
> 按 `breezell-design` 两遍流程产出：Pass 1a 栈检测 → Pass 1b 设计方向与令牌 → Pass 2 实施规格。
> 参考页：`ProxyView.vue`（3485 行，本地反代）、`AgentView.vue`（刚按 v2 重构，本页与其保持同族）。
> 更新日期：2026-09-23

---

## 一、Pass 1a：栈与设计系统检测

| 检测项 | 实际值 |
| --- | --- |
| 框架 | Vue 3 + `<script setup lang="ts">` + Vite 6 |
| 组件库 | **naive-ui**（`n-select` `n-button` `n-modal` `n-form` `n-switch` `n-input` `n-input-number` `n-tag`） |
| 图标 | `@vicons/ionicons5`（本页目前未用图标） |
| 令牌系统 | `web/src/styles/variables.css`：`:root` + `html.dark` 双主题，`--fonu-*` 前缀 |
| 共享组件 | `PageHeader` `FonuCard` `LoadError` `EmptyState` `StatusBadge`（本页**未用** `StatusBadge`） |
| 工具 | `utils/format.ts`（已用 `formatBytes` / `formatRate`） |
| 同族页面 | `ProxyView`（本地反代，交互参照）、`AgentView`（已按 v2 重构，同为公网侧） |

**结论**：沿用既有体系（naive-ui + `--fonu-*` + 共享组件），**不引入**第二套框架/令牌。

---

## 二、现状盘点（含实测缺陷）

`ProxyRemoteView.vue` 现为 584 行，结构是：`PageHeader` → Nginx 提示条 → 三张卡片（Nginx 状态 / 穿透规则与公网反代状态 / 使用说明）→ 两个弹窗（部署设置 / 反代配置）。

| # | 问题 | 证据 | 影响 |
| --- | --- | --- | --- |
| 1 | **刷新失败静默** | `refreshAll()` 的 catch 写入 `loadErrorMsg`，但该变量**从未在模板中渲染** | 拉取规则/健康/流量失败时页面毫无反馈，用户以为"就是没数据" |
| 2 | **没有统计区** | 全页无 `.stats-row` / `.stat-card`，而数据齐全（规则数、已部署数、健康通过数、流量、连接数） | 与所有标准页（Dashboard/Ddns/Certificates/Proxy）不一致，一眼看不到全局 |
| 3 | **硬编码颜色 6 处** | `#f0c36d`、`#e79b9b`、`#dc2626`、`#16a34a`、`rgba(240,195,109,.12)`、`rgba(34,197,94,.12)` | 违反令牌规则；暗色主题下对比度未验证 |
| 4 | **自造状态组件** | `nginxState()` 返回自造 `n-tag`；`healthBadges()` 自造 `.health-badge/.health-ok/.health-bad` | 与 `StatusBadge` + `utils/status.ts` 语义分叉，同类状态在不同页面长得不一样 |
| 5 | **「使用说明」整卡占位** | 一整张 `FonuCard` 只有一段 `form-hint` | 占掉整行网格，信息密度低（`AgentView` 已把这类内容改成 `n-alert` + 入口行） |
| 6 | **空态与折叠区未令牌化** | 自造 `.empty-route` div；`<details class="raw-routes">` 原生折叠 | 空态应为 `EmptyState`；原生折叠样式不随主题 |
| 7 | **loading 粒度粗** | 单个 `refreshing` 同时驱动「刷新」「重新检测」与轮询 | 轮询时按钮闪 loading；无法区分"在部署"与"在刷新" |
| 8 | **切换服务端不重置** | `watch(selectedId)` 不存在 | 上一台的健康/流量/agent 状态残留在新服务端视图里（串台） |
| 9 | **轮询未做可见性/冲突保护** | 15 秒 `setInterval` 里只判断 `!refreshing` | 页面隐藏时仍打 6 个接口；`refreshAll` 串行 6 请求，慢时可能叠压 |
| 10 | **内联尺寸** | `style="width: 220px"` | 应为类名（如 `AgentView` 的 `.agent-select`） |
| 11 | **能力缺口（本地页有、本页无）** | 本地页有：批量排序（`reorderProxies`）、扫描发现（`scanDiscovery`）、Nginx 配置编辑与**回滚**（`saveProxyNginx`/`rollbackProxyNginx`）、客户端连接列表（`getProxyClients`）、实时流量曲线 | 公网侧对应位置缺失：无批量部署、无历史回滚、无已部署路由的客户端/连接明细视图 |

---

## 三、功能总清单（Capability Inventory）

> 原则同 `AgentView` v2：**没有隐式功能**——每个能力有固定位置、可见状态、明确反馈；能做的显示按钮，不能做的显示原因（禁用 + 提示）。

| # | 能力 | 数据来源 | 现状 | 新设计位置 | 可见性形态 |
| --- | --- | --- | --- | --- | --- |
| A | 选择公网服务端 | `listFrpServers`（agent/ssh 已配置） | 已有 | `PageHeader#actions` | 下拉 + 类名宽度 |
| B | 刷新全部数据 | `refreshAll()` | 已有 | `PageHeader#actions` | 主按钮 + `:loading`（独立于轮询） |
| C | **页面级状态提示** | `agentStatus.nginx_missing / nginx_ok / nginx_error` | 自造 alert（硬编码色） | `PageHeader` 之后 | `n-alert`（error/warning），含动作按钮 |
| D | Nginx 可用性概览 | `nginx_missing/ok/running/version/path/conf_dir/error` | 8 项 `nginx-status-grid` | ① 环境区（与 AgentView 同族） | 徽标 + 值字段（令牌化） |
| E | 一键安装 Nginx | `installFrpAgentNginx` | 已有（提示条 + 卡片各一处，重复） | ① 环境区（**只保留一处**） | 按钮 + `:loading` |
| F | **规则汇总统计** | 由 `routeCandidates` + `routeDetails` + `routeHealth` 计算 | **无** | ② 统计区（4 列） | 统计卡：规则总数 / 已生效 / 健康通过 / 连接数 |
| G | 规则列表（每行） | `getFrpAgentRouteCandidates` | 已有 | ③ 规则区 | 行：名称 + 域名 + 上游 + 回源端口 |
| H | 部署态徽标（四态） | `getFrpAgentRoutes` → `stateOf()` | 已有（自造 tag） | ③ 规则行 | `StatusBadge`：已生效 / 已写入未加载 / 语法错误 / 未部署 |
| I | 健康检查（4 段） | `getFrpAgentRouteHealth` → DNS/证书/隧道/服务 | 已有（自造 badge） | ③ 规则行 | `StatusBadge` × 4 + 悬停详情 |
| J | 流量指标（6 项） | `listFrpProxiesTraffic` | 已有（自造 `.traffic-metric`） | ③ 规则行 | 等宽小字 + `formatBytes/formatRate`；采集失败标红 |
| K | 部署反代 | `deployFrpAgentRoute` | 已有 | ③ 规则行 | 主按钮（仅该行 loading） |
| L | 移除反代 | `removeFrpAgentRoute` | 已有 | ③ 规则行 | warning 按钮 + **二次确认**（现无确认） |
| M | 部署设置（3 Tab） | `updateFrpProxy` + 重新部署 | 已有（字段齐全） | 弹窗（沿用 `proxy-modal` 结构） | 基础 / 安全 / 高级 |
| N | 查看生成的 nginx 配置 | `getFrpAgentRouteConf` | 已有 | ③ 规则行 → 弹窗 | 只读代码块 + 复制 |
| O | **VPS 上 agent 管理的路由文件** | `getFrpAgentRoutes`（含 syntax_ok/loaded） | `<details>` 折叠 | ③ 规则区底部 | 与部署态合并展示，令牌化折叠（或独立 Tab） |
| P | 空态引导 | — | 自造 `.empty-route` | ③ 规则区 | `EmptyState` + 「去内网穿透页」动作 |
| Q | 前往公网 Agent 页 | — | 无入口 | `PageHeader` 下方入口行 | 次级链接（Nginx/agent 异常时指向排查页） |
| R | **批量部署**（能力缺口） | 循环 `deployFrpAgentRoute` | **无** | ③ 规则区头部 | 「部署全部未部署」+ 进度反馈（**分批实施，见第十一节**） |
| S | **配置回滚**（能力缺口） | agent 侧需新增历史版本接口 | **无** | 弹窗内（部署设置 → 高级）或规则行 | 暂列"不做"，需后端支持（见第九节） |

---

## 四、与本地反代页（`ProxyView.vue`）的对照

| 本地页能力 | 公网反代对应做法 | 结论 |
| --- | --- | --- |
| 6 张统计卡（服务总数/总上传/总下载/当前上传/当前下载/当前连接） | 公网侧统计对象是**规则与部署态**，不是单机流量 | **对齐形式**（`.stats-row` + `.stat-card`），**换指标**：规则总数 / 已生效 / 健康通过 / 连接数 |
| 概览区：域名 · 监听 · 目标 · 协议（`overview-strip`） | 公网侧对应"这台 VPS 的入口"：域名 / 回源 / vhost 端口 / 协议 | **对齐**：放进规则行的上游行，不单开概览卡 |
| 实时流量卡（最近 5 分钟 + `live-badge` 实时） | frps 侧只有**当日累计 + 相邻两次采样速率**，无时间序列接口 | **不对齐**：保留"当前速率"文本，不做曲线；若要做需后端提供采样序列 |
| 客户端连接列表（`getProxyClients`） | 公网侧无对应接口（frps dashboard 只给连接数） | **不对齐**：显示连接数；明细需后端扩展 |
| Nginx 配置编辑 + 保存 + **回滚** | 公网侧配置由 agent 生成（模板化），手工编辑会与模板冲突 | **不对齐（有意）**：只提供"查看配置"；回滚需 agent 侧版本化，列入不做 |
| 扫描发现（`scanDiscovery`）、排序（`reorderProxies`） | 公网侧规则来源是穿透规则（内网侧），排序/发现属内网语义 | **不对齐**：排序按"名称 + 域名"稳定排序；发现入口指向「内网穿透」页 |
| 规则弹窗 3 Tab（基础配置/安全设置/Nginx） | 公网侧弹窗已同构（基础/安全/高级） | **已对齐**（本轮不改字段，只做校验与提示） |
| 表单里「使用目标 Host 头」「忽略后端 TLS 校验」 | 公网侧明确不做（破坏 frps vhost 路由 / 回源为明文） | **不照搬**（已写入 `docs/agent-proxy-settings-plan.md` 排除清单） |

---

## 五、设计方向与令牌（Pass 1b）

**方向：部署台账（deployment ledger）**——与 `AgentView` 的"机架面板"同族：一行一条规则，左边是"这条反代是什么"，右边是"它现在什么状态、能做什么"；颜色只用来表达状态，不用来装饰。

- **Layout**：状态提示 → 统计区（4 列）→ 环境与 Nginx（2 列）→ 规则台账（整宽）→ 入口行；左对齐，无居中、无渐变。
- **Signature**：**规则行左侧的「四段健康徽标串」**（DNS · 证书 · 隧道 · 服务），一眼扫完所有规则的健康。

**Color**：沿用 5 个语义令牌（同 `AgentView` v2）：`--fonu-brand` 正常 / `--fonu-warning` 待处理 / `--fonu-error` 需人工干预 / 中性 `--fonu-text-*` / `--fonu-info` 引导。**页面零硬编码色值**（现状 6 处全部替换：`#16a34a`→`--fonu-brand`、`#dc2626`→`--fonu-error`、`#f0c36d`→`--fonu-warning`、`#e79b9b`→`--fonu-error`、两处 `rgba(...)`→`--fonu-brand-soft` / `--fonu-warning` 的 soft 变体或 `--fonu-bg-muted`）。

**Type**：沿用 `--fonu-font` / `--fonu-mono`；页内 4 档字号（12 / 12.5 / 13 / 18-30）+ 3 档字重，与 `CertificatesView` 统计卡规格一致。

**Spacing & Radius**：只用 `--fonu-space-1…6` 与 `--fonu-radius(-sm)`、`--fonu-shadow`；`style="width: 220px"` → `.proxy-remote-select`。

---

## 六、页面分区与线框

```text
┌ PageHeader  公网反代 · 一句话说明            [服务端 ▾] [刷新] ┐
├ n-alert（Nginx 缺失 / 校验失败，含「一键安装 Nginx」动作）
├ 入口行：Agent 与 frps 的生命周期、配置下发在「公网 Agent」页 →
├ ① 统计区（4 列）  规则总数 · 已生效 · 健康通过 · 当前连接
├ ② 环境与 Nginx（2 列）
│     可用性 / 进程 / 版本 / 可执行文件 / vhost 目录 / 配置校验 / 端口监听
├ ③ 规则台账（整宽）
│   [名称]  域名（mono）            [已生效] [查看配置] [部署设置] [部署反代] [移除反代]
│   上游 192.168.31.152:7023（回源 127.0.0.1:8080）
│   DNS ✓  证书 ✓  隧道 ✓  服务 ✓     连接 3 · 当前 ↑12KB/s ↓2MB/s · 总 ↑1.2GB ↓8.4GB
│   …（每行一条；空态 EmptyState + 「去内网穿透页」）
│   折叠：VPS 上 agent 管理的路由文件（3）—— 7023.36800.cc 已加载 / …
└ 弹窗：部署设置（基础 / 安全 / 高级）· 反代配置（只读 + 复制）
```

分区原则：**统计区只读汇总 / 环境区只读诊断 + 安装动作 / 规则区一行一规则（状态在左、动作在右）**；「使用说明」整卡删除，内容并入 `PageHeader.description` 与 `n-alert`。

---

## 七、组件与状态矩阵（Pass 2 规格）

| 元素 | default | hover | pressed | focused | disabled | loading |
| --- | --- | --- | --- | --- | --- | --- |
| 刷新按钮 | 库默认 | 库默认 | 库默认 | 库默认焦点环 | 无服务端时禁用 | `:loading="manualRefreshing"`（轮询不驱动） |
| 服务端选择器 | 库默认 | 库默认 | — | 库默认 | 无服务端时禁用 | — |
| 「部署反代」 | primary | 库默认 | 库默认 | 库默认 | 该行操作进行中 | `busy === 'deploy:<id>'` |
| 「移除反代」 | warning quaternary | 库默认 | 库默认 | 库默认 | 同上 | `busy === 'remove:<id>'` |
| 「一键安装 Nginx」 | primary | 库默认 | 库默认 | 库默认 | agent 未连接 | `:loading="installingNginx"` |
| 健康徽标 / 部署态 | `StatusBadge` | —（只读） | — | 不可聚焦 | — | 数据未加载时显示 `—` |
| 统计卡 | 令牌底色 | **有意不加 hover** | — | 不可聚焦 | — | 未加载显示 `—`（不是 0） |
| 折叠「路由文件」 | 原生 summary + 令牌样式 | 文字色变 | — | 库默认 | — | — |

**内容状态**：未加载 `—` / 首次加载 `n-spin` / 拉取失败 `n-alert error`（修 #1，`loadErrorMsg` 必须渲染）/ 无规则 `EmptyState` / 无路由文件 折叠区隐藏 / 流量采集失败该行标红并给 tooltip。

---

## 八、状态与数据流（含缺陷修复）

```ts
// 1) 切换服务端必须重置（修 #8）
watch(selectedId, () => {
  routeCandidates.value = []; routeDetails.value = []; routeHealth.value = []
  proxyTraffic.value = {}; trafficErrors.value = {}; agentStatus.value = null
  loadErrorMsg.value = ''
  void refreshAll({ silent: false })
})

// 2) 错误必须可见（修 #1）
const loadErrorMsg = ref('')   // 模板：<n-alert v-if="loadErrorMsg" type="error" :bordered="false">{{ loadErrorMsg }}</n-alert>

// 3) loading 分粒度（修 #7）
const manualRefreshing = ref(false)   // 仅"刷新/重新检测"按钮
type Action = `deploy:${number}` | `remove:${number}` | 'install-nginx' | 'save-settings'
const busy = ref<Action | null>(null)
async function run<T>(action: Action, fn: () => Promise<T>) { /* 同 AgentView：串行 + 失败写入 loadErrorMsg */ }

// 4) 轮询：静默 + 可见性保护（修 #9）
timer = window.setInterval(() => {
  if (!server.value || document.visibilityState === 'hidden') return
  void refreshAll({ silent: true })     // 不触碰 manualRefreshing，不覆盖 loadErrorMsg（失败仅记 console + 顶部提示去抖）
}, 15000)
```

`refreshAll` 的 6 个接口调用保持串行（`routeCandidates → routeDetails → routeHealth → traffic → status`），但**单个接口失败不再中断整轮**：分别 try/catch，把失败项汇总成一句提示（现状是一个失败就整轮丢失后续数据）。

---

## 九、本页不做（Never）

- 不引入第二套 UI 框架 / 令牌；不改全局字体与令牌；
- 不用硬编码色值（现状 6 处全部清除）与魔数间距；
- **不做** Nginx 配置手工编辑与回滚（公网配置由 agent 模板生成，手工编辑会与模板冲突；回滚需 agent 侧版本化，属后端能力）；
- **不做**流量时间序列曲线（frps 只提供当日累计 + 采样速率，无历史接口）；
- **不做**目标 Host 头改写、忽略后端 TLS 校验（会破坏 frps vhost 路由 / 回源是明文）；
- 不把「使用说明」这类静态文案单独占一张卡；
- 不在本页重复 Agent 生命周期操作（安装/升级/卸载 agent 归「公网 Agent」页，本页只放入口链接）。

---

## 十、验收清单

- [ ] 复用项目真实栈与令牌（naive-ui + `--fonu-*` + `PageHeader/FonuCard/LoadError/EmptyState/StatusBadge`），未新增第二套
- [ ] 单一设计方向（部署台账）落地；零硬编码色值；`style="width: …"` 清零
- [ ] 交互状态齐全：hover / pressed / focused（键盘实测）/ disabled / loading / empty / error
- [ ] 无障碍：对比度 AA（亮+暗，重点验 `--fonu-brand` / `--fonu-warning` / `--fonu-error` 在 `--fonu-surface` 上）、键盘可达全部动作、装饰图标 `aria-hidden`、健康徽标有可读文本
- [ ] 最窄 320px 无横向溢出；≤760px 规则行纵向堆叠、动作按钮铺满且 ≥44px
- [ ] **功能可见性：对照第三节 A–Q，每项有明确位置；#1 错误可见、#8 切换不串台、#7 loading 分粒度 三项缺陷已修**
- [ ] 回归：部署/移除/保存部署设置三条路径真机验证；`vue-tsc` 0 错误 + `vite build` 成功

---

## 十一、实施顺序与工作量

| 阶段 | 内容 | 预计 |
| --- | --- | --- |
| P1 缺陷修复 | #1 错误可见、#8 切换重置、#7 loading 分粒度、#9 轮询保护、#10 内联尺寸、#3 硬编码色值 | 1h |
| P2 结构对齐 | 统计区（4 卡）、环境与 Nginx 卡令牌化、删「使用说明」卡改 `n-alert` + 入口行、空态换 `EmptyState` | 1.5h |
| P3 规则行重排 | `StatusBadge` 化部署态与健康徽标、流量指标排版、移除二次确认、折叠区令牌化 | 1.5h |
| P4 弹窗收尾 | 部署设置字段校验与提示统一、反代配置弹窗加「复制」 | 0.5h |
| P5 验证 | tsc / build / 真机回归（部署·移除·保存设置） | 1h |

合计约 **5.5–6 小时**，单文件改动（`ProxyRemoteView.vue`），**不涉及后端与 agent**。
能力缺口 R（批量部署）与 S（配置回滚）不在本轮：R 可后续单独做（纯前端循环 + 进度），S 需 agent 侧新增历史接口。

验证：`npx vue-tsc --noEmit` + `npx vite build`，再同步 `web/dist → cmd/fonu/web/dist` 后 `go build ./...`。
