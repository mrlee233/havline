<template>
  <LoadError v-if="loadError" :message="loadError" @retry="load" />

  <template v-else>
    <section class="hero-banner">
      <div class="hero-content">
        <h1 class="hero-banner__title">{{ greetingText }}</h1>
        <p class="hero-banner__desc">Havline 正在为你的 NAS 提供稳定、安全的访问服务。</p>
        <div class="hero-banner__badges">
          <span class="hero-banner__badge"><n-icon :component="CheckmarkCircleOutline" /> 简单易用</span>
          <span class="hero-banner__badge"><n-icon :component="FlashOutline" /> 安全稳定</span>
          <span class="hero-banner__badge"><n-icon :component="GlobeOutline" /> 随时随地访问</span>
        </div>
      </div>
      <img class="hero-art" :src="bannerImg" alt="" aria-hidden="true" />
    </section>

    <n-spin :show="loading && !status">
      <div class="stats-row">
        <StatCard label="公网 IP" tone="blue">
          <template #icon><n-icon :component="GlobeOutline" /></template>
          <template #extra><StatusBadge v-if="publicIPv4Label !== '-'" value="ok" text="正常" /></template>
          <template #value>
            <span class="stat-primary mono">{{ publicIPv4Label }}</span>
          </template>
          <p class="stat-desc mono">{{ publicIPv6Label }}</p>
          <div class="stat-foot">
            <div class="stat-foot__line">{{ publicIPSourceLabel }}</div>
            <div class="stat-foot__muted">上次更新 {{ formatRelativeTime(status?.ddns_last_updated) || '刚刚' }}</div>
          </div>
        </StatCard>

        <StatCard label="域名" tone="green">
          <template #icon><n-icon :component="WifiOutline" /></template>
          <template #extra><StatusBadge v-if="status?.ddns_status" :value="status.ddns_status" :text="statusLabel(status.ddns_status)" /></template>
          <template #value>{{ domainCountLabel }}</template>
          <p class="stat-desc">{{ domainSubLabel }}</p>
          <div class="stat-foot">
            <div class="stat-foot__line">{{ domainExamplesLabel }}</div>
            <div class="stat-foot__muted">上次检查 {{ formatRelativeTime(status?.ddns_last_updated) || '刚刚' }}</div>
          </div>
        </StatCard>

        <StatCard label="HTTPS 证书" tone="blue">
          <template #icon><n-icon :component="ShieldCheckmarkOutline" /></template>
          <template #extra><StatusBadge v-if="status?.certificate_status" :value="status.certificate_status" :text="statusLabel(status.certificate_status)" /></template>
          <template #value>{{ certCountLabel }}</template>
          <p class="stat-desc">{{ certSubLabel }}</p>
          <div v-if="primaryCert" class="stat-foot">
            <div class="stat-foot__line mono">{{ certDisplayName }}</div>
            <div class="stat-foot__muted">
              到期 {{ formatDate(primaryCert.expires_at) }} · 剩余 {{ primaryCert.days_left }} 天
            </div>
          </div>
        </StatCard>

        <StatCard label="服务" tone="teal">
          <template #icon><n-icon :component="GitNetworkOutline" /></template>
          <template #extra><StatusBadge :value="serviceHealth.badgeValue" :text="serviceHealth.badgeText" /></template>
          <template #value>{{ serviceHealth.value }}</template>
          <p class="stat-desc">{{ serviceHealth.desc }}</p>
          <div class="stat-foot">
            <div class="stat-foot__line">Havline · Nginx · DDNS · 证书</div>
            <div class="stat-foot__muted">
              运行 {{ formatUptime(status?.uptime_seconds ?? 0) }} · 上次检查 {{ formatRelativeTime(status?.started_at) || '刚刚' }}
            </div>
          </div>
        </StatCard>

        <StatCard label="公网穿透" tone="brand">
          <template #icon><n-icon :component="SwapVerticalOutline" /></template>
          <template #extra><StatusBadge :value="frpHealth.badgeValue" :text="frpHealth.badgeText" /></template>
          <template #value>{{ frpHealth.value }}</template>
          <p class="stat-desc">{{ frpHealth.desc }}</p>
          <div class="stat-foot">
            <div class="stat-foot__line mono">{{ frpHealth.version }}</div>
            <div class="stat-foot__muted">上次应用 {{ frpHealth.applied }}</div>
          </div>
        </StatCard>
      </div>

      <div class="metrics-row">
        <HavlineCard class="panel requests-panel">
          <div class="panel__head">
            <div class="panel-title">
              <n-icon :component="FlashOutline" class="panel-title__icon panel-title__icon--amber" />
              <span>今日请求</span>
            </div>
          </div>
          <div class="panel__body requests-panel__body">
            <div class="metric-hero">
              <span class="metric-hero__value">{{ (status?.request_today ?? 0).toLocaleString() }}</span>
              <n-tag v-if="requestTrend !== null" size="small" :bordered="false" :type="requestTrend >= 0 ? 'success' : 'warning'">
                较昨日 {{ requestTrend >= 0 ? '+' : '' }}{{ requestTrend }}%
              </n-tag>
            </div>
            <div class="panel-chart">
              <MiniBarChart :values="hourlyBars.values" :labels="hourlyBars.labels" />
            </div>
          </div>
        </HavlineCard>

        <HavlineCard class="panel traffic-panel">
          <div class="panel__head">
            <div class="panel-title">
              <n-icon :component="SwapVerticalOutline" class="panel-title__icon panel-title__icon--teal" />
              <span>实时流量</span>
            </div>
            <div class="traffic-source">
              <n-button
                size="tiny"
                :quaternary="trafficSource !== 'local'"
                :type="trafficSource === 'local' ? 'primary' : 'default'"
                @click="switchTrafficSource('local')"
              >本机</n-button>
              <n-button
                size="tiny"
                :quaternary="trafficSource !== 'public'"
                :type="trafficSource === 'public' ? 'primary' : 'default'"
                :disabled="!hasPublicTrafficSource"
                :title="hasPublicTrafficSource ? '公网（VPS 反代）侧流量，速率由 frps 两次采样得出' : '还没有穿透规则，公网侧暂无可显示的流量'"
                @click="switchTrafficSource('public')"
              >公网</n-button>
              <n-tag size="small" :bordered="false" class="range-tag">最近 5 分钟</n-tag>
            </div>
          </div>
          <div class="panel__body traffic-panel__body">
            <div class="traffic-rates">
              <div class="traffic-rate traffic-rate--up">
                <span class="traffic-rate__dot" />
                <span class="traffic-rate__label">上传</span>
                <span class="traffic-rate__value">{{ formatRate(trafficTotals.uploadRate) }}</span>
              </div>
              <div class="traffic-rate traffic-rate--down">
                <span class="traffic-rate__dot" />
                <span class="traffic-rate__label">下载</span>
                <span class="traffic-rate__value">{{ formatRate(trafficTotals.downloadRate) }}</span>
              </div>
            </div>
            <div class="panel-chart">
              <MiniTrafficChart
                :labels="trafficChart.labels"
                :upload="trafficChart.upload"
                :download="trafficChart.download"
              />
            </div>
            <div v-if="trafficTop.length > 0" class="traffic-top">
              <span class="traffic-top__title">流量 Top {{ trafficTop.length }}</span>
              <ul class="traffic-top__list">
                <li v-for="item in trafficTop" :key="item.name" class="traffic-top__item">
                  <span class="traffic-top__name" :title="item.name">{{ item.name }}</span>
                  <span class="traffic-top__rate">{{ item.rate }}</span>
                </li>
              </ul>
            </div>
          </div>
        </HavlineCard>

        <HavlineCard class="panel trend-card">
          <div class="panel__head">
            <div class="panel-title">
              <n-icon :component="StatsChartOutline" class="panel-title__icon panel-title__icon--teal" />
              <span>资源与流量趋势</span>
            </div>
            <div class="trend-range">
              <n-button
                v-for="item in METRICS_RANGES"
                :key="item.hours"
                size="tiny"
                quaternary
                :type="trendHours === item.hours ? 'primary' : 'default'"
                @click="trendHours = item.hours"
              >
                {{ item.label }}
              </n-button>
            </div>
          </div>
          <div class="panel__body">
            <MetricsTrendPanel :hours="trendHours" />
          </div>
        </HavlineCard>

      </div>

      <div class="ops-row">
        <HavlineCard class="panel perf-panel">
          <div class="panel__head">
            <div class="panel-title">
              <n-icon :component="FlashOutline" class="panel-title__icon panel-title__icon--amber" />
              <span>性能</span>
            </div>
          </div>
          <div class="panel__body perf-list">
            <div class="health-item">
              <div class="health-item__icon health-item__icon--blue">
                <n-icon :component="PulseOutline" />
              </div>
              <div class="health-item__main">
                <span class="health-item__name">平均响应</span>
                <span class="health-item__meta">今日访问平均耗时</span>
                <span class="health-item__detail">{{ avgResponseMeta }}</span>
              </div>
              <span class="perf-value">
                {{ avgResponseLabel }}<small v-if="requestToday > 0">ms</small>
              </span>
            </div>
            <div class="health-item">
              <div
                class="health-item__icon"
                :class="errorToday > 0 ? 'health-item__icon--error' : 'health-item__icon--green'"
              >
                <n-icon :component="AlertCircleOutline" />
              </div>
              <div class="health-item__main">
                <span class="health-item__name">今日异常</span>
                <span class="health-item__meta">状态码 ≥ 400 的请求</span>
                <span class="health-item__detail">{{ errorRateLabel }}</span>
              </div>
              <span class="perf-value" :class="{ 'perf-value--error': errorToday > 0 }">{{ errorTodayLabel }}</span>
            </div>
          </div>
        </HavlineCard>

        <HavlineCard class="panel">
          <div class="panel__head">
            <div class="panel-title">
              <n-icon :component="ServerOutline" class="panel-title__icon panel-title__icon--teal" />
              <span>系统资源</span>
            </div>
          </div>
          <div class="panel__body perf-list">
            <div v-for="item in resourceItems" :key="item.name" class="health-item">
              <div class="health-item__icon" :class="`health-item__icon--${item.tone}`">
                <n-icon :component="item.icon" />
              </div>
              <div class="health-item__main">
                <span class="health-item__name">{{ item.name }}</span>
                <span class="health-item__meta">{{ item.detail }}</span>
                <span v-if="item.available" class="res-bar" :class="`res-bar--${item.barTone}`">
                  <span class="res-bar__fill" :style="{ width: item.barWidth }" />
                </span>
              </div>
              <span class="perf-value">{{ item.value }}</span>
            </div>
          </div>
        </HavlineCard>

        <HavlineCard class="panel">
          <div class="panel__head">
            <div class="panel-title">
              <n-icon :component="GlobeOutline" class="panel-title__icon panel-title__icon--teal" />
              <span>中国 IP 段</span>
            </div>
            <div class="head-actions">
              <StatusBadge :value="cidrBadge.value" :text="cidrBadge.text" />
              <n-button size="tiny" quaternary :loading="refreshingCIDR" @click="refreshCIDR">刷新</n-button>
            </div>
          </div>
          <div class="panel__body perf-list">
            <div v-for="item in cidrItems" :key="item.name" class="health-item">
              <div class="health-item__icon" :class="`health-item__icon--${item.tone}`">
                <n-icon :component="item.icon" />
              </div>
              <div class="health-item__main">
                <span class="health-item__name">{{ item.name }}</span>
                <span class="health-item__meta">{{ item.detail }}</span>
              </div>
              <span class="perf-value">{{ item.value }}</span>
            </div>
          </div>
        </HavlineCard>

        <HavlineCard class="panel">
          <div class="panel__head">
            <div class="panel-title">
              <n-icon :component="ShieldCheckmarkOutline" class="panel-title__icon panel-title__icon--teal" />
              <span>通知通道</span>
            </div>
            <div class="head-actions">
              <StatusBadge :value="notifyChannel.badge" :text="notifyChannel.badgeText" />
              <router-link :to="{ name: 'settings' }" class="card-link">
                去设置
                <n-icon :component="ChevronForwardOutline" />
              </router-link>
            </div>
          </div>
          <div class="panel__body perf-list">
            <div v-for="row in notifyChannel.rows" :key="row.name" class="health-item">
              <div class="health-item__icon" :class="`health-item__icon--${row.tone}`">
                <n-icon :component="row.icon" />
              </div>
              <div class="health-item__main">
                <span class="health-item__name">{{ row.name }}</span>
                <span class="health-item__meta">{{ row.detail }}</span>
              </div>
              <span class="perf-value">{{ row.value }}</span>
            </div>
          </div>
        </HavlineCard>
      </div>

      <div class="bottom-row">
        <HavlineCard flush class="bottom-card">
          <template #title>
            <n-icon :component="ListOutline" class="card-title-icon" />
            <span>最近访问</span>
          </template>
          <template #header>
            <router-link :to="{ name: 'logs', query: { tab: 'access' } }" class="card-link">
              查看全部
              <n-icon :component="ChevronForwardOutline" />
            </router-link>
          </template>
          <div class="bottom-card__body">
            <n-data-table
              v-if="accessLogs.length > 0"
              class="bottom-table"
              :columns="accessColumns"
              :data="accessLogs"
              :bordered="false"
              size="small"
            />
            <EmptyState v-else title="暂无访问记录" description="产生访问后这里会显示最近请求。" />
          </div>
        </HavlineCard>

        <HavlineCard flush class="bottom-card">
          <template #title>
            <n-icon :component="DocumentTextOutline" class="card-title-icon" />
            <span>最新日志</span>
          </template>
          <template #header>
            <router-link :to="{ name: 'logs', query: { tab: 'system' } }" class="card-link">
              查看全部
              <n-icon :component="ChevronForwardOutline" />
            </router-link>
          </template>
          <div class="bottom-card__body">
            <n-data-table
              v-if="systemLogs.length > 0"
              class="bottom-table"
              :columns="systemColumns"
              :data="systemLogs"
              :bordered="false"
              size="small"
            />
            <EmptyState v-else title="暂无系统日志" description="应用运行后会产生日志。" />
          </div>
        </HavlineCard>
      </div>
    </n-spin>
  </template>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { NButton, NDataTable, NIcon, NSpin, NTag, useMessage, type DataTableColumns } from 'naive-ui'
