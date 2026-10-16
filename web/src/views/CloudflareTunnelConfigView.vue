<template>
  <PageHeader :title="`配置 · ${tunnel?.name || 'Cloudflare 隧道'}`" description="配置隧道、查看副本指标与已发布路由">
    <template #actions>
      <!-- 进程操作提到页头：原来在页面最底部的「运行控制」卡，要滚动到底才能点到 -->
      <n-button
        v-if="runtimeState === 'connected'"
        type="warning"
        :loading="actionRunning === '停止'"
        :disabled="actionRunning !== null"
        @click="runAction('停止')"
      >停止</n-button>
      <n-button
        v-else
        type="success"
        :loading="actionRunning === '启动'"
        :disabled="actionRunning !== null"
        @click="runAction('启动')"
      >启动</n-button>
      <n-button :loading="actionRunning === '重启'" :disabled="actionRunning !== null" @click="runAction('重启')">重启</n-button>
      <n-button @click="router.push('/cloudflare')">
        <template #icon><n-icon :component="ArrowBackOutline" /></template>
        返回列表
      </n-button>
    </template>
  </PageHeader>

  <LoadError v-if="loadError" :message="loadError" @retry="load" />

  <template v-else-if="tunnel">
    <div class="stats-row">
      <StatCard label="活动副本" :value="status?.connections ?? 0" sub="cloudflared HA 连接" tone="blue">
        <template #icon><n-icon :component="GitNetworkOutline" /></template>
      </StatCard>
      <StatCard label="路由" :value="routeData?.routes.length ?? 0" sub="已发布应用" tone="green">
        <template #icon><n-icon :component="LayersOutline" /></template>
      </StatCard>
      <StatCard label="状态" :sub="status?.transport || tunnel.mode" tone="teal">
        <template #icon><n-icon :component="PulseOutline" /></template>
        <template #value><StatusBadge :kind="statusKind(runtimeState)" :text="statusText(runtimeState)" /></template>
      </StatCard>
      <!-- 原来叫「运行时间」但值是进程状态，名实不符：改为「进程」，并换成服务端图标 -->
      <StatCard label="进程" :value="status?.running ? '运行中' : '未运行'" :sub="status?.metrics_port ? `metrics :${status.metrics_port}` : '无 metrics 端口'" tone="brand" value-small>
        <template #icon><n-icon :component="ServerOutline" /></template>
      </StatCard>
    </div>

    <!-- 原来这张卡只有一个按钮／一条提示，等于白占一屏；改成直接展示关键字段，编辑按钮放卡头 -->
    <HavlineCard class="section-card" title="基础配置" subtitle="手工隧道可在这里修改；托管隧道由反向代理规则派生">
      <template #header>
        <n-button v-if="!tunnel.managed" size="small" @click="basicVisible = true">编辑基础配置</n-button>
      </template>
      <div class="section-body">
        <div class="config-facts">
          <div class="config-fact"><label>名称</label><span>{{ tunnel.name || '—' }}</span></div>
          <div class="config-fact"><label>模式</label><span>{{ tunnel.managed ? '自动托管' : '手工隧道' }}</span></div>
          <div class="config-fact"><label>隧道 ID</label><span class="mono">{{ tunnel.tunnel_id || '—' }}</span></div>
          <div class="config-fact"><label>完整主机名</label><span class="mono">{{ tunnel.hostnames?.join('、') || '—' }}</span></div>
          <div class="config-fact"><label>回源服务</label><span class="mono">{{ tunnel.origin_service || '—' }}</span></div>
          <div class="config-fact"><label>随 Havline 启动</label><span>{{ tunnel.auto_start ? '是' : '否' }}</span></div>
        </div>
        <n-alert v-if="tunnel.managed" type="info" :bordered="false" class="config-managed-hint">
          自动托管隧道：暴露域名与回源在
          <router-link to="/proxies">反向代理</router-link>
          的 Cloudflare 出口中维护。
        </n-alert>
        <p v-else class="config-hint">名称、主机名与回源服务可在右上角「编辑基础配置」里调整；路由在下方「路由」卡片维护。</p>
      </div>
    </HavlineCard>

    <HavlineCard class="section-card" title="路由" subtitle="流量通过以下路由到应用程序和服务">
      <div class="section-body">
        <div class="topology-canvas" :style="{ height: `${topologyHeight}px` }">
          <div class="topology-graph">
            <div
              v-for="(route, index) in routeData?.routes || []"
              :key="'domain-' + route.hostname"
              class="topology-domain"
              :style="{ top: `${routeY(index)}px` }"
            >
              <div class="topology-node-copy">
                <span class="topology-domain__name mono">{{ route.hostname }}</span>
              </div>
              <n-tag size="small" type="info" :bordered="false" round>已发布的应用程序</n-tag>
            </div>
            <n-button
              v-if="!tunnel.managed"
              class="topology-add"
              size="small"
              :style="{ top: `${addRouteY}px` }"
              @click="openAddRoute"
            >
              <template #icon><n-icon :component="AddOutline" /></template>
              添加路由
            </n-button>
            <div class="topology-exit" :style="{ top: `${centerY - 22}px` }">
              <n-icon :component="CloudOutline" />
              <div class="topology-node-copy">
                <strong>Cloudflare</strong>
              </div>
            </div>
            <div class="topology-tunnel" :style="{ top: `${centerY - 22}px` }">
              <n-icon :component="GitNetworkOutline" />
              <div class="topology-node-copy">
                <strong>{{ tunnel.name }}</strong>
              </div>
            </div>
            <div
              v-for="(service, index) in topologyServices"
              :key="'service-' + service"
              class="topology-service"
              :style="{ top: `${serviceY(index)}px` }"
            >
              <n-icon :component="ServerOutline" />
              <div class="topology-node-copy">
                <span class="mono">{{ service }}</span>
              </div>
            </div>
            <svg class="topology-lines" :viewBox="`0 0 ${TOPOLOGY_WIDTH} ${topologyHeight}`">
              <template v-for="(route, index) in routeData?.routes || []" :key="'line-' + route.hostname">
                <path :d="domainToExitPath(routeY(index) + 22)" />
                <path :d="exitToTunnelPath" />
                <path :d="tunnelToServicePath(serviceY(topologyServices.indexOf(route.service)) + 22)" />
              </template>
              <path v-if="!tunnel.managed" :d="domainToExitPath(addRouteY + 17)" />
            </svg>
          </div>
        </div>
        <div v-if="routeData?.routes.length" class="routes-table">
          <div class="routes-table__head">
            <span>顺序</span>
            <span>目标</span>
            <span>路径</span>
            <span>服务</span>
            <span>来源</span>
            <span>操作</span>
          </div>
          <div v-for="(route, index) in routeData.routes" :key="route.hostname + route.service" class="routes-table__row">
            <span>{{ index + 1 }}</span>
            <span class="mono">{{ route.hostname }}</span>
            <span class="mono">{{ route.path }}</span>
            <span class="mono">{{ route.service }}</span>
            <n-tag size="small" :bordered="false" round>{{ route.source === 'rule' ? '反向代理规则' : 'config.yml' }}</n-tag>
            <div class="routes-table__actions">
              <!-- 原来一行平铺 6 个按钮，把这列撑到 360px；改为主操作「编辑」+「更多」 -->
              <n-button size="tiny" :disabled="tunnel.managed" @click="openEditRoute(route, index)">编辑</n-button>
              <n-dropdown trigger="click" :options="routeActionOptions(index)" @select="handleRouteAction($event, route, index)">
                <n-button size="tiny" quaternary circle title="更多操作">
                  <template #icon><n-icon :component="EllipsisHorizontalOutline" /></template>
                </n-button>
              </n-dropdown>
            </div>
          </div>
        </div>
        <EmptyState
          v-else
          title="还没有已发布路由"
          description="手工隧道可在基础配置中添加域名和源服务，托管隧道请到反向代理规则维护。"
        >
          <template v-if="!tunnel.managed" #action>
            <n-button type="primary" @click="openAddRoute">添加路由</n-button>
          </template>
        </EmptyState>
        <div class="routes-catchall">
          <span>全部捕获规则：</span>
          <strong class="mono">{{ routeData?.catch_all || 'http_status:404' }}</strong>
        </div>
      </div>
    </HavlineCard>

  </template>

  <n-modal v-model:show="basicVisible" preset="card" :style="{ width: 'min(720px, 96vw)' }" title="基础配置">
    <n-form label-placement="top" class="config-form">
      <n-form-item label="名称">
        <n-input v-model:value="form.name" />
      </n-form-item>
      <div class="config-form__grid">
        <n-form-item label="子域名（可选）">
          <n-input v-model:value="form.subdomain" placeholder="例如 999" />
        </n-form-item>
        <n-form-item label="域">
          <n-select
            v-model:value="form.domain"
            :options="zoneOptions"
            filterable
            tag
            placeholder="从 Cloudflare Zone 选择"
          />
        </n-form-item>
      </div>
      <div class="hostname-preview">
        <span>完整主机名：</span>
        <strong class="mono">{{ basicHostname() || '未选择域' }}</strong>
      </div>
      <n-form-item label="服务">
        <div class="config-origin">
          <n-select v-model:value="form.type" :options="originTypeOptions" class="config-origin__type" />
          <span class="config-origin__scheme">://</span>
          <n-input v-model:value="form.target" placeholder="192.168.11.30:8001" />
        </div>
      </n-form-item>
      <div class="config-form__footer">
        <n-switch v-model:value="form.autoStart" />
        <span>随 Havline 自动启动</span>
      </div>
    </n-form>
    <template #footer>
      <div class="config-modal__footer">
        <n-button @click="basicVisible = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="save">保存配置</n-button>
      </div>
    </template>
  </n-modal>

  <n-modal v-model:show="routeVisible" preset="card" :style="{ width: 'min(620px, 96vw)' }" :title="routeEditingIndex === null ? '添加路由' : '编辑路由'">
    <n-form label-placement="top" class="config-form">
      <div class="config-form__grid">
        <n-form-item label="子域名（可选）">
          <n-input v-model:value="routeForm.subdomain" placeholder="例如 999" />
        </n-form-item>
        <n-form-item label="域">
          <n-select
            v-model:value="routeForm.domain"
            :options="zoneOptions"
            filterable
            tag
            placeholder="从 Cloudflare Zone 选择"
          />
        </n-form-item>
      </div>
      <div class="hostname-preview">
        <span>完整主机名：</span>
        <strong class="mono">{{ routeHostname() || '未选择域' }}</strong>
      </div>
      <n-form-item label="服务">
        <div class="config-origin">
          <n-select v-model:value="routeForm.type" :options="originTypeOptions" class="config-origin__type" />
          <span class="config-origin__scheme">://</span>
          <n-input v-model:value="routeForm.target" placeholder="192.168.11.30:8001" />
        </div>
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="config-modal__footer">
        <n-button @click="routeVisible = false">取消</n-button>
        <n-button type="primary" :loading="routeSaving" @click="saveRoute">保存路由</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NDropdown, NForm, NFormItem, NIcon, NInput, NModal, NSelect, NSwitch, NTag, useDialog, useMessage, type DropdownOption } from 'naive-ui'
