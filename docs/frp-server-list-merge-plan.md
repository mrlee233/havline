# FRP 服务端列表与公网 Agent 页合并方案

> 问题：`/frp`（内网穿透）页里的「公网服务端」列表，是否应该并入 `/frp-agent`（公网 Agent）页——像 DDNS 页那样「列表 + 详情同页」？
>
> 依据：本仓库 `web/src/layouts/AppLayout.vue`、`web/src/router/index.ts`、`web/src/views/{FrpView,AgentView,DdnsView,ProxyRemoteView}.vue`（读代码确认，非推测）。
> 更新日期：2026-09-24

---

## 一、现状盘点

### 1.1 侧边栏与三个 FRP 相关页

`web/src/layouts/AppLayout.vue:171-179` 的菜单顺序：

| 菜单 | 路由名 | 视图 | 页面职责 |
| --- | --- | --- | --- |
| 内网穿透 | `frp` | `FrpView.vue` | 客户端状态 + **公网服务端列表（增删改查 + 启停/测速/校验/日志）** + 穿透规则列表 + 8 个弹窗 |
| 公网 Agent | `frp-agent` | `AgentView.vue` | **单个服务端**的 agent 详情：运行状态 / 环境与依赖（含 VPS 资源、VPS 证书）/ frps 控制 / Agent 生命周期（SSH）/ 输出与日志 |
| 公网反代 | `public-proxy` | `ProxyRemoteView.vue` | 单个服务端的公网反代台账（部署 / 移除 / 健康 / 漂移） |

### 1.2 重叠矩阵（同一件事现在有几处入口）

| 能力 | 内网穿透页 | 公网 Agent 页 | 公网反代页 | 说明 |
| --- | --- | --- | --- | --- |
| 服务端增删改 | ✅ 列表 + 弹窗表单 | ❌ | ❌ | 唯一入口在 `/frp` |
| 服务端列表（名称/主机/端口/延迟/位置/配置/启动状态） | ✅ `n-data-table` 8 列 | ❌（只有顶部下拉选服务端） | ❌（顶部下拉） | **同一份数据的两种呈现** |
| 启动 / 停止 frpc | ✅ 行内按钮 | ❌ | ❌ | |
| 查看服务端详情 | ✅ 弹窗（代理/服务器状态/诊断） | ✅ 整页 | ❌ | **内容重叠** |
| frps 安装 / 启停 / 版本 | ❌ | ✅ | ❌ | |
| Agent 安装 / 升级 / 卸载 / SSH 诊断 | ❌（只有「Agent 管理」跳转按钮） | ✅ | ❌ | |
| Agent 环境探测 / Nginx 安装 | ❌ | ✅ | ❌ | |
| 穿透规则 | ✅ | ❌ | ❌ | 内网穿透页的核心 |
| 公网反代规则 | ❌ | ❌ | ✅ | |

**结论**：服务端本身（连接配置 + 启停）归 `/frp`，而「这台服务端上的 agent 与 frps 状态」归 `/frp-agent`。两页通过 `/frp` 列表行内的「Agent 管理」按钮跳转，跳转后 AgentView 用**顶部下拉**重新选一遍服务端——**同一个对象被选两次**，这是用户感到割裂的根源。

### 1.3 现有跳转关系（合并时要一并处理）

| 来源 | 目标 | 位置 |
| --- | --- | --- |
| `/frp` 服务端行「Agent 管理」 | `/frp-agent` | `FrpView.vue` `serverColumns` 操作列 |
| `/frp` 穿透规则区（未配置服务端时的空态） | 本页「添加服务端」 | `FrpView.vue` |
| `/public-proxy` 空态「尚未配置 FRP 服务端」 | `/frp` | `ProxyRemoteView.vue` |
| `/frp-agent` Nginx 卡「去『公网反代』处理」 | `/public-proxy` | `AgentView.vue` |
| `/ddns` 顶部提示「FRP 内网穿透已启用」 | `/frp` | `DdnsView.vue` |

---

## 二、DDNS 页作为参照：它是什么模式

