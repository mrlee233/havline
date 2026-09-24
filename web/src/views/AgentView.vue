<template>
  <PageHeader title="公网 Agent" :description="headerDescription">
    <template #actions>
      <n-select
        v-model:value="selectedId"
        :options="serverOptions"
        placeholder="选择公网服务端"
        class="agent-select"
        :disabled="servers.length === 0"
      />
      <n-button :loading="statusLoading" :disabled="!server" @click="refreshAll({ silent: false })">
        <template #icon><n-icon :component="RefreshOutline" /></template>
        刷新
      </n-button>
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
      <!-- 状态提示：互斥，最多一条 -->
      <n-alert v-if="statusAlert" :type="statusAlert.type" :bordered="false" class="page-alert">
        <div class="page-alert__body">
          <span>{{ statusAlert.text }}</span>
          <n-button v-if="statusAlert.action" size="small" :disabled="busy !== null" @click="statusAlert.action.run()">
            {{ statusAlert.action.label }}
          </n-button>
        </div>
      </n-alert>

      <!-- 入口行：公网反代（不占 alert 位） -->
      <div class="entrance-row">
        <span class="entrance-row__text">
          已注册公网反代 <strong>{{ routesCount }}</strong> 条；逐条部署、配置查看与访问控制都在「公网反代」页
        </span>
        <router-link to="/public-proxy"><n-button size="small" type="primary">前往公网反代</n-button></router-link>
      </div>

      <!-- ① 运行状态 -->
      <section class="stats-row" aria-label="运行状态">
        <div class="stat-card">
          <n-icon :component="CloudOutline" class="stat-card__icon" :class="stateClass(agentStatusOk, statusLoaded)" aria-hidden="true" />
          <div class="stat-card__value stat-card__value--sm">{{ agentConnectionText }}</div>
          <div class="stat-card__label">Agent 连接</div>
          <div class="stat-card__sub mono" :title="server.agent_url">{{ server.agent_url || '未配置 Agent 地址' }}</div>
        </div>
        <div class="stat-card">
          <n-icon :component="GitNetworkOutline" class="stat-card__icon" :class="stateClass(!!agentStatus?.frps_active, statusLoaded)" aria-hidden="true" />
          <div class="stat-card__value stat-card__value--sm">{{ frpsStateText }}</div>
          <div class="stat-card__label">frps 服务</div>
          <div class="stat-card__sub">{{ installedFrpsVersion || '未安装' }}</div>
        </div>
        <div class="stat-card">
          <n-icon :component="LayersOutline" class="stat-card__icon" :class="routesCount > 0 ? 'stat-card__icon--blue' : 'stat-card__icon--amber'" aria-hidden="true" />
          <div class="stat-card__value stat-card__value--sm">{{ routesCount }} 条</div>
          <div class="stat-card__label">反代路由</div>
          <div class="stat-card__sub">VPS 上已注册</div>
        </div>
        <div class="stat-card">
          <n-icon :component="SwapHorizontalOutline" class="stat-card__icon" :class="portIconClass" aria-hidden="true" />
          <div class="port-list">
            <span
              v-for="port in watchedPorts"
              :key="port"
              class="port-item"
              :class="{ 'port-item--on': portOn(port) }"
              :title="portMeaning(port)"
            >
              {{ port }} {{ portOn(port) ? '✓' : '✗' }}
            </span>
          </div>
          <div class="stat-card__label">端口监听</div>
          <div class="stat-card__sub">frps 运行所需端口</div>
        </div>
        <FonuCard class="section-card vps-resource-card" title="VPS 资源">
          <div class="section-body">
            <p v-if="metricsError" class="form-hint">{{ metricsError }}</p>
            <div v-else-if="agentMetrics" class="res-strip">
              <div v-for="row in metricsRows" :key="row.label" class="res-row">
                <span class="res-row__label">{{ row.label }}</span>
                <span class="res-row__value">{{ row.value }}</span>
                <span class="res-row__detail">{{ row.detail }}</span>
                <span class="res-bar" :class="`res-bar--${row.tone}`">
                  <span class="res-bar__fill" :style="{ width: `${row.percent.toFixed(1)}%` }" />
                </span>
              </div>
              <div class="res-meta">
                <span>平均负载 {{ metricsLoadText }}</span>
                <span>运行时长 {{ metricsUptimeText }}</span>
              </div>
            </div>
            <p v-else class="form-hint">{{ metricsLoading ? '正在采集 VPS 资源…' : '暂无 VPS 资源数据' }}</p>
          </div>
        </FonuCard>
      </section>

      <!-- ② 环境与依赖 -->
      <section class="dep-grid" aria-label="环境与依赖">
        <FonuCard class="section-card" title="Fonu Agent">
          <div class="section-body">
            <div class="field-grid">
              <div><label>Agent 版本</label><span class="mono">{{ agentStatus?.agent_version || '—' }}</span></div>
              <div><label>凭据状态</label><span class="field-tags">
                <StatusBadge :kind="server.agent_configured ? 'success' : 'warning'" :text="server.agent_configured ? '已配置' : '未配置'" />
              </span></div>
              <div><label>SSH 凭据</label><span class="field-tags">
                <StatusBadge :kind="server.ssh_configured ? 'success' : 'disabled'" :text="server.ssh_configured ? '已配置' : '未配置'" />
              </span></div>
              <div><label>Agent 地址</label><span class="mono mono-wrap">{{ server.agent_url || '未配置' }}</span></div>
            </div>
            <div class="agent-actions">
              <n-button size="small" :loading="busy === 'probe'" :disabled="busy !== null && busy !== 'probe'" @click="probe(server)">探测环境</n-button>
            </div>
          </div>
        </FonuCard>

        <FonuCard class="section-card" title="Nginx（公网反代依赖）">
          <div class="section-body">
            <div class="field-grid">
              <div><label>可用性</label><span class="field-tags"><StatusBadge :kind="nginxKind" :text="nginxText" /></span></div>
              <div><label>进程</label><span>{{ agentStatus?.nginx_running ? '运行中' : '未运行' }}</span></div>
              <div><label>版本</label><span class="mono">{{ agentStatus?.nginx_version || '—' }}</span></div>
              <div><label>可执行文件</label><span class="mono mono-wrap">{{ agentStatus?.nginx_path || agentStatus?.nginx_binary || '—' }}</span></div>
              <div class="field-grid__wide"><label>vhost 配置目录</label><span class="mono mono-wrap">{{ agentStatus?.nginx_conf_dir || '—' }}</span></div>
            </div>
            <p v-if="agentStatus?.nginx_error" class="form-hint nginx-error mono">{{ agentStatus.nginx_error }}</p>
            <div class="agent-actions">
              <router-link to="/public-proxy"><n-button size="small" quaternary>去「公网反代」处理</n-button></router-link>
            </div>
          </div>
        </FonuCard>

        <FonuCard class="section-card" title="frps 运行时">
          <div class="section-body">
            <div class="field-grid">
              <div><label>已安装版本</label><span class="field-tags"><StatusBadge :kind="agentStatus?.frps_version ? 'success' : 'warning'" :text="installedFrpsVersion || '未安装'" /></span></div>
              <div><label>开机自启</label><span class="field-tags"><StatusBadge :kind="agentStatus?.frps_enabled ? 'success' : 'warning'" :text="agentStatus?.frps_enabled ? '已启用' : '未启用'" /></span></div>
              <div><label>二进制路径</label><span class="mono mono-wrap">{{ agentStatus?.frps_binary || '—' }}</span></div>
              <div><label>配置文件</label><span class="mono mono-wrap">{{ agentStatus?.frps_config_path || '—' }}</span></div>
              <div class="field-grid__wide"><label>启动命令</label><span class="mono mono-wrap">{{ agentStatus?.frps_unit_exec || '—' }}</span></div>
              <div><label>启动一致性</label><span class="field-tags"><StatusBadge :kind="frpsConsistency.kind" :text="frpsConsistency.text" /></span></div>
            </div>
            <p v-if="!frpsConsistency.ok && frpsConsistency.detail" class="form-hint">{{ frpsConsistency.detail }}</p>
          </div>
        </FonuCard>
      </section>

      <!-- ④ Agent 生命周期（SSH） + ③ frps 控制：两列并排（生命周期在左） -->
      <div class="control-grid">
      <FonuCard class="section-card" title="Agent 生命周期（SSH）">
        <div class="section-body">
          <p class="form-hint">
            首次安装、升级、卸载需要 SSH（在「内网穿透」页服务端表单的「公网 Agent 管理」区填写）；安装成功后自动回填 Agent 地址与 Token。
            日常管理走 Agent API，无需 SSH。SSH 信息仅用于生命周期操作，凭据加密存储、不回显。
          </p>
          <div class="agent-actions">
            <n-button size="small" :loading="busy === 'agent-install'" :disabled="busy !== null && busy !== 'agent-install'" @click="installAgent(server)">{{ server.agent_configured ? '升级 Agent' : '安装 Agent' }}</n-button>
            <n-button size="small" :loading="busy === 'diagnose'" :disabled="busy !== null && busy !== 'diagnose'" @click="diagnoseAgent(server)">SSH 诊断</n-button>
            <n-button size="small" quaternary :loading="busy === 'agent-uninstall'" :disabled="busy !== null && busy !== 'agent-uninstall'" @click="uninstallAgent(server, true)">卸载（保留路由/证书）</n-button>
            <n-popconfirm @positive-click="uninstallAgent(server, false)">
              <template #trigger>
                <n-button size="small" type="warning" :disabled="busy !== null">卸载（含数据）</n-button>
              </template>
              将删除 VPS 上的 fonu-agent、frps 单元与 /var/lib/fonu-agent（已注册的路由与证书一并丢失），确认继续？
            </n-popconfirm>
          </div>
        </div>
      </FonuCard>

      <FonuCard class="section-card" title="frps 控制">
        <div class="section-body">
          <div class="agent-actions">
            <n-button v-if="agentStatus?.frps_active" size="small" type="warning" :loading="busy === 'frps-stop'" :disabled="busy !== null && busy !== 'frps-stop'" @click="frpsAction(server, 'stop')">停止 frps</n-button>
            <n-button v-else size="small" type="success" :loading="busy === 'frps-start'" :disabled="busy !== null && busy !== 'frps-start'" @click="frpsAction(server, 'start')">启动 frps</n-button>
            <n-button size="small" :loading="busy === 'frps-restart'" :disabled="busy !== null && busy !== 'frps-restart'" @click="frpsAction(server, 'restart')">重启 frps</n-button>
            <n-tooltip :disabled="!!dashboardURL">
              <template #trigger>
                <span class="tooltip-trigger">
                  <n-button size="small" quaternary :disabled="!dashboardURL" @click="openDashboard">frps 管理面板</n-button>
                </span>
              </template>
              未配置 frps 管理接口：在「内网穿透」页服务端表单的「frps 管理接口」填写地址与端口后保存
            </n-tooltip>
            <n-button size="small" :loading="busy === 'push-config'" :disabled="busy !== null && busy !== 'push-config'" @click="pushAgentConfig(server)">下发 frps 配置</n-button>
            <n-button size="small" quaternary :loading="configLoading" :disabled="busy !== null" @click="viewConfig(server)">配置查看</n-button>
          </div>
          <div class="frps-install-row">
            <n-select
              v-model:value="frpsInstallVersion"
              :options="frpsVersionOptions"
              placeholder="选择 frps 官方版本"
              class="frps-version-select"
              filterable
              :loading="releasesLoading"
            />
            <n-button size="small" type="success" :loading="busy === 'frps-install'" :disabled="(busy !== null && busy !== 'frps-install') || !releases.length" @click="installFrps(server)">安装 / 升级 frps</n-button>
            <span v-if="!releases.length" class="form-hint">正在获取官方版本列表…（获取失败时可手动输入版本号）</span>
            <span v-else class="form-hint">VPS 无 frps 二进制时使用：自动下载并注册 frps.service，完成后自动下发同源配置</span>
          </div>
        </div>
      </FonuCard>

      </div>

      <!-- ⑤ 输出与日志 -->
      <FonuCard class="section-card" title="输出与日志">
        <template #header>
          <n-button size="small" quaternary :disabled="!opOutput.length" @click="opOutput = []">清空操作输出</n-button>
        </template>
        <div class="section-body">
          <n-tabs type="line" animated>
            <n-tab-pane name="ops" tab="操作输出">
              <div v-if="opOutput.length" class="op-list" aria-live="polite">
                <div v-for="(item, index) in opOutput" :key="index" class="op-item" :class="{ 'op-item--fail': !item.ok }">
                  <div class="op-item__head">
                    <StatusBadge :kind="item.ok ? 'success' : 'error'" :text="item.ok ? '成功' : '失败'" />
                    <span class="op-item__source">{{ item.source }}</span>
                    <span class="op-item__time">{{ formatRelativeTime(item.at) }}</span>
                  </div>
                  <pre class="code-block">{{ item.text }}</pre>
                </div>
              </div>
              <EmptyState v-else title="暂无操作输出" description="执行探测、安装、重启等操作后，这里会按时间倒序显示来源、时间与结果。" />
            </n-tab-pane>
            <n-tab-pane name="logs" tab="frps 日志">
              <pre v-if="agentStatus?.frps_log_tail" class="code-block">{{ agentStatus.frps_log_tail }}</pre>
              <EmptyState v-else title="暂无 frps 日志" description="VPS 上 frps 服务输出为空，或 frps 尚未启动。" />
            </n-tab-pane>
          </n-tabs>
        </div>
      </FonuCard>

      <!-- 配置查看弹窗：展示即将下发的 frps.toml -->
      <n-modal v-model:show="configModal" style="width: 640px">
        <div class="config-modal">
          <div class="config-modal__header">
            <h3 class="modal-title">frps 配置（即将下发的内容）</h3>
            <n-button size="small" quaternary @click="configModal = false">
              <template #icon><n-icon :component="CloseOutline" /></template>
            </n-button>
          </div>
          <div class="config-modal__body">
            <p class="form-hint">
              由 Fonu 本地生成；点「下发 frps 配置」后写入 VPS 的
              <span class="mono">{{ agentStatus?.frps_config_path || '/etc/frp/frps.toml' }}</span>，重启 frps 后生效。
            </p>
            <pre class="code-block config-modal__code">{{ configContent || '# 暂无内容' }}</pre>
          </div>
          <div class="config-modal__footer">
            <n-button size="small" :disabled="!configContent" @click="copyConfig">复制</n-button>
            <n-button size="small" type="primary" @click="configModal = false">关闭</n-button>
          </div>
        </div>
      </n-modal>
    </template>
  </template>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import {
  CloseOutline, CloudOutline, GitNetworkOutline, LayersOutline, RefreshOutline, SwapHorizontalOutline,
} from '@vicons/ionicons5'
import EmptyState from '../components/EmptyState.vue'
import FonuCard from '../components/FonuCard.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { api } from '../api/client'
import type { FrpAgentStatus, FrpRelease, FrpServer } from '../api/types'
import { formatBytes, formatRelativeTime, formatUptime } from '../utils/format'
import { copyText } from '../utils/clipboard'

