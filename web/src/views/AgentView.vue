<template>
  <PageHeader title="公网 Agent" description="管理部署在公网 VPS 上的 fonu-agent 与 frps 服务">
    <template #actions>
      <n-select
        v-model:value="selectedId"
        :options="serverOptions"
        placeholder="选择公网服务端"
        style="width: 220px"
        :disabled="servers.length === 0"
      />
    </template>
  </PageHeader>

  <LoadError v-if="loadError" :message="loadError" @retry="load" />

  <template v-else>
    <EmptyState
      v-if="servers.length === 0"
      title="尚未配置 FRP 服务端"
      description="先在「内网穿透」页添加公网服务端并保存 SSH 信息，再回到本页管理 Agent。"
    >
      <template #action>
        <router-link to="/frp"><n-button type="primary">去内网穿透页</n-button></router-link>
      </template>
    </EmptyState>

    <template v-else-if="server">
      <section class="status-grid">
        <FonuCard class="status-card">
          <div class="status-card__icon" :class="agentStatusOk ? 'status-card__icon--running' : 'status-card__icon--stopped'">
            <n-icon :component="CloudOutline" />
          </div>
          <div class="status-card__body">
            <div class="status-card__label">Agent 连接</div>
            <div class="status-card__value">{{ agentStatusOk ? '在线' : '未连接' }}</div>
            <div class="status-card__meta mono">{{ server.agent_url || '未配置 Agent 地址' }}</div>
          </div>
        </FonuCard>
        <FonuCard class="status-card">
          <div class="status-card__icon" :class="agentStatus?.frps_active ? 'status-card__icon--running' : 'status-card__icon--stopped'">
            <n-icon :component="GitNetworkOutline" />
          </div>
          <div class="status-card__body">
            <div class="status-card__label">frps 服务</div>
            <div class="status-card__value">{{ agentStatus?.frps_active ? '运行中' : '未运行' }}</div>
            <div class="status-card__meta">{{ agentStatus?.frps_version?.trim() || '未安装' }}</div>
          </div>
        </FonuCard>
        <FonuCard class="status-card">
          <div class="status-card__icon" :class="agentStatus?.ports?.['80'] ? 'status-card__icon--running' : 'status-card__icon--stopped'">
            <n-icon :component="SwapHorizontalOutline" />
          </div>
          <div class="status-card__body">
            <div class="status-card__label">80 端口</div>
            <div class="status-card__value">{{ agentStatus?.ports?.['80'] ? '监听中' : '未监听' }}</div>
            <div class="status-card__meta">无端口访问的入口</div>
          </div>
        </FonuCard>
        <FonuCard class="status-card">
          <div class="status-card__icon" :class="agentStatus?.ports?.['443'] ? 'status-card__icon--running' : 'status-card__icon--stopped'">
            <n-icon :component="LockClosedOutline" />
          </div>
          <div class="status-card__body">
            <div class="status-card__label">443 端口</div>
            <div class="status-card__value">{{ agentStatus?.ports?.['443'] ? '监听中' : '未监听' }}</div>
            <div class="status-card__meta">HTTPS 入口</div>
          </div>
        </FonuCard>
      </section>

      <section class="agent-page-grid">
        <FonuCard class="section-card" title="frps 服务控制">
          <template #header>
            <n-button size="small" quaternary :loading="agentActionId === server.id" @click="refreshAll">刷新</n-button>
          </template>
          <div class="agent-section-body">
            <div class="field-grid">
              <div><label>Agent 版本</label><span class="field-tags"><n-tag size="small" :bordered="false">{{ agentStatus?.agent_version || '-' }}</n-tag></span></div>
              <div><label>frps 启动命令</label><span class="mono mono-wrap">{{ agentStatus?.frps_unit_exec || '-' }}</span></div>
              <div><label>frps 配置</label><span class="mono" :title="agentStatus?.frps_config_path">{{ agentStatus?.frps_config_path || '-' }}</span></div>
              <div><label>开机自启</label><span class="field-tags"><n-tag size="small" :bordered="false" :type="agentStatus?.frps_enabled ? 'success' : 'warning'">{{ agentStatus?.frps_enabled ? '已启用' : '未启用' }}</n-tag></span></div>
              <div><label>7000 端口</label><span class="field-tags"><n-tag size="small" :bordered="false" :type="agentStatus?.ports?.['7000'] ? 'success' : 'warning'">{{ agentStatus?.ports?.['7000'] ? '监听中' : '未监听' }}</n-tag></span></div>
            </div>
            <div class="agent-actions">
              <n-button v-if="agentStatus?.frps_active" size="small" type="warning" :loading="agentActionId === server.id" @click="frpsAction(server, 'stop')">停止 frps</n-button>
              <n-button v-else size="small" type="success" :loading="agentActionId === server.id" @click="frpsAction(server, 'start')">启动 frps</n-button>
              <n-button size="small" :loading="agentActionId === server.id" @click="frpsAction(server, 'restart')">重启 frps</n-button>
              <n-button v-if="dashboardURL" size="small" quaternary @click="openDashboard(dashboardURL)">frps 管理</n-button>
            </div>
            <p v-if="!dashboardURL" class="form-hint" style="margin-top: 8px">未配置 frps 管理接口：在「内网穿透」页服务端表单的「frps 管理接口」填写地址与端口后保存即可开启面板入口</p>
          </div>
        </FonuCard>

        <FonuCard class="section-card" title="frps 配置与版本">
          <div class="agent-section-body">
            <div class="field-grid">
              <div><label>已安装 frps 版本</label><span class="field-tags"><n-tag size="small" :bordered="false" :type="agentStatus?.frps_version ? 'success' : 'warning'">{{ installedFrpsVersion || '未安装' }}</n-tag></span></div>
              <div><label>安装位置</label><span class="mono" :title="agentStatus?.frps_binary">{{ agentStatus?.frps_binary || '-' }}</span></div>
            </div>
            <div class="agent-actions" style="margin-top: var(--fonu-space-3)">
              <n-button size="small" :loading="agentActionId === server.id" @click="pushAgentConfig(server)">下发 frps 配置</n-button>
            </div>
            <div class="frps-install-row">
              <n-select v-model:value="frpsInstallVersion" :options="frpsVersionOptions" placeholder="选择 frps 官方版本" style="width: 200px" filterable />
              <n-button size="small" type="success" :loading="agentActionId === server.id" @click="installFrps(server)">安装/升级 frps</n-button>
            </div>
            <p class="form-hint">VPS 无 frps 二进制时使用：自动下载并注册 frps.service，完成后自动下发同源配置。</p>
          </div>
        </FonuCard>
      </section>

      <section class="agent-page-grid">
        <FonuCard class="section-card" title="公网 Nginx 反代">
          <div class="agent-section-body">
            <p class="form-hint">反代管理已独立成「公网反代」页面：逐条部署/移除、配置查看、强制跳转、IP 白名单、Basic Auth、WebSocket 等都在那里。</p>
            <router-link to="/public-proxy"><n-button size="small" type="primary">前往公网反代</n-button></router-link>
          </div>
        </FonuCard>

        <FonuCard class="section-card" title="frps 最近日志">
          <div class="agent-section-body">
            <pre class="code-block">{{ agentStatus?.frps_log_tail || '暂无日志' }}</pre>
          </div>
        </FonuCard>
      </section>

      <FonuCard class="section-card" title="Agent 生命周期（SSH）">
        <template #header>
          <div class="section-actions">
            <n-button size="small" type="primary" :loading="agentActionId === server.id" @click="probeAgent(server)">探测环境</n-button>
            <n-button size="small" type="success" :loading="agentActionId === server.id" @click="installAgent(server)">{{ server.agent_configured ? '升级 Agent' : '安装 Agent' }}</n-button>
            <n-button size="small" type="warning" :loading="agentActionId === server.id" @click="uninstallAgent(server, false)">卸载（含数据）</n-button>
            <n-button size="small" quaternary :loading="agentActionId === server.id" @click="uninstallAgent(server, true)">卸载（保留路由/证书）</n-button>
            <n-button size="small" quaternary :loading="agentActionId === server.id" @click="diagnoseAgent(server)">SSH 诊断</n-button>
          </div>
        </template>
        <div class="agent-section-body">
          <p class="form-hint">
            首次安装、升级、卸载需要 SSH（在「内网穿透」页服务端表单的「公网 Agent 管理」区填写）；安装成功后自动回填 Agent 地址与 Token。
            日常管理走 Agent API，无需 SSH。SSH 信息仅用于生命周期操作，凭据加密存储、不回显。
          </p>
          <pre v-if="agentOutput" class="code-block">{{ agentOutput }}</pre>
        </div>
      </FonuCard>
    </template>
  </template>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { CloudOutline, GitNetworkOutline, LockClosedOutline, SwapHorizontalOutline } from '@vicons/ionicons5'
