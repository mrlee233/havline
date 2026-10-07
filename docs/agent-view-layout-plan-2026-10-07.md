# 「公网服务端」页面排版与结构调整方案

> 日期：2026-10-07
> 对象：[AgentView.vue](F:/Fonu-main/havline/web/src/views/AgentView.vue)（2244 行 / 136 KB）
> 性质：只出方案，不含代码改动
> 参考页面：DdnsView.vue、ProxyRemoteView.vue、FrpView.vue

## 一、现状测绘

页面当前的 DOM 骨架：

```
PageHeader（公网服务端 + 刷新）
├─ LoadError / EmptyState
└─ .agent-layout（grid: 260-320px | 1fr）
   ├─ aside.server-sidebar            服务端列表
   └─ div.agent-detail
      ├─ .server-toolbar              7 个操作按钮平铺
      ├─ n-alert                      状态提示（互斥）
      ├─ .entrance-row                公网反代入口引导
      ├─ ① section.stats-row          4 张指标卡 + VPS 资源卡 + VPS 证书卡
      ├─ ② section.dep-grid           3 张卡（Agent / Nginx / frps 运行时）
      ├─ ④③ .control-grid             2 张卡（生命周期 / frps 控制）
      └─ ⑤ HavlineCard                输出与日志（内部两个 tab）

另外 5 个 n-modal 集中在 template 尾部：
  frps.toml 预览、服务端表单（390-615，共 225 行）、日志、服务端详情、frps 配置
```

页面要做的事：服务端增删改查、Agent 生命周期、frps 控制、frps.toml 预览下发、VPS 资源与证书巡检、操作输出与日志。

## 二、同类页面参考

| 页面 | 布局 | 详情区组织 | 操作按钮 |
| --- | --- | --- | --- |
| [DdnsView.vue](F:/Fonu-main/havline/web/src/views/DdnsView.vue:97) | `.ddns-layout` + `aside.task-sidebar` + `section.task-detail` | 头部 `.task-detail__head` + 若干 `HavlineCard.detail-section` | 集中在头部，分三级：主操作（primary ghost）、次级（默认）、危险（error） |
| [ProxyRemoteView.vue](F:/Fonu-main/havline/web/src/views/ProxyRemoteView.vue:27) | `.proxy-layout` + `.proxy-detail` | `stats-row` + 2 张 `section-card` | 数量少，未出现平铺 |
| FrpView.vue | 规则列表 + 弹窗 | 弹窗内分 tab | — |

全站通行模式：**指标用 `stats-row`，内容用 `HavlineCard.section-card`**，两者语义不同、不混用。这一点上「公网服务端」页是偏离的。

## 三、发现的问题

### A. 信息架构

**A1 同一份详情做了两套。** 主区已经完整展示了状态、依赖、生命周期、日志，工具栏上的「查看」又打开一个 `n-modal`，里面是基本信息 / 服务器状态 / 延迟曲线 / 代理 / 连接日志（[第 633-667 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:633)）。两边字段重叠，用户不知道该点哪个，维护时也容易只改一边。

**A2 区块编号与渲染顺序不符。** 注释写着 `<!-- ④ Agent 生命周期（SSH） + ③ frps 控制：两列并排 -->`（[第 239 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:239)），但 ① ② 之后直接是 ④，编号失去导航作用，反而让人以为漏了 ③。

**A3 主区没有内部导航。** `.agent-detail` 里纵向堆了 6 个区块，要看到「输出与日志」至少滚动 4-5 屏。左侧切服务端、右侧滚很久，是这个页面最明显的使用负担。

**A4 `stats-row` 混装了两种语义的卡片。** 4 张 `.stat-card`（指标）与 2 张 `HavlineCard.section-card.vps-resource-card`（内容）共用一个 4 列网格。样式里已经要靠特判来收拾：

```css
/* 前四张卡每行三张（各占 2/6），VPS 资源与 VPS 证书各占 3/6 并排 */
/* VPS 资源：占满「环境与依赖」整行，避免单独占一格显得突兀 */
.dep-grid > .vps-resource-card { grid-column: span 3; }
```

注释本身就说明了"不协调"。

**A5 工具栏 7 个操作完全平权。** [第 76-83 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:76)：启动/停止、查看、日志、测速、校验、编辑、删除并列，全部 `size="small"`，其中「删除」是 `quaternary type="error"`，与「测速」视觉权重接近。DdnsView 的做法是主操作 / 次级 / 危险三级分组，值得对齐。

### B. 排版与代码可读性

**B1 缩进错乱（DOM 正确）。** [第 86 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:86)起，「状态提示」到「输出与日志」的整段缩进比父级 `.server-toolbar` 浅两格，直到 [第 360 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:360)附近才恢复。实际层级是对的（`</div>` 在 361-362 行才闭合 `.agent-detail` / `.agent-layout`），但读代码时无法判断区块归属，后续改结构极易改错层级。

**B2 单行过长。** Nginx / frps 运行时两张卡的字段全部写成单行，最长的超过 200 字符（例如 [第 233 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:233)的"启动命令"）。diff、review 和合并冲突都不友好。

**B3 图标体系不一致。** 端口监听卡用文本符号 `{{ port }} {{ portOn(port) ? '✓' : '✗' }}`（[第 134 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:134)），页面其余位置统一用 `n-icon`。

**B4 冗余字号规则。** `.stat-card__value` 定义了 30px（[第 1841 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:1841)），但本页所有 stat-card 都加了 `--sm`（18px），30px 规则形同虚设，多半是从 FrpView 迁入时留下的。

### C. 文件结构

**C1 单文件职责过载。** 2244 行承担 7 类功能，`n-modal` 表单就占 225 行。任何一处调整都要在 136 KB 的文件里定位。

