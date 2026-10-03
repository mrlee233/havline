<template>
  <PageHeader title="Cloudflare 隧道" description="通过 cloudflared 把内网 Web 服务发布到 Cloudflare 边缘，无需公网 IP">
    <template #actions>
      <n-button :loading="preflightLoading" @click="openPreflight">接入向导</n-button>
      <n-button :loading="settingsLoading" @click="openSettings">应用配置</n-button>
      <n-button type="primary" @click="openCreate">新建隧道</n-button>
    </template>
  </PageHeader>

  <div class="cf-global" :class="{ 'cf-global--off': !appSettings.enabled }">
    <div class="cf-global__text">
      <strong>全局托管</strong>
      <span>{{ appSettings.enabled ? '已启用：开启时和下次启动 Havline 时都会启动全部隧道' : '已停用：所有隧道已停止，应用重启后也不会自动拉起' }}</span>
    </div>
    <n-switch :value="appSettings.enabled" :loading="globalLoading" @update:value="toggleGlobal" />
  </div>

  <n-alert v-if="binaryVersion" type="info" :bordered="false" class="cf-alert">
    已安装：<span class="mono">{{ displayBinaryVersion(binaryVersion) }}</span>
  </n-alert>
  <n-alert v-else type="warning" :bordered="false" class="cf-alert">
    尚未安装 cloudflared。请先点右上角「cloudflared 二进制」下载，再创建隧道。
  </n-alert>
  <n-alert type="info" :bordered="false" class="cf-alert">
    Cloudflare Tunnel 适合 NAS 面板、相册和小型 Web 服务。免费套餐不适合持续大文件或视频分发，请不要把它当作大流量媒体出口。
  </n-alert>

  <HavlineCard title="隧道列表" subtitle="托管隧道从「反向代理」规则的 Cloudflare 出口自动派生；手工隧道仍在 Cloudflare 面板管理">
    <div v-if="!tunnels.length" class="cf-empty">还没有隧道，点「新建隧道」开始。</div>
    <div v-else class="cf-table">
      <div class="cf-table__head">
        <span>隧道</span>
        <span>状态</span>
        <span>副本</span>
        <span>版本</span>
        <span>路由</span>
        <span>调和</span>
        <span>操作</span>
      </div>
      <div v-for="item in tunnels" :key="item.id" class="cf-table__row">
        <div class="cf-table__tunnel">
          <strong>{{ item.name }}</strong>
          <div class="cf-table__meta">
            <n-tag v-if="item.managed" size="small" type="info" :bordered="false" round>自动托管</n-tag>
            <span class="cf-meta mono">{{ modeText(item.mode) }}</span>
            <span class="cf-meta mono">{{ item.tunnel_id || '—' }}</span>
            <span v-if="item.metrics_port" class="cf-meta mono">metrics :{{ item.metrics_port }}</span>
          </div>
        </div>
        <div>
          <StatusBadge
            :kind="statusKind(runtimeStatus(item).status || item.status)"
            :text="statusText(runtimeStatus(item).status || item.status)"
          />
        </div>
        <div class="cf-table__number">{{ replicaCount(item) }}</div>
        <div class="cf-meta mono">{{ displayBinaryVersion(binaryVersion) || '—' }}</div>
        <div><n-tag size="small" :bordered="false" round>{{ routeCount(item) }} 条</n-tag></div>
        <div>
          <n-tag size="small" :type="driftTagType(item)" :bordered="false" round>{{ driftText(item) }}</n-tag>
        </div>
        <div class="cf-row__actions">
          <n-button size="small" @click="openDetail(item)">详情</n-button>
          <n-button size="small" @click="openConfig(item)">配置</n-button>
          <n-button v-if="runtimeStatus(item).status !== 'connected' && runtimeStatus(item).status !== 'starting'" size="small" type="success" :disabled="!appSettings.enabled" @click="startTunnel(item)">启动</n-button>
          <n-button v-else size="small" type="warning" @click="stopTunnel(item)">停止</n-button>
          <n-button size="small" @click="restartTunnel(item)">重启</n-button>
          <n-button v-if="driftByTunnel[item.id]?.has_drift" size="small" type="warning" @click="repairTunnel(item)">修复</n-button>
          <n-popconfirm @positive-click="removeTunnel(item)">
            <template #trigger><n-button size="small" type="error" quaternary>删除</n-button></template>
            将停止该隧道并删除本地配置、凭据与日志。继续？
          </n-popconfirm>
        </div>
      </div>
    </div>
  </HavlineCard>

  <n-modal v-model:show="editorVisible" preset="card" :style="{ width: 'min(720px, 96vw)' }" title="新建隧道">
    <div class="cf-modal__body">
      <n-alert type="info" :bordered="false" class="cf-mode-alert">
        隧道由 Cloudflare API 创建；Havline 负责本地配置、进程、日志与健康。
      </n-alert>
      <n-alert v-if="!appSettings.account_id || !appSettings.api_token_configured" type="warning" :bordered="false" class="cf-mode-alert">
        请先在「应用配置」中填写 Account ID 与 API Token，再创建隧道。
      </n-alert>
      <n-divider title-placement="left">基本信息</n-divider>
      <n-form label-placement="top">
        <n-form-item label="名称"><n-input v-model:value="form.name" placeholder="例如 home-nas" /></n-form-item>
        <n-form-item label="自动托管">
          <div class="cf-managed-field">
            <n-switch v-model:value="form.managed" />
            <span>启用后，暴露域名与回源由「反向代理」规则中的 Cloudflare 出口自动生成。</span>
          </div>
        </n-form-item>
      </n-form>
      <template v-if="!form.managed">
        <n-divider title-placement="left">暴露规则</n-divider>
        <n-form label-placement="top">
          <div class="cf-rule-row">
            <n-form-item label="子域名（可选）" class="cf-rule-row__subdomain">
              <n-input v-model:value="manualRule.subdomain" placeholder="例如 999" />
            </n-form-item>
            <n-form-item label="域" class="cf-rule-row__domain">
              <n-select
                v-model:value="manualRule.domain"
                :options="domainOptions"
                filterable
                tag
                placeholder="从 Cloudflare Zone 选择"
              />
            </n-form-item>
          </div>
          <div class="cf-hostname-preview">
            <span>完整主机名：</span>
            <strong class="mono">{{ manualHostname() || '未选择域' }}</strong>
          </div>
          <n-form-item label="服务">
            <div class="cf-origin-row">
              <n-select v-model:value="manualRule.type" :options="originTypeOptions" class="cf-origin-row__type" />
              <span class="cf-origin-row__scheme">://</span>
              <n-input v-model:value="manualRule.target" placeholder="192.168.11.30:8001" />
            </div>
            <p class="cf-hint">路由流量的源服务，例如：192.168.11.30:8001、localhost:8080、192.168.1.2:3306</p>
          </n-form-item>
        </n-form>
      </template>
      <n-alert v-else type="info" :bordered="false" class="cf-mode-alert">
        保存托管隧道后，请到「反向代理」编辑服务规则，在出口中选择 Cloudflare 并指定该隧道。
      </n-alert>
      <n-divider title-placement="left">运行</n-divider>
      <n-form label-placement="top">
        <n-form-item label="随 Havline 自动启动"><n-switch v-model:value="form.auto_start" /></n-form-item>
      </n-form>
      <p class="cf-hint">网络调优统一在「应用配置」中管理，保存后会应用到新建隧道。</p>
    </div>
    <template #footer>
      <div class="cf-modal__footer">
        <n-button @click="editorVisible = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="saveTunnel">保存</n-button>
      </div>
    </template>
  </n-modal>

  <n-modal v-model:show="detailVisible" preset="card" :style="{ width: 'min(760px, 96vw)' }" :title="detailTunnel?.name || '隧道详情'">
    <div v-if="detailTunnel" class="cf-detail">
      <div class="cf-detail__stats">
        <div class="cf-stat"><label>状态</label><StatusBadge :kind="statusKind(detailStatus?.status || detailTunnel.status)" :text="statusText(detailStatus?.status || detailTunnel.status)" /></div>
        <div class="cf-stat"><label>连接数</label><strong>{{ detailStatus?.connections ?? 0 }}</strong></div>
        <div class="cf-stat"><label>延迟</label><strong>{{ detailStatus?.transport === 'http2' ? '—' : `${(detailStatus?.latency_ms ?? 0).toFixed(0)} ms` }}</strong></div>
        <div class="cf-stat"><label>传输协议</label><strong>{{ detailStatus?.transport || '—' }}</strong></div>
        <div class="cf-stat"><label>metrics 端口</label><strong>{{ detailStatus?.metrics_port || '—' }}</strong></div>
      </div>
      <p v-if="detailStatus?.bandwidth_notice" class="cf-hint">{{ detailStatus.bandwidth_notice }}</p>
      <n-tabs type="line" animated>
        <n-tab-pane name="config" tab="config.yml">
          <div class="cf-code-toolbar"><n-button size="tiny" @click="copyConfig">复制</n-button></div>
          <pre class="cf-pre">{{ detailConfig || '暂无配置' }}</pre>
        </n-tab-pane>
        <n-tab-pane name="logs" tab="日志">
          <div class="cf-code-toolbar"><n-button size="tiny" :loading="detailLoading" @click="refreshDetail">刷新</n-button></div>
          <pre class="cf-pre">{{ detailLogs || '暂无日志' }}</pre>
        </n-tab-pane>
        <n-tab-pane name="diagnostics" tab="漂移与诊断">
          <div class="cf-code-toolbar">
            <n-button size="tiny" :loading="driftLoading" @click="loadDrift">检测漂移</n-button>
            <n-button size="tiny" :disabled="!detailDrift?.has_drift" :loading="driftSyncing" @click="syncDrift">以本地覆盖</n-button>
            <n-button size="tiny" :loading="diagnosticsLoading" @click="loadDiagnostics">运行诊断</n-button>
            <n-button size="tiny" :loading="accessLoading" @click="loadAccessStatus">检查 Access</n-button>
            <n-button size="tiny" :loading="credentialLoading" @click="refreshCredentials">刷新凭据</n-button>
          </div>
          <n-alert v-if="detailDrift" :type="detailDrift.has_drift ? 'warning' : 'success'" :bordered="false" class="cf-mode-alert">
            <template v-if="detailDrift.has_drift">
              发现 {{ detailDrift.items.length }} 项云端漂移：
              <div v-for="item in detailDrift.items" :key="item" class="mono">{{ item }}</div>
            </template>
            <template v-else>本地规则与 Cloudflare 云端配置一致。</template>
          </n-alert>
          <div v-if="detailDiagnostics" class="cf-diagnostics">
            <div v-for="check in detailDiagnostics.checks" :key="check.name" class="cf-diagnostic-row">
              <n-tag size="small" :type="diagnosticTag(check.status)" :bordered="false">{{ check.name }}</n-tag>
              <span>{{ check.message }}</span>
            </div>
          </div>
          <n-alert v-if="detailAccess" type="info" :bordered="false" class="cf-mode-alert">
            {{ detailAccess.enabled ? `已启用 Cloudflare Access${detailAccess.name ? '：' + detailAccess.name : ''}` : '未检测到 Cloudflare Access 保护。Havline 只做只读检测，不代配策略。' }}
          </n-alert>
        </n-tab-pane>
      </n-tabs>
    </div>
    <template #footer>
      <div class="cf-modal__footer">
        <n-button @click="refreshDetail">刷新全部</n-button>
        <n-button type="primary" @click="detailVisible = false">关闭</n-button>
      </div>
    </template>
  </n-modal>

  <n-modal v-model:show="settingsVisible" preset="card" :style="{ width: 'min(640px, 96vw)' }" title="Cloudflare 应用配置">
    <div class="cf-modal__body">
      <n-divider title-placement="left">程序安装</n-divider>
      <div class="cf-binary">
        <div><label>当前版本</label><strong class="mono">{{ displayBinaryVersion(binaryVersion) || '未安装' }}</strong></div>
        <div><label>检测到的最新版本</label><strong class="mono">{{ latestBinaryVersion || '未检查' }}</strong></div>
        <div><label>目标文件</label><strong class="mono">cloudflared</strong></div>
      </div>
      <n-form label-placement="top">
        <n-form-item label="下载镜像">
          <n-select v-model:value="settingsForm.mirror" :options="mirrorOptions" />
        </n-form-item>
      </n-form>
      <div class="cf-code-toolbar">
        <n-button size="small" :loading="binaryChecking" @click="checkLatestBinary">检查更新</n-button>
        <n-button size="small" :loading="binaryLoading" @click="downloadBinary">下载并安装</n-button>
        <n-button size="small" :loading="binaryRollbackLoading" @click="rollbackBinary">回滚上一版</n-button>
      </div>
      <p class="cf-hint">下载后会校验文件大小与可执行性，并原子替换现有二进制。国内环境推荐 gh-proxy 镜像。</p>

      <n-divider title-placement="left">Cloudflare 账号</n-divider>
      <n-form label-placement="top">
        <n-form-item label="Account ID">
          <n-input v-model:value="settingsForm.account_id" :placeholder="appSettings.account_id || '例如 b590ee0f16701d36f2819b0e3c938e81'" />
        </n-form-item>
        <n-form-item label="API Token">
          <n-input
            v-model:value="settingsForm.api_token"
            type="password"
            show-password-on="click"
            :placeholder="appSettings.api_token_configured ? `已配置 ${appSettings.api_token_masked}，留空保留` : 'cfat_...'"
          />
        </n-form-item>
      </n-form>
      <div class="cf-code-toolbar">
        <n-button size="small" :loading="tokenTesting" @click="testToken">测试连接</n-button>
        <n-button size="small" tag="a" href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noopener">获取 API Token</n-button>
      </div>
      <n-alert v-if="tokenTestResult" :type="tokenTestOk ? 'success' : 'error'" :bordered="false" class="cf-mode-alert">
        {{ tokenTestResult }}
      </n-alert>
      <p class="cf-hint">Havline 使用 Account ID + API Token 调用 Cloudflare API 创建和管理隧道与 DNS。API Token 需要 Account: Cloudflare Tunnel: Edit、Zone: Zone: Read、Zone: DNS: Edit 权限，且 Zone Resources 要包含所有要发布的根域名。</p>

      <n-divider title-placement="left">默认网络设置</n-divider>
      <n-form label-placement="top">
        <n-form-item label="传输协议"><n-select v-model:value="settingsForm.network.transport_protocol" :options="protocolOptions" /></n-form-item>
        <n-form-item label="边缘地址族"><n-select v-model:value="settingsForm.network.edge_ip_version" :options="edgeOptions" /></n-form-item>
        <n-form-item label="HA 连接数"><n-input-number v-model:value="settingsForm.network.ha_connections" :min="1" :max="4" /></n-form-item>
        <n-form-item label="代理模式"><n-select v-model:value="settingsForm.network.proxy_mode" :options="proxyOptions" /></n-form-item>
      </n-form>
    </div>
    <template #footer>
      <div class="cf-modal__footer">
        <n-button @click="settingsVisible = false">取消</n-button>
        <n-button type="primary" :loading="settingsSaving" @click="saveSettings">保存配置</n-button>
      </div>
    </template>
  </n-modal>

  <n-modal v-model:show="preflightVisible" preset="card" :style="{ width: 'min(620px, 96vw)' }" title="Cloudflare 接入向导">
    <div class="cf-preflight">
      <div v-for="check in preflight?.checks || []" :key="check.name" class="cf-preflight__row">
        <n-tag size="small" :type="check.status === 'ok' ? 'success' : 'error'" :bordered="false">{{ check.name }}</n-tag>
        <span>{{ check.message }}</span>
        <n-button v-if="check.action === 'settings' && check.status !== 'ok'" size="tiny" @click="openSettings">去配置</n-button>
      </div>
      <n-alert v-if="preflight" :type="preflight.ready ? 'success' : 'warning'" :bordered="false" class="cf-mode-alert">
        {{ preflight.ready ? '预检通过，可以创建隧道或发布服务。' : '还有项目未完成，请按上面的提示处理。' }}
      </n-alert>
    </div>
    <template #footer>
      <div class="cf-modal__footer">
        <n-button :loading="preflightLoading" @click="loadPreflight">重新检查</n-button>
        <n-button type="primary" @click="preflightVisible = false">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NDivider, NForm, NFormItem, NInput, NInputNumber, NModal, NPopconfirm, NSelect, NSwitch, NTabPane, NTabs, useMessage } from 'naive-ui'