import EmptyState from '../components/EmptyState.vue'
import FonuCard from '../components/FonuCard.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import { api } from '../api/client'
import type { FrpAgentStatus, FrpRelease, FrpServer } from '../api/types'

const message = useMessage()
const servers = ref<FrpServer[]>([])
const selectedId = ref<number | null>(null)
const loadError = ref('')
const agentStatus = ref<FrpAgentStatus | null>(null)
const agentStatusOk = ref(false)
const agentActionId = ref<number | null>(null)
const agentOutput = ref('')
const frpsInstallVersion = ref<string | null>(null)
const releases = ref<FrpRelease[]>([])
let timer: number | undefined

const server = computed(() => servers.value.find((item) => item.id === selectedId.value) ?? null)
const serverOptions = computed(() => servers.value
  .filter((item) => item.agent_configured || item.ssh_configured)
  .map((item) => ({ label: item.name, value: item.id })))

const installedFrpsVersion = computed(() => (agentStatus.value?.frps_version || '').trim() || '')
const frpsVersionOptions = computed(() => [
  ...(releases.value.length ? [] : [{ label: '0.71.0（默认，官方版本未加载）', value: '0.71.0' }]),
  ...releases.value.map((release) => ({ label: release.version, value: release.version })),
])

async function load() {
  loadError.value = ''
  try {
    servers.value = (await api.listFrpServers()).filter((item) => item.agent_configured || item.ssh_configured)
    if (selectedId.value === null || !servers.value.some((item) => item.id === selectedId.value)) {
      selectedId.value = servers.value[0]?.id ?? null
    }
    if (server.value) void refreshAll()
  } catch (error) { loadError.value = error instanceof Error ? error.message : '读取服务端失败' }
}


