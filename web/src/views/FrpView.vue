<template>
  <PageHeader title="FRP 穿透" description="管理 NAS 到公网 VPS 的 frpc 客户端连接">
    <template #actions>
      <n-button @click="showExport = true; loadExport()">查看脱敏配置</n-button>
      <n-button @click="openRuntimeModal">
        <template #icon><n-icon :component="SettingsOutline" /></template>
        应用配置
      </n-button>
      <n-button @click="showGuide = true">
        <template #icon><n-icon :component="DocumentTextOutline" /></template>
        使用说明
      </n-button>
      <n-button v-if="status.status !== 'running'" type="primary" :loading="actionLoading === 'start'" @click="startClient">
        <template #icon><n-icon :component="PlayOutline" /></template>
        启动 frpc
      </n-button>
      <n-button v-else type="error" ghost :loading="actionLoading === 'stop'" @click="stopClient">
        <template #icon><n-icon :component="StopOutline" /></template>
        停止 frpc
      </n-button>
    </template>
  </PageHeader>

  <LoadError v-if="loadError" :message="loadError" @retry="load" />

  <template v-else>
    <section class="stats-row" aria-label="frpc 状态">
      <div class="stat-card">
        <n-icon :component="GitNetworkOutline" class="stat-card__icon" :class="`stat-card__icon--${status.status}`" aria-hidden="true" />
        <div class="stat-card__value stat-card__value--sm"><StatusBadge :kind="statusKind" :text="statusLabel" /></div>
        <div class="stat-card__label">客户端状态</div>
        <div class="stat-card__sub">frpc 进程状态</div>
      </div>
      <div class="stat-card">
        <n-icon :component="LayersOutline" class="stat-card__icon stat-card__icon--blue" aria-hidden="true" />
        <div class="stat-card__value">{{ status.enabled_proxies }}</div>
        <div class="stat-card__label">启用规则</div>
        <div class="stat-card__sub">已启用的穿透规则数</div>
      </div>
      <div class="stat-card">
        <n-icon :component="TerminalOutline" class="stat-card__icon stat-card__icon--blue" aria-hidden="true" />
        <div class="stat-card__value stat-card__value--sm">{{ status.frpc_version || '未检测' }}</div>
        <div class="stat-card__label">frpc 版本</div>
        <div class="stat-card__sub">本机运行的 frpc</div>
      </div>
      <div class="stat-card">
        <n-icon :component="TimeOutline" class="stat-card__icon stat-card__icon--amber" aria-hidden="true" />
        <div class="stat-card__value stat-card__value--sm">{{ formatDate(status.last_reloaded_at) || '尚未应用' }}</div>
        <div class="stat-card__label">最近应用</div>
        <div class="stat-card__sub">配置最近一次生效</div>
      </div>
    </section>

    <EmptyState
      v-if="servers.length === 0"
      title="尚未配置公网服务端"
      description="穿透规则要归属到一台公网服务端；服务端的增删改、启停与 Agent 安装升级都在「公网服务端」页。"
    >
      <template #action><router-link to="/frp-agent"><n-button type="primary">去公网服务端页</n-button></router-link></template>
    </EmptyState>
    <EmptyState v-else-if="proxies.length === 0" title="尚未创建穿透规则" description="可发布 TCP 服务，或通过 HTTP/HTTPS 域名将请求转入内网。">
      <template #action><n-button type="primary" @click="openProxyModal()">添加规则</n-button></template>
    </EmptyState>

    <!-- 左：服务端列表（选服务端）；右：该服务端的穿透规则与选中规则详情 -->
    <div v-else class="frp-layout">
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
            :class="{ 'server-item--active': item.id === selectedServerId }"
            @click="selectServer(item.id)"
          >
            <div class="server-item__top">
              <span class="server-item__name">{{ item.name }}</span>
              <n-tag size="tiny" :bordered="false" :type="serverBootType(item)">{{ serverBootLabel(item) }}</n-tag>
            </div>
            <div class="server-item__endpoint mono">{{ item.server_addr }}:{{ item.server_port }}</div>
            <div class="server-item__meta">
              <span>{{ serverRuleCount(item.id) }} 条规则</span>
              <span>{{ serverEnabledRuleCount(item.id) }} 条启用</span>
            </div>
            <div class="server-item__tags">
              <n-tag size="tiny" :bordered="false" :type="item.tls_enabled ? 'success' : 'warning'">TLS</n-tag>
              <n-tag size="tiny" :bordered="false" :type="item.has_token ? 'success' : 'error'">Token</n-tag>
              <n-tag v-if="item.agent_configured" size="tiny" :bordered="false" type="info">Agent</n-tag>
              <n-tag v-if="item.ssh_configured" size="tiny" :bordered="false" type="info">SSH</n-tag>
            </div>
          </button>
        </div>
      </aside>

      <div class="rule-pane">
        <div class="rule-pane__head">
          <div class="rule-pane__intro">
            <h3 class="rule-pane__title">穿透规则</h3>
            <span class="rule-pane__sub">{{ selectedServer?.name || '未选择服务端' }} · {{ serverProxies.length }} 条</span>
          </div>
          <div class="rule-pane__actions">
            <n-button size="small" @click="showExport = true; loadExport()">查看脱敏配置</n-button>
            <router-link v-if="selectedServerId" :to="{ name: 'frp-agent', query: { server: String(selectedServerId) } }">
              <n-button size="small" quaternary>管理该服务端</n-button>
            </router-link>
            <n-button size="small" type="primary" :disabled="servers.length === 0" @click="openProxyModal()">
              <template #icon><n-icon :component="AddOutline" /></template>
              添加规则
            </n-button>
          </div>
        </div>

        <EmptyState v-if="serverProxies.length === 0" title="该服务端还没有穿透规则" description="可发布 TCP 服务，或通过 HTTP/HTTPS 域名将请求转入内网。">
          <template #action><n-button type="primary" :disabled="servers.length === 0" @click="openProxyModal()">添加规则</n-button></template>
        </EmptyState>
        <div class="rule-table-wrap">
          <n-data-table
            class="frp-rules-table"
            :columns="columns"
            :data="serverProxies"
            :bordered="false"
            :single-line="false"
            :scroll-x="1890"
            :row-key="(row: FrpProxy) => row.id"
          />
        </div>
      </div>
    </div>

    <HavlineCard v-if="status.last_error" class="error-card" title="最近错误">
      <div class="error-card__content">{{ status.last_error }}</div>
    </HavlineCard>
  </template>

  <n-modal
    v-model:show="showProxyModal"
    :mask-closable="false"
    transform-origin="center"
  >
    <div class="proxy-modal frp-proxy-modal">
      <div class="proxy-modal__form">
        <div class="proxy-modal__header">
          <h3 class="modal-title">{{ editingProxy ? '编辑穿透规则' : '添加穿透规则' }}</h3>
          <n-button size="small" quaternary @click="showProxyModal = false">
            <template #icon><n-icon :component="CloseOutline" /></template>
          </n-button>
        </div>
        <div class="proxy-modal__tabbar">
          <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': proxyFormTab === 'basic' }" @click="proxyFormTab = 'basic'">基础配置</button>
          <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': proxyFormTab === 'advanced' }" @click="proxyFormTab = 'advanced'">高级配置</button>
        </div>
        <div class="proxy-modal__scroll">
          <n-form label-placement="top" class="proxy-modal__pane" @submit.prevent="saveProxy">
            <div v-show="proxyFormTab === 'basic'" class="frp-proxy-modal__basic">
      <div class="form-grid">
        <n-form-item label="公网服务端" required><n-select v-model:value="proxyForm.server_id" :options="proxyServerOptions" placeholder="选择 FRP 服务端" /></n-form-item>
        <n-form-item label="规则名称" required><n-input v-model:value="proxyForm.name" placeholder="例如：nas-web" /></n-form-item>
        <n-form-item label="类型" required><n-select v-model:value="proxyForm.type" :options="proxyTypeOptions" /></n-form-item>
        <n-form-item v-if="!proxyForm.plugin_type" label="内网地址" required><n-input v-model:value="proxyForm.local_ip" placeholder="127.0.0.1" /></n-form-item>
        <n-form-item v-if="!proxyForm.plugin_type" label="内网端口" required><n-input-number v-model:value="proxyForm.local_port" :min="1" :max="65535" /></n-form-item>
        <n-form-item v-if="['tcp', 'udp'].includes(proxyForm.type)" label="公网远程端口" required><n-input-number v-model:value="proxyForm.remote_port" :min="1" :max="65535" /></n-form-item>
        <n-form-item v-if="['http', 'https', 'tcpmux'].includes(proxyForm.type)" label="自定义域名"><n-input v-model:value="proxyForm.domains_text" type="textarea" :rows="2" placeholder="nas.example.com&#10;app.example.com" /></n-form-item>
      </div>
      <section class="proxy-form-section" v-if="['http', 'https'].includes(proxyForm.type)">
        <h4>HTTP / HTTPS 配置</h4>
        <div class="form-grid">
          <n-form-item label="Subdomain"><n-input v-model:value="proxyForm.options.subdomain" placeholder="例如：nas" /></n-form-item>
          <n-form-item label="公网反代选项" :show-feedback="true">
            <span class="form-hint">WebSocket / 强制跳转 / IP 白名单 / Basic Auth 已移至「公网反代」页逐条规则的「部署设置」中配置；此处填写的值会保留。已填的 IP 白名单：{{ (proxyForm.options.allow_ips || []).join(', ') || '无' }}</span>
          </n-form-item>
          <n-form-item label="上游 Host 重写"><n-input v-model:value="proxyForm.host_header_rewrite" placeholder="例如：nas.example.com" /></n-form-item>
          <n-form-item label="路径匹配（Locations）"><n-input v-model:value="proxyForm.locations_text" placeholder="多个路径用逗号或换行分隔" /></n-form-item>
          <n-form-item label="按 HTTP 用户路由"><n-input v-model:value="proxyForm.options.route_by_http_user" placeholder="可选" /></n-form-item>
          <n-form-item label="HTTP 基础认证用户名"><n-input v-model:value="proxyForm.options.http_user" placeholder="可选" /></n-form-item>
          <n-form-item label="HTTP 基础认证密码"><n-input v-model:value="proxyForm.options.http_password" type="password" show-password-on="click" placeholder="可选" /></n-form-item>
        </div>
      </section>
      <section class="proxy-form-section" v-if="proxyForm.type === 'tcpmux'">
        <h4>TCPMUX 配置</h4>
        <div class="form-grid"><n-form-item label="多路复用器" required><n-select v-model:value="proxyForm.options.multiplexer" :options="[{ label: 'HTTP CONNECT', value: 'httpconnect' }]" /></n-form-item><n-form-item label="按 HTTP 用户路由"><n-input v-model:value="proxyForm.options.route_by_http_user" placeholder="可选" /></n-form-item></div>
      </section>
      <section class="proxy-form-section" v-if="['stcp', 'sudp', 'xtcp'].includes(proxyForm.type)">
        <h4>访问控制</h4>
        <div class="form-grid"><n-form-item label="访问密钥" required><n-input v-model:value="proxyForm.options.secret_key" type="password" show-password-on="click" placeholder="与访客端保持一致" /></n-form-item><n-form-item label="允许的用户"><n-input v-model:value="proxyForm.allow_users_text" placeholder="多个用户用逗号分隔，* 表示全部" /></n-form-item></div>
      </section>
      <n-form-item label="备注"><n-input v-model:value="proxyForm.remark" placeholder="可选" /></n-form-item>
      <div class="switch-row"><span>启用规则</span><n-switch v-model:value="proxyForm.enabled" /></div>
            </div>
            <div v-show="proxyFormTab === 'advanced'" class="frp-proxy-modal__advanced">
      <n-collapse class="advanced-collapse proxy-advanced-collapse">
        <n-collapse-item title="传输与健康检查" name="proxy-transport">
          <div class="form-grid">
            <n-form-item label="带宽限制"><n-input v-model:value="proxyForm.options.transport.bandwidth_limit" placeholder="例如：1MB" /></n-form-item>
            <n-form-item label="限速位置"><n-select v-model:value="proxyForm.options.transport.bandwidth_limit_mode" :options="[{ label: '客户端', value: 'client' }, { label: '服务端', value: 'server' }]" /></n-form-item>
            <n-form-item label="代理协议版本"><n-select v-model:value="proxyForm.options.transport.proxy_protocol_version" clearable :options="[{ label: 'v1', value: 'v1' }, { label: 'v2', value: 'v2' }]" /></n-form-item>
            <n-form-item label="健康检查类型"><n-select v-model:value="proxyForm.options.health_check.type" clearable :options="[{ label: 'TCP', value: 'tcp' }, { label: 'HTTP', value: 'http' }]" /></n-form-item>
            <n-form-item v-if="proxyForm.options.health_check.type === 'http'" label="健康检查路径"><n-input v-model:value="proxyForm.options.health_check.path" placeholder="/health" /></n-form-item>
            <n-form-item v-if="proxyForm.options.health_check.type" label="检查间隔（秒）"><n-input-number v-model:value="proxyForm.options.health_check.interval_seconds" :min="1" :max="3600" /></n-form-item>
            <n-form-item v-if="proxyForm.options.health_check.type" label="连续失败次数"><n-input-number v-model:value="proxyForm.options.health_check.max_failed" :min="1" :max="100" /></n-form-item>
            <n-form-item v-if="proxyForm.options.health_check.type" label="超时（秒）"><n-input-number v-model:value="proxyForm.options.health_check.timeout_seconds" :min="1" :max="300" /></n-form-item>
          </div>
          <div class="switch-row"><span>启用流量加密</span><n-switch v-model:value="proxyForm.options.transport.use_encryption" /><span>启用压缩</span><n-switch v-model:value="proxyForm.options.transport.use_compression" /></div>
        </n-collapse-item>
        <n-collapse-item title="负载均衡与请求头" name="proxy-advanced">
          <div class="form-grid"><n-form-item label="负载均衡组"><n-input v-model:value="proxyForm.options.load_balancer.group" placeholder="同组规则共享流量" /></n-form-item><n-form-item label="负载均衡组密钥"><n-input v-model:value="proxyForm.options.load_balancer.group_key" type="password" show-password-on="click" /></n-form-item><n-form-item label="元数据"><n-input v-model:value="proxyForm.metadatas_text" type="textarea" :rows="2" placeholder="每行 key=value" /></n-form-item><n-form-item label="请求头（set）"><n-input v-model:value="proxyForm.request_headers_text" type="textarea" :rows="2" placeholder="每行 key=value" /></n-form-item><n-form-item label="响应头（set）"><n-input v-model:value="proxyForm.response_headers_text" type="textarea" :rows="2" placeholder="每行 key=value" /></n-form-item></div>
        </n-collapse-item>
        <n-collapse-item title="插件与 NAT 穿透" name="proxy-plugin">
          <div class="form-grid"><n-form-item label="插件类型"><n-select v-model:value="proxyForm.plugin_type" clearable :options="pluginTypeOptions" /></n-form-item><n-form-item v-if="proxyForm.plugin_type" label="插件本地地址"><n-input v-model:value="proxyForm.plugin_local_addr" placeholder="127.0.0.1:80" /></n-form-item><n-form-item v-if="proxyForm.plugin_type === 'unix_domain_socket'" label="Unix Socket 路径"><n-input v-model:value="proxyForm.plugin_unix_path" /></n-form-item><n-form-item v-if="proxyForm.plugin_type === 'static_file'" label="静态文件目录"><n-input v-model:value="proxyForm.plugin_local_path" /></n-form-item><n-form-item v-if="proxyForm.plugin_type" label="插件用户名"><n-input v-model:value="proxyForm.plugin_username" /></n-form-item><n-form-item v-if="proxyForm.plugin_type" label="插件密码"><n-input v-model:value="proxyForm.plugin_password" type="password" show-password-on="click" /></n-form-item></div>
          <div class="switch-row"><span>禁用 NAT 辅助地址</span><n-switch v-model:value="proxyForm.options.nat_traversal.disable_assisted_addrs" /></div>
        </n-collapse-item>
      </n-collapse>
            </div>
            <div class="modal-actions"><n-button @click="showProxyModal = false">取消</n-button><n-button type="primary" :loading="savingProxy" @click="saveProxy">保存</n-button></div>
          </n-form>
        </div>
      </div>
      <div class="proxy-modal__help">
        <h4>{{ proxyFormTab === 'basic' ? '配置说明' : '高级配置说明' }}</h4>
        <ol>
          <li v-for="item in proxyHelpItems" :key="item.title"><strong>{{ item.title }}</strong>：{{ item.text }}</li>
        </ol>
        <div class="proxy-modal__tip">
          <n-icon :component="InformationCircleOutline" class="proxy-modal__tip-icon" />
          <span>{{ proxyHelpTip }}</span>
        </div>
        <section v-if="proxyFormTab === 'basic'" class="proxy-modal__example">
          <div class="proxy-modal__example-header">
            <div>
              <h5>{{ proxyQuickExample.title }}示例</h5>
              <p>{{ proxyQuickExample.description }}</p>
            </div>
            <n-button size="small" type="primary" @click="applyProxyQuickExample">
              <template #icon><n-icon :component="FlashOutline" /></template>
              快速配置
            </n-button>
          </div>
          <pre>{{ proxyQuickExample.lines.join('\n') }}</pre>
        </section>
      </div>
    </div>
  </n-modal>

  <n-modal v-model:show="showProxyDetail" :mask-closable="true" transform-origin="center">
    <div v-if="detailProxy" class="proxy-detail-modal frp-rule-detail-modal">
      <div class="proxy-detail-modal__header">
        <div class="proxy-detail-modal__intro">
          <h3 class="proxy-detail-modal__title">{{ detailProxy.name }}</h3>
          <n-tag size="small" :type="detailProxy.enabled ? 'success' : 'default'" :bordered="false" round>{{ detailProxy.enabled ? '运行中' : '已停止' }}</n-tag>
          <span class="proxy-detail-modal__conn" :class="{ 'is-active': (proxyTraffic[detailProxy.id]?.cur_conns ?? 0) > 0 }">
            <n-icon :component="PeopleOutline" />
            {{ proxyTraffic[detailProxy.id]?.cur_conns ?? 0 }} 连接
          </span>
        </div>
        <n-button size="small" quaternary @click="closeProxyDetail">关闭</n-button>
      </div>
      <div class="proxy-detail__tabbar">
        <button type="button" class="proxy-detail__tab" :class="{ 'proxy-detail__tab--active': proxyDetailTab === 'overview' }" @click="proxyDetailTab = 'overview'">概览</button>
        <button type="button" class="proxy-detail__tab" :class="{ 'proxy-detail__tab--active': proxyDetailTab === 'logs' }" @click="proxyDetailTab = 'logs'; loadProxyLogs(detailProxy.id)">日志</button>
      </div>
      <div class="proxy-detail__scroll">
        <div v-show="proxyDetailTab === 'overview'" class="proxy-detail__pane proxy-detail__pane--overview">
          <div class="overview-strip">
            <div class="overview-strip__item">
              <span class="overview-strip__label">服务端</span>
              <span class="overview-strip__value">{{ proxyDetailServerName(detailProxy) }}</span>
            </div>
            <div class="overview-strip__item">
              <span class="overview-strip__label">类型</span>
              <span class="overview-strip__value">
                <n-tag size="small" :type="frpTypeTagType[detailProxy.type]" :bordered="false" round>{{ detailProxy.type.toUpperCase() }}</n-tag>
              </span>
            </div>
            <div class="overview-strip__item">
              <span class="overview-strip__label">内网目标</span>
              <span class="overview-strip__value mono">{{ proxyDetailValue(detailProxy, 'local') }}</span>
            </div>
            <div class="overview-strip__item overview-strip__item--wide">
              <span class="overview-strip__label">公网入口</span>
              <span class="overview-strip__value mono">{{ proxyDetailValue(detailProxy, 'public') }}</span>
            </div>
          </div>
          <div class="overview-security">
            <span class="overview-security__label">传输策略</span>
            <template v-if="detailProxy.options?.transport">
              <n-tag v-if="detailProxy.options.transport.use_encryption" size="small" round :bordered="false">加密</n-tag>
              <n-tag v-if="detailProxy.options.transport.use_compression" size="small" round :bordered="false">压缩</n-tag>
              <n-tag v-if="detailProxy.options.transport.proxy_protocol_version" size="small" round :bordered="false">PP {{ detailProxy.options.transport.proxy_protocol_version }}</n-tag>
              <n-tag v-if="detailProxy.options.transport.bandwidth_limit" size="small" round :bordered="false">限速 {{ detailProxy.options.transport.bandwidth_limit }}</n-tag>
            </template>
            <span v-if="!detailProxy.options?.transport?.use_encryption && !detailProxy.options?.transport?.use_compression && !detailProxy.options?.transport?.proxy_protocol_version && !detailProxy.options?.transport?.bandwidth_limit" class="overview-security__empty">未启用</span>
          </div>
          <div class="overview-main">
            <section class="overview-card overview-card--chart">
              <div class="overview-card__head">
                <h4>实时流量 <span class="overview-card__sub">最近 5 分钟</span></h4>
                <span class="live-badge"><span class="live-badge__dot" />实时</span>
              </div>
              <div class="traffic-live-legend traffic-live-legend--compact">
                <span class="traffic-live-legend__item traffic-live-legend__item--up"><span class="traffic-live-legend__dot" />上传 {{ formatRate(proxyTraffic[detailProxy.id]?.traffic_in_rate ?? 0) }}</span>
                <span class="traffic-live-legend__item traffic-live-legend__item--down"><span class="traffic-live-legend__dot" />下载 {{ formatRate(proxyTraffic[detailProxy.id]?.traffic_out_rate ?? 0) }}</span>
              </div>
              <MiniTrafficChart class="overview-chart" :labels="proxyRateChart.labels" :upload="proxyRateChart.upload" :download="proxyRateChart.download" />
            </section>
            <aside class="overview-side">
              <section class="overview-metrics">
                <div class="overview-metric">
                  <div class="overview-metric__label">总上传</div>
                  <div class="overview-metric__value">{{ formatBytes(proxyTraffic[detailProxy.id]?.traffic_in ?? 0) }}</div>
                </div>
                <div class="overview-metric">
                  <div class="overview-metric__label">总下载</div>
                  <div class="overview-metric__value">{{ formatBytes(proxyTraffic[detailProxy.id]?.traffic_out ?? 0) }}</div>
                </div>
                <div class="overview-metric">
                  <div class="overview-metric__label">当前上传</div>
                  <div class="overview-metric__value">{{ formatRate(proxyTraffic[detailProxy.id]?.traffic_in_rate ?? 0) }}</div>
                </div>
                <div class="overview-metric">
                  <div class="overview-metric__label">当前下载</div>
                  <div class="overview-metric__value">{{ formatRate(proxyTraffic[detailProxy.id]?.traffic_out_rate ?? 0) }}</div>
                </div>
              </section>
            </aside>
          </div>
        </div>
        <div v-show="proxyDetailTab === 'logs'" class="proxy-detail__pane">
          <div class="rule-detail__log-panel-head">
            <span class="table-muted">frpc 规则日志（按规则名过滤所属服务端日志）</span>
            <n-button size="tiny" quaternary :loading="proxyLogsLoading" @click="loadProxyLogs(detailProxy.id)">刷新</n-button>
          </div>
          <pre class="logs-box rule-detail__logs">{{ proxyLogLines.length ? proxyLogLines.join('\n') : '暂无该规则的日志' }}</pre>
        </div>
      </div>
    </div>
  </n-modal>

  <n-modal v-model:show="showExport">
    <div class="frp-card-modal">
      <div class="proxy-modal__header">
        <h3 class="modal-title">脱敏 frpc.toml</h3>
        <n-button size="small" quaternary @click="showExport = false">关闭</n-button>
      </div>
      <div class="proxy-modal__scroll">
        <pre class="logs-box">{{ exportContent || '暂无可导出的启用配置' }}</pre>
      </div>
      <div class="modal-footer"><n-button @click="showExport = false">关闭</n-button></div>
    </div>
  </n-modal>

  <!-- 公网 Agent 管理已独立为 /frp-agent 页面（AgentView.vue） -->

  <n-modal
    v-model:show="showGuide"
    :mask-closable="false"
    transform-origin="center"
  >
    <div class="proxy-modal frp-guide-modal">
      <div class="proxy-modal__form">
        <div class="proxy-modal__header">
          <h3 class="modal-title">FRP 穿透使用说明</h3>
          <n-button size="small" quaternary @click="showGuide = false">
            <template #icon><n-icon :component="CloseOutline" /></template>
          </n-button>
        </div>
        <div class="proxy-modal__tabbar">
          <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': guideTab === 'guide' }" @click="guideTab = 'guide'">使用说明</button>
          <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': guideTab === 'scenes' }" @click="guideTab = 'scenes'">组合场景详解</button>
        </div>
        <div class="proxy-modal__scroll">
          <div class="proxy-modal__pane">
            <div v-show="guideTab === 'guide'">
    <div class="guide-section">
      <h4>一、配置流程（推荐顺序）</h4>
      <ol>
        <li><strong>VPS 侧部署 frps</strong>：在服务端「详情」弹窗点「frps.toml」生成与本服务端同源的配置（bindPort / 认证 / 管理接口一致），复制到 VPS 保存为 frps.toml 并执行 <code>./frps -c frps.toml</code>；按文件末尾的防火墙清单放行端口。</li>
        <li><strong>添加服务端</strong>：主机地址 / 端口 / 认证方式与 frps 一致；Token 加密存储，编辑时留空表示不修改；frps 管理接口（webServer）选填，仅用于拉取穿透流量与连接数，与连接认证无关。</li>
        <li><strong>添加穿透规则</strong>：8 种类型——tcp / udp 需远程端口（同一服务端内查重）；http / https / tcpmux 需自定义域名（走 frps 的 vhost 端口）；stcp / sudp / xtcp 无公网监听端口（凭 secretKey 访问）；内网目标可填 local_ip:port，或启用插件（static_file / socks5 / https2http 等，按类型校验必填字段）。</li>
        <li><strong>校验并应用</strong>：右上「应用配置」弹窗可管理 frpc 版本（下载 / 启用 / 删除）、配置下载代理，并校验应用配置；单台服务端可独立「启动 / 停止」。配置 verify 通过才落盘，坏配置不会覆盖现有运行。</li>
        <li><strong>验证</strong>：服务端「详情」含五项诊断（TCP 可达 / frpc 登录 / 代理可达 / DNS 解析 / 归属地）与可用率、延迟曲线；「日志」为该台服务端独立的 frpc 运行日志；规则「详情 → 日志」按规则名过滤。</li>
      </ol>
    </div>

    <div class="guide-section">
      <h4>二、与 DDNS 的组合</h4>
      <ul>
        <li><strong>IP 来源留空（默认）</strong>：每次同步自动检测本机公网 IP——家庭宽带 NAS 场景，域名指向家里出口。</li>
        <li><strong>IP 来源自定义</strong>：DDNS 页选中任务 → 「自定义 IP」填 VPS 的 IP，域名直接解析到 VPS（Web 服务经 VPS 对外时使用）。须为公网地址，内网 IP 会被拒绝。</li>
        <li><strong>VPS 动态 IP</strong>：DDNS 自定义 IP 填当前 VPS IP，FRP 服务端「主机地址」填 DDNS 域名而非裸 IP——VPS 换 IP 后域名自动跟随，frpc 重连无需改 Havline 配置。</li>
      </ul>
    </div>

    <div class="guide-section">
      <h4>三、与反向代理的组合（按需求选型）</h4>
      <ul>
        <li><strong>多 Web 应用统一入口（推荐）</strong>：FRP 一条 tcp 规则，远程端口 443，内网目标 127.0.0.1:443（Havline Nginx 的 HTTPS 监听）；反向代理页按域名分流到各内网应用，TLS 与证书都在本地 Nginx 终止。加规则只改反代，不动 FRP。</li>
        <li><strong>HTTP/HTTPS 直通</strong>：frps 按域名分流，不需要本地反代。存在 http/https 规则时，本页生成的 frps.toml 会自动带上 vhostHTTPPort / vhostHTTPSPort；FRP 规则选 http / https 类型并填自定义域名。TLS 终止在 VPS 侧或上游。</li>
        <li><strong>端口映射</strong>：SSH、远程桌面、数据库等非 HTTP 服务——tcp / udp 规则 + 远程端口，访问 <code>VPS_IP:远程端口</code> 即达内网。</li>
        <li><strong>STCP 安全穿透</strong>：管理入口不想暴露公网端口时用 stcp，只放行 frps 的 bindPort；访问端运行同 secretKey 的 visitor 配置连回内网。</li>
        <li><strong>有公网 IP 的直连</strong>：无需 FRP——DDNS 指向家庭宽带出口 + 路由器端口映射 + 反代页的域名 / HTTPS / 证书即可，少一跳延迟但暴露出口 IP。</li>
      </ul>
    </div>

    <div class="guide-section">
      <h4>四、与证书的配合</h4>
      <ul>
        <li>证书页用 ACME 自动申请（DNS-01 验证复用 DDNS 的服务商凭证），通配符证书 <code>*.example.com</code> 一张即可覆盖反代的多个域名。</li>
        <li>「统一入口」方案：证书装在本地 Nginx（反代规则里选择）；「HTTP/HTTPS 直通」方案：证书在 VPS 侧或上游终止，本地不需要。</li>
        <li>反代指定证书不可用（过期 / 不覆盖域名）时保存会被阻断，不会静默降级为 HTTP。</li>
      </ul>
    </div>

    <div class="guide-section">
      <h4>五、常见问题速查</h4>
      <table class="guide-table">
        <thead><tr><th>现象</th><th>排查</th></tr></thead>
        <tbody>
          <tr><td>校验失败 / login to server failed</td><td>Token 不一致或 frps 未启动；用服务端详情的「frps.toml」对照两端配置</td></tr>
          <tr><td>登录成功但外部访问失败</td><td>依次查：DNS 解析 → VPS 防火墙 → frps 端口（vhost / 远程端口）→ 本地 Nginx</td></tr>
          <tr><td>域名打不开、IP 能通</td><td>frps 未开 vhostHTTPPort / vhostHTTPSPort（http/https 规则必需）</td></tr>
          <tr><td>流量 / 连接数全为 0</td><td>frps 未配置 webServer，或服务端表单未填管理地址 / 端口；非必须功能</td></tr>
          <tr><td>改了配置没生效</td><td>「应用配置」里点「校验并应用配置」，或对单台服务端重启</td></tr>
        </tbody>
      </table>
    </div>
            </div>
            <div v-show="guideTab === 'scenes'">
    <div class="guide-section">
      <h4>六、组合场景详解</h4>
      <p class="guide-note">按需求选型，点开场景查看拓扑与操作步骤；步骤均基于当前页面实际功能（服务端 / 穿透规则 / frps.toml 同源生成 / DDNS / 反向代理 / 证书）。</p>
      <n-collapse accordion :default-expanded-names="['scene-a']" class="guide-collapse">
        <n-collapse-item title="场景 A：多 Web 应用统一入口（DDNS + FRP TCP + 反向代理 + 本地证书）" name="scene-a">
          <p class="guide-note">适用：家庭宽带 NAS，多个 Web 应用统一 443 对外，VPS 做公网入口中转。</p>
          <pre class="guide-topo">用户 → https://app.example.com (443)
     → DNS 解析到 VPS IP
     → VPS:443 → frps → FRP TCP 隧道（远程端口 443）
     → NAS frpc → 127.0.0.1:443
     → Havline Nginx（本地证书终止 TLS，按域名分流）→ 内网应用</pre>
          <ol class="guide-steps">
            <li>VPS 部署 frps：服务端「详情 → frps.toml」生成同源配置，放行 bindPort 与 443；</li>
            <li>DDNS：解析 example.com / *.example.com 到 VPS IP（「自定义 IP」填 VPS IP）；</li>
            <li>证书：为 *.example.com 申请通配符证书（DNS-01 复用 DDNS 凭证）；</li>
            <li>FRP 服务端：添加 VPS，Token 与 frps 一致；</li>
            <li>FRP 规则：类型 tcp，远程端口 443，内网目标 127.0.0.1:443（Havline Nginx 的 HTTPS 监听）；</li>
            <li>反向代理：按域名建规则（HTTPS + 强制跳转 + 选证书），目标为各内网应用；以后加应用只改反代，不动 FRP；</li>
            <li>「应用配置」校验并应用 → 服务端「详情」看诊断与日志验证。</li>
          </ol>
          <p class="guide-note">域名也可解析到家庭宽带 IP（DDNS 留空自动检测），VPS 仅做隧道；两种都可行，关键是 frps 监听的 443 必须有流量进来。</p>
        </n-collapse-item>
        <n-collapse-item title="场景 B：VPS 动态 IP 跟随（DDNS + FRP 服务端地址）" name="scene-b">
          <p class="guide-note">适用：VPS 是动态 IP / 周期变化 / 多台轮换，域名始终指向当前 VPS。</p>
          <ol class="guide-steps">
            <li>DDNS「自定义 IP」填当前 VPS IP（注意：留空检测的是 NAS 本机出口 IP，两者语义不同）；</li>
            <li>FRP 服务端「主机地址」填 DDNS 域名（如 vps.example.com）而非裸 IP；</li>
            <li>VPS IP 变化后 DDNS 定时任务自动跟随，frpc 重连无需改 Havline 配置；</li>
            <li>服务端「日志」确认重新出现 login to server success。</li>
          </ol>
          <p class="guide-note">通常与场景 A / C 叠加：DDNS 保证「地址跟着 VPS 走」，FRP 保证「隧道通」。</p>
        </n-collapse-item>
        <n-collapse-item title="场景 C：HTTP/HTTPS 直通（frps vhost 分流 + DDNS）" name="scene-c">
          <p class="guide-note">适用：少量 Web 服务、不想配本地反代，frps 直接按域名分流。</p>
          <pre class="guide-topo">用户 → http://app.example.com:8080
     → DNS → VPS
     → frps（vhostHTTPPort，按 Host 分流）
     → FRP HTTP 隧道 → NAS 内网应用</pre>
          <ol class="guide-steps">
            <li>存在 http/https 规则时，frps.toml 会自动带 vhostHTTPPort / vhostHTTPSPort；VPS 上运行 Nginx 时请使用 8080/8443，80/443 留给 Nginx 对外监听；</li>
            <li>DDNS「自定义 IP」把 app.example.com 解析到 VPS IP；</li>
            <li>FRP 规则：类型 http（或 https），自定义域名 app.example.com，内网目标为应用地址；</li>
            <li>应用配置后验证。</li>
          </ol>
          <p class="guide-note">此方案 TLS 终止在 VPS 侧或上游，本地 Nginx 不参与；HTTPS 场景必须明确 TLS 终止位置。</p>
        </n-collapse-item>
        <n-collapse-item title="场景 D：TCP / UDP 端口映射（SSH / 远程桌面 / 数据库）" name="scene-d">
          <p class="guide-note">适用：SSH、远程桌面、数据库、游戏等非 HTTP 服务。</p>
          <ol class="guide-steps">
            <li>FRP 规则：类型 tcp（或 udp），远程端口如 6022，内网目标 127.0.0.1:22；</li>
            <li>应用配置后访问 VPS_IP:6022 即达内网 SSH；</li>
            <li>可选：传输加密 / 压缩；同一服务端内 tcp、udp 的远程端口分别查重。</li>
          </ol>
          <p class="guide-note">管理入口不想暴露公网端口时改用 stcp（见场景 G）。</p>
        </n-collapse-item>
        <n-collapse-item title="场景 E：多服务端运维" name="scene-e">
          <p class="guide-note">适用：多台 VPS、规则分组管理。</p>
          <ol class="guide-steps">
            <li>添加多个 FRP 服务端（各自地址 / Token / 认证方式），规则分别归属；</li>
            <li>每台服务端独立「启动 / 停止 / 日志 / 诊断 / frps.toml 生成」，互不干扰；</li>
            <li>「应用配置」弹窗统一管理 frpc 版本（下载 / 启用 / 删除）与下载代理，对所有服务端生效；</li>
            <li>顶部状态卡显示全局客户端状态与启用规则数；规则「详情 → 流量」看各规则实时连接与速率（需 frps 配置 webServer）。</li>
          </ol>
        </n-collapse-item>
        <n-collapse-item title="场景 F：有公网 IP 直连（DDNS + 本地 Nginx + 证书，无 VPS）" name="scene-f">
          <p class="guide-note">适用：有公网 IPv4 / IPv6 的家庭宽带，不需要中转。</p>
          <pre class="guide-topo">用户 → https://app.example.com
     → DNS（DDNS 自动更新到宽带出口 IP）
     → 家庭宽带路由器端口映射 :443
     → NAS Havline Nginx（本地证书）→ 内网应用</pre>
          <ol class="guide-steps">
            <li>确认宽带有公网 IP（没有则只能走 FRP 中转或 IPv6 直连）；</li>
            <li>DDNS IP 来源留空（自动检测本机出口）；</li>
            <li>路由器把 WAN 80/443 映射到 NAS；</li>
            <li>反向代理 + 证书按需配置，浏览器直接访问域名验证。</li>
          </ol>
          <p class="guide-note">与场景 A 的取舍：直连少一跳延迟、不依赖 VPS，但暴露家庭出口 IP 且需路由器权限；FRP 中转隐藏出口、多一层安全，但流量过 VPS。</p>
        </n-collapse-item>
        <n-collapse-item title="场景 G：STCP 安全穿透（不暴露公网端口）" name="scene-g">
          <p class="guide-note">适用：SSH、远程桌面等管理入口，不想在 VPS 上开任何业务监听端口。</p>
          <ol class="guide-steps">
            <li>FRP 规则：类型 stcp，无远程端口；填 secret_key（可选 allow_users 限定访问者），内网目标如 127.0.0.1:22；</li>
            <li>访问端设备运行同 secretKey 的 frpc visitor 配置，生成本地入口（如 127.0.0.1:6022）；</li>
            <li>访问：ssh -p 6022 127.0.0.1（在装有 visitor 的设备上）。</li>
          </ol>
          <p class="guide-note">VPS 防火墙只需放行 bindPort，扫描器扫不到服务；sudp / xtcp 同理（xtcp 优先 P2P 直连，失败回退中转）。</p>
        </n-collapse-item>
        <n-collapse-item title="场景 H：FRP 插件（static_file / socks5 / https2http 等）" name="scene-h">
          <p class="guide-note">适用：不建反代、不起本地应用，用 frpc 内置插件直接提供能力；插件必填字段在保存规则时按类型校验。</p>
          <table class="guide-table">
            <thead><tr><th>插件</th><th>用途</th><th>必填</th></tr></thead>
            <tbody>
              <tr><td>static_file</td><td>快速共享一个目录（简单文件服务器）</td><td>本地目录路径</td></tr>
              <tr><td>unix_domain_socket</td><td>暴露本机 Unix Socket 服务</td><td>Socket 路径</td></tr>
              <tr><td>http_proxy / socks5</td><td>把 NAS 变成出口代理（配合 visitor 内网穿透用）</td><td>本地地址</td></tr>
              <tr><td>https2http / https2https / tls2raw</td><td>frpc 侧 TLS 卸载 / 转换后转发到本地 HTTP</td><td>本地地址 + 证书 crt/key</td></tr>
            </tbody>
          </table>
          <p class="guide-note">操作：添加 / 编辑规则 → 「内网目标」启用插件 → 按类型填写字段 → 保存（校验不过会明确提示缺哪个字段）；远程端口 / 域名要求与普通规则相同。</p>
        </n-collapse-item>
        <n-collapse-item title="场景 I：CDN 前置 + FRP TCP + 本地 Nginx" name="scene-i">
          <p class="guide-note">适用：域名挂在 Cloudflare 等 CDN 后面，隐藏源站 IP、边缘加速。</p>
          <pre class="guide-topo">用户 → CDN（边缘 TLS）
     → 回源 → VPS frps :远程端口（FRP TCP 隧道）
     → NAS Havline Nginx（HTTP 或本地证书）→ 内网应用</pre>
          <ol class="guide-steps">
            <li>CDN 侧添加域名，DNS 由 CDN 托管（此时可不建 DDNS，或 DDNS 只用于 FRP 服务端地址，见场景 B）；</li>
            <li>FRP tcp 规则：远程端口如 8443，内网目标指向 Havline Nginx 的对应监听；</li>
            <li>反代 HTTPS 按回源模式决定（Flexible → HTTP 回源可关 443；Full → 需本地证书）；</li>
            <li>真实客户端 IP 来自 CDN，需在设置中配置信任代理网段，IP 黑白名单 / 限流才基于真实 IP 生效。</li>
          </ol>
        </n-collapse-item>
        <n-collapse-item title="场景 J：安全策略组合（反向代理访问控制）" name="scene-j">
          <p class="guide-note">适用：对外服务加访问控制，多层叠加。</p>
          <ol class="guide-steps">
            <li>IP 白名单：仅办公室 / 家庭出口可访问管理面板类应用；</li>
            <li>大陆 IP 白名单：只允许境内用户（China CIDR 数据源在设置中配置）；</li>
            <li>全局 IP 黑名单：设置页配置，对所有反代规则生效（如封扫描源）；</li>
            <li>Basic Auth：简单口令保护无登录页的内网工具；</li>
            <li>限流 / 连接数限制：防 CC / 爬虫；</li>
            <li>组合建议：管理入口 = STCP（场景 G）或白名单 + Basic Auth；公开 Web = CDN + 大陆白名单 / 黑名单 + 限流。规则修改保存后 Nginx 原子应用，失败自动回滚。</li>
          </ol>
        </n-collapse-item>
      </n-collapse>
    </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </n-modal>

  <n-modal v-model:show="showRuntimeModal">
    <div class="frp-card-modal frp-runtime-modal">
      <div class="proxy-modal__header">
        <h3 class="modal-title">FRP 应用配置</h3>
        <n-button size="small" quaternary @click="showRuntimeModal = false">关闭</n-button>
      </div>
      <div class="proxy-modal__scroll">
    <div class="runtime-actions">
      <n-button size="small" :loading="runtimeLoading" @click="loadRuntimeConfig">
        <template #icon><n-icon :component="RefreshOutline" /></template>
        刷新
      </n-button>
      <n-button size="small" type="primary" :loading="actionLoading === 'reload'" @click="applyRuntimeConfig">
        校验并应用配置
      </n-button>
    </div>

    <section class="runtime-section">
      <h3>本机运行环境</h3>
      <n-descriptions :column="1" label-placement="left" bordered size="small">
        <n-descriptions-item label="系统架构">{{ runtime.platform || '检测中' }}</n-descriptions-item>
        <n-descriptions-item label="当前 frpc">{{ runtime.active_bin || '未检测到' }}</n-descriptions-item>
        <n-descriptions-item label="检测版本">{{ runtime.detected_version || '未检测到' }}</n-descriptions-item>
        <n-descriptions-item label="配置文件"><span v-if="runtime.config_paths?.length">{{ runtime.config_paths.join('、') }}</span><span v-else>{{ runtime.config_path }}</span>{{ runtime.config_exists ? '（已生成）' : '（尚未生成）' }}</n-descriptions-item>
      </n-descriptions>
    </section>

    <section class="runtime-section">
      <div class="runtime-section__title"><h3>下载加速代理</h3><span>仅代理 FRP 官方 Release 下载地址</span></div>
      <div class="download-proxy-form">
        <n-select v-model:value="downloadProxy" :options="downloadProxyOptions" placeholder="选择下载方式" />
        <n-button type="primary" :loading="savingDownloadProxy" @click="saveDownloadProxy">保存</n-button>
      </div>
    </section>

    <section class="runtime-section">
      <div class="runtime-section__title"><h3>已下载客户端</h3><span>{{ binaries.length }} 个版本</span></div>
      <n-empty v-if="!runtimeLoading && binaries.length === 0" size="small" description="尚未下载 FRP 客户端" />
      <div v-else class="binary-list">
        <div v-for="binary in binaries" :key="binary.version" class="binary-row">
          <div><strong>{{ binary.version }}</strong><div class="binary-row__meta">{{ formatBinarySize(binary.size) }} · {{ formatDate(binary.installed_at) }}</div></div>
          <div class="binary-row__actions">
            <n-tag v-if="binary.active" size="small" type="success" :bordered="false">当前使用</n-tag>
            <n-button v-else size="small" :loading="activatingVersion === binary.version" @click="confirmActivate(binary.version)">启用</n-button>
            <n-button v-if="!binary.active" size="small" quaternary type="error" :loading="deletingVersion === binary.version" @click="confirmDeleteBinary(binary)">删除</n-button>
          </div>
        </div>
      </div>
    </section>

    <section class="runtime-section">
      <div class="runtime-section__title"><h3>FRP 官方下载</h3><span>仅显示当前系统 {{ runtime.platform || '' }} 可用的稳定版本</span></div>
      <n-empty v-if="!runtimeLoading && releases.length === 0" size="small" description="未获取到官方版本列表" />
      <div v-else class="release-list">
        <div v-for="release in visibleReleases" :key="release.version" class="release-row">
          <div class="release-row__info"><strong>{{ release.version }}</strong><div class="binary-row__meta">{{ release.asset_name }} · {{ formatBinarySize(release.size) }}</div></div>
          <div v-if="downloadTask.version === release.version && downloadTask.status !== 'idle'" class="release-progress" :class="`release-progress--${downloadTask.status}`" :title="downloadTask.error || downloadTask.message">
            <span class="release-progress__message">{{ downloadTask.error || downloadTask.message }}</span>
            <n-progress v-if="downloadTask.status !== 'failed'" class="release-progress__bar" type="line" :percentage="downloadPercent" :status="downloadProgressStatus" :show-indicator="false" />
            <span v-if="downloadTask.status !== 'failed'" class="release-progress__value">{{ downloadTask.total > 0 ? `${downloadPercent}%` : '...' }}</span>
          </div>
          <div v-else class="release-progress-placeholder" />
          <n-button size="small" :loading="downloadingVersion === release.version" :disabled="isDownloaded(release.version) || downloadInProgress" @click="confirmDownload(release)">
            {{ isDownloaded(release.version) ? '已下载' : '下载' }}
          </n-button>
        </div>
      </div>
      <div v-if="releases.length > releaseLimit" class="release-list__more">
        <n-button text type="primary" @click="releasesExpanded = !releasesExpanded">
          {{ releasesExpanded ? '收起列表' : `显示更多（剩余 ${releases.length - releaseLimit} 条）` }}
        </n-button>
      </div>
    </section>
      </div>
      <div class="modal-footer"><n-button @click="showRuntimeModal = false">关闭</n-button></div>
    </div>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, h, onMounted, reactive, ref, type VNode } from 'vue'