import { api, asList } from '../api/client'
import type { CfAccessStatus, CfAppSettings, CfAppSettingsInput, CfDiagnosticsResult, CfDriftResult, CfMirror, CfPreflightResult, CfRuntimeStatus, CfTunnel, CfTunnelPayload, CfZone } from '../api/types'
import HavlineCard from '../components/HavlineCard.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { useVisibilityPolling } from '../composables/useVisibilityPolling'
import { copyText } from '../utils/clipboard'
import type { StatusKind } from '../utils/status'

const message = useMessage()
const router = useRouter()
const tunnels = ref<CfTunnel[]>([])
const runtimeStatuses = ref<Record<number, CfRuntimeStatus>>({})
const driftByTunnel = ref<Record<number, CfDriftResult>>({})
const binaryVersion = ref('')
const latestBinaryVersion = ref('')
const mirrors = ref<CfMirror[]>([])
const zones = ref<CfZone[]>([])
const loading = ref(false)
const saving = ref(false)
const binaryLoading = ref(false)
const binaryChecking = ref(false)
const binaryRollbackLoading = ref(false)
const globalLoading = ref(false)
const settingsVisible = ref(false)
const settingsLoading = ref(false)
const settingsSaving = ref(false)
const tokenTesting = ref(false)
const tokenTestResult = ref('')
const tokenTestOk = ref(false)
const appSettings = ref<CfAppSettings>({
  account_id: '',
  enabled: true,
  default_token_configured: false,
  default_token_masked: '',
  api_token_configured: false,
  api_token_masked: '',
  mirror: 'official',
  network: emptyNetwork(),
})
const settingsForm = ref<CfAppSettingsInput>({ account_id: '', token: '', api_token: '', mirror: 'official', network: emptyNetwork() })
const editorVisible = ref(false)
const detailVisible = ref(false)
const detailTunnel = ref<CfTunnel | null>(null)
const detailStatus = ref<CfRuntimeStatus | null>(null)
const detailConfig = ref('')
const detailLogs = ref('')
const detailLoading = ref(false)
const detailDrift = ref<CfDriftResult | null>(null)
const detailDiagnostics = ref<CfDiagnosticsResult | null>(null)
const driftLoading = ref(false)
const driftSyncing = ref(false)
const diagnosticsLoading = ref(false)
const detailAccess = ref<CfAccessStatus | null>(null)
const accessLoading = ref(false)
const credentialLoading = ref(false)
const preflightVisible = ref(false)
const preflightLoading = ref(false)
const preflight = ref<CfPreflightResult | null>(null)