import { AddOutline, ArrowBackOutline, CloudOutline, EllipsisHorizontalOutline, GitNetworkOutline, LayersOutline, PulseOutline, ServerOutline } from '@vicons/ionicons5'
import { api, asList } from '../api/client'
import type { CfRouteView, CfRuntimeStatus, CfTunnel, CfTunnelPayload, CfTunnelRoutes, CfZone } from '../api/types'
import HavlineCard from '../components/HavlineCard.vue'
import EmptyState from '../components/EmptyState.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import StatCard from '../components/StatCard.vue'
import StatusBadge from '../components/StatusBadge.vue'
import type { StatusKind } from '../utils/status'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const tunnel = ref<CfTunnel | null>(null)
const status = ref<CfRuntimeStatus | null>(null)
const routeData = ref<CfTunnelRoutes | null>(null)
const zones = ref<CfZone[]>([])
const loading = ref(false)
const saving = ref(false)
// 进程操作（启动 / 停止 / 重启）进行中：按钮显示 loading 并互相禁用
const actionRunning = ref<'启动' | '停止' | '重启' | null>(null)
const basicVisible = ref(false)
const routeVisible = ref(false)
const routeSaving = ref(false)
const routeEditingIndex = ref<number | null>(null)
const routeForm = ref({ subdomain: '', domain: '', type: 'http', target: '' })
const loadError = ref('')
const form = ref({ name: '', subdomain: '', domain: '', type: 'http', target: '', autoStart: false })
const TOPOLOGY_WIDTH = 880
const TOPOLOGY_ROW_HEIGHT = 54
const TOPOLOGY_START_Y = 38

