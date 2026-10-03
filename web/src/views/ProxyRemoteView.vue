<template>
  <PageHeader
    title="公网反代"
    description="把穿透规则的域名发布到 VPS 的 Nginx：部署 / 移除、部署设置（HTTPS、跳转、白名单、Basic Auth、限流）、健康检查与配置查看"
  >
    <template #actions>
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
      description="服务端的添加与 Agent / SSH 配置在「公网服务端」页；本页只把启用中的 http 规则发布到 VPS 的 Nginx。"
    >
      <template #action>
        <router-link to="/frp-agent"><n-button type="primary">去公网服务端页</n-button></router-link>
      </template>
    </EmptyState>

    <div v-else-if="server" class="proxy-layout">
      <!-- 左：服务端列表；右：该服务端的漂移提示 / 统计 / Nginx 与反代台账 -->
      <aside class="server-sidebar">
        <div class="server-sidebar__head">
          <div class="server-sidebar__head-row">
            <h3 class="server-sidebar__title">服务端列表</h3>
          </div>
        </div>
        <div class="server-sidebar__list">
          <button
            v-for="item in servers"
            :key="item.id"
            type="button"
            class="server-item"
            :class="{ 'server-item--active': item.id === selectedId }"
            @click="selectedId = item.id"
          >
            <div class="server-item__top">
              <span class="server-item__name">{{ item.name }}</span>
              <n-tag size="tiny" :bordered="false" :type="item.agent_configured ? 'success' : 'warning'">{{ item.agent_configured ? 'Agent 就绪' : 'Agent 未配' }}</n-tag>
            </div>
            <div class="server-item__endpoint mono">{{ item.server_addr }}:{{ item.server_port }}</div>
            <div class="server-item__meta">
              <span>{{ item.ssh_configured ? 'SSH 已配置' : 'SSH 未配置' }}</span>
            </div>
            <div class="server-item__tags">
              <n-tag size="tiny" :bordered="false" :type="item.tls_enabled ? 'success' : 'warning'">TLS</n-tag>
              <n-tag size="tiny" :bordered="false" :type="item.has_token ? 'success' : 'error'">Token</n-tag>
            </div>
          </button>
        </div>
      </aside>

      <div class="proxy-detail">
      <!-- 数据读取失败：必须可见（原先 loadErrorMsg 写了但从未渲染） -->
      <n-alert v-if="loadErrorMsg" type="error" :bordered="false" class="page-alert">{{ loadErrorMsg }}</n-alert>

      <!-- 部署漂移：库里的期望与 VPS 上的实际不一致时才出现（P0-2） -->
      <n-alert v-if="driftItems.length > 0" type="warning" :bordered="false" class="page-alert">
        <div class="alert-body">
          <div class="alert-body__text">
            <strong>检测到 {{ driftItems.length }} 项部署漂移（{{ driftSummary }}）</strong>
            <span>VPS 上的实际配置与 Havline 记录不一致：可能被手工修改过，或 agent 重装后没有重新部署。</span>
            <span class="mono alert-body__detail">{{ driftPreview }}</span>
          </div>
          <n-popconfirm @positive-click="redeployAll">
            <template #trigger>
              <n-button size="small" type="primary" :loading="redeploying" :disabled="busy !== null">
                重新部署全部
              </n-button>
            </template>
            将按 Havline 里的记录重新下发该服务端的全部启用规则，会覆盖 VPS 上的手工修改。确定继续？
          </n-popconfirm>
        </div>
      </n-alert>

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
          Agent 与 frps 的安装、升级、启停、配置下发在「公网服务端」页；本页只负责把规则发布到 Nginx
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
      <HavlineCard class="section-card nginx-card" title="Nginx（公网反代依赖）">
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
      </HavlineCard>

      <!-- ③ 规则台账 -->
      <HavlineCard class="section-card" title="规则台账">
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
      </HavlineCard>
      </div>
    </div>

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

      <!-- 反代配置查看 / 版本对比与回滚 -->
      <n-modal v-model:show="confModal" :mask-closable="true">
        <div class="proxy-modal">
          <div class="proxy-modal__header">
            <h3 class="modal-title">反代配置 — {{ confDomain }}</h3>
            <n-button size="small" quaternary @click="confModal = false">关闭</n-button>
          </div>
          <div class="proxy-modal__scroll conf-scroll">
            <p class="field-hint">
              由 VPS 上的 havline-agent 生成，位于 Nginx 的 vhost 配置目录（文件名
              <span class="mono">havline-{{ confDomain }}.conf</span>）。请勿在 VPS 上手工修改：
              下次部署会被重新生成覆盖，改动请到「部署设置」或穿透规则里调整。
            </p>
            <div class="conf-layout">
              <aside class="conf-versions">
                <div class="conf-versions__head">
                  <span>历史版本</span>
                  <span class="conf-versions__count">{{ routeVersions.length }} / 最近 10 次</span>
                </div>
                <p v-if="versionError" class="conf-versions__empty conf-versions__empty--error">{{ versionError }}</p>
                <p v-else-if="routeVersions.length === 0" class="conf-versions__empty">暂无历史版本（本次部署是第一个版本）</p>
                <ul v-else class="conf-versions__list">
                  <li
                    v-for="version in routeVersions"
                    :key="version.name"
                    class="conf-versions__item"
                    :class="{ 'conf-versions__item--active': selectedVersionName === version.name }"
                    @click="selectVersion(version)"
                  >
                    <span class="mono conf-versions__time">{{ formatDate(version.created_at) }}</span>
                    <span class="conf-versions__size">{{ formatBytes(version.size) }}</span>
                  </li>
                </ul>
              </aside>
              <div class="conf-body">
                <ConfigDiffView
                  v-if="selectedVersionName"
                  :from-text="confContent"
                  :to-text="selectedVersionContent"
                  from-label="当前配置"
                  :to-label="`版本 ${formatDate(versionCreatedAt(selectedVersionName))}`"
                />
                <pre v-else class="code-block conf-view">{{ confContent || '加载中…' }}</pre>
                <div v-if="selectedVersionName" class="conf-actions">
                  <span class="conf-actions__hint">回滚前会先做语法校验与 nginx -t，校验不过不会覆盖当前配置。</span>
                  <n-popconfirm @positive-click="rollbackToVersion">
                    <template #trigger>
                      <n-button size="small" type="warning" :loading="rollingBack" :disabled="versionLoading">
                        回滚到该版本
                      </n-button>
                    </template>
                    确定把 <span class="mono">{{ confDomain }}</span> 的 vhost 回滚到该版本？
                  </n-popconfirm>
                </div>
              </div>
            </div>
          </div>
          <div class="modal-footer">
            <n-button :disabled="!confContent" @click="copyConf">复制</n-button>
            <n-button type="primary" @click="confModal = false">关闭</n-button>
          </div>
        </div>
      </n-modal>
  </template>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { NButton, NIcon, NPopconfirm, useMessage, type DataTableColumns } from 'naive-ui'