// 操作动作：按动作粒度控制 loading，保证"只有被点的按钮转圈"
type Action =
  | 'probe' | 'agent-install' | 'agent-uninstall' | 'diagnose'
  | 'frps-start' | 'frps-stop' | 'frps-restart' | 'frps-install' | 'push-config'

interface OpOutput { at: string; source: string; text: string; ok: boolean }

const message = useMessage()
const servers = ref<FrpServer[]>([])
const selectedId = ref<number | null>(null)
const loadError = ref('')
const agentStatus = ref<FrpAgentStatus | null>(null)
const agentStatusOk = ref(false)
const statusLoaded = ref(false)
const statusLoading = ref(false)
const busy = ref<Action | null>(null)
const opOutput = ref<OpOutput[]>([])
const frpsInstallVersion = ref<string | null>(null)
const releases = ref<FrpRelease[]>([])
const releasesLoading = ref(false)
const configModal = ref(false)
const configContent = ref('')
const configLoading = ref(false)
let timer: number | undefined

const watchedPorts = ['80', '443', '7000', '8080'] as const

// 端口含义（P0-1）：让端口矩阵直接可读，不用再去查哪个端口是谁的
const portMeanings: Record<string, string> = {
  '80': '公网 HTTP 入口（Nginx）',
  '443': '公网 HTTPS 入口（Nginx）',
  '7000': 'frps 控制端口（frpc 连接用）',
  '8080': 'frps vhost HTTP 端口（Nginx 回源到 frps）',
}