const tunnelID = computed(() => Number(route.params.id))
const runtimeState = computed(() => status.value?.status || tunnel.value?.status || 'stopped')
const zoneOptions = computed(() => zones.value.map((zone) => ({ label: zone.name, value: zone.name })))
const originTypeOptions = [
  { label: 'HTTP', value: 'http' },
  { label: 'HTTPS', value: 'https' },
  { label: 'TCP', value: 'tcp' },
  { label: 'SSH', value: 'ssh' },
]
const topologyServices = computed(() => {
  const values = new Set((routeData.value?.routes || []).map((route) => route.service))
  return [...values]
})
const addRouteY = computed(() => TOPOLOGY_START_Y + (routeData.value?.routes.length ?? 0) * TOPOLOGY_ROW_HEIGHT)
const topologyHeight = computed(() => Math.max(240, Math.max(addRouteY.value + 70, topologyServices.value.length * TOPOLOGY_ROW_HEIGHT + TOPOLOGY_START_Y + 30)))
const centerY = computed(() => topologyHeight.value / 2)

function routeY(index: number) {
  return TOPOLOGY_START_Y + index * TOPOLOGY_ROW_HEIGHT
}

function serviceY(index: number) {
  const count = Math.max(1, topologyServices.value.length)
  const total = (count - 1) * TOPOLOGY_ROW_HEIGHT
  return topologyHeight.value / 2 - total / 2 + index * TOPOLOGY_ROW_HEIGHT - 22
}