import {
  CheckmarkCircleOutline, CopyOutline, InformationCircleOutline, LayersOutline, OpenOutline, PulseOutline, RefreshOutline, SwapHorizontalOutline,
} from '@vicons/ionicons5'
import EmptyState from '../components/EmptyState.vue'
import ConfigDiffView from '../components/ConfigDiffView.vue'
import { useVisibilityPolling } from '../composables/useVisibilityPolling'
import HavlineCard from '../components/HavlineCard.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { api } from '../api/client'
import type {
  CertificateRecord, FrpAgentRouteDetail, FrpAgentRouteHealth, FrpAgentStatus, FrpProxy, FrpProxyTraffic,
  FrpRouteDriftReport, FrpRouteUptime, FrpServer, NginxConfigVersionEntry,
} from '../api/types'
import { formatBytes, formatDate, formatRate, formatRelativeTime } from '../utils/format'
import { copyText } from '../utils/clipboard'

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
const routeDrift = ref<FrpRouteDriftReport | null>(null)
const routeUptime = ref<FrpRouteUptime[]>([])
const certificates = ref<CertificateRecord[]>([])
const redeploying = ref(false)
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
const confProxyId = ref<number | null>(null)
const routeVersions = ref<NginxConfigVersionEntry[]>([])
const selectedVersionName = ref('')
const selectedVersionContent = ref('')
const versionLoading = ref(false)
const versionError = ref('')
const rollingBack = ref(false)

