# 公网 Agent 页（`AgentView.vue`）整合整改方案

> 目标：把「公网 Agent」页从"功能堆叠"整理成**状态 → 控制 → 生命周期 → 输出**四段式结构，
> 信息层级清晰、操作归属明确、危险操作可控，并完全符合 `fonu-page-ui` 页面 UI 标准。
>
> 参照页：`DdnsView.vue`（状态卡 + 提示）、`CertificatesView.vue`（统计卡规格）、`ProxyView.vue`（分组与按钮区）、`SettingsView.vue`（分区）。
> 更新日期：2026-09-23

---

## 一、现状问题清单

| # | 问题 | 具体表现 | 影响 |
| --- | --- | --- | --- |
| 1 | **状态与操作混在一起** | 4 张状态卡只覆盖「Agent 连接 / frps 服务 / 80 / 443」；而 Agent 版本、frps 启动命令、配置路径、开机自启、7000 端口散落在「frps 服务控制」卡的 `field-grid` 里，与启停按钮同处一卡 | 看状态要跨两张卡，操作区被字段挤占 |
| 2 | **两个 `agent-page-grid` 列宽漂移** | 均用 `repeat(auto-fit, minmax(360px, 1fr))`，窗口宽度变化时卡片宽度与列数跳动，与标准页 `repeat(4, minmax(0,1fr))` 的稳定网格不一致 | 视觉不整齐，窄屏断裂点不可预期 |
| 3 | **loading 粒度太粗** | 所有按钮都用 `:loading="agentActionId === server.id"`——任何操作进行中，页面**全部**按钮一起转圈 | 无法分辨在跑哪个操作，也无法阻止并发操作 |
| 4 | **输出全部塞进一个 `agentOutput`** | 探测 / 安装 / 卸载 / SSH 诊断 / frps 启停 / 下发配置 / 安装 frps 七类输出共用一个 ref，显示在「Agent 生命周期」卡底部 | 输出归属不明；执行「重启 frps」后输出出现在 SSH 卡里，语义错位 |
| 5 | **日志与输出分居两卡** | 「frps 最近日志」独立成卡，`agentOutput` 在另一张卡 | 排障时要在两处来回看 |
| 6 | **危险操作无二次确认** | 「卸载（含数据）」按钮点击即执行，`rm -rf /var/lib/fonu-agent` 不可逆 | 误点即丢路由与证书 |
| 7 | **页面级提示缺失** | agent 未连接、frps 未安装等状态只在卡片里体现，没有 `n-alert` 引导（技能要求：页面级提示放 PageHeader 之后） | 新用户不知道下一步该去哪一页 |
| 8 | **跳转卡占位浪费** | 「公网 Nginx 反代」整卡只放一句说明 + 一个跳转按钮 | 占掉半行网格，信息密度低 |
| 9 | **轮询与用户操作耦合** | 10 秒 `refreshAll()` 里设置 `agentActionId`，与用户点击共用同一状态 | 轮询时按钮闪烁 loading |
| 10 | **默认版本硬编码** | `frpsInstallVersion.value?.trim() \|\| '0.71.0'`，与「官方版本列表」加载结果脱节 | 列表加载成功后默认值仍可能是写死的旧版本 |
| 11 | **空态用字符串** | 日志空时渲染 `'暂无日志'` 文本 | 与标准页的 `EmptyState` 不一致 |
| 12 | **窄屏断点不全** | 只有 `@media (max-width: 900px)`，缺 760px 档（技能第 7 节） | 手机宽度下卡片内按钮换行拥挤 |

---

## 二、目标结构

```text
PageHeader（公网 Agent）
  └─ #actions：服务端选择器 · 刷新
n-alert（条件渲染：Agent 未连接 / frps 未安装 / Nginx 缺失 → 一句引导 + 跳转）
stats-row（统一 4 列稳定网格）
  ├─ Agent 连接（在线/未连接 + agent 地址）
  ├─ frps 服务（运行中/未运行 + 版本）
  ├─ 80 端口
  └─ 443 端口
FonuCard「服务状态详情」
  └─ field-grid：Agent 版本 / 已安装 frps 版本 / frps 启动命令 / 配置文件 / 安装位置 / 开机自启 / 7000 端口（StatusBadge）
FonuCard「frps 服务控制」
  └─ 按钮区：启动|停止 · 重启 · frps 管理面板 · 下发配置 · [版本选择] 安装/升级
FonuCard「Agent 生命周期（SSH）」
  └─ 按钮区：探测环境 · 安装/升级 Agent · SSH 诊断 · 卸载（保留数据）· 卸载（含数据，二次确认）
FonuCard「输出与日志」（n-tabs）
  ├─ Tab 操作输出：带来源标签 + 时间；空态 EmptyState；「清空」按钮
  └─ Tab frps 日志：frps_log_tail；空态 EmptyState
```