import {
  AlertCircleOutline,
  AppsOutline,
  CheckmarkCircleOutline,
  ChevronForwardOutline,
  CloudOutline,
  DocumentTextOutline,
  FlashOutline,
  GitNetworkOutline,
  GlobeOutline,
  ListOutline,
  PulseOutline,
  ServerOutline,
  ShieldCheckmarkOutline,
  StatsChartOutline,
  SwapVerticalOutline,
  WifiOutline,
} from '@vicons/ionicons5'
import bannerImg from '../assets/brand/banner.png'
import { api, asList } from '../api/client'
import { useVisibilityPolling } from '../composables/useVisibilityPolling'
import type {
  AccessLogEntry,
  CertificateRecord,
  ChinaCIDRStatus,
  DashboardStatus,
  DDNSConfig,
  FrpRouteHealthSummary,
  FrpStatus,
  NotifyEmailConfig,
  NotifyTelegramConfig,
  NotifyType,
  NotifyWebhookConfig,
  ProxyRule,
  ProxyTraffic,
  SettingsMap,
  SystemLogEntry,
} from '../api/types'
import EmptyState from '../components/EmptyState.vue'
import HavlineCard from '../components/HavlineCard.vue'
import LoadError from '../components/LoadError.vue'
import MiniBarChart from '../components/MiniBarChart.vue'
import MiniTrafficChart from '../components/MiniTrafficChart.vue'
import MetricsTrendPanel from '../components/MetricsTrendPanel.vue'
import { METRICS_RANGES } from '../constants/metrics'