const server = computed(() => servers.value.find((item) => item.id === selectedId.value) ?? null)
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
// 7 个接口彼此独立，并行发：此前串行 await 让每次刷新等于 7 个往返相加
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
  await Promise.all([
    track('规则列表', () => api.getFrpAgentRouteCandidates(current.id), (value) => { candidates.value = value }),
    track('路由文件', () => api.getFrpAgentRoutes(current.id), (value) => { routeDetails.value = value }),
    track('健康检查', () => api.getFrpAgentRouteHealth(current.id), (value) => { routeHealth.value = value }),
    track('配置漂移', () => api.getFrpAgentRouteDrift(current.id), (value) => { routeDrift.value = value }),
    track('可用率', () => api.getFrpRouteUptime(), (value) => { routeUptime.value = value.items ?? [] }),
    track('证书列表', () => api.listCertificates(), (value) => { certificates.value = value }),
    track('流量采集', () => api.listFrpProxiesTraffic(), (value) => {
      proxyTraffic.value = value.items ?? {}
      trafficErrors.value = value.errors ?? {}
    }),
    track('Agent 状态', () => api.getFrpAgentStatus(current.id), (value) => {
      agentStatus.value = value
      statusLoaded.value = true
    }),
  ])
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

// 证书来源（P0-4）：Havline 证书库里是否有该域名的证书（决定重新部署时能否推送证书、公网 HTTPS 能否终结）
function certSummary(domain: string) {
  const record = certificates.value.find((item) => item.domain === domain)
  if (!record) return 'Havline 证书库：无（需先在「证书管理」申请）'
  return `Havline 证书库：有（剩余 ${record.days_left} 天）`
}

// 健康四段（按域名）：DNS / 证书 / 隧道 / 服务。
// unknown 表示这一段判不出来（frps 管理接口未配或不可用），UI 必须与「失败」区分开，
// 否则用户会看到一条红色的「隧道」却怎么查都没问题。
function healthOf(domain: string) {
  const health = routeHealth.value.find((item) => item.domain === domain)
  const tunnelUnknown = health?.tunnel_known === false
  return [
    { label: 'DNS', ok: health?.dns_ok ?? false, unknown: false, detail: health?.dns_detail || (health?.dns_ok ? '已指向服务端' : '未指向服务端或未检测') },
    {
      label: '证书',
      ok: health?.cert_ok ?? false,
      unknown: false,
      detail: `${health?.cert_detail || (health?.cert_ok ? '证书已覆盖该域名' : '未发现该域名的证书')}｜${certSummary(domain)}`,
    },
    {
      label: '隧道',
      ok: health?.tunnel_ok ?? false,
      unknown: tunnelUnknown,
      detail: tunnelUnknown
        ? `无法判定：${health?.tunnel_detail || 'frps 管理接口未配置或不可用'}（不代表隧道不通）`
        : health?.tunnel_detail || (health?.tunnel_ok ? '已注册到 frps' : 'frps 未发现该规则的隧道'),
    },
    { label: '服务', ok: health?.service_ok ?? false, unknown: false, detail: health?.service_ok ? '内网服务可达' : '内网服务不可达' },
  ]
}

// 部署漂移：库里的期望与 VPS 上的实际之间的差异（未部署 / 孤儿配置 / 未生效 / 内容不一致）
const driftKindText: Record<string, string> = {
  missing: '未部署',
  orphan: '孤儿配置',
  not_loaded: '未生效',
  content_mismatch: '内容不一致',
}

const driftItems = computed(() => routeDrift.value?.items ?? [])

