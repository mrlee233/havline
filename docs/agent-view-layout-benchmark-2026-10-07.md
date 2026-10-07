# 「公网服务端」页面布局对标与优化方案（GitHub 开源项目版）

> 日期：2026-10-07
> 对象：[AgentView.vue](F:/Fonu-main/havline/web/src/views/AgentView.vue)（2244 行 / 136 KB）
> 性质：只出方案，不含代码改动
> 前置文档：[agent-view-layout-plan-2026-10-07.md](F:/Fonu-main/havline/docs/agent-view-layout-plan-2026-10-07.md)（站内同类页面对比）
> 本文补充的是**站外开源项目的对标注解**，两者可合并使用

## 一、检索方法与候选项目

通过 GitHub Search API 按 `frp web panel`、`homelab dashboard`、`server management panel`、`admin dashboard list detail` 四组关键词检索，按 star 排序取候选，再拉取各仓库的默认分支文件树定位前端页面目录。数据抓取时间：2026-10-07。

| 项目 | Star | 技术栈 | 定位 | 参考价值 |
| --- | --- | --- | --- | --- |
| [louislam/uptime-kuma](https://github.com/louislam/uptime-kuma) | 92,180 | Vue 3 | 自托管监控面板 | **最高**，同为「列表 + 详情」形态 |
| [djylb/nps](https://github.com/djylb/nps) | 1,079 | Go + Web UI | 内网穿透 / 反向代理面板 | 高，业务同源 |
| [MCSManager/MCSManager](https://github.com/MCSManager/MCSManager) | 4,982 | TypeScript + Vue | 多实例服务器管理面板 | 中，多节点组织方式 |
| [27260102/frp-admin](https://github.com/27260102/frp-admin) | 9 | React + Ant Design | FRP 管理面板 | **最高**，业务完全同源 |
| [Gogowwww/frp-manager](https://github.com/Gogowwww/frp-manager) | 4 | Python | frps/frpc 管理面板 | 低，仅参考功能面 |
| [BloretCrew/BloraPanel](https://github.com/BloretCrew/BloraPanel) | 1 | Go + Vue 3 | 多节点服务器面板 | 低 |

说明：`nps` 与 `BloraPanel` 的文件树未成功取到前端页面目录，未纳入结构对比；其余项目只取了页面文件与目录结构，未做完整代码审计。

## 二、对标项目的布局拆解

### 2.1 uptime-kuma：列表与详情拆到路由级

这是本次最有价值的发现。它把「列表 + 详情」拆成了**三个独立路由页面**：

```
src/pages/
├─ List.vue          36 行   ← 入口页极薄，只做一件事：
│                              <MonitorList :scrollbar="true" />
├─ Details.vue     1053 行   ← 单个监控项的详情页
├─ EditMonitor.vue           ← 编辑表单独立成页
└─ Dashboard.vue             ← 概览
```

关键点：

1. `List.vue` 只有 36 行，列表的全部逻辑（分组、状态点、实时心跳条、排序）都在 `MonitorList` 组件里。入口页面不承载业务。
2. 详情是**独立路由**（`/details/:id`），不是同页右栏，也不是弹窗。用户可以前进后退、可以直接分享链接。
3. 详情页内部继续拆子组件（心跳条、图表、历史记录），单文件控制在千行量级且职责单一。
4. 编辑用独立页面，与「查看」彻底分离。

### 2.2 frp-admin：业务同源，页面按职责切分

```
frontend/src/
├─ components/MainLayout.tsx
└─ pages/
   ├─ Dashboard.tsx      概览
   ├─ ClientList.tsx     客户端 / 代理列表
   ├─ ProxyStatus.tsx    代理运行状态
   ├─ ServerConfig.tsx   服务端配置
   ├─ Settings.tsx       设置
   └─ Login.tsx
```

关键点：

1. **配置（ServerConfig）与运行状态（ProxyStatus）分开**，不塞进同一个页面。
2. 从 `ClientList.tsx` 的导入可以看到它用 `Tabs` + `Collapse` + `Divider` 组织信息密度：

```ts
import {
  Card, Table, Button, Space, Modal, Form, Input, Select, message, Popconfirm, Tag,
  InputNumber, Empty, Alert, Tabs, Collapse, Switch, Divider,
} from 'antd';
```

3. 危险操作统一用 `Popconfirm` 包裹，不与普通按钮并列平铺。
4. 代理类型、健康检查类型等用**数据表驱动**渲染（`proxyTypes`、`healthCheckTypes`、`commonPorts` 常量表），而不是每个表单项手写一遍。这一点 Havline 的 FrpView 已经做到了，AgentView 的表单还没有。

### 2.3 MCSManager：容器 + 组件化

`frontend/src/views/` 下只有 `LayoutContainer.vue`、`Login.vue`、`Install.vue`、`SsoBindLogin.vue`——业务页面全部落在 `components/` 下的可复用组件里，视图层只负责布局壳。

## 三、对标结论：六条可借鉴的模式

把上面的做法逐条对到 Havline「公网服务端」页：

| # | 开源做法 | Havline 现状 | 差距 |
| --- | --- | --- | --- |
| 1 | 列表与详情是独立路由（uptime-kuma） | 同页左右分栏 | 详情无法直链，返回键行为不符合直觉 |
| 2 | 列表逻辑抽成组件，入口页 36 行（uptime-kuma） | 全部内联在 2244 行单文件 | 差距最大的一条 |
| 3 | 编辑表单独立成页 / 独立组件（uptime-kuma `EditMonitor.vue`） | 225 行弹窗内联在 view 里 | 表单无法单独测试与复用 |
| 4 | 配置与运行状态分页（frp-admin） | 同页堆叠 6 个区块 | 见前置文档 A3 |
| 5 | `Tabs` + `Collapse` 收敛信息密度（frp-admin） | 主区全部展开，需滚 4-5 屏 | 见前置文档 A3 |
| 6 | 数据表驱动表单选项 + `Popconfirm` 隔离危险操作（frp-admin） | 表单字段手写；删除与测速同级 | 见前置文档 A5 |

另外有一处**反向印证**：uptime-kuma 的详情页靠子组件拆分控制在千行量级，而不是靠 `max-height` 加内部滚动。Havline 侧栏 `max-height: 720px` 的写死高度（前置文档 D1）属于相反的思路。

## 四、优化方案（对标版）

在原方案 A/B/C 基础上，按对标结果重新编排。**方案 A 不变**（纯格式与样式，风险最低），下面只讲 B、C 的对标调整。

### 方案 B′：详情内聚 + 主区 tab（对应差距 4、5）

1. 主区改 `n-tabs`，四个页签：`概览` / `环境与依赖` / `运行控制` / `日志与输出`。
   - 对齐 frp-admin 的「配置与状态分开」思路，把「运行控制」独立成页签
2. 消除重复详情（前置文档 A1）：删除「查看」弹窗，或将其降级为「深度诊断」并只保留主区没有的内容
3. 侧栏去掉写死的 `max-height`，改为页面统一滚动

### 方案 C′：按 uptime-kuma 的粒度拆分（对应差距 1、2、3）

**C′-1 组件拆分**（先做，收益直接）

| 新组件 | 承担的现有代码 | 预估行数 |
| --- | --- | --- |
| `ServerListSidebar.vue` | 侧栏列表 + 列表项 | ~120 |
| `ServerOverviewPanel.vue` | stats-row + VPS 资源 + VPS 证书 | ~150 |
| `ServerDependencyPanel.vue` | dep-grid 三张卡 | ~120 |
| `ServerControlPanel.vue` | 生命周期 + frps 控制 | ~180 |
| `ServerFormModal.vue` | 表单弹窗（390-615 行） | ~230 |
| `ServerLogsPanel.vue` | 输出与日志 | ~90 |

完成后 `AgentView.vue` 预计降到 400 行以内（壳 + 状态编排 + 弹窗挂载），与 uptime-kuma 的 `List.vue` 定位一致。

**C′-2 路由级拆分**（可选，改动较大）

如果希望进一步对齐 uptime-kuma，可把详情拆成独立路由：

```
/servers            列表（可保留当前左栏）
/servers/:id        详情（页签：概览 / 环境 / 控制 / 日志）
```

好处是详情可直链、可前进后退、可分享；代价是列表与详情间的切换从"点一下"变成"路由跳转"，对高频切换场景反而略慢。**建议先做 C′-1，观察实际使用是否真的需要路由级拆分，再决定是否做 C′-2。**

**C′-3 表单数据表驱动**（对齐差距 6）

参考 frp-admin 的 `proxyTypes` / `commonPorts` 常量表，把 AgentView 服务端表单里重复的字段声明抽成配置数组，减少手写重复项，也顺带缩短 `ServerFormModal.vue`。

### 不做的事（补充）

在原文档「不做的事」基础上，再补两条：

1. **不引入 antd / Element Plus 等第二套组件库**：对标的是布局思路，不是 UI 框架。Havline 已统一使用 Naive UI。
2. **不照搬 uptime-kuma 的 `v-html` 描述区**：那里有 `eslint-disable vue/no-v-html` 注释，属于其自身的历史包袱，不适合作为参考。

## 五、落地顺序

| 阶段 | 内容 | 对标来源 | 风险 |
| --- | --- | --- | --- |
| 1 | 原方案 A 全部 6 项 | — | 低 |
| 2 | 方案 B′（页签 + 去重复详情 + 侧栏滚动） | frp-admin | 中 |
| 3 | C′-1 组件拆分（按表逐个剥离） | uptime-kuma `List.vue` | 中 |
| 4 | C′-3 表单数据表驱动 | frp-admin | 低 |
| 5 | C′-2 路由级拆分（视需要） | uptime-kuma 路由 | 中高 |

前三步做完，页面结构与这两个成熟项目的差距基本拉平；第 4、5 步属于进一步优化。

## 六、验证方式

沿用前置文档第七节的验证清单（`typecheck` / `build`、三档宽度目视、深浅主题、关键交互回归）。组件拆分阶段额外需要：

1. 每剥离一个组件就单独提交，保证可回滚
2. 剥离过程中不改任何 props 之外的行为，提交信息里注明「仅搬移结构」
3. 全部拆完后回归：切换服务端、安装/升级 Agent、切换传输模式、下发 frps 配置、启停 frps、轮换证书、查看日志
