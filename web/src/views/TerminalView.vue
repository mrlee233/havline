<template>
  <PageHeader
    title="SSH 终端"
    description="在浏览器里直接连到公网服务端：SSH 凭据与主机密钥都复用「公网服务端」页已保存的配置"
  >
    <template #actions>
      <n-tag v-if="unlocked" size="small" :bordered="false" type="success">
        终端已解锁 · 剩余 {{ unlockRemainingText }}
      </n-tag>
      <n-button v-if="unlocked" size="small" quaternary @click="lockTerminal">锁定</n-button>
      <n-button size="small" quaternary @click="openAudits">
        <template #icon><n-icon :component="ListOutline" /></template>
        审计记录
      </n-button>
      <n-button v-if="sessions.length" size="small" type="error" quaternary @click="closeAll">全部关闭</n-button>
      <n-button type="primary" size="small" @click="openPicker">
        <template #icon><n-icon :component="AddOutline" /></template>
        新建会话
      </n-button>
    </template>
  </PageHeader>

  <LoadError v-if="loadError" :message="loadError" @retry="load" />

  <n-alert v-else-if="!enabled" type="warning" :bordered="false" class="term-alert">
    内置 SSH 终端未启用：请到「系统设置 → 安全」打开「SSH 终端」开关后再使用。
  </n-alert>

  <template v-else>
    <!-- 左侧终端工作区；右侧是服务器列表 / 录像审计 / 常用命令面板 -->
    <div class="term-workspace">
      <div class="term-main">
    <div v-if="sessions.length" class="term-tabs">
      <button
        v-for="item in sessions"
        :key="item.key"
        type="button"
        class="term-tab"
        :class="{ 'term-tab--active': item.key === activeKey }"
        @click="activeKey = item.key"
      >
        <span class="term-tab__dot" :class="`term-tab__dot--${item.state}`" aria-hidden="true" />
        <span class="term-tab__label">{{ item.label }}</span>
        <span class="term-tab__close" title="关闭会话" @click.stop="closeSession(item)">×</span>
      </button>
    </div>

    <div v-if="sessions.length" class="term-panel">
      <div class="term-panel__bar">
        <span class="term-panel__label">{{ activeSession?.label }}</span>
        <StatusBadge :kind="stateKind(activeSession)" :text="stateText(activeSession)" />
        <span class="term-panel__meta mono">{{ activeSession?.size || '—' }}</span>
        <span v-if="readOnly" class="term-panel__readonly">只读</span>
        <span v-if="activeReason" class="term-panel__reason">{{ activeReason }}</span>
        <div class="term-panel__tools">
          <n-button size="tiny" quaternary title="复制选中文本（Ctrl+Shift+C）" @click="copySelection">
            <template #icon><n-icon :component="CopyOutline" /></template>
          </n-button>
          <n-button
            size="tiny"
            quaternary
            :disabled="readOnly"
            title="粘贴剪贴板（Ctrl+Shift+V）"
            @click="pasteClipboard"
          >
            <template #icon><n-icon :component="ClipboardOutline" /></template>
          </n-button>
          <n-button size="tiny" quaternary title="清屏" @click="clearScreen">
            <template #icon><n-icon :component="TrashOutline" /></template>
          </n-button>
          <n-button size="tiny" quaternary title="减小字号" @click="changeFont(-1)">A-</n-button>
          <n-button size="tiny" quaternary title="增大字号" @click="changeFont(1)">A+</n-button>
          <n-button
            v-if="isNarrow"
            size="tiny"
            quaternary
            :title="readOnly ? '允许用本机键盘输入命令' : '切回只读模式'"
            @click="toggleMobileInput"
          >
            <template #icon><n-icon :component="readOnly ? LockClosedOutline : LockOpenOutline" /></template>
            {{ readOnly ? '允许输入' : '只读' }}
          </n-button>
          <n-button
            size="tiny"
            :loading="activeSession?.state === 'connecting'"
            title="重新建立会话"
            @click="reconnectActive"
          >
            <template #icon><n-icon :component="RefreshOutline" /></template>
            重连
          </n-button>
        </div>
      </div>
      <!-- 每个会话一个 host：切换标签用 v-show，各自的滚动缓冲与光标都保留 -->
      <div ref="stackHost" class="term-stack" :style="stackStyle">
        <div
          v-for="item in sessions"
          :key="item.key"
          v-show="item.key === activeKey"
          :ref="(element) => bindHost(item.key, element)"
          class="term-host"
        />
      </div>
      <!-- 拖动分隔条调整终端高度：默认按视口自适应，双击恢复自适应 -->
      <div
        class="term-resizer"
        :class="{ 'term-resizer--active': resizing }"
        role="separator"
        aria-orientation="horizontal"
        aria-label="拖动调整终端高度"
        title="上下拖动调整高度，双击恢复自适应"
        @pointerdown="startResize"
        @dblclick="resetHeight"
      >
        <span class="term-resizer__grip" />
      </div>
    </div>

    <HavlineCard v-else title="还没有会话" subtitle="选择一个已配置 SSH 的服务端开始；凭据在「公网服务端」页维护">
      <div class="term-empty">
        <n-button type="primary" @click="openPicker">
          <template #icon><n-icon :component="AddOutline" /></template>
          选择服务端
        </n-button>
        <span class="term-empty__hint">快捷键：Ctrl/⌘+Shift+C 复制选中、Ctrl/⌘+Shift+V 粘贴</span>
      </div>
      </HavlineCard>
      </div>

      <aside class="term-aside">
        <n-tabs v-model:value="asideTab" type="line" size="small">
          <n-tab-pane name="servers" tab="服务器列表">
            <div class="aside-actions">
              <n-button size="small" type="primary" @click="goServerSettings">添加服务器</n-button>
              <n-button size="small" quaternary :loading="loadingTargets" @click="load">刷新</n-button>
            </div>
            <div v-if="targets.length" class="aside-list">
              <button
                v-for="item in targets"
                :key="item.value"
                type="button"
                class="aside-row aside-row--button"
                :class="{ 'is-disabled': !item.ready }"
                :title="item.ready ? '连接到这台服务端' : item.disabled_reason"
                @click="connectFromAside(item)"
              >
                <span class="aside-row__dot" :class="item.ready ? 'is-ready' : 'is-off'" aria-hidden="true" />
                <span class="aside-row__body">
                  <span class="aside-row__title">{{ item.label }}</span>
                  <span class="aside-row__meta mono">{{ item.detail }}</span>
                </span>
                <span class="aside-row__hint">{{ item.ready ? '连接' : '未配置' }}</span>
              </button>
            </div>
            <p v-else class="aside-empty">还没有服务端：先到「公网服务端」添加 VPS 并填写 SSH。</p>
          </n-tab-pane>

          <n-tab-pane name="recordings" tab="录像审计">
            <div class="aside-actions">
              <n-button size="small" quaternary :loading="loadingRecordings" @click="loadRecordings">刷新</n-button>
              <n-button size="small" quaternary @click="openAudits">审计明细</n-button>
            </div>
            <p v-if="!recordingEnabled" class="aside-note">
              会话录制默认关闭：到「系统设置 → 安全」打开「录制会话内容」后，新会话会生成可回放的录像；
              录像写在 logs/terminal，保留 90 天。
            </p>
            <div v-if="recordings.length" class="aside-list">
              <div v-for="item in recordings" :key="item.session_id" class="aside-row">
                <span class="aside-row__body">
                  <span class="aside-row__title">{{ recordingTitle(item) }}</span>
                  <span class="aside-row__meta">
                    {{ formatDate(item.started_at || item.ended_at) }} · {{ formatBytes(item.size) }}
                  </span>
                </span>
                <n-button size="tiny" quaternary title="回放这段录像" @click="playRecording(item)">播放</n-button>
              </div>
            </div>
            <p v-else class="aside-empty">还没有录像。</p>
          </n-tab-pane>

          <n-tab-pane name="commands" tab="常用命令">
            <div class="aside-form">
              <n-input v-model:value="commandName" size="small" placeholder="名称，如：查看磁盘" />
              <n-input
                v-model:value="commandText"
                size="small"
                placeholder="命令，如：df -h"
                @keyup.enter="saveCommand"
              />
              <n-button
                size="small"
                type="primary"
                :loading="savingCommand"
                :disabled="!commandName.trim() || !commandText.trim()"
                @click="saveCommand"
              >
                保存
              </n-button>
            </div>
            <div v-if="commands.length" class="aside-list">
              <div v-for="item in commands" :key="item.id" class="aside-row">
                <span class="aside-row__body">
                  <span class="aside-row__title">{{ item.name }}</span>
                  <span class="aside-row__meta mono">{{ item.command }}</span>
                </span>
                <n-button size="tiny" quaternary title="发送到当前会话" @click="runCommand(item)">发送</n-button>
                <n-button size="tiny" quaternary title="删除这条命令" @click="removeCommand(item)">×</n-button>
              </div>
            </div>
            <p v-else class="aside-empty">还没有常用命令：保存一条后可以一键发送到当前会话。</p>
          </n-tab-pane>
        </n-tabs>
      </aside>
    </div>
  </template>

  <n-modal
    v-model:show="pickerVisible"
    preset="card"
    :style="{ width: 'min(680px, 96vw)' }"
    :title="unlocked ? '新建 SSH 会话' : '终端二次认证'"
  >
    <!-- 未解锁：先过一次管理员密码，避免别人用你已登录的浏览器点一下就进 root shell -->
    <div v-if="!unlocked" class="term-unlock">
      <p class="term-unlock__lead">终端是 root 级入口，需要再次输入管理员密码确认；解锁后 10 分钟内新建会话不再重复输入。</p>
      <n-input
        v-model:value="unlockPassword"
        type="password"
        show-password-on="click"
        placeholder="管理员密码"
        :disabled="unlocking"
        @keyup.enter="submitUnlock"
      />
      <p v-if="unlockError" class="term-unlock__error">{{ unlockError }}</p>
      <p v-if="insecureContext" class="term-unlock__warn">
        当前是 HTTP 访问，密码在链路上为明文；建议只在可信网络使用，或为 Havline 配置 HTTPS。
      </p>
      <n-button type="primary" :loading="unlocking" @click="submitUnlock">解锁并继续</n-button>
    </div>
    <div v-else-if="targets.length" class="term-targets">
      <button
        v-for="item in targets"
        :key="item.value"
        type="button"
        class="term-target"
        :disabled="!item.ready"
        @click="startSession(item)"
      >
        <div class="term-target__top">
          <span class="term-target__label">{{ item.label }}</span>
          <n-tag size="tiny" :bordered="false" :type="item.ready ? 'success' : 'warning'">
            {{ item.ready ? '可连接' : '未配置 SSH' }}
          </n-tag>
        </div>
        <div class="term-target__detail mono">{{ item.detail }}</div>
        <div v-if="!item.ready" class="term-target__hint">{{ item.disabled_reason }}</div>
      </button>
    </div>
    <p v-else class="term-hint">还没有服务端：请先到「公网服务端」页添加 VPS，并填写 SSH 地址与用户。</p>
  </n-modal>

  <!-- 录像回放：按 asciinema v2 的时间轴逐帧写入一个只读终端 -->
  <n-modal
    v-model:show="playerVisible"
    preset="card"
    :style="{ width: 'min(1000px, 96vw)' }"
    :title="playerTitle"
    @after-leave="closePlayer"
  >
    <div ref="playerHost" class="term-player" />
    <p class="aside-note">回放的是当时屏幕上的原始输出；长时间空闲已压缩。</p>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NAlert, NButton, NIcon, NInput, NModal, NTabPane, NTabs, NTag, useMessage } from 'naive-ui'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import {
  AddOutline,
  ClipboardOutline,
  CopyOutline,
  ListOutline,
  LockClosedOutline,
  LockOpenOutline,
  RefreshOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { ApiRequestError, api, asList, terminalSocketURL } from '../api/client'
import { formatBytes, formatDate } from '../utils/format'
import HavlineCard from '../components/HavlineCard.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { copyText } from '../utils/clipboard'
import type { TerminalAuditEntry, TerminalCommand, TerminalRecording, TerminalTarget } from '../api/types'

// 终端配色与页面深色终端区一致；字体交给 xterm 默认等宽栈
const TERMINAL_THEME = {
  background: '#0b1220',
  foreground: '#e2e8f0',
  cursor: '#34d399',
  selectionBackground: '#1e293b',
}
// 服务端读超时 90 秒，这里每 30 秒发一次 ping 保活
const PING_INTERVAL_MS = 30_000
// 输出停下再补留白：留白要往缓冲区里写序列，等输出落定再动手最稳妥
const PAD_SYNC_DELAY_MS = 150
// 升级请求迟迟不返回时的兜底：避免标签永远停在「连接中」
const CONNECT_TIMEOUT_MS = 12_000
const FONT_SIZE_KEY = 'havline.terminal.fontSize'
const HEIGHT_KEY = 'havline.terminal.height'

type SessionState = 'connecting' | 'ready' | 'closed' | 'error'

// 会话元信息参与渲染，保持响应式；xterm / socket 等实例放在非响应式 Map 里，
// 避免 Vue 深度代理 xterm 对象导致内部状态异常。
interface SessionMeta {
  key: string
  id: string
  label: string
  serverID: number
  state: SessionState
  reason: string
  size: string
}

interface SessionResources {
  term: Terminal
  fit: FitAddon
  observer: ResizeObserver
  scrollSub?: { dispose(): void }
  socket: WebSocket | null
  pingTimer?: number
  connectTimer?: number
  padTimer?: number
  // 是否处于「跟随输出」的位置：用户翻历史后不再自动钉回尾部
  following: boolean
  // 补留白时我们自己会滚屏，期间的滚动事件不能当成用户操作
  suppressScroll?: boolean
}

const message = useMessage()
const router = useRouter()
const enabled = ref(true)
const loadError = ref('')
// 二次认证：unlockExpiresAt 是解锁票据到期时间（毫秒），0 表示当前未解锁
const unlockExpiresAt = ref(0)
const unlockPassword = ref('')
const unlockError = ref('')
const unlocking = ref(false)
const now = ref(Date.now())
const targets = ref<TerminalTarget[]>([])
const loadingTargets = ref(false)
// 右侧面板：服务器列表 / 录像审计 / 常用命令
const asideTab = ref<'servers' | 'recordings' | 'commands'>('servers')
const audits = ref<TerminalAuditEntry[]>([])
const loadingAudits = ref(false)
const recordings = ref<TerminalRecording[]>([])
const loadingRecordings = ref(false)
const recordingEnabled = ref(false)
// 录像回放用的临时终端（不进响应式，避免 Vue 代理 xterm 内部状态）
const playerVisible = ref(false)
const playerTitle = ref('录像回放')
const playerHost = ref<HTMLDivElement | null>(null)
let playerTerm: Terminal | null = null
let playerTimers: number[] = []
const commands = ref<TerminalCommand[]>([])
const commandName = ref('')
const commandText = ref('')
const savingCommand = ref(false)
const pickerVisible = ref(false)
const sessions = ref<SessionMeta[]>([])
const activeKey = ref('')
const fontSize = ref(readFontSize())
// 窄屏（手机）默认进入只读：可以滚动、复制，但软键盘不直接敲命令，避免误操作
const isNarrow = ref(false)
const allowMobileInput = ref(false)
// 终端区域高度：null 表示按视口自适应，拖动后固定像素并持久化
const stackHost = ref<HTMLDivElement | null>(null)
const stackHeight = ref<number | null>(readStoredHeight())
const resizing = ref(false)

const hosts = new Map<string, HTMLDivElement>()
const resources = new Map<string, SessionResources>()

const activeSession = computed(() => sessions.value.find((item) => item.key === activeKey.value) ?? null)
const readOnly = computed(() => isNarrow.value && !allowMobileInput.value)
const activeReason = computed(() => describeReason(activeSession.value?.reason ?? ''))
const unlocked = computed(() => unlockExpiresAt.value > now.value)
const unlockRemainingText = computed(() => `${Math.max(1, Math.ceil((unlockExpiresAt.value - now.value) / 60_000))} 分钟`)
const insecureContext = computed(() => window.location.protocol !== 'https:')
// 注意 0 是合法高度（手柄贴到顶边框），不能用 truthy 判断，否则会掉回自适应
const stackStyle = computed(() => (stackHeight.value === null ? undefined : { height: `${stackHeight.value}px` }))

function readFontSize(): number {
  try {
    const raw = Number(localStorage.getItem(FONT_SIZE_KEY))
    if (raw >= 11 && raw <= 20) return raw
  } catch { /* 隐私模式下忽略 */ }
  return 13
}

function readStoredHeight(): number | null {
  try {
    const text = localStorage.getItem(HEIGHT_KEY)
    if (!text) return null
    const raw = Number(text)
    if (raw >= 0 && raw <= 1600) return raw
  } catch { /* 隐私模式下忽略 */ }
  return null
}

function stateText(item: SessionMeta | null) {
  if (!item) return '未连接'
  return ({ connecting: '连接中', ready: '已连接', closed: '已断开', error: '连接失败' } as Record<SessionState, string>)[item.state]
}

function stateKind(item: SessionMeta | null): 'warning' | 'success' | 'disabled' | 'error' {
  if (!item) return 'disabled'
  return ({ connecting: 'warning', ready: 'success', closed: 'disabled', error: 'error' } as const)[item.state]
}

// 后端下发的是原始原因（含“空闲超时”“前端断开”等），这里翻译成用户能看懂的短句
const REASON_TEXT: Array<[string, string]> = [
  ['用户关闭', '已主动关闭'],
  ['空闲超时', '长时间无操作，已自动断开'],
  ['超过最长会话时长', '会话达到最长时长，已断开'],
  ['前端断开', '连接通道中断：网络波动或页面被切换'],
  ['会话已结束', '远端 shell 已退出'],
]

function describeReason(reason: string): string {
  if (!reason) return ''
  const hit = REASON_TEXT.find(([key]) => reason.includes(key))
  return hit ? hit[1] : reason
}

async function load() {
  loadError.value = ''
  loadingTargets.value = true
  try {
    const result = await api.listTerminalTargets()
    targets.value = asList(result?.targets)
    enabled.value = true
  } catch (error) {
    const text = error instanceof Error ? error.message : '读取服务端列表失败'
    // 开关关闭时后端返回 403，这里按「未启用」提示，而不是当成加载失败
    if (text.includes('终端未启用')) {
      enabled.value = false
      return
    }
    loadError.value = text
  } finally {
    loadingTargets.value = false
  }
}

function openPicker() {
  pickerVisible.value = true
  unlockPassword.value = ''
  unlockError.value = ''
  void loadUnlock()
  void load()
  void loadRecordings()
}

// loadUnlock 读当前会话的解锁状态：页面刷新后仍能识别 10 分钟内的有效解锁
async function loadUnlock() {
  try {
    const status = await api.getTerminalUnlock()
    unlockExpiresAt.value = status.unlocked && status.expires_at ? Date.parse(status.expires_at) : 0
  } catch {
    // 读不到就按未解锁处理：顶多让用户多输一次密码
    unlockExpiresAt.value = 0
  }
}

// submitUnlock 提交管理员密码换取短时解锁；密码只用一次，不留在前端
async function submitUnlock() {
  if (unlocking.value) return
  if (!unlockPassword.value) {
    unlockError.value = '请输入管理员密码'
    return
  }
  unlocking.value = true
  unlockError.value = ''
  try {
    const result = await api.unlockTerminal(unlockPassword.value)
    unlockExpiresAt.value = result.expires_at ? Date.parse(result.expires_at) : Date.now() + 600_000
    unlockPassword.value = ''
    now.value = Date.now()
  } catch (error) {
    unlockError.value = error instanceof Error ? error.message : '解锁失败，请重试'
  } finally {
    unlocking.value = false
  }
}

async function lockTerminal() {
  try {
    await api.lockTerminal()
  } catch {
    // 锁定失败也要把本地状态收回来，否则界面会显示一个已失效的解锁
  }
  unlockExpiresAt.value = 0
  unlockPassword.value = ''
  unlockError.value = ''
  message.info('已锁定终端，下次新建会话需要重新输入密码')
}

// 审计页是独立路由：这里只做跳转，不把表格塞进终端工作区
function openAudits() {
  void router.push({ name: 'terminal-audits' })
}

// ===== 右侧面板：最近审计与常用命令 =====

async function loadAudits() {
  loadingAudits.value = true
  try {
    const result = await api.listTerminalAudits(20)
    audits.value = asList(result?.entries)
  } catch {
    // 面板里的辅助信息，读不到就留空，不打断终端操作
    audits.value = []
  } finally {
    loadingAudits.value = false
  }
}

async function loadCommands() {
  try {
    const result = await api.listTerminalCommands()
    commands.value = asList(result?.commands)
  } catch {
    commands.value = []
  }
}

async function loadRecordings() {
  loadingRecordings.value = true
  try {
    const result = await api.listTerminalRecordings()
    recordings.value = asList(result?.recordings)
  } catch {
    recordings.value = []
  } finally {
    loadingRecordings.value = false
  }
}

// loadRecordingSetting 只为了在面板上提示「录制是否开启」
async function loadRecordingSetting() {
  try {
    const settings = await api.getSettings()
    recordingEnabled.value = settings.terminal_recording_enabled === '1'
  } catch {
    recordingEnabled.value = false
  }
}

// recordingTitle 用审计记录补一个可读标题：录像文件本身只有会话 ID
function recordingTitle(item: TerminalRecording) {
  const audit = audits.value.find((entry) => entry.session_id === item.session_id)
  if (audit) return `${audit.target || `服务端 #${audit.server_id}`} · ${audit.user}`
  return `会话 ${item.session_id.slice(0, 8)}`
}

async function playRecording(item: TerminalRecording) {
  playerTitle.value = recordingTitle(item)
  playerVisible.value = true
  await nextTick()
  mountPlayer()
  try {
    const text = await api.getTerminalRecording(item.session_id)
    feedRecording(text)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '读取录像失败')
  }
}