const driftSummary = computed(() => {
  const counts = new Map<string, number>()
  for (const item of driftItems.value) {
    counts.set(item.kind, (counts.get(item.kind) ?? 0) + 1)
  }
  return [...counts.entries()].map(([kind, count]) => `${driftKindText[kind] ?? kind} ${count}`).join(' · ')
})

const driftPreview = computed(() => {
  const preview = driftItems.value.slice(0, 3).map((item) => `${item.domain}（${driftKindText[item.kind] ?? item.kind}）`)
  return driftItems.value.length > 3 ? `${preview.join('；')} …` : preview.join('；')
})

// 全量重部署：逐条下发，失败不中断（后端已逐条汇总）
async function redeployAll() {
  const current = server.value
  if (!current) return
  redeploying.value = true
  try {
    const result = await api.redeployFrpAgentRoutes(current.id)
    await refreshAll()
    if (result.failed > 0) {
      const firstError = result.results.find((item) => !item.ok)?.error ?? ''
      message.warning(`重新部署：成功 ${result.succeeded} 条，失败 ${result.failed} 条${firstError ? `；首个失败：${firstError}` : ''}`)
    } else {
      message.success(`重新部署完成：${result.succeeded} 条规则全部成功`)
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '重新部署失败')
  } finally {
    redeploying.value = false
  }
}

// 可用率：由后台巡检落库的状态变更推导；没有记录时返回 undefined（UI 要写「暂无数据」）
function uptimeOf(candidate: FrpProxy) {
  const domain = domainsOf(candidate)[0]
  if (!domain) return undefined
  return routeUptime.value.find((item) => item.domain === domain)
}

function uptimeDetail(stat: FrpRouteUptime) {
  const parts = [
    `可用率 ${stat.uptime_pct.toFixed(1)}%`,
    `不可用 ${stat.down_minutes} 分钟`,
    `无法判定 ${stat.unknown_minutes} 分钟`,
    `观测 ${stat.covered_minutes} 分钟`,
  ]
  if (stat.incidents.length > 0) {
    const list = stat.incidents
      .map((item) => `${formatDate(item.from)}${item.to ? ` → ${formatDate(item.to)}` : ' → 至今'}（${item.reason || '未知环节'}，${item.minutes} 分钟）`)
      .join('；')
    parts.push(`不可用区间：${list}`)
  }
  return parts.join('｜')
}

function isHealthy(candidate: FrpProxy) {
  const domains = domainsOf(candidate)
  if (!domains.length) return false
  // 判不出来的那一段不能算作「不健康」：否则 dashboard 一挂，整页规则都会变红
  return domains.every((domain) => healthOf(domain).every((item) => item.ok || item.unknown))
}

// 域名快捷操作：复制 / 新窗口打开（http 规则走 http，https 规则走 https）
async function copyDomain(domain: string) {
  if (await copyText(domain)) message.success(`已复制 ${domain}`)
  else message.error('复制失败，请手动选择文本复制')
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
        [h(StatusBadge, { kind: item.unknown ? 'warning' : item.ok ? 'success' : 'error', text: item.label })])))),
  },
  {
    title: '可用率 24h', key: 'uptime', width: 120, align: 'right',
    render: (row) => {
      const stat = uptimeOf(row)
      // 没有巡检记录时明说「暂无数据」：直接显示 100% 会让人以为一直没问题
      if (!stat) return h('span', { class: 'mono cell-muted' }, '暂无数据')
      const kind = stat.state === 'down' ? 'error' : stat.state === 'up' ? 'success' : 'warning'
      return h('span', { class: 'mono', title: uptimeDetail(stat) },
        [h(StatusBadge, { kind, text: `${stat.uptime_pct.toFixed(1)}%` })])
    },
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
  if (result === undefined) return
  // 自动生成的 Basic Auth 密码只在这一次响应里返回：不再写进 nginx 配置，错过就得在「部署设置」里重设
  if (result.basic_auth_password) {
    message.warning(`Basic Auth 已生成（仅显示这一次，请立即保存）：${result.basic_auth_user} / ${result.basic_auth_password}`, { duration: 15000 })
  }
  await refreshAll()
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
  confProxyId.value = candidate.id
  try {
    const conf = await api.getFrpAgentRouteConf(current.id, candidate.id)
    confDomain.value = conf.domain
    confContent.value = conf.content
    confModal.value = true
    await loadRouteVersions(candidate.id)
  } catch (error) {
    loadErrorMsg.value = `读取配置失败 —— ${error instanceof Error ? error.message : '未知错误'}`
  } finally { busy.value = null }
}

