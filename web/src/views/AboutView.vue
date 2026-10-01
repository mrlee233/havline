<template>
  <PageHeader title="关于项目" description="查看 Havline 版本、源开源项目和技术信息">
    <template #actions>
      <n-button :loading="refreshing" @click="refreshVersion">
        <template #icon><n-icon :component="RefreshOutline" /></template>
        刷新版本信息
      </n-button>
    </template>
  </PageHeader>

  <div class="stats-row">
    <StatCard label="当前版本" :value="`v${version}`" sub="来自 /api/version" tone="brand">
      <template #icon><n-icon :component="CloudOutline" /></template>
    </StatCard>
    <StatCard label="源项目" :value="GITHUB_REPO" sub="上游开源项目地址" tone="blue" value-small>
      <template #icon><n-icon :component="LogoGithub" /></template>
    </StatCard>
    <StatCard label="开源协议" value="GPL-3.0" sub="GNU General Public License" tone="green">
      <template #icon><n-icon :component="DocumentTextOutline" /></template>
    </StatCard>
    <StatCard label="运行状态" value="正常" sub="Havline 服务运行中" tone="teal">
      <template #icon><n-icon :component="CheckmarkCircleOutline" /></template>
    </StatCard>
  </div>

  <HavlineCard title="项目信息" subtitle="当前版本、项目定位和最近检查时间">
    <div class="about-grid">
      <div class="about-item">
        <label>项目名称</label>
        <strong>Havline</strong>
      </div>
      <div class="about-item">
        <label>项目定位</label>
        <span>面向 NAS 的轻量公网访问管理工具</span>
      </div>
      <div class="about-item">
        <label>当前版本</label>
        <span class="mono">v{{ version }}</span>
      </div>
      <div class="about-item">
        <label>最近检查</label>
        <span>{{ checkedAt || '刚刚' }}</span>
      </div>
    </div>

    <div class="about-links">
      <a :href="GITHUB_REPO_URL" target="_blank" rel="noopener noreferrer">
        <n-icon :component="LogoGithub" />
        源项目：{{ GITHUB_REPO }}
      </a>
      <a :href="GPL_URL" target="_blank" rel="noopener noreferrer">
        <n-icon :component="DocumentTextOutline" />
        查看 GPL-3.0 协议
      </a>
    </div>
  </HavlineCard>

  <HavlineCard title="源项目与开源说明" subtitle="Havline 的源项目地址保持不变">
    <div class="about-copy">
      <p>
        Havline 的源开源项目为
        <a :href="GITHUB_REPO_URL" target="_blank" rel="noopener noreferrer">{{ GITHUB_REPO }}</a>。
        该地址用于查阅上游代码、提交记录和 Release 信息。
      </p>
      <p>
        本项目继续采用
        <a :href="GPL_URL" target="_blank" rel="noopener noreferrer">GPL-3.0</a>
        开源。品牌名称调整不会改变原项目及其衍生版本的许可证义务。
      </p>
    </div>
  </HavlineCard>

  <HavlineCard title="技术栈" subtitle="Havline 当前使用的主要技术组件">
    <div class="about-stack">
      <span v-for="item in stack" :key="item">{{ item }}</span>
    </div>
  </HavlineCard>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton, NIcon, useMessage } from 'naive-ui'
import {
  CheckmarkCircleOutline,
  CloudOutline,
  DocumentTextOutline,
  LogoGithub,
  RefreshOutline,
} from '@vicons/ionicons5'
import { api } from '../api/client'
import HavlineCard from '../components/HavlineCard.vue'
import PageHeader from '../components/PageHeader.vue'
import StatCard from '../components/StatCard.vue'
import { GITHUB_REPO, GITHUB_REPO_URL } from '../constants/app'
import { formatDate } from '../utils/format'

const GPL_URL = 'https://www.gnu.org/licenses/gpl-3.0.html'
const message = useMessage()
const version = ref(__APP_VERSION__)
const checkedAt = ref('')
const refreshing = ref(false)
const stack = [
  'Go 1.23',
  'Vue 3',
  'TypeScript',
  'Vite',
  'Naive UI',
  'ECharts',
  'SQLite',
  'Nginx',
  'FRP',
  'Cloudflare Tunnel',
]

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
.about-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--havline-space-4);
}

.about-item {
  min-width: 0;
  padding: var(--havline-space-4);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
}

.about-item label {
  display: block;
  margin-bottom: 6px;
  color: var(--havline-text-muted);
  font-size: 12px;
}

.about-item strong,
.about-item span {
  color: var(--havline-text);
}

.about-links,
.about-stack {
  display: flex;
  flex-wrap: wrap;
  gap: var(--havline-space-3);
  margin-top: var(--havline-space-4);
}

.about-links a {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  color: var(--havline-brand-text);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-surface);
  text-decoration: none;
}

.about-copy {
  display: grid;
  gap: var(--havline-space-3);
  color: var(--havline-text-secondary);
}

.about-copy p {
  margin: 0;
}

.about-copy a {
  color: var(--havline-brand-text);
  text-decoration: none;
}

.about-stack span {
  padding: 6px 10px;
  color: var(--havline-text-secondary);
  border: 1px solid var(--havline-border);
  border-radius: 999px;
  background: var(--havline-bg);
  font-size: 12px;
}
</style>
