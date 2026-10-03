// 趋势图的时间档：后端按 hours 取窗口、按 buckets 取桶数（点位在该数量级最清楚）
export interface MetricsRange {
  label: string
  hours: number
  buckets: number
}

export const METRICS_RANGES: MetricsRange[] = [
  { label: '1 小时', hours: 1, buckets: 30 },
  { label: '24 小时', hours: 24, buckets: 48 },
  { label: '7 天', hours: 24 * 7, buckets: 84 },
]