import type { DataTableColumns } from 'naive-ui'
import type { FrpProxyTraffic, FrpProxiesTraffic } from '../api/types'
import { NButton, NIcon, NSpace, NSwitch, NTag, useDialog, useMessage } from 'naive-ui'
import {
  AddOutline, CloseOutline, DocumentTextOutline, FlashOutline, GitNetworkOutline, InformationCircleOutline,
  LayersOutline, PeopleOutline, PlayOutline, RefreshOutline, SettingsOutline, StopOutline, TerminalOutline, TimeOutline,
} from '@vicons/ionicons5'
import { RouterLink } from 'vue-router'
import { api, asList } from '../api/client'
import { useVisibilityPolling } from '../composables/useVisibilityPolling'
import { formatBytes, formatDate, formatRate } from '../utils/format'

import type { FrpBinary, FrpDownloadTask, FrpProxy, FrpProxyOptions, FrpProxyPayload, FrpRelease, FrpRuntimeInfo, FrpServer, FrpStatus } from '../api/types'
import HavlineCard from '../components/HavlineCard.vue'
import EmptyState from '../components/EmptyState.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import MiniTrafficChart from '../components/MiniTrafficChart.vue'

const message = useMessage()
const dialog = useDialog()
const status = ref<FrpStatus>({ status: 'unknown', configured: false, enabled_proxies: 0 })
const servers = ref<FrpServer[]>([])
const proxies = ref<FrpProxy[]>([])
const proxyTraffic = ref<Record<number, FrpProxyTraffic>>({})
const proxyTrafficErrors = ref<Record<number, string>>({})
const loadError = ref('')
const actionLoading = ref<'start' | 'stop' | 'reload' | null>(null)
const savingProxy = ref(false)
const showProxyModal = ref(false)
const showExport = ref(false)
const showGuide = ref(false)
const guideTab = ref<'guide' | 'scenes'>('guide')

