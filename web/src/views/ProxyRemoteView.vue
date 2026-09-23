<template>
  <PageHeader title="公网反代" description="管理 VPS 上 Nginx 反代：部署/移除穿透规则的公网入口，支持强制跳转、IP 白名单、Basic Auth 与 WebSocket">
    <template #actions>
      <n-select
        v-model:value="selectedId"
        :options="serverOptions"
        placeholder="选择公网服务端"
        style="width: 220px"
        :disabled="servers.length === 0"
      />
      <n-button :loading="refreshing" @click="refreshAll">刷新</n-button>
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
      <div v-if="agentStatus?.nginx_missing" class="nginx-alert">
        <div class="nginx-alert__text">
          <strong>VPS 未安装 Nginx</strong>
          <span>公网反代需要 Nginx 承接 80/443 并终结 HTTPS。可点右侧按钮由 agent 自动安装（识别 apt-get / dnf / yum / apk，需 root），或在 VPS 手动执行：<code>apt install -y nginx</code> / <code>yum install -y nginx</code></span>
        </div>
        <n-button size="small" type="primary" :loading="installingNginx" @click="installNginx">一键安装 Nginx</n-button>
      </div>
      <div v-else-if="agentStatus && agentStatus.nginx_ok === false" class="nginx-alert nginx-alert--warn">
        <div class="nginx-alert__text">
          <strong>Nginx 配置校验未通过</strong>
          <span class="mono">{{ agentStatus.nginx_error || 'nginx -t 执行失败' }}</span>
        </div>
      </div>

      <FonuCard class="section-card" title="Nginx 状态">
        <div class="section-body">
          <div class="nginx-status-grid">
            <div class="nginx-status-item">
              <span class="nginx-status-item__label">状态</span>
              <span class="nginx-status-item__value">
                <n-tag :type="nginxState().type" size="small" :bordered="false">{{ nginxState().label }}</n-tag>
              </span>
            </div>
            <div class="nginx-status-item">
              <span class="nginx-status-item__label">进程</span>
              <span class="nginx-status-item__value">{{ agentStatus?.nginx_running ? '运行中' : '未运行' }}</span>
            </div>
            <div class="nginx-status-item">
              <span class="nginx-status-item__label">版本</span>
              <span class="nginx-status-item__value mono">{{ agentStatus?.nginx_version || '—' }}</span>
            </div>
            <div class="nginx-status-item">
              <span class="nginx-status-item__label">可执行文件</span>
              <span class="nginx-status-item__value mono">{{ agentStatus?.nginx_path || agentStatus?.nginx_binary || '—' }}</span>
            </div>
            <div class="nginx-status-item">
              <span class="nginx-status-item__label">vhost 配置目录</span>
              <span class="nginx-status-item__value mono">{{ agentStatus?.nginx_conf_dir || '—' }}</span>
            </div>
            <div class="nginx-status-item">
              <span class="nginx-status-item__label">配置校验</span>
              <span class="nginx-status-item__value">{{ agentStatus?.nginx_ok ? 'nginx -t 通过' : '未通过' }}</span>
            </div>
            <div class="nginx-status-item">
              <span class="nginx-status-item__label">端口监听</span>
              <span class="nginx-status-item__value mono">80 {{ agentStatus?.ports?.['80'] ? '✓' : '✗' }} · 443 {{ agentStatus?.ports?.['443'] ? '✓' : '✗' }}</span>
            </div>
            <div class="nginx-status-item">
              <span class="nginx-status-item__label">反代配置文件</span>
              <span class="nginx-status-item__value">{{ agentStatus?.routes?.length ?? 0 }} 个</span>
            </div>
          </div>
          <p v-if="agentStatus?.nginx_error" class="form-hint mono nginx-status-error">{{ agentStatus.nginx_error }}</p>
          <div class="nginx-status-actions">
            <n-button v-if="agentStatus?.nginx_missing" size="small" type="primary" :loading="installingNginx" @click="installNginx">一键安装 Nginx</n-button>
            <n-button size="small" :loading="refreshing" @click="refreshAll">重新检测</n-button>
          </div>
        </div>
      </FonuCard>

      <FonuCard class="section-card" title="穿透规则与公网反代状态">
        <div class="section-body">
          <p class="form-hint">穿透规则保存后不会自动修改 VPS。点击「部署反代」把该规则的域名发布到公网 Nginx；部署时自动推送证书并做回源自检。</p>
          <div v-if="routeCandidates.length" class="route-list">
            <div v-for="candidate in routeCandidates" :key="candidate.id" class="route-item">
              <div class="route-item__main">
                <strong>{{ candidate.name }}</strong>
                <span class="route-item__domains mono">{{ candidate.custom_domains.join('、') }}</span>
                <span class="route-item__upstream mono">{{ candidate.local_ip }}:{{ candidate.local_port }}（回源 127.0.0.1:{{ vhostPortOf(candidate) }}）</span>
                <div class="route-item__health">
                  <span v-for="h in healthBadges(candidate)" :key="h.label" class="health-badge" :class="h.ok ? 'health-ok' : 'health-bad'" :title="h.detail">{{ h.label }} {{ h.ok ? '✓' : '✗' }}</span>
                </div>
                <div class="route-item__traffic">
                  <span class="traffic-metric" title="frps 侧当前连接数">连接 {{ trafficOf(candidate).cur_conns }}</span>
                  <span class="traffic-metric" title="当前上传速率（近两次采样均值）">当前上传 {{ formatRate(trafficOf(candidate).traffic_in_rate) }}</span>
                  <span class="traffic-metric" title="当前下载速率（近两次采样均值）">当前下载 {{ formatRate(trafficOf(candidate).traffic_out_rate) }}</span>
                  <span class="traffic-metric" title="frps 侧当日累计上传">总上传 {{ formatBytes(trafficOf(candidate).traffic_in) }}</span>
                  <span class="traffic-metric" title="frps 侧当日累计下载">总下载 {{ formatBytes(trafficOf(candidate).traffic_out) }}</span>
                  <span v-if="trafficErrors[String(candidate.id)]" class="traffic-metric traffic-metric--error" :title="trafficErrors[String(candidate.id)]">流量采集失败</span>
                </div>
              </div>
              <div class="route-item__actions">
                <n-tag :type="stateMeta(candidate).type" size="small" :bordered="false">{{ stateMeta(candidate).label }}</n-tag>
                <n-button size="small" quaternary :loading="confLoadingId === candidate.id" @click="viewConf(candidate)">查看配置</n-button>
                <n-button size="small" @click="openDeploySettings(candidate)">部署设置</n-button>
                <n-button size="small" type="primary" :loading="routeActionId === candidate.id" @click="deployRoute(candidate)">部署反代</n-button>
                <n-button v-if="stateOf(candidate) !== 'none'" size="small" type="warning" quaternary :loading="routeActionId === candidate.id" @click="removeRoute(candidate)">移除反代</n-button>
              </div>
            </div>
          </div>
          <div v-else class="empty-route">暂无可部署的 HTTP/HTTPS 穿透规则——先到「内网穿透」页创建启用的 http 类型规则（填自定义域名）</div>
          <details v-if="routeDetails.length" class="raw-routes">
            <summary>VPS 上 agent 管理的路由文件（{{ routeDetails.length }}）</summary>
            <div v-for="detail in routeDetails" :key="detail.domain" class="raw-route-row mono">
              {{ detail.domain }} — {{ detail.loaded ? '已加载' : detail.syntax_ok ? '已写入未加载' : '语法错误' }}
            </div>
          </details>
        </div>
      </FonuCard>

      <FonuCard class="section-card" title="使用说明">
        <div class="section-body">
          <p class="form-hint">部署选项（启用 HTTPS / 强制跳转 / WebSocket / 上传大小 / 代理超时 / IP 白名单与黑名单 / Basic Auth / 仅 TLS 1.3 / 安全响应头）在本页规则行的「部署设置」里配置，点「保存并重新部署」即生效。Basic Auth 密码留空时首次部署自动生成，写在生成的配置注释里（点「查看配置」可见）。注意：这些选项需要 VPS 上已升级到新版 agent。</p>
        </div>
      </FonuCard>

      <n-modal
        v-model:show="settingsModal"
        :mask-closable="false"
        style="width: 640px"
      >
        <div class="proxy-modal">
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
                  <n-input-number v-model:value="settingsForm.rate_limit_rate" :min="0" :max="100000" placeholder="留空不限制" style="width: 100%" />
                </n-form-item>
                <n-form-item label="突发允许量">
                  <n-input-number v-model:value="settingsForm.rate_limit_burst" :min="0" :max="100000" placeholder="留空按 0" style="width: 100%" />
                </n-form-item>
                <n-form-item label="连接数上限（每 IP）">
                  <n-input-number v-model:value="settingsForm.conn_limit_max" :min="0" :max="100000" placeholder="留空不限制" style="width: 100%" />
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
            </n-form>
          </div>
          <div class="proxy-modal__footer">
            <n-button size="small" @click="settingsModal = false">取消</n-button>
            <n-button size="small" type="primary" :loading="routeActionId === settingsTarget?.id" @click="saveDeploySettings">保存并重新部署</n-button>
          </div>
        </div>
      </n-modal>

      <n-modal
        v-model:show="confModal"
        :mask-closable="true"
        style="width: 720px"
      >
        <div class="proxy-modal">
          <div class="proxy-modal__header">
            <h3 class="modal-title">反代配置 — {{ confDomain }}</h3>
            <n-button size="small" quaternary @click="confModal = false">关闭</n-button>
          </div>
          <pre class="code-block conf-view">{{ confContent || '加载中…' }}</pre>
        </div>
      </n-modal>
    </template>
  </template>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import EmptyState from '../components/EmptyState.vue'