function mountPlayer() {
  closePlayer()
  const host = playerHost.value
  if (!host) return
  host.innerHTML = ''
  playerTerm = new Terminal({
    cols: 80,
    rows: 24,
    fontSize: 12,
    scrollback: 5000,
    disableStdin: true,
    theme: TERMINAL_THEME,
  })
  playerTerm.open(host)
}

// feedRecording 按时间轴回放 asciinema v2：单次停顿封顶，避免长空闲把回放拖住
function feedRecording(text: string) {
  const term = playerTerm
  if (!term) return
  const lines = text.split('\n').filter((line) => line.trim() !== '')
  if (lines.length === 0) return
  try {
    const header = JSON.parse(lines[0]) as { width?: number; height?: number }
    if (header.width && header.height) term.resize(header.width, header.height)
  } catch {
    // 头不合法就按默认尺寸回放
  }
  let previous = 0
  let elapsed = 0
  for (const line of lines.slice(1)) {
    let frame: unknown
    try {
      frame = JSON.parse(line)
    } catch {
      continue
    }
    if (!Array.isArray(frame) || frame.length < 3 || frame[1] !== 'o') continue
    const at = typeof frame[0] === 'number' ? frame[0] : previous
    elapsed += Math.min(Math.max((at - previous) * 1000, 0), 300)
    previous = at
    const chunk = String(frame[2])
    playerTimers.push(window.setTimeout(() => term.write(chunk), elapsed))
  }
}