// 版本历史读不到不影响「查看配置」本身：只在弹窗内提示（老版本 agent 没有这个端点）
async function loadRouteVersions(proxyId: number) {
  const current = server.value
  if (!current) return
  routeVersions.value = []
  selectedVersionName.value = ''
  selectedVersionContent.value = ''
  versionError.value = ''
  try {
    const result = await api.getFrpAgentRouteVersions(current.id, proxyId)
    routeVersions.value = result.versions ?? []
  } catch (error) {
    versionError.value = error instanceof Error ? error.message : '读取版本历史失败'
  }
}

// 点已选中的版本 = 取消对比，回到当前配置
async function selectVersion(version: NginxConfigVersionEntry) {
  const current = server.value
  if (!current || confProxyId.value === null || versionLoading.value) return
  if (selectedVersionName.value === version.name) {
    selectedVersionName.value = ''
    selectedVersionContent.value = ''
    return
  }
  versionLoading.value = true
  try {
    const result = await api.getFrpAgentRouteVersion(current.id, confProxyId.value, version.name)
    selectedVersionName.value = version.name
    selectedVersionContent.value = result.content
  } catch (error) {
    message.error(`读取该版本失败：${error instanceof Error ? error.message : '未知错误'}`)
  } finally { versionLoading.value = false }
}

async function rollbackToVersion() {
  const current = server.value
  if (!current || confProxyId.value === null || !selectedVersionName.value) return
  rollingBack.value = true
  try {
    await api.rollbackFrpAgentRouteVersion(current.id, confProxyId.value, selectedVersionName.value)
    message.success('已回滚到所选版本，Nginx 已重载')
    const conf = await api.getFrpAgentRouteConf(current.id, confProxyId.value)
    confContent.value = conf.content
    // 回滚会新增一个版本，且可能改变漂移状态，两边都要重读
    await loadRouteVersions(confProxyId.value)
    await refreshAll()
  } catch (error) {
    message.error(`回滚失败：${error instanceof Error ? error.message : '未知错误'}`)
  } finally { rollingBack.value = false }
}

function versionCreatedAt(name: string): string {
  return routeVersions.value.find((item) => item.name === name)?.created_at ?? ''
}

async function copyConf() {
  if (await copyText(confContent.value)) message.success('已复制反代配置')
  else message.error('复制失败，请手动选择文本复制')
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

// 轮询走 useVisibilityPolling：标签页隐藏时暂停，回到前台补一次
useVisibilityPolling(() => {
  if (!server.value) return // 未选服务端时不轮询
  if (busy.value) return    // 有操作进行中不叠加轮询
  void refreshAll()
}, 15000)

onMounted(() => { void load() })
</script>

<style scoped>
/* 分区：提示 → 入口行 → ① 统计区 → ② 环境与 Nginx → ③ 规则台账 */
.page-alert { margin-bottom: var(--havline-space-3); }
.alert-body { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-4); }
.alert-body__text { display: grid; gap: 4px; font-size: 13px; }
.alert-body__detail { font-size: 12px; color: var(--havline-error); overflow-wrap: anywhere; }
.entrance-row { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-4); margin-bottom: var(--havline-space-5); padding: 10px var(--havline-space-5); border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); background: var(--havline-info-soft); }
.entrance-row__text { font-size: 13px; color: var(--havline-text); }

