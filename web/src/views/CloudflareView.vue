<template>
  <PageHeader title="Cloudflare 隧道" description="通过 cloudflared 把内网 Web 服务发布到 Cloudflare 边缘，无需公网 IP">
    <template #actions>
      <n-button :loading="settingsLoading" @click="openSettings">应用配置</n-button>
      <n-button type="primary" @click="openCreate">新建隧道</n-button>
    </template>
  </PageHeader>

  <n-alert v-if="binaryVersion" type="info" :bordered="false" class="cf-alert">
    已安装：<span class="mono">{{ binaryVersion }}</span>
  </n-alert>
  <n-alert v-else type="warning" :bordered="false" class="cf-alert">
    尚未安装 cloudflared。请先点右上角「cloudflared 二进制」下载，再创建隧道。
  </n-alert>

  <FonuCard title="隧道列表" subtitle="token 模式默认使用 Fonu 生成的本地 ingress；云端规则模式请在 Cloudflare 面板管理">
    <div v-if="!tunnels.length" class="cf-empty">还没有隧道，点「新建隧道」开始。</div>
    <div v-for="item in tunnels" :key="item.id" class="cf-row">
      <div class="cf-row__main">
        <strong>{{ item.name }}</strong>
        <StatusBadge :kind="statusKind(item.status)" :text="statusText(item.status)" />
        <span class="cf-meta mono">{{ modeText(item.mode) }}</span>
        <span class="cf-meta mono">{{ item.tunnel_id || '—' }}</span>
        <span v-if="item.metrics_port" class="cf-meta mono">metrics :{{ item.metrics_port }}</span>
      </div>
      <div class="cf-row__actions">
        <n-button size="small" @click="openDetail(item)">详情</n-button>
        <n-button size="small" @click="openEdit(item)">编辑</n-button>
        <n-button v-if="item.status !== 'connected' && item.status !== 'starting'" size="small" type="success" @click="startTunnel(item)">启动</n-button>
        <n-button v-else size="small" type="warning" @click="stopTunnel(item)">停止</n-button>
        <n-button size="small" @click="restartTunnel(item)">重启</n-button>
        <n-popconfirm @positive-click="removeTunnel(item)">
          <template #trigger><n-button size="small" type="error" quaternary>删除</n-button></template>
          将停止该隧道并删除本地配置、凭据与日志。继续？
        </n-popconfirm>
      </div>
    </div>
  </FonuCard>

  <n-modal v-model:show="editorVisible" preset="card" :style="{ width: 'min(720px, 96vw)' }" :title="editing ? '编辑隧道' : '新建隧道'">
    <div class="cf-modal__body">
      <n-alert type="info" :bordered="false" class="cf-mode-alert">
        隧道由 Cloudflare API 创建；Fonu 负责本地配置、进程、日志与健康。
      </n-alert>
      <n-alert v-if="!appSettings.account_id || !appSettings.api_token_configured" type="warning" :bordered="false" class="cf-mode-alert">
        请先在「应用配置」中填写 Account ID 与 API Token，再创建隧道。
      </n-alert>
      <n-divider title-placement="left">基本信息</n-divider>
      <n-form label-placement="top">
        <n-form-item label="名称"><n-input v-model:value="form.name" placeholder="例如 home-nas" /></n-form-item>
      </n-form>
      <n-divider title-placement="left">暴露规则</n-divider>
      <n-form label-placement="top">
        <n-form-item label="暴露域名">
          <n-select v-model:value="form.hostnames" multiple filterable tag :options="hostnameOptions" placeholder="从本机反代规则选择，或直接输入域名" />
        </n-form-item>
        <n-form-item label="回源 service">
          <n-input v-model:value="form.origin_service" placeholder="http://127.0.0.1:80" />
        </n-form-item>
      </n-form>
      <n-divider title-placement="left">运行</n-divider>
      <n-form label-placement="top">
        <n-form-item label="随 Fonu 自动启动"><n-switch v-model:value="form.auto_start" /></n-form-item>
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
        <div class="cf-stat"><label>延迟</label><strong>{{ (detailStatus?.latency_ms ?? 0).toFixed(0) }} ms</strong></div>
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
        <div><label>当前版本</label><strong class="mono">{{ binaryVersion || '未安装' }}</strong></div>
        <div><label>目标文件</label><strong class="mono">cloudflared</strong></div>
      </div>
      <n-form label-placement="top">
        <n-form-item label="下载镜像">
          <n-select v-model:value="settingsForm.mirror" :options="mirrorOptions" />
        </n-form-item>
      </n-form>
      <div class="cf-code-toolbar">
        <n-button size="small" :loading="binaryLoading" @click="downloadBinary">下载并安装</n-button>
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
      <p class="cf-hint">Fonu 使用 Account ID + API Token 调用 Cloudflare API 创建和管理隧道，不再需要手工填写 Tunnel Token。</p>

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
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NDivider, NForm, NFormItem, NInput, NInputNumber, NModal, NPopconfirm, NSelect, NSwitch, NTabPane, NTabs, useMessage } from 'naive-ui'
import { api, asList } from '../api/client'
import type { CfAppSettings, CfAppSettingsInput, CfMirror, CfRuntimeStatus, CfTunnel, CfTunnelPayload } from '../api/types'
import FonuCard from '../components/FonuCard.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { useVisibilityPolling } from '../composables/useVisibilityPolling'
import { copyText } from '../utils/clipboard'
import type { StatusKind } from '../utils/status'

