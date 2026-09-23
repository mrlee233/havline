<template>
  <PageHeader
    title="公网反代"
    description="把穿透规则的域名发布到 VPS 的 Nginx：部署 / 移除、部署设置（HTTPS、跳转、白名单、Basic Auth、限流）、健康检查与配置查看"
  >
    <template #actions>
      <n-select
        v-model:value="selectedId"
        :options="serverOptions"
        placeholder="选择公网服务端"
        class="proxy-remote-select"
        :disabled="servers.length === 0"
      />
      <n-button :loading="manualRefreshing" :disabled="!server" @click="refreshAll({ silent: false })">
        <template #icon><n-icon :component="RefreshOutline" /></template>
        刷新
      </n-button>
    </template>
  </PageHeader>

  <LoadError v-if="loadError" :message="loadError" @retry="load" />

  <template v-else>
    <EmptyState
      v-if="servers.length === 0"
      title="尚未配置 FRP 服务端"
      description="先在「内网穿透」页添加公网服务端并保存 Agent/SSH 信息，再回到本页管理公网反代。"
    >
      <template #action>
        <router-link to="/frp"><n-button type="primary">去内网穿透页</n-button></router-link>
      </template>
    </EmptyState>

    <template v-else-if="server">
      <!-- 数据读取失败：必须可见（原先 loadErrorMsg 写了但从未渲染） -->
      <n-alert v-if="loadErrorMsg" type="error" :bordered="false" class="page-alert">{{ loadErrorMsg }}</n-alert>

      <!-- Nginx 前置条件：缺失 / 校验未通过 -->
      <n-alert v-if="nginxBlocking" :type="nginxBlocking.type" :bordered="false" class="page-alert">
        <div class="alert-body">
          <div class="alert-body__text">
            <strong>{{ nginxBlocking.title }}</strong>
            <span>{{ nginxBlocking.text }}</span>
            <span v-if="nginxBlocking.detail" class="mono alert-body__detail">{{ nginxBlocking.detail }}</span>
          </div>
          <n-button
            v-if="agentStatus?.nginx_missing"
            size="small"
            type="primary"
            :loading="busy === 'install-nginx'"
            :disabled="!agentStatusOk || (busy !== null && busy !== 'install-nginx')"
            @click="installNginx"
          >一键安装 Nginx</n-button>
        </div>
      </n-alert>

      <!-- 入口行：agent / frps 生命周期在另一页 -->
      <div class="entrance-row">
        <span class="entrance-row__text">
          Agent 与 frps 的安装、升级、启停、配置下发在「公网 Agent」页；本页只负责把规则发布到 Nginx
        </span>
        <router-link to="/frp-agent"><n-button size="small" quaternary>前往公网 Agent</n-button></router-link>
      </div>

      <!-- ① 统计区 -->
      <section class="stats-row" aria-label="反代概览">
        <div class="stat-card">
          <n-icon :component="LayersOutline" class="stat-card__icon stat-card__icon--blue" aria-hidden="true" />
          <div class="stat-card__value">{{ candidates.length }}</div>
          <div class="stat-card__label">可部署规则</div>
          <div class="stat-card__sub">来自「内网穿透」的 http 规则</div>
        </div>
        <div class="stat-card">
          <n-icon :component="CheckmarkCircleOutline" class="stat-card__icon" :class="deployedCount ? 'stat-card__icon--green' : 'stat-card__icon--amber'" aria-hidden="true" />
          <div class="stat-card__value">{{ deployedCount }}</div>
          <div class="stat-card__label">已生效</div>
          <div class="stat-card__sub">{{ pendingCount }} 条未部署 / 未加载</div>
        </div>
        <div class="stat-card">
          <n-icon :component="PulseOutline" class="stat-card__icon" :class="healthyCount ? 'stat-card__icon--green' : 'stat-card__icon--amber'" aria-hidden="true" />
          <div class="stat-card__value">{{ healthyCount }}<span class="stat-card__value-total">/{{ candidates.length }}</span></div>
          <div class="stat-card__label">健康通过</div>
          <div class="stat-card__sub">DNS · 证书 · 隧道 · 服务 四段全通过</div>
        </div>
        <div class="stat-card">
          <n-icon :component="SwapHorizontalOutline" class="stat-card__icon stat-card__icon--blue" aria-hidden="true" />
          <div class="stat-card__value">{{ totalConns }}</div>
          <div class="stat-card__label">当前连接</div>
          <div class="stat-card__sub">frps 侧实时连接数</div>
        </div>
      </section>

      <!-- ② 环境与 Nginx -->
      <FonuCard class="section-card nginx-card" title="Nginx（公网反代依赖）">
        <div class="section-body">
          <div class="field-grid">
            <div><label>可用性</label><span class="field-tags"><StatusBadge :kind="nginxKind" :text="nginxText" /></span></div>
            <div><label>进程</label><span>{{ statusLoaded ? (agentStatus?.nginx_running ? '运行中' : '未运行') : '—' }}</span></div>
            <div><label>版本</label><span class="mono">{{ agentStatus?.nginx_version || '—' }}</span></div>
            <div><label>配置校验</label><span class="field-tags">
              <StatusBadge v-if="statusLoaded" :kind="agentStatus?.nginx_ok ? 'success' : 'error'" :text="agentStatus?.nginx_ok ? 'nginx -t 通过' : '未通过'" />
              <span v-else>—</span>
            </span></div>
            <div><label>可执行文件</label><span class="mono mono-wrap">{{ agentStatus?.nginx_path || agentStatus?.nginx_binary || '—' }}</span></div>
            <div><label>vhost 配置目录</label><span class="mono mono-wrap">{{ agentStatus?.nginx_conf_dir || '—' }}</span></div>
            <div><label>端口监听</label><span class="field-tags">
              <StatusBadge :kind="agentStatus?.ports?.['80'] ? 'success' : 'warning'" :text="`80 ${agentStatus?.ports?.['80'] ? '✓' : '✗'}`" />
              <StatusBadge :kind="agentStatus?.ports?.['443'] ? 'success' : 'warning'" :text="`443 ${agentStatus?.ports?.['443'] ? '✓' : '✗'}`" />
            </span></div>
            <div><label>反代配置文件</label><span>{{ routeDetails.length }} 个</span></div>
          </div>
          <p v-if="agentStatus?.nginx_error" class="form-hint nginx-error mono">{{ agentStatus.nginx_error }}</p>
          <div class="agent-actions">
            <n-button
              v-if="agentStatus?.nginx_missing"
              size="small"
              type="primary"
              :loading="busy === 'install-nginx'"
              :disabled="busy !== null && busy !== 'install-nginx'"
              @click="installNginx"
            >一键安装 Nginx</n-button>
            <n-button size="small" :loading="manualRefreshing" @click="refreshAll({ silent: false })">重新检测</n-button>
          </div>
        </div>
      </FonuCard>

      <!-- ③ 规则台账 -->
      <FonuCard class="section-card" title="规则台账">
        <div class="section-body">
          <p class="form-hint">穿透规则保存后不会自动改 VPS：点「部署反代」才会发布到 Nginx（自动推送证书并做回源自检）</p>
          <div v-if="candidates.length" class="route-table-wrap">
            <n-data-table
              class="route-table"
              :columns="columns"
              :data="candidates"
              :bordered="false"
              size="small"
              :scroll-x="1790"
              :row-key="(row: FrpProxy) => row.id"
            />
          </div>
          <EmptyState
            v-else
            title="暂无可部署的反代规则"
            description="公网反代的来源是穿透规则：先到「内网穿透」页创建启用的 http 类型规则并填写自定义域名。"
          >
            <template #action>
              <router-link to="/frp"><n-button type="primary">去内网穿透页</n-button></router-link>
            </template>
          </EmptyState>

          <details v-if="routeDetails.length" class="raw-routes">
            <summary>VPS 上 agent 管理的路由文件（{{ routeDetails.length }}）</summary>
            <div class="raw-routes__list">
              <div v-for="detail in routeDetails" :key="detail.domain" class="raw-route-row">
                <span class="mono">{{ detail.domain }}</span>
                <StatusBadge :kind="detail.loaded ? 'success' : detail.syntax_ok ? 'warning' : 'error'" :text="detail.loaded ? '已加载' : detail.syntax_ok ? '已写入未加载' : '语法错误'" />
              </div>
            </div>
          </details>
        </div>
      </FonuCard>

      <!-- 部署设置 -->
      <n-modal v-model:show="settingsModal" :mask-closable="false">
        <div class="proxy-modal proxy-modal--with-help">
          <div class="proxy-modal__form">
          <div class="proxy-modal__header">
            <h3 class="modal-title">部署设置 — {{ settingsTarget?.name }}</h3>
            <n-button size="small" quaternary @click="settingsModal = false">取消</n-button>
          </div>
          <div class="proxy-modal__tabbar">
            <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': settingsTab === 'basic' }" @click="settingsTab = 'basic'">基础</button>
            <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': settingsTab === 'security' }" @click="settingsTab = 'security'">安全</button>
            <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': settingsTab === 'advanced' }" @click="settingsTab = 'advanced'">高级</button>
          </div>
          <div class="proxy-modal__scroll">
            <n-form v-show="settingsTab === 'basic'" label-placement="top" class="proxy-modal__pane">
              <div class="form-switch-list">
                <div class="form-switch-row">
                  <div class="form-switch-row__text">
                    <div class="form-switch-row__label">启用 HTTPS</div>
                    <div class="form-switch-row__hint">证书已部署时监听 443；关闭后仅保留 80 明文入口</div>
                  </div>
                  <n-switch v-model:value="settingsForm.https_enabled" size="small" />
                </div>
                <div class="form-switch-row">
                  <div class="form-switch-row__text">
                    <div class="form-switch-row__label">HTTP 强制跳转 HTTPS</div>
                    <div class="form-switch-row__hint">80 端口访问自动 301 到 HTTPS（需启用 HTTPS）</div>
                  </div>
                  <n-switch v-model:value="settingsForm.redirect_https" size="small" :disabled="!settingsForm.https_enabled" />
                </div>
                <div class="form-switch-row">
                  <div class="form-switch-row__text">
                    <div class="form-switch-row__label">WebSocket 支持</div>
                    <div class="form-switch-row__hint">附加 Upgrade / Connection 头，实时服务（家庭影院、监控等）需要</div>
                  </div>
                  <n-switch v-model:value="settingsForm.websocket" size="small" />
                </div>
              </div>
              <div class="field-grid">
                <n-form-item label="上传大小限制">
                  <n-input v-model:value="settingsForm.client_max_body_size" placeholder="留空用 Nginx 默认，如 50m" />
                </n-form-item>
                <n-form-item label="代理读超时">
                  <n-input v-model:value="settingsForm.proxy_read_timeout" placeholder="留空用 Nginx 默认，如 3600s" />
                </n-form-item>
              </div>
            </n-form>
            <n-form v-show="settingsTab === 'security'" label-placement="top" class="proxy-modal__pane">
              <div class="panel-title">IP 访问控制</div>
              <div class="field-grid">
                <n-form-item label="IP 白名单（逗号分隔）">
                  <n-input v-model:value="settingsForm.allow_ips_text" placeholder="如 1.2.3.4,10.0.0.0/24，留空不限制" />
                </n-form-item>
                <n-form-item label="IP 黑名单（逗号分隔）">
                  <n-input v-model:value="settingsForm.deny_ips_text" placeholder="如 5.6.7.8，留空不限制" />
                </n-form-item>
              </div>
              <div class="form-switch-list">
                <div class="form-switch-row">
                  <div class="form-switch-row__text">
                    <div class="form-switch-row__label">仅中国大陆 IP</div>
                    <div class="form-switch-row__hint">部署时自动把本地维护的中国 IP 段下发到 VPS；非大陆 IP 返回 403</div>
                  </div>
                  <n-switch v-model:value="settingsForm.china_only" size="small" />
                </div>
              </div>
              <div class="panel-title">认证</div>
              <div class="field-grid">
                <n-form-item label="Basic Auth 用户名">
                  <n-input v-model:value="settingsForm.basic_auth_user" placeholder="留空不启用" />
                </n-form-item>
                <n-form-item label="Basic Auth 密码">
                  <n-input v-model:value="settingsForm.basic_auth_password" type="password" show-password-on="click" placeholder="留空则首次部署自动生成" />
                </n-form-item>
              </div>
              <div class="panel-title">流量控制</div>
              <div class="field-grid field-grid--three">
                <n-form-item label="请求限流（次/秒）">
                  <n-input-number v-model:value="settingsForm.rate_limit_rate" :min="0" :max="100000" placeholder="留空不限制" class="full-width" />
                </n-form-item>
                <n-form-item label="突发允许量">
                  <n-input-number v-model:value="settingsForm.rate_limit_burst" :min="0" :max="100000" placeholder="留空按 0" class="full-width" />
                </n-form-item>
                <n-form-item label="连接数上限（每 IP）">
                  <n-input-number v-model:value="settingsForm.conn_limit_max" :min="0" :max="100000" placeholder="留空不限制" class="full-width" />
                </n-form-item>
              </div>
            </n-form>
            <n-form v-show="settingsTab === 'advanced'" label-placement="top" class="proxy-modal__pane">
              <div class="form-switch-list">
                <div class="form-switch-row">
                  <div class="form-switch-row__text">
                    <div class="form-switch-row__label">安全响应头</div>
                    <div class="form-switch-row__hint">HSTS / X-Frame-Options / X-Content-Type-Options / Referrer-Policy</div>
                  </div>
                  <n-switch v-model:value="settingsForm.security_headers" size="small" />
                </div>
                <div class="form-switch-row">
                  <div class="form-switch-row__text">
                    <div class="form-switch-row__label">仅 TLS 1.3</div>
                    <div class="form-switch-row__hint">只允许 TLS 1.3 握手（需启用 HTTPS）</div>
                  </div>
                  <n-switch v-model:value="settingsForm.tls13_only" size="small" :disabled="!settingsForm.https_enabled" />
                </div>
              </div>
              <p class="field-hint">
                这些选项存放在穿透规则的 options 里，点「保存并重新部署」后立即生效。Basic Auth 密码留空时首次部署自动生成，写在生成的配置注释中（点「查看配置」可见）。
              </p>
            </n-form>
          </div>
          <div class="modal-footer">
            <n-button @click="settingsModal = false">取消</n-button>
            <n-button type="primary" :loading="busy === `save-settings:${settingsTarget?.id}`" :disabled="busy !== null && busy !== `save-settings:${settingsTarget?.id}`" @click="saveDeploySettings">保存并重新部署</n-button>
          </div>
          </div>
          <aside class="proxy-modal__help" aria-label="配置说明">
            <h4>配置说明</h4>
            <ol>
              <li><strong>保存后会发生什么</strong>：选项写入穿透规则 options → 立即重新部署该域名 → 自动推送证书 → 回源自检（DNS / 证书 / 隧道 / 服务）。</li>
              <li><strong>前置条件</strong>：VPS 需已装 Nginx 且 nginx -t 通过；未装时页面顶部有「一键安装 Nginx」。</li>
              <li><strong>访问控制优先级</strong>：IP 白名单 &gt; 黑名单 &gt; 仅中国大陆 IP；后者依赖本地中国 IP 段，未更新时部署会明确报错。</li>
              <li><strong>Basic Auth</strong>：填用户名即启用；密码留空时首次部署自动生成随机密码，写在生成的 Nginx 配置注释里（点「查看配置」可见）。</li>
              <li><strong>限流与连接数</strong>：限流触发返回 429，超过每 IP 连接数返回 503；留空表示不限制。</li>
            </ol>
            <div class="proxy-modal__tip">
              <n-icon class="proxy-modal__tip-icon" :component="InformationCircleOutline" aria-hidden="true" />
              <span>改动只影响这一条规则的公网入口；域名与内网目标在「内网穿透」页修改。</span>
            </div>
          </aside>
        </div>
      </n-modal>

      <!-- 反代配置查看 -->
      <n-modal v-model:show="confModal" :mask-closable="true">
        <div class="proxy-modal">
          <div class="proxy-modal__header">
            <h3 class="modal-title">反代配置 — {{ confDomain }}</h3>
            <n-button size="small" quaternary @click="confModal = false">关闭</n-button>
          </div>
          <pre class="code-block conf-view">{{ confContent || '加载中…' }}</pre>
          <div class="modal-footer">
            <n-button :disabled="!confContent" @click="copyConf">复制</n-button>
            <n-button type="primary" @click="confModal = false">关闭</n-button>
          </div>
        </div>
      </n-modal>
    </template>
  </template>