.stats-row { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--havline-space-4); margin-bottom: var(--havline-space-5); }
.stat-card { background: var(--havline-surface); border: 1px solid var(--havline-border); border-radius: var(--havline-radius); box-shadow: var(--havline-shadow); padding: var(--havline-space-4) var(--havline-space-5); min-height: 118px; }
.stat-card__icon { font-size: 18px; margin-bottom: 4px; }
.stat-card__icon--blue { color: var(--havline-info); }
.stat-card__icon--green { color: var(--havline-brand); }
.stat-card__icon--amber { color: var(--havline-warning); }
.stat-card__value { margin-top: 4px; font-size: 30px; font-weight: 700; line-height: 1.1; color: var(--havline-text); letter-spacing: -0.03em; }
.stat-card__value-total { margin-left: 2px; font-size: 15px; font-weight: 600; color: var(--havline-text-muted); }
.stat-card__label { margin-top: 8px; font-size: 13px; font-weight: 600; color: var(--havline-text); }
.stat-card__sub { margin-top: 4px; font-size: 12px; color: var(--havline-text-muted); }

/* HavlineCard 自身已有 24px 内边距，这里不再叠加（与 FrpView 的用法一致） */
.section-card { margin-bottom: var(--havline-space-5); }
.section-body { padding: 0; }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--havline-space-3) var(--havline-space-4); }
/* Nginx 卡的 label/value 双列：只在本卡生效，避免影响弹窗里 n-form-item 的内部布局 */
.nginx-card .field-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.nginx-card .field-grid > div { display: grid; gap: 4px; min-width: 0; }
.nginx-card .field-grid label { color: var(--havline-text-secondary); font-size: 12px; }
.field-tags { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.field-grid--three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.full-width { width: 100%; }
.agent-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; margin-top: var(--havline-space-3); }
.nginx-error { margin-top: var(--havline-space-2); color: var(--havline-error); overflow-wrap: anywhere; }