// 「资源与流量趋势」的时间档：切换即让子组件按新窗口重新取数
const trendHours = ref(24)
import StatCard from '../components/StatCard.vue'
import StatusBadge from '../components/StatusBadge.vue'
import {
  accessServiceTooltip,
  buildProxyBindingIndex,
  resolveAccessServiceLabel,
} from '../utils/accessService'
import {
  formatBytes,
  formatDate,
  formatLogTime,
  formatRate,
  formatRelativeTime,
  formatUptime,
} from '../utils/format'
import { displaySystemLog } from '../utils/logDisplay'
import { httpStatusKind, statusLabel } from '../utils/status'

const message = useMessage()
const status = ref<DashboardStatus | null>(null)
const ddnsConfigs = ref<DDNSConfig[]>([])
const certificates = ref<CertificateRecord[]>([])
const accessLogs = ref<AccessLogEntry[]>([])
const proxyRules = ref<ProxyRule[]>([])
const systemLogs = ref<SystemLogEntry[]>([])
const frpStatus = ref<FrpStatus | null>(null)
const routeHealth = ref<FrpRouteHealthSummary | null>(null)
const loadingRemoteHealth = ref(false)
const chinaCIDR = ref<ChinaCIDRStatus | null>(null)
const refreshingCIDR = ref(false)
const settings = ref<SettingsMap | null>(null)
const proxyBindingIndex = computed(() => buildProxyBindingIndex(proxyRules.value))
const loading = ref(false)
const loadError = ref('')
const trafficByRule = ref<Record<number, ProxyTraffic>>({})
type RatePoint = { at: number; upload: number; download: number }
const rateHistory = ref<RatePoint[]>([])
const RATE_HISTORY_MS = 5 * 60 * 1000

// 实时流量的数据源：本机（Havline 管理的本机 Nginx 访问日志）或公网（frps 侧的逐规则计数）
type TrafficSource = 'local' | 'public'
type TrafficRow = { name: string; uploadRate: number; downloadRate: number }
const trafficSource = ref<TrafficSource>('local')
// 公网侧：只取需要的两个速率字段，避免引额外类型；键为 frp 规则 ID
const publicTraffic = ref<Record<number, { traffic_in_rate: number; traffic_out_rate: number }>>({})
const frpProxyNames = ref<Record<number, string>>({})
// 公网源是否可选：得先有穿透规则（有规则才有名字）
const hasPublicTrafficSource = computed(() => Object.keys(frpProxyNames.value).length > 0)

// activeTrafficRows 把两个源归一成同一形状，后面的汇总、Top 与采样都只认它
const activeTrafficRows = computed<TrafficRow[]>(() => {
  if (trafficSource.value === 'public') {
    return Object.entries(publicTraffic.value).map(([key, item]) => ({
      name: frpProxyNames.value[Number(key)] ?? `规则 #${key}`,
      uploadRate: item.traffic_in_rate,
      downloadRate: item.traffic_out_rate,
    }))
  }
  return Object.entries(trafficByRule.value).map(([key, stats]) => {
    const id = Number(key)
    const rule = proxyRules.value.find((item) => item.id === id)
    const name = rule ? rule.name?.trim() || rule.domain || `规则 #${id}` : `规则 #${id}`
    return { name, uploadRate: stats.upload_rate, downloadRate: stats.download_rate }
  })
})

function timeGreeting(hour: number): string {
  if (hour >= 5 && hour < 12) return '上午好'
  if (hour >= 12 && hour < 18) return '下午好'
  return '晚上好'
}

const greetingText = computed(() => `${timeGreeting(new Date().getHours())}，管理员`)

const primaryCert = computed(() => {
  if (certificates.value.length === 0) return null
  return [...certificates.value].sort((a, b) => a.days_left - b.days_left)[0]
})
const certWildcard = computed(() => {
  const domains = primaryCert.value?.domains ?? []
  return domains.find((d) => d.startsWith('*.')) ?? (primaryCert.value?.wildcard ? `*.${primaryCert.value.domain}` : '')
})
const certDisplayName = computed(() => certWildcard.value || primaryCert.value?.domain || '')

const publicIPv4Label = computed(() => status.value?.public_ipv4 || '-')
const publicIPv6Label = computed(() => {
  if (status.value?.public_ip_source === 'ddns' && !status.value?.public_ipv6) return '未获取'
  return status.value?.public_ipv6 || '-'
})
const publicIPSourceLabel = computed(() => {
  switch (status.value?.public_ip_source) {
    case 'ddns':
      return status.value?.public_ipv4 || status.value?.public_ipv6
        ? '来源：DDNS 已同步记录'
        : '来源：DDNS 已配置，等待同步'
    case 'detect':
      return '来源：出口 IP 探测（未配置 DDNS）'
    default:
      return '来源：暂无'
  }
})

const domainCountLabel = computed(() => {
  const count = status.value?.ddns_count ?? ddnsConfigs.value.length
  if (count === 0) return '未配置'
  return `${count} 个`
})

const domainSubLabel = computed(() => {
  const count = status.value?.ddns_count ?? ddnsConfigs.value.length
  if (count === 0) return '尚未配置域名解析'
  return '已配置并正常解析的域名'
})