async function refreshAll() {
  const current = server.value
  if (!current) return
  agentActionId.value = current.id
  try {
    agentStatus.value = await api.getFrpAgentStatus(current.id)
    agentStatusOk.value = true
  } catch (error) {
    agentStatusOk.value = false
    agentOutput.value = error instanceof Error ? error.message : 'Agent 状态获取失败'
  } finally { agentActionId.value = null }
}

async function probeAgent(target: FrpServer) {
  agentActionId.value = target.id
  try {
    const result = await api.probeFrpAgent(target.id)
    agentOutput.value = Object.entries(result).map(([key, value]) => `${key}: ${value}`).join('\n')
  } catch (error) { agentOutput.value = error instanceof Error ? error.message : '探测失败' }
  finally { agentActionId.value = null }
}

async function installAgent(target: FrpServer) {
  agentActionId.value = target.id
  try {
    await api.installFrpAgent(target.id)
    agentOutput.value = 'Agent 安装完成，已自动回填 Agent 地址与 Token'
    message.success('Agent 安装成功')
    await load()
  } catch (error) { agentOutput.value = error instanceof Error ? error.message : '安装失败' }
  finally { agentActionId.value = null }
}

async function uninstallAgent(target: FrpServer, keepData: boolean) {
  agentActionId.value = target.id
  try {
    const result = await api.uninstallFrpAgent(target.id, keepData)
    agentOutput.value = result.output || '已卸载'
    message.success('Agent 已卸载')
    await load()
  } catch (error) { agentOutput.value = error instanceof Error ? error.message : '卸载失败' }
  finally { agentActionId.value = null }
}

async function diagnoseAgent(target: FrpServer) {
  agentActionId.value = target.id
  try {
    const result = await api.sshDiagnoseFrpAgent(target.id)
    agentOutput.value = result.output || '无输出'
  } catch (error) { agentOutput.value = error instanceof Error ? error.message : '诊断失败' }
  finally { agentActionId.value = null }
}

// frps 管理面板地址：直接读服务端表单里已配置的「frps 管理接口」（dashboard_addr/port）；
// 监听 0.0.0.0/:: 时用服务端公网地址拼 URL（浏览器在外网访问）
const dashboardURL = computed(() => {
  const current = server.value
  if (!current) return ''
  const addr = (current.options.dashboard_addr || '').trim()
  const port = current.options.dashboard_port || 0
  if (!addr || !port) return ''
  const host = addr === '0.0.0.0' || addr === '::' ? (current.server_addr || '127.0.0.1') : addr
  return `http://${host}:${port}`
})

function openDashboard(url: string) {
  if (url) window.open(url, '_blank', 'noopener')
}

async function frpsAction(target: FrpServer, action: 'start' | 'stop' | 'restart') {
  agentActionId.value = target.id
  const actionText = action === 'stop' ? '停止' : action === 'start' ? '启动' : '重启'
  try {
    // agent 对"命令成功但服务未进入运行态"返回 200+ok:false+error（含 journal 日志）
    const result = await api.frpsServerActionRaw(target.id, action)
    if (result && result.ok === false) {
      agentOutput.value = result.error || `frps ${actionText}后未进入预期状态`
      message.error(`frps ${actionText}失败，详见输出`)
    } else {
      message.success(`frps 已${actionText}`)
    }
    await refreshAll()
  } catch (error) {
    agentOutput.value = error instanceof Error ? error.message : 'frps 操作失败'
    message.error(`frps ${actionText}失败，详见输出`)
  } finally { agentActionId.value = null }
}