import FonuCard from '../components/FonuCard.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import { api } from '../api/client'
import type { FrpAgentRouteDetail, FrpAgentStatus, FrpProxy, FrpProxyTraffic, FrpServer } from '../api/types'
import { formatBytes, formatRate } from '../utils/format'

const message = useMessage()
const servers = ref<FrpServer[]>([])
const selectedId = ref<number | null>(null)
const loadError = ref('')
const loadErrorMsg = ref('')
const refreshing = ref(false)
const routeCandidates = ref<FrpProxy[]>([])
const routeDetails = ref<FrpAgentRouteDetail[]>([])
const routeHealth = ref<import('../api/types').FrpAgentRouteHealth[]>([])
const proxyTraffic = ref<Record<string, FrpProxyTraffic>>({})
const trafficErrors = ref<Record<string, string>>({})
const agentStatus = ref<FrpAgentStatus | null>(null)
const installingNginx = ref(false)
const routeActionId = ref<number | null>(null)
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
const confLoadingId = ref<number | null>(null)
const confModal = ref(false)
const confDomain = ref('')
const confContent = ref('')
let timer: number | undefined

const server = computed(() => servers.value.find((item) => item.id === selectedId.value) ?? null)
const serverOptions = computed(() => servers.value
  .filter((item) => item.agent_configured || item.ssh_configured)
  .map((item) => ({ label: item.name, value: item.id })))