const domainExamplesLabel = computed(() => {
  const examples: string[] = []
  for (const cfg of ddnsConfigs.value.filter((c) => c.enabled)) {
    const root = cfg.root_domain
    const names = cfg.record_names?.length ? cfg.record_names : [cfg.record_name || '@']
    for (const name of names) {
      if (!name || name === '@') examples.push(root)
      else if (name === '*') examples.push(`*.${root}`)
      else if (name.includes('.')) examples.push(name)
      else examples.push(`${name}.${root}`)
    }
  }
  return examples.length > 0 ? examples.join(', ') : '暂无域名记录'
})

const certCountLabel = computed(() => {
  const count = status.value?.certificate_count ?? certificates.value.length
  if (count === 0) return '未申请'
  return `${count} 个`
})

const certSubLabel = computed(() => {
  if (!primaryCert.value) return '尚未申请 HTTPS 证书'
  if (status.value?.certificate_status === 'ok') return '证书有效，自动续期中'
  return '证书需要关注'
})

// 「服务」卡按四项实际健康状态聚合：任一项非 running/ok 都降级并点名，避免写死「正常」
const serviceHealth = computed(() => {
  const items = healthItems.value
  const bad = items.filter((item) => item.status !== 'running' && item.status !== 'ok')
  if (bad.length === 0) {
    return { value: `${items.length} 个`, desc: '核心服务运行正常', badgeValue: 'ok', badgeText: '正常' }
  }
  const severe = bad.some((item) => item.status === 'error' || item.status === 'stopped')
  return {
    value: `${items.length - bad.length}/${items.length} 正常`,
    desc: `异常：${bad.map((item) => item.name).join('、')}`,
    badgeValue: severe ? 'error' : 'warning',
    badgeText: '需关注',
  }
})

// 今日异常与平均响应：/api/status 早已返回，此前未展示
const requestToday = computed(() => status.value?.request_today ?? 0)
const errorToday = computed(() => status.value?.error_today ?? 0)
// 今日无请求时不显示 0ms / 0（看起来像真实测量值），统一显示为 —
const avgResponseLabel = computed(() =>
  requestToday.value > 0 ? String(Math.round(status.value?.avg_response_ms ?? 0)) : '—',
)
const avgResponseMeta = computed(() =>
  requestToday.value > 0 ? `共 ${requestToday.value.toLocaleString()} 次请求` : '今日暂无请求',
)
const errorTodayLabel = computed(() => (requestToday.value > 0 ? errorToday.value.toLocaleString() : '—'))
const errorRateLabel = computed(() => {
  if (requestToday.value <= 0) return '今日暂无请求'
  return `占今日请求 ${((errorToday.value / requestToday.value) * 100).toFixed(1)}%`
})

// 公网穿透（FRP）：frpc 客户端侧状态
const frpHealth = computed(() => {
  const frp = frpStatus.value
  if (!frp) {
    return {
      value: '未检测',
      desc: '未获取到 frpc 状态',
      badgeValue: 'none',
      badgeText: '未检测',
      version: 'frpc 未检测',
      applied: '暂无',
    }
  }
  if (!frp.configured) {
    return {
      value: '未配置',
      desc: '尚未配置 FRP 服务端',
      badgeValue: 'disabled',
      badgeText: '未配置',
      version: 'frpc 未安装',
      applied: '暂无',
    }
  }
  const running = frp.status === 'running'
  return {
    value: `${frp.enabled_proxies} 条`,
    desc: running ? 'frpc 已连接服务端' : frp.last_error ? `frpc 异常：${frp.last_error}` : 'frpc 已停止',
    badgeValue: running ? 'running' : frp.status === 'stopped' ? 'stopped' : 'warning',
    badgeText: running ? '运行中' : frp.status === 'stopped' ? '已停止' : '未检测',
    version: frp.frpc_version ? `frpc ${frp.frpc_version}` : 'frpc 版本未知',
    applied: formatRelativeTime(frp.last_reloaded_at) || '暂无',
  }
})

// 公网反代部署健康：逐台服务端联网采集，单独请求（见 loadRouteHealth）
const remoteHealth = computed(() => {
  if (loadingRemoteHealth.value) return { status: 'none', label: '检查中' }
  const summary = routeHealth.value
  if (!summary) return { status: 'none', label: '未获取' }
  if (summary.servers === 0) return { status: 'disabled', label: '未配置' }
  if (summary.routes === 0) return { status: 'none', label: '暂无路由' }
  const allHealthy = summary.healthy === summary.routes && !summary.failed_servers?.length
  return { status: allHealthy ? 'ok' : 'warning', label: allHealthy ? '全部健康' : '需关注' }
})

const remoteHealthMeta = computed(() => {
  if (loadingRemoteHealth.value) return '正在采集公网侧状态…'
  const summary = routeHealth.value
  if (!summary) return '未能获取公网侧状态'
  if (summary.servers === 0) return '尚未配置 FRP 服务端'
  if (summary.routes === 0) return '尚无已配置域名的穿透规则'
  const issues: string[] = []
  if (summary.dns_failed) issues.push(`DNS ${summary.dns_failed}`)
  if (summary.cert_missing) issues.push(`证书 ${summary.cert_missing}`)
  if (summary.tunnel_down) issues.push(`隧道 ${summary.tunnel_down}`)
  if (summary.tunnel_unknown) issues.push(`隧道无法判定 ${summary.tunnel_unknown}`)
  if (summary.service_down) issues.push(`服务 ${summary.service_down}`)
  const parts = [`${summary.healthy}/${summary.routes} 条健康`]
  if (issues.length > 0) parts.push(issues.join(' · '))
  if (summary.failed_servers?.length) parts.push(`采集失败 ${summary.failed_servers.join('、')}`)
  return parts.join('｜')
})

const trafficTotals = computed(() => {
  let uploadRate = 0
  let downloadRate = 0
  for (const row of activeTrafficRows.value) {
    uploadRate += row.uploadRate
    downloadRate += row.downloadRate
  }
  return { uploadRate, downloadRate }
})

const trafficChart = computed(() => {
  const now = Date.now()
  const labels: string[] = []
  const upload: number[] = []
  const download: number[] = []
  for (let i = 5; i >= 0; i--) {
    const bucketEnd = now - i * 60 * 1000
    const bucketStart = bucketEnd - 60 * 1000
    const points = rateHistory.value.filter((p) => p.at > bucketStart && p.at <= bucketEnd)
    const avg = (key: 'upload' | 'download') =>
      points.length ? points.reduce((sum, p) => sum + p[key], 0) / points.length : 0
    upload.push(avg('upload'))
    download.push(avg('download'))
    const d = new Date(bucketEnd)
    labels.push(
      `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`,
    )
  }
  return { labels, upload, download }
})

