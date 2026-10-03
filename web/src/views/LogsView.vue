<template>
  <PageHeader
    title="日志中心"
    description="系统日志记录 Havline 全部运行输出；访问日志与 Nginx 错误日志来自反向代理"
  />

  <LoadError v-if="pageError" :message="pageError" @retry="loadAll" />

  <HavlineCard v-else flush class="logs-card">
    <n-tabs v-model:value="tab" type="line" animated class="logs-tabs">
      <n-tab-pane name="system" tab="系统日志">
        <LogToolbar
          v-model:keyword="systemKeyword"
          v-model:level="systemLevel"
          :show-level="true"
          @refresh="loadSystemLogs"
        />
        <p class="tab-hint">Havline 全部运行日志（API、DDNS、证书、通知、Nginx 重载等），不含 Nginx 访问与错误文件。</p>
        <n-data-table
          v-if="filteredSystemLogs.length > 0"
          class="log-table"
          :columns="systemColumns"
          :data="pagedSystemLogs"
          :loading="loadingSystem"
          :bordered="false"
          :row-class-name="systemRowClassName"
        />
        <LogPagination
          v-if="filteredSystemLogs.length > 0"
          v-model:page="systemPage"
          :item-count="filteredSystemLogs.length"
        />
        <EmptyState
          v-if="!loadingSystem && systemLogs.length === 0"
          title="暂无系统日志"
          description="应用启动、DDNS 更新、证书操作或 API 请求会记录在这里。"
        />
      </n-tab-pane>

      <n-tab-pane name="frp" tab="FRP 日志">
        <LogToolbar
          v-model:keyword="frpKeyword"
          v-model:level="frpLevel"
          :show-level="true"
          @refresh="loadFrpLogs"
        />
        <p class="tab-hint">frpc 运行日志：解析为 时间 / 级别 / 来源 / 消息，用于排查隧道登录、注册与连接错误。</p>
        <n-data-table
          v-if="filteredFrpLogs.length > 0"
          class="log-table"
          :columns="frpColumns"
          :data="pagedFrpLogs"
          :loading="loadingFrp"
          :bordered="false"
          :row-class-name="frpRowClassName"
        />
        <LogPagination
          v-if="filteredFrpLogs.length > 0"
          v-model:page="frpPage"
          :item-count="filteredFrpLogs.length"
        />
        <EmptyState
          v-if="!loadingFrp && filteredFrpLogs.length === 0"
          :title="frpLogs.length === 0 ? '暂无 FRP 日志' : '没有匹配的日志'"
          :description="frpLogs.length === 0 ? '启动 frpc 后，登录、隧道注册与连接错误会记录在这里。' : '换个关键词或级别再试。'"
        />
      </n-tab-pane>

      <n-tab-pane name="cloudflare" tab="Cloudflare 隧道">
        <LogToolbar v-model:keyword="cfKeyword" @refresh="loadCfLogs" />
        <div class="remote-nginx-bar">
          <n-select
            v-model:value="cfTunnelId"
            :options="cfTunnelOptions"
            placeholder="选择 Cloudflare 隧道"
            size="small"
            class="remote-nginx-bar__server"
          />
          <n-select v-model:value="cfLines" :options="cfLineOptions" size="small" style="width: 110px" />
        </div>
        <pre v-if="filteredCfLogLines.length" class="cf-log-pre">{{ filteredCfLogLines.join('\n') }}</pre>
        <EmptyState
          v-else-if="!cfLoading"
          title="暂无 Cloudflare 隧道日志"
          :description="cfError || '选择隧道并启动 cloudflared 后，连接与错误日志会显示在这里。'"
        />
      </n-tab-pane>

      <n-tab-pane name="remote-nginx" tab="公网 Nginx">
        <LogToolbar v-model:keyword="nginxKeyword" @refresh="loadRemoteNginxLogs" />
        <div class="remote-nginx-bar">
          <n-select
            v-model:value="nginxTargetId"
            :options="nginxTargetOptions"
            placeholder="选择服务端"
            size="small"
            class="remote-nginx-bar__server"
          />
          <n-select v-model:value="nginxKind" :options="nginxKindOptions" size="small" style="width: 120px" />
          <n-select v-model:value="nginxLines" :options="nginxLineOptions" size="small" style="width: 110px" />
          <span v-if="nginxPath" class="tab-hint mono remote-nginx-bar__path">
            {{ nginxPath }}{{ nginxTruncated ? '（仅显示尾部）' : '' }}
          </span>
        </div>
        <n-data-table
          v-if="filteredNginxLines.length > 0"
          class="log-table"
          :columns="nginxColumns"
          :data="pagedNginxLines"
          :loading="nginxLoading"
          :bordered="false"
        />
        <LogPagination
          v-if="filteredNginxLines.length > 0"
          v-model:page="nginxPage"
          :item-count="filteredNginxLines.length"
        />
        <EmptyState
          v-if="!nginxLoading && filteredNginxLines.length === 0"
          title="暂无公网 Nginx 日志"
          :description="nginxError || 'VPS 上产生访问后，这里会显示 Nginx 的访问 / 错误日志（需 agent ≥ 0.10.0）。'"
        />
      </n-tab-pane>

      <n-tab-pane name="access" tab="访问日志">
        <LogToolbar
          v-model:keyword="accessKeyword"
          v-model:status="accessStatus"
          :show-domain="true"
          :show-status="true"
          :auto-refresh="autoRefresh"
          @refresh="loadAccess"
          @toggle-auto="autoRefresh = !autoRefresh"
        />
        <n-data-table
          v-if="filteredAccessLogs.length > 0"
          class="log-table"
          :columns="accessColumns"
          :data="pagedAccessLogs"
          :loading="loadingAccess"
          :bordered="false"
          :scroll-x="920"
          :row-class-name="accessRowClassName"
        />
        <LogPagination
          v-if="filteredAccessLogs.length > 0"
          v-model:page="accessPage"
          :item-count="filteredAccessLogs.length"
        />
        <EmptyState
          v-if="!loadingAccess && accessLogs.length === 0"
          title="暂无访问日志"
          description="此处记录经 Nginx 反向代理的访问（非 Havline 管理界面本身）。请通过代理域名访问后刷新。"
        />
      </n-tab-pane>

      <n-tab-pane name="nginx" tab="Nginx 日志">
        <p class="tab-hint">Nginx 错误日志（error.log），记录 SSL 握手失败、上游连接异常、配置冲突等。</p>
        <LogToolbar v-model:keyword="errorKeyword" @refresh="loadErrorLogs" />
        <n-data-table
          v-if="filteredErrorLogs.length > 0"
          class="log-table"
          :columns="errorColumns"
          :data="pagedErrorLogs"
          :loading="loadingError"
          :bordered="false"
          :row-class-name="errorRowClassName"
        />
        <LogPagination
          v-if="filteredErrorLogs.length > 0"
          v-model:page="errorPage"
          :item-count="filteredErrorLogs.length"
        />
        <EmptyState
          v-if="!loadingError && errorLogs.length === 0"
          title="暂无 Nginx 错误日志"
          description="Nginx 出现 SSL、上游或配置相关错误时会记录在这里。"
        />
      </n-tab-pane>
    </n-tabs>
  </HavlineCard>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  NButton,
  NDataTable,
  NInput,
  NPagination,
  NSelect,
  NTabPane,
  NTabs,
  NTag,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import { api, asList } from '../api/client'