</template>

<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NIcon, NPopconfirm, useMessage, type DataTableColumns } from 'naive-ui'
import {
  CheckmarkCircleOutline, CopyOutline, InformationCircleOutline, LayersOutline, OpenOutline, PulseOutline, RefreshOutline, SwapHorizontalOutline,
} from '@vicons/ionicons5'
import EmptyState from '../components/EmptyState.vue'
import FonuCard from '../components/FonuCard.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { api } from '../api/client'
import type {
  FrpAgentRouteDetail, FrpAgentRouteHealth, FrpAgentStatus, FrpProxy, FrpProxyTraffic, FrpServer,
} from '../api/types'
import { formatBytes, formatDate, formatRate, formatRelativeTime } from '../utils/format'

type Action =
  | `deploy:${number}` | `remove:${number}` | `conf:${number}` | `save-settings:${number}` | 'install-nginx'
type RouteState = 'loaded' | 'written' | 'broken' | 'none'
type BadgeKind = 'success' | 'warning' | 'error' | 'disabled'

const message = useMessage()
const servers = ref<FrpServer[]>([])
const selectedId = ref<number | null>(null)
const loadError = ref('')
const loadErrorMsg = ref('')
const manualRefreshing = ref(false)
const busy = ref<Action | null>(null)
const candidates = ref<FrpProxy[]>([])
const routeDetails = ref<FrpAgentRouteDetail[]>([])
const routeHealth = ref<FrpAgentRouteHealth[]>([])
const proxyTraffic = ref<Record<string, FrpProxyTraffic>>({})
const trafficErrors = ref<Record<string, string>>({})
const agentStatus = ref<FrpAgentStatus | null>(null)
const statusLoaded = ref(false)
const settingsModal = ref(false)
const settingsTarget = ref<FrpProxy | null>(null)
const settingsTab = ref<'basic' | 'security' | 'advanced'>('basic')
const settingsForm = ref({
  websocket: false,
  // 存储用反向字段 https_disabled（缺省 = 启用 HTTPS，保持旧数据行为）
  https_enabled: true,
  redirect_https: false,
  client_max_body_size: '',
  proxy_read_timeout: '',
  allow_ips_text: '',
  deny_ips_text: '',
  basic_auth_user: '',
  basic_auth_password: '',
  security_headers: false,
  tls13_only: false,
  china_only: false,
  rate_limit_rate: null as number | null,
  rate_limit_burst: null as number | null,
  conn_limit_max: null as number | null,
})
const confModal = ref(false)
const confDomain = ref('')
const confContent = ref('')
let timer: number | undefined

