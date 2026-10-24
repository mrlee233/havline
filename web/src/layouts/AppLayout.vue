<template>
  <n-layout has-sider class="app-layout">
    <n-layout-sider
      class="app-sider"
      bordered
      collapse-mode="width"
      :collapsed-width="72"
      :width="240"
      :collapsed="collapsed"
      show-trigger="bar"
      @collapse="collapsed = true"
      @expand="collapsed = false"
    >
      <router-link class="brand" :class="{ 'brand--collapsed': collapsed }" :to="{ name: 'dashboard' }">
        <img class="brand__logo" :src="brandLogo" alt="Havline" />
        <div v-if="!collapsed" class="brand__text">
          <div class="brand__name">Havline</div>
          <div class="brand__tagline">让 NAS 访问更简单</div>
        </div>
      </router-link>

      <!-- 导航按业务域分组：12 个平铺项不容易扫读，分组后能按「概览 / 网络 / 运维 / 系统」定位 -->
      <nav class="nav" aria-label="主导航">
        <template v-for="group in menuGroups" :key="group.label">
          <div v-if="!collapsed" class="nav__group">{{ group.label }}</div>
          <div v-else class="nav__divider" aria-hidden="true"></div>
          <button
            v-for="item in group.items"
            :key="item.key"
            type="button"
            class="nav__item"
            :class="{ 'nav__item--active': activeKey === item.key, 'nav__item--icon-only': collapsed }"
            :title="collapsed ? item.label : undefined"
            :aria-current="activeKey === item.key ? 'page' : undefined"
            @click="handleMenu(item.key)"
          >
            <n-icon class="nav__icon" :component="item.icon" />
            <span v-if="!collapsed" class="nav__label">{{ item.label }}</span>
          </button>
        </template>
      </nav>

      <div v-if="!collapsed" class="sider-footer">
        <div class="sider-footer__version-row">
          <div class="sider-footer__title">
            <span class="sider-footer__brand">Havline</span>
            <span class="sider-footer__version">v{{ appVersion }}</span>
          </div>
          <a
            v-if="canOneClickUpdate"
            class="sider-footer__update"
            href="#"
            @click.prevent="showUpdateModal = true"
          >
            <n-icon :component="ArrowUpOutline" />
            一键升级
          </a>
          <a
            v-else-if="hasUpdate"
            class="sider-footer__update"
            :href="latestReleaseUrl"
            target="_blank"
            rel="noopener noreferrer"
          >
            <n-icon :component="ArrowUpOutline" />
            有新版本
            <n-icon :component="OpenOutline" />
          </a>
        </div>

        <div class="sider-footer__status">
          <n-icon :component="Ellipse" class="sider-footer__status-icon" />
          <span>运行中</span>
        </div>

        <a
          class="sider-footer__github"
          :href="GITHUB_REPO_URL"
          target="_blank"
          rel="noopener noreferrer"
        >
          <n-icon :component="LogoGithub" />
          <span>{{ GITHUB_REPO }}</span>
        </a>
      </div>
    </n-layout-sider>

    <n-layout class="app-main">
      <header class="topbar">
        <span class="topbar__datetime">{{ datetime }}</span>
        <div class="topbar__right">
          <n-button quaternary circle @click="handleToggleTheme">
            <template #icon>
              <n-icon :component="isDark ? MoonOutline : SunnyOutline" />
            </template>
          </n-button>
          <n-dropdown :options="userMenuOptions" @select="handleUserMenu">
            <n-button quaternary class="user-btn">
              <n-icon :component="PersonOutline" class="user-btn__icon" />
              <span class="user-name">管理员</span>
              <n-icon :component="ChevronDownOutline" class="user-btn__chevron" />
            </n-button>
          </n-dropdown>
        </div>
      </header>

      <n-layout-content class="content">
        <div class="page-shell page-body">
          <router-view />
        </div>
      </n-layout-content>
    </n-layout>
  </n-layout>
  <UpdateModal v-model:show="showUpdateModal" @updated="checkForUpdate" />
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NButton,
  NDropdown,
  NIcon,
  NLayout,
  NLayoutContent,
  NLayoutSider,
  useMessage,
} from 'naive-ui'
import {
  ArrowUpOutline,
  ChevronDownOutline,
  CloudOutline,
  CogOutline,
  DocumentTextOutline,
  Ellipse,
  GitNetworkOutline,
  GlobeOutline,
  GridOutline,
  HardwareChipOutline,
  InformationCircleOutline,
  LockClosedOutline,
  LogoGithub,
  MoonOutline,
  OpenOutline,
  PersonOutline,
  ShieldCheckmarkOutline,
  SunnyOutline,
  SwapHorizontalOutline,
  TerminalOutline,
} from '@vicons/ionicons5'
import { api } from '../api/client'
import { GITHUB_REPO, GITHUB_REPO_URL } from '../constants/app'
import { useTheme } from '../composables/useTheme'
import { fetchLatestRelease, isNewerVersion } from '../utils/version'
import type { UpdateStatus } from '../api/types'
import brandLogo from '../assets/brand/havline-logo.svg'
import UpdateModal from '../components/UpdateModal.vue'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const { isDark, themeMode, toggleTheme, syncFromSettings } = useTheme()
const collapsed = ref(false)
const datetime = ref('')
const appVersion = ref(__APP_VERSION__)
const latestVersion = ref('')
const latestReleaseUrl = ref('')
const hasUpdate = ref(false)
const showUpdateModal = ref(false)
const updateStatus = ref<UpdateStatus | null>(null)
let clockTimer: number | undefined