function portMeaning(port: string) {
  return `${portMeanings[port] ?? '端口'}：${portOn(port) ? '正在监听' : '未监听'}`
}

// frps 运行一致性（P0-3）：systemd 实际执行的启动命令，是否用的是 Fonu 下发的二进制与配置文件。
// 不一致意味着：手工改过 unit，或者 Fonu 写入的 frps.toml 并不是 frps 实际加载的那一份。
const frpsConsistency = computed(() => {
  const exec = agentStatus.value?.frps_unit_exec ?? ''
  if (!exec) return { ok: false, kind: 'warning' as const, text: '未获取', detail: '' }
  const binary = agentStatus.value?.frps_binary ?? ''
  const config = agentStatus.value?.frps_config_path ?? ''
  const problems: string[] = []
  if (binary && !exec.includes(binary)) problems.push(`启动命令未使用配置的二进制（${binary}）`)
  if (config && !exec.includes(config)) problems.push(`启动命令未使用 Fonu 下发的配置文件（${config}）`)
  if (problems.length > 0) {
    return { ok: false, kind: 'warning' as const, text: '不一致', detail: problems.join('；') }
  }
  return { ok: true, kind: 'success' as const, text: '一致', detail: '启动命令与配置的二进制、配置文件一致' }
})

const server = computed(() => servers.value.find((item) => item.id === selectedId.value) ?? null)
const serverOptions = computed(() => servers.value
  .filter((item) => item.agent_configured || item.ssh_configured)
  .map((item) => ({ label: item.name, value: item.id })))