const server = computed(() => servers.value.find((item) => item.id === selectedId.value) ?? null)
const serverOptions = computed(() => servers.value
  .filter((item) => item.agent_configured || item.ssh_configured)
  .map((item) => ({ label: item.name, value: item.id })))
const agentStatusOk = computed(() => statusLoaded.value && agentStatus.value !== null)

// 统计：已生效 / 待处理 / 健康通过 / 当前连接
const deployedCount = computed(() => candidates.value.filter((item) => stateOf(item).state === 'loaded').length)
const pendingCount = computed(() => candidates.value.length - deployedCount.value)
const healthyCount = computed(() => candidates.value.filter((item) => isHealthy(item)).length)
const totalConns = computed(() => candidates.value.reduce((sum, item) => sum + trafficOf(item).cur_conns, 0))

// Nginx 前置条件提示：缺失 / 校验未通过
const nginxBlocking = computed<{ type: 'error' | 'warning'; title: string; text: string; detail?: string } | null>(() => {
  if (!statusLoaded.value) return null
  if (agentStatus.value?.nginx_missing) {
    return {
      type: 'error',
      title: 'VPS 未安装 Nginx',
      text: '公网反代需要 Nginx 承接 80/443 并终结 HTTPS。可点右侧按钮由 agent 自动安装（识别 apt-get / dnf / yum / apk，需 root），或手动执行 apt install -y nginx / yum install -y nginx。',
    }
  }
  if (agentStatus.value && agentStatus.value.nginx_ok === false) {
    return {
      type: 'warning',
      title: 'Nginx 配置校验未通过',
      text: '部署前请先修掉配置错误，否则新规则不会生效。',
      detail: agentStatus.value.nginx_error || 'nginx -t 执行失败',
    }
  }
  return null
})