import { useVisibilityPolling } from '../composables/useVisibilityPolling'
import type { AccessLogEntry, CfTunnel, ProxyRule, SystemLogEntry } from '../api/types'
import EmptyState from '../components/EmptyState.vue'
import HavlineCard from '../components/HavlineCard.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import {
  accessServiceTooltip,
  buildProxyBindingIndex,
  formatAccessFallback,
  resolveAccessServiceLabel,
} from '../utils/accessService'
import { formatLogTime, formatMs } from '../utils/format'
import {
  latencyClass,
  nginxLevelTagType,
  parseNginxErrorLine,
  type ParsedNginxError,
  displaySystemLog,
  systemLevelTagType,
  systemModuleTagType,
} from '../utils/logDisplay'
import { httpStatusKind } from '../utils/status'

const PAGE_SIZE = 20

const LogPagination = defineComponent({
  name: 'LogPagination',
  props: {
    page: { type: Number, required: true },
    itemCount: { type: Number, required: true },
  },
  emits: ['update:page'],
  setup(props, { emit }) {
    return () =>
      h('div', { class: 'log-pagination' }, [
        h(NPagination, {
          page: props.page,
          pageSize: PAGE_SIZE,
          itemCount: props.itemCount,
          showSizePicker: false,
          'onUpdate:page': (p: number) => emit('update:page', p),
        }),
      ])
  },
})

