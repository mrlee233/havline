<template>
  <LoadError v-if="loadError" :message="loadError" @retry="load" />

  <template v-else>
    <!-- 顶部不写问候语，直接给结论：一眼看到「几项正常 / 哪条链路有问题」，
         右侧把公网访问链路按 入口 → DDNS → 证书 → Nginx → 穿透 排出来 -->
    <section class="hero">
      <div class="hero__main">
        <div class="hero__lead">
          <div class="hero__topline">
            <span class="hero__medal" :class="{ 'hero__medal--warn': !coreHealthy }">
              <n-icon :component="coreHealthy ? CheckmarkOutline : AlertCircleOutline" />
            </span>
            <span class="hero__kicker">
              <i :class="{ 'is-warn': !coreHealthy }"></i>
              {{ coreHealthy ? 'LIVE · 核心链路正常' : 'DEGRADED · 核心链路异常' }}
            </span>
          </div>
          <h1 class="hero__title">{{ heroTitle }}</h1>
          <p class="hero__sub">{{ heroSub }}</p>
          <div class="hero__actions">
            <button
              v-if="attentionCount > 0"
              type="button"
              class="hero__alert"
              :title="attentionHint"
              @click="goToAttention"
            >
              <n-icon :component="AlertCircleOutline" />
              处理 {{ attentionCount }} 项提醒
            </button>
            <button type="button" class="hero__ghost" @click="load">
              <n-icon :component="PulseOutline" />
              刷新状态
            </button>
            <button type="button" class="hero__primary" @click="router.push({ name: 'proxies' })">
              <n-icon :component="AddOutline" />
              新建反代
            </button>
          </div>
        </div>

        <div class="hero__edge">
          <div class="hero__edge-head">
            <span>PUBLIC EDGE / 公网访问链路</span>
            <span>{{ linkWarningCount }} 项提醒 · {{ linkNodes.length }} 个节点</span>
          </div>
          <div class="edge">
            <div
              v-for="node in linkNodes"
              :key="node.name"
              class="edge__node"
              :class="{ 'edge__node--warn': node.warn }"
            >
              <span class="edge__dot"></span>
              <span class="edge__name">{{ node.name }}</span>
              <span class="edge__value mono">{{ node.value }}</span>
              <span class="edge__state">{{ node.state }}</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <n-spin :show="loading && !status">
      <div class="stats-row">
        <StatCard label="公网 IP" tone="blue">
          <template #icon><n-icon :component="GlobeOutline" /></template>
          <template #extra><StatusBadge v-if="publicIPEntries.length > 0" value="ok" text="正常" /></template>
          <template #value>
            <!-- 多个 DDNS 服务商可能对应不同的出口 IP（家宽/多线/VPS），
                 只显示「最后更新的那条」会让人以为只配了一个。
                 这里按配置逐条列出，并标出来源名称。 -->
            <ul v-if="publicIPEntries.length > 0" class="ip-list">
              <li v-for="item in publicIPEntries" :key="item.key" class="ip-list__row">
                <span class="ip-list__addr mono">{{ item.address }}</span>
                <span class="ip-list__tag">{{ item.label }}</span>
              </li>
            </ul>
            <span v-else class="stat-primary mono">-</span>
          </template>
          <p class="stat-desc">{{ publicIPSecondaryLabel }}</p>
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
import { useRouter } from 'vue-router'
import { NButton, NDataTable, NIcon, NSpin, NTag, useMessage, type DataTableColumns } from 'naive-ui'
import {
  AddOutline,
  AlertCircleOutline,
  AppsOutline,
  CheckmarkCircleOutline,
  CheckmarkOutline,
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
const router = useRouter()
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

// 顶部主视觉的三段结论：
// 1) 核心服务几项在线（标题）
// 2) 一句话说清「哪条链路有问题」或「一切正常」（副标题）
// 3) 需要处理的提醒数量（按钮）
//
// 「未配置」不算故障：新装用户一点 DDNS/证书/隧道都没配时，
// 如果按「非 running 即异常」判定，顶部会直接显示降级，看起来像系统坏了。
function isProblemStatus(status?: string): boolean {
  if (!status) return false
  return status !== 'running' && status !== 'ok' && status !== 'none'
}

const badHealthItems = computed(() => healthItems.value.filter(
  (item) => isProblemStatus(item.status),
))

const coreHealthy = computed(() => badHealthItems.value.length === 0)

const heroTitle = computed(() => {
  const items = healthItems.value
  const bad = badHealthItems.value
  if (bad.length === 0) return `${items.length} 个核心服务全部在线。`
  return `${items.length - bad.length}/${items.length} 个核心服务在线。`
})

const heroSub = computed(() => {
  const bad = badHealthItems.value
  const proxyCount = status.value?.proxy_count ?? proxyRules.value.length
  const certDays = status.value?.certificate_days ?? primaryCert.value?.days_left ?? 0
  const certClause = certDays > 0 && certDays <= 14
    ? `有 1 张证书将在 ${certDays} 天后到期。`
    : ''
  if (bad.length === 0) {
    return `Havline、Nginx、DDNS 与证书服务运行正常；${proxyCount} 条反代规则已生效。${certClause}`
  }
  return `${bad.map((item) => item.name).join('、')} 状态异常；${proxyCount} 条反代规则。${certClause}`
})

// 需要提醒的项：服务异常 + 证书临期，作为顶部「处理 N 项提醒」的来源
const attentionCount = computed(() => attentionTargets.value.length)

// 提醒各自的跳转目标。
//
// 之前这里写死跳证书页，但提醒可能来自 Nginx 停止、DDNS 未同步、公网反代健康下降、
// Cloudflare 隧道异常——统一跳证书页等于把用户送到一个和问题无关的页面。
// 这里按异常项推导出「去哪修」，按钮文案也带上第一个问题的名字。
const attentionTargets = computed(() => {
  const out: { label: string; routeName: string }[] = []
  for (const item of badHealthItems.value) {
    const routeName = attentionRouteFor(item.name)
    if (routeName) out.push({ label: item.name, routeName })
  }
  const certDays = status.value?.certificate_days ?? primaryCert.value?.days_left ?? 0
  if (certDays > 0 && certDays <= 14) {
    out.push({ label: `证书 ${certDays} 天后到期`, routeName: 'certificates' })
  }
  return out
})

function attentionRouteFor(name: string): string {
  switch (name) {
    case 'Nginx':
      return 'proxies'
    case 'DDNS':
      return 'ddns'
    case 'HTTPS 证书':
      return 'certificates'
    case '公网反代':
    case '公网服务端':
      return 'public-proxy'
    case 'Cloudflare 隧道':
      return 'cloudflare'
    default:
      // Havline 自身异常没有可跳转的页面，就不提供入口
      return ''
  }
}

const attentionHint = computed(() => {
  const first = attentionTargets.value[0]
  return first ? `处理「${first.label}」` : ''
})

function goToAttention() {
  const first = attentionTargets.value[0]
  if (first) {
    router.push({ name: first.routeName })
  }
}

// 「未配置」不算提醒：新装用户一点 DDNS/证书都没配时，
// 链路图会全程黄点，看起来像系统坏了。这里只统计「已配置但状态不对」的段落。
const linkWarningCount = computed(() => linkNodes.value.filter((n) => n.warn && !n.unconfigured).length)

// 公网访问链路：入口 → DDNS → 证书 → Nginx → 穿透，每段给出实际值，异常段标黄
const linkNodes = computed(() => {
  const ip = status.value?.public_ipv4 || '-'
  const ddnsCount = status.value?.ddns_count ?? ddnsConfigs.value.length
  const recordCount = ddnsConfigs.value.reduce(
    (sum, cfg) => sum + (cfg.record_names?.length || (cfg.record_name ? 1 : 0)),
    0,
  )
  const certCount = status.value?.certificate_count ?? certificates.value.length
  const certDays = status.value?.certificate_days ?? primaryCert.value?.days_left ?? 0
  const proxyCount = status.value?.proxy_count ?? proxyRules.value.length
  const enabledCount = proxyRules.value.filter((rule) => rule.enabled).length
  const frpCount = Object.keys(frpProxyNames.value).length
  const ddnsOk = status.value?.ddns_status === 'ok'
  const nginxOk = status.value?.nginx_status === 'running'

  return [
    {
      name: '公网入口',
      value: ip,
      state: ip === '-' ? '未检测到' : '可达',
      warn: ip === '-',
      unconfigured: ip === '-',
    },
    {
      name: 'DDNS',
      value: ddnsCount > 0 ? `${ddnsCount} 条配置 · ${recordCount} 条记录` : '未配置',
      state: ddnsOk ? '已同步' : ddnsCount > 0 ? '待同步' : '未配置',
      warn: !ddnsOk && ddnsCount > 0,
      unconfigured: ddnsCount === 0,
    },
    {
      name: 'HTTPS 证书',
      value: certCount > 0 ? `${certCount} 张` : '未申请',
      state: certDays > 0 ? `${certDays} 天到期` : certCount > 0 ? '已过期' : '未申请',
      // 已配置但临期/过期才算提醒；一张证书都没有时不标黄
      warn: certCount > 0 && certDays <= 14,
      unconfigured: certCount === 0,
    },
    {
      name: 'Nginx',
      value: `${proxyCount} 条规则 · ${enabledCount} 启用`,
      state: nginxOk ? '运行中' : '已停止',
      warn: !nginxOk,
      unconfigured: false,
    },
    {
      name: 'FRP 穿透',
      value: frpCount > 0 ? `${frpCount} 条隧道` : '未配置',
      state: frpCount > 0 ? '已连接' : '未配置',
      warn: false,
      unconfigured: frpCount === 0,
    },
  ]
})

const primaryCert = computed(() => {
  if (certificates.value.length === 0) return null
  return [...certificates.value].sort((a, b) => a.days_left - b.days_left)[0]
})
const certWildcard = computed(() => {
  const domains = primaryCert.value?.domains ?? []
  return domains.find((d) => d.startsWith('*.')) ?? (primaryCert.value?.wildcard ? `*.${primaryCert.value.domain}` : '')
})
const certDisplayName = computed(() => certWildcard.value || primaryCert.value?.domain || '')

// 公网 IP 逐条列出。
//
// 后端 /api/status 只给一条 public_ipv4（取「最后更新的那条 DDNS 配置」），
// 多条配置各自的 IP 在 ddnsConfigs 里。这里按配置聚合去重：
// 同一出口 IP 被多个域名共用时只显示一次，标签合并成「名称1 / 名称2」，
// 避免卡片被重复的同一串 IP 撑满。
type PublicIPEntry = { key: string; address: string; label: string }

const publicIPEntries = computed<PublicIPEntry[]>(() => {
  const byAddress = new Map<string, string[]>()
  for (const cfg of ddnsConfigs.value) {
    if (!cfg.enabled) continue
    // 自定义 IP 优先生效（配置了就按它同步），其次才是上次同步到的 IP
    const v4 = (cfg.custom_ipv4 || cfg.last_ipv4 || '').trim()
    const v6 = (cfg.custom_ipv6 || cfg.last_ipv6 || '').trim()
    const label = cfg.name?.trim() || cfg.root_domain || `配置 #${cfg.id}`
    for (const address of [v4, v6]) {
      if (!address) continue
      const labels = byAddress.get(address) ?? []
      if (!labels.includes(label)) labels.push(label)
      byAddress.set(address, labels)
    }
  }

  // 没有任何 DDNS 配置时，退回后端探测到的出口 IP（未配置 DDNS 的场景）
  if (byAddress.size === 0) {
    const fallback = (status.value?.public_ipv4 || status.value?.public_ipv6 || '').trim()
    if (fallback) {
      const how = status.value?.public_ip_source === 'detect' ? '出口探测' : 'DDNS'
      return [{ key: fallback, address: fallback, label: how }]
    }
    return []
  }

  return [...byAddress.entries()].map(([address, labels]) => ({
    key: address,
    address,
    label: labels.join(' / '),
  }))
})

// 副行：配置了多个 IP 时说明总数，只有 IPv6 时提示未获取 IPv4
const publicIPSecondaryLabel = computed(() => {
  const entries = publicIPEntries.value
  if (entries.length === 0) return '暂无公网 IP'
  if (entries.length > 1) return `共 ${entries.length} 个出口地址`
  const hasIPv6 = entries.some((item) => item.address.includes(':'))
  if (status.value?.public_ip_source === 'ddns' && !hasIPv6 && !status.value?.public_ipv6) {
    return '未获取 IPv6'
  }
  return ''
})

const publicIPSourceLabel = computed(() => {
  const enabledCount = ddnsConfigs.value.filter((cfg) => cfg.enabled).length
  switch (status.value?.public_ip_source) {
    case 'ddns':
      if (enabledCount > 1) return `来源：${enabledCount} 条 DDNS 配置`
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
/* 顶部主视觉：左侧状态结论 + 右侧公网访问链路（两条栏在 1080px 以下堆叠） */
.hero {
  position: relative;
  overflow: hidden;
  margin-bottom: var(--havline-space-4);
  padding: 24px 26px;
  border: 1px solid var(--havline-border);
  border-radius: 24px;
  background:
    radial-gradient(720px 320px at 90% -30%, rgba(16, 185, 129, 0.1), transparent 60%),
    var(--havline-surface);
  box-shadow: 0 26px 60px -50px rgba(6, 78, 59, 0.45), var(--havline-shadow);
}

.hero__main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.05fr);
  gap: 34px;
  align-items: center;
}

.hero__lead {
  min-width: 0;
}

.hero__topline {
  display: flex;
  align-items: center;
  gap: 14px;
}

.hero__medal {
  display: grid;
  place-items: center;
  width: 46px;
  height: 46px;
  flex: none;
  border: 1px solid rgba(16, 185, 129, 0.28);
  border-radius: 50%;
  background: var(--havline-brand-soft);
  color: var(--havline-brand-text);
  box-shadow: inset 0 0 0 5px var(--havline-surface);
}

.hero__medal .n-icon {
  font-size: 20px;
}

.hero__medal--warn {
  border-color: rgba(245, 158, 11, 0.4);
  background: rgba(245, 158, 11, 0.12);
  color: var(--havline-warning);
}

.hero__kicker {
  display: flex;
  align-items: center;
  gap: 7px;
  font-family: var(--havline-mono);
  font-size: 10px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--havline-text-muted);
}

.hero__kicker i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--havline-brand);
}