const form = ref<CfTunnelPayload>(emptyForm())
const manualRule = ref({ subdomain: '', domain: '', type: 'http', target: '127.0.0.1:80' })

const protocolOptions = [
  { label: 'HTTP/2（国内推荐）', value: 'http2' },
  { label: 'QUIC', value: 'quic' },
  { label: '自动', value: 'auto' },
]
const edgeOptions = [
  { label: 'IPv4（推荐）', value: '4' },
  { label: 'IPv6', value: '6' },
  { label: '自动', value: 'auto' },
]
const proxyOptions = [
  { label: '跟随系统环境变量', value: 'system' },
  { label: '禁用代理', value: 'disabled' },
]
const originTypeOptions = [
  { label: 'HTTP', value: 'http' },
  { label: 'HTTPS', value: 'https' },
  { label: 'TCP', value: 'tcp' },
  { label: 'SSH', value: 'ssh' },
]
const domainOptions = computed(() => zones.value.map((zone) => ({ label: zone.name, value: zone.name })))
const mirrorOptions = computed(() => {
  const list = mirrors.value.length ? mirrors.value : [{ id: 'official', name: 'GitHub 官方源', base: '' }]
  return list.map((item) => ({ label: item.name, value: item.id }))
})
function emptyForm(): CfTunnelPayload {
  return {
    name: '',
    mode: 'account-local',
    token: '',
    hostnames: [],
    origin_service: 'http://127.0.0.1:80',
    network: emptyNetwork(),
    auto_start: false,
    managed: true,
  }
}