function closePlayer() {
  for (const id of playerTimers) window.clearTimeout(id)
  playerTimers = []
  playerTerm?.dispose()
  playerTerm = null
}

async function saveCommand() {
  const name = commandName.value.trim()
  const command = commandText.value.trim()
  if (!name || !command || savingCommand.value) return
  savingCommand.value = true
  try {
    const created = await api.createTerminalCommand({ name, command })
    commands.value = [created, ...commands.value]
    commandName.value = ''
    commandText.value = ''
    message.success('已保存常用命令')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存常用命令失败')
  } finally {
    savingCommand.value = false
  }
}

async function removeCommand(item: TerminalCommand) {
  try {
    await api.deleteTerminalCommand(item.id)
  } catch {
    // 删除失败也把本地列表收回，避免界面停在过期状态
  }
  commands.value = commands.value.filter((entry) => entry.id !== item.id)
}

// runCommand 把命令发到当前会话；没有会话或处于只读模式时给出明确提示
function runCommand(item: TerminalCommand) {
  const session = activeSession.value
  if (!session) {
    message.warning('先打开一个会话，再发送常用命令')
    return
  }
  if (readOnly.value) {
    message.info('当前为只读模式：先点工具条上的「允许输入」再发送')
    return
  }
  sendInput(session.key, `${item.command}\r`)
}