const showRuntimeModal = ref(false)
const proxyFormTab = ref<'basic' | 'advanced'>('basic')
const exportContent = ref('')
const runtimeLoading = ref(false)
const savingDownloadProxy = ref(false)
const downloadingVersion = ref<string | null>(null)
const activatingVersion = ref<string | null>(null)
const deletingVersion = ref<string | null>(null)
const downloadTask = ref<FrpDownloadTask>({ status: 'idle', message: '', downloaded: 0, total: 0 })
const runtime = ref<FrpRuntimeInfo>({ platform: '', configured_bin: '', download_proxy: '', config_path: '', config_exists: false })
const releases = ref<FrpRelease[]>([])
const releasesExpanded = ref(false)
const binaries = ref<FrpBinary[]>([])
const downloadProxy = ref('')
const downloadProxyOptions = [
  { label: '直连 GitHub 官方 Release', value: '' },
  { label: 'gh-proxy.org', value: 'https://gh-proxy.org/' },
  { label: 'v4.gh-proxy.org', value: 'https://v4.gh-proxy.org/' },
  { label: 'cdn.gh-proxy.org', value: 'https://cdn.gh-proxy.org/' },
]
const editingProxy = ref<FrpProxy | null>(null)
const downloadPolling = ref(false)

function defaultProxyOptions(): FrpProxyOptions & { allow_ips_text?: string } {
  return { transport: {}, health_check: {}, load_balancer: {}, nat_traversal: { disable_assisted_addrs: false }, allow_ips_text: '' }
}