async function load() {
  loadError.value = ''
  try {
    servers.value = (await api.listFrpServers()).filter((item) => item.agent_configured || item.ssh_configured)
    if (selectedId.value === null || !servers.value.some((item) => item.id === selectedId.value)) {
      selectedId.value = servers.value[0]?.id ?? null
    }
    await refreshAll()
  } catch (error) { loadError.value = error instanceof Error ? error.message : '读取服务端失败' }
}

async function refreshAll() {
  const current = server.value
  if (!current) return
  refreshing.value = true
  try {
    routeCandidates.value = await api.getFrpAgentRouteCandidates(current.id)
    routeDetails.value = await api.getFrpAgentRoutes(current.id)
    routeHealth.value = await api.getFrpAgentRouteHealth(current.id)
    // 流量按规则 ID 聚合（frps dashboard）；旧版 frps 会在后端回退为逐规则查询
    const traffic = await api.listFrpProxiesTraffic()
    proxyTraffic.value = traffic.items ?? {}
    trafficErrors.value = traffic.errors ?? {}
    // agent 状态用于 Nginx 可用性提示（nginx_missing / nginx_ok）
    try { agentStatus.value = await api.getFrpAgentStatus(current.id) } catch { agentStatus.value = null }
  } catch (error) { loadErrorMsg.value = error instanceof Error ? error.message : '读取规则失败' }
  finally { refreshing.value = false }
}