`DdnsView.vue` 的结构（`PageHeader` → `stats-row` → 详情卡 → 弹窗）：

- **一页承载「列表 + 选中项详情」**：页面顶部是全局动作（「全部刷新」）与 5 张统计卡（外网 IP 检测 / 任务总数 / 正常 / 异常 / 最后同步），下面直接是**当前任务的详情分区**（`detail-section` 卡片），新增/编辑走弹窗。
- 也就是说：**用户不需要在「列表页」和「详情页」之间跳**，选中的对象在同一页内切换。

对照到 FRP：`/frp-agent` 现在已经完全是「单对象详情页」的形态（顶部下拉选服务端 + 分区详情），**缺的正是 DDNS 那样的列表**——把 `/frp` 的服务端列表搬进来，形态立刻与 DDNS 一致，而且顶部下拉可以退化为「列表选中行」的结果（或保留下拉作为快捷切换）。

---

## 三、三个方案对比

| 方案 | 做法 | 优点 | 缺点 | 工作量 |
| --- | --- | --- | --- | --- |
| **A（推荐）** | 把服务端列表并入 `/frp-agent`：页面改为「统计区 + 服务端列表 + 选中服务端的 agent 详情」；`/frp` 只保留客户端状态与穿透规则 | 与 DDNS 形态一致；服务端管理只在一处；列表与详情同屏，不再「选两次」；`/frp` 回归「穿透规则」本职 | 服务端表单弹窗（含 frps 选项 / 远程 / Agent / SSH 四个分栏）要搬到 AgentView；两页文案与跳转需同步 | 中，约 1–1.5 天 |
| **B** | 反向合并：把 agent 详情塞进 `/frp` 行内的「查看」弹窗 | 不动列表位置 | AgentView 现有 5 个分区 + 2 张并排卡 + 日志区，弹窗塞不下；等于推倒已成型页面重做 | 大且不划算 |
| **C** | 不动结构，只补深链：「Agent 管理」跳转带 `?server=<id>`，AgentView 读 query 自动选中 | 改动最小（约 0.5h）；消除「再选一次」的割裂感 | 仍是两个页面，服务端管理仍分散 | 小 |

**推荐 A，并把 C 作为 A 的第一步**：即使 A 中途暂停，深链带来的收益已经落地，且不会与 A 的后续步骤冲突。

---

## 四、方案 A 详细设计

### 4.1 目标页面结构（`/frp-agent`，参照 DDNS）

```
PageHeader「公网服务端」（原「公网 Agent」）
  actions: [选择服务端 ▾]（快捷切换，选项与列表一致）  [刷新]
├─ stats-row：沿用现有 5 张卡（Agent 状态 / Nginx / frps 服务 / 反代路由 / 端口监听）
├─ 服务端列表（从 /frp 迁入）
│    n-data-table 8 列：名称 / 主机 / 端口 / 延迟 / 位置 / 配置 / 启动状态 / 操作
│    操作列：启动·停止 · 查看 · 日志 · 测速 · 校验 · 编辑 · 删除
│            （去掉「Agent 管理」——它就在本页；行点击即选中）
├─ 选中服务端的详情（现有 AgentView 分区原样搬过来）
│    环境与依赖（Havline Agent / Nginx / frps 运行时 / VPS 资源 / VPS 证书）
│    frps 控制 + Agent 生命周期（SSH）
│    输出与日志
└─ 弹窗：服务端表单（新增/编辑）、服务端日志、服务端详情、frps 同源配置、导出、FRP 应用配置
```

`/frp`（内网穿透）保留并收敛为：

- 4 张状态卡（客户端状态 / 启用规则 / frpc 版本 / 最近应用）
- 穿透规则列表 + 规则相关弹窗（规则表单、规则详情、FRP 应用配置）
- 空态：「尚未配置服务端」→ 改为链接到 `/frp-agent`（原来是本页直接开「添加服务端」弹窗）

### 4.2 数据流与选中态