// connectFromAside 从右侧列表连服务端：还没解锁就先走二次认证
function connectFromAside(item: TerminalTarget) {
  if (!item.ready) {
    message.warning(item.disabled_reason || '这台服务端还没配置 SSH')
    return
  }
  if (!unlocked.value) {
    openPicker()
    return
  }
  void startSession(item)
}

function goServerSettings() {
  void router.push({ name: 'frp-agent' })
}

// bindHost 记录每个会话的挂载节点，xterm 实例在 nextTick 之后才创建。
function bindHost(key: string, element: unknown) {
  if (element instanceof HTMLDivElement) hosts.set(key, element)
  else hosts.delete(key)
}

async function startSession(target: TerminalTarget) {
  if (!target.ready) return
  if (!unlocked.value) {
    // 理论上按钮在未解锁时不会出现，这里兜住「解锁刚好过期」的窗口
    unlockError.value = '请先完成二次认证再建立会话'
    return
  }
  pickerVisible.value = false
  const meta: SessionMeta = {
    key: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    id: '', label: target.label, serverID: Number(target.value),
    state: 'connecting', reason: '', size: '',
  }
  sessions.value = [...sessions.value, meta]
  activeKey.value = meta.key
  await nextTick()
  // 面板刚出现时才知道它的顶部位置，这时把上次保存的高度收回可视范围
  clampStoredHeight()
  mountTerminal(meta)
  await createAndConnect(meta)
}