const LogToolbar = defineComponent({
  name: 'LogToolbar',
  props: {
    keyword: { type: String, default: '' },
    status: { type: Number as () => number | null, default: null },
    level: { type: String, default: '' },
    showDomain: Boolean,
    showStatus: Boolean,
    showLevel: Boolean,
    autoRefresh: Boolean,
  },
  emits: ['update:keyword', 'update:status', 'update:level', 'refresh', 'toggle-auto'],
  setup(props, { emit }) {
    return () =>
      h('div', { class: 'toolbar' }, [
        h(NInput, {
          value: props.keyword,
          placeholder: '关键词搜索',
          clearable: true,
          style: 'max-width: 220px',
          'onUpdate:value': (v: string) => emit('update:keyword', v),
        }),
        props.showStatus
          ? h(NSelect, {
              value: props.status,
              placeholder: '状态码',
              clearable: true,
              style: 'width: 120px',
              options: [200, 301, 400, 403, 404, 500].map((c) => ({ label: String(c), value: c })),
              'onUpdate:value': (v: number | null) => emit('update:status', v),
            })
          : null,
        props.showLevel
          ? h(NSelect, {
              value: props.level,
              placeholder: '日志级别',
              clearable: true,
              style: 'width: 120px',
              options: [
                { label: '信息', value: 'INFO' },
                { label: '警告', value: 'WARN' },
                { label: '错误', value: 'ERROR' },
              ],
              'onUpdate:value': (v: string) => emit('update:level', v),
            })
          : null,
        h(NButton, { onClick: () => emit('refresh') }, () => '刷新'),
        props.autoRefresh !== undefined
          ? h(NButton, { quaternary: true, onClick: () => emit('toggle-auto') }, () =>
              props.autoRefresh ? '关闭自动刷新' : '自动刷新',
            )
          : null,
      ])
  },
})

const message = useMessage()
const route = useRoute()
const LOG_TABS = ['system', 'frp', 'cloudflare', 'remote-nginx', 'access', 'nginx'] as const
type LogTab = (typeof LOG_TABS)[number]

function resolveTab(queryTab: unknown): LogTab {
  if (queryTab === 'error' || queryTab === 'nginx') return 'nginx'
  if (queryTab === 'stream') return 'access'
  if (typeof queryTab === 'string' && (LOG_TABS as readonly string[]).includes(queryTab)) {
    return queryTab as LogTab
  }
  return 'system'
}

const tab = ref(resolveTab(route.query.tab))
const pageError = ref('')
const autoRefresh = ref(false)

const accessLogs = ref<AccessLogEntry[]>([])
const proxyRules = ref<ProxyRule[]>([])
const proxyBindingIndex = computed(() => buildProxyBindingIndex(proxyRules.value))
const errorLogs = ref<ParsedNginxError[]>([])
const systemLogs = ref<SystemLogEntry[]>([])
const loadingAccess = ref(false)
const loadingError = ref(false)
const loadingSystem = ref(false)

const accessKeyword = ref('')
const accessStatus = ref<number | null>(null)
const errorKeyword = ref('')
const systemKeyword = ref('')
const systemLevel = ref('')

const accessPage = ref(1)
const errorPage = ref(1)
const systemPage = ref(1)

const filteredAccessLogs = computed(() => {
  return accessLogs.value.filter((log) => {
    if (accessKeyword.value) {
      const kw = accessKeyword.value.toLowerCase()
      const service = resolveAccessServiceLabel(log, proxyBindingIndex.value)
      const fallback = formatAccessFallback(log)
      if (!`${service} ${fallback} ${log.domain} ${log.path} ${log.client_ip}`.toLowerCase().includes(kw)) return false
    }
    if (accessStatus.value && log.status !== accessStatus.value) return false
    return true
  })
})