function vhostPortOf(candidate: FrpProxy) {
  const port = server.value?.options.vhost_http_port || 8080
  return candidate.type === 'https' ? (server.value?.options.vhost_https_port || 8443) : port
}

type RouteState = 'loaded' | 'written' | 'broken' | 'none'

function stateOf(candidate: FrpProxy): RouteState {
  for (const domain of candidate.custom_domains) {
    const d = routeDetails.value.find((item) => item.domain === domain)
    if (d) {
      if (d.loaded) return 'loaded'
      if (d.syntax_ok) return 'written'
      return 'broken'
    }
  }
  return 'none'
}

function stateMeta(candidate: FrpProxy): { label: string; type: 'success' | 'warning' | 'error' | 'default' } {
  switch (stateOf(candidate)) {
    case 'loaded': return { label: '已生效', type: 'success' }
    case 'written': return { label: '已写入未加载', type: 'warning' }
    case 'broken': return { label: '语法错误', type: 'error' }
    default: return { label: '未部署', type: 'default' }
  }
}

async function deployRoute(candidate: FrpProxy) {
  const current = server.value
  if (!current) return
  routeActionId.value = candidate.id
  try {
    await api.deployFrpAgentRoute(current.id, candidate.id)
    message.success('公网反代已部署')
    await refreshAll()
  } catch (error) { message.error(error instanceof Error ? error.message : '部署反代失败') }
  finally { routeActionId.value = null }
}

async function removeRoute(candidate: FrpProxy) {
  const current = server.value
  if (!current) return
  routeActionId.value = candidate.id
  try {
    await api.removeFrpAgentRoute(current.id, candidate.id)
    message.success('公网反代已移除')
    await refreshAll()
  } catch (error) { message.error(error instanceof Error ? error.message : '移除反代失败') }
  finally { routeActionId.value = null }
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
  routeActionId.value = target.id
  try {
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
    await api.updateFrpProxy(target.id, { server_id: target.server_id, name: target.name, type: target.type, local_ip: target.local_ip, local_port: target.local_port, remote_port: target.remote_port ?? null, custom_domains: target.custom_domains, host_header_rewrite: target.host_header_rewrite, options, enabled: target.enabled, remark: target.remark })
    settingsModal.value = false
    message.success('部署设置已保存，正在重新部署…')
    await deployRoute(target)
  } catch (error) { message.error(error instanceof Error ? error.message : '保存部署设置失败') }
  finally { routeActionId.value = null }
}

// nginxState 汇总 Nginx 状态徽标：未安装 / 配置校验未通过 / 已安装未运行 / 正常。
function nginxState(): { label: string; type: 'success' | 'warning' | 'error' | 'default' } {
  const status = agentStatus.value
  if (!status) return { label: '未知（未连接 agent）', type: 'default' }
  if (status.nginx_missing) return { label: '未安装', type: 'error' }
  if (!status.nginx_ok) return { label: '配置校验未通过', type: 'error' }
  if (status.nginx_running === false) return { label: '已安装未运行', type: 'warning' }
  return { label: '正常', type: 'success' }
}