function mountTerminal(meta: SessionMeta) {
  const host = hosts.get(meta.key)
  if (!host) return
  const term = new Terminal({
    cursorBlink: true,
    disableStdin: readOnly.value,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
    fontSize: fontSize.value,
    scrollback: 5000,
    // 留白把视口钉在「光标行贴底」的位置，打字时的自动回底会破坏这个位置
    scrollOnUserInput: false,
    theme: TERMINAL_THEME,
  })
  const fit = new FitAddon()
  term.loadAddon(fit)
  // 输出里的 URL 直接可点，默认在新标签页打开
  term.loadAddon(new WebLinksAddon())
  term.open(host)
  // 用 key 而不是闭包里的 meta：会话 ID 是建立后写回响应式数组的，
  // 闭包拿到的是旧对象，按 id 判断会把所有按键都挡掉
  term.onData((data) => sendInput(meta.key, data))
  term.onResize(({ cols, rows }) => {
    updateSession(meta.key, { size: `${cols}×${rows}` })
    sendMessage(meta, { type: 'resize', cols, rows })
  })
  term.attachCustomKeyEventHandler((event) => handleShortcut(event))
  const observer = new ResizeObserver(() => fitSession(meta.key))
  observer.observe(host)
  const entry: SessionResources = { term, fit, observer, socket: null, following: true }
  // 视口停在尾部才算跟随；用户翻历史或看留白时不再自动钉回
  entry.scrollSub = term.onScroll(() => {
    // 补留白时我们也会滚屏，那不算用户操作
    if (entry.suppressScroll) return
    const wasFollowing = entry.following
    const viewportY = term.buffer.active.viewportY
    entry.following = Math.abs(viewportY - tailLine(term)) <= 1
    if (!wasFollowing && entry.following) scheduleScrollPadding(meta.key)
  })
  resources.set(meta.key, entry)
  fitSession(meta.key)
}

// fitSession 只在标签可见时重算尺寸：隐藏的标签页 offsetParent 为 null，强行 fit 会得到 0×0
function fitSession(key: string) {
  const item = resources.get(key)
  const host = hosts.get(key)
  if (!item || !host || host.offsetParent === null) return
  try {
    item.fit.fit()
  } catch { /* 容器尺寸尚未就绪时忽略 */ }
}

// ===== 滚动留白：让最后一行（光标行）能在视口里自由上下滚动 =====
// xterm 本身没有「滚过末尾」的选项，这里只在本地缓冲区尾部补空行：
// 补 N 行后把光标移回原行，再把视口钉回「光标行仍贴底」的位置，
// 于是鼠标往下滚就能把最后一行推到中间甚至顶部，往上滚还是普通历史回看。
// 这些转义序列只进本地终端，不会发给远端。

// tailLine 是「光标行贴在视口底部」时，视口顶部对应的缓冲区行号
function tailLine(term: Terminal): number {
  const buf = term.buffer.active
  const cursorAbs = buf.baseY + buf.cursorY
  return Math.max(0, Math.min(buf.baseY, cursorAbs + 1 - term.rows))
}

function scheduleScrollPadding(key: string) {
  const item = resources.get(key)
  if (!item) return
  if (item.padTimer !== undefined) window.clearTimeout(item.padTimer)
  item.padTimer = window.setTimeout(() => {
    item.padTimer = undefined
    applyScrollPadding(key)
  }, PAD_SYNC_DELAY_MS)
}

function clearPadTimer(item: SessionResources) {
  if (item.padTimer === undefined) return
  window.clearTimeout(item.padTimer)
  item.padTimer = undefined
}

// pinTail 只挪视口、不写缓冲区：让光标行继续贴在视口底行，留白藏在视口下方
function pinTail(key: string) {
  const item = resources.get(key)
  if (!item || !item.following) return
  item.term.scrollToLine(tailLine(item.term))
}

function applyScrollPadding(key: string) {
  const item = resources.get(key)
  // 用户翻历史时不打扰；等他滚回尾部再补
  if (!item || !item.following) return
  const term = item.term
  const buf = term.buffer.active
  // 备用屏（vim/top 等整屏程序）里不动；窗口太矮也没留白的意义
  if (buf.type !== 'normal' || term.rows < 4) return
  const cursorAbs = buf.baseY + buf.cursorY
  // 还没写满一屏时屏幕下方本来就是空的，不需要留白
  if (cursorAbs < term.rows - 1) return
  // 留白最多补到一屏：正好够把最后一行推到视口顶部
  const target = term.rows - 1
  const padBelow = Math.max(0, buf.length - 1 - cursorAbs)
  const add = Math.min(target - padBelow, buf.cursorY)
  if (add <= 0) {
    pinTail(key)
    return
  }
  const row = Math.max(1, buf.cursorY - add + 1)
  const col = buf.cursorX + 1
  // 一次写完：先到末行 → 补空行（触发滚动）→ 把光标放回原来的行列
  const sequence = `\x1b[${term.rows};1H${'\n'.repeat(add)}\x1b[${row};${col}H`
  item.suppressScroll = true
  term.write(sequence, () => {
    const current = resources.get(key)
    if (!current) return
    if (current.following) current.term.scrollToLine(tailLine(current.term))
    // 滚动事件是异步派发的，等它过去再恢复「用户滚动」的判定
    window.setTimeout(() => {
      const latest = resources.get(key)
      if (latest) latest.suppressScroll = false
    }, 60)
  })
}

// handleShortcut 接管 Ctrl/⌘+Shift+C / V：普通 Ctrl+C 仍要发给远端（中断信号）。
function handleShortcut(event: KeyboardEvent): boolean {
  const withModifier = event.ctrlKey || event.metaKey
  if (!withModifier || !event.shiftKey) return true
  if (event.code === 'KeyC') {
    void copySelection()
    return false
  }
  if (event.code === 'KeyV') {
    void pasteClipboard()
    return false
  }
  return true
}