const installedFrpsVersion = computed(() => (agentStatus.value?.frps_version || '').trim())
const routesCount = computed(() => agentStatus.value?.routes?.length ?? 0)
const frpsVersionOptions = computed(() => releases.value.map((release) => ({ label: release.version, value: release.version })))

// O：最后检查时间（PageHeader 副标题，判断数据新鲜度）
const headerDescription = computed(() => {
  if (!server.value) return '管理部署在公网 VPS 上的 fonu-agent 与 frps 服务'
  if (!statusLoaded.value) return '正在读取 VPS 状态…'
  const checkedAt = agentStatus.value?.checked_at
  return checkedAt ? `管理部署在公网 VPS 上的 fonu-agent 与 frps · 检查于 ${formatRelativeTime(checkedAt)}` : '管理部署在公网 VPS 上的 fonu-agent 与 frps 服务'
})

// 三态：未加载 / 加载中 / 失败——避免把"没拉到"显示成"未连接"
const agentConnectionText = computed(() => {
  if (!statusLoaded.value) return statusLoading.value ? '读取中…' : '—'
  return agentStatusOk.value ? '在线' : '未连接'
})
const frpsStateText = computed(() => {
  if (!statusLoaded.value) return '—'
  return agentStatus.value?.frps_active ? '运行中' : '未运行'
})
const portIconClass = computed(() => {
  if (!statusLoaded.value) return 'stat-card__icon--blue'
  return watchedPorts.some((port) => portOn(port)) ? 'stat-card__icon--green' : 'stat-card__icon--amber'
})
const nginxKind = computed<'success' | 'warning' | 'error' | 'disabled'>(() => {
  if (!statusLoaded.value) return 'disabled'
  if (agentStatus.value?.nginx_missing) return 'error'
  if (!agentStatus.value?.nginx_ok) return 'error'
  return agentStatus.value?.nginx_running === false ? 'warning' : 'success'
})
const nginxText = computed(() => {
  if (!statusLoaded.value) return '未读取'
  if (agentStatus.value?.nginx_missing) return '未安装'
  if (!agentStatus.value?.nginx_ok) return '配置校验未通过'
  return agentStatus.value?.nginx_running === false ? '已安装未运行' : '正常'
})