const filteredErrorLogs = computed(() => {
  if (!errorKeyword.value) return errorLogs.value
  const kw = errorKeyword.value.toLowerCase()
  return errorLogs.value.filter((entry) => entry.raw.toLowerCase().includes(kw))
})

const filteredSystemLogs = computed(() => {
  return systemLogs.value.filter((log) => {
    const display = displaySystemLog(log)
    if (systemLevel.value && log.level.toUpperCase() !== systemLevel.value) return false
    if (
      systemKeyword.value &&
      !`${display.displayModule} ${log.message}`.toLowerCase().includes(systemKeyword.value.toLowerCase())
    ) {
      return false
    }
    return true
  })
})

function paginate<T>(items: T[], page: number) {
  const start = (page - 1) * PAGE_SIZE
  return items.slice(start, start + PAGE_SIZE)
}

const pagedAccessLogs = computed(() => paginate(filteredAccessLogs.value, accessPage.value))
const pagedErrorLogs = computed(() => paginate(filteredErrorLogs.value, errorPage.value))
const pagedSystemLogs = computed(() => paginate(filteredSystemLogs.value, systemPage.value))

const methodTagType = (method: string) => {
  const map: Record<string, 'success' | 'info' | 'warning' | 'error' | 'default'> = {
    GET: 'success',
    POST: 'info',
    PUT: 'warning',
    DELETE: 'error',
  }
  return map[method] ?? 'default'
}

const statusTagType = (code: number) => {
  const kind = httpStatusKind(code)
  if (kind === 'success') return 'success'
  if (kind === 'warning') return 'warning'
  if (kind === 'error') return 'error'
  return 'info'
}

const logTimeCell = (time: string) =>
  h('span', { class: 'log-time' }, formatLogTime(time))

function accessRowClassName(row: AccessLogEntry) {
  if (row.status >= 500) return 'log-row log-row--error'
  if (row.status >= 400) return 'log-row log-row--warn'
  return 'log-row'
}

function errorRowClassName(row: ParsedNginxError) {
  const type = nginxLevelTagType(row.level)
  if (type === 'error') return 'log-row log-row--error'
  if (type === 'warning') return 'log-row log-row--warn'
  return 'log-row'
}

function systemRowClassName(row: SystemLogEntry) {
  const display = displaySystemLog(row)
  if (display.displayLevel === '错误') return 'log-row log-row--error'
  if (display.displayLevel === '警告') return 'log-row log-row--warn'
  return 'log-row'
}

// FRP 日志（frpc）：后端返回原始行，前端解析为 时间 / 级别 / 来源 / 消息 四列
interface FrpLogEntry { time: string; level: string; source: string; message: string }

const frpLogs = ref<FrpLogEntry[]>([])
const frpKeyword = ref('')
const frpLevel = ref('')
const frpPage = ref(1)
const loadingFrp = ref(false)
const cfTunnels = ref<CfTunnel[]>([])
const cfTunnelId = ref<number | null>(null)
const cfLines = ref(200)
const cfKeyword = ref('')
const cfRawLog = ref('')
const cfLoading = ref(false)
const cfError = ref('')
const frpPageSize = 50
const frpLevelText: Record<string, string> = { I: '信息', W: '警告', E: '错误', D: '调试' }
// 工具条的级别选项值是 INFO / WARN / ERROR，而 frpc 的级别是单字母 I/W/E/D，需转换后再比较
const frpLevelAlias: Record<string, string> = { INFO: 'I', WARN: 'W', ERROR: 'E' }

// frpc 的「来源」是 Go 源码文件行号（如 client/control.go:174），对用户没有意义；
// 这里按文件前缀映射成中文模块名，认不出来的（例如回落到的组件名）原样保留。
const FRP_SOURCE_LABELS: { prefix: string; label: string }[] = [
  { prefix: 'client/control.go', label: '连接' },
  { prefix: 'client/service.go', label: '登录' },
  { prefix: 'client/proxy.go', label: '连接' },
  { prefix: 'client/', label: '客户端' },
  { prefix: 'proxy/proxy_manager.go', label: '代理' },
  { prefix: 'proxy/', label: '代理' },
  { prefix: 'sub/root.go', label: '服务' },
  { prefix: 'sub/', label: '服务' },
  { prefix: 'config/', label: '配置' },
  { prefix: 'health/', label: '健康检查' },
  { prefix: 'server/', label: '服务端' },
  { prefix: 'pkg/', label: '组件' },
]