- 列表数据 `api.listFrpServers()`：两页现在各自拉一次，迁入后只拉一次（AgentView 本来就在拉）。
- 选中态：沿用 AgentView 的 `selectedId`；新增**行点击选中 + 选中行高亮**。
- 深链：`/frp-agent?server=<id>` → 初始化时读 query 决定 `selectedId`（即方案 C，作为第一步落地）。
- `/frp` 的「Agent 管理」跳转改为 `router.push({ name: 'frp-agent', query: { server: id } })`。
- 列表为空：整页 EmptyState（AgentView 已有）+「添加服务端」按钮直接开弹窗（不再跳去 `/frp`）。

### 4.3 弹窗归属（关键决定）

| 弹窗 | 现在位置 | 迁移后 | 判据 |
| --- | --- | --- | --- |
| 服务端表单（新增 / 编辑） | `/frp` | `/frp-agent` | 服务端的「家」在这里；`/frp` 不再需要 |
| 服务端日志 / 服务端详情 | `/frp` | `/frp-agent` | 都是单服务端视角，与详情区同屏更好 |
| frps 同源配置 / 导出脱敏配置 | `/frp` | `/frp-agent` | 依赖服务端的 frps 选项 → 属服务端视角 |
| 规则表单 / 规则详情 / FRP 应用配置 | `/frp` | 不动 | 与穿透规则绑定 |

实现时以「弹窗依赖的是**服务端**还是**规则**」为判据，不要按文件位置机械搬运。

### 4.4 命名与文案

- 页面标题与菜单：`公网 Agent` → **`公网服务端`**（或「公网服务端（Agent）」，二者都行，推荐前者，因为它现在同时管服务端与 agent）。
- **路由名 `frp-agent` 与路径 `/frp-agent` 保持不变**，避免破坏深链、侧边栏高亮与已有跳转；只改显示文案。
- `/frp` 的「公网服务端」卡片整体移除，其 `subtitle`（"可添加并管理多个 frps 服务端连接"）文案迁到新页描述。

### 4.5 空态与降级

- 没有服务端：`/frp-agent` 显示 EmptyState + 「添加服务端」；`/frp` 规则区空态改为「先去『公网服务端』添加服务端」。
- 服务端未配 agent/SSH：详情区沿用现有降级（`agentStatusOk`、状态告警），列表「配置」列沿用现有 TLS/Token 标签。
- agent 版本过低：沿用 404/405 降级文案，无需新逻辑。

---

## 五、迁移步骤（四步，每步独立可交付 / 可回滚）

| 步骤 | 内容 | 交付标准 | 预估 |
| --- | --- | --- | --- |
| **步 1 · 深链** | `/frp` 的「Agent 管理」改为 `router.push({ name: 'frp-agent', query: { server: id } })`；`AgentView` 初始化读 query 决定 `selectedId` | 从任意一行点进 Agent 页，直接落在该服务端（不再需要再选一次） | 0.5h |
| **步 2 · 列表迁入** | 把 `serverColumns`、列表卡片、行操作、服务端表单 / 服务端日志 / 服务端详情弹窗整体搬到 `AgentView`；行点击即选中并高亮；`FrpView` 删除该卡片与相关弹窗、`serverColumns`、`openServerModal` 等 | 新增 / 编辑 / 启停 / 删除 / 测速 / 校验 / 日志全部在新页可用；`/frp` 只剩状态卡 + 规则 | 0.5 天 |
| **步 3 · 收尾** | 菜单与页面标题改「公网服务端」；`/frp` 规则区空态改指向新页；`ProxyRemoteView` / `DdnsView` 里「去内网穿透页添加服务端」这类文案改指向 `/frp-agent`（指向规则页的链接不动）；核对侧边栏顺序（内网穿透 → 公网服务端 → 公网反代） | 所有入口文案自洽 | 0.25 天 |
| **步 4 · 清理** | 删除迁移后残留的死样式与死变量（如 `/frp` 里只服务端用到的 `.server-detail*`、`.frp-detail-card` 等）；`grep` 校验无残留引用 | `vue-tsc` + `vite build` 通过，grep 旧类名/旧函数名为 0 | 0.25 天 |

