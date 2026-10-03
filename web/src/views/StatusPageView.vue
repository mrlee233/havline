<template>
  <div class="status-page">
    <div class="status-page__card">
      <header class="status-page__head">
        <h1 class="status-page__title">{{ page?.title || '服务状态' }}</h1>
        <n-button size="small" :loading="loading" @click="load">刷新</n-button>
      </header>

      <p v-if="page" class="status-page__meta">
        共 {{ page.routes.length }} 项 · 正常 {{ page.up }} · 异常 {{ page.down }} · 无法判定 {{ page.unknown }}
        <span v-if="page.checked_at">｜数据截至 {{ formatDate(page.checked_at) }}</span>
      </p>

      <div v-if="needsCode" class="status-page__code">
        <n-input v-model:value="code" placeholder="请输入访问码" @keyup.enter="load" />
        <n-button type="primary" :loading="loading" @click="load">查看</n-button>
      </div>

      <p v-else-if="error" class="status-page__error">{{ error }}</p>

      <div v-if="page && page.routes.length" class="status-page__list">
        <div v-for="route in page.routes" :key="route.domain" class="status-route">
          <div class="status-row">
            <StatusBadge :kind="kindOf(route.state)" :text="stateText(route.state)" />
            <span class="status-row__domain mono">{{ route.domain }}</span>
            <span v-if="route.source" class="status-row__source">{{ route.source === 'cloudflare' ? 'Cloudflare' : '公网反代' }}</span>
            <span class="status-row__uptime mono">{{ route.uptime_pct.toFixed(1) }}%（24h）</span>
          </div>
          <div v-if="route.incidents?.length" class="incident-list">
            <div v-for="incident in route.incidents.slice(0, 3)" :key="incident.from" class="incident-row">
              <span class="incident-row__dot" />
              <span class="mono">{{ formatDate(incident.from) }} 起</span>
              <span>{{ incident.minutes }} 分钟</span>
              <span>{{ incident.detail || incident.reason || '服务不可用' }}</span>
            </div>
          </div>
        </div>
      </div>
      <p v-else-if="page && !error" class="status-page__hint">还没有巡检记录。</p>

      <p class="status-page__hint status-page__hint--foot">本页只展示域名与可用率，由 Havline 生成。</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import StatusBadge from '../components/StatusBadge.vue'
import { api } from '../api/client'
import type { StatusPagePayload } from '../api/types'
import type { StatusKind } from '../utils/status'
import { formatDate } from '../utils/format'

const page = ref<StatusPagePayload | null>(null)
const code = ref('')
const loading = ref(false)
const error = ref('')
const needsCode = ref(false)

async function load() {
  loading.value = true
  try {
    page.value = await api.getStatusPage(code.value.trim())
    error.value = ''
    needsCode.value = false
  } catch (err) {
    page.value = null
    // 状态页关闭与访问码错误都返回 404（不向匿名访问者确认页面是否存在）
    error.value = code.value.trim() ? '访问码不正确或状态页未开启' : '状态页未开启'
    needsCode.value = true
  } finally {
    loading.value = false
  }
}

function kindOf(state: string): StatusKind {
  if (state === 'up') return 'success'
  if (state === 'down') return 'error'
  return 'warning'
}

function stateText(state: string): string {
  if (state === 'up') return '正常'
  if (state === 'down') return '异常'
  return '无法判定'
}

onMounted(load)
</script>

<style scoped>
.status-page {
  min-height: 100vh;
  display: flex;
  justify-content: center;
  padding: var(--havline-space-6) var(--havline-space-4);
  background: var(--havline-bg);
}
.status-page__card {
  width: min(680px, 100%);
  align-self: flex-start;
  padding: var(--havline-space-5);
  background: var(--havline-surface);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius);
  box-shadow: var(--havline-shadow);
}
.status-page__head { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); }
.status-page__title { margin: 0; font-size: 18px; color: var(--havline-text); }
.status-page__meta { margin: 8px 0 0; font-size: 12.5px; color: var(--havline-text-secondary); }
.status-page__code { display: flex; gap: 8px; margin-top: var(--havline-space-4); }
.status-page__error { margin: var(--havline-space-4) 0 0; font-size: 13px; color: var(--havline-error); }
.status-page__list { display: grid; gap: 6px; margin-top: var(--havline-space-4); }
.status-row {
  display: flex; align-items: center; gap: var(--havline-space-3);
  padding: 8px var(--havline-space-3);
  border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm);
}
.status-row__domain { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.status-row__source { flex: none; font-size: 11.5px; color: var(--havline-text-muted); }
.status-row__uptime { flex: none; font-size: 12.5px; color: var(--havline-text-secondary); }
.status-route { border-bottom: 1px solid var(--havline-border); }
.status-route:last-child { border-bottom: none; }
.incident-list { display: grid; gap: 5px; padding: 0 var(--havline-space-3) 10px; }
.incident-row { display: flex; align-items: center; gap: 10px; color: var(--havline-text-muted); font-size: 12px; flex-wrap: wrap; }
.incident-row__dot { width: 6px; height: 6px; border-radius: 50%; background: var(--havline-error); }
.status-page__hint { margin: var(--havline-space-3) 0 0; font-size: 12.5px; color: var(--havline-text-muted); }
.status-page__hint--foot { margin-top: var(--havline-space-5); }
</style>