function domainToExitPath(sourceY: number) {
  return `M 280 ${sourceY} C 295 ${sourceY}, 305 ${centerY.value}, 320 ${centerY.value}`
}

function exitToTunnelPath() {
  return `M 420 ${centerY.value} L 460 ${centerY.value}`
}

function tunnelToServicePath(targetY: number) {
  return `M 570 ${centerY.value} C 585 ${centerY.value}, 598 ${targetY}, 610 ${targetY}`
}

async function load() {
  if (!Number.isFinite(tunnelID.value) || tunnelID.value <= 0) {
    loadError.value = '隧道 ID 无效'
    return
  }
  loading.value = true
  loadError.value = ''
  try {
    const [item, runtime, routes, zoneResult] = await Promise.all([
      api.getCfTunnel(tunnelID.value),
      api.getCfTunnelStatus(tunnelID.value),
      api.getCfTunnelRoutes(tunnelID.value),
      api.getCfZones().catch(() => ({ zones: [] as CfZone[] })),
    ])
    tunnel.value = item
    status.value = runtime
    routeData.value = routes
    zones.value = asList(zoneResult.zones)
    loadForm(item)
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '加载隧道配置失败'
  } finally {
    loading.value = false
  }
}

function loadForm(item: CfTunnel) {
  const hostname = item.hostnames?.[0] || ''
  const origin = item.origin_service || 'http://127.0.0.1:80'
  const [type, target = ''] = origin.split('://')
  const zone = zones.value
    .filter((entry) => hostname === entry.name || hostname.endsWith('.' + entry.name))
    .sort((a, b) => b.name.length - a.name.length)[0]
  form.value = {
    name: item.name,
    subdomain: zone ? hostname.slice(0, -(zone.name.length + 1)) : '',
    domain: zone?.name || hostname,
    type: type || 'http',
    target: target || '127.0.0.1:80',
    autoStart: item.auto_start,
  }
}