function emptyNetwork() {
  return { transport_protocol: 'http2', edge_ip_version: '4', ha_connections: 2, proxy_mode: 'system' }
}

async function load() {
  loading.value = true
  try {
    tunnels.value = asList(await api.listCfTunnels())
    const pairs = await Promise.all(
      tunnels.value.map(async (tunnel) => {
        try {
          return [tunnel.id, await api.getCfTunnelStatus(tunnel.id)] as const
        } catch {
          return [tunnel.id, { running: false, status: tunnel.status, connections: 0, latency_ms: 0, bandwidth_in: 0, bandwidth_out: 0, transport: '' }] as const
        }
      }),
    )
    runtimeStatuses.value = Object.fromEntries(pairs)
    const driftPairs = await Promise.all(
      tunnels.value.filter((tunnel) => tunnel.managed).map(async (tunnel) => {
        try {
          return [tunnel.id, await api.getCfTunnelDrift(tunnel.id)] as const
        } catch {
          return [tunnel.id, { tunnel_id: tunnel.id, managed: true, has_drift: false, items: [] }] as const
        }
      }),
    )
    driftByTunnel.value = Object.fromEntries(driftPairs)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '读取隧道列表失败')
  } finally {
    loading.value = false
  }
}

function runtimeStatus(item: CfTunnel): CfRuntimeStatus {
  return runtimeStatuses.value[item.id] ?? {
    running: false,
    status: item.status,
    connections: 0,
    latency_ms: 0,
    bandwidth_in: 0,
    bandwidth_out: 0,
    transport: '',
  }
}