// 当前流量 Top 5：直接复用已有轮询的数据，不额外请求接口
const trafficTop = computed(() =>
  activeTrafficRows.value
    .filter((row) => row.uploadRate + row.downloadRate > 0)
    .sort((a, b) => b.uploadRate + b.downloadRate - (a.uploadRate + a.downloadRate))
    .slice(0, 5)
    .map((row) => ({ name: row.name, rate: formatRate(row.uploadRate + row.downloadRate) })),
)

const calendarHourLabels = Array.from({ length: 12 }, (_, i) =>
  `${String(i * 2).padStart(2, '0')}:00`,
)

const hourlyBars = computed(() => {
  const hourly = status.value?.requests_hourly
  const apiLabels = status.value?.requests_hourly_labels
  if (hourly?.length === 12 && apiLabels?.length === 12) {
    return { labels: apiLabels, values: hourly }
  }
  if (hourly?.length === 12) {
    return { labels: calendarHourLabels, values: hourly }
  }
  return { labels: calendarHourLabels, values: Array(12).fill(0) }
})

const requestTrend = computed(() => {
  const trend = status.value?.request_trend
  return trend == null ? null : trend
})

const healthItems = computed(() => [
  {
    name: 'Havline',
    desc: '应用核心服务',
    meta: `运行 ${formatUptime(status.value?.uptime_seconds ?? 0)}`,
    status: 'running',
    label: '运行中',
    icon: AppsOutline,
    tone: 'brand',
  },
  {
    name: 'Nginx',
    desc: '反向代理服务',
    meta: status.value?.nginx_status === 'running' ? `运行 ${formatUptime(status.value?.uptime_seconds ?? 0)}` : '已停止',
    status: status.value?.nginx_status,
    label: status.value?.nginx_status === 'running' ? '运行中' : '已停止',
    icon: ServerOutline,
    tone: 'blue',
  },
  {
    name: 'DDNS',
    desc: '域名动态解析',
    meta: formatRelativeTime(status.value?.ddns_last_updated) || '未更新',
    status: status.value?.ddns_status,
    label: status.value?.ddns_status === 'ok' ? '运行中' : '异常',
    icon: CloudOutline,
    tone: 'teal',
  },
  {
    name: 'HTTPS 证书',
    desc: '自动续期管理',
    meta: primaryCert.value ? `${primaryCert.value.days_left} 天后到期` : '未配置',
    status: status.value?.certificate_status,
    label: status.value?.certificate_status === 'ok' ? '运行中' : '需关注',
    icon: ShieldCheckmarkOutline,
    tone: 'green',
  },
  {
    name: '公网反代',
    desc: '公网侧部署与健康',
    meta: remoteHealthMeta.value,
    status: remoteHealth.value.status,
    label: remoteHealth.value.label,
    icon: GlobeOutline,
    tone: 'green',
  },
  {
    name: 'Cloudflare 隧道',
    desc: '无公网 IP 的 Cloudflare 出口',
    meta: status.value?.cloudflare_message || '未创建隧道',
    status: status.value?.cloudflare_status,
    label: status.value?.cloudflare_status === 'ok' ? '运行中' : status.value?.cloudflare_status === 'warning' ? '部分在线' : status.value?.cloudflare_status === 'none' ? '未配置' : '需关注',
    icon: CloudOutline,
    tone: 'blue',
  },
])

const accessColumns = computed<DataTableColumns<AccessLogEntry>>(() => [
  { title: '时间', key: 'time', width: 168, render: (r) => formatLogTime(r.time) },
  {
    title: '服务',
    key: 'service',
    minWidth: 100,
    ellipsis: { tooltip: true },
    render: (r) =>
      h(
        'span',
        { title: accessServiceTooltip(r, proxyBindingIndex.value) },
        resolveAccessServiceLabel(r, proxyBindingIndex.value),
      ),
  },
  {
    title: '方法',
    key: 'method',
    width: 68,
    render: (r) =>
      h(
        NTag,
        { size: 'tiny', bordered: false, type: r.method === 'GET' ? 'info' : r.method === 'POST' ? 'success' : 'warning' },
        () => r.method,
      ),
  },
  {
    title: '状态',
    key: 'status',
    width: 68,
    render: (r) => {
      const kind = httpStatusKind(r.status)
      const type = kind === 'success' ? 'success' : kind === 'warning' ? 'warning' : 'error'
      return h(NTag, { size: 'tiny', bordered: false, type }, () => String(r.status))
    },
  },
  { title: '延迟(ms)', key: 'response_time', width: 80, render: (r) => Math.round(r.response_time * 1000) },
  {
    title: '来源 IP',
    key: 'client_ip',
    minWidth: 120,
    render: (r) => h('span', { class: 'mono ip-cell' }, r.client_ip),
  },
])

const systemColumns: DataTableColumns<SystemLogEntry> = [
  { title: '时间', key: 'time', width: 168, render: (r) => formatLogTime(r.time) },
  {
    title: '级别',
    key: 'level',
    width: 72,
    render: (r) => {
      const display = displaySystemLog(r)
      return h(
        NTag,
        { size: 'tiny', bordered: false, type: levelTag(display.displayLevel) },
        () => display.displayLevel,
      )
    },
  },
  {
    title: '模块',
    key: 'module',
    width: 108,
    render: (r) => h('span', { class: 'nowrap-cell' }, displaySystemLog(r).displayModule),
  },
  {
    title: '内容',
    key: 'message',
    ellipsis: { tooltip: true },
    render: (r) => displaySystemLog(r).message,
  },
]

function levelTag(level: string) {
  if (level === '错误' || level === 'ERROR') return 'error'
  if (level === '警告' || level === 'WARN') return 'warning'
  return 'info'
}

function recordRateSample() {
  const now = Date.now()
  rateHistory.value.push({
    at: now,
    upload: trafficTotals.value.uploadRate,
    download: trafficTotals.value.downloadRate,
  })
  const cutoff = now - RATE_HISTORY_MS
  rateHistory.value = rateHistory.value.filter((p) => p.at >= cutoff)
}

let trafficInFlight = false

async function refreshTraffic() {
  // 2 秒一轮且链路慢时会堆积并发请求：上一轮没回来就跳过这一轮
  if (trafficInFlight) return
  trafficInFlight = true
  // 首次轮询时顺手把公网规则名拉回来：用于判断「公网」源是否可选 + Top 展示
  void ensureFrpProxyNames()
  try {
    if (trafficSource.value === 'public') {
      // 公网侧：frps 返回的是累计计数，速率由后端基于两次采样算好（间隔需 ≥2 秒）
      const payload = await api.listFrpProxiesTraffic()
      const next: Record<number, { traffic_in_rate: number; traffic_out_rate: number }> = {}
      for (const [key, value] of Object.entries(payload.items ?? {})) {
        next[Number(key)] = {
          traffic_in_rate: value.traffic_in_rate,
          traffic_out_rate: value.traffic_out_rate,
        }
      }
      publicTraffic.value = next
    } else {
      const rows = asList(await api.getProxyTraffic())
      const next: Record<number, ProxyTraffic> = {}
      for (const row of rows) {
        next[row.rule_id] = row
      }
      trafficByRule.value = next
    }
    recordRateSample()
  } catch {
    // ignore polling errors
  } finally {
    trafficInFlight = false
  }
}

