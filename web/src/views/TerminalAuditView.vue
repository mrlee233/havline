<template>
  <PageHeader
    title="终端审计"
    description="每次 SSH 会话的发起人、来源、目标、时长与断开原因；记录保留 90 天，不落终端内容"
  >
    <template #actions>
      <n-select
        v-model:value="limit"
        size="small"
        class="audit-limit"
        :options="limitOptions"
        @update:value="load"
      />
      <n-button size="small" :loading="loading" @click="load">
        <template #icon><n-icon :component="RefreshOutline" /></template>
        刷新
      </n-button>
      <n-button size="small" type="primary" @click="openTerminal">
        <template #icon><n-icon :component="TerminalOutline" /></template>
        返回终端
      </n-button>
    </template>
  </PageHeader>

  <LoadError v-if="loadError" :message="loadError" @retry="load" />

  <HavlineCard v-else title="会话记录" :subtitle="summary">
    <EmptyState
      v-if="!loading && entries.length === 0"
      title="还没有会话记录"
      description="在「SSH 终端」里建立会话后，这里会记录谁、何时、连了哪台、多久、为什么断开。"
    >
      <template #action>
        <n-button type="primary" size="small" @click="openTerminal">去 SSH 终端</n-button>
      </template>
    </EmptyState>
    <div v-else class="audit-table-wrap">
      <n-data-table
        class="audit-table"
        :columns="columns"
        :data="entries"
        :loading="loading"
        :bordered="false"
        size="small"
        :scroll-x="1120"
        :row-key="rowKey"
      />
    </div>
  </HavlineCard>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import type { DataTableColumns } from 'naive-ui'
import { NButton, NDataTable, NIcon, NSelect, NTag } from 'naive-ui'
import { RefreshOutline, TerminalOutline } from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { api, asList } from '../api/client'
import { formatBytes, formatDate, formatDuration, formatRelativeTime } from '../utils/format'
import EmptyState from '../components/EmptyState.vue'
import HavlineCard from '../components/HavlineCard.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import type { TerminalAuditEntry } from '../api/types'

const router = useRouter()
const loading = ref(false)
const loadError = ref('')
const entries = ref<TerminalAuditEntry[]>([])
const limit = ref(100)

const limitOptions = [
  { label: '最近 50 条', value: 50 },
  { label: '最近 100 条', value: 100 },
  { label: '最近 200 条', value: 200 },
]

const summary = computed(() => {
  if (loading.value && entries.value.length === 0) return '正在读取记录…'
  if (entries.value.length === 0) return '暂无记录'
  const latest = entries.value[0]?.ended_at
  return `共 ${entries.value.length} 条 · 最近一次 ${formatRelativeTime(latest)}`
})

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const result = await api.listTerminalAudits(limit.value)
    entries.value = asList(result?.entries)
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '读取审计记录失败'
  } finally {
    loading.value = false
  }
}

function openTerminal() {
  void router.push({ name: 'terminal' })
}

// session_id 已经唯一，id 只是自增主键：优先用 id，避免极端情况下同 id 冲突
function rowKey(row: TerminalAuditEntry): string {
  return row.id ? `a-${row.id}` : row.session_id
}

// 断开原因是审计结论，用颜色区分「主动关闭」和「被动断开」
function reasonTagType(reason?: string): 'info' | 'warning' | 'default' {
  switch (reason) {
    case '用户关闭':
      return 'info'
    case '空闲超时':
    case '超过最长会话时长':
    case '前端断开':
    case '服务停止':
      return 'warning'
    default:
      return 'default'
  }
}

const columns = computed<DataTableColumns<TerminalAuditEntry>>(() => [
  {
    title: '结束时间',
    key: 'ended_at',
    width: 170,
    render: (row) => h('div', { class: 'cell-time' }, [
      h('span', { class: 'cell-time__main' }, formatDate(row.ended_at)),
      h('span', { class: 'cell-muted' }, formatRelativeTime(row.ended_at)),
    ]),
  },
  {
    title: '操作人',
    key: 'user',
    width: 170,
    render: (row) => h('div', { class: 'cell-stack' }, [
      h('strong', { class: 'cell-strong' }, row.user || '未知'),
      h('span', { class: 'mono cell-muted cell-wrap' }, row.client_ip || '-'),
    ]),
  },
  {
    title: '目标服务端',
    key: 'target',
    width: 250,
    render: (row) => h('div', { class: 'cell-stack' }, [
      h('span', { class: 'cell-wrap' }, row.target || `服务端 #${row.server_id}`),
      h('span', { class: 'mono cell-muted' }, `#${row.server_id} · ${row.session_id}`),
    ]),
  },
  {
    title: '时长',
    key: 'duration_ms',
    width: 120,
    align: 'right',
    render: (row) => h('span', { class: 'mono' }, formatDuration(row.duration_ms)),
  },
  {
    title: '断开原因',
    key: 'reason',
    width: 160,
    render: (row) =>
      h(
        NTag,
        { size: 'small', bordered: false, round: true, type: reasonTagType(row.reason) },
        () => row.reason || '未知',
      ),
  },
  {
    title: '流量',
    key: 'traffic',
    width: 170,
    render: (row) => h('div', { class: 'cell-stack cell-traffic' }, [
      h('span', { class: 'mono' }, `输入 ${formatBytes(row.bytes_in)}`),
      h('span', { class: 'mono cell-muted' }, `输出 ${formatBytes(row.bytes_out)}`),
    ]),
  },
])

onMounted(() => {
  void load()
})
</script>

<style scoped>
.audit-limit { width: 140px; }
/* 台账外框对齐「穿透规则台账」的版式 */
.audit-table-wrap {
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-surface);
  box-shadow: var(--havline-shadow);
  overflow: hidden;
}

/* 单元格由 render 创建，作用域样式必须 :deep() 才能命中表格内部节点 */
.audit-table :deep(td) { vertical-align: middle; }
.audit-table :deep(.mono) { font-family: var(--havline-mono); font-size: 13px; }
.audit-table :deep(.cell-time),
.audit-table :deep(.cell-stack) { display: grid; gap: 2px; }
.audit-table :deep(.cell-time__main),
.audit-table :deep(.cell-strong) { font-size: 13px; color: var(--havline-text); }
.audit-table :deep(.cell-muted) { font-size: 12px; color: var(--havline-text-muted); }
.audit-table :deep(.cell-wrap) { overflow-wrap: anywhere; }
</style>