function frpSourceLabel(source: string): string {
  const file = source.split(':')[0] ?? ''
  for (const item of FRP_SOURCE_LABELS) {
    if (file === item.prefix || file.startsWith(item.prefix)) return item.label
  }
  return file || source
}

// frpc 行有两种形态：
//   2026-09-23 15:20:31.123 [I] [service.go:295] login to server success
//   [web] 2026-09-20 19:38:51.771 [I] [client/control.go:174] [trace-id] [web-http] start proxy success
// 行首可能带一个 [组件] 前缀，来源优先取 [文件:行号]，缺失时回落为该组件名
function parseFrpLogLine(line: string): FrpLogEntry {
  const matched = line.match(
    /^(?:\[([^\]]+)\]\s+)?(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:\.\d+)?)\s*\[([A-Za-z])\]\s*(?:\[([^\]]+)\])?\s*(.*)$/,
  )
  if (!matched) return { time: '', level: '', source: '', message: line }
  const component = matched[1] ?? ''
  return {
    time: matched[2],
    level: matched[3].toUpperCase(),
    source: frpSourceLabel(matched[4] || component),
    message: matched[5] ?? '',
  }
}

const filteredFrpLogs = computed(() => {
  const keyword = frpKeyword.value.trim().toLowerCase()
  return frpLogs.value.filter((log) => {
    if (frpLevel.value && frpLevelAlias[frpLevel.value] !== log.level) return false
    if (keyword && !`${log.source} ${log.message}`.toLowerCase().includes(keyword)) return false
    return true
  })
})

const pagedFrpLogs = computed(() => {
  const start = (frpPage.value - 1) * frpPageSize
  return filteredFrpLogs.value.slice(start, start + frpPageSize)
})

// 与「系统日志」页签同一套渲染：时间走 logTimeCell，级别/来源用小号标签，消息单行省略并带 tooltip
const frpColumns: DataTableColumns<FrpLogEntry> = [
  { title: '时间', key: 'time', width: 168, render: (row) => logTimeCell(row.time) },
  {
    title: '级别',
    key: 'level',
    width: 80,
    render: (row) => {
      const label = frpLevelText[row.level] ?? (row.level || '—')
      return h(NTag, { size: 'small', bordered: false, type: systemLevelTagType(label) }, () => label)
    },
  },
  {
    title: '来源',
    key: 'source',
    width: 170,
    render: (row) =>
      h(
        NTag,
        {
          size: 'small',
          bordered: false,
          type: systemModuleTagType(row.source),
          class: 'log-module-tag',
        },
        () => row.source || '—',
      ),
  },
  {
    title: '消息',
    key: 'message',
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'log-message' }, row.message),
  },
]

function frpRowClassName(row: FrpLogEntry) {
  if (row.level === 'E') return 'log-row log-row--error'
  if (row.level === 'W') return 'log-row log-row--warn'
  return 'log-row'
}

const cfTunnelOptions = computed(() =>
  cfTunnels.value.map((tunnel) => ({
    label: `${tunnel.name}${tunnel.status === 'connected' ? '（已连接）' : ''}`,
    value: tunnel.id,
  })),
)

const cfLineOptions = [
  { label: '200 行', value: 200 },
  { label: '500 行', value: 500 },
  { label: '2000 行', value: 2000 },
]

const filteredCfLogLines = computed(() => {
  const keyword = cfKeyword.value.trim().toLowerCase()
  const lines = cfRawLog.value ? cfRawLog.value.split('\n') : []
  if (!keyword) return lines
  return lines.filter((line) => line.toLowerCase().includes(keyword))
})

async function loadCfTunnels() {
  try {
    cfTunnels.value = asList(await api.listCfTunnels())
  } catch {
    cfTunnels.value = []
  }
  if (cfTunnelId.value === null || !cfTunnels.value.some((tunnel) => tunnel.id === cfTunnelId.value)) {
    cfTunnelId.value = cfTunnels.value[0]?.id ?? null
  }
}