要点：**状态只读 → 操作只写 → 输出只展示**，三段物理分离；「公网反代」跳转改为 `n-alert` 或 PageHeader 次级入口。

---

## 三、逐项改造清单

### 1. 状态详情独立成卡（解决 #1）

- 把「frps 服务控制」里的 `field-grid` 整体搬到新的「服务状态详情」卡；
- 状态类字段用 `StatusBadge`（开机自启、7000 端口、frps 版本），值类字段用 `.mono` / `.mono-wrap`；
- 「frps 服务控制」只留按钮区与安装版本选择。

### 2. 统一网格（解决 #2、#12）

```css
.stats-row { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-4); }
.agent-page-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-4); }
@media (max-width: 900px) { .stats-row { grid-template-columns: repeat(2, minmax(0, 1fr)); } .agent-page-grid { grid-template-columns: 1fr; } }
@media (max-width: 760px) { .stats-row { grid-template-columns: 1fr; } .agent-actions { width: 100%; } .agent-actions .n-button { flex: 1 1 auto; } }
```

### 3. 操作状态精细化（解决 #3、#9）

```ts
// 当前进行中的动作：'status' | 'probe' | 'agent-install' | 'agent-uninstall' | 'diagnose'
//              | 'frps-start' | 'frps-stop' | 'frps-restart' | 'frps-install' | 'push-config'
const busy = ref<string | null>(null)
const statusLoading = ref(false)

async function run<T>(action: string, fn: () => Promise<T>): Promise<T | undefined> {
  if (busy.value) return            // 串行化，避免并发操作
  busy.value = action
  try { return await fn() } finally { busy.value = null }
}

// 轮询走静默：不设 busy，只设 statusLoading
async function refreshAll(options: { silent?: boolean } = { silent: true }) { /* ... */ }
```

按钮写法：`:loading="busy === 'frps-restart'"`、`:disabled="busy !== null && busy !== 'frps-restart'"`。

### 4. 输出与日志合并 + 来源标注（解决 #4、#5、#11）

```ts
interface OpOutput { at: string; source: string; text: string; ok: boolean }
const opOutput = ref<OpOutput[]>([])      // 最多保留 20 条

function pushOutput(source: string, text: string, ok = true) {
  opOutput.value = [{ at: new Date().toISOString(), source, text, ok }, ...opOutput.value].slice(0, 20)
}
```

- 每类操作写入自己的 `source`（如「重启 frps」「SSH 诊断」「安装 Agent」），失败用 `ok=false` 标红；
- 展示：`n-tabs`（操作输出 / frps 日志），操作输出项显示 `formatDate(at)` 或 `formatRelativeTime(at)` + 来源徽标 + `.code-block`；
- 空态用 `EmptyState`，不再渲染「暂无日志」字符串；
- 提供「清空」按钮（`opOutput.value = []`）。

### 5. 危险操作二次确认（解决 #6）

```vue
<n-popconfirm @positive-click="uninstallAgent(server, false)">
  <template #trigger><n-button size="small" type="warning">卸载（含数据）</n-button></template>
  将删除 VPS 上的 agent、frps 单元与 /var/lib/fonu-agent（路由与证书一并丢失），确认继续？
</n-popconfirm>
```

「卸载（保留路由/证书）」保持一步执行（`--keep-data`），仅加 `quaternary` 视觉区分。

### 6. 页面级提示（解决 #7、#8）

在 `PageHeader` 之后按条件渲染 `n-alert`（互斥，取最高优先级一条）：