async function save() {
  if (!tunnel.value || tunnel.value.managed) return
  const hostname = manualHostname()
  if (!hostname || !form.value.target.trim()) {
    message.warning('请填写域名和源服务')
    return
  }
  const payload: CfTunnelPayload = {
    name: form.value.name.trim(),
    mode: tunnel.value.mode,
    token: '',
    hostnames: [hostname],
    origin_service: `${form.value.type}://${form.value.target.trim().replace(/^[a-z]+:\/\//i, '')}`,
    network: tunnel.value.network,
    auto_start: form.value.autoStart,
    managed: false,
  }
  saving.value = true
  try {
    const result = await api.updateCfTunnel(tunnel.value.id, payload)
    tunnel.value = result
    if (result.dns_warning) message.warning(result.dns_warning)
    else message.success('配置已保存')
    basicVisible.value = false
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    saving.value = false
  }
}

function openAddRoute() {
  routeEditingIndex.value = null
  routeForm.value = { subdomain: '', domain: zones.value[0]?.name || '', type: 'http', target: '127.0.0.1:80' }
  routeVisible.value = true
}

function openEditRoute(route: CfRouteView, index: number) {
  const [type, target = ''] = route.service.split('://')
  const zone = zones.value
    .filter((entry) => route.hostname === entry.name || route.hostname.endsWith('.' + entry.name))
    .sort((a, b) => b.name.length - a.name.length)[0]
  routeEditingIndex.value = index
  routeForm.value = {
    subdomain: zone ? route.hostname.slice(0, -(zone.name.length + 1)) : '',
    domain: zone?.name || route.hostname,
    type: type || 'http',
    target,
  }
  routeVisible.value = true
}