// installNginx 触发 VPS 上一键安装 Nginx（agent 侧识别包管理器并执行，需 root）。
async function installNginx() {
  const current = server.value
  if (!current) return
  installingNginx.value = true
  try {
    const result = await api.installFrpAgentNginx(current.id)
    if (result.ok) {
      message.success(`Nginx 安装完成${result.path ? `（${result.path}）` : ''}`)
    } else {
      message.warning(`安装命令已执行，但 Nginx 仍不可用：${(result.error || '').slice(0, 300)}`)
    }
    await refreshAll()
  } catch (error) { message.error(error instanceof Error ? error.message : '一键安装 Nginx 失败') }
  finally { installingNginx.value = false }
}

// trafficOf 返回该规则的 frps 流量快照；无数据时回落到全零，避免模板出现 undefined。
function trafficOf(candidate: FrpProxy): FrpProxyTraffic {
  return proxyTraffic.value[String(candidate.id)] ?? {
    traffic_in: 0, traffic_out: 0, cur_conns: 0, traffic_in_rate: 0, traffic_out_rate: 0,
  }
}

// 四段健康徽标：DNS 指向 / 证书覆盖 / 隧道注册 / 内网服务
function healthBadges(candidate: FrpProxy) {
  return candidate.custom_domains.map((domain) => {
    const h = routeHealth.value.find((item) => item.domain === domain)
    return [
      { label: 'DNS', ok: h?.dns_ok ?? false, detail: h?.dns_detail || (h?.dns_ok ? '已指向服务端' : '待检测') },
      { label: '证书', ok: h?.cert_ok ?? false, detail: h?.cert_detail || (h?.cert_ok ? '已覆盖' : '未发现证书') },
      { label: '隧道', ok: h?.tunnel_ok ?? false, detail: h?.tunnel_detail || (h?.tunnel_ok ? '已注册到 frps' : 'frps 未发现该规则的隧道') },
      { label: '服务', ok: h?.service_ok ?? false, detail: h?.service_ok ? '内网服务可达' : '内网服务不可达' },
    ]
  }).flat()
}

async function viewConf(candidate: FrpProxy) {
  const current = server.value
  if (!current) return
  confLoadingId.value = candidate.id
  try {
    const conf = await api.getFrpAgentRouteConf(current.id, candidate.id)
    confDomain.value = conf.domain
    confContent.value = conf.content
    confModal.value = true
  } catch (error) { message.error(error instanceof Error ? error.message : '读取配置失败') }
  finally { confLoadingId.value = null }
}

onMounted(() => {
  void load()
  timer = window.setInterval(() => { if (server.value && !refreshing.value) void refreshAll() }, 15000)
})

onBeforeUnmount(() => { if (timer) window.clearInterval(timer) })
</script>