function replicaCount(item: CfTunnel): number {
  return runtimeStatus(item).connections || 0
}

function routeCount(item: CfTunnel): number {
  return item.hostnames?.length || 0
}

function driftText(item: CfTunnel): string {
  if (!item.managed) return '手工'
  const drift = driftByTunnel.value[item.id]
  if (!drift) return '未知'
  return drift.has_drift ? '有漂移' : '已同步'
}

function driftTagType(item: CfTunnel): 'success' | 'warning' | 'default' {
  if (!item.managed) return 'default'
  return driftByTunnel.value[item.id]?.has_drift ? 'warning' : 'success'
}

async function repairTunnel(item: CfTunnel) {
  try {
    await api.syncCfTunnelRoutes(item.id)
    message.success('已按本地规则修复云端配置')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '修复失败')
  }
}

async function toggleGlobal(value: boolean) {
  globalLoading.value = true
  try {
    const result = await api.setCfEnabled(value)
    appSettings.value = result
    message.success(value ? '全局托管已启用' : '全局托管已停用，所有隧道已停止')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '切换全局托管失败')
  } finally {
    globalLoading.value = false
  }
}

async function openPreflight() {
  preflightVisible.value = true
  await loadPreflight()
}

async function loadPreflight() {
  preflightLoading.value = true
  try {
    preflight.value = await api.getCfPreflight()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '预检失败')
  } finally {
    preflightLoading.value = false
  }
}