async function saveRoute() {
  if (!tunnel.value || !routeData.value) return
  const hostname = routeHostname()
  const target = routeForm.value.target.trim().replace(/^[a-z]+:\/\//i, '')
  if (!hostname || !target) {
    message.warning('请填写目标域名和源服务')
    return
  }
  const next = routeData.value.routes.map((route) => ({
    hostname: route.hostname,
    path: route.path || '*',
    service: route.service,
  }))
  const item = { hostname, path: '*', service: `${routeForm.value.type}://${target}` }
  if (routeEditingIndex.value === null) next.push(item)
  else next[routeEditingIndex.value] = item

  routeSaving.value = true
  try {
    routeData.value = await api.replaceCfTunnelRoutes(tunnel.value.id, next)
    routeVisible.value = false
    message.success('路由已保存')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存路由失败')
  } finally {
    routeSaving.value = false
  }
}

// 路由行的「更多」菜单：托管隧道与首/末行按情况禁用，避免点了才发现不可用
function routeActionOptions(index: number): DropdownOption[] {
  const locked = tunnel.value?.managed ?? false
  const isLast = index === ((routeData.value?.routes.length ?? 1) - 1)
  return [
    { label: '上移', key: 'up', disabled: locked || index === 0 },
    { label: '下移', key: 'down', disabled: locked || isLast },
    { type: 'divider', key: 'd1' },
    { label: '复制主机名与服务', key: 'copy', disabled: locked },
    { label: '测试连通性', key: 'test' },
    { type: 'divider', key: 'd2' },
    { label: '删除路由', key: 'delete', disabled: locked },
  ]
}

function handleRouteAction(key: string | number, route: CfRouteView, index: number) {
  if (key === 'up') return void moveRoute(index, -1)
  if (key === 'down') return void moveRoute(index, 1)
  if (key === 'copy') return void copyRoute(route)
  if (key === 'test') return void testRoute(route)
  if (key === 'delete') confirmDeleteRoute(route, index)
}

// 删除走对话框二次确认，替代原来包在行内的 n-popconfirm（菜单里无法嵌套 Popconfirm）
function confirmDeleteRoute(route: CfRouteView, index: number) {
  dialog.warning({
    title: '删除路由',
    content: `删除路由 ${route.hostname}？保存后会同步到 Cloudflare。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: () => deleteRoute(index),
  })
}

async function deleteRoute(index: number) {
  if (!tunnel.value || !routeData.value || tunnel.value.managed) return
  const next = routeData.value.routes
    .filter((_, routeIndex) => routeIndex !== index)
    .map((route) => ({
      hostname: route.hostname,
      path: route.path || '*',
      service: route.service,
    }))
  try {
    routeData.value = await api.replaceCfTunnelRoutes(tunnel.value.id, next)
    message.success('路由已删除')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '删除路由失败')
  }
}

async function moveRoute(index: number, delta: number) {
  if (!tunnel.value || !routeData.value || tunnel.value.managed) return
  const target = index + delta
  if (target < 0 || target >= routeData.value.routes.length) return
  const routes = routeData.value.routes.map((route) => ({
    hostname: route.hostname,
    path: route.path || '*',
    service: route.service,
  }))
  ;[routes[index], routes[target]] = [routes[target], routes[index]]
  try {
    routeData.value = await api.replaceCfTunnelRoutes(tunnel.value.id, routes)
    message.success('路由顺序已更新')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '调整顺序失败')
  }
}

function copyRoute(route: CfRouteView) {
  openAddRoute()
  const [type, target = ''] = route.service.split('://')
  routeForm.value = {
    subdomain: '',
    domain: zones.value.find((zone) => route.hostname.endsWith('.' + zone.name))?.name || route.hostname,
    type: type || 'http',
    target,
  }
}

async function testRoute(route: CfRouteView) {
  try {
    const result = await api.testCfRoute(route.service)
    if (result.ok) message.success(`${route.hostname}：${result.message}，${result.latency_ms} ms`)
    else message.error(`${route.hostname}：${result.message}`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '测试失败')
  }
}

function routeHostname() {
  const subdomain = routeForm.value.subdomain.trim().replace(/^\.+|\.+$/g, '')
  const domain = routeForm.value.domain.trim().replace(/^\.+|\.+$/g, '')
  if (!domain) return ''
  return (subdomain ? `${subdomain}.${domain}` : domain).toLowerCase()
}

function basicHostname() {
  const subdomain = form.value.subdomain.trim().replace(/^\.+|\.+$/g, '')
  const domain = form.value.domain.trim().replace(/^\.+|\.+$/g, '')
  if (!domain) return ''
  return (subdomain ? `${subdomain}.${domain}` : domain).toLowerCase()
}

function manualHostname() {
  const subdomain = form.value.subdomain.trim().replace(/^\.+|\.+$/g, '')
  const domain = form.value.domain.trim().replace(/^\.+|\.+$/g, '')
  if (!domain) return ''
  return subdomain ? `${subdomain}.${domain}` : domain
}

async function runAction(action: '启动' | '停止' | '重启') {
  if (!tunnel.value) return
  actionRunning.value = action
  try {
    if (action === '启动') await api.startCfTunnel(tunnel.value.id)
    if (action === '停止') await api.stopCfTunnel(tunnel.value.id)
    if (action === '重启') await api.restartCfTunnel(tunnel.value.id)
    message.success(`${action}指令已下发`)
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : `${action}失败`)
  } finally {
    actionRunning.value = null
  }
}

function statusText(value: string) {
  return ({
    connected: '已连接',
    starting: '启动中',
    reconnecting: '重连中',
    failed: '失败',
    stopped: '已停止',
  } as Record<string, string>)[value] || value || '未知'
}

function statusKind(value: string): StatusKind {
  if (value === 'connected') return 'success'
  if (value === 'starting' || value === 'reconnecting') return 'warning'
  if (value === 'failed') return 'error'
  return 'unknown'
}

onMounted(load)
</script>

<style scoped>
.stats-row {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--havline-space-4);
  margin-bottom: var(--havline-space-4);
}
.section-card + .section-card { margin-top: var(--havline-space-4); }
/* 基础配置：把原来空着的一张卡换成关键字段网格 */
.config-facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--havline-space-3) var(--havline-space-4);
}
.config-fact { display: grid; gap: 4px; min-width: 0; }
.config-fact label { font-size: 12px; color: var(--havline-text-secondary); }
.config-fact span { font-size: 13px; color: var(--havline-text); overflow-wrap: anywhere; }
.config-managed-hint { margin-top: var(--havline-space-3); }
.config-hint { margin: var(--havline-space-3) 0 0; font-size: 12px; color: var(--havline-text-muted); }
.topology-canvas {
  position: relative;
  width: 100%;
  margin-bottom: var(--havline-space-4);
  overflow-x: auto;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background-color: var(--havline-bg);
  background-image: radial-gradient(var(--havline-border) 0.8px, transparent 0.8px);
  background-size: 10px 10px;
}
.topology-graph {
  position: relative;
  width: 880px;
  height: 100%;
  margin: 0 auto;
}
.topology-domain {
  position: absolute;
  left: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 8px;
  width: 280px;
  height: 44px;
}
.topology-domain__name {
  white-space: nowrap;
  color: var(--havline-brand-text);
  text-decoration: underline;
}
.topology-node-copy {
  display: grid;
  gap: 2px;
  min-width: 0;
}
.topology-lines {
  position: absolute;
  inset: 0;
  z-index: 1;
  width: 880px;
  height: 100%;
  pointer-events: none;
}
.topology-lines path {
  fill: none;
  stroke: var(--havline-border);
  stroke-width: 1.5;
}
.topology-add {
  position: absolute;
  left: 0;
  z-index: 2;
  width: 150px;
}
.topology-exit,
.topology-tunnel {
  position: absolute;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 14px;
  border: 1px solid var(--havline-border);
  border-radius: 999px;
  background: var(--havline-surface);
  box-shadow: var(--havline-shadow);
  white-space: nowrap;
}
.topology-exit { left: 320px; min-width: 100px; }
.topology-tunnel { left: 460px; min-width: 110px; }
.topology-service {
  position: absolute;
  left: 610px;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 8px;
  width: max-content;
  min-width: 260px;
  height: 44px;
  padding: 7px 8px;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-surface);
  color: var(--havline-text-secondary);
  font-size: 12px;
  white-space: nowrap;
}
.topology-service .mono { white-space: nowrap; }
.topology-service :deep(.n-icon) { flex: none; color: var(--havline-brand); }
.config-form { max-width: 780px; }
.config-form__grid { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 12px; }
.config-origin {
  display: grid;
  grid-template-columns: 128px auto minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  width: 100%;
}
.config-origin__type { width: 128px; }
.config-origin__scheme {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  color: var(--havline-text-muted);
  white-space: nowrap;
}
.config-form__footer { display: flex; align-items: center; gap: 10px; font-size: 13px; }
.config-form__spacer { flex: 1; }
.config-modal__footer { display: flex; justify-content: flex-end; gap: 8px; }
.hostname-preview {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 var(--havline-space-4);
  padding: 10px 12px;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
}
.hostname-preview span { color: var(--havline-text-muted); font-size: 12px; }
.hostname-preview strong { color: var(--havline-text); }
.routes-table { overflow-x: auto; }
.routes-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--havline-space-3);
  margin-bottom: var(--havline-space-3);
}
.routes-table__head,
.routes-table__row {
  display: grid;
  /* 操作列从 360px 收到 120px（编辑 + 更多），窄屏不必再横向滚动才能点到操作 */
  grid-template-columns: 56px minmax(180px, 1fr) 90px minmax(200px, 1.2fr) 130px 120px;
  gap: 12px;
  align-items: center;
  min-width: 860px;
  padding: 10px 0;
}
.routes-table__actions { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; }
.routes-table__head { color: var(--havline-text-muted); font-size: 12px; border-bottom: 1px solid var(--havline-border); }
.routes-table__row { border-bottom: 1px solid var(--havline-border); }
.routes-table__row:last-child { border-bottom: none; }
.routes-empty { padding: 20px 0; color: var(--havline-text-muted); }
.routes-catchall { margin-top: 12px; padding: 10px 12px; background: var(--havline-bg); border-radius: var(--havline-radius-sm); font-size: 13px; }
@media (max-width: 900px) {
  .stats-row { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .config-form__grid { grid-template-columns: 1fr; }
}
@media (max-width: 760px) {
  .stats-row { grid-template-columns: 1fr; }
}
</style>