<style scoped>
.section-card { margin-bottom: var(--fonu-space-4); }
.nginx-alert { display: flex; align-items: center; justify-content: space-between; gap: var(--fonu-space-4); margin-bottom: var(--fonu-space-4); padding: 12px 16px; border: 1px solid #f0c36d; border-radius: var(--fonu-radius-sm); background: rgba(240, 195, 109, 0.12); }
.nginx-alert--warn { border-color: #e79b9b; background: rgba(231, 155, 155, 0.12); }
.nginx-alert__text { display: grid; gap: 4px; font-size: 13px; color: var(--fonu-text); }
.nginx-alert__text code { padding: 1px 5px; border-radius: 4px; background: var(--fonu-bg-muted); font-family: var(--fonu-mono); font-size: 12px; }
.nginx-status-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px var(--fonu-space-4); }
.nginx-status-item { display: grid; gap: 3px; min-width: 0; }
.nginx-status-item__label { font-size: 12px; color: var(--fonu-text-secondary); }
.nginx-status-item__value { font-size: 13px; color: var(--fonu-text); overflow-wrap: anywhere; }
.nginx-status-error { margin-top: var(--fonu-space-3); color: #dc2626; }
.nginx-status-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: var(--fonu-space-3); }
.section-body { padding: var(--fonu-space-2) var(--fonu-space-5) var(--fonu-space-4); }
.route-list { display: grid; gap: 8px; margin-top: var(--fonu-space-3); }
.route-item { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 12px; border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm); background: var(--fonu-surface); }
.route-item__main { display: grid; gap: 3px; min-width: 0; }
.route-item__domains { color: var(--fonu-text); font-size: 12px; overflow-wrap: anywhere; }
.route-item__upstream { color: var(--fonu-text-muted); font-size: 12px; }
.route-item__actions { display: flex; align-items: center; justify-content: flex-end; gap: 6px; flex-wrap: wrap; flex-shrink: 0; }
.route-item__health { display: flex; gap: 6px; flex-wrap: wrap; margin-top: 2px; }
.health-badge { font-size: 11px; padding: 1px 6px; border-radius: 999px; }
.health-ok { background: rgba(34, 197, 94, 0.12); color: #16a34a; }
.health-bad { background: rgba(239, 68, 68, 0.12); color: #dc2626; }
.route-item__traffic { display: flex; gap: 10px; flex-wrap: wrap; margin-top: 4px; }
.traffic-metric { font-size: 11px; color: var(--fonu-text-secondary); }
.traffic-metric--error { color: #dc2626; }
.empty-route { margin-top: var(--fonu-space-3); padding: 18px; border: 1px dashed var(--fonu-border); border-radius: var(--fonu-radius-sm); color: var(--fonu-text-muted); text-align: center; }
.raw-routes { margin-top: var(--fonu-space-3); font-size: 12px; color: var(--fonu-text-muted); }
.raw-route-row { padding: 2px 0; }
.conf-view { max-height: 60vh; }
.proxy-modal { background: var(--fonu-surface); border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius); padding: var(--fonu-space-4) var(--fonu-space-5); box-shadow: var(--fonu-shadow-md); }
.proxy-modal__header { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--fonu-space-3); }
.proxy-modal__footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: var(--fonu-space-4); }
.proxy-modal__tabbar { display: flex; gap: var(--fonu-space-5); padding: 0 0 var(--fonu-space-3); margin-bottom: var(--fonu-space-2); border-bottom: 1px solid var(--fonu-border); }
.proxy-modal__tab { margin: 0 0 -1px; padding: 8px 2px 10px; border: 0; border-bottom: 2px solid transparent; background: none; color: var(--fonu-text-secondary); font: inherit; font-size: 14px; cursor: pointer; }
.proxy-modal__tab:hover { color: var(--fonu-text); }
.proxy-modal__tab--active { color: var(--fonu-brand-text); font-weight: 600; border-bottom-color: var(--fonu-brand); }
.proxy-modal__scroll { max-height: 60vh; overflow-y: auto; }
.proxy-modal__pane { width: 100%; }
.form-switch-list { display: grid; gap: 2px; margin-bottom: var(--fonu-space-3); }
.form-switch-row { display: flex; align-items: center; justify-content: space-between; gap: var(--fonu-space-4); padding: 10px 12px; border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm); }
.form-switch-row__label { font-size: 13px; color: var(--fonu-text); }
.form-switch-row__hint { margin-top: 2px; font-size: 12px; color: var(--fonu-text-secondary); }
.field-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0 var(--fonu-space-4); }
.field-grid--three { grid-template-columns: 1fr 1fr 1fr; }
.panel-title { margin: var(--fonu-space-2) 0; font-size: 13px; font-weight: 600; color: var(--fonu-text-secondary); }
.modal-title { margin: 0; font-size: 16px; font-weight: 700; color: var(--fonu-text); }
.code-block {
  margin: 0; padding: var(--fonu-space-3) var(--fonu-space-4); background: var(--fonu-bg-muted);
  border: 1px solid var(--fonu-border); border-radius: var(--fonu-radius-sm);
  font-family: var(--fonu-mono); font-size: 12.5px; line-height: 1.65; color: var(--fonu-text);
  overflow: auto; white-space: pre-wrap; word-break: break-all;
}
</style>