// 切换数据源：先清空 5 分钟曲线，避免把两个来源的采样混在一张图里
function switchTrafficSource(next: TrafficSource) {
  if (trafficSource.value === next) return
  trafficSource.value = next
  rateHistory.value = []
  void refreshTraffic()
}

// 公网侧只需要规则名做 Top 展示；只拉一次，失败不阻断流量轮询
async function ensureFrpProxyNames() {
  if (Object.keys(frpProxyNames.value).length > 0) return
  try {
    const proxies = asList(await api.listFrpProxies())
    const names: Record<number, string> = {}
    for (const item of proxies) {
      names[item.id] = item.name?.trim() || item.custom_domains?.[0] || `规则 #${item.id}`
    }
    frpProxyNames.value = names
  } catch {
    // 静默：名称拿不到时 Top 里会退化成「规则 #id」
  }
}

// CPU 占用率需要两次采样才有意义，因此 /api/status 每 15 秒刷新一次
async function refreshStatus() {
  try {
    status.value = await api.getStatus()
  } catch {
    // 静默失败：保留上一次快照
  }
}

// 系统资源：/api/status 返回的宿主机 CPU / 内存 / 磁盘（容器内为宿主机视角）
const resourcesAvailable = computed(
  () => (status.value?.mem_total_bytes ?? 0) > 0 || (status.value?.disk_total_bytes ?? 0) > 0,
)

function usagePercent(used?: number, total?: number): number {
  if (!used || !total || total <= 0) return 0
  return Math.min(100, (used / total) * 100)
}

const resourceItems = computed(() => {
  const data = status.value
  const rows = [
    {
      name: 'CPU',
      icon: AppsOutline,
      tone: 'blue',
      detail: '整机 CPU 占用率',
      percent: Math.min(100, Math.max(0, data?.cpu_percent ?? 0)),
      available: resourcesAvailable.value,
    },
    {
      name: '内存',
      icon: ServerOutline,
      tone: 'green',
      detail: `${formatBytes(data?.mem_used_bytes ?? 0)} / ${formatBytes(data?.mem_total_bytes ?? 0)}`,
      percent: usagePercent(data?.mem_used_bytes, data?.mem_total_bytes),
      available: (data?.mem_total_bytes ?? 0) > 0,
    },
    {
      name: '磁盘',
      icon: ListOutline,
      tone: 'teal',
      detail: `${formatBytes(data?.disk_used_bytes ?? 0)} / ${formatBytes(data?.disk_total_bytes ?? 0)}`,
      percent: usagePercent(data?.disk_used_bytes, data?.disk_total_bytes),
      available: (data?.disk_total_bytes ?? 0) > 0,
    },
  ]
  return rows.map((row) => ({
    ...row,
    value: row.available ? `${Math.round(row.percent)}%` : '—',
    barWidth: `${row.percent.toFixed(1)}%`,
    barTone: row.percent >= 90 ? 'danger' : row.percent >= 75 ? 'warn' : 'ok',
  }))
})

// 中国 IP 段：安全模块（china_only）依赖的国内地址库
const cidrBadge = computed(() => {
  const data = chinaCIDR.value
  if (!data) return { value: 'none', text: '未获取' }
  if (data.last_error) return { value: 'error', text: '更新失败' }
  if (!data.ready) return { value: 'warning', text: '未就绪' }
  return { value: 'ok', text: '就绪' }
})

const cidrItems = computed(() => {
  const data = chinaCIDR.value
  return [
    {
      name: 'IPv4 条目',
      icon: GlobeOutline,
      tone: 'blue',
      detail: '国内 IPv4 地址段',
      value: (data?.entry_count_v4 ?? 0).toLocaleString(),
    },
    {
      name: 'IPv6 条目',
      icon: WifiOutline,
      tone: 'teal',
      detail: '国内 IPv6 地址段',
      value: (data?.entry_count_v6 ?? 0).toLocaleString(),
    },
    {
      name: '上次更新',
      icon: DocumentTextOutline,
      tone: 'green',
      detail: data?.last_error ? `上次错误：${data.last_error}` : '按设置周期自动更新',
      value: formatRelativeTime(data?.updated_at) || '从未更新',
    },
  ]
})

async function loadChinaCIDR() {
  try {
    chinaCIDR.value = await api.getChinaCIDRStatus()
  } catch {
    chinaCIDR.value = null
  }
}

// 中国 IP 段更新是后端后台任务：刷新后按 2 秒轮询直到就绪或结束
const cidrPolling = ref(false)

async function pollCIDR() {
  await loadChinaCIDR()
  const data = chinaCIDR.value
  if (!data || data.ready || !data.updating) {
    cidrPolling.value = false
    if (data?.ready) message.success('中国 IP 段已更新')
  }
}

async function refreshCIDR() {
  refreshingCIDR.value = true
  cidrPolling.value = false
  try {
    chinaCIDR.value = await api.refreshChinaCIDR()
    if (chinaCIDR.value.ready) {
      message.success(
        `中国 IP 段已更新：IPv4 ${chinaCIDR.value.entry_count_v4} 条，IPv6 ${chinaCIDR.value.entry_count_v6} 条`,
      )
    } else if (chinaCIDR.value.updating) {
      cidrPolling.value = true
    } else {
      message.warning('更新完成，但数据尚未就绪，请查看错误信息')
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '更新中国 IP 段失败')
  } finally {
    refreshingCIDR.value = false
  }
}

// 通知通道：设置里的 notify_type + 各通道 JSON（has_* 标记由后端保存时写入）
function parseSettingsJSON<T>(raw?: string): T | null {
  if (!raw) return null
  try {
    return JSON.parse(raw) as T
  } catch {
    return null
  }
}

const notifyChannel = computed(() => {
  const map = settings.value ?? {}
  const type = (map.notify_type ?? '') as NotifyType
  const telegram = parseSettingsJSON<NotifyTelegramConfig>(map.notify_telegram_json)
  const webhook = parseSettingsJSON<NotifyWebhookConfig>(map.notify_webhook_json)
  const email = parseSettingsJSON<NotifyEmailConfig>(map.notify_email_json)
  const events = Object.entries(map).filter(
    ([key, value]) => key.startsWith('notify_on_') && (value === '1' || value === 'true'),
  ).length

  let name = '未启用'
  let secret = '通知凭据'
  let configured = false
  if (type === 'telegram') {
    name = 'Telegram'
    secret = 'Bot Token'
    configured = Boolean(telegram?.has_bot_token)
  } else if (type === 'webhook') {
    name = `Webhook（${webhook?.provider || '未选择服务商'}）`
    secret = 'Webhook 密钥'
    configured = Boolean(webhook?.has_secret)
  } else if (type === 'email') {
    name = '邮件 SMTP'
    secret = 'SMTP 密码'
    configured = Boolean(email?.has_password)
  }

  return {
    badge: type ? (configured ? 'ok' : 'warning') : 'disabled',
    badgeText: type ? (configured ? '已启用' : '待补全') : '未启用',
    rows: [
      {
        name: '当前通道',
        icon: CheckmarkCircleOutline,
        tone: 'blue',
        detail: type ? '告警通知的发送方式' : '尚未选择通知方式',
        value: name,
      },
      {
        name: '密钥凭据',
        icon: ShieldCheckmarkOutline,
        tone: 'green',
        detail: secret,
        value: configured ? '已保存' : '未保存',
      },
      {
        name: '已开启告警',
        icon: PulseOutline,
        tone: 'teal',
        detail: 'DDNS、证书、登录失败等事件',
        value: `${events} 项`,
      },
    ],
  }
})