async function loadCfLogs() {
  if (cfTunnelId.value === null) {
    cfRawLog.value = ''
    cfError.value = '还没有 Cloudflare 隧道'
    return
  }
  cfLoading.value = true
  cfError.value = ''
  try {
    const result = await api.getCfTunnelLogs(cfTunnelId.value, cfLines.value)
    cfRawLog.value = result.output || ''
  } catch (error) {
    cfRawLog.value = ''
    cfError.value = error instanceof Error ? error.message : '读取 Cloudflare 日志失败'
  } finally {
    cfLoading.value = false
  }
}

watch([cfTunnelId, cfLines], () => {
  if (cfTunnelId.value !== null) void loadCfLogs()
})

// 公网 Nginx 日志（VPS 侧，需 agent ≥ 0.10.0）：路径由 agent 从 nginx -T 解析，前端只展示原始行
const nginxTargets = ref<import('../api/types').FrpServer[]>([])
const nginxTargetId = ref<number | null>(null)
const nginxKind = ref<'access' | 'error'>('access')
const nginxLines = ref(200)
const nginxRaw = ref<string[]>([])
const nginxPath = ref('')
const nginxTruncated = ref(false)
const nginxLoading = ref(false)
const nginxError = ref('')
const nginxKeyword = ref('')
const nginxPage = ref(1)
const nginxPageSize = 50

const nginxTargetOptions = computed(() =>
  nginxTargets.value.map((item) => ({ label: item.name || item.agent_url || `服务端 #${item.id}`, value: item.id })),
)

const nginxKindOptions = [
  { label: '访问日志', value: 'access' },
  { label: '错误日志', value: 'error' },
]

const nginxLineOptions = [
  { label: '200 行', value: 200 },
  { label: '500 行', value: 500 },
  { label: '2000 行', value: 2000 },
]

const filteredNginxLines = computed(() => {
  const rows = nginxRaw.value.map((line) => ({ line }))
  const keyword = nginxKeyword.value.trim().toLowerCase()
  if (!keyword) return rows
  return rows.filter((row) => row.line.toLowerCase().includes(keyword))
})

const pagedNginxLines = computed(() => {
  const start = (nginxPage.value - 1) * nginxPageSize
  return filteredNginxLines.value.slice(start, start + nginxPageSize)
})

const nginxColumns: DataTableColumns<{ line: string }> = [
  {
    title: '日志',
    key: 'line',
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'log-message' }, row.line),
  },
]

async function loadNginxTargets() {
  try {
    nginxTargets.value = asList(await api.listFrpServers()).filter((item) => item.agent_configured)
  } catch {
    nginxTargets.value = []
  }
  if (nginxTargetId.value === null || !nginxTargets.value.some((item) => item.id === nginxTargetId.value)) {
    nginxTargetId.value = nginxTargets.value[0]?.id ?? null
  }
}

async function loadRemoteNginxLogs() {
  if (nginxTargetId.value === null) {
    nginxRaw.value = []
    nginxError.value = '没有已配置 Agent 的服务端'
    return
  }
  nginxLoading.value = true
  nginxError.value = ''
  try {
    const result = await api.getFrpAgentNginxLogs(nginxTargetId.value, nginxKind.value, nginxLines.value)
    nginxRaw.value = result.lines ?? []
    nginxPath.value = result.path ?? ''
    nginxTruncated.value = result.truncated ?? false
    nginxPage.value = 1
  } catch (error) {
    nginxRaw.value = []
    nginxError.value = error instanceof Error ? error.message : '读取公网 Nginx 日志失败'
  } finally {
    nginxLoading.value = false
  }
}

watch([nginxTargetId, nginxKind, nginxLines], () => {
  nginxPage.value = 1
  if (nginxTargetId.value !== null) void loadRemoteNginxLogs()
})

watch(nginxKeyword, () => {
  nginxPage.value = 1
})

// 切到本页签时才拉服务端列表与日志，避免影响其它页签的首屏
watch(tab, (value) => {
  if (value === 'remote-nginx' && nginxTargets.value.length === 0) {
    void loadNginxTargets().then(() => loadRemoteNginxLogs())
  }
})