const nginxKind = computed<BadgeKind>(() => {
  if (!statusLoaded.value) return 'disabled'
  if (agentStatus.value?.nginx_missing) return 'error'
  if (!agentStatus.value?.nginx_ok) return 'error'
  return agentStatus.value?.nginx_running === false ? 'warning' : 'success'
})
const nginxText = computed(() => {
  if (!statusLoaded.value) return '未读取'
  if (agentStatus.value?.nginx_missing) return '未安装'
  if (!agentStatus.value?.nginx_ok) return '配置校验未通过'
  return agentStatus.value?.nginx_running === false ? '已安装未运行' : '正常'
})

async function load() {
  loadError.value = ''
  try {
    servers.value = (await api.listFrpServers()).filter((item) => item.agent_configured || item.ssh_configured)
    if (selectedId.value === null || !servers.value.some((item) => item.id === selectedId.value)) {
      selectedId.value = servers.value[0]?.id ?? null
    }
    await refreshAll({ silent: false })
  } catch (error) { loadError.value = error instanceof Error ? error.message : '读取服务端失败' }
}

// refreshAll：单个接口失败不再中断整轮，失败项汇总成一句可见提示（原先 loadErrorMsg 从未渲染）
async function refreshAll(options: { silent?: boolean } = { silent: true }) {
  const current = server.value
  if (!current) return
  if (options.silent === false) manualRefreshing.value = true
  const failures: string[] = []
  const track = async <T>(label: string, fn: () => Promise<T>, assign: (value: T) => void) => {
    try { assign(await fn()) } catch (error) {
      failures.push(`${label}${error instanceof Error ? `：${error.message}` : '失败'}`)
    }
  }
  await track('规则列表', () => api.getFrpAgentRouteCandidates(current.id), (value) => { candidates.value = value })
  await track('路由文件', () => api.getFrpAgentRoutes(current.id), (value) => { routeDetails.value = value })
  await track('健康检查', () => api.getFrpAgentRouteHealth(current.id), (value) => { routeHealth.value = value })
  await track('流量采集', () => api.listFrpProxiesTraffic(), (value) => {
    proxyTraffic.value = value.items ?? {}
    trafficErrors.value = value.errors ?? {}
  })
  await track('Agent 状态', () => api.getFrpAgentStatus(current.id), (value) => {
    agentStatus.value = value
    statusLoaded.value = true
  })
  loadErrorMsg.value = failures.length ? `数据读取不完整 —— ${failures.join('；')}` : ''
  if (options.silent === false) manualRefreshing.value = false
}

// run：按动作粒度 loading + 串行化（按钮已 disabled，这里只是安全网）
async function run<T>(action: Action, source: string, fn: () => Promise<T>) {
  if (busy.value) return undefined
  busy.value = action
  try {
    const result = await fn()
    message.success(`${source}完成`)
    return result
  } catch (error) {
    const text = error instanceof Error ? error.message : `${source}失败`
    loadErrorMsg.value = `${source}失败 —— ${text}`
    message.error(`${source}失败`)
    return undefined
  } finally { busy.value = null }
}

// 接口返回的 custom_domains 理论上必填，但历史数据可能为 null；统一收口避免渲染期抛错
function domainsOf(candidate: FrpProxy) {
  return candidate.custom_domains ?? []
}

function vhostPortOf(candidate: FrpProxy) {
  const port = server.value?.options.vhost_http_port || 8080
  return candidate.type === 'https' ? (server.value?.options.vhost_https_port || 8443) : port
}

// 部署态：已生效 / 已写入未加载 / 语法错误 / 未部署
function stateOf(candidate: FrpProxy): { state: RouteState; label: string; kind: BadgeKind } {
  for (const domain of domainsOf(candidate)) {
    const detail = routeDetails.value.find((item) => item.domain === domain)
    if (!detail) continue
    if (detail.loaded) return { state: 'loaded', label: '已生效', kind: 'success' }
    if (detail.syntax_ok) return { state: 'written', label: '已写入未加载', kind: 'warning' }
    return { state: 'broken', label: '语法错误', kind: 'error' }
  }
  return { state: 'none', label: '未部署', kind: 'disabled' }
}