async function loadBinary() {
  try {
    const result = await api.getCfBinary()
    binaryVersion.value = result.version || ''
    mirrors.value = asList(result.mirrors)
  } catch {
    // 二进制状态不影响列表
  }
}

async function loadZones() {
  try {
    const result = await api.getCfZones()
    zones.value = asList(result.zones)
  } catch {
    zones.value = []
  }
}

function resetManualRule() {
  manualRule.value = { subdomain: '', domain: zones.value[0]?.name || '', type: 'http', target: '127.0.0.1:80' }
}

function manualHostname() {
  const subdomain = manualRule.value.subdomain.trim().replace(/^\.+|\.+$/g, '')
  const domain = manualRule.value.domain.trim().replace(/^\.+|\.+$/g, '')
  if (!domain) return ''
  return subdomain ? `${subdomain}.${domain}` : domain
}

function manualOriginService() {
  const type = manualRule.value.type || 'http'
  const target = manualRule.value.target.trim().replace(/^[a-z]+:\/\//i, '')
  return `${type}://${target}`
}

async function openCreate() {
  if (!zones.value.length) await loadZones()
  form.value = emptyForm()
  resetManualRule()
  editorVisible.value = true
}

function openConfig(item: CfTunnel) {
  void router.push(`/cloudflare/${item.id}/config`)
}

async function saveTunnel() {
  if (!form.value.name.trim()) {
    message.warning('请填写隧道名称')
    return
  }
  if (!appSettings.value.account_id || !appSettings.value.api_token_configured) {
    message.warning('请先在「应用配置」中填写 Account ID 与 API Token')
    return
  }
  if (!form.value.managed) {
    const hostname = manualHostname()
    if (!hostname) {
      message.warning('请选择或填写域')
      return
    }
    if (!manualRule.value.target.trim()) {
      message.warning('请填写源服务 URL')
      return
    }
    form.value.hostnames = [hostname]
    form.value.origin_service = manualOriginService()
  }
  const payload: CfTunnelPayload = {
    ...form.value,
    mode: 'account-local',
    token: '',
    network: { ...appSettings.value.network },
  }
  saving.value = true
  try {
    const result = await api.createCfTunnel(payload)
    editorVisible.value = false
    if (result.dns_warning) {
      message.warning(result.dns_warning)
    } else {
      message.success('隧道已保存')
    }
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存隧道失败')
  } finally {
    saving.value = false
  }
}

async function startTunnel(item: CfTunnel) {
  await runAction('启动', () => api.startCfTunnel(item.id))
}

async function stopTunnel(item: CfTunnel) {
  await runAction('停止', () => api.stopCfTunnel(item.id))
}

async function restartTunnel(item: CfTunnel) {
  await runAction('重启', () => api.restartCfTunnel(item.id))
}

async function removeTunnel(item: CfTunnel) {
  try {
    await api.deleteCfTunnel(item.id)
    message.success('隧道已删除')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '删除隧道失败')
  }
}

async function runAction(label: string, fn: () => Promise<unknown>) {
  try {
    await fn()
    message.success(`${label}指令已下发`)
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : `${label}失败`)
  }
}

async function openDetail(item: CfTunnel) {
  detailTunnel.value = item
  detailDrift.value = null
  detailDiagnostics.value = null
  detailAccess.value = null
  detailVisible.value = true
  await refreshDetail()
}

async function refreshDetail() {
  const item = detailTunnel.value
  if (!item) return
  detailLoading.value = true
  try {
    const [status, config, logs] = await Promise.all([
      api.getCfTunnelStatus(item.id),
      api.getCfTunnelConfig(item.id),
      api.getCfTunnelLogs(item.id, 200),
    ])
    detailStatus.value = status
    detailConfig.value = config.content || ''
    detailLogs.value = logs.output || ''
  } catch (error) {
    message.error(error instanceof Error ? error.message : '读取隧道详情失败')
  } finally {
    detailLoading.value = false
  }
}

async function loadDrift() {
  const item = detailTunnel.value
  if (!item) return
  driftLoading.value = true
  try {
    detailDrift.value = await api.getCfTunnelDrift(item.id)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '漂移检测失败')
  } finally {
    driftLoading.value = false
  }
}