const proxyForm = reactive({ server_id: 0, name: '', type: 'tcp' as FrpProxy['type'], local_ip: '127.0.0.1', local_port: 80, remote_port: 0, domains_text: '', locations_text: '', allow_users_text: '', metadatas_text: '', request_headers_text: '', response_headers_text: '', host_header_rewrite: '', enabled: true, remark: '', options: defaultProxyOptions() as FrpProxyOptions & { allow_ips_text?: string }, plugin_type: '', plugin_local_addr: '', plugin_unix_path: '', plugin_local_path: '', plugin_username: '', plugin_password: '' })
const proxyTypeOptions = [
  { label: 'TCP', value: 'tcp' }, { label: 'UDP', value: 'udp' }, { label: 'HTTP', value: 'http' }, { label: 'HTTPS', value: 'https' },
  { label: 'TCPMUX（HTTP CONNECT）', value: 'tcpmux' }, { label: 'STCP（密钥访问）', value: 'stcp' }, { label: 'SUDP（密钥 UDP）', value: 'sudp' }, { label: 'XTCP（P2P 穿透）', value: 'xtcp' },
]
const proxyQuickExamples: Record<FrpProxy['type'], {
  title: string
  description: string
  lines: string[]
  name: string
  localPort: number
  remotePort: number
  domains: string
  remark: string
  secretKey: string
  multiplexer: string
}> = {
  tcp: { title: 'TCP 端口映射', description: '将公网 TCP 端口映射到本机 SSH 服务。', lines: ['name = "ssh-tcp"', 'type = "tcp"', 'localPort = 22', 'remotePort = 6000'], name: 'ssh-tcp', localPort: 22, remotePort: 6000, domains: '', remark: '公网 SSH 端口映射', secretKey: '', multiplexer: '' },
  udp: { title: 'UDP 端口映射', description: '将公网 UDP 端口映射到本机 DNS 服务。', lines: ['name = "dns-udp"', 'type = "udp"', 'localPort = 53', 'remotePort = 6001'], name: 'dns-udp', localPort: 53, remotePort: 6001, domains: '', remark: '公网 DNS 端口映射', secretKey: '', multiplexer: '' },
  http: { title: 'HTTP 域名路由', description: '通过域名将 HTTP 请求转发到本机 Web 服务。', lines: ['name = "web-http"', 'type = "http"', 'localPort = 80', 'customDomains = ["web.example.com"]'], name: 'web-http', localPort: 80, remotePort: 0, domains: 'web.example.com', remark: 'HTTP Web 服务', secretKey: '', multiplexer: '' },
  https: { title: 'HTTPS 域名路由', description: '通过域名将 HTTPS 请求转发到本机 HTTPS 服务。', lines: ['name = "web-https"', 'type = "https"', 'localPort = 443', 'customDomains = ["web.example.com"]'], name: 'web-https', localPort: 443, remotePort: 0, domains: 'web.example.com', remark: 'HTTPS Web 服务', secretKey: '', multiplexer: '' },
  tcpmux: { title: 'TCPMUX 多路复用', description: '通过 HTTP CONNECT 和域名区分多个内网服务。', lines: ['name = "connect-service"', 'type = "tcpmux"', 'multiplexer = "httpconnect"', 'customDomains = ["connect.example.com"]'], name: 'connect-service', localPort: 8080, remotePort: 0, domains: 'connect.example.com', remark: 'HTTP CONNECT 多路复用', secretKey: '', multiplexer: 'httpconnect' },
  stcp: { title: 'STCP 密钥访问', description: '仅允许持有相同访问密钥的访客端连接。', lines: ['name = "ssh-stcp"', 'type = "stcp"', 'localPort = 22', 'secretKey = "请替换为强密钥"'], name: 'ssh-stcp', localPort: 22, remotePort: 0, domains: '', remark: '受密钥保护的 SSH 服务', secretKey: '请替换为强密钥', multiplexer: '' },
  sudp: { title: 'SUDP 密钥访问', description: '仅允许持有相同访问密钥的访客端访问 UDP 服务。', lines: ['name = "dns-sudp"', 'type = "sudp"', 'localPort = 53', 'secretKey = "请替换为强密钥"'], name: 'dns-sudp', localPort: 53, remotePort: 0, domains: '', remark: '受密钥保护的 DNS 服务', secretKey: '请替换为强密钥', multiplexer: '' },
  xtcp: { title: 'XTCP P2P 穿透', description: '通过访客端和服务端建立点对点连接。', lines: ['name = "ssh-xtcp"', 'type = "xtcp"', 'localPort = 22', 'secretKey = "请替换为强密钥"'], name: 'ssh-xtcp', localPort: 22, remotePort: 0, domains: '', remark: 'P2P SSH 服务', secretKey: '请替换为强密钥', multiplexer: '' },
}
const proxyQuickExample = computed(() => proxyQuickExamples[proxyForm.type])
const pluginTypeOptions = [
  { label: 'Unix Domain Socket', value: 'unix_domain_socket' }, { label: 'HTTP Proxy', value: 'http_proxy' }, { label: 'Socks5', value: 'socks5' }, { label: 'Static File', value: 'static_file' },
  { label: 'HTTP 转 HTTPS', value: 'http2https' }, { label: 'HTTPS 转 HTTP', value: 'https2http' }, { label: 'HTTPS 转 HTTPS', value: 'https2https' }, { label: 'HTTP 转 HTTP', value: 'http2http' }, { label: 'TLS 转 Raw', value: 'tls2raw' },
]
const proxyHelpItems = computed(() => {
  if (proxyFormTab.value === 'advanced') return [
    { title: '传输安全', text: '加密和压缩只作用于该规则；Proxy Protocol 需要内网服务支持对应版本。' },
    { title: '健康检查', text: 'FRP 会定期检查内网服务，连续失败后暂时从服务端移除。HTTP 检查需要填写可返回 2xx 的路径。' },
    { title: '负载均衡', text: '同一服务端下的规则使用相同负载均衡组和组密钥后，FRP 才会在它们之间分配连接。' },
    { title: '插件', text: '启用插件后，插件接管本地服务；TCP/UDP 插件仍需要填写公网远程端口。' },
  ]
  const common = [
    { title: '公网服务端', text: '选择规则实际连接的 frps 服务端，可同时配置多个服务端。' },
    { title: '内网目标', text: '填写运行在本机或局域网内的服务地址和端口，保存后会写入对应 frpc 配置文件。' },
  ]
  if (proxyForm.type === 'tcp' || proxyForm.type === 'udp') return [...common, { title: '公网远程端口', text: 'frps 监听的公网端口；请确保该端口未被其他规则占用，并已在防火墙放行。' }]
  if (proxyForm.type === 'http' || proxyForm.type === 'https') return [...common, { title: '域名', text: '将域名解析到 FRP 服务端地址；HTTP/HTTPS 规则通过域名匹配请求。' }, { title: 'Host 重写', text: '需要内网服务按指定 Host 路由时填写；留空则保留客户端请求的 Host。' }]
  if (proxyForm.type === 'tcpmux') return [...common, { title: 'TCPMUX', text: '使用 HTTP CONNECT 多路复用，客户端通过自定义域名区分不同内网服务。' }]
  return [...common, { title: '访问密钥', text: 'STCP、SUDP、XTCP 不暴露公网端口，需要访客端使用相同密钥和服务名称连接。' }]
})
const proxyHelpTip = computed(() => {
  if (proxyFormTab.value === 'advanced') return '高级参数按 FRP v0.71 官方 frpc 配置示例生成；修改后请点击“校验并应用配置”。'
  if (proxyForm.type === 'http' || proxyForm.type === 'https') return '请先将域名解析到 FRP 服务端，并确认 frps 已开放对应端口。'
  if (proxyForm.type === 'stcp' || proxyForm.type === 'sudp' || proxyForm.type === 'xtcp') return '访客端需要单独配置 visitors，且 serverName、secretKey 和用户设置必须匹配。'
  return '请确认公网服务端可连接、远程端口已放行，且内网目标服务正在运行。'
})