// 健康四段（按域名）：DNS / 证书 / 隧道 / 服务
function healthOf(domain: string) {
  const health = routeHealth.value.find((item) => item.domain === domain)
  return [
    { label: 'DNS', ok: health?.dns_ok ?? false, detail: health?.dns_detail || (health?.dns_ok ? '已指向服务端' : '未指向服务端或未检测') },
    { label: '证书', ok: health?.cert_ok ?? false, detail: health?.cert_detail || (health?.cert_ok ? '证书已覆盖该域名' : '未发现该域名的证书') },
    { label: '隧道', ok: health?.tunnel_ok ?? false, detail: health?.tunnel_detail || (health?.tunnel_ok ? '已注册到 frps' : 'frps 未发现该规则的隧道') },
    { label: '服务', ok: health?.service_ok ?? false, detail: health?.service_ok ? '内网服务可达' : '内网服务不可达' },
  ]
}

function isHealthy(candidate: FrpProxy) {
  const domains = domainsOf(candidate)
  if (!domains.length) return false
  return domains.every((domain) => healthOf(domain).every((item) => item.ok))
}

// 域名快捷操作：复制 / 新窗口打开（http 规则走 http，https 规则走 https）
async function copyDomain(domain: string) {
  try {
    await navigator.clipboard.writeText(domain)
    message.success(`已复制 ${domain}`)
  } catch { message.error('复制失败，请手动选择文本复制') }
}

function openDomain(candidate: FrpProxy, domain: string) {
  window.open(`${candidate.type === 'https' ? 'https' : 'http'}://${domain}`, '_blank', 'noopener')
}

// 协议：公网反代的规则 type 恒为 http，入口协议由部署设置的「启用 HTTPS」决定，故按 options 自动识别
function protocolOf(candidate: FrpProxy) {
  const httpsEnabled = !(candidate.options?.https_disabled ?? false)
  const redirect = candidate.options?.redirect_https ?? false
  return {
    label: httpsEnabled ? 'HTTPS' : 'HTTP',
    kind: (httpsEnabled ? 'success' : 'disabled') as BadgeKind,
    detail: httpsEnabled
      ? `443 由 Nginx 终结 TLS${redirect ? '，且 80 端口强制 301 跳转' : '，80 端口同时可用'}`
      : '仅 80 端口明文提供服务（部署设置里未启用 HTTPS）',
  }
}

// 采集失败时把原因挂到四个流量单元格的 title 上，不再单占一列
function trafficErrorOf(candidate: FrpProxy) {
  return trafficErrors.value[String(candidate.id)] || undefined
}

// 规则台账列定义：与本地反向代理页一致，用 n-data-table 呈现（列宽固定，各行列天然对齐）
const columns = computed<DataTableColumns<FrpProxy>>(() => [
  {
    title: '规则 / 域名', key: 'name', width: 250,
    render: (row) => h('div', { class: 'cell-name' }, [
      h('div', { class: 'cell-name__head' }, [
        h('strong', row.name),
        h(StatusBadge, { kind: stateOf(row).kind, text: stateOf(row).label }),
        row.enabled ? null : h('span', { class: 'cell-muted' }, '已停用'),
      ]),
      domainsOf(row).length
        ? h('div', { class: 'cell-domains' }, domainsOf(row).map((domain) => h('div', { class: 'cell-domain' }, [
            h('span', { class: 'mono cell-wrap' }, domain),
            h(NButton, { size: 'tiny', quaternary: true, title: `复制 ${domain}`, onClick: () => copyDomain(domain) }, { icon: () => h(NIcon, { component: CopyOutline }) }),
            h(NButton, { size: 'tiny', quaternary: true, title: `打开 ${domain}`, onClick: () => openDomain(row, domain) }, { icon: () => h(NIcon, { component: OpenOutline }) }),
          ])))
        : h('span', { class: 'mono cell-wrap' }, '未填域名'),
    ]),
  },
  {
    title: '协议', key: 'protocol', width: 90,
    render: (row) => {
      const protocol = protocolOf(row)
      return h('span', { title: protocol.detail }, [h(StatusBadge, { kind: protocol.kind, text: protocol.label })])
    },
  },
  {
    title: '上游 / 回源', key: 'upstream', width: 200,
    render: (row) => h('div', { class: 'cell-upstream' }, [
      h('span', { class: 'mono cell-wrap' }, `${row.local_ip}:${row.local_port}`),
      h('span', { class: 'mono cell-wrap cell-muted' }, `→ 127.0.0.1:${vhostPortOf(row)}`),
    ]),
  },
  {
    title: '健康（DNS · 证书 · 隧道 · 服务）', key: 'health', width: 260,
    render: (row) => h('div', { class: 'cell-health' }, domainsOf(row).flatMap((domain) =>
      healthOf(domain).map((item) => h('span', { title: item.detail },
        [h(StatusBadge, { kind: item.ok ? 'success' : 'error', text: item.label })])))),
  },
  {
    title: '连接', key: 'conns', width: 80, align: 'right',
    render: (row) => h('span', { class: 'mono' }, String(trafficOf(row).cur_conns)),
  },
  {
    title: '当前上传', key: 'in_rate', width: 100, align: 'right',
    render: (row) => h('span', { class: 'mono', title: trafficErrorOf(row) }, formatRate(trafficOf(row).traffic_in_rate)),
  },
  {
    title: '当前下载', key: 'out_rate', width: 100, align: 'right',
    render: (row) => h('span', { class: 'mono', title: trafficErrorOf(row) }, formatRate(trafficOf(row).traffic_out_rate)),
  },
  {
    title: '总上传', key: 'in_total', width: 95, align: 'right',
    render: (row) => h('span', { class: 'mono', title: trafficErrorOf(row) }, formatBytes(trafficOf(row).traffic_in)),
  },
  {
    title: '总下载', key: 'out_total', width: 95, align: 'right',
    render: (row) => h('span', { class: 'mono', title: trafficErrorOf(row) }, formatBytes(trafficOf(row).traffic_out)),
  },
  {
    title: '更新时间', key: 'updated_at', width: 155,
    render: (row) => h('div', { class: 'cell-time' }, [
      h('span', { class: 'mono' }, formatDate(row.updated_at)),
      h('span', { class: 'cell-muted' }, formatRelativeTime(row.updated_at)),
    ]),
  },
  {
    title: '操作', key: 'actions', width: 330, align: 'right',
    render: (row) => h('div', { class: 'cell-actions' }, [
      h(NButton, { size: 'small', quaternary: true, loading: busy.value === `conf:${row.id}`,
        disabled: busy.value !== null && busy.value !== `conf:${row.id}`, onClick: () => viewConf(row) },
        { default: () => '查看配置' }),
      h(NButton, { size: 'small', disabled: busy.value !== null, onClick: () => openDeploySettings(row) },
        { default: () => '部署设置' }),
      h(NButton, { size: 'small', type: 'primary', loading: busy.value === `deploy:${row.id}`,
        disabled: busy.value !== null && busy.value !== `deploy:${row.id}`, onClick: () => deployRoute(row) },
        { default: () => '部署反代' }),
      stateOf(row).state === 'none' ? null : h(NPopconfirm, { onPositiveClick: () => removeRoute(row) }, {
        trigger: () => h(NButton, { size: 'small', type: 'warning', quaternary: true,
          loading: busy.value === `remove:${row.id}`,
          disabled: busy.value !== null && busy.value !== `remove:${row.id}` },
          { default: () => '移除反代' }),
        default: () => `将从 VPS 删除 ${domainsOf(row).join('、')} 的反代配置并重载 Nginx（穿透规则本身保留），确认继续？`,
      }),
    ]),
  },
])
// trafficOf 返回该规则的 frps 流量快照；无数据时回落到全零，避免模板出现 undefined
function trafficOf(candidate: FrpProxy): FrpProxyTraffic {
  return proxyTraffic.value[String(candidate.id)] ?? {
    traffic_in: 0, traffic_out: 0, cur_conns: 0, traffic_in_rate: 0, traffic_out_rate: 0,
  }
}