const canOneClickUpdate = computed(() => Boolean(
  updateStatus.value?.enabled && updateStatus.value.update_available
))

async function checkForUpdate() {
  const release = await fetchLatestRelease()
  if (!release) {
    return
  }

  latestVersion.value = release.version
  latestReleaseUrl.value = release.url
  hasUpdate.value = isNewerVersion(release.version, appVersion.value)
}

async function loadVersion() {
  try {
    const { version } = await api.getVersion()
    if (version) {
      appVersion.value = version
    }
  } catch {
    // keep build-time fallback
  }

  await Promise.all([checkForUpdate(), loadUpdateStatus()])
}

async function loadUpdateStatus() {
  try {
    updateStatus.value = await api.getUpdateStatus()
  } catch {
    updateStatus.value = null
  }
}

type NavItem = { label: string; key: string; icon: Component }
type NavGroup = { label: string; items: NavItem[] }

// 分组与原型一致：概览 / 网络 / 运维 / 系统。
// 顺序按「先看状态、再管流量、最后进设置」的使用路径排，而不是按模块实现顺序。
const menuGroups: NavGroup[] = [
  {
    label: '概览',
    items: [{ label: '仪表盘', key: 'dashboard', icon: GridOutline }],
  },
  {
    label: '网络',
    items: [
      { label: '反向代理', key: 'proxies', icon: SwapHorizontalOutline },
      { label: '内网穿透', key: 'frp', icon: GitNetworkOutline },
      { label: '公网服务端', key: 'frp-agent', icon: HardwareChipOutline },
      { label: '公网反代', key: 'public-proxy', icon: ShieldCheckmarkOutline },
      { label: 'DDNS', key: 'ddns', icon: GlobeOutline },
      { label: 'Cloudflare 隧道', key: 'cloudflare', icon: CloudOutline },
    ],
  },
  {
    label: '运维',
    items: [
      { label: '证书管理', key: 'certificates', icon: LockClosedOutline },
      { label: 'SSH 终端', key: 'terminal', icon: TerminalOutline },
      { label: '日志中心', key: 'logs', icon: DocumentTextOutline },
    ],
  },
  {
    label: '系统',
    items: [
      { label: '系统设置', key: 'settings', icon: CogOutline },
      { label: '关于项目', key: 'about', icon: InformationCircleOutline },
    ],
  },
]