async function pushAgentConfig(target: FrpServer) {
  agentActionId.value = target.id
  try {
    await api.pushFrpConfigToAgent(target.id)
    message.success('frps 配置已下发')
    agentOutput.value = 'frps 同源配置已下发到 VPS，点「重启 frps」后生效'
  } catch (error) { agentOutput.value = error instanceof Error ? error.message : '下发配置失败' }
  finally { agentActionId.value = null }
}

async function installFrps(target: FrpServer) {
  agentActionId.value = target.id
  try {
    const version = frpsInstallVersion.value?.trim() || '0.71.0'
    const result = await api.installFrpServer(target.id, version, '')
    const notes = Array.isArray(result?.notes) ? result.notes.join('\n') : ''
    agentOutput.value = `frps ${result?.version ?? version} 安装完成\n${notes}`
    message.success('VPS frps 已安装')
    await refreshAll()
  } catch (error) { agentOutput.value = error instanceof Error ? error.message : '安装 frps 失败' }
  finally { agentActionId.value = null }
}

async function loadReleases() {
  if (releases.value.length > 0) return
  try { releases.value = await api.listFrpReleases() } catch { /* 回退默认版本 */ }
}

onMounted(() => {
  void load()
  void loadReleases()
  timer = window.setInterval(() => { if (server.value) void refreshAll() }, 10000)
})

onBeforeUnmount(() => { if (timer) window.clearInterval(timer) })
</script>

<style scoped>
/* 对齐 DDNS 页风格：surface 卡片、settings-grid 字段、统一代码块 */
.status-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(210px, 1fr)); gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-4); }
.status-card { display: flex; gap: var(--fonu-space-3); align-items: center; padding: var(--fonu-space-4) var(--fonu-space-5); }
.status-card__icon { width: 44px; height: 44px; border-radius: var(--fonu-radius-sm); display: flex; align-items: center; justify-content: center; font-size: 22px; flex-shrink: 0; }
.status-card__icon--running { background: rgba(34, 197, 94, 0.14); color: #16a34a; }
.status-card__icon--stopped { background: rgba(148, 163, 184, 0.16); color: #64748b; }
.status-card__body { min-width: 0; }
.status-card__label { color: var(--fonu-text-secondary); font-size: 12px; }
.status-card__value { font-size: 18px; font-weight: 700; color: var(--fonu-text); }
.status-card__meta { color: var(--fonu-text-muted); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.agent-page-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(360px, 1fr)); gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-4); }
.agent-section-body { padding: var(--fonu-space-2) var(--fonu-space-5) var(--fonu-space-4); }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--fonu-space-4); }
.field-grid > div { display: grid; gap: 4px; min-width: 0; }
.field-grid label { color: var(--fonu-text-secondary); font-size: 12px; }
.field-grid .mono { font-family: var(--fonu-mono); font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.field-grid .mono-wrap { white-space: normal; word-break: break-all; overflow: visible; text-overflow: unset; }
.field-tags { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.agent-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: var(--fonu-space-4); }
.route-list { display: grid; gap: 8px; margin-top: var(--fonu-space-3); }
.route-item { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 12px; border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm); background: var(--fonu-surface); }
.route-item__main { display: grid; gap: 3px; min-width: 0; }
.route-item__domains { color: var(--fonu-text); font-size: 12px; overflow-wrap: anywhere; }
.route-item__upstream { color: var(--fonu-text-muted); font-size: 12px; }
.route-item__actions { display: flex; align-items: center; justify-content: flex-end; gap: 6px; flex-wrap: wrap; flex-shrink: 0; }
.empty-route { margin-top: var(--fonu-space-3); padding: 18px; border: 1px dashed var(--fonu-border); border-radius: var(--fonu-radius-sm); color: var(--fonu-text-muted); text-align: center; }
.frps-install-row { display: flex; align-items: center; gap: 10px; margin-top: var(--fonu-space-4); flex-wrap: wrap; }
.frps-install-row .form-hint { flex-basis: 100%; margin: 0; }
.code-block {
  margin: var(--fonu-space-3) 0 0;
  padding: var(--fonu-space-3) var(--fonu-space-4);
  background: var(--fonu-bg-muted);
  border: 1px solid var(--fonu-border);
  border-radius: var(--fonu-radius-sm);
  font-family: var(--fonu-mono);
  font-size: 12.5px;
  line-height: 1.65;
  color: var(--fonu-text);
  overflow: auto;
  max-height: 240px;
  white-space: pre-wrap;
  word-break: break-all;
}
.form-hint { color: var(--fonu-text-secondary); font-size: 12.5px; }
.mono { font-family: var(--fonu-mono); }
.section-actions { display: flex; flex-wrap: wrap; gap: 8px; }
@media (max-width: 900px) { .field-grid { grid-template-columns: 1fr; } }
</style>
