<template>
  <PageHeader title="示例页面" description="用于验证 Web 一键升级是否成功生效">
    <template #actions>
      <n-button :loading="refreshing" @click="refreshVersion">
        <template #icon><n-icon :component="RefreshOutline" /></template>
        刷新版本信息
      </n-button>
    </template>
  </PageHeader>

  <n-alert type="success" :bordered="false" class="example-alert" title="v1.5.3 示例页面验证标识已生效">
    当前页面由 v{{ version }} 提供，验证标识为 <span class="mono">{{ verificationMarker }}</span>。
    如果你是在升级后首次看到这行内容，说明前端构建、镜像重建和服务重启均已成功。
  </n-alert>

  <div class="example-proof">
    <div>
      <span class="example-proof__label">本次升级验证</span>
      <strong class="example-proof__value">{{ verificationRound }}</strong>
    </div>
    <div class="example-proof__hint">
      看到这个新区块，说明前端资源已经从旧版本切换到 v1.5.3。
    </div>
  </div>

  <div class="stats-row">
    <StatCard label="当前版本" :value="`v${version}`" sub="来自 /api/version" tone="brand">
      <template #icon><n-icon :component="CloudOutline" /></template>
    </StatCard>
    <StatCard label="升级验证" value="通过" sub="示例路由与菜单已加载" tone="green">
      <template #icon><n-icon :component="CheckmarkCircleOutline" /></template>
    </StatCard>
    <StatCard label="本次验证标识" :value="verificationMarker" sub="用于区分升级前后的前端资源" tone="blue" value-small>
      <template #icon><n-icon :component="FlashOutline" /></template>
    </StatCard>
    <StatCard label="页面状态" value="已更新" sub="v1.5.3 前端资源已加载" tone="teal">
      <template #icon><n-icon :component="CheckmarkCircleOutline" /></template>
    </StatCard>
  </div>

  <FonuCard title="本次示例页变化" subtitle="用于确认升级后的页面内容确实发生变化">
    <ul class="example-changes">
      <li>页面版本提示更新为 v1.5.3。</li>
      <li>「本次升级验证」区块和页面状态同步更新为 v1.5.3。</li>
      <li>验证标识更新为 <span class="mono">v1.5.3-example-page</span>。</li>
    </ul>
  </FonuCard>

  <FonuCard title="验证步骤" subtitle="用这个页面确认线上实例已经更新到目标版本">
    <ol class="example-steps">
      <li>在旧版本页面点击侧栏的「一键升级」。</li>
      <li>等待容器重建完成，页面自动刷新或恢复访问。</li>
      <li>打开侧栏「示例页面」，确认本页可访问。</li>
      <li>确认页面上的版本号为 <span class="mono">v{{ version }}</span>。</li>
    </ol>
  </FonuCard>

  <FonuCard title="验证结果" subtitle="用于确认当前前端资源与后端版本一致">
    <div class="example-result">
      <div><label>前端路由</label><span class="mono">/example</span></div>
      <div><label>侧栏菜单</label><span>已加载「示例页面」</span></div>
      <div><label>后端版本</label><span class="mono">{{ version }}</span></div>
      <div><label>验证标识</label><span class="mono">{{ verificationMarker }}</span></div>
      <div><label>验证状态</label><StatusBadge kind="success" text="通过" /></div>
      <div><label>最近检查</label><span>{{ checkedAt || '刚刚' }}</span></div>
    </div>
  </FonuCard>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NIcon, useMessage } from 'naive-ui'
import {
  CheckmarkCircleOutline,
  CloudOutline,
  FlashOutline,
  RefreshOutline,
} from '@vicons/ionicons5'
import { api } from '../api/client'
import FonuCard from '../components/FonuCard.vue'
import PageHeader from '../components/PageHeader.vue'
import StatCard from '../components/StatCard.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { formatDate } from '../utils/format'

const message = useMessage()
const version = ref(__APP_VERSION__)
const checkedAt = ref('')
const refreshing = ref(false)
const verificationRound = 'v1.5.3'
const verificationMarker = 'v1.5.3-example-page'

async function refreshVersion() {
  refreshing.value = true
  try {
    const result = await api.getVersion()
    version.value = result.version || version.value
    checkedAt.value = formatDate(new Date().toISOString())
  } catch (error) {
    message.error(error instanceof Error ? error.message : '读取版本失败')
  } finally {
    refreshing.value = false
  }
}

onMounted(() => {
  void refreshVersion()
})
</script>

<style scoped>
.example-alert {
  margin-bottom: var(--fonu-space-5);
}

.example-proof {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--fonu-space-5);
  margin-bottom: var(--fonu-space-5);
  padding: var(--fonu-space-4) var(--fonu-space-5);
  border: 1px solid var(--fonu-brand-soft);
  border-radius: var(--fonu-radius);
  background: var(--fonu-brand-soft);
}

.example-proof__label {
  display: block;
  font-size: 12px;
  color: var(--fonu-text-secondary);
}

.example-proof__value {
  display: block;
  margin-top: 2px;
  font-size: 28px;
  line-height: 1.2;
  color: var(--fonu-brand-text);
}

.example-proof__hint {
  max-width: 360px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--fonu-text-secondary);
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--fonu-space-4);
  margin-bottom: var(--fonu-space-5);
}

.example-steps {
  display: grid;
  gap: 10px;
  margin: 0;
  padding-left: 22px;
  color: var(--fonu-text-secondary);
  line-height: 1.7;
}

.example-changes {
  display: grid;
  gap: 10px;
  margin: 0;
  padding-left: 22px;
  color: var(--fonu-text-secondary);
  line-height: 1.7;
}

.example-result {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--fonu-space-3);
}

.example-result > div {
  display: grid;
  gap: 4px;
  padding: 12px 14px;
  border: 1px solid var(--fonu-border);
  border-radius: var(--fonu-radius-sm);
  background: var(--fonu-bg);
}

.example-result label {
  font-size: 12px;
  color: var(--fonu-text-muted);
}

@media (max-width: 640px) {
  .example-proof {
    align-items: flex-start;
    flex-direction: column;
    gap: 8px;
  }

  .example-result {
    grid-template-columns: 1fr;
  }
}
</style>