async function loadFrpLogs() {
  loadingFrp.value = true
  try {
    const raw = asList(await api.getFrpLogs(500))
    // 日志文件按时间升序返回，展示按时间倒序（新日期在前）：反转后分页与筛选也都跟着一致
    frpLogs.value = raw.map((line) => parseFrpLogLine(String(line))).reverse()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载 FRP 日志失败')
  } finally {
    loadingFrp.value = false
  }
}

const accessColumns = computed<DataTableColumns<AccessLogEntry>>(() => [
  { title: '时间', key: 'time', width: 168, render: (row) => logTimeCell(row.time) },
  {
    title: '服务',
    key: 'service',
    width: 128,
    render: (row) => {
      const tooltip = accessServiceTooltip(row, proxyBindingIndex.value)
      const label = resolveAccessServiceLabel(row, proxyBindingIndex.value)
      return h('span', { class: 'log-service nowrap', title: tooltip }, label)
    },
  },
  {
    title: '方法',
    key: 'method',
    width: 68,
    render: (row) => h(NTag, { size: 'small', bordered: false, type: methodTagType(row.method) }, () => row.method),
  },
  {
    title: '路径',
    key: 'path',
    width: 220,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'log-path mono nowrap', title: row.path }, row.path),
  },
  {
    title: '状态',
    key: 'status',
    width: 68,
    render: (row) => h(NTag, { size: 'small', bordered: false, type: statusTagType(row.status) }, () => String(row.status)),
  },
  {
    title: '耗时',
    key: 'response_time',
    width: 80,
    render: (row) => h('span', { class: latencyClass(row.response_time) }, formatMs(row.response_time)),
  },
  {
    title: '来源 IP',
    key: 'client_ip',
    width: 140,
    render: (row) => h('span', { class: 'mono nowrap', title: row.client_ip }, row.client_ip),
  },
])

const errorColumns: DataTableColumns<ParsedNginxError> = [
  { title: '时间', key: 'time', width: 170, render: (row) => logTimeCell(row.time || '-') },
  {
    title: '级别',
    key: 'level',
    width: 88,
    render: (row) =>
      h(
        NTag,
        { size: 'small', bordered: false, type: nginxLevelTagType(row.level) },
        () => (row.level === 'unknown' ? '未知' : row.level.toUpperCase()),
      ),
  },
  {
    title: '消息',
    key: 'message',
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'log-message mono' }, row.message),
  },
]

const systemColumns: DataTableColumns<SystemLogEntry> = [
  { title: '时间', key: 'time', width: 168, render: (row) => logTimeCell(row.time) },
  {
    title: '级别',
    key: 'level',
    width: 80,
    render: (row) => {
      const display = displaySystemLog(row)
      return h(
        NTag,
        { size: 'small', bordered: false, type: systemLevelTagType(display.displayLevel) },
        () => display.displayLevel,
      )
    },
  },
  {
    title: '模块',
    key: 'module',
    width: 108,
    render: (row) => {
      const display = displaySystemLog(row)
      return h(
        NTag,
        {
          size: 'small',
          bordered: false,
          type: systemModuleTagType(display.displayModule),
          class: 'log-module-tag',
        },
        () => display.displayModule,
      )
    },
  },
  {
    title: '消息',
    key: 'message',
    ellipsis: { tooltip: true },
    render: (row) => {
      const display = displaySystemLog(row)
      return h('span', { class: 'log-message' }, display.message)
    },
  },
]

async function loadProxyRules() {
  try {
    proxyRules.value = asList(await api.listProxies())
  } catch {
    proxyRules.value = []
  }
}

