<template>
  <n-modal v-model:show="show" :mask-closable="false">
    <div class="update-modal">
      <div class="update-modal__header">
        <div>
          <h3 class="update-modal__title">升级 Havline</h3>
          <p class="update-modal__subtitle">升级完成后容器会重启，数据目录不会被删除</p>
        </div>
        <n-button quaternary size="small" :disabled="applying" @click="show = false">关闭</n-button>
      </div>

      <div class="update-modal__body">
        <n-alert :type="alertType" :bordered="false" class="update-modal__alert">
          {{ displayMessage }}
        </n-alert>

        <div class="update-modal__grid">
          <div>
            <span class="update-modal__label">当前版本</span>
            <strong class="mono">{{ status?.current_version || appVersion || '未知' }}</strong>
          </div>
          <div>
            <span class="update-modal__label">目标版本</span>
            <strong class="mono">{{ status?.latest_version || '检查中…' }}</strong>
          </div>
          <div>
            <span class="update-modal__label">升级方式</span>
            <strong>{{ modeLabel }}</strong>
          </div>
          <div>
            <span class="update-modal__label">服务状态</span>
            <strong>{{ enabledLabel }}</strong>
          </div>
        </div>

        <p class="update-modal__hint">
          升级会重新构建应用镜像并只重建 <span class="mono">havline</span> 容器。构建期间管理页面可能短暂无法访问，
          浏览器会自动等待服务恢复。
        </p>

        <pre v-if="logText" class="update-modal__log">{{ logText }}</pre>
      </div>

      <div class="update-modal__footer">
        <n-button :disabled="applying" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="applying || loading" :disabled="!canApply" @click="startUpdate">
          {{ applying ? '正在升级…' : '确认升级' }}
        </n-button>
      </div>
    </div>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NAlert, NButton, NModal, useMessage } from 'naive-ui'
import { api } from '../api/client'
import { useVisibilityPolling } from '../composables/useVisibilityPolling'
import type { UpdateStatus } from '../api/types'

const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ updated: [] }>()
const message = useMessage()

const status = ref<UpdateStatus | null>(null)
const appVersion = ref(__APP_VERSION__)
const loading = ref(false)
const applying = ref(false)
const localMessage = ref('')

const modeLabel = computed(() => {
  if (status.value?.mode === 'git') return 'Git 仓库构建'
  if (status.value?.mode === 'local') return '本机源码构建'
  return '未启用'
})

const enabledLabel = computed(() => (status.value?.enabled ? '侧车可用' : '不可用'))

const displayMessage = computed(() => {
  if (localMessage.value) return localMessage.value
  if (!status.value) return loading.value ? '正在检查更新状态…' : '尚未检查更新状态'
  return status.value.message || '未知状态'
})

const alertType = computed(() => {
  if (status.value?.phase === 'failed') return 'error'
  if (status.value?.phase === 'success') return 'success'
  if (applying.value || status.value?.busy) return 'warning'
  return status.value?.update_available ? 'info' : 'default'
})

const logText = computed(() => (status.value?.log ?? []).slice(-20).join('\n'))

const canApply = computed(() => Boolean(
  status.value?.enabled &&
  status.value.update_available &&
  !applying.value &&
  !status.value.busy
))

async function loadStatus() {
  loading.value = true
  localMessage.value = ''
  try {
    status.value = await api.getUpdateStatus()
  } catch (error) {
    localMessage.value = error instanceof Error ? error.message : '读取升级状态失败'
  } finally {
    loading.value = false
  }
}

async function startUpdate() {
  applying.value = true
  localMessage.value = '升级任务已启动，正在等待容器重建…'
  try {
    status.value = await api.applyUpdate()
  } catch (error) {
    applying.value = false
    localMessage.value = error instanceof Error ? error.message : '启动升级失败'
  }
}

async function pollStatus() {
  try {
    status.value = await api.getUpdateStatus()
    localMessage.value = ''
    if (status.value.phase === 'success') {
      applying.value = false
      message.success('Havline 已升级完成')
      emit('updated')
      window.setTimeout(() => window.location.reload(), 1500)
      return
    }
    if (status.value.phase === 'failed') {
      applying.value = false
      message.error(status.value.message || '升级失败')
    }
  } catch {
    localMessage.value = '服务正在重启，等待恢复…'
  }
}

watch(show, (visible) => {
  if (!visible) return
  void loadStatus()
})

useVisibilityPolling(pollStatus, 2000, { enabled: applying })
</script>

<style scoped>
.update-modal {
  width: 560px;
  max-width: 94vw;
  background: var(--havline-surface);
  border-radius: var(--havline-radius);
  box-shadow: var(--havline-shadow-md);
  overflow: hidden;
}

.update-modal__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--havline-space-3);
  padding: var(--havline-space-5) var(--havline-space-5) var(--havline-space-3);
}

.update-modal__title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--havline-text);
}

.update-modal__subtitle {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--havline-text-muted);
}

.update-modal__body {
  display: grid;
  gap: var(--havline-space-4);
  padding: 0 var(--havline-space-5) var(--havline-space-4);
}

.update-modal__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--havline-space-3);
}

.update-modal__grid > div {
  display: grid;
  gap: 3px;
  padding: 10px 12px;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
}

.update-modal__label {
  font-size: 12px;
  color: var(--havline-text-muted);
}

.update-modal__hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--havline-text-secondary);
}

.update-modal__log {
  max-height: 220px;
  margin: 0;
  padding: 12px;
  overflow: auto;
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg-muted);
  color: var(--havline-text-secondary);
  font-family: var(--havline-mono);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
}

.update-modal__footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--havline-space-3);
  padding: var(--havline-space-4) var(--havline-space-5);
  border-top: 1px solid var(--havline-border);
}

@media (max-width: 640px) {
  .update-modal__grid {
    grid-template-columns: 1fr;
  }
}
</style>