| 条件 | 类型 | 文案要点 | 动作 |
| --- | --- | --- | --- |
| `!agentStatusOk` | `error` | Agent 未连接：检查 VPS 上 fonu-agent 是否运行、Agent 地址与 Token 是否正确 | 「探测环境」 |
| `!agentStatus?.frps_active` | `warning` | frps 未运行：可点「启动 frps」或在下方安装 | 「启动 frps」 |
| 反代入口引导（常驻） | `info` | 反代的逐条部署/配置查看/访问控制在「公网反代」页 | 跳转 `/public-proxy` |

原「公网 Nginx 反代」整卡随之删除。

### 7. 版本默认值取列表（解决 #10）

```ts
const frpsVersionOptions = computed(() => releases.value.map((r) => ({ label: r.version, value: r.version })))
watch(releases, (list) => { if (!frpsInstallVersion.value && list.length) frpsInstallVersion.value = list[0].version })
```

列表为空时禁用「安装/升级 frps」并提示「正在获取官方版本列表…」，不再回退硬编码版本。

### 8. 轮询策略

- 保持 10 秒刷新，但 `refreshAll({ silent: true })` 不触碰 `busy`；
- 页面不可见时暂停：`document.visibilityState === 'hidden'` 时跳过（减少无谓请求）；
- `onBeforeUnmount` 清理定时器（现有行为保留）。

---

## 四、涉及文件与验证

| 文件 | 改动 |
| --- | --- |
| `web/src/views/AgentView.vue` | 模板结构重排、脚本状态重构、样式网格与断点补齐（单文件改动） |
| `web/src/api/client.ts` / `types.ts` | 预期**无需**改动（现有 `getFrpAgentStatus` / `probeFrpAgent` / `installFrpAgent` / `uninstallFrpAgent` / `sshDiagnoseFrpAgent` / `frpsServerActionRaw` / `pushFrpConfigToAgent` / `installFrpServer` / `listFrpReleases` 已够用） |
| `cmd/fonu/web/dist` | 构建产物同步（gitignore，不入库） |

验证闭环（技能第 8 节）：

```powershell
cd web
npx vue-tsc --noEmit      # 0 错误
npx vite build            # 成功
# 本地生效：同步 dist → cmd/fonu/web/dist 后 go build ./...
```

回归检查点（真机，VPS 38.95.74.87）：

1. 页面加载：状态卡 4 张、无 loading 闪烁（轮询静默）；
2. 点「重启 frps」：**仅该按钮**转圈，其余按钮 disabled；输出出现在「操作输出」且来源标为「重启 frps」；
3. 点「SSH 诊断」：输出进入同一区域但来源不同，时间正确；
4. 点「卸载（含数据）」：出现二次确认；取消后不执行；
5. agent 未连接场景：顶部出现 `error` 级 `n-alert`，点「探测环境」有反馈；
6. 窄屏（≤760px）：统计卡单列、按钮区铺满不溢出；
7. 暗色主题：所有颜色随令牌切换，无残留写死色值。

---

## 五、风险与回滚

| 风险 | 处理 |
| --- | --- |
| 结构调整导致既有操作失效 | 只重排模板与状态，**不改 api 调用**；逐项对照本方案第三节，改完按第四节回归 |
| 危险操作加确认后影响老用户肌肉记忆 | 仅「卸载（含数据）」加确认，其余保持一步 |
| 输出改为列表后信息过多 | 上限 20 条 + 清空按钮 |
| 回滚 | 单文件改动，`git checkout -- web/src/views/AgentView.vue` 即可回到当前版本（当前版本已按 skill 对齐，可安全回退） |

---

## 六、工作量

| 步骤 | 内容 | 预计 |
| --- | --- | --- |
| 1 | 状态详情卡拆分 + 网格统一 + 断点 | 0.5h |
| 2 | `busy` 状态机 + 按钮 loading 精细化 | 0.5h |
| 3 | 输出与日志合并（tabs + 来源 + 空态） | 1h |
| 4 | 页面级 `n-alert` + 删跳转卡 | 0.5h |
| 5 | 危险操作确认 + 版本默认值 + 轮询静默 | 0.5h |
| 6 | 验证（tsc/build/回归清单） | 0.5h |

合计约 **3–3.5 小时**，单文件改动，不涉及后端与 agent。