const message = useMessage()
const tunnels = ref<CfTunnel[]>([])
const binaryVersion = ref('')
const mirrors = ref<CfMirror[]>([])
const candidates = ref<string[]>([])
const loading = ref(false)
const saving = ref(false)
const binaryLoading = ref(false)
const settingsVisible = ref(false)
const settingsLoading = ref(false)
const settingsSaving = ref(false)
const tokenTesting = ref(false)
const tokenTestResult = ref('')
const tokenTestOk = ref(false)
const appSettings = ref<CfAppSettings>({
  account_id: '',
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
const editing = ref<CfTunnel | null>(null)
const detailTunnel = ref<CfTunnel | null>(null)
const detailStatus = ref<CfRuntimeStatus | null>(null)
const detailConfig = ref('')
const detailLogs = ref('')
const detailLoading = ref(false)

const form = ref<CfTunnelPayload>(emptyForm())

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
const hostnameOptions = computed(() => candidates.value.map((host) => ({ label: host, value: host })))
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
  }
}

function emptyNetwork() {
  return { transport_protocol: 'http2', edge_ip_version: '4', ha_connections: 2, proxy_mode: 'system' }
}

async function load() {
  loading.value = true
  try {
    tunnels.value = asList(await api.listCfTunnels())
  } catch (error) {
    message.error(error instanceof Error ? error.message : '读取隧道列表失败')
  } finally {
    loading.value = false
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

async function loadCandidates() {
  try {
    candidates.value = (await api.getCfIngressCandidates()).hostnames || []
  } catch {
    candidates.value = []
  }
}

function openCreate() {
  editing.value = null
  form.value = emptyForm()
  editorVisible.value = true
}

function openEdit(item: CfTunnel) {
  editing.value = item
  form.value = {
    name: item.name,
    mode: item.mode,
    token: '',
    hostnames: [...(item.hostnames || [])],
    origin_service: item.origin_service || 'http://127.0.0.1:80',
    network: emptyNetwork(),
    auto_start: item.auto_start,
  }
  editorVisible.value = true
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
  if (!form.value.hostnames.length) {
    message.warning('请至少选择一个暴露域名')
    return
  }
  const payload: CfTunnelPayload = {
    ...form.value,
    mode: editing.value ? form.value.mode : 'account-local',
    token: '',
    network: { ...appSettings.value.network },
  }
  saving.value = true
  try {
    if (editing.value) {
      await api.updateCfTunnel(editing.value.id, payload)
    } else {
      await api.createCfTunnel(payload)
    }
    editorVisible.value = false
    message.success('隧道已保存')
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
    const result = await api.testCfToken((settingsForm.value.api_token || '').trim(), 'api')
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
    const result = await api.downloadCfBinary(settingsForm.value.mirror || 'official')
    binaryVersion.value = result.version || ''
    message.success('cloudflared 已下载并安装')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '下载 cloudflared 失败')
  } finally {
    binaryLoading.value = false
  }
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
  void loadCandidates()
})

useVisibilityPolling(() => load(), 5000)
useVisibilityPolling(refreshDetail, 3000, { enabled: detailVisible })
</script>

<style scoped>
.cf-alert { margin-bottom: var(--fonu-space-4); }
.cf-empty { padding: 24px 0; color: var(--fonu-text-muted); }
.cf-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--fonu-space-4);
  padding: 12px 0;
  border-bottom: 1px solid var(--fonu-border);
}
.cf-row:last-child { border-bottom: none; }
.cf-row__main { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.cf-row__actions { display: flex; gap: 6px; flex-wrap: wrap; }
.cf-meta { font-size: 12px; color: var(--fonu-text-secondary); }
.cf-modal__body { max-height: 68vh; overflow: auto; padding-right: 4px; }
.cf-modal__footer { display: flex; justify-content: flex-end; gap: 8px; }
.cf-mode-alert { margin-bottom: 4px; }
.cf-detail__stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: 10px; margin-bottom: 12px; }
.cf-stat { display: grid; gap: 4px; padding: 10px 12px; border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm); background: var(--fonu-bg); }
.cf-stat label { font-size: 12px; color: var(--fonu-text-muted); }
.cf-stat strong { font-size: 15px; }
.cf-code-toolbar { display: flex; justify-content: flex-end; margin-bottom: 6px; }
.cf-binary { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 10px; margin-bottom: 12px; }
.cf-binary > div { display: grid; gap: 4px; padding: 10px 12px; border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm); background: var(--fonu-bg); }
.cf-binary label { font-size: 12px; color: var(--fonu-text-muted); }
.cf-hint { font-size: 12px; color: var(--fonu-text-muted); }
.cf-pre {
  max-height: 240px;
  overflow: auto;
  padding: 12px;
  border: 1px solid var(--fonu-border);
  border-radius: var(--fonu-radius-sm);
  background: var(--fonu-bg);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