async function deployRoute(candidate: FrpProxy) {
  const current = server.value
  if (!current) return
  const result = await run(`deploy:${candidate.id}`, '部署反代', () => api.deployFrpAgentRoute(current.id, candidate.id))
  if (result !== undefined) await refreshAll()
}

async function removeRoute(candidate: FrpProxy) {
  const current = server.value
  if (!current) return
  const result = await run(`remove:${candidate.id}`, '移除反代', () => api.removeFrpAgentRoute(current.id, candidate.id))
  if (result !== undefined) await refreshAll()
}

function openDeploySettings(candidate: FrpProxy) {
  settingsTarget.value = candidate
  settingsTab.value = 'basic'
  settingsForm.value = {
    websocket: candidate.options?.websocket ?? false,
    https_enabled: !(candidate.options?.https_disabled ?? false),
    redirect_https: candidate.options?.redirect_https ?? false,
    client_max_body_size: candidate.options?.client_max_body_size ?? '',
    proxy_read_timeout: candidate.options?.proxy_read_timeout ?? '',
    allow_ips_text: (candidate.options?.allow_ips || []).join(','),
    deny_ips_text: (candidate.options?.deny_ips || []).join(','),
    basic_auth_user: candidate.options?.basic_auth_user ?? '',
    basic_auth_password: candidate.options?.basic_auth_password ?? '',
    security_headers: candidate.options?.security_headers ?? false,
    tls13_only: candidate.options?.tls13_only ?? false,
    china_only: candidate.options?.china_only ?? false,
    rate_limit_rate: candidate.options?.rate_limit_rate ?? null,
    rate_limit_burst: candidate.options?.rate_limit_burst ?? null,
    conn_limit_max: candidate.options?.conn_limit_max ?? null,
  }
  settingsModal.value = true
}

async function saveDeploySettings() {
  const target = settingsTarget.value
  const current = server.value
  if (!target || !current) return
  // 选项存在规则的 options 里（与穿透配置同库），更新后立即重部署生效
  const options = {
    ...target.options,
    websocket: settingsForm.value.websocket,
    https_disabled: !settingsForm.value.https_enabled,
    redirect_https: settingsForm.value.redirect_https,
    client_max_body_size: settingsForm.value.client_max_body_size.trim(),
    proxy_read_timeout: settingsForm.value.proxy_read_timeout.trim(),
    allow_ips: settingsForm.value.allow_ips_text.split(',').map((s) => s.trim()).filter(Boolean),
    deny_ips: settingsForm.value.deny_ips_text.split(',').map((s) => s.trim()).filter(Boolean),
    basic_auth_user: settingsForm.value.basic_auth_user.trim(),
    basic_auth_password: settingsForm.value.basic_auth_password,
    security_headers: settingsForm.value.security_headers,
    tls13_only: settingsForm.value.tls13_only,
    china_only: settingsForm.value.china_only,
    rate_limit_rate: settingsForm.value.rate_limit_rate ?? 0,
    rate_limit_burst: settingsForm.value.rate_limit_burst ?? 0,
    conn_limit_max: settingsForm.value.conn_limit_max ?? 0,
  }
  const result = await run(`save-settings:${target.id}`, '保存部署设置', () => api.updateFrpProxy(target.id, {
    server_id: target.server_id, name: target.name, type: target.type,
    local_ip: target.local_ip, local_port: target.local_port,
    remote_port: target.remote_port ?? null, custom_domains: target.custom_domains,
    host_header_rewrite: target.host_header_rewrite, options,
    enabled: target.enabled, remark: target.remark,
  }))
  if (result === undefined) return
  settingsModal.value = false
  await deployRoute(target)
}

async function installNginx() {
  const current = server.value
  if (!current) return
  const result = await run('install-nginx', '安装 Nginx', () => api.installFrpAgentNginx(current.id))
  if (result && !result.ok) {
    message.warning(`安装命令已执行，但 Nginx 仍不可用：${(result.error || '').slice(0, 300)}`)
  }
  await refreshAll()
}