**实施状态（2026-09-24）**：**步 1、步 2 已完成**。

- **步 1 · 深链**：`FrpView.vue` 的「Agent 管理」改为 `{ name: 'frp-agent', query: { server: String(row.id) } }`；`AgentView.vue` 新增 `useRoute` 并在 `selectedId` 初始化时读 `?server=<id>`（非法 id 由 `load()` 回落为第一台）。
- **步 2 · 列表迁入**：服务端列表卡片、服务端表单 / 日志 / 详情 / frps 同源配置四个弹窗、`serverColumns` 与全部服务端脚本、相关样式整体迁入 `AgentView.vue`（971 → 1935 行），`FrpView.vue` 删除同批代码（2189 → 1425 行）。实施要点：
  - 行点击即选中（`serverRowProps` + `.frp-server-row--active` 高亮），与顶部下拉共用 `selectedId`；操作列去掉「Agent 管理」，单元格加 `stopPropagation` 避免点按钮时误选。
  - 列表改为展示**全部**服务端（原先按 `agent_configured || ssh_configured` 过滤），顶部下拉同步放开过滤——否则未配 Agent 的服务端无法在本页编辑 / 删除。
  - 列表为空时整页 EmptyState + 「添加服务端」直接开表单（不再跳去 `/frp`）；`/frp` 规则区在「无服务端」时给出指向 `/frp-agent` 的空态，避免按钮被禁用后无处可去。
  - 服务端表单里的「打开管理面板」改为「查看服务端详情」（关弹窗 + 页内选中）；详情弹窗「客户端」一行所需的 `runtime` 由新增的 `loadRuntimeVersion()` 拉取。
  - 详情弹窗「代理」表依赖的 `proxyPublicEndpoint` 在 `AgentView` 本地补了一份（原属规则侧）；表单里的「下发 frps 配置」复用 `AgentView` 既有的 `run('push-config', …)` 版本，输出进「输出与日志」。
  - **未迁移**：`/frp` 的「查看脱敏配置」弹窗（`showExport`）留在原页——触发按钮在「穿透规则」卡片头部，导出物是脱敏 frpc.toml，属客户端 / 规则视角。这是按 §4.3 自己的判据（弹窗依赖服务端还是规则）做的取舍，与 §4.3 表格里「导出脱敏配置 → 迁走」一行不同。
- 验证：`vue-tsc --noEmit` 0 错误、`vite build` 通过、`FrpView.vue` 内旧标识符 grep 为 0、全仓无 0 字节源文件、`bin\havline.exe` 已重建。
- **踩坑记录（scoped 样式不随模板搬迁）**：`.frp-proxy-modal` 系列规则在 `FrpView.vue` 里分散在多处——1250–1260 的弹窗外壳、1263–1282 的说明栏 / 高级折叠 / 服务端选项区。只搬前一段时，说明栏失去 `width: 300px; flex-shrink: 0`，在 `display: flex` 的弹窗里按内容宽度膨胀，把 `flex: 1` 的表单列挤成一条，表现为标题重叠、页签一字一行。**步 4 清理时注意**：`.proxy-modal*` / `.frp-proxy-modal*` 在 `/frp` 仍被规则表单弹窗使用，不能删。
- **布局调整（2026-09-24，用户要求对齐 DDNS 页）**：`/frp-agent` 改为 DDNS 那样的**左列表 + 右详情**——`.agent-layout { grid-template-columns: minmax(260px, 320px) minmax(0, 1fr) }`，≤1100px 收成一列；侧栏结构照抄 DDNS 的 `.ddns-layout` / `.task-sidebar`（类名换成本页语义的 `.server-sidebar*` / `.server-item*`）。
  - 原 8 列 `n-data-table` 换成紧凑列表项：名称 + 启动状态标签 + `主机:端口` + 延迟 / 位置 + TLS / Token / Agent / SSH 标签；行点击即选中（`--active` 高亮）。
  - 行内操作移到右侧工具条 `.server-toolbar`（启动·停止 · 查看 · 日志 · 测速 · 校验 · 编辑 · 删除），标题旁显示当前服务端名与端点。
  - `serverColumns` / `serverRowProps` / 顶部「选择服务端」下拉及其 `serverOptions` 一并删除（左侧列表即选择器）；页面标题改「公网服务端」。
  - 与 §4.1 里「统计区 → 列表 → 详情」的**堆叠**结构不同，这是用户确认后的取舍；右侧内容顺序不变（状态提示 → 公网反代入口 → ① 运行状态 → ② 环境与依赖 → ③④ → ⑤ 输出与日志）。