/* 规则台账：与本地反向代理页一致，用 n-data-table 呈现（列宽固定，各行列天然对齐） */
/* 单元格元素由 render 函数的 h() 创建，不带 scoped 的 data-v 属性，必须用 :deep() 才能命中 */
.route-table-wrap { border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); overflow: hidden; }
.route-table-wrap :deep(.cell-name) { display: grid; gap: 4px; }
.route-table-wrap :deep(.cell-name__head) { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.route-table-wrap :deep(.cell-name strong) { font-size: 13.5px; color: var(--havline-text); }
.route-table-wrap :deep(.cell-muted) { font-size: 12px; color: var(--havline-text-muted); }
.route-table-wrap :deep(.cell-wrap) { overflow-wrap: anywhere; }
.route-table-wrap :deep(.cell-health) { display: flex; gap: var(--havline-space-2); flex-wrap: wrap; }
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
.route-table-wrap :deep(.mono) { font-family: var(--havline-mono); font-size: 13px; }

.raw-routes { margin-top: var(--havline-space-4); font-size: 12px; color: var(--havline-text-secondary); }
.raw-routes summary { cursor: pointer; color: var(--havline-text-secondary); }
.raw-routes summary:hover { color: var(--havline-text); }
.raw-routes__list { display: grid; gap: 6px; margin-top: var(--havline-space-3); }
.raw-route-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.raw-route-row .mono { color: var(--havline-text); }

/* 弹窗样式对齐本地反向代理页（ProxyView）的规则弹窗：900px 宽、无外层内边距、头部/tabbar/页脚各自带内边距 */
.proxy-modal { display: flex; flex-direction: column; width: 900px; max-width: 95vw; max-height: 90vh; background: var(--havline-surface); border-radius: var(--havline-radius); overflow: hidden; box-shadow: var(--havline-shadow-md); }
/* 带右侧说明栏的弹窗：整体改为横向，左表单列 + 右说明栏 */
.proxy-modal--with-help { flex-direction: row; }
.proxy-modal__form { flex: 1; min-width: 0; display: flex; flex-direction: column; max-height: 90vh; }
.proxy-modal__help { width: 300px; flex-shrink: 0; padding: var(--havline-space-5); background: var(--havline-bg-muted); border-left: 1px solid var(--havline-border); font-size: 13px; color: var(--havline-text-secondary); overflow: auto; }
.proxy-modal__help h4 { margin: 0 0 var(--havline-space-3); font-size: 14px; font-weight: 600; color: var(--havline-text); }
.proxy-modal__help ol { margin: 0; padding-left: 18px; line-height: 1.75; }
.proxy-modal__help li + li { margin-top: 10px; }
.proxy-modal__help code { font-size: 12px; background: var(--havline-surface); padding: 1px 4px; border-radius: 4px; }
.proxy-modal__tip { display: flex; align-items: flex-start; gap: 8px; margin-top: var(--havline-space-5); padding: 12px; border-radius: 8px; background: var(--havline-info-soft); color: var(--havline-info); font-size: 12px; line-height: 1.6; }
.proxy-modal__tip-icon { flex-shrink: 0; margin-top: 1px; font-size: 16px; }
.proxy-modal__header { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); padding: var(--havline-space-5) var(--havline-space-5) 0; flex-shrink: 0; }
.proxy-modal__tabbar { display: flex; gap: var(--havline-space-5); padding: var(--havline-space-3) var(--havline-space-5) 0; border-bottom: 1px solid var(--havline-border); flex-shrink: 0; }
.proxy-modal__tab { margin: 0; padding: 8px 2px 10px; border: none; background: none; font: inherit; font-size: 14px; color: var(--havline-text-secondary); cursor: pointer; border-bottom: 2px solid transparent; margin-bottom: -1px; }
.proxy-modal__tab:hover { color: var(--havline-text); }
.proxy-modal__tab--active { color: var(--havline-brand-text); font-weight: 600; border-bottom-color: var(--havline-brand); }
.proxy-modal__scroll { flex: 1; min-height: 0; overflow-y: auto; }
.proxy-modal__pane { width: 100%; box-sizing: border-box; padding: var(--havline-space-4) var(--havline-space-5) 0; }
.proxy-modal__pane :deep(.n-form) { width: 100%; }
.proxy-modal__pane :deep(.n-form-item .n-form-item-blank) { display: block; width: 100%; }
.modal-footer { display: flex; justify-content: flex-end; gap: var(--havline-space-3); padding: var(--havline-space-4) var(--havline-space-5) var(--havline-space-5); border-top: 1px solid var(--havline-border); margin-top: var(--havline-space-2); flex-shrink: 0; }
.form-switch-list { display: flex; flex-direction: column; gap: 14px; margin-bottom: var(--havline-space-4); }
.form-switch-row { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-4); padding: 12px 14px; border: 1px solid var(--havline-border); border-radius: 10px; background: var(--havline-bg); }
.form-switch-row__label { font-size: 14px; font-weight: 600; color: var(--havline-text); }
.form-switch-row__hint { margin-top: 4px; font-size: 12px; color: var(--havline-text-muted); line-height: 1.5; }
.field-hint { margin: 0; font-size: 12px; color: var(--havline-text-muted); line-height: 1.5; }
.panel-title { margin: var(--havline-space-4) 0 var(--havline-space-3); font-size: 14px; font-weight: 600; color: var(--havline-text); }
.modal-title { margin: 0; font-size: 18px; font-weight: 600; color: var(--havline-text); }
.code-block {
  margin: 0; padding: var(--havline-space-3) var(--havline-space-4); background: var(--havline-bg-muted);
  border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm);
  font-family: var(--havline-mono); font-size: 12.5px; line-height: 1.65; color: var(--havline-text);
  overflow: auto; white-space: pre-wrap; word-break: break-all;
}
/* 反代配置查看弹窗：内容区与头部同样的 24px 内边距，代码块自身滚动 */
.conf-scroll { padding: var(--havline-space-4) var(--havline-space-5) 0; }
.conf-scroll .field-hint { margin-bottom: var(--havline-space-3); line-height: 1.65; }
.conf-view { max-height: 62vh; }
/* 版本面板 + 差异区：左侧固定宽度列表，右侧自适应（与弹窗 900px 宽度对齐） */
.conf-layout { display: grid; grid-template-columns: 208px minmax(0, 1fr); gap: var(--havline-space-4); }
.conf-versions { border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); overflow: hidden; }
.conf-versions__head {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  padding: 6px var(--havline-space-3); border-bottom: 1px solid var(--havline-border);
  background: var(--havline-bg-muted); font-size: 12px; color: var(--havline-text-secondary);
}
.conf-versions__count { color: var(--havline-text-muted); }
.conf-versions__empty { padding: var(--havline-space-3); font-size: 12px; color: var(--havline-text-muted); line-height: 1.6; }
.conf-versions__empty--error { color: var(--havline-error); }
.conf-versions__list { max-height: 52vh; overflow: auto; margin: 0; padding: 0; list-style: none; }
.conf-versions__item {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  padding: 6px var(--havline-space-3); border-bottom: 1px solid var(--havline-border);
  font-size: 12px; color: var(--havline-text-secondary); cursor: pointer;
}
.conf-versions__item:hover { background: var(--havline-bg-muted); }
.conf-versions__item--active { background: var(--havline-info-soft); color: var(--havline-text); }
.conf-versions__time { font-size: 12px; }
.conf-versions__size { flex: none; color: var(--havline-text-muted); }
.conf-body { min-width: 0; display: grid; gap: var(--havline-space-3); align-content: start; }
.conf-actions { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); }
.conf-actions__hint { font-size: 12px; color: var(--havline-text-muted); line-height: 1.6; }
.form-hint { color: var(--havline-text-secondary); font-size: 12.5px; }
.mono { font-family: var(--havline-mono); font-size: 13px; }
.mono-wrap { white-space: normal; word-break: break-all; }