.hero__kicker i.is-warn {
  background: var(--havline-warning);
}

.hero__title {
  margin: 16px 0 0;
  font-size: 32px;
  font-weight: 750;
  letter-spacing: -0.035em;
  line-height: 1.1;
  color: var(--havline-text);
}

.hero__sub {
  margin: 9px 0 0;
  max-width: 46ch;
  font-size: 12.5px;
  line-height: 1.65;
  color: var(--havline-text-muted);
}

.hero__actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 20px;
}

.hero__alert,
.hero__ghost,
.hero__primary {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 13px;
  border-radius: 9px;
  font-size: 12px;
  font-weight: 600;
  font-family: inherit;
  cursor: pointer;
  transition: border-color 0.15s ease, color 0.15s ease, opacity 0.15s ease;
}

.hero__alert {
  border: 1px solid rgba(245, 158, 11, 0.35);
  background: rgba(245, 158, 11, 0.12);
  color: var(--havline-warning);
  font-weight: 650;
}

.hero__alert:hover {
  border-color: var(--havline-warning);
}

.hero__ghost {
  border: 1px solid var(--havline-border);
  background: var(--havline-surface);
  color: var(--havline-text-secondary);
}

.hero__ghost:hover {
  border-color: var(--havline-brand);
  color: var(--havline-text);
}