- **`/frp` 布局调整（2026-09-24，用户两次纠正后定稿）**：`/frp` 改成**左：服务端列表 / 右：该服务端的穿透规则**——左侧 `.server-sidebar`（名称 + `boot_status` 状态标签 + `主机:端口` + 规则数 / 启用数 + TLS / Token / Agent / SSH 标签，点行即切换服务端）；右侧 `.rule-pane`（头部显示当前服务端与规则条数，含「查看脱敏配置 / 管理该服务端（深链到 `/frp-agent?server=<id>`）/ 添加规则」）+ **原来的穿透规则表格**（恢复 `columns`，`:data="serverProxies"` 按选中服务端过滤；按用户要求删掉冗余的「服务端」列，`:scroll-x` 2020 → 1890）；规则详情回到弹窗（表格「详情」按钮 → `openProxyDetail`，速率采样随弹窗开关）。顶部 4 张客户端状态卡保持整宽。左侧列表的状态标签只用列表接口就带的 `boot_status`，不为此重新引入已迁走的 `serverDiagnostics` / `serverStartupText`。
- **步 3（文案与入口收尾）、步 4（死样式清理）待做**。

### 5.1 步 2 执行清单（含 `FrpView.vue` 代码锚点）

> 行号为 2026-09-24 快照；执行时以标识符为准（行号仅用于快速定位）。

**模板（迁出 → `AgentView.vue`）**

| 片段 | 行 | 备注 |
| --- | --- | --- |
| 「公网服务端」卡片（含 `serverColumns` 驱动的 `n-data-table`） | 53–77 | 卡片本身整块搬走 |
| 服务端表单弹窗 | 约 350–500 | 四个分栏；内含「公网 Agent 管理」段（219–236），其中 `<router-link to="/frp-agent">` 迁入后应改为**页面内选中** |
| 服务端日志弹窗 | 512–528 | |
| 服务端详情弹窗 | 529–564 | `preset="card"` + `.frp-detail-card` |
| 导出脱敏配置弹窗 | 565–576 | 触发点在 81 行按钮 |
| frps 同源配置弹窗 | 580–600 | 含 P1-3 的「比对 VPS 实际文件」 |

**脚本（迁出）**

- 状态：`servers`(924)、`serverDiagnostics`(932)、`testingId`(935)、`agentActionId`(937)、`savingServer`(939)、`showServerModal`(941)、`showExport`(943)、`showServerDetail`(946)、`showFRPSConfig`(949)、`frpsConfigContent`(950)、`frpsConfigTarget`(992)、`serverFormTab`(996)、`exportContent`(997)、`editingServer`(1015)、`viewingServer`(1016)、`serverForm`(1037)、`showServerLogs`(1408)
- 函数：`openServerModal`(1370)、`openServerDetail`(1381)、`openServerLogs`(1413)、`saveServer`(1510)、`testAgent`(1534)、`pushAgentConfig`(1541)、`startServer`(1599)、`stopServer`(1610)、`testServer`(1772)、`openFRPSConfig`(1797)
- `serverColumns`（约 1160–1200，其中 1180 行的「Agent 管理」`RouterLink` 迁入后删除）
- 一并迁出的辅助：`serverStartupText` / `latencyOf` / `buildServerPayload`（内联在 `saveServer`）/ `ConfiguredSecretField` 组件与相关图标导入