async function viewConf(candidate: FrpProxy) {
  const current = server.value
  if (!current) return
  if (busy.value) return
  busy.value = `conf:${candidate.id}`
  try {
    const conf = await api.getFrpAgentRouteConf(current.id, candidate.id)
    confDomain.value = conf.domain
    confContent.value = conf.content
    confModal.value = true
  } catch (error) {
    loadErrorMsg.value = `读取配置失败 —— ${error instanceof Error ? error.message : '未知错误'}`
  } finally { busy.value = null }
}

async function copyConf() {
  try {
    await navigator.clipboard.writeText(confContent.value)
    message.success('已复制反代配置')
  } catch { message.error('复制失败，请手动选择文本复制') }
}

// 切换服务端必须重置，避免上一台的健康 / 流量 / agent 状态串台
watch(selectedId, () => {
  candidates.value = []
  routeDetails.value = []
  routeHealth.value = []
  proxyTraffic.value = {}
  trafficErrors.value = {}
  agentStatus.value = null
  statusLoaded.value = false
  loadErrorMsg.value = ''
  void refreshAll({ silent: false })
})

onMounted(() => {
  void load()
  timer = window.setInterval(() => {
    if (!server.value) return
    if (document.visibilityState === 'hidden') return // 页面不可见时暂停轮询
    if (busy.value) return                            // 有操作进行中不叠加轮询
    void refreshAll()
  }, 15000)
})

onBeforeUnmount(() => { if (timer) window.clearInterval(timer) })
</script>

<style scoped>
/* 分区：提示 → 入口行 → ① 统计区 → ② 环境与 Nginx → ③ 规则台账 */
.page-alert { margin-bottom: var(--fonu-space-3); }
.alert-body { display: flex; align-items: center; justify-content: space-between; gap: var(--fonu-space-4); }
.alert-body__text { display: grid; gap: 4px; font-size: 13px; }
.alert-body__detail { font-size: 12px; color: var(--fonu-error); overflow-wrap: anywhere; }
.entrance-row { display: flex; align-items: center; justify-content: space-between; gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-5); padding: 10px var(--fonu-space-5); border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm); background: var(--fonu-info-soft); }
.entrance-row__text { font-size: 13px; color: var(--fonu-text); }

.stats-row { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-5); }
.stat-card { background: var(--fonu-surface); border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius); box-shadow: var(--fonu-shadow); padding: var(--fonu-space-4) var(--fonu-space-5); min-height: 118px; }
.stat-card__icon { font-size: 18px; margin-bottom: 4px; }
.stat-card__icon--blue { color: var(--fonu-info); }
.stat-card__icon--green { color: var(--fonu-brand); }
.stat-card__icon--amber { color: var(--fonu-warning); }
.stat-card__value { margin-top: 4px; font-size: 30px; font-weight: 700; line-height: 1.1; color: var(--fonu-text); letter-spacing: -0.03em; }
.stat-card__value-total { margin-left: 2px; font-size: 15px; font-weight: 600; color: var(--fonu-text-muted); }
.stat-card__label { margin-top: 8px; font-size: 13px; font-weight: 600; color: var(--fonu-text); }
.stat-card__sub { margin-top: 4px; font-size: 12px; color: var(--fonu-text-muted); }

/* FonuCard 自身已有 24px 内边距，这里不再叠加（与 FrpView 的用法一致） */
.section-card { margin-bottom: var(--fonu-space-5); }
.section-body { padding: 0; }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--fonu-space-3) var(--fonu-space-4); }
/* Nginx 卡的 label/value 双列：只在本卡生效，避免影响弹窗里 n-form-item 的内部布局 */
.nginx-card .field-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.nginx-card .field-grid > div { display: grid; gap: 4px; min-width: 0; }
.nginx-card .field-grid label { color: var(--fonu-text-secondary); font-size: 12px; }
.field-tags { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.field-grid--three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.full-width { width: 100%; }
.agent-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; margin-top: var(--fonu-space-3); }
.nginx-error { margin-top: var(--fonu-space-2); color: var(--fonu-error); overflow-wrap: anywhere; }