function applyProxyQuickExample() {
  const example = proxyQuickExample.value
  Object.assign(proxyForm, {
    name: example.name, local_ip: '127.0.0.1', local_port: example.localPort, remote_port: example.remotePort,
    domains_text: example.domains, locations_text: '', allow_users_text: '', metadatas_text: '', request_headers_text: '',
    response_headers_text: '', host_header_rewrite: '', remark: example.remark, plugin_type: '', plugin_local_addr: '',
    plugin_unix_path: '', plugin_local_path: '', plugin_username: '', plugin_password: '',
  })
  proxyForm.options = { ...defaultProxyOptions(), secret_key: example.secretKey || undefined, multiplexer: example.multiplexer || undefined }
  message.success(`已填入${example.title}示例，请按实际环境调整后保存`)
}

const proxyServerOptions = computed(() => servers.value.map((server) => ({ label: `${server.name} (${server.server_addr}:${server.server_port})`, value: server.id })))
const statusLabel = computed(() => ({ running: '运行中', stopped: '已停止', unknown: '未配置' })[status.value.status] ?? '异常')
// 返回的必须是 StatusKind 合法值（success/warning/error/disabled/unknown），
// 之前用 'default' 且当 value 传给 StatusBadge，导致“运行中”被判为 unknown（灰色）
const statusKind = computed(() => (status.value.status === 'running' ? 'success' : status.value.status === 'stopped' ? 'disabled' : 'warning') as 'success' | 'disabled' | 'warning')
const downloadInProgress = computed(() => ['queued', 'downloading', 'verifying', 'extracting'].includes(downloadTask.value.status))
const downloadPercent = computed(() => downloadTask.value.total > 0 ? Math.min(100, Math.round(downloadTask.value.downloaded / downloadTask.value.total * 100)) : 0)
const downloadProgressStatus = computed(() => downloadTask.value.status === 'completed' ? 'success' : downloadTask.value.status === 'failed' ? 'error' : 'default')
const releaseLimit = 5
const visibleReleases = computed(() => {
  if (releasesExpanded.value || releases.value.length <= releaseLimit) return releases.value
  const visible = releases.value.slice(0, releaseLimit)
  if (!downloadTask.value.version || visible.some((release) => release.version === downloadTask.value.version)) return visible
  const activeRelease = releases.value.find((release) => release.version === downloadTask.value.version)
  return activeRelease ? [...visible.slice(0, releaseLimit - 1), activeRelease] : visible
})

const frpTypeTagType: Record<FrpProxy['type'], 'info' | 'success' | 'warning' | 'default'> = {
  tcp: 'info', udp: 'default', http: 'success', https: 'success', tcpmux: 'info', stcp: 'warning', sudp: 'warning', xtcp: 'warning',
}

// 穿透规则表格：列宽固定，各行列天然对齐（单元格由 render 创建，样式必须 :deep()）
const columns = computed<DataTableColumns<FrpProxy>>(() => [
  { title: '名称', key: 'name', minWidth: 150, render: (row) => h('div', [h('strong', row.name), row.remark ? h('div', { class: 'table-muted' }, row.remark) : null]) },
  { title: '类型', key: 'type', width: 92, render: (row) => h(NTag, { size: 'small', type: frpTypeTagType[row.type], bordered: false, round: true }, () => row.type.toUpperCase()) },
  { title: '内网目标', key: 'local', minWidth: 170, render: (row) => h('code', proxyLocalTarget(row)) },
  { title: '公网入口', key: 'public', minWidth: 200, render: (row) => h('code', proxyPublicEndpoint(row)) },
  { title: '传输', key: 'transport', width: 150, render: (row) => {
      const tags = proxyTransportTags(row)
      return tags.length ? h(NSpace, { size: 4, wrap: false }, () => tags) : h('span', { class: 'table-muted' }, '标准传输')
    } },
  { title: '健康检查', key: 'health_check', width: 96, render: (row) => {
      const checkType = row.options?.health_check?.type
      return checkType
        ? h(NTag, { size: 'small', type: 'success', bordered: false, round: true }, () => checkType.toUpperCase())
        : h('span', { class: 'table-muted' }, '—')
    } },
  { title: '负载均衡', key: 'load_balancer', width: 104, render: (row) => {
      const group = row.options?.load_balancer?.group
      return group
        ? h(NTag, { size: 'small', type: 'info', bordered: false, round: true }, () => group)
        : h('span', { class: 'table-muted' }, '—')
    } },
  { title: '状态', key: 'enabled', width: 100, render: (row) => h('div', { onClick: (event: MouseEvent) => event.stopPropagation() }, h(NSwitch, { value: row.enabled, size: 'small', onUpdateValue: (value: boolean) => toggleProxy(row, value) })) },
  { title: '当前连接', key: 'cur_conns', width: 88, render: (row) => h('span', { class: 'mono' }, String(proxyTraffic.value[row.id]?.cur_conns ?? 0)) },
  { title: '当前上传', key: 'traffic_in_rate', width: 96, render: (row) => h('span', { class: 'mono text-secondary' }, formatRate(proxyTraffic.value[row.id]?.traffic_in_rate ?? 0)) },
  { title: '当前下载', key: 'traffic_out_rate', width: 96, render: (row) => h('span', { class: 'mono text-secondary' }, formatRate(proxyTraffic.value[row.id]?.traffic_out_rate ?? 0)) },
  { title: '总上传', key: 'traffic_in', width: 96, render: (row) => h('span', { class: 'mono' }, formatBytes(proxyTraffic.value[row.id]?.traffic_in ?? 0)) },
  { title: '总下载', key: 'traffic_out', width: 96, render: (row) => h('span', { class: 'mono' }, formatBytes(proxyTraffic.value[row.id]?.traffic_out ?? 0)) },
  { title: '更新时间', key: 'updated_at', width: 110, render: (row) => h('span', { class: 'table-muted' }, formatDate(row.updated_at)) },
  { title: '操作', key: 'actions', width: 200, render: (row) => h(NSpace, { size: 2, wrap: false }, () => [
    h(NButton, { size: 'small', quaternary: true, onClick: () => openProxyDetail(row) }, () => '详情'),
    h(NButton, { size: 'small', quaternary: true, onClick: () => openProxyModal(row) }, () => '编辑'),
    h(NButton, { size: 'small', quaternary: true, type: 'error', onClick: () => confirmDeleteProxy(row) }, () => '删除'),
  ]) },
])

// 左侧选中服务端 → 右侧只列该服务端的规则；存 id 而不是对象，避免列表刷新后指向旧快照
const selectedServerId = ref<number | null>(null)
const selectedServer = computed(() => servers.value.find((item) => item.id === selectedServerId.value) ?? null)
const serverProxies = computed(() => proxies.value.filter((item) => item.server_id === selectedServerId.value))

const showProxyDetail = ref(false)
const detailProxy = ref<FrpProxy | null>(null)
const proxyDetailTab = ref<'overview' | 'logs'>('overview')

// 左侧列表的状态标签：只用列表接口就带的 boot_status，不为此额外拉诊断
function serverBootLabel(server: FrpServer) {
  switch (server.boot_status) {
    case 'running': return '运行中'
    case 'starting': return '启动中'
    case 'error': return '异常'
    default: return '已停止'
  }
}
function serverBootType(server: FrpServer): 'success' | 'warning' | 'error' | 'default' {
  switch (server.boot_status) {
    case 'running': return 'success'
    case 'starting': return 'warning'
    case 'error': return 'error'
    default: return 'default'
  }
}
function serverRuleCount(serverID: number) {
  return proxies.value.filter((item) => item.server_id === serverID).length
}
function serverEnabledRuleCount(serverID: number) {
  return proxies.value.filter((item) => item.server_id === serverID && item.enabled).length
}
const proxyLogLines = ref<string[]>([])
const proxyLogsLoading = ref(false)

// 详情弹窗的实时速率采样，复用列表页 5 秒轮询更新的 proxyTraffic 快照
const proxyRateHistory = ref<Array<{ at: number; upload: number; download: number }>>([])
const proxyRatePolling = computed(() => showProxyDetail.value)

const proxyRateChart = computed(() => {
  const now = Date.now()
  const labels: string[] = []
  const upload: number[] = []
  const download: number[] = []
  for (let i = 5; i >= 0; i--) {
    const bucketEnd = now - i * 60 * 1000
    const bucketStart = bucketEnd - 60 * 1000
    const points = proxyRateHistory.value.filter((p) => p.at > bucketStart && p.at <= bucketEnd)
    const avg = (key: 'upload' | 'download') =>
      points.length ? points.reduce((sum, p) => sum + p[key], 0) / points.length : 0
    upload.push(avg('upload'))
    download.push(avg('download'))
    const d = new Date(bucketEnd)
    labels.push(`${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`)
  }
  return { labels, upload, download }
})

function recordProxyRateSample() {
  const proxy = detailProxy.value
  if (!proxy) return
  const stats = proxyTraffic.value[proxy.id]
  if (!stats) return
  const now = Date.now()
  proxyRateHistory.value.push({ at: now, upload: stats.traffic_in_rate, download: stats.traffic_out_rate })
  proxyRateHistory.value = proxyRateHistory.value.filter((p) => now - p.at <= 5 * 60 * 1000)
}

// 切服务端：右侧表格跟着换
function selectServer(id: number) {
  if (selectedServerId.value === id) return
  selectedServerId.value = id
}

function openProxyDetail(proxy: FrpProxy, tab: 'overview' | 'logs' = 'overview') {
  detailProxy.value = proxy
  proxyDetailTab.value = tab
  showProxyDetail.value = true
  proxyRateHistory.value = []
  recordProxyRateSample()
  if (tab === 'logs') void loadProxyLogs(proxy.id)
}