**留在 `/frp`**：4 张状态卡、穿透规则列表、规则表单 / 规则详情 / FRP 应用配置等规则侧弹窗与逻辑。

**执行注意**

1. 迁出后 grep `FrpView.vue` 里旧标识符（`serverForm` / `showServerModal` / `serverColumns` / `openServerLogs` / `frpsConfigTarget` / `showExport` / `editingServer` / `viewingServer`）应为 **0**；
2. `busy` / `run()` / `message` 若被规则侧共用，**不要整体迁走**——保留在 `FrpView` 或按需抽成 composable；
3. 迁入 `AgentView` 后：行点击 = `selectedId = row.id`，选中行加高亮类；操作列去掉「Agent 管理」；
4. 迁完后 `/frp` 的规则区空态改指向 `/frp-agent`（步 3）。

---

## 六、影响面清单

- **前端**：`FrpView.vue`（删列表与 4 个服务端弹窗 + 死样式）、`AgentView.vue`（加列表与弹窗 + 行选中）、`AppLayout.vue`（菜单文案）、`DdnsView.vue` / `ProxyRemoteView.vue`（指向服务端配置的文案与链接）、`router/index.ts`（**不改**，仅确认）。
- **后端**：**零改动**——所有接口都已存在且两页共用（`listFrpServers` / `createFrpServer` / `updateFrpServer` / `deleteFrpServer` / `startFrpServer` / `stopFrpServer` / 诊断与日志等）。
- **文档**：`docs/agent-view-design-v2.md` 若把「公网 Agent」定义为「单服务端详情页」，需补一句「现已并入服务端列表」；本文在实施完成后标注状态。

---

## 七、风险与回滚

| 风险 | 说明 | 应对 |
| --- | --- | --- |
| 弹窗搬迁漏掉入口 | 某个弹窗可能不止一个触发点 | 迁移前对每个弹窗 `grep` 触发点逐个确认归属；搬迁后 grep 旧文件确认无残留引用 |
| `selectedId` 语义冲突 | 两页各有一份选中态 | 迁入后 `/frp` 不再持有 `selectedId`，并确认没有别的逻辑依赖它 |
| 首屏变慢 | 列表 + 状态 + 资源 + 证书并发请求 | 列表与状态走同一轮 `Promise.all`，资源与证书仍单独异步（沿用现状） |
| 深链指向不存在的 id | 手工改 URL 或服务端已删除 | 回落为列表第一个（AgentView 现有逻辑已是「不在列表就取第一个」） |
| 回滚 | 步 2 是主要风险点 | 四步各自独立提交：步 1/2/3 可分别 revert，不会互相牵连 |

---

## 八、验收清单

- `/frp-agent` 一页内完成：选服务端（列表行或顶部下拉）→ 看 Agent / Nginx / frps / VPS 资源 / VPS 证书 → 启停 frpc → 编辑 / 删除服务端 → 看服务端日志、详情与 frps 同源配置。
- `/frp` 只剩客户端状态卡 + 穿透规则；规则的新增 / 编辑 / 应用 / 导出不受影响。
- 深链：从任意页用 `?server=<id>` 直达该服务端；id 不存在时不报错。
- 文案自洽：菜单、页面标题、空态、跨页链接都指向 `/frp-agent`。
- 构建：`vue-tsc --noEmit` 0 错误、`vite build` 通过、无残留死代码（grep 旧类名与旧函数名）。

---

## 九、明确不做

1. **不把「穿透规则」也搬进 AgentView**：规则属于内网穿透页，且 `/frp` 需要保留「状态卡 + 规则」以维持「配置 → 应用 → 查看」的闭环。
2. **不把 `/public-proxy` 的反代台账并进来**：它虽按服务端维度，但属另一业务，且已有统计区 + 台账 + 漂移清单的成熟形态。
3. **不改路由路径与路由名**（只改显示文案），避免破坏深链、侧边栏高亮与已有跳转。
4. **不动后端接口**：本次是纯前端信息架构调整。