async function createAndConnect(meta: SessionMeta) {
  const item = resources.get(meta.key)
  if (!item) return
  updateSession(meta.key, { state: 'connecting', reason: '' })
  try {
    const created = await api.createTerminalSession({
      server_id: meta.serverID,
      cols: item.term.cols,
      rows: item.term.rows,
    })
    updateSession(meta.key, { id: created.session_id, label: created.label || meta.label })
    openSocket(meta.key, created.session_id)
  } catch (error) {
    // 解锁在打开会话的瞬间过期：撤掉这个空会话，回到二次认证步骤
    if (error instanceof ApiRequestError && error.code === 'terminal_locked') {
      unlockExpiresAt.value = 0
      unlockError.value = '解锁已过期，请重新输入管理员密码'
      await closeSession(meta)
      pickerVisible.value = true
      return
    }
    updateSession(meta.key, {
      state: 'error',
      reason: error instanceof Error ? error.message : '建立会话失败',
    })
  }
}

function openSocket(key: string, id: string) {
  const item = resources.get(key)
  if (!item) return
  closeSocket(key)
  const ws = new WebSocket(terminalSocketURL(id))
  item.socket = ws
  item.connectTimer = window.setTimeout(() => {
    if (currentState(key) !== 'connecting') return
    updateSession(key, { state: 'error', reason: '连接超时：请确认 Havline 服务可达；开发环境下 Vite 代理需要开启 ws' })
    try { ws.close() } catch { /* 忽略关闭异常 */ }
  }, CONNECT_TIMEOUT_MS)
  ws.onopen = () => {
    clearConnectTimer(item)
    updateSession(key, { state: 'ready', reason: '' })
    item.term.focus()
    item.pingTimer = window.setInterval(() => sendMessageByKey(key, { type: 'ping' }), PING_INTERVAL_MS)
  }
  ws.onmessage = (event) => handleSocketMessage(key, event.data)
  ws.onerror = () => {
    if (currentState(key) === 'ready') updateSession(key, { state: 'error', reason: '终端通道出错' })
  }
  ws.onclose = () => {
    clearConnectTimer(item)
    stopPing(item)
    const state = currentState(key)
    if (state === 'ready' || state === 'connecting') {
      updateSession(key, { state: 'closed', reason: currentReason(key) || '连接已断开' })
    }
  }
}

function handleSocketMessage(key: string, raw: unknown) {
  let payload: { type?: string; data?: string; reason?: string; message?: string }
  try {
    payload = JSON.parse(String(raw)) as { type?: string; data?: string; reason?: string; message?: string }
  } catch {
    return
  }
  const item = resources.get(key)
  if (payload.type === 'output' && payload.data) {
    if (!item) return
    // 写完先立刻跟随尾部（不注入序列），空闲后再补留白
    item.term.write(base64ToBytes(payload.data), () => {
      pinTail(key)
      scheduleScrollPadding(key)
    })
    return
  }
  if (payload.type === 'error') {
    updateSession(key, { state: 'error', reason: payload.message || '终端通道出错' })
    return
  }
  if (payload.type === 'exit') {
    updateSession(key, { state: 'closed', reason: payload.reason || '远端已断开' })
  }
}

function sendInput(key: string, data: string) {
  // 移动端只读模式下不发按键
  if (readOnly.value) return
  // 会话还没建好时 socket 不是 OPEN，sendMessageByKey 会自己丢掉这一帧
  sendMessageByKey(key, { type: 'input', data: bytesToBase64(new TextEncoder().encode(data)) })
}

function sendMessage(meta: SessionMeta, payload: Record<string, unknown>) {
  sendMessageByKey(meta.key, payload)
}

function sendMessageByKey(key: string, payload: Record<string, unknown>) {
  const item = resources.get(key)
  if (item?.socket?.readyState !== WebSocket.OPEN) return
  item.socket.send(JSON.stringify(payload))
}

async function reconnect(meta: SessionMeta) {
  const previous = meta.id
  closeSocket(meta.key)
  updateSession(meta.key, { id: '', state: 'connecting', reason: '' })
  if (previous) {
    try { await api.closeTerminalSession(previous) } catch { /* 旧会话可能已结束 */ }
  }
  // 保留滚动缓冲，只补一行分隔，便于对比断开前后
  resources.get(meta.key)?.term.write('\r\n\x1b[90m—— 重新连接 ——\x1b[0m\r\n')
  await createAndConnect(meta)
}

function reconnectActive() {
  if (activeSession.value) void reconnect(activeSession.value)
}

async function closeSession(meta: SessionMeta) {
  closeSocket(meta.key)
  const item = resources.get(meta.key)
  item?.scrollSub?.dispose()
  item?.observer.disconnect()
  item?.term.dispose()
  resources.delete(meta.key)
  hosts.delete(meta.key)
  sessions.value = sessions.value.filter((entry) => entry.key !== meta.key)
  if (activeKey.value === meta.key) activeKey.value = sessions.value[0]?.key ?? ''
  if (meta.id) {
    try { await api.closeTerminalSession(meta.id) } catch { /* 忽略关闭失败 */ }
  }
}

async function closeAll() {
  for (const meta of [...sessions.value]) await closeSession(meta)
  message.info('已关闭全部会话')
}

function closeSocket(key: string) {
  const item = resources.get(key)
  if (!item) return
  clearConnectTimer(item)
  clearPadTimer(item)
  stopPing(item)
  const socket = item.socket
  item.socket = null
  if (!socket) return
  socket.onopen = null
  socket.onmessage = null
  socket.onerror = null
  socket.onclose = null
  if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) socket.close()
}

function stopPing(item: SessionResources) {
  if (item.pingTimer === undefined) return
  window.clearInterval(item.pingTimer)
  item.pingTimer = undefined
}

function clearConnectTimer(item: SessionResources) {
  if (item.connectTimer === undefined) return
  window.clearTimeout(item.connectTimer)
  item.connectTimer = undefined
}

function updateSession(key: string, patch: Partial<SessionMeta>) {
  sessions.value = sessions.value.map((item) => (item.key === key ? { ...item, ...patch } : item))
}

function currentState(key: string): SessionState {
  return sessions.value.find((item) => item.key === key)?.state ?? 'closed'
}