function closeProxyDetail() {
  showProxyDetail.value = false
}

async function loadProxyLogs(proxyID: number) {
  proxyLogsLoading.value = true
  try {
    // 日志文件按时间升序返回，倒过来让最新一条在最上面，与「FRP 日志」页签的展示一致
    proxyLogLines.value = asList(await api.getFrpProxyLogs(proxyID)).reverse()
  }
  catch (error) { message.error(error instanceof Error ? error.message : '读取规则日志失败') }
  finally { proxyLogsLoading.value = false }
}

function proxyDetailServerName(proxy: FrpProxy) {
  return servers.value.find((server) => server.id === proxy.server_id)?.name ?? `#${proxy.server_id}`
}

function proxyDetailValue(proxy: FrpProxy, key: 'local' | 'public' | 'transport' | 'health' | 'lb') {
  switch (key) {
    case 'local': return proxyLocalTarget(proxy)
    case 'public': return proxyPublicEndpoint(proxy)
    case 'transport': { const tags = proxyTransportTags(proxy); return tags.length ? '' : '标准传输' }
    case 'health': return proxy.options?.health_check?.type ? proxy.options.health_check.type.toUpperCase() : '未配置'
    case 'lb': return proxy.options?.load_balancer?.group ?? '未配置'
  }
}
function resetProxyForm() {
  proxyFormTab.value = 'basic'
  Object.assign(proxyForm, { server_id: selectedServerId.value ?? servers.value.find((server) => server.enabled)?.id ?? servers.value[0]?.id ?? 0, name: '', type: 'tcp', local_ip: '127.0.0.1', local_port: 80, remote_port: 0, domains_text: '', locations_text: '', allow_users_text: '', metadatas_text: '', request_headers_text: '', response_headers_text: '', host_header_rewrite: '', enabled: true, remark: '', plugin_type: '', plugin_local_addr: '', plugin_unix_path: '', plugin_local_path: '', plugin_username: '', plugin_password: '' })
  Object.assign(proxyForm.options, defaultProxyOptions())
}

function openProxyModal(proxy?: FrpProxy) {
  proxyFormTab.value = 'basic'
  editingProxy.value = proxy ?? null
  resetProxyForm()
  if (proxy) {
    Object.assign(proxyForm, { ...proxy, remote_port: proxy.remote_port ?? 0, domains_text: proxy.custom_domains.join('\n'), locations_text: proxy.options?.locations?.join('\n') ?? '', allow_users_text: proxy.options?.allow_users?.join(',') ?? '', metadatas_text: mapToText(proxy.options?.metadatas), request_headers_text: mapToText(proxy.options?.request_headers), response_headers_text: mapToText(proxy.options?.response_headers), plugin_type: proxy.options?.plugin?.type ?? '', plugin_local_addr: proxy.options?.plugin?.local_addr ?? '', plugin_unix_path: proxy.options?.plugin?.unix_path ?? '', plugin_local_path: proxy.options?.plugin?.local_path ?? '', plugin_username: proxy.options?.plugin?.username ?? '', plugin_password: proxy.options?.plugin?.password ?? '', allow_ips_text: proxy.options?.allow_ips?.join(',') ?? '' })
    Object.assign(proxyForm.options, defaultProxyOptions(), proxy.options ?? {})
    proxyForm.options.transport = { ...defaultProxyOptions().transport, ...(proxy.options?.transport ?? {}) }
    proxyForm.options.health_check = { ...defaultProxyOptions().health_check, ...(proxy.options?.health_check ?? {}) }
    proxyForm.options.load_balancer = { ...defaultProxyOptions().load_balancer, ...(proxy.options?.load_balancer ?? {}) }
    proxyForm.options.nat_traversal = { disable_assisted_addrs: proxy.options?.nat_traversal?.disable_assisted_addrs ?? false }
  }
  showProxyModal.value = true
}

async function load() {
  loadError.value = ''
  try {
    const [nextStatus, nextServers, nextProxies] = await Promise.all([api.getFrpStatus(), api.listFrpServers(), api.listFrpProxies()])
    status.value = nextStatus
    servers.value = asList(nextServers)
    proxies.value = asList(nextProxies)
    // 选中服务端失效（被删 / 首次进入）时回落到第一台，右侧表格不空
    if (selectedServerId.value === null || !servers.value.some((item) => item.id === selectedServerId.value)) {
      selectedServerId.value = servers.value[0]?.id ?? null
    }
    void loadProxyTraffic()
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '加载 FRP 配置失败'
  }
}

async function saveProxy() {
  const server = servers.value.find((item) => item.id === proxyForm.server_id)
  if (!server) return
  savingProxy.value = true
  try {
    const payload = buildProxyPayload(server.id)
    if (editingProxy.value) await api.updateFrpProxy(editingProxy.value.id, payload)
    else await api.createFrpProxy(payload)
    showProxyModal.value = false
    message.success('穿透规则已保存')
    await load()
  } catch (error) { message.error(error instanceof Error ? error.message : '保存失败') }
  finally { savingProxy.value = false }
}

function buildProxyPayload(serverID: number): FrpProxyPayload {
  const domains = proxyForm.domains_text.split(/[\n,\s]+/).map((item) => item.trim()).filter(Boolean)
  // allow_ips 由「公网反代」页管理：编辑时保留既有值，不再从弹窗文本读取
  const { allow_ips_text: _ignored, ...restOptions } = proxyForm.options as FrpProxyOptions & { allow_ips_text?: string }
  const options: FrpProxyOptions = { ...restOptions, allow_ips: restOptions.allow_ips ?? [], locations: splitText(proxyForm.locations_text), allow_users: splitText(proxyForm.allow_users_text), metadatas: textToMap(proxyForm.metadatas_text), request_headers: textToMap(proxyForm.request_headers_text), response_headers: textToMap(proxyForm.response_headers_text), plugin: buildPluginOptions() }
  return { server_id: serverID, name: proxyForm.name, type: proxyForm.type, local_ip: proxyForm.local_ip, local_port: proxyForm.local_port, remote_port: ['tcp', 'udp'].includes(proxyForm.type) ? proxyForm.remote_port : null, custom_domains: domains, host_header_rewrite: proxyForm.host_header_rewrite, options, enabled: proxyForm.enabled, remark: proxyForm.remark }
}

function buildPluginOptions(): FrpProxyOptions['plugin'] {
  if (!proxyForm.plugin_type) return undefined
  const plugin = { type: proxyForm.plugin_type, local_addr: proxyForm.plugin_local_addr, unix_path: proxyForm.plugin_unix_path, local_path: proxyForm.plugin_local_path, username: proxyForm.plugin_username, password: proxyForm.plugin_password } as NonNullable<FrpProxyOptions['plugin']>
  if (proxyForm.plugin_type === 'http_proxy' || proxyForm.plugin_type === 'static_file') {
    plugin.http_user = proxyForm.plugin_username
    plugin.http_password = proxyForm.plugin_password
  }
  return plugin
}

function splitText(value: string) { return value.split(/[\n,\s]+/).map((item) => item.trim()).filter(Boolean) }
function textToMap(value: string) { return Object.fromEntries(value.split('\n').map((line) => line.split('=').map((item) => item.trim())).filter(([key, item]) => key && item)) }
function mapToText(value?: Record<string, string>) { return Object.entries(value ?? {}).map(([key, item]) => `${key}=${item}`).join('\n') }

async function runAction(action: 'start' | 'stop' | 'reload') {
  actionLoading.value = action
  try {
    const methods = { start: api.startFrp, stop: api.stopFrp, reload: api.reloadFrp }
    status.value = await methods[action]()
    message.success(action === 'start' ? 'frpc 已启动' : action === 'stop' ? 'frpc 已停止' : '配置已应用')
  } catch (error) { message.error(error instanceof Error ? error.message : '操作失败') }
  finally { actionLoading.value = null; await load() }
}

function startClient() { return runAction('start') }
function stopClient() { return runAction('stop') }
function reloadClient() { return runAction('reload') }

async function openRuntimeModal() {
  showRuntimeModal.value = true
  await loadRuntimeConfig()
}

async function loadRuntimeConfig() {
  runtimeLoading.value = true
  try {
    const [nextRuntime, nextBinaries, nextReleases, nextDownloadTask] = await Promise.all([api.getFrpRuntime(), api.listFrpBinaries(), api.listFrpReleases(), api.getFrpDownloadStatus()])
    runtime.value = nextRuntime
    downloadProxy.value = nextRuntime.download_proxy
    binaries.value = asList(nextBinaries)
    releases.value = asList(nextReleases)
    downloadTask.value = nextDownloadTask.status === 'completed' ? emptyDownloadTask() : nextDownloadTask
    downloadPolling.value = downloadInProgress.value
  } catch (error) { message.error(error instanceof Error ? error.message : '读取 FRP 应用配置失败') }
  finally { runtimeLoading.value = false }
}

async function saveDownloadProxy() {
  savingDownloadProxy.value = true
  try {
    runtime.value = await api.setFrpDownloadProxy(downloadProxy.value)
    message.success(downloadProxy.value ? '下载加速代理已保存' : '已切换为直连官方下载')
  } catch (error) { message.error(error instanceof Error ? error.message : '保存下载代理失败') }
  finally { savingDownloadProxy.value = false }
}

function applyRuntimeConfig() { return reloadClient() }

function isDownloaded(version: string) {
  return binaries.value.some((binary) => binary.version === version)
}

function confirmDownload(release: FrpRelease) {
  const method = downloadProxy.value ? `将通过 ${downloadProxyLabel()} 加速` : '将直连 GitHub 官方 Release'
  dialog.warning({ title: '下载 FRP 客户端', content: `${method} 下载 ${release.version}，并校验 SHA-256。是否继续？`, positiveText: '下载', negativeText: '取消', onPositiveClick: () => downloadRelease(release.version) })
}

function downloadProxyLabel() {
  return downloadProxyOptions.find((item) => item.value === downloadProxy.value)?.label ?? '所选代理'
}

async function downloadRelease(version: string) {
  downloadingVersion.value = version
  try {
    downloadTask.value = await api.downloadFrpRelease(version)
    downloadPolling.value = downloadInProgress.value
  } catch (error) { message.error(error instanceof Error ? error.message : '下载 FRP 客户端失败') }
  finally { downloadingVersion.value = null }
}

async function refreshDownloadTask() {
  try {
    downloadTask.value = await api.getFrpDownloadStatus()
    if (downloadInProgress.value) return
    downloadPolling.value = false
    if (downloadTask.value.status === 'completed') {
      message.success(`${downloadTask.value.version} 已下载并完成校验`)
      await loadRuntimeConfig()
      downloadTask.value = emptyDownloadTask()
      return
    }
    if (downloadTask.value.status === 'failed') message.error(downloadTask.value.error || '下载 FRP 客户端失败')
  } catch (error) {
    downloadPolling.value = false
    message.error(error instanceof Error ? error.message : '读取下载进度失败')
  }
}

function emptyDownloadTask(): FrpDownloadTask {
  return { status: 'idle', message: '', downloaded: 0, total: 0 }
}

function confirmActivate(version: string) {
  dialog.warning({ title: '启用 FRP 客户端', content: `将切换为 ${version}。若 frpc 正在运行，将自动重启以应用新版本。是否继续？`, positiveText: '启用', negativeText: '取消', onPositiveClick: () => activateBinary(version) })
}

async function activateBinary(version: string) {
  activatingVersion.value = version
  try {
    status.value = await api.activateFrpBinary(version)
    message.success(`${version} 已启用`)
    await loadRuntimeConfig()
  } catch (error) { message.error(error instanceof Error ? error.message : '启用 FRP 客户端失败') }
  finally { activatingVersion.value = null }
}

function confirmDeleteBinary(binary: FrpBinary) {
  dialog.warning({ title: '删除 FRP 客户端', content: `确定删除已下载的 ${binary.version} 吗？此操作不会影响当前正在使用的客户端。`, positiveText: '删除', negativeText: '取消', onPositiveClick: () => deleteBinary(binary.version) })
}

async function deleteBinary(version: string) {
  deletingVersion.value = version
  try {
    await api.deleteFrpBinary(version)
    message.success(`${version} 已删除`)
    await loadRuntimeConfig()
  } catch (error) { message.error(error instanceof Error ? error.message : '删除 FRP 客户端失败') }
  finally { deletingVersion.value = null }
}