/* 规则台账：与本地反向代理页一致，用 n-data-table 呈现（列宽固定，各行列天然对齐） */
/* 单元格元素由 render 函数的 h() 创建，不带 scoped 的 data-v 属性，必须用 :deep() 才能命中 */
.route-table-wrap { border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm); overflow: hidden; }
.route-table-wrap :deep(.cell-name) { display: grid; gap: 4px; }
.route-table-wrap :deep(.cell-name__head) { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.route-table-wrap :deep(.cell-name strong) { font-size: 13.5px; color: var(--fonu-text); }
.route-table-wrap :deep(.cell-muted) { font-size: 12px; color: var(--fonu-text-muted); }
.route-table-wrap :deep(.cell-wrap) { overflow-wrap: anywhere; }
.route-table-wrap :deep(.cell-health) { display: flex; gap: var(--fonu-space-2); flex-wrap: wrap; }
.route-table-wrap :deep(.cell-upstream) { display: grid; gap: 2px; }
.route-table-wrap :deep(.cell-time) { display: grid; gap: 2px; }
.route-table-wrap :deep(.cell-domains) { display: grid; gap: 2px; }
.route-table-wrap :deep(.cell-domain) { display: flex; align-items: center; gap: 6px; min-width: 0; }
/* 单元格内容整体上下居中（行高被多行内容撑高时，按钮/文字不再贴顶） */
.route-table-wrap :deep(.n-data-table-td) { vertical-align: middle; }
.route-table-wrap :deep(.cell-name) { align-content: center; }
/* 域名不撑满整格：按钮紧跟在域名后面（不贴近右侧的协议列） */
.route-table-wrap :deep(.cell-domain .mono) { flex: 0 1 auto; min-width: 0; }
.route-table-wrap :deep(.cell-actions) { display: flex; align-items: center; justify-content: flex-end; gap: 6px; flex-wrap: wrap; }
.route-table-wrap :deep(.mono) { font-family: var(--fonu-mono); font-size: 13px; }

.raw-routes { margin-top: var(--fonu-space-4); font-size: 12px; color: var(--fonu-text-secondary); }
.raw-routes summary { cursor: pointer; color: var(--fonu-text-secondary); }
.raw-routes summary:hover { color: var(--fonu-text); }
.raw-routes__list { display: grid; gap: 6px; margin-top: var(--fonu-space-3); }
.raw-route-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.raw-route-row .mono { color: var(--fonu-text); }

/* 弹窗样式对齐本地反向代理页（ProxyView）的规则弹窗：900px 宽、无外层内边距、头部/tabbar/页脚各自带内边距 */
.proxy-modal { display: flex; flex-direction: column; width: 900px; max-width: 95vw; max-height: 90vh; background: var(--fonu-surface); border-radius: var(--fonu-radius); overflow: hidden; box-shadow: var(--fonu-shadow-md); }
/* 带右侧说明栏的弹窗：整体改为横向，左表单列 + 右说明栏 */
.proxy-modal--with-help { flex-direction: row; }
.proxy-modal__form { flex: 1; min-width: 0; display: flex; flex-direction: column; max-height: 90vh; }
.proxy-modal__help { width: 300px; flex-shrink: 0; padding: var(--fonu-space-5); background: var(--fonu-bg-muted); border-left: 1px solid var(--fonu-border); font-size: 13px; color: var(--fonu-text-secondary); overflow: auto; }
.proxy-modal__help h4 { margin: 0 0 var(--fonu-space-3); font-size: 14px; font-weight: 600; color: var(--fonu-text); }
.proxy-modal__help ol { margin: 0; padding-left: 18px; line-height: 1.75; }
.proxy-modal__help li + li { margin-top: 10px; }
.proxy-modal__help code { font-size: 12px; background: var(--fonu-surface); padding: 1px 4px; border-radius: 4px; }
.proxy-modal__tip { display: flex; align-items: flex-start; gap: 8px; margin-top: var(--fonu-space-5); padding: 12px; border-radius: 8px; background: var(--fonu-info-soft); color: var(--fonu-info); font-size: 12px; line-height: 1.6; }
.proxy-modal__tip-icon { flex-shrink: 0; margin-top: 1px; font-size: 16px; }
.proxy-modal__header { display: flex; align-items: center; justify-content: space-between; gap: var(--fonu-space-3); padding: var(--fonu-space-5) var(--fonu-space-5) 0; flex-shrink: 0; }
.proxy-modal__tabbar { display: flex; gap: var(--fonu-space-5); padding: var(--fonu-space-3) var(--fonu-space-5) 0; border-bottom: 1px solid var(--fonu-border); flex-shrink: 0; }
.proxy-modal__tab { margin: 0; padding: 8px 2px 10px; border: none; background: none; font: inherit; font-size: 14px; color: var(--fonu-text-secondary); cursor: pointer; border-bottom: 2px solid transparent; margin-bottom: -1px; }
.proxy-modal__tab:hover { color: var(--fonu-text); }
.proxy-modal__tab--active { color: var(--fonu-brand-text); font-weight: 600; border-bottom-color: var(--fonu-brand); }
.proxy-modal__scroll { flex: 1; min-height: 0; overflow-y: auto; }
.proxy-modal__pane { width: 100%; box-sizing: border-box; padding: var(--fonu-space-4) var(--fonu-space-5) 0; }
.proxy-modal__pane :deep(.n-form) { width: 100%; }
.proxy-modal__pane :deep(.n-form-item .n-form-item-blank) { display: block; width: 100%; }
.modal-footer { display: flex; justify-content: flex-end; gap: var(--fonu-space-3); padding: var(--fonu-space-4) var(--fonu-space-5) var(--fonu-space-5); border-top: 1px solid var(--fonu-border); margin-top: var(--fonu-space-2); flex-shrink: 0; }
.form-switch-list { display: flex; flex-direction: column; gap: 14px; margin-bottom: var(--fonu-space-4); }
.form-switch-row { display: flex; align-items: center; justify-content: space-between; gap: var(--fonu-space-4); padding: 12px 14px; border: 1px solid var(--fonu-border); border-radius: 10px; background: var(--fonu-bg); }
.form-switch-row__label { font-size: 14px; font-weight: 600; color: var(--fonu-text); }
.form-switch-row__hint { margin-top: 4px; font-size: 12px; color: var(--fonu-text-muted); line-height: 1.5; }
.field-hint { margin: 0; font-size: 12px; color: var(--fonu-text-muted); line-height: 1.5; }
.panel-title { margin: var(--fonu-space-4) 0 var(--fonu-space-3); font-size: 14px; font-weight: 600; color: var(--fonu-text); }
.modal-title { margin: 0; font-size: 18px; font-weight: 600; color: var(--fonu-text); }
.code-block {
  margin: 0; padding: var(--fonu-space-3) var(--fonu-space-4); background: var(--fonu-bg-muted);
  border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm);
  font-family: var(--fonu-mono); font-size: 12.5px; line-height: 1.65; color: var(--fonu-text);
  overflow: auto; white-space: pre-wrap; word-break: break-all;
}
.conf-view { max-height: 60vh; }
.form-hint { color: var(--fonu-text-secondary); font-size: 12.5px; }
.mono { font-family: var(--fonu-mono); font-size: 13px; }
.mono-wrap { white-space: normal; word-break: break-all; }
.proxy-remote-select { width: 220px; }

/* 首屏一次性入场；用户偏好减少动效时关闭 */
@keyframes fonu-fade-up {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: none; }
}
.stats-row, .entrance-row, .section-card { animation: fonu-fade-up 160ms ease-out both; }
@media (prefers-reduced-motion: reduce) {
  .stats-row, .entrance-row, .section-card { animation: none; }
}

@media (max-width: 1199px) {
  .stats-row { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .nginx-card .field-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  /* 台账为表格：窄屏靠 scroll-x 横向滚动，不改列 */
  .proxy-modal__help { display: none; }
}
@media (max-width: 900px) {
  .field-grid { grid-template-columns: 1fr; }
  .field-grid--three { grid-template-columns: 1fr; }
  .nginx-card .field-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 760px) {
  .stats-row { grid-template-columns: 1fr; }
  .nginx-card .field-grid { grid-template-columns: 1fr; }
  .entrance-row { flex-direction: column; align-items: flex-start; }
  .route-table-wrap :deep(.cell-actions) { width: 100%; justify-content: flex-start; }
  .route-table-wrap :deep(.cell-actions .n-button) { min-height: 44px; }
  .alert-body { flex-direction: column; align-items: flex-start; }
}
</style>