.hero__primary {
  border: 1px solid transparent;
  background: var(--havline-brand);
  color: #fff;
}

.hero__primary:hover {
  background: var(--havline-brand-hover);
}

.hero__alert .n-icon,
.hero__ghost .n-icon,
.hero__primary .n-icon {
  font-size: 13px;
}

/* ------------------------- 右侧公网访问链路 ------------------------- */

.hero__edge {
  min-width: 0;
  padding: 14px 16px 16px;
  border: 1px solid var(--havline-border);
  border-radius: 16px;
  background: var(--havline-bg-muted);
}

/* 顶部元信息行：左标题 + 右侧统计，两端分别贴住容器左右边界。
   容器宽度与下方的 flex 链路一致，所以文字与首尾圆点自然对齐。 */
.hero__edge-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-family: var(--havline-mono);
  font-size: 9px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--havline-text-muted);
}

.hero__edge-head > :first-child {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hero__edge-head > :last-child {
  white-space: nowrap;
}

/* 链路节点：5 个节点用 flex 等距分布，首尾贴住容器两端。
   不能用 5 等分 grid —— 那样第一个圆点在 0%、最后一个在 80%，
   右端会空出近一整列，看起来左边顶满、右边悬空。 */
.edge {
  --edge-gap: 12px;
  --edge-dot: 9px;
  position: relative;
  display: flex;
  justify-content: space-between;
  gap: var(--edge-gap);
  margin-top: 20px;
}

.edge__node {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 2px;
  /* 等宽分配剩余空间：连接线要靠这个宽度精确接到下一个圆点 */
  flex: 1 1 0;
  min-width: 0;
  padding-right: 0;
  text-align: left;
}

/* 最后一个节点不参与等宽拉伸，贴住右边界，两端留白才对称。
   注意圆点仍要左对齐：它前面还要接上一段的连接线，右对齐会让线断开。 */
.edge__node:last-child {
  flex: 0 0 auto;
  align-items: flex-start;
  text-align: left;
}

/* 连接线：每个节点各画一小段，从自己的圆点连到下一个圆点。
   - left 取圆点半径，从圆心起步（圆点 z-index 更高，会盖住重合部分）
   - right 取负的「gap + 圆点半径」，跨过 flex 间隙接到下一个圆心；
     只写 -半径 的话线会在间隙处断开，看起来像几段悬空的短线。 */
.edge__node:not(:last-child)::after {
  content: '';
  position: absolute;
  top: calc(var(--edge-dot) / 2 - 0.5px);
  left: calc(var(--edge-dot) / 2);
  right: calc(-1 * (var(--edge-gap) + var(--edge-dot) / 2));
  height: 1px;
  background: rgba(16, 185, 129, 0.35);
}

.edge__dot {
  position: relative;
  z-index: 1;
  width: var(--edge-dot);
  height: var(--edge-dot);
  margin-bottom: 10px;
  border-radius: 50%;
  background: var(--havline-brand);
  box-shadow: 0 0 0 4px var(--havline-bg-muted);
}

.edge__node--warn .edge__dot {
  background: var(--havline-warning);
}

.edge__name {
  font-size: 11.5px;
  font-weight: 650;
  color: var(--havline-text);
}

.edge__value {
  overflow: hidden;
  font-size: 9.5px;
  color: var(--havline-text-muted);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.edge__state {
  font-size: 9.5px;
  color: var(--havline-text-muted);
}

.edge__node--warn .edge__state {
  color: var(--havline-warning);
}

@media (max-width: 1080px) {
  .hero__main {
    grid-template-columns: minmax(0, 1fr);
    gap: 22px;
  }

  .hero__edge {
    padding: 12px 14px 14px;
  }
}

@media (max-width: 720px) {
  .hero {
    padding: 18px;
  }

  .hero__title {
    font-size: 24px;
  }

  .hero__medal {
    width: 38px;
    height: 38px;
  }

  .hero__medal .n-icon {
    font-size: 17px;
  }

  .hero__actions {
    margin-top: 16px;
  }

  .hero__actions > * {
    flex: 1 1 auto;
    justify-content: center;
  }

  /* 窄屏改成两列换行：节点不再「一行串起来」，连线会指向错的地方，直接去掉 */
  .edge {
    --edge-gap: 16px;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 14px var(--edge-gap);
    margin-top: 16px;
  }

  .edge__node,
  .edge__node:last-child {
    flex: initial;
    align-items: flex-start;
    text-align: left;
  }

  .edge__node:not(:last-child)::after {
    display: none;
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

/* 公网 IP 逐条列表：地址与来源标签同一行，地址过长时省略而不是换行，
   保证多条 IP 时卡片高度可控。 */
.ip-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.ip-list__row {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.ip-list__addr {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  font-size: 18px;
  font-weight: 700;
  line-height: 1.3;
  color: var(--havline-text);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ip-list__tag {
  flex: 0 0 auto;
  padding: 1px 6px;
  border-radius: 6px;
  background: var(--havline-bg-muted);
  font-size: 10px;
  color: var(--havline-text-muted);
  white-space: nowrap;
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