// 状态提示：互斥，最多一条（error 未连接 > warning frps 未运行）
const statusAlert = computed<{ type: 'error' | 'warning'; text: string; action?: { label: string; run: () => void } } | null>(() => {
  const current = server.value
  if (!current || !statusLoaded.value) return null
  if (!agentStatusOk.value) {
    return {
      type: 'error',
      text: 'Agent 未连接：请确认 VPS 上 fonu-agent 正在运行，且 Agent 地址与 Token 与页面一致。',
      action: { label: '探测环境', run: () => probe(current) },
    }
  }
  if (!agentStatus.value?.frps_active) {
    return {
      type: 'warning',
      text: 'frps 未运行：反代与隧道都不会生效，可先启动；未安装则用下方「安装 / 升级 frps」。',
      action: { label: '启动 frps', run: () => frpsAction(current, 'start') },
    }
  }
  return null
})

const dashboardURL = computed(() => {
  const current = server.value
  if (!current) return ''
  const addr = (current.options.dashboard_addr || '').trim()
  const port = current.options.dashboard_port || 0
  if (!addr || !port) return ''
  const host = addr === '0.0.0.0' || addr === '::' ? (current.server_addr || '127.0.0.1') : addr
  return `http://${host}:${port}`
})

function portOn(port: string) {
  return agentStatus.value?.ports?.[port] === true
}

function stateClass(ok: boolean, loaded: boolean) {
  if (!loaded) return 'stat-card__icon--blue'
  return ok ? 'stat-card__icon--green' : 'stat-card__icon--red'
}

function pushOutput(source: string, text: string, ok = true) {
  const trimmed = (text || '').trim()
  if (!trimmed) return
  opOutput.value = [{ at: new Date().toISOString(), source, text: trimmed, ok }, ...opOutput.value].slice(0, 20)
}

// 统一的操作执行器：按动作粒度 loading + 串行化（按钮已 disabled，这里只是安全网）
async function run<T>(action: Action, source: string, fn: () => Promise<T>, describe?: (result: T) => { text: string; ok: boolean }) {
  if (busy.value) return
  busy.value = action
  try {
    const result = await fn()
    const described = describe ? describe(result) : { text: '操作完成', ok: true }
    pushOutput(source, described.text, described.ok)
    if (!described.ok) message.error(`${source}失败，详见「输出与日志」`)
    return result
  } catch (error) {
    const text = error instanceof Error ? error.message : `${source}失败`
    pushOutput(source, text, false)
    message.error(`${source}失败，详见「输出与日志」`)
    return undefined
  } finally { busy.value = null }
}

async function load() {
  loadError.value = ''
  try {
    servers.value = (await api.listFrpServers()).filter((item) => item.agent_configured || item.ssh_configured)
    if (selectedId.value === null || !servers.value.some((item) => item.id === selectedId.value)) {
      selectedId.value = servers.value[0]?.id ?? null
    }
    if (server.value) await refreshAll({ silent: false })
  } catch (error) { loadError.value = error instanceof Error ? error.message : '读取服务端失败' }
}