function currentReason(key: string): string {
  return sessions.value.find((item) => item.key === key)?.reason ?? ''
}

async function copySelection() {
  const text = activeKey.value ? resources.get(activeKey.value)?.term.getSelection() : ''
  if (!text) {
    message.info('先在终端里选中要复制的文本')
    return
  }
  if (await copyText(text)) message.success('已复制选中内容')
  else message.error('复制失败，请手动选择文本复制')
}

async function pasteClipboard() {
  const meta = activeSession.value
  if (!meta) return
  if (readOnly.value) {
    message.info('当前为只读模式：先点工具条上的「允许输入」再粘贴')
    return
  }
  try {
    const text = await navigator.clipboard.readText()
    if (text) sendInput(meta.key, text)
  } catch {
    message.error('无法读取剪贴板：浏览器可能要求 HTTPS 或剪贴板权限')
  }
}

function clearScreen() {
  if (activeKey.value) resources.get(activeKey.value)?.term.clear()
}

function changeFont(delta: number) {
  const next = Math.min(20, Math.max(11, fontSize.value + delta))
  if (next === fontSize.value) return
  fontSize.value = next
  try { localStorage.setItem(FONT_SIZE_KEY, String(next)) } catch { /* 隐私模式下忽略 */ }
  for (const [key, item] of resources.entries()) {
    item.term.options.fontSize = next
    fitSession(key)
  }
}

// xterm 输出是任意字节，用 base64 传输可避免 UTF-8 边界被 JSON 破坏
function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index)
  return bytes
}

let mediaQuery: MediaQueryList | null = null
let clockTimer: number | undefined

function syncNarrow() {
  isNarrow.value = mediaQuery?.matches ?? false
  // 回到宽屏时收回移动端输入授权，下次窄屏仍从只读开始
  if (!isNarrow.value) allowMobileInput.value = false
}

// applyReadOnly 把只读状态同步到每个 xterm 实例：只读时按键不再送给远端
function applyReadOnly() {
  for (const item of resources.values()) item.term.options.disableStdin = readOnly.value
}

function toggleMobileInput() {
  allowMobileInput.value = !allowMobileInput.value
  message.info(allowMobileInput.value ? '已允许移动端输入，请谨慎执行命令' : '已切回只读模式')
}

watch(readOnly, () => applyReadOnly())

// 切换标签后新显示的终端尺寸才有效，这里补一次 fit
watch(activeKey, async () => {
  await nextTick()
  if (activeKey.value) fitSession(activeKey.value)
})

let resizeStartY = 0
let resizeStartHeight = 0

// maxStackHeight 让拖动条始终停在视口内：面板底边最多到窗口底部上方 24px，
// 否则手柄会滑出屏幕，用户就没法再抓回来。
function maxStackHeight(): number {
  const top = stackHost.value?.getBoundingClientRect().top
  if (top === undefined) return Math.max(240, window.innerHeight - 200)
  return Math.max(200, window.innerHeight - top - 24)
}

// clampStoredHeight 在窗口变小或恢复上次高度时把面板收回可视范围
function clampStoredHeight() {
  if (stackHeight.value === null) return
  const max = maxStackHeight()
  if (stackHeight.value > max) stackHeight.value = Math.round(max)
}

// startResize 拖动的是面板底边：手柄跟着指针走，向下拖放大、向上拖缩小
function startResize(event: PointerEvent) {
  if (!stackHost.value) return
  resizing.value = true
  resizeStartY = event.clientY
  resizeStartHeight = stackHost.value.getBoundingClientRect().height
  // 捕获指针：拖到窗口外也继续收事件，松手时能正常收尾
  const target = event.currentTarget
  if (target instanceof HTMLElement) {
    try {
      target.setPointerCapture(event.pointerId)
    } catch { /* 个别浏览器不支持捕获时忽略 */ }
  }
  window.addEventListener('pointermove', onResizeMove)
  window.addEventListener('pointerup', stopResize)
  event.preventDefault()
}

function onResizeMove(event: PointerEvent) {
  const max = maxStackHeight()
  // 方向：指针往下（clientY 变大）就变高，往上就变矮
  const next = resizeStartHeight + (event.clientY - resizeStartY)
  // 下限 0：手柄可以一路收到面板顶边框，但不能越过去
  stackHeight.value = Math.round(Math.min(max, Math.max(0, next)))
}

function stopResize() {
  if (!resizing.value) return
  resizing.value = false
  window.removeEventListener('pointermove', onResizeMove)
  window.removeEventListener('pointerup', stopResize)
  if (stackHeight.value !== null) {
    try {
      localStorage.setItem(HEIGHT_KEY, String(stackHeight.value))
    } catch { /* 隐私模式下忽略 */ }
  }
  if (activeKey.value) fitSession(activeKey.value)
}

// resetHeight 双击恢复自适应高度，并清掉本地记录
function resetHeight() {
  stackHeight.value = null
  try {
    localStorage.removeItem(HEIGHT_KEY)
  } catch { /* 隐私模式下忽略 */ }
  for (const key of resources.keys()) fitSession(key)
}

onMounted(() => {
  mediaQuery = window.matchMedia('(max-width: 820px)')
  syncNarrow()
  mediaQuery.addEventListener('change', syncNarrow)
  window.addEventListener('resize', clampStoredHeight)
  void load()
  void loadUnlock()
  void loadAudits()
  void loadCommands()
  void loadRecordings()
  void loadRecordingSetting()
  // 每 30 秒刷新一次「剩余解锁时间」，让票据过期能在界面上体现出来
  clockTimer = window.setInterval(() => {
    now.value = Date.now()
  }, 30_000)
})

onBeforeUnmount(() => {
  closePlayer()
  if (clockTimer !== undefined) window.clearInterval(clockTimer)
  mediaQuery?.removeEventListener('change', syncNarrow)
  window.removeEventListener('resize', clampStoredHeight)
  window.removeEventListener('pointermove', onResizeMove)
  window.removeEventListener('pointerup', stopResize)
  for (const meta of [...sessions.value]) {
    closeSocket(meta.key)
    const item = resources.get(meta.key)
    item?.scrollSub?.dispose()
    item?.observer.disconnect()
    item?.term.dispose()
    resources.delete(meta.key)
    if (meta.id) void api.closeTerminalSession(meta.id).catch(() => undefined)
  }
  sessions.value = []
})
</script>