async function syncDrift() {
  const item = detailTunnel.value
  if (!item) return
  driftSyncing.value = true
  try {
    await api.syncCfTunnelRoutes(item.id)
    message.success('已按本地规则覆盖云端配置')
    await Promise.all([loadDrift(), refreshDetail(), load()])
  } catch (error) {
    message.error(error instanceof Error ? error.message : '同步失败')
  } finally {
    driftSyncing.value = false
  }
}

async function loadDiagnostics() {
  const item = detailTunnel.value
  if (!item) return
  diagnosticsLoading.value = true
  try {
    detailDiagnostics.value = await api.getCfTunnelDiagnostics(item.id)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '诊断失败')
  } finally {
    diagnosticsLoading.value = false
  }
}

async function loadAccessStatus() {
  const item = detailTunnel.value
  const hostname = item?.hostnames?.[0]
  if (!hostname) {
    message.warning('该隧道暂无暴露域名')
    return
  }
  accessLoading.value = true
  try {
    detailAccess.value = await api.getCfAccessStatus(hostname)
  } catch (error) {
    message.error(error instanceof Error ? error.message : 'Access 检测失败，请确认 Token 具备 Access: Apps: Read 权限')
  } finally {
    accessLoading.value = false
  }
}

async function refreshCredentials() {
  const item = detailTunnel.value
  if (!item) return
  credentialLoading.value = true
  try {
    detailTunnel.value = await api.refreshCfCredentials(item.id)
    message.success('隧道凭据已刷新')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '刷新凭据失败')
  } finally {
    credentialLoading.value = false
  }
}

function diagnosticTag(status: string) {
  if (status === 'ok') return 'success'
  if (status === 'warning') return 'warning'
  return 'error'
}

async function copyConfig() {
  if (!detailConfig.value) return
  try {
    await copyText(detailConfig.value)
    message.success('已复制 config.yml')
  } catch {
    message.error('复制失败')
  }
}

async function openSettings() {
  preflightVisible.value = false
  settingsVisible.value = true
  await loadAppSettings()
}

async function loadAppSettings() {
  settingsLoading.value = true
  tokenTestResult.value = ''
  try {
    const result = await api.getCfSettings()
    appSettings.value = result
    settingsForm.value = { account_id: result.account_id || '', token: '', api_token: '', mirror: result.mirror || 'official', network: { ...result.network } }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '读取应用配置失败')
  } finally {
    settingsLoading.value = false
  }
}

async function testToken() {
  tokenTesting.value = true
  tokenTestResult.value = ''
  try {
    const result = await api.testCfToken(
      (settingsForm.value.account_id || '').trim(),
      (settingsForm.value.api_token || '').trim(),
      'api',
    )
    tokenTestOk.value = true
    tokenTestResult.value = result.message || '连接成功'
  } catch (error) {
    tokenTestOk.value = false
    tokenTestResult.value = error instanceof Error ? error.message : '连接失败'
  } finally {
    tokenTesting.value = false
  }
}

async function saveSettings() {
  settingsSaving.value = true
  try {
    const result = await api.saveCfSettings(settingsForm.value)
    appSettings.value = result
    settingsForm.value = { account_id: result.account_id || '', token: '', api_token: '', mirror: result.mirror, network: { ...result.network } }
    await loadZones()
    message.success('应用配置已保存')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存应用配置失败')
  } finally {
    settingsSaving.value = false
  }
}

async function downloadBinary() {
  binaryLoading.value = true
  try {
    const result = await api.downloadCfBinary(settingsForm.value.mirror || 'official', latestBinaryVersion.value)
    binaryVersion.value = result.version || ''
    message.success('cloudflared 已下载并安装')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '下载 cloudflared 失败')
  } finally {
    binaryLoading.value = false
  }
}

async function checkLatestBinary() {
  binaryChecking.value = true
  try {
    const result = await api.getLatestCfBinary()
    latestBinaryVersion.value = result.version || ''
    if (binaryVersion.value.includes(latestBinaryVersion.value)) {
      message.success('当前已是最新版本')
    } else {
      message.info(`发现新版本 ${latestBinaryVersion.value}`)
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '检查更新失败')
  } finally {
    binaryChecking.value = false
  }
}

async function rollbackBinary() {
  binaryRollbackLoading.value = true
  try {
    const result = await api.rollbackCfBinary()
    binaryVersion.value = result.version || ''
    latestBinaryVersion.value = ''
    message.success('已回滚 cloudflared')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '回滚失败')
  } finally {
    binaryRollbackLoading.value = false
  }
}

function displayBinaryVersion(version: string) {
  return version.match(/\d{4}\.\d+\.\d+/)?.[0] || version
}

function modeText(mode: string) {
  if (mode === 'account-local') return 'API 管理'
  if (mode === 'token-remote') return '云端规则'
  if (mode === 'token-local') return '本地规则'
  return mode
}