const activeKey = computed(() => {
  const name = route.name
  // 终端审计是终端页的子页面：菜单里只有「SSH 终端」，这里让它保持高亮
  if (name === 'terminal-audits') return 'terminal'
  return typeof name === 'string' ? name : 'dashboard'
})

const userMenuOptions = [{ label: '退出登录', key: 'logout' }]

function updateClock() {
  const now = new Date()
  const date = now.toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
  const time = now.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  const weekday = now.toLocaleDateString('zh-CN', { weekday: 'long' })
  datetime.value = `${date} ${time} · ${weekday}`
}

function handleMenu(key: string) {
  router.push({ name: key })
}

// handleToggleTheme 顶栏的快捷切换：本地立即生效，同时回写后端设置。
// 不回写的话会出现「这里切了、设置页显示的还是旧值」，下次任何一次保存设置
// 又会把主题改回去。
async function handleToggleTheme() {
  toggleTheme()
  try {
    await api.saveSettings({ theme: themeMode.value })
  } catch {
    // 回写失败不影响当前会话的外观，下次进设置页仍能看到服务端的值
  }
}

async function handleUserMenu(key: string) {
  if (key === 'logout') {
    await api.logout()
    message.success('已退出登录')
    router.push({ name: 'login' })
  }
}

onMounted(() => {
  loadVersion()
  updateClock()
  clockTimer = window.setInterval(updateClock, 30_000)
  // 主题以后端设置为准：localStorage 只是首屏回显，登录后要用服务端的值覆盖，
  // 否则换设备/换浏览器两个人看到的是不同主题（而设置页显示的是服务端那个）。
  api
    .getSettings()
    .then((settings) => {
      syncFromSettings(settings.theme)
    })
    .catch(() => {
      // 读不到设置就沿用本地偏好，不影响使用
    })
})

onUnmounted(() => {
  if (clockTimer) clearInterval(clockTimer)
})
</script>

<style scoped>
.app-layout {
  min-height: 100vh;
  height: 100vh;
  background: var(--havline-bg);
}

.app-layout :deep(.n-layout-scroll-container) {
  min-height: 100%;
}

.app-sider {
  background: var(--havline-surface) !important;
  border-right: 1px solid var(--havline-border) !important;
  min-height: 100vh;
}

.app-sider :deep(.n-layout-sider-scroll-container) {
  display: flex;
  flex-direction: column;
}

/* ------------------------------ 品牌区 ------------------------------ */

.brand {
  display: flex;
  align-items: center;
  gap: var(--havline-space-3);
  padding: var(--havline-space-5) var(--havline-space-4);
  border-bottom: 1px solid var(--havline-border);
  color: inherit;
  text-decoration: none;
  cursor: pointer;
  transition: background 0.15s ease;
}

.brand:hover {
  background: var(--havline-bg-muted);
}

.brand--collapsed {
  justify-content: center;
  padding: var(--havline-space-4) 0;
}

.brand__logo {
  width: 40px;
  height: 40px;
  flex-shrink: 0;
}

.brand__name {
  font-size: 18px;
  font-weight: 700;
  color: var(--havline-text);
  line-height: 1.2;
}

.brand__tagline {
  font-size: 11px;
  color: var(--havline-text-muted);
  margin-top: 2px;
  line-height: 1.3;
}

/* ------------------------------ 导航 ------------------------------ */

.nav {
  flex: 1;
  overflow-y: auto;
  padding: 10px 12px 16px;
}

.nav__group {
  padding: 16px 12px 6px;
  font-size: 10.5px;
  font-weight: 700;
  letter-spacing: 0.09em;
  text-transform: uppercase;
  color: var(--havline-text-muted);
}

.nav__group:first-child {
  padding-top: 6px;
}

/* 折叠态没有文字标题，用细分隔线保留分组感 */
.nav__divider {
  height: 1px;
  margin: 10px 8px;
  background: var(--havline-border-soft);
}

.nav__divider:first-child {
  display: none;
}