**C2 与 DDNS 页"同构"却各写一套。** [第 25 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:25)注释自己写明「与 DDNS 页（`.ddns-layout` / `.task-sidebar`）同构」，但类名与样式完全重复实现（`.agent-layout`/`.server-sidebar` vs `.ddns-layout`/`.task-sidebar`）。

**C3 样式分两段来源。** [第 2064 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:2064)有注释「以下样式随「公网服务端」列表与弹窗从内网穿透页迁入（2026-09-24）」，与前半段并存，存在历史迁移残留。

### D. 交互细节

**D1 侧栏高度写死。** `.server-sidebar__list { max-height: 720px; overflow: auto; }`（[第 2221 行](F:/Fonu-main/havline/web/src/views/AgentView.vue:2221)），服务端多时会出现"侧栏内部滚动 + 页面滚动"双重滚动。

**D2 选中态偏弱。** `.server-item--active` 只有边框色与一圈 1px 阴影，在深色主题下辨识度有限。

**D3 值得保留的部分。** `EmptyState` + `LoadError` + `statusAlert`（互斥最多一条）的设计是好的，不要在这次调整中丢掉。

## 四、优化方案

按"改动量 / 收益"排成三档，可分批做。

### 方案 A：低风险整理（建议先做）

不改信息架构，只修可读性与一致性。

1. 修正 86-361 行的缩进，使层级与 DOM 一致（纯格式）
2. 修正 ①②③④⑤ 注释编号，与渲染顺序对齐；或直接删掉编号，避免误导
3. 长单行字段拆行（Nginx / frps 运行时两张卡）
4. 端口监听卡的 `✓`/`✗` 换成 `n-icon`（如 `CheckmarkOutline` / `CloseOutline`）
5. 把 VPS 资源 / VPS 证书从 `stats-row` 移到独立的 `section-card` 容器，去掉 `grid-column: span 3` 特判
6. 工具栏按三级重组：`启动/停止`（主）→ `查看 / 日志 / 测速 / 校验 / 编辑`（次级）→ `删除`（危险，右侧分离）

预期：不改变用户可见的信息，但代码可读性与操作层级明显改善。

### 方案 B：信息架构调整（推荐）

1. **消除重复详情**：主区保留完整详情，把「查看」弹窗改为「深度诊断」并只保留主区没有的内容（延迟曲线、代理列表、连接日志），或直接删除该入口。二选一，不要并存。
2. **主区改 tab**：把 ①~⑤ 收进 `n-tabs`，例如
   - `概览`：stats-row + VPS 资源 / 证书
   - `环境与依赖`：dep-grid 三张卡
   - `运行控制`：控制网格两张卡
   - `日志与输出`：现有输出与日志卡

   这样主区高度从"5 屏"收敛到"1 屏 + 按需切换"，与本页"左侧选服务端、右侧看状态"的用法一致。
3. **侧栏高度自适应**：`.server-sidebar__list` 去掉固定 `max-height`，改由父级 `align-items: start` 与页面滚动统一处理，避免双滚动。
4. **选中态加强**：在现有边框基础上加左侧 2px 强调条或背景色差，提升深色主题下的辨识度。

### 方案 C：结构重构（收益最大，风险也最高）

1. 抽公共组件 `ListDetailLayout`（sidebar + detail 网格），由本页与 DdnsView 共用，消除 C2 的重复实现。
2. 拆分本页为多个子组件，建议边界：
   - `ServerListSidebar.vue`
   - `ServerOverviewPanel.vue`（stats + VPS 资源/证书）
   - `ServerDependencyPanel.vue`
   - `ServerControlPanel.vue`（生命周期 + frps 控制）
   - `ServerFormModal.vue`（225 行表单）
   - `ServerLogsPanel.vue`
3. 清理 C3 的历史迁移样式段，确认无引用后合并去重。

这一步做完，单文件预计从 2244 行降到 400 行以内，但需要回归测试本页全部交互（安装 Agent、切换传输模式、下发 frps 配置、启停 frps、轮换证书）。

## 五、建议的推进顺序

| 阶段 | 内容 | 风险 | 可独立上线 |
| --- | --- | --- | --- |
| 第一步 | 方案 A 全部 6 项 | 低（纯格式与样式） | 是 |
| 第二步 | 方案 B 的 1、2 项（去重复详情 + 主区 tab） | 中（改交互路径） | 是 |
| 第三步 | 方案 B 的 3、4 项 | 低 | 是 |
| 第四步 | 方案 C 拆分 | 中高（涉及多组件） | 是，建议按组件逐个剥离 |

## 六、不做的事

为避免过度重构，以下不在本次范围内：

1. **不改后端接口**：本次全部为前端结构与样式调整。
2. **不合并「公网服务端」与「公网反代」两个页面**：两者职责不同（一个管 Agent/frps 主机，一个管域名路由），页面之间已有 `.entrance-row` 引导，保持独立。
3. **不改动 `statusAlert` 互斥提示机制**与 `EmptyState` / `LoadError` 的既有约定。
4. **不动 `n-modal` 的字段与提交逻辑**：方案 C 拆分时只搬移结构，不重写表单校验。

## 七、验证方式

每阶段完成后需要确认：

1. `npm run typecheck` 与 `npm run build` 通过
2. 在 3 档宽度下目视核对布局：≥1280px（双列）、1100px（切换单列断点）、≤760px（移动端）
3. 深色 / 浅色两套主题下检查选中态、告警、徽标对比度
4. 关键交互回归：切换服务端、安装/升级 Agent、切换传输模式（tunnel / https-pin / http）、下发 frps 配置、启停 frps、打开日志