// VPS 资源（需 agent ≥ 0.10.0）：老版本 agent 没有该端点，错误信息里已带升级提示
const agentMetrics = ref<import('../api/types').FrpAgentMetrics | null>(null)
const metricsError = ref('')
const metricsLoading = ref(false)

async function loadMetrics(serverID: number) {
  metricsLoading.value = true
  try {
    agentMetrics.value = await api.getFrpAgentMetrics(serverID)
    metricsError.value = ''
  } catch (error) {
    agentMetrics.value = null
    metricsError.value = error instanceof Error ? error.message : 'VPS 资源获取失败'
  } finally {
    metricsLoading.value = false
  }
}

function usagePercentOf(used?: number, total?: number) {
  if (!used || !total || total <= 0) return 0
  return Math.min(100, (used / total) * 100)
}

const metricsRows = computed(() => {
  const data = agentMetrics.value
  if (!data) return []
  const rows = [
    { label: 'CPU', percent: Math.min(100, Math.max(0, data.cpu_percent ?? 0)), detail: '整机占用率' },
    {
      label: '内存',
      percent: usagePercentOf(data.mem_used_bytes, data.mem_total_bytes),
      detail: `${formatBytes(data.mem_used_bytes)} / ${formatBytes(data.mem_total_bytes)}`,
    },
    {
      label: '磁盘',
      percent: usagePercentOf(data.disk_used_bytes, data.disk_total_bytes),
      detail: `${formatBytes(data.disk_used_bytes)} / ${formatBytes(data.disk_total_bytes)}`,
    },
  ]
  return rows.map((row) => ({
    ...row,
    value: `${Math.round(row.percent)}%`,
    tone: row.percent >= 90 ? 'danger' : row.percent >= 75 ? 'warn' : 'ok',
  }))
})

const metricsLoadText = computed(() =>
  agentMetrics.value?.load_avg?.length ? agentMetrics.value.load_avg.map((value) => value.toFixed(2)).join(' / ') : '—',
)

const metricsUptimeText = computed(() =>
  agentMetrics.value?.uptime_seconds ? formatUptime(agentMetrics.value.uptime_seconds) : '—',
)

// 轮询走静默（不触碰 busy）；首次加载显示 statusLoading
async function refreshAll(options: { silent?: boolean } = { silent: true }) {
  const current = server.value
  if (!current) return
  if (options.silent === false) statusLoading.value = true
  try {
    agentStatus.value = await api.getFrpAgentStatus(current.id)
    agentStatusOk.value = true
    statusLoaded.value = true
    // 资源采集单独走，不阻断状态加载
    void loadMetrics(current.id)
  } catch (error) {
    agentStatusOk.value = false
    statusLoaded.value = true
    if (options.silent === false) {
      message.error(error instanceof Error ? error.message : 'Agent 状态获取失败')
    }
  } finally { statusLoading.value = false }
}

function probe(target: FrpServer) {
  return run('probe', '探测环境', () => api.probeFrpAgent(target.id), (result) => ({
    text: Object.entries(result as Record<string, unknown>).map(([key, value]) => `${key}: ${value}`).join('\n'),
    ok: true,
  }))
}

function installAgent(target: FrpServer) {
  return run('agent-install', target.agent_configured ? '升级 Agent' : '安装 Agent',
    () => api.installFrpAgent(target.id),
    () => ({ text: 'Agent 安装/升级完成，已自动回填 Agent 地址与 Token', ok: true }))
    .then(async (result) => { if (result !== undefined) await load() })
}

function uninstallAgent(target: FrpServer, keepData: boolean) {
  const source = keepData ? '卸载 Agent（保留路由/证书）' : '卸载 Agent（含数据）'
  return run('agent-uninstall', source, () => api.uninstallFrpAgent(target.id, keepData),
    (result) => ({ text: result?.output || '已卸载', ok: true }))
    .then(async (result) => { if (result !== undefined) await load() })
}

function diagnoseAgent(target: FrpServer) {
  return run('diagnose', 'SSH 诊断', () => api.sshDiagnoseFrpAgent(target.id),
    (result) => ({ text: result?.output || '无输出', ok: true }))
}

function frpsAction(target: FrpServer, action: 'start' | 'stop' | 'restart') {
  const actionText = action === 'stop' ? '停止' : action === 'start' ? '启动' : '重启'
  return run(`frps-${action}` as Action, `${actionText} frps`,
    () => api.frpsServerActionRaw(target.id, action),
    (result) => result && result.ok === false
      ? { text: result.error || `frps ${actionText}后未进入预期状态`, ok: false }
      : { text: `frps 已${actionText}`, ok: true })
    .then(async () => { await refreshAll() })
}