.nav__item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 11px;
  width: 100%;
  height: 42px;
  padding: 0 12px;
  margin-bottom: 2px;
  border: 0;
  border-radius: 11px;
  background: none;
  color: var(--havline-text-secondary);
  font-size: 13.5px;
  font-weight: 500;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  white-space: nowrap;
  transition: background 0.16s ease, color 0.16s ease;
}

.nav__item--icon-only {
  justify-content: center;
  gap: 0;
  padding: 0;
}

.nav__icon {
  font-size: 18px;
  flex: none;
  color: var(--havline-text-muted);
  transition: color 0.16s ease;
}

.nav__item:hover {
  background: var(--havline-bg-muted);
  color: var(--havline-text);
}

.nav__item:hover .nav__icon {
  color: var(--havline-text-secondary);
}

.nav__item--active {
  background: linear-gradient(90deg, var(--havline-brand-soft), rgba(16, 185, 129, 0.03));
  color: var(--havline-brand-text);
  font-weight: 650;
}

.nav__item--active .nav__icon {
  color: var(--havline-brand);
}

/* 选中态左侧竖条：比整块底色更容易在长列表里定位当前页 */
.nav__item--active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 12px;
  bottom: 12px;
  width: 3px;
  border-radius: 0 3px 3px 0;
  background: var(--havline-brand);
}

.nav__label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* --------------------------- 底部状态区 --------------------------- */

.sider-footer {
  padding: var(--havline-space-4);
  border-top: 1px solid var(--havline-border);
}

.sider-footer__version-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: var(--havline-space-4);
}

.sider-footer__title {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.sider-footer__brand {
  font-size: 13px;
  font-weight: 600;
  color: var(--havline-text);
}

.sider-footer__version {
  font-size: 12px;
  color: var(--havline-text-muted);
}

.sider-footer__update {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 0;
  border: 0;
  background: none;
  font-size: 11px;
  font-weight: 500;
  font-family: inherit;
  color: var(--havline-brand);
  text-decoration: none;
  white-space: nowrap;
  cursor: pointer;
  transition: opacity 0.15s;
}

.sider-footer__update:hover {
  opacity: 0.8;
}

.sider-footer__update :deep(.n-icon) {
  font-size: 12px;
}

.sider-footer__status {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: var(--havline-space-4);
  font-size: 12px;
  color: var(--havline-success);
  font-weight: 500;
}

.sider-footer__status-icon {
  font-size: 8px;
  color: var(--havline-success);
}

.sider-footer__github {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--havline-text-secondary);
  text-decoration: none;
  transition: color 0.15s;
}

.sider-footer__github:hover {
  color: var(--havline-text);
}

.sider-footer__github :deep(.n-icon) {
  font-size: 15px;
}

/* ------------------------------ 顶栏与内容 ------------------------------ */

.app-main {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  background: var(--havline-bg);
}

.topbar {
  flex-shrink: 0;
  height: var(--havline-topbar-height);
  padding: 0 var(--havline-space-5);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--havline-space-4);
  background: var(--havline-surface);
  border-bottom: 1px solid var(--havline-border);
}

.topbar__datetime {
  font-size: 13px;
  color: var(--havline-text-secondary);
}

.topbar__right {
  display: flex;
  align-items: center;
  gap: var(--havline-space-4);
}

.user-btn {
  display: flex;
  align-items: center;
  gap: var(--havline-space-3);
  padding: 0 var(--havline-space-2);
}

.user-btn__icon,
.user-btn__chevron {
  font-size: 18px;
  color: var(--havline-text-secondary);
}

.user-name {
  font-size: 14px;
  color: var(--havline-text);
}

.content {
  flex: 1;
  padding: var(--havline-space-5);
  overflow: auto;
}

.page-body {
  min-height: 100%;
}

@media (max-width: 768px) {
  .topbar,
  .content {
    padding-inline: var(--havline-space-4);
  }

  .user-name,
  .topbar__datetime {
    display: none;
  }
}
</style>