// 公网侧健康需逐台服务端联网采集，单独请求，避免拖慢整页加载
async function loadRouteHealth() {
  loadingRemoteHealth.value = true
  try {
    routeHealth.value = await api.getFrpRouteHealthSummary()
  } catch {
    routeHealth.value = null
  } finally {
    loadingRemoteHealth.value = false
  }
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [s, access, system, certs, rules, frp, cidr, config] = await Promise.all([
      api.getStatus(),
      api.getAccessLogs({ limit: 10 }),
      api.getSystemLogs({ limit: 10 }),
      api.listCertificates(),
      api.listProxies().catch((): ProxyRule[] => []),
      api.getFrpStatus().catch((): FrpStatus | null => null),
      api.getChinaCIDRStatus().catch((): ChinaCIDRStatus | null => null),
      api.getSettings().catch((): SettingsMap | null => null),
    ])
    status.value = s
    accessLogs.value = asList(access)
    proxyRules.value = asList(rules)
    systemLogs.value = asList(system)
    certificates.value = asList(certs)
    frpStatus.value = frp
    chinaCIDR.value = cidr
    settings.value = config
    void loadRouteHealth()
    api.listDDNSLite()
      .then((ddns) => {
        ddnsConfigs.value = asList(ddns)
      })
      .catch(() => {})
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '请检查 Havline 服务是否正常运行'
    message.error('加载仪表盘失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
})

useVisibilityPolling(refreshTraffic, 2000, { immediate: true })
useVisibilityPolling(refreshStatus, 15000, { immediate: true })
useVisibilityPolling(pollCIDR, 2000, { enabled: cidrPolling })
</script>

<style scoped>
.hero-banner {
  position: relative;
  overflow: hidden;
  margin-bottom: var(--havline-space-4);
  min-height: 208px;
  height: 220px;
  border-radius: 16px;
  box-shadow: var(--havline-shadow);
  border: 1px solid rgba(16, 185, 129, 0.08);
  background: linear-gradient(100deg, #f4fffc 0%, #f1fbff 55%, #eef9ff 100%);
}

html[data-theme='dark'] .hero-banner,
html.dark .hero-banner {
  background: linear-gradient(100deg, #0f172a 0%, #134e4a 100%);
  border-color: rgba(16, 185, 129, 0.15);
}

.hero-content {
  position: relative;
  z-index: 2;
  display: flex;
  flex-direction: column;
  justify-content: center;
  width: 46%;
  height: 100%;
  padding: 28px 32px;
  box-sizing: border-box;
}

.hero-art {
  position: absolute;
  z-index: 1;
  right: 0;
  bottom: 0;
  height: 100%;
  width: auto;
  max-width: none;
  object-fit: contain;
  object-position: right bottom;
  pointer-events: none;
  user-select: none;
}

html[data-theme='dark'] .hero-art,
html.dark .hero-art {
  opacity: 0.9;
}

.hero-banner__title {
  margin: 0;
  font-size: 30px;
  font-weight: 700;
  color: var(--havline-text);
  letter-spacing: -0.03em;
}

.hero-banner__desc {
  margin: 10px 0 0;
  font-size: 14px;
  color: var(--havline-text-secondary);
  max-width: 520px;
  line-height: 1.65;
}

.hero-banner__badges {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 16px;
}

.hero-banner__badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 999px;
  font-size: 13px;
  color: var(--havline-text-secondary);
  background: var(--havline-surface);
  border: 1px solid rgba(16, 185, 129, 0.14);
}

.hero-banner__badge .n-icon {
  font-size: 16px;
  color: var(--havline-brand);
}

@media (max-width: 768px) {
  .hero-content {
    width: 100%;
    padding: 22px 20px;
  }

  .hero-banner {
    height: auto;
    min-height: 180px;
  }

  .hero-art {
    height: 100%;
    opacity: 0.28;
  }

  html[data-theme='dark'] .hero-art,
  html.dark .hero-art {
    opacity: 0.22;
  }

  .hero-banner__title {
    font-size: 24px;
  }

  .hero-banner__badges {
    gap: 8px;
  }
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: var(--havline-space-4);
  margin-bottom: var(--havline-space-4);
}

.stat-primary {
  font-size: 22px;
  font-weight: 700;
  color: var(--havline-text);
  line-height: 1.3;
}

.stat-desc {
  margin: 8px 0 0;
  font-size: 13px;
  color: var(--havline-text-secondary);
  line-height: 1.5;
}

.stat-foot {
  margin-top: var(--havline-space-3);
  padding-top: var(--havline-space-3);
  border-top: 1px solid var(--havline-border);
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat-foot__line {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--havline-text);
}

.stat-foot__muted {
  font-size: 12px;
  color: var(--havline-text-muted);
}

.traffic-panel {
  display: flex;
  flex-direction: column;
}

.traffic-panel :deep(.havline-card__body) {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.requests-panel__body,
.traffic-panel__body {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.panel-chart {
  flex: 1;
  min-height: 240px;
}

.requests-panel .panel-chart :deep(.chart),
.traffic-panel .panel-chart :deep(.chart) {
  height: 100%;
  min-height: 240px;
}

.traffic-rates {
  display: flex;
  gap: var(--havline-space-4);
  margin-bottom: var(--havline-space-3);
}

.traffic-rate {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.traffic-rate__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.traffic-rate--up .traffic-rate__dot {
  background: #10b981;
}

.traffic-rate--down .traffic-rate__dot {
  background: #3b82f6;
}

.traffic-rate__label {
  font-size: 12px;
  color: var(--havline-text-muted);
}

.traffic-rate__value {
  font-size: 14px;
  font-weight: 600;
  color: var(--havline-text);
  font-variant-numeric: tabular-nums;
}

.traffic-top {
  margin-top: var(--havline-space-3);
  padding-top: var(--havline-space-3);
  border-top: 1px solid var(--havline-border);
}

.traffic-top__title {
  font-size: 12px;
  color: var(--havline-text-muted);
}

.traffic-top__list {
  margin: 6px 0 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 4px;
}

.traffic-top__item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: var(--havline-space-3);
  font-size: 13px;
}

.traffic-top__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--havline-text);
}

.traffic-top__rate {
  color: var(--havline-text-secondary);
  font-variant-numeric: tabular-nums;
}

.ops-row {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--havline-space-4);
  margin-bottom: var(--havline-space-4);
}