<style scoped>
.term-alert { margin-bottom: var(--havline-space-4); }
/* 左侧终端工作区 + 右侧功能面板；窄屏时面板落到下方 */
.term-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 330px;
  gap: var(--havline-space-4);
  align-items: start;
}
.term-main { min-width: 0; }
.term-aside {
  padding: 0 var(--havline-space-4) var(--havline-space-4);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius);
  background: var(--havline-surface);
  box-shadow: var(--havline-shadow);
}
.term-aside :deep(.n-tabs-nav) { margin-bottom: var(--havline-space-3); }
.aside-actions { display: flex; align-items: center; gap: var(--havline-space-2); margin-bottom: var(--havline-space-3); }
.aside-note { margin: 0 0 var(--havline-space-3); font-size: 12px; line-height: 1.6; color: var(--havline-text-muted); }
.aside-list { display: grid; gap: 6px; max-height: 420px; overflow: auto; }
.aside-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
  text-align: left;
  font: inherit;
}
.aside-row--button { cursor: pointer; transition: border-color 0.15s ease; }
.aside-row--button:hover { border-color: var(--havline-brand-hover); }
.aside-row.is-disabled { cursor: not-allowed; opacity: 0.65; }
.aside-row__dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; background: var(--havline-disabled); }
.aside-row__dot.is-ready { background: var(--havline-success); }
.aside-row__dot.is-off { background: var(--havline-disabled); }
.aside-row__body { display: grid; gap: 2px; min-width: 0; flex: 1; }
.aside-row__title { font-size: 13px; font-weight: 600; color: var(--havline-text); overflow-wrap: anywhere; }
.aside-row__meta { font-size: 12px; color: var(--havline-text-muted); overflow-wrap: anywhere; }
.aside-row__hint { font-size: 12px; color: var(--havline-brand); flex-shrink: 0; }
.aside-empty { margin: 0; font-size: 13px; line-height: 1.6; color: var(--havline-text-muted); }
.aside-form { display: grid; gap: 6px; margin-bottom: var(--havline-space-3); }
@media (max-width: 1180px) {
  .term-workspace { grid-template-columns: 1fr; }
  .aside-list { max-height: 300px; }
}
.term-tabs { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-bottom: var(--havline-space-3); }
.term-tab {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  max-width: 240px;
  padding: 6px 10px;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-surface);
  font: inherit;
  font-size: 13px;
  color: var(--havline-text-secondary);
  cursor: pointer;
}
.term-tab:hover { border-color: var(--havline-brand-hover); }
.term-tab--active { border-color: var(--havline-brand); color: var(--havline-text); background: var(--havline-bg); }
.term-tab__dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; background: var(--havline-disabled); }
.term-tab__dot--connecting { background: var(--havline-warning); }
.term-tab__dot--ready { background: var(--havline-success); }
.term-tab__dot--error { background: var(--havline-error); }
.term-tab__label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.term-tab__close { font-size: 15px; line-height: 1; color: var(--havline-text-muted); }
.term-tab__close:hover { color: var(--havline-error); }
.term-empty { display: flex; align-items: center; gap: var(--havline-space-3); flex-wrap: wrap; }
.term-empty__hint { font-size: 12px; color: var(--havline-text-muted); }
.term-targets { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: var(--havline-space-3); }
.term-target {
  display: grid;
  gap: 6px;
  padding: 12px 14px;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-surface);
  text-align: left;
  font: inherit;
  cursor: pointer;
  transition: border-color 0.15s ease;
}
.term-target:hover:not(:disabled) { border-color: var(--havline-brand-hover); }
.term-target:disabled { cursor: not-allowed; opacity: 0.65; }
.term-target__top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.term-target__label { font-size: 14px; font-weight: 600; color: var(--havline-text); }
.term-target__detail { font-size: 12px; color: var(--havline-text-secondary); overflow-wrap: anywhere; }
.term-target__hint { font-size: 12px; color: var(--havline-text-muted); }
.term-hint { margin: 0; color: var(--havline-text-muted); font-size: 13px; }
/* 二次认证这一步：说明、密码框、错误与明文链路提示 */
.term-unlock { display: grid; gap: var(--havline-space-3); max-width: 460px; }
.term-unlock__lead { margin: 0; font-size: 13px; line-height: 1.6; color: var(--havline-text-secondary); }
.term-unlock__error { margin: 0; font-size: 12px; color: var(--havline-error); }
.term-unlock__warn { margin: 0; font-size: 12px; line-height: 1.5; color: var(--havline-warning); }
/* 录像回放画布：按录制时的列宽行数渲染，超出就滚动 */
.term-player {
  height: min(60vh, 520px);
  padding: 8px;
  border-radius: var(--havline-radius-sm);
  background: #0b1220;
  overflow: auto;
}
.term-panel { display: flex; flex-direction: column; border: 1px solid var(--havline-border); border-radius: var(--havline-radius); overflow: hidden; background: #0b1220; box-shadow: var(--havline-shadow); }
.term-panel__bar { display: flex; align-items: center; gap: var(--havline-space-3); flex-wrap: wrap; padding: 8px 14px; border-bottom: 1px solid var(--havline-border); background: var(--havline-surface); }
.term-panel__label { font-size: 13px; font-weight: 600; color: var(--havline-text); }
.term-panel__meta { font-size: 12px; color: var(--havline-text-muted); }
.term-panel__readonly { font-size: 12px; font-weight: 500; color: var(--havline-brand); background: var(--havline-bg); padding: 2px 8px; border-radius: 999px; }
.term-panel__reason { font-size: 12px; color: var(--havline-warning); }
.term-panel__tools { display: flex; align-items: center; gap: 4px; margin-left: auto; }
/* 默认高度跟着视口走，保证提示符行完整落在画布内；用户在分隔条拖动后用内联高度覆盖 */
.term-stack { position: relative; height: clamp(200px, calc(100vh - 320px), 1200px); }
/* 底部固定留 18px：不管 fit 取整结果如何，提示符都不会贴住面板下沿 */
.term-host { position: absolute; top: 0; left: 0; right: 0; bottom: 18px; padding: 8px 10px 0; }
.term-host :deep(.xterm) { height: 100%; }
.term-resizer {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  height: 12px;
  border-top: 1px solid var(--havline-border);
  background: var(--havline-surface);
  cursor: ns-resize;
  touch-action: none;
}
.term-resizer__grip { width: 44px; height: 4px; border-radius: 999px; background: var(--havline-border); transition: background 0.15s ease; }
.term-resizer:hover .term-resizer__grip,
.term-resizer--active .term-resizer__grip { background: var(--havline-brand); }
</style>
