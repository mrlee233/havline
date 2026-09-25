<template>
  <VChart class="trend-chart" :option="option" autoresize />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import VChart from 'vue-echarts'
import { useTheme } from '../composables/useTheme'
import { formatRate } from '../utils/format'

use([CanvasRenderer, LineChart, GridComponent, LegendComponent, TooltipComponent])

const props = defineProps<{
  labels: string[]
  series: { name: string; values: (number | null)[] }[]
  unit: 'percent' | 'rate'
}>()

const { isDark } = useTheme()

// 与其它图表保持同一套配色（ECharts 是色值的既有例外）
const SERIES_COLORS = ['#10b981', '#3b82f6', '#f59e0b', '#8b5cf6']

function axisValue(value: number): string {
  if (props.unit === 'percent') return `${Math.round(value)}%`
  return formatRate(value)
}

const option = computed(() => {
  const textColor = isDark.value ? '#94a3b8' : '#9ca3af'
  const splitColor = isDark.value ? '#334155' : '#e5e7eb'

  return {
    grid: { left: 4, right: 12, top: 30, bottom: 6, containLabel: true },
    legend: {
      top: 0,
      left: 0,
      icon: 'roundRect',
      itemWidth: 8,
      itemHeight: 8,
      itemGap: 14,
      textStyle: { color: textColor, fontSize: 11 },
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: isDark.value ? '#1e293b' : '#fff',
      borderColor: isDark.value ? '#334155' : '#e5e7eb',
      textStyle: { color: isDark.value ? '#f1f5f9' : '#111827', fontSize: 12 },
      valueFormatter: (value: number | null | undefined) =>
        value === null || value === undefined ? '—' : axisValue(value),
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: props.labels,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: textColor, fontSize: 11 },
    },
    yAxis: {
      type: 'value',
      min: 0,
      splitNumber: 3,
      splitLine: { lineStyle: { color: splitColor, type: 'dashed' } },
      axisLabel: { color: textColor, fontSize: 11, formatter: (v: number) => axisValue(v) },
    },
    series: props.series.map((item, index) => ({
      name: item.name,
      type: 'line',
      smooth: true,
      // 刚开始采样时只有一两个点：必须画点，否则「无线可画」看起来像没有数据
      showSymbol: props.labels.length <= 12,
      symbolSize: 5,
      // 空桶在数据里是 null：图上断开而不是补 0，避免凭空出现谷底
      connectNulls: false,
      data: item.values,
      lineStyle: { width: 2, color: SERIES_COLORS[index % SERIES_COLORS.length] },
      itemStyle: { color: SERIES_COLORS[index % SERIES_COLORS.length] },
    })),
  }
})
</script>

<style scoped>
.trend-chart {
  width: 100%;
  height: 200px;
}
</style>