function formatBinarySize(size: number) {
  if (size < 1024 * 1024) return `${Math.max(1, Math.round(size / 1024))} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

async function toggleProxy(proxy: FrpProxy, enabled: boolean) {
  try { await api.updateFrpProxy(proxy.id, { ...proxy, enabled }); await load() }
  catch (error) { message.error(error instanceof Error ? error.message : '更新失败') }
}

function confirmDeleteProxy(proxy: FrpProxy) {
  dialog.warning({ title: '删除规则', content: `确定删除「${proxy.name}」吗？`, positiveText: '删除', negativeText: '取消', onPositiveClick: async () => { try { await api.deleteFrpProxy(proxy.id); message.success('规则已删除'); await load() } catch (error) { message.error(error instanceof Error ? error.message : '删除失败') } } })
}

async function loadExport() {
  try { exportContent.value = (await api.exportFrpConfig()).content }
  catch (error) { message.error(error instanceof Error ? error.message : '导出失败') }
}

function proxyPublicEndpoint(proxy: FrpProxy) {
  if (proxy.type === 'tcp' || proxy.type === 'udp') return `:${proxy.remote_port ?? '-'}`
  if (proxy.type === 'stcp' || proxy.type === 'sudp' || proxy.type === 'xtcp') return '密钥访问'
  return proxy.custom_domains.join(', ') || proxy.options?.subdomain || '未设置入口'
}

/** 拉取穿透流量快照并按规则 ID 展开;失败的服务端错误记录到 proxyTrafficErrors */
async function loadProxyTraffic() {
  try {
    const payload: FrpProxiesTraffic = await api.listFrpProxiesTraffic()
    const next: Record<number, FrpProxyTraffic> = {}
    for (const [key, value] of Object.entries(payload.items ?? {})) {
      const id = Number(key)
      if (Number.isFinite(id)) next[id] = value
    }
    proxyTraffic.value = next
    const errors: Record<number, string> = {}
    for (const [key, value] of Object.entries(payload.errors ?? {})) {
      const id = Number(key)
      if (Number.isFinite(id)) errors[id] = value
    }
    proxyTrafficErrors.value = errors
  } catch {
    // 流量采集失败不打断列表刷新,保持上一次快照
  }
}

function proxyLocalTarget(proxy: FrpProxy) {
  // 插件规则由插件接管本地服务，localIP/localPort 不生效，展示插件类型更有意义
  const plugin = proxy.options?.plugin?.type
  if (plugin) return `插件 · ${plugin}`
  return `${proxy.local_ip}:${proxy.local_port}`
}

function proxyTransportTags(proxy: FrpProxy) {
  const tags: VNode[] = []
  const transport = proxy.options?.transport
  if (transport?.use_encryption) tags.push(h(NTag, { size: 'small', type: 'success', bordered: false, round: true }, () => '加密'))
  if (transport?.use_compression) tags.push(h(NTag, { size: 'small', type: 'info', bordered: false, round: true }, () => '压缩'))
  if (transport?.proxy_protocol_version) tags.push(h(NTag, { size: 'small', bordered: false, round: true }, () => `PP ${transport.proxy_protocol_version}`))
  if (transport?.bandwidth_limit) tags.push(h(NTag, { size: 'small', type: 'warning', bordered: false, round: true }, () => `限速 ${transport.bandwidth_limit}`))
  return tags
}

// 轮询走 useVisibilityPolling：标签页隐藏时暂停，回到前台补一次
useVisibilityPolling(() => load(), 5_000)

onMounted(async () => { await load() })

useVisibilityPolling(recordProxyRateSample, 5000, { enabled: proxyRatePolling })
useVisibilityPolling(refreshDownloadTask, 500, { enabled: downloadPolling })
</script>

<style scoped>
/* 统计区：对齐标准页（CertificatesView）的 .stats-row / .stat-card 规格 */
.stats-row { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--havline-space-4); margin-bottom: var(--havline-space-4); }
.stat-card { background: var(--havline-surface); border: 1px solid var(--havline-border); border-radius: var(--havline-radius); box-shadow: var(--havline-shadow); padding: var(--havline-space-4) var(--havline-space-5); min-height: 118px; }
.stat-card__icon { font-size: 18px; margin-bottom: 4px; }
.stat-card__icon--blue { color: var(--havline-info); }
.stat-card__icon--amber { color: var(--havline-warning); }
.stat-card__icon--green, .stat-card__icon--running { color: var(--havline-brand); }
.stat-card__icon--red { color: var(--havline-error); }
.stat-card__icon--stopped { color: var(--havline-disabled); }
.stat-card__icon--unknown { color: var(--havline-warning); }
.stat-card__value { margin-top: 4px; font-size: 30px; font-weight: 700; line-height: 1.1; color: var(--havline-text); letter-spacing: -0.03em; }
.stat-card__value--sm { font-size: 18px; }
.stat-card__label { margin-top: 8px; font-size: 13px; font-weight: 600; color: var(--havline-text); }
.stat-card__sub { margin-top: 4px; font-size: 12px; color: var(--havline-text-muted); }
/* 公网服务端列表：n-data-table（与规则表一致）；单元格由 render 创建，样式必须 :deep() */
.rule-detail__logs { max-height: 420px; }
.rule-detail__log-panel-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--havline-space-3); }
/* 详情弹窗概览区：与反代详情同版式 */
.overview-strip {
  display: grid;
  grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.8fr) minmax(0, 1.2fr) minmax(0, 1.4fr);
  gap: 12px 16px;
  padding: 12px 14px;
  background: var(--havline-bg);
  border: 1px solid var(--havline-border);
  border-radius: 10px;
  flex-shrink: 0;
}
.overview-strip__item { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.overview-strip__item--wide { grid-column: span 1; }
.overview-strip__label { font-size: 11px; color: var(--havline-text-muted); letter-spacing: 0.02em; }
.overview-strip__value { font-size: 13px; font-weight: 500; color: var(--havline-text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.overview-security { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 12px; padding: 10px 14px; background: var(--havline-bg); border: 1px solid var(--havline-border); border-radius: 10px; flex-shrink: 0; }
.overview-security__label { font-size: 11px; color: var(--havline-text-muted); letter-spacing: 0.02em; flex-shrink: 0; }
.overview-security__empty { font-size: 13px; color: var(--havline-text-muted); }
.overview-main { display: grid; grid-template-columns: minmax(0, 1.45fr) minmax(0, 1fr); gap: var(--havline-space-4); flex: 1 1 0; min-height: 0; align-items: stretch; }
.overview-side { display: flex; flex-direction: column; gap: 10px; min-height: 0; }
.overview-card { border: 1px solid var(--havline-border); border-radius: 10px; background: var(--havline-surface); padding: 14px 16px; }
.overview-card__head { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); margin-bottom: 10px; }
.overview-card__head h4 { margin: 0; font-size: 14px; font-weight: 600; }
.overview-card__sub { margin-left: 6px; font-size: 12px; font-weight: 400; color: var(--havline-text-muted); }
.overview-card--chart { display: flex; flex-direction: column; min-height: 0; }
.overview-chart { flex: 1; min-height: 180px; }
.overview-metrics { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); grid-template-rows: 1fr 1fr; gap: 10px; flex-shrink: 0; }
.overview-metric { display: flex; flex-direction: column; justify-content: center; padding: 12px 14px; border-radius: 10px; background: var(--havline-bg); border: 1px solid var(--havline-border); }
.overview-metric__label { font-size: 12px; color: var(--havline-text-muted); }
.overview-metric__value { margin-top: 4px; font-size: 18px; font-weight: 600; line-height: 1.2; color: var(--havline-text); }
.live-badge { display: inline-flex; align-items: center; gap: 5px; font-size: 12px; color: var(--havline-brand); font-weight: 500; }
.live-badge__dot { width: 6px; height: 6px; border-radius: 50%; background: var(--havline-brand); animation: live-pulse 1.5s ease-in-out infinite; }
@keyframes live-pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }
.traffic-live-legend { display: flex; flex-wrap: wrap; gap: 14px; margin-bottom: 8px; font-size: 12px; color: var(--havline-text-secondary); }
.traffic-live-legend--compact { margin-bottom: 6px; }
.traffic-live-legend__item { display: inline-flex; align-items: center; gap: 6px; }
.traffic-live-legend__dot { width: 8px; height: 8px; border-radius: 50%; }
.traffic-live-legend__item--up .traffic-live-legend__dot { background: var(--havline-brand); }
.traffic-live-legend__item--down .traffic-live-legend__dot { background: var(--havline-info); }
.proxy-detail-modal__conn { display: inline-flex; align-items: center; gap: 4px; padding: 2px 10px; border-radius: 999px; font-size: 12px; font-weight: 500; color: var(--havline-text-muted); background: var(--havline-bg); border: 1px solid var(--havline-border); }
.proxy-detail-modal__conn.is-active { color: var(--havline-brand-text); background: var(--havline-brand-soft); border-color: var(--havline-brand-soft); }
/* 穿透规则详情弹窗：反代详情的样式类定义在 ProxyView 的 scoped 样式中,对 FrpView 不生效,此处本地定义 */
.frp-rule-detail-modal {
  width: min(920px, 96vw);
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  background: var(--havline-surface);
  border-radius: var(--havline-radius);
  overflow: hidden;
  box-shadow: var(--havline-shadow-md);
}
.frp-rule-detail-modal .proxy-detail-modal__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--havline-space-4);
  padding: var(--havline-space-5) var(--havline-space-5) 0;
  flex-shrink: 0;
}
.frp-rule-detail-modal .proxy-detail-modal__intro { display: flex; align-items: center; gap: var(--havline-space-3); min-width: 0; }
.frp-rule-detail-modal .proxy-detail-modal__title { margin: 0; font-size: 18px; font-weight: 600; }
.frp-rule-detail-modal .proxy-detail__tabbar {
  display: flex;
  gap: var(--havline-space-5);
  padding: var(--havline-space-3) var(--havline-space-5) 0;
  border-bottom: 1px solid var(--havline-border);
  flex-shrink: 0;
}
.frp-rule-detail-modal .proxy-detail__tab {
  margin: 0 0 -1px;
  padding: 8px 2px 10px;
  border: none;
  background: none;
  font: inherit;
  font-size: 14px;
  color: var(--havline-text-secondary);
  cursor: pointer;
  border-bottom: 2px solid transparent;
}
.frp-rule-detail-modal .proxy-detail__tab:hover { color: var(--havline-text); }
.frp-rule-detail-modal .proxy-detail__tab--active { color: var(--havline-brand-text); font-weight: 600; border-bottom-color: var(--havline-brand); }
.frp-rule-detail-modal .proxy-detail__scroll {
  flex: 1;
  min-height: min(480px, 60vh);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.frp-rule-detail-modal .proxy-detail__pane {
  width: 100%;
  box-sizing: border-box;
  padding: var(--havline-space-4) var(--havline-space-5) var(--havline-space-5);
}
.frp-rule-detail-modal .proxy-detail__pane:first-of-type {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--havline-space-4);
}
.frp-proxy-modal { display: flex; width: 900px; max-width: 95vw; max-height: 90vh; background: var(--havline-surface); border-radius: var(--havline-radius); overflow: hidden; box-shadow: var(--havline-shadow-md); }
.frp-proxy-modal .proxy-modal__form { flex: 1; min-width: 0; display: flex; flex-direction: column; max-height: 90vh; }
.frp-proxy-modal .proxy-modal__header { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); padding: var(--havline-space-5) var(--havline-space-5) 0; }
.frp-proxy-modal .modal-title { margin: 0; font-size: 18px; font-weight: 600; }
.frp-proxy-modal .proxy-modal__tabbar { display: flex; gap: var(--havline-space-5); padding: var(--havline-space-3) var(--havline-space-5) 0; border-bottom: 1px solid var(--havline-border); flex-shrink: 0; }
.frp-proxy-modal .proxy-modal__tab { margin: 0 0 -1px; padding: 8px 2px 10px; border: 0; border-bottom: 2px solid transparent; background: none; color: var(--havline-text-secondary); font: inherit; font-size: 14px; cursor: pointer; }
.frp-proxy-modal .proxy-modal__tab:hover { color: var(--havline-text); }
.frp-proxy-modal .proxy-modal__tab--active { color: var(--havline-brand-text); font-weight: 600; border-bottom-color: var(--havline-brand); }
.frp-proxy-modal .proxy-modal__scroll { flex: 1; min-height: 0; overflow-y: auto; }
.frp-proxy-modal .proxy-modal__pane { width: 100%; box-sizing: border-box; padding: var(--havline-space-4) var(--havline-space-5) 0; }
.frp-proxy-modal .proxy-modal__pane > .modal-actions { padding-bottom: var(--havline-space-5); }
.frp-proxy-modal__basic .form-grid { grid-template-columns: 1fr; }
.frp-server-modal__basic .form-grid { grid-template-columns: 1fr; }
.frp-proxy-modal .proxy-form-section { margin-top: var(--havline-space-2); padding: var(--havline-space-4) 0 0; border-top: 1px solid var(--havline-border); }
.frp-proxy-modal .proxy-form-section h4 { margin: 0 0 var(--havline-space-4); font-size: 14px; font-weight: 600; color: var(--havline-text); }
.frp-proxy-modal__advanced { padding-bottom: var(--havline-space-4); }
.frp-proxy-modal .proxy-advanced-collapse { width: 100%; border: 1px solid var(--havline-border); border-radius: 10px; overflow: hidden; background: var(--havline-surface); }
.frp-proxy-modal .proxy-advanced-collapse :deep(.n-collapse-item) { margin: 0 !important; border: 0 !important; border-radius: 0 !important; }
.frp-proxy-modal .proxy-advanced-collapse :deep(.n-collapse-item + .n-collapse-item) { border-top: 1px solid var(--havline-border) !important; }
.frp-proxy-modal .proxy-advanced-collapse :deep(.n-collapse-item__header) { min-height: 44px; padding: 12px 14px !important; background: var(--havline-bg); font-weight: 600; box-sizing: border-box; }
.frp-proxy-modal .proxy-advanced-collapse :deep(.n-collapse-item__content-inner) { padding: 0 14px 14px; }
.frp-proxy-modal .proxy-modal__help { width: 300px; flex-shrink: 0; padding: var(--havline-space-5); background: var(--havline-bg-muted); border-left: 1px solid var(--havline-border); color: var(--havline-text-secondary); font-size: 13px; line-height: 1.65; overflow: auto; }
.frp-proxy-modal .proxy-modal__help h4 { margin: 0 0 var(--havline-space-3); color: var(--havline-text); font-size: 14px; font-weight: 600; }
.frp-proxy-modal .proxy-modal__help ol { margin: 0; padding-left: 18px; }
.frp-proxy-modal .proxy-modal__help li + li { margin-top: 10px; }
.frp-proxy-modal .proxy-modal__help strong { color: var(--havline-text); font-weight: 600; }
.frp-proxy-modal .proxy-modal__tip { display: flex; align-items: flex-start; gap: 8px; margin-top: var(--havline-space-5); padding: 12px; border-radius: 8px; background: var(--havline-info-soft); color: var(--havline-info); font-size: 12px; line-height: 1.6; }
.frp-proxy-modal .proxy-modal__tip-icon { flex-shrink: 0; margin-top: 1px; font-size: 16px; }
.frp-proxy-modal .proxy-modal__example { margin-top: var(--havline-space-5); padding-top: var(--havline-space-4); border-top: 1px solid var(--havline-border); }
.frp-proxy-modal .proxy-modal__example-header { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--havline-space-3); }
.frp-proxy-modal .proxy-modal__example h5 { margin: 0; color: var(--havline-text); font-size: 13px; font-weight: 600; }
.frp-proxy-modal .proxy-modal__example p { margin: 4px 0 0; font-size: 12px; line-height: 1.55; }
.frp-proxy-modal .proxy-modal__example pre { margin: var(--havline-space-3) 0 0; padding: 10px; overflow-x: auto; border-radius: 6px; background: var(--havline-bg-muted); color: var(--havline-text); font: 12px/1.6 var(--havline-mono); white-space: pre; }
.form-section { padding: var(--havline-space-4) 0; border-bottom: 1px solid var(--havline-border); }
.form-section:first-child { padding-top: 0; }
.form-section:last-of-type { border-bottom: 0; }
.form-section h3 { display: flex; align-items: center; gap: 8px; margin: 0 0 var(--havline-space-4); font-size: 16px; }
.form-section h3 :deep(.n-icon) { display: grid; place-items: center; width: 26px; height: 26px; border-radius: var(--havline-radius-sm); background: var(--havline-brand); color: #fff; font-size: 15px; }
.form-hint { margin: 0 0 var(--havline-space-4); color: var(--havline-text-secondary); font-size: 13px; line-height: 1.55; }
/* 统一卡片式弹窗外壳（运行日志 / 导出配置 / frps 同源配置）：与 .proxy-modal 体系一致的头部 + 滚动区 + 页脚 */
.frp-card-modal { display: flex; flex-direction: column; width: min(860px, calc(100vw - 32px)); max-height: 90vh; background: var(--havline-surface); border-radius: var(--havline-radius); overflow: hidden; box-shadow: var(--havline-shadow-md); }
.frp-card-modal .proxy-modal__header { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); padding: var(--havline-space-5) var(--havline-space-5) 0; flex-shrink: 0; }
.frp-card-modal .modal-title { margin: 0; font-size: 18px; font-weight: 600; color: var(--havline-text); }
.frp-card-modal .proxy-modal__scroll { flex: 1; min-height: 0; overflow-y: auto; padding: var(--havline-space-4) var(--havline-space-5) 0; }
.frp-card-modal .modal-footer { display: flex; justify-content: flex-end; gap: var(--havline-space-3); padding: var(--havline-space-4) var(--havline-space-5) var(--havline-space-5); border-top: 1px solid var(--havline-border); margin-top: var(--havline-space-2); flex-shrink: 0; }
.frp-runtime-modal { width: min(880px, 92vw); }
/* 服务端详情弹窗（仍用 naive-ui preset=card）：排版对齐 .proxy-modal */

.frp-guide-modal { display: flex; width: 900px; max-width: 95vw; max-height: 90vh; background: var(--havline-surface); border-radius: var(--havline-radius); overflow: hidden; box-shadow: var(--havline-shadow-md); }
.frp-guide-modal .proxy-modal__form { flex: 1; min-width: 0; display: flex; flex-direction: column; max-height: 90vh; }
.frp-guide-modal .proxy-modal__header { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); padding: var(--havline-space-5) var(--havline-space-5) 0; }
.frp-guide-modal .modal-title { margin: 0; font-size: 18px; font-weight: 600; }
.frp-guide-modal .proxy-modal__tabbar { display: flex; gap: var(--havline-space-5); padding: var(--havline-space-3) var(--havline-space-5) 0; border-bottom: 1px solid var(--havline-border); flex-shrink: 0; }
.frp-guide-modal .proxy-modal__tab { margin: 0 0 -1px; padding: 8px 2px 10px; border: 0; border-bottom: 2px solid transparent; background: none; color: var(--havline-text-secondary); font: inherit; font-size: 14px; cursor: pointer; }
.frp-guide-modal .proxy-modal__tab:hover { color: var(--havline-text); }
.frp-guide-modal .proxy-modal__tab--active { color: var(--havline-brand-text); font-weight: 600; border-bottom-color: var(--havline-brand); }
.frp-guide-modal .proxy-modal__scroll { flex: 1; min-height: 0; overflow-y: auto; }
.frp-guide-modal .proxy-modal__pane { width: 100%; box-sizing: border-box; padding: var(--havline-space-4) var(--havline-space-5) var(--havline-space-5); }
@media (max-width: 760px) { .frp-guide-modal { display: block; max-height: 92vh; overflow-y: auto; } }

.guide-section { margin-bottom: 18px; }
.guide-section h4 { margin: 0 0 8px; font-size: 14px; }
.guide-section ol, .guide-section ul { margin: 0; padding-left: 20px; display: flex; flex-direction: column; gap: 6px; font-size: 13px; line-height: 1.7; }
.guide-section code { padding: 1px 5px; border-radius: 4px; background: var(--havline-bg-muted); font-size: 12px; }
.guide-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.guide-table th, .guide-table td { border: 1px solid var(--havline-border); padding: 6px 10px; text-align: left; line-height: 1.6; }
.guide-table th { font-weight: 600; }
.guide-note { margin: 6px 0; font-size: 12.5px; color: var(--havline-text-secondary); line-height: 1.6; }
.guide-topo { margin: 8px 0; padding: 10px 12px; border-radius: 6px; background: var(--havline-bg-muted); font-size: 12px; line-height: 1.6; overflow-x: auto; }
.guide-steps { margin: 8px 0; padding-left: 20px; display: flex; flex-direction: column; gap: 5px; font-size: 13px; line-height: 1.7; }
.guide-collapse :deep(.n-collapse-item__header) { font-size: 13px; font-weight: 500; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 var(--havline-space-4); }
.form-grid--wide > :first-child { grid-column: 1 / -1; }
.advanced-collapse { margin-top: var(--havline-space-3); }
.switch-row, .modal-actions { display: flex; align-items: center; gap: var(--havline-space-2); flex-wrap: wrap; }
.switch-row { padding: 4px 0 var(--havline-space-5); }
.switch-row span:nth-of-type(2) { margin-left: var(--havline-space-4); }
.modal-actions { justify-content: flex-end; }
.logs-box { margin: var(--havline-space-3) 0 0; padding: var(--havline-space-4); min-height: 260px; max-height: 480px; overflow: auto; border: 1px solid var(--havline-border); background: var(--havline-bg-muted); color: var(--havline-text); white-space: pre-wrap; word-break: break-word; font: 12px/1.55 var(--havline-mono); }
.error-card__content { color: var(--havline-error); white-space: pre-wrap; }
.table-muted { margin-top: 4px; color: var(--havline-text-muted); font-size: 12px; }
/* 规则表单元格由 render 函数的 h() 创建，不带 scoped 属性，必须 :deep() 才能命中 */
.runtime-actions, .runtime-section__title, .binary-row, .binary-row__actions { display: flex; align-items: center; gap: var(--havline-space-3); }
.runtime-actions { justify-content: flex-end; margin-bottom: var(--havline-space-4); }
.runtime-section { padding: var(--havline-space-4) 0; border-top: 1px solid var(--havline-border); }
.runtime-section:first-of-type { border-top: 0; padding-top: 0; }
.runtime-section h3 { margin: 0; font-size: 15px; }
.runtime-section__title { justify-content: space-between; margin-bottom: var(--havline-space-3); }
.runtime-section__title span, .binary-row__meta { color: var(--havline-text-secondary); font-size: 12px; word-break: break-all; }
.download-proxy-form { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: var(--havline-space-3); align-items: center; }
.binary-list, .release-list { display: grid; gap: var(--havline-space-2); }
.release-list__more { display: flex; justify-content: center; margin-top: var(--havline-space-3); }
.binary-row { justify-content: space-between; padding: var(--havline-space-3); border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); }
.release-row { display: grid; grid-template-columns: minmax(180px, 1fr) minmax(240px, 320px) auto; align-items: center; gap: var(--havline-space-3); padding: var(--havline-space-3); border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); }
.release-row__info { min-width: 0; }
.binary-row__actions { flex-shrink: 0; }
.release-progress, .release-progress-placeholder { min-width: 0; }
.release-progress { display: grid; grid-template-columns: minmax(0, 110px) minmax(80px, 1fr) auto; align-items: center; gap: var(--havline-space-2); color: var(--havline-text-secondary); font-size: 12px; white-space: nowrap; }
.release-progress__message { overflow: hidden; text-overflow: ellipsis; }
.release-progress__bar { min-width: 0; }
.release-progress__value { font-variant-numeric: tabular-nums; }
.release-progress--failed { color: var(--havline-error); grid-template-columns: minmax(0, 1fr); }
.latency-panel__hint { color: var(--havline-text-muted); font-size: 12px; }
.detail-status-meta { display: flex; justify-content: space-between; gap: var(--havline-space-3); margin: -4px 0 var(--havline-space-3); color: var(--havline-text-muted); font-size: 12px; }
.detail-status-error { color: var(--havline-error); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.server-status-panel .detail-status-meta { margin: var(--havline-space-4) 0 0; }
.latency-chart { width: 100%; height: 240px; }
.detail-empty { display: grid; place-items: center; height: 240px; color: var(--havline-text-muted); border: 1px dashed var(--havline-border); }
.detail-legend-failed { display: inline-block; width: 6px; height: 6px; margin: 0 4px 1px 12px; border-radius: 50%; background: var(--havline-error); }
.latency-bars { display: flex; align-items: end; gap: 4px; height: 92px; padding: 8px 0; border-bottom: 1px solid var(--havline-border); }
.latency-bars span { flex: 1; min-width: 5px; max-width: 24px; border-radius: 3px 3px 0 0; background: linear-gradient(180deg, var(--havline-success), color-mix(in srgb, var(--havline-success) 55%, white)); }
.latency-bars span.latency-bars__item--bad { background: var(--havline-warning); }
.detail-chart-footer { display: flex; justify-content: space-between; margin-top: 8px; color: var(--havline-text-muted); font-size: 12px; }
.detail-chart-footer i { display: inline-block; width: 6px; height: 6px; margin-right: 4px; border-radius: 50%; background: var(--havline-success); }
.detail-status--on { color: var(--havline-success); }
.detail-status--off { color: var(--havline-text-muted); }
.detail-logs { margin: 0; max-height: 220px; overflow: auto; padding: var(--havline-space-3); background: var(--havline-bg-muted); color: var(--havline-text-secondary); white-space: pre-wrap; word-break: break-word; font: 12px/1.55 var(--havline-mono); }
@media (max-width: 1100px) { .stats-row { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 700px) { .stats-row, .form-grid, .download-proxy-form, .server-detail__grid, .detail-fields, .detail-stats, .server-status-summary { grid-template-columns: 1fr; } .server-status-summary__item--state { min-height: 0; padding-right: 0; padding-bottom: var(--havline-space-3); border-right: 0; border-bottom: 1px solid var(--havline-border); } .status-bars { gap: 3px; } .status-bar { height: 28px; } .binary-row { align-items: flex-start; flex-direction: column; } .release-row { grid-template-columns: 1fr; } .release-progress-placeholder { display: none; } .switch-row span:nth-of-type(2) { margin-left: 0; } }
@media (max-width: 760px) { .frp-proxy-modal { display: block; max-height: 92vh; overflow-y: auto; } .frp-proxy-modal .proxy-modal__form { max-height: none; } .frp-proxy-modal .proxy-modal__scroll { overflow: visible; } .frp-proxy-modal .proxy-modal__help { width: auto; border-top: 1px solid var(--havline-border); border-left: 0; } }
.frps-compare--warn {
  color: var(--havline-warning);
}
/* 左：服务端列表；右：该服务端的穿透规则（与「公网服务端」页 .agent-layout 同构） */
.frp-layout { display: grid; grid-template-columns: minmax(240px, 300px) minmax(0, 1fr); gap: var(--havline-space-4); align-items: start; }
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

.rule-pane { min-width: 0; display: flex; flex-direction: column; gap: var(--havline-space-4); }
.rule-pane__head { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--havline-space-3); padding: var(--havline-space-3) var(--havline-space-4); background: var(--havline-surface); border: 1px solid var(--havline-border); border-radius: var(--havline-radius); box-shadow: var(--havline-shadow); }
.rule-pane__intro { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--havline-space-3); min-width: 0; }
.rule-pane__title { margin: 0; font-size: 15px; font-weight: 600; color: var(--havline-text); }
.rule-pane__sub { font-size: 12.5px; color: var(--havline-text-secondary); }
.rule-pane__actions { display: flex; align-items: center; flex-wrap: wrap; gap: var(--havline-space-2); }
.rule-table-wrap { border: 1px solid var(--havline-border); border-radius: var(--havline-radius); background: var(--havline-surface); box-shadow: var(--havline-shadow); overflow: hidden; padding: var(--havline-space-2) 0; }
.frp-rules-table :deep(.table-muted) { color: var(--havline-text-muted); font-size: 12px; }
.frp-rules-table :deep(.mono) { font-family: var(--havline-mono); font-size: 13px; }
.frp-rules-table :deep(.text-secondary) { color: var(--havline-text-secondary); }
@media (max-width: 1100px) { .frp-layout { grid-template-columns: 1fr; } .server-sidebar__list { max-height: none; } }
</style>
