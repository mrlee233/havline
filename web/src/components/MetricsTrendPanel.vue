<template>
  <div class="trend-panel" :class="{ 'trend-panel--loading': loading }">
    <p v-if="error" class="trend-panel__hint trend-panel__hint--error">{{ error }}</p>
    <template v-else>
      <section class="trend-panel__section">
        <span class="trend-panel__label">资源占用</span>
        <MetricsTrendChart :labels="resource.labels" :series="resource.series" unit="percent" />
        <p v-if="resource.labels.length === 0" class="trend-panel__hint">
          还没有资源数据：从这一版起每 2 分钟采样一次，等一两分钟就会出现。
        </p>
      </section>

      <section class="trend-panel__section">
        <span class="trend-panel__label">隧道流量</span>
        <MetricsTrendChart :labels="traffic.labels" :series="traffic.series" unit="rate" />
        <p v-if="traffic.labels.length === 0" class="trend-panel__hint">
          还没有流量数据（需要反代规则产生过访问）。
        </p>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import MetricsTrendChart from './MetricsTrendChart.vue'
import { api } from '../api/client'
import { METRICS_RANGES } from '../constants/metrics'
import type { MetricsSeries } from '../api/types'

// 时间档与切换按钮都由父级（卡片表头）给，本组件只负责取数与绘制
const props = defineProps<{ hours: number }>()

const seriesRaw = ref<MetricsSeries[]>([])
const error = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const range = METRICS_RANGES.find((item) => item.hours === props.hours) ?? METRICS_RANGES[1]
    const result = await api.getMetricsHistory(range.hours, range.buckets)
    seriesRaw.value = result.series ?? []
    error.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : '读取指标历史失败'
  } finally {
    loading.value = false
  }
}

function formatLabel(at: string): string {
  const date = new Date(at)
  if (Number.isNaN(date.getTime())) return at
  const pad = (value: number) => String(value).padStart(2, '0')
  const time = `${pad(date.getHours())}:${pad(date.getMinutes())}`
  return props.hours <= 24 ? time : `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${time}`
}

function pick(items: MetricsSeries[], metric: string) {
  return items.find((item) => item.metric === metric)
}

// 各序列的点数不一致（空桶不产点），所以按时间戳对齐、并集做横轴
function unionLabels(items: MetricsSeries[]): string[] {
  const set = new Set<string>()
  for (const item of items) {
    for (const point of item.points) set.add(point.at)
  }
  return [...set].sort()
}

function valueByTime(series?: MetricsSeries) {
  const map = new Map<string, number>()
  for (const point of series?.points ?? []) map.set(point.at, point.value)
  return map
}

const resource = computed(() => {
  const host = seriesRaw.value.filter((item) => item.scope === 'host')
  const labels = unionLabels(host)
  const cpu = valueByTime(pick(host, 'cpu_percent'))
  const memUsed = valueByTime(pick(host, 'mem_used_bytes'))
  const memTotal = valueByTime(pick(host, 'mem_total_bytes'))
  const diskUsed = valueByTime(pick(host, 'disk_used_bytes'))
  const diskTotal = valueByTime(pick(host, 'disk_total_bytes'))

  const percentOf = (used: Map<string, number>, total: Map<string, number>, at: string) => {
    const u = used.get(at)
    const t = total.get(at)
    if (u === undefined || t === undefined || t <= 0) return null
    return (u / t) * 100
  }

  return {
    labels: labels.map(formatLabel),
    series: [
      { name: 'CPU', values: labels.map((at) => cpu.get(at) ?? null) },
      { name: '内存', values: labels.map((at) => percentOf(memUsed, memTotal, at)) },
      { name: '磁盘', values: labels.map((at) => percentOf(diskUsed, diskTotal, at)) },
    ],
  }
})

const traffic = computed(() => {
  const routes = seriesRaw.value.filter((item) => item.scope.startsWith('route:'))
  const labels = unionLabels(routes)
  const sumByTime = (metric: string) => {
    const totals = new Map<string, number>()
    for (const item of routes) {
      if (item.metric !== metric) continue
      for (const point of item.points) {
        totals.set(point.at, (totals.get(point.at) ?? 0) + point.value)
      }
    }
    return totals
  }
  const upload = sumByTime('traffic_in_rate')
  const download = sumByTime('traffic_out_rate')
  return {
    labels: labels.map(formatLabel),
    series: [
      { name: '上传', values: labels.map((at) => upload.get(at) ?? null) },
      { name: '下载', values: labels.map((at) => download.get(at) ?? null) },
    ],
  }
})

onMounted(load)
watch(() => props.hours, load)
</script>

<style scoped>
.trend-panel { display: flex; flex-direction: column; gap: var(--havline-space-4); }
.trend-panel--loading { opacity: 0.6; }
/* 每块自带小标题，卡片被同行撑高时也不会显得内容散掉 */
.trend-panel__section { display: flex; flex-direction: column; gap: 6px; }
.trend-panel__label { font-size: 12px; font-weight: 600; color: var(--havline-text-secondary); }
.trend-panel__hint { margin: 0; font-size: 12.5px; color: var(--havline-text-muted); }
.trend-panel__hint--error { color: var(--havline-error); }
</style>