async function loadAccess() {
  loadingAccess.value = true
  try {
    await loadProxyRules()
    accessLogs.value = asList(await api.getAccessLogs({ limit: 100 }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载访问日志失败')
  } finally {
    loadingAccess.value = false
  }
}

async function loadErrorLogs() {
  loadingError.value = true
  try {
    const errors = asList(await api.getErrorLogs({ limit: 100 }))
    errorLogs.value = errors.map((line) => parseNginxErrorLine(line))
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载错误日志失败')
  } finally {
    loadingError.value = false
  }
}

async function loadSystemLogs() {
  loadingSystem.value = true
  try {
    systemLogs.value = asList(await api.getSystemLogs({ limit: 100 }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载系统日志失败')
  } finally {
    loadingSystem.value = false
  }
}

async function loadAll() {
  pageError.value = ''
  try {
    await Promise.all([loadAccess(), loadErrorLogs(), loadSystemLogs(), loadFrpLogs(), loadCfTunnels()])
  } catch (error) {
    pageError.value = error instanceof Error ? error.message : '请检查 Havline 服务是否正常运行'
  }
}

watch([accessKeyword, accessStatus], () => {
  accessPage.value = 1
})

watch(errorKeyword, () => {
  errorPage.value = 1
})

watch([systemKeyword, systemLevel], () => {
  systemPage.value = 1
})

watch([frpKeyword, frpLevel], () => {
  frpPage.value = 1
})

useVisibilityPolling(() => loadAccess(), 10000, { enabled: autoRefresh })

watch(
  () => route.query.tab,
  (queryTab) => {
    tab.value = resolveTab(queryTab)
  },
)

watch(tab, (name) => {
  if (name === 'access') loadAccess()
  else if (name === 'nginx') loadErrorLogs()
  else if (name === 'system') loadSystemLogs()
  else if (name === 'cloudflare') {
    void loadCfTunnels().then(() => loadCfLogs())
  }
})

onMounted(() => {
  tab.value = resolveTab(route.query.tab)
  loadAll()
})
</script>

<style scoped>
.logs-card :deep(.n-tabs-nav) {
  padding: var(--havline-space-4) var(--havline-space-5) 0;
}

.logs-card :deep(.n-tabs-tab) {
  font-size: 14px;
}

.logs-card :deep(.n-tab-pane) {
  padding-top: var(--havline-space-2);
}

.tab-hint {
  margin: 0 var(--havline-space-5) var(--havline-space-3);
  font-size: 13px;
  color: var(--havline-text-muted);
}

.log-pagination {
  display: flex;
  justify-content: flex-end;
  padding: var(--havline-space-4) var(--havline-space-5);
  border-top: 1px solid var(--havline-border);
}

.logs-card :deep(.log-table .n-data-table-th) {
  font-size: 13px;
  font-weight: 600;
  color: var(--havline-text-secondary);
}

.logs-card :deep(.log-table .n-data-table-base-table) {
  table-layout: fixed;
}

.logs-card :deep(.log-table .n-data-table-td) {
  font-size: 13px;
  padding-top: 10px;
  padding-bottom: 10px;
}

.nowrap {
  white-space: nowrap;
}

.logs-card :deep(.log-table .n-data-table-tr.log-row--error .n-data-table-td) {
  background: color-mix(in srgb, #ef4444 7%, transparent);
}

.logs-card :deep(.log-table .n-data-table-tr.log-row--warn .n-data-table-td) {
  background: color-mix(in srgb, #f59e0b 7%, transparent);
}

.logs-card :deep(.log-table .n-data-table-tr.log-row:hover .n-data-table-td) {
  background: color-mix(in srgb, var(--havline-brand) 5%, var(--havline-surface));
}

.log-time {
  white-space: nowrap;
  color: var(--havline-text-secondary);
  font-variant-numeric: tabular-nums;
}

.log-service {
  font-weight: 500;
}

.log-path {
  color: var(--havline-text-secondary);
}

.log-message {
  line-height: 1.5;
}

.mono {
  font-family: var(--havline-mono);
  font-size: 12px;
}

.log-duration {
  font-variant-numeric: tabular-nums;
}

.log-duration--slow {
  color: #d97706;
  font-weight: 600;
}

.log-duration--critical {
  color: #dc2626;
  font-weight: 600;
}

.logs-card :deep(.log-module-tag) {
  white-space: nowrap;
}
.remote-nginx-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--havline-space-3);
  margin-bottom: var(--havline-space-2);
}

.cf-log-pre {
  max-height: 620px;
  margin: 0 var(--havline-space-5);
  padding: var(--havline-space-4);
  overflow: auto;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.remote-nginx-bar__server {
  width: 200px;
}

.remote-nginx-bar__path {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 420px;
}
</style>