function statusText(status: string) {
  return ({
    connected: '已连接',
    starting: '启动中',
    reconnecting: '重连中',
    failed: '失败',
    stopped: '已停止',
  } as Record<string, string>)[status] || status || '未知'
}

function statusKind(status: string): StatusKind {
  if (status === 'connected') return 'success'
  if (status === 'starting' || status === 'reconnecting') return 'warning'
  if (status === 'failed') return 'error'
  return 'unknown'
}

onMounted(() => {
  void load()
  void loadBinary()
  void loadAppSettings()
  void loadZones()
})

useVisibilityPolling(() => load(), 5000)
useVisibilityPolling(refreshDetail, 3000, { enabled: detailVisible })
</script>

<style scoped>
.cf-global {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--havline-space-4);
  padding: 12px var(--havline-space-5);
  margin-bottom: var(--havline-space-4);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-surface);
}
.cf-global--off { border-color: var(--havline-warning); background: var(--havline-bg); }
.cf-global__text { display: grid; gap: 4px; font-size: 13px; }
.cf-global__text span { color: var(--havline-text-secondary); }
.cf-alert { margin-bottom: var(--havline-space-4); }
.cf-empty { padding: 24px 0; color: var(--havline-text-muted); }
.cf-table { width: 100%; overflow-x: auto; }
.cf-table__head,
.cf-table__row {
  display: grid;
  grid-template-columns: minmax(220px, 1.6fr) 110px 70px minmax(120px, 0.8fr) 80px 90px minmax(340px, 1.8fr);
  gap: 12px;
  align-items: center;
  min-width: 980px;
  padding: 10px 0;
}
.cf-table__head {
  color: var(--havline-text-muted);
  font-size: 12px;
  border-bottom: 1px solid var(--havline-border);
}
.cf-table__row { border-bottom: 1px solid var(--havline-border); }
.cf-table__row:last-child { border-bottom: none; }
.cf-table__tunnel { min-width: 0; }
.cf-table__meta { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-top: 6px; }
.cf-table__number { font-size: 14px; font-weight: 600; }
.cf-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--havline-space-4);
  padding: 12px 0;
  border-bottom: 1px solid var(--havline-border);
}
.cf-row:last-child { border-bottom: none; }
.cf-row__main { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.cf-row__actions { display: flex; gap: 6px; flex-wrap: wrap; }
.cf-meta { font-size: 12px; color: var(--havline-text-secondary); }
.cf-modal__body { max-height: 68vh; overflow: auto; padding-right: 4px; }
.cf-modal__footer { display: flex; justify-content: flex-end; gap: 8px; }
.cf-mode-alert { margin-bottom: 4px; }
.cf-preflight { display: grid; gap: 10px; }
.cf-preflight__row { display: flex; align-items: center; gap: 10px; padding: 10px 12px; border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); }
.cf-preflight__row span { flex: 1; min-width: 0; color: var(--havline-text-secondary); font-size: 12.5px; }
.cf-managed-field { display: flex; align-items: center; gap: 10px; font-size: 12px; color: var(--havline-text-muted); }
.cf-rule-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 12px; }
.cf-rule-row__subdomain,
.cf-rule-row__domain { min-width: 0; }
.cf-hostname-preview {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 var(--havline-space-4);
  padding: 10px 12px;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
}
.cf-hostname-preview span { color: var(--havline-text-muted); font-size: 12px; }
.cf-hostname-preview strong { color: var(--havline-text); }
.cf-origin-row { display: flex; align-items: center; width: 100%; gap: 8px; }
.cf-origin-row__type { width: 126px; flex: none; }
.cf-origin-row__scheme { color: var(--havline-text-muted); flex: none; }
.cf-diagnostics { display: grid; gap: 8px; margin-top: 10px; }
.cf-diagnostic-row { display: flex; align-items: flex-start; gap: 10px; font-size: 12.5px; line-height: 1.5; }
.cf-detail__stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: 10px; margin-bottom: 12px; }
.cf-stat { display: grid; gap: 4px; padding: 10px 12px; border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); background: var(--havline-bg); }
.cf-stat label { font-size: 12px; color: var(--havline-text-muted); }
.cf-stat strong { font-size: 15px; }
.cf-code-toolbar { display: flex; justify-content: flex-end; margin-bottom: 6px; }
.cf-binary { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 10px; margin-bottom: 12px; }
.cf-binary > div { display: grid; gap: 4px; padding: 10px 12px; border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); background: var(--havline-bg); }
.cf-binary label { font-size: 12px; color: var(--havline-text-muted); }
.cf-hint { font-size: 12px; color: var(--havline-text-muted); }
.cf-pre {
  max-height: 240px;
  overflow: auto;
  padding: 12px;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