function pushAgentConfig(target: FrpServer) {
  return run('push-config', '下发 frps 配置', () => api.pushFrpConfigToAgent(target.id),
    () => ({ text: 'frps 同源配置已下发到 VPS，点「重启 frps」后生效', ok: true }))
}

// 配置查看：只读，展示 Fonu 本地生成的 frps.toml（即「下发」会写入 VPS 的内容）
async function viewConfig(target: FrpServer) {
  configLoading.value = true
  try {
    const result = await api.getFrpFRPSConfig(target.id)
    configContent.value = result?.content || ''
    configModal.value = true
  } catch (error) {
    message.error(error instanceof Error ? error.message : '读取 frps 配置失败')
  } finally { configLoading.value = false }
}

async function copyConfig() {
  if (await copyText(configContent.value)) message.success('已复制 frps 配置')
  else message.error('复制失败，请手动选择文本复制')
}

function installFrps(target: FrpServer) {
  const version = frpsVersionOptions.value[0]?.value ? (frpsInstallVersion.value?.trim() || frpsVersionOptions.value[0].value) : ''
  return run('frps-install', '安装 / 升级 frps',
    () => api.installFrpServer(target.id, version, ''),
    (result) => {
      const notes = Array.isArray(result?.notes) ? result.notes.join('\n') : ''
      return { text: `frps ${result?.version ?? version} 安装完成\n${notes}`.trim(), ok: true }
    })
    .then(async () => { await refreshAll() })
}

function openDashboard() {
  if (dashboardURL.value) window.open(dashboardURL.value, '_blank', 'noopener')
}

async function loadReleases() {
  if (releases.value.length) return
  releasesLoading.value = true
  try { releases.value = await api.listFrpReleases() } catch { /* 拉取失败时按钮保持禁用并提示 */ }
  finally {
    releasesLoading.value = false
    if (!frpsInstallVersion.value && releases.value.length) frpsInstallVersion.value = releases.value[0].version
  }
}

// 切换服务端必须重置，避免上一台的状态与输出串台
watch(selectedId, () => {
  opOutput.value = []
  agentStatus.value = null
  agentStatusOk.value = false
  statusLoaded.value = false
  void refreshAll({ silent: false })
})

onMounted(() => {
  void load()
  void loadReleases()
  timer = window.setInterval(() => {
    if (!server.value) return
    if (document.visibilityState === 'hidden') return // 页面不可见时暂停轮询
    void refreshAll()
  }, 10000)
})

onBeforeUnmount(() => { if (timer) window.clearInterval(timer) })
</script>

<style scoped>
/* 分区：① 运行状态 → ② 环境与依赖 → ③ frps 控制 → ④ 生命周期 → ⑤ 输出与日志 */
.page-alert { margin-bottom: var(--fonu-space-3); }
.page-alert__body { display: flex; align-items: center; justify-content: space-between; gap: var(--fonu-space-4); }
.entrance-row { display: flex; align-items: center; justify-content: space-between; gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-4); padding: 10px var(--fonu-space-5); border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm); background: var(--fonu-info-soft); }
.entrance-row__text { font-size: 13px; color: var(--fonu-text); }

.stats-row { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-4); }
.stat-card { background: var(--fonu-surface); border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius); box-shadow: var(--fonu-shadow); padding: var(--fonu-space-4) var(--fonu-space-5); min-height: 118px; }
.stat-card__icon { font-size: 18px; margin-bottom: 4px; }
.stat-card__icon--blue { color: var(--fonu-info); }
.stat-card__icon--green { color: var(--fonu-brand); }
.stat-card__icon--amber { color: var(--fonu-warning); }
.stat-card__icon--red { color: var(--fonu-error); }
.stat-card__value { margin-top: 4px; font-size: 30px; font-weight: 700; line-height: 1.1; color: var(--fonu-text); letter-spacing: -0.03em; }
.stat-card__value--sm { font-size: 18px; }
.stat-card__label { margin-top: 8px; font-size: 13px; font-weight: 600; color: var(--fonu-text); }
.stat-card__sub { margin-top: 4px; font-size: 12px; color: var(--fonu-text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.port-list { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
.port-item { font-family: var(--fonu-mono); font-size: 12.5px; padding: 1px 6px; border-radius: 999px; background: var(--fonu-bg-muted); color: var(--fonu-text-muted); }
.port-item--on { background: var(--fonu-brand-soft); color: var(--fonu-brand-text); }

.dep-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-4); }
/* 三列下卡片较窄：短字段（版本/徽标）双列并排，长路径与命令用 .field-grid__wide 整行，控制卡片高度 */
.dep-grid .field-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--fonu-space-2) var(--fonu-space-3); }
/* frps 控制与 Agent 生命周期两列并排，卡片等高 */
.control-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-4); }
.control-grid .section-card { margin-bottom: 0; }
.section-card { margin-bottom: var(--fonu-space-4); }
.section-body { padding: var(--fonu-space-2) var(--fonu-space-5) var(--fonu-space-4); }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--fonu-space-3) var(--fonu-space-4); }
.field-grid > div { display: grid; gap: 4px; min-width: 0; }
.field-grid__wide { grid-column: 1 / -1; }
.field-grid label { color: var(--fonu-text-secondary); font-size: 12px; }
.field-tags { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.agent-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: var(--fonu-space-3); }
.tooltip-trigger { display: inline-flex; }
.frps-install-row { display: flex; align-items: center; gap: 10px; margin-top: var(--fonu-space-3); flex-wrap: wrap; }
.frps-version-select { width: 200px; }
.agent-select { width: 220px; }
.nginx-error { margin-top: var(--fonu-space-2); color: var(--fonu-error); overflow-wrap: anywhere; }