.metrics-row {
  display: grid;
  grid-template-columns: 3fr 3fr 2fr 2fr;
  gap: var(--havline-space-4);
  margin-bottom: var(--havline-space-4);
  align-items: stretch;
}

/* 趋势卡片占两列：这样「性能」才能跟「系统资源」等一起排到下一行的四列里 */
.trend-card { grid-column: span 2; }

.metrics-row > .havline-card {
  height: 100%;
}

.panel :deep(.havline-card__body) {
  padding-top: 0;
}

.panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--havline-space-5) var(--havline-space-5) 0;
}

.panel__body {
  padding: var(--havline-space-3) var(--havline-space-5) var(--havline-space-5);
}

.panel-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: var(--havline-text);
}

.panel-title__icon {
  font-size: 18px;
  color: var(--havline-brand);
}

.panel-title__icon--amber {
  color: #f59e0b;
}

.panel-title__icon--teal {
  color: #14b8a6;
}

/* 实时流量卡片的来源切换（本机 / 公网） */
.traffic-source { display: flex; align-items: center; gap: 6px; }
/* 趋势卡片表头里的时间档切换（对齐「实时流量」的 range-tag 位置） */
.trend-range { display: flex; gap: 2px; }

.range-tag {
  background: var(--havline-bg) !important;
  color: var(--havline-text-secondary) !important;
}

.metric-hero {
  display: flex;
  align-items: baseline;
  gap: var(--havline-space-3);
  margin-bottom: 4px;
}

.metric-hero__value {
  font-size: 34px;
  font-weight: 700;
  letter-spacing: -0.03em;
  color: var(--havline-text);
  line-height: 1.1;
}

.requests-panel {
  display: flex;
  flex-direction: column;
}

.requests-panel :deep(.havline-card__body) {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.health-panel {
  display: flex;
  flex-direction: column;
}

.health-panel :deep(.havline-card__body) {
  flex: 1;
  display: flex;
  flex-direction: column;
}

/* 性能面板的行沿用「运行状态」的 .health-item，但不随面板高度被拉高（该面板只有两项指标） */
.perf-list {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 8px;
  min-height: 0;
}

.perf-list .health-item {
  flex: 0 0 auto;
}

.perf-value {
  justify-self: end;
  font-size: 16px;
  font-weight: 700;
  color: var(--havline-text);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.perf-value small {
  margin-left: 2px;
  font-size: 12px;
  font-weight: 500;
  color: var(--havline-text-secondary);
}

.perf-value--error {
  color: var(--havline-error);
}

.head-actions {
  display: flex;
  align-items: center;
  gap: var(--havline-space-2);
}

.res-bar {
  display: block;
  height: 4px;
  margin-top: 6px;
  border-radius: 2px;
  background: var(--havline-border);
  overflow: hidden;
}

.res-bar__fill {
  display: block;
  height: 100%;
  border-radius: 2px;
  background: var(--havline-brand);
  transition: width 0.3s ease;
}

.res-bar--warn .res-bar__fill {
  background: var(--havline-warning);
}

.res-bar--danger .res-bar__fill {
  background: var(--havline-error);
}

.health-panel .panel__head {
  padding: var(--havline-space-3) var(--havline-space-4) 0;
}

.health-panel .panel__body {
  flex: 1;
  padding: var(--havline-space-2) var(--havline-space-4) var(--havline-space-3);
}

.health-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
  justify-content: space-between;
  min-height: 0;
}

.health-item {
  display: grid;
  grid-template-columns: 36px minmax(0, 1fr) 76px;
  align-items: center;
  gap: 12px;
  flex: 1;
  min-height: 64px;
  padding: 12px 14px;
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
  min-width: 0;
}

.health-item__icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 17px;
  flex-shrink: 0;
}

.health-item__icon--brand {
  background: var(--havline-brand-soft);
  color: var(--havline-brand);
}

.health-item__icon--blue {
  background: rgba(59, 130, 246, 0.12);
  color: #3b82f6;
}

.health-item__icon--teal {
  background: rgba(20, 184, 166, 0.12);
  color: #14b8a6;
}

.health-item__icon--green {
  background: rgba(16, 185, 129, 0.12);
  color: #10b981;
}

.health-item__icon--error {
  background: color-mix(in srgb, var(--havline-error) 12%, transparent);
  color: var(--havline-error);
}

.health-item__main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.health-item :deep(.status-badge) {
  justify-self: end;
  width: 100%;
  justify-content: center;
  white-space: nowrap;
}

.health-item__name {
  font-size: 14px;
  font-weight: 600;
  color: var(--havline-text);
  line-height: 1.2;
}

.health-item__meta {
  font-size: 12px;
  color: var(--havline-text-secondary);
  line-height: 1.3;
}

.health-item__detail {
  font-size: 11px;
  color: var(--havline-text-muted);
  line-height: 1.3;
}

.ip-cell {
  display: inline-block;
  white-space: nowrap;
  font-size: 12px;
}

.bottom-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--havline-space-4);
  align-items: stretch;
}

.bottom-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.bottom-card :deep(.havline-card__body) {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding-left: 0;
  padding-right: 0;
  padding-bottom: 0;
}

.bottom-card :deep(.havline-card__header) {
  padding-bottom: var(--havline-space-3);
}

.bottom-card__body {
  flex: 1;
  height: 392px;
  padding: 0;
  overflow: auto;
}

.bottom-card__body :deep(.bottom-table) {
  width: 100%;
}

.bottom-card__body :deep(.n-data-table-wrapper) {
  width: 100%;
}

.bottom-card__body :deep(.n-data-table-base-table) {
  width: 100%;
  table-layout: fixed;
}

.bottom-card__body :deep(.n-data-table-th),
.bottom-card__body :deep(.n-data-table-td) {
  padding-left: 12px;
  padding-right: 12px;
}

.bottom-card__body :deep(.n-data-table-th:first-child),
.bottom-card__body :deep(.n-data-table-td:first-child) {
  padding-left: var(--havline-space-5);
}

.bottom-card__body :deep(.n-data-table-th:last-child),
.bottom-card__body :deep(.n-data-table-td:last-child) {
  padding-right: var(--havline-space-5);
}

.nowrap-cell {
  white-space: nowrap;
}

.card-title-icon {
  font-size: 18px;
  color: var(--havline-brand);
}

.card-link {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 13px;
  color: var(--havline-text-secondary);
  text-decoration: none;
  font-weight: 500;
}

.card-link:hover {
  color: var(--havline-brand);
}

.mono {
  font-family: var(--havline-mono);
}

@media (max-width: 1199px) {
  .stats-row { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .metrics-row { grid-template-columns: 1fr 1fr; }
  .ops-row { grid-template-columns: 1fr 1fr; }
  .bottom-row { grid-template-columns: 1fr; }
}

@media (max-width: 767px) {
  .stats-row { grid-template-columns: 1fr; }
  .metrics-row { grid-template-columns: 1fr; }
  .ops-row { grid-template-columns: 1fr; }
  .health-panel { grid-column: auto; }
}
</style>