/* 左：服务端列表 / 右：该服务端的反代台账（与「内网穿透」「公网服务端」页同构） */
.proxy-layout { display: grid; grid-template-columns: minmax(240px, 300px) minmax(0, 1fr); gap: var(--havline-space-4); align-items: start; }
.proxy-layout .section-card { min-width: 0; }
.proxy-detail { min-width: 0; }
.server-sidebar { background: var(--havline-surface); border: 1px solid var(--havline-border); border-radius: var(--havline-radius); box-shadow: var(--havline-shadow); overflow: hidden; }
.server-sidebar__head { padding: var(--havline-space-4); border-bottom: 1px solid var(--havline-border); }
.server-sidebar__head-row { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); }
.server-sidebar__title { margin: 0; font-size: 15px; font-weight: 600; color: var(--havline-text); flex-shrink: 0; }
.server-sidebar__list { display: flex; flex-direction: column; gap: 10px; padding: var(--havline-space-3); max-height: 720px; overflow: auto; }
.server-item { width: 100%; font: inherit; text-align: left; padding: 12px 14px; border: 1px solid var(--havline-border); border-radius: var(--havline-radius); background: var(--havline-surface); cursor: pointer; transition: border-color 0.15s ease, box-shadow 0.15s ease; }
.server-item:hover { border-color: var(--havline-brand-hover); }
.server-item--active { border-color: var(--havline-brand); box-shadow: 0 0 0 1px var(--havline-brand-soft); }
.server-item__top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.server-item__name { font-size: 14px; font-weight: 600; color: var(--havline-text); overflow-wrap: anywhere; }
.server-item__endpoint { margin-top: 6px; font-size: 12px; color: var(--havline-text-secondary); overflow-wrap: anywhere; }
.server-item__meta { display: flex; align-items: center; flex-wrap: wrap; gap: var(--havline-space-3); margin-top: 6px; font-size: 12px; color: var(--havline-text-muted); }
.server-item__tags { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
@media (max-width: 1100px) { .proxy-layout { grid-template-columns: 1fr; } .server-sidebar__list { max-height: none; } }

/* 首屏一次性入场；用户偏好减少动效时关闭 */
@keyframes havline-fade-up {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: none; }
}
.stats-row, .entrance-row, .section-card { animation: havline-fade-up 160ms ease-out both; }
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