/* 配置查看弹窗（只读） */
.config-modal { background: var(--fonu-surface); border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius); box-shadow: var(--fonu-shadow-md); padding: var(--fonu-space-4) var(--fonu-space-5); }
.config-modal__header { display: flex; align-items: center; justify-content: space-between; gap: var(--fonu-space-3); }
.config-modal__body { margin-top: var(--fonu-space-3); }
.config-modal__code { max-height: 380px; }
.config-modal__footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: var(--fonu-space-4); }
.modal-title { margin: 0; font-size: 15px; font-weight: 600; color: var(--fonu-text); }

.op-list { display: grid; gap: var(--fonu-space-3); }
.op-item { padding: var(--fonu-space-3); border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm); background: var(--fonu-surface); }
.op-item--fail { border-color: var(--fonu-error); }
.op-item__head { display: flex; align-items: center; gap: 8px; }
.op-item__source { font-size: 13px; font-weight: 600; color: var(--fonu-text); }
.op-item__time { margin-left: auto; font-size: 12px; color: var(--fonu-text-muted); }

.code-block {
  margin: var(--fonu-space-2) 0 0;
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
.mono { font-family: var(--fonu-mono); font-size: 13px; }
.mono-wrap { white-space: normal; word-break: break-all; }

/* 首屏一次性入场；用户偏好减少动效时关闭 */
@keyframes fonu-fade-up {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: none; }
}
.stats-row, .dep-grid, .section-card, .entrance-row { animation: fonu-fade-up 160ms ease-out both; }
@media (prefers-reduced-motion: reduce) {
  .stats-row, .dep-grid, .section-card, .entrance-row { animation: none; }
}

@media (max-width: 1199px) {
  .stats-row { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .dep-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 900px) {
  .dep-grid { grid-template-columns: 1fr; }
  .control-grid { grid-template-columns: 1fr; }
  .field-grid { grid-template-columns: 1fr; }
}
@media (max-width: 760px) {
  .stats-row { grid-template-columns: 1fr; }
  .agent-actions { width: 100%; }
  .agent-actions .n-button { min-height: 44px; flex: 1 1 auto; }
  .entrance-row { flex-direction: column; align-items: flex-start; }
}
/* VPS 资源：占满「环境与依赖」整行，避免单独占一格显得突兀 */
.vps-resource-card {
  grid-column: 1 / -1;
}

.res-strip {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr)) auto;
  align-items: center;
  gap: var(--fonu-space-5);
}

.res-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  grid-template-areas:
    'label value'
    'detail detail'
    'bar bar';
  align-items: center;
  gap: var(--fonu-space-2);
  font-size: 13px;
}

.res-row__label {
  grid-area: label;
  color: var(--fonu-text-secondary);
}

.res-row__detail {
  grid-area: detail;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--fonu-text-muted);
}

.res-row__value {
  grid-area: value;
  font-weight: 600;
  color: var(--fonu-text);
  font-variant-numeric: tabular-nums;
}

.res-bar {
  grid-area: bar;
  display: block;
  height: 4px;
  border-radius: 2px;
  background: var(--fonu-border);
  overflow: hidden;
}

.res-bar__fill {
  display: block;
  height: 100%;
  border-radius: 2px;
  background: var(--fonu-brand);
  transition: width 0.3s ease;
}

.res-bar--warn .res-bar__fill {
  background: var(--fonu-warning);
}

.res-bar--danger .res-bar__fill {
  background: var(--fonu-error);
}

.res-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-left: var(--fonu-space-4);
  border-left: 1px solid var(--fonu-border);
  font-size: 12px;
  color: var(--fonu-text-muted);
}

@media (max-width: 900px) {
  .res-strip { grid-template-columns: 1fr; }
  .res-meta { padding-left: 0; border-left: none; }
}
</style>
