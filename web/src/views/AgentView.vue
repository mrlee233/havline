<template>
  <PageHeader title="公网服务端" :description="headerDescription">
    <template #actions>
      <n-button :loading="statusLoading" :disabled="!server" @click="refreshAll({ silent: false })">
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
      description="添加公网 VPS 的 frps 地址与认证 Token；填好「公网 Agent 管理」区的 Agent 地址与 SSH 信息后，可直接在本页安装与管理 Agent。"
    >
      <template #action>
        <n-button type="primary" @click="openServerModal()">添加服务端</n-button>
      </template>
    </EmptyState>

    <template v-else-if="server">
      <!-- 左列表 + 右详情：与 DDNS 页（.ddns-layout / .task-sidebar）同构 -->
      <div class="agent-layout">
        <aside class="server-sidebar">
          <div class="server-sidebar__head">
            <div class="server-sidebar__head-row">
              <h3 class="server-sidebar__title">服务端列表</h3>
              <n-button size="small" type="primary" @click="openServerModal()">
                <template #icon><n-icon :component="AddOutline" /></template>
                添加服务端
              </n-button>
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
                <span :title="serverStateTooltip(item)">
                  <n-tag size="tiny" :bordered="false" :type="serverStartupType(item)">{{ serverStartupText(item) }} · {{ connectionStateText(item) }}</n-tag>
                </span>
              </div>
              <div class="server-item__endpoint mono">{{ item.server_addr }}:{{ item.server_port }}</div>
              <div class="server-item__meta">
                <span :title="latencyDetail(item)"><n-icon :component="FlashOutline" /> {{ latencyText(item) }}</span>
                <span><n-icon :component="LocationOutline" /> {{ locationText(item) }}</span>
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

        <div class="agent-detail">
          <!-- 选中服务端的操作：原列表行内操作移到右侧工具条 -->
          <div class="server-toolbar">
            <div class="server-toolbar__info">
              <span class="server-toolbar__name">{{ server.name }}</span>
              <span class="server-toolbar__endpoint mono">{{ server.server_addr }}:{{ server.server_port }}</span>
            </div>
            <div class="server-toolbar__actions">
              <n-button v-if="server.boot_status === 'running'" size="small" type="error" ghost :loading="serverStartingId === server.id" @click="stopServer(server)">停止</n-button>
              <n-button v-else size="small" type="success" ghost :loading="serverStartingId === server.id" @click="startServer(server)">启动</n-button>
              <n-button size="small" @click="openServerModal(server)">编辑</n-button>
              <n-dropdown trigger="click" :options="serverMoreOptions" @select="handleServerMoreSelect">
                <n-button size="small" quaternary>
                  <template #icon><n-icon :component="EllipsisHorizontalOutline" /></template>
                  更多
                </n-button>
              </n-dropdown>
            </div>
          </div>

          <!-- 状态总览：参考内网穿透页的状态卡，先回答“Agent 是否连通、frps 是否正常” -->
          <HavlineCard class="section-card agent-hero-card" title="Agent 状态">
            <div class="agent-hero" :class="agentHeroClass">
              <div class="agent-hero__main">
                <div class="agent-hero__icon">
                  <n-icon :component="CloudOutline" />
                </div>
                <div class="agent-hero__text">
                  <div class="agent-hero__title">{{ agentHeroTitle }}</div>
                  <div class="agent-hero__sub">{{ agentHeroSubtitle }}</div>
                </div>
              </div>
              <div class="agent-hero__aside">
                <StatusBadge :kind="agentStatusOk ? 'success' : 'warning'" :text="agentStatusOk ? '在线' : agentConnectionText" />
                <span class="agent-hero__time">
                  {{ agentStatus?.checked_at ? `检查于 ${formatRelativeTime(agentStatus.checked_at)}` : '尚未检测' }}
                </span>
              </div>
            </div>
            <div class="agent-hero__flow">
              <span class="agent-hero__flow-step"><n-icon :component="ServerOutline" /> 本地 Havline</span>
              <span class="agent-hero__flow-arrow">→</span>
              <span class="agent-hero__flow-step agent-hero__flow-step--accent"><n-icon :component="SwapHorizontalOutline" /> {{ agentTransportText(server) }}</span>
              <span class="agent-hero__flow-arrow">→</span>
              <span class="agent-hero__flow-step"><n-icon :component="CloudOutline" /> VPS Agent</span>
              <span class="agent-hero__flow-arrow">→</span>
              <span class="agent-hero__flow-step"><n-icon :component="GitNetworkOutline" /> frps / Nginx</span>
            </div>
            <div class="agent-hero__metrics">
              <div><span>Agent 连接</span><strong>{{ agentConnectionText }}</strong></div>
              <div><span>frps 服务</span><strong>{{ frpsStateText }}</strong></div>
              <div><span>反代路由</span><strong>{{ routesCount }} 条</strong></div>
              <div><span>frps 版本</span><strong>{{ installedFrpsVersion || '未安装' }}</strong></div>
            </div>
            <div class="agent-hero__ports">
              <span class="agent-hero__ports-label">端口监听</span>
              <span
                v-for="port in watchedPorts"
                :key="port"
                class="port-item"
                :class="{ 'port-item--on': portOn(port) }"
                :title="portMeaning(port)"
              >
                {{ port }}
                <n-icon :component="portOn(port) ? CheckmarkCircleOutline : CloseCircleOutline" />
              </span>
            </div>
          </HavlineCard>

      <!-- 状态提示：互斥，最多一条 -->
      <n-alert v-if="statusAlert" :type="statusAlert.type" :bordered="false" class="page-alert">
        <div class="page-alert__body">
          <span>{{ statusAlert.text }}</span>
          <n-button v-if="statusAlert.action" size="small" :disabled="busy !== null" @click="statusAlert.action.run()">
            {{ statusAlert.action.label }}
          </n-button>
        </div>
      </n-alert>

      <!-- 入口行：公网反代（不占 alert 位） -->
      <div class="entrance-row">
        <span class="entrance-row__text">
          已注册公网反代 <strong>{{ routesCount }}</strong> 条；逐条部署、配置查看与访问控制都在「公网反代」页
        </span>
        <router-link to="/public-proxy"><n-button size="small" type="primary">前往公网反代</n-button></router-link>
      </div>

      <!-- 主区页签：概览 / 环境与依赖 / 运行控制 / 日志与输出 -->
      <n-tabs v-model:value="activeTab" type="line" animated class="agent-tabs">
        <n-tab-pane name="overview" tab="概览">
          <section class="stats-row" aria-label="运行状态">
        <div class="stat-card">
          <n-icon :component="CloudOutline" class="stat-card__icon" :class="stateClass(agentStatusOk, statusLoaded)" aria-hidden="true" />
          <div class="stat-card__value stat-card__value--sm">{{ agentConnectionText }}</div>
          <div class="stat-card__label">Agent 连接</div>
          <div class="stat-card__sub mono" :title="server.agent_url">{{ server.agent_url || '未配置 Agent 地址' }}</div>
        </div>
        <div class="stat-card">
          <n-icon :component="GitNetworkOutline" class="stat-card__icon" :class="stateClass(!!agentStatus?.frps_active, statusLoaded)" aria-hidden="true" />
          <div class="stat-card__value stat-card__value--sm">{{ frpsStateText }}</div>
          <div class="stat-card__label">frps 服务</div>
          <div class="stat-card__sub">{{ installedFrpsVersion || '未安装' }}</div>
        </div>
        <div class="stat-card">
          <n-icon :component="LayersOutline" class="stat-card__icon" :class="routesCount > 0 ? 'stat-card__icon--blue' : 'stat-card__icon--amber'" aria-hidden="true" />
          <div class="stat-card__value stat-card__value--sm">{{ routesCount }} 条</div>
          <div class="stat-card__label">反代路由</div>
          <div class="stat-card__sub">VPS 上已注册</div>
        </div>
        <div class="stat-card">
          <n-icon :component="SwapHorizontalOutline" class="stat-card__icon" :class="portIconClass" aria-hidden="true" />
          <div class="port-list">
            <span
              v-for="port in watchedPorts"
              :key="port"
              class="port-item"
              :class="{ 'port-item--on': portOn(port) }"
              :title="portMeaning(port)"
            >
              {{ port }}
              <n-icon :component="portOn(port) ? CheckmarkCircleOutline : CloseCircleOutline" />
            </span>
          </div>
          <div class="stat-card__label">端口监听</div>
          <div class="stat-card__sub">frps 运行所需端口</div>
        </div>
          </section>
          <section class="overview-grid" aria-label="VPS 资源与证书">
        <HavlineCard class="section-card vps-resource-card" title="VPS 资源">
          <div class="section-body">
            <p v-if="metricsError" class="form-hint">{{ metricsError }}</p>
            <div v-else-if="agentMetrics" class="res-strip">
              <div v-for="row in metricsRows" :key="row.label" class="res-row">
                <span class="res-row__label">{{ row.label }}</span>
                <span class="res-row__value">{{ row.value }}</span>
                <span class="res-row__detail">{{ row.detail }}</span>
                <span class="res-bar" :class="`res-bar--${row.tone}`">
                  <span class="res-bar__fill" :style="{ width: `${row.percent.toFixed(1)}%` }" />
                </span>
              </div>
              <div class="res-meta">
                <span>平均负载 {{ metricsLoadText }}</span>
                <span>运行时长 {{ metricsUptimeText }}</span>
              </div>
            </div>
            <p v-else class="form-hint">{{ metricsLoading ? '正在采集 VPS 资源…' : '暂无 VPS 资源数据' }}</p>
          </div>
        </HavlineCard>

        <HavlineCard class="section-card vps-resource-card" title="VPS 证书">
          <div class="section-body">
            <p v-if="certsError" class="form-hint">{{ certsError }}</p>
            <div v-else-if="agentCerts.length > 0" class="cert-list">
              <div class="cert-row cert-row--head">
                <span>域名</span>
                <span>剩余天数</span>
                <span>Havline 证书库</span>
                <span>到期时间</span>
              </div>
              <div v-for="item in agentCerts" :key="item.domain" class="cert-row">
                <span class="mono cert-domain">{{ item.domain }}</span>
                <span :class="{ 'cert-days--warn': item.days_left < 30, 'cert-days--error': item.days_left < 0 }">
                  {{ item.days_left < 0 ? '已过期' : `${item.days_left} 天` }}
                </span>
                <span>{{ havlineCertDomains.has(item.domain) ? '有' : '无' }}</span>
                <span class="cert-muted">{{ item.not_after ? formatDate(item.not_after) : '未解析' }}</span>
              </div>
            </div>
            <p v-else class="form-hint">{{ certsLoading ? '正在读取 VPS 证书…' : 'VPS 证书目录为空' }}</p>
            <p v-if="certWarnings.length > 0" class="form-hint">{{ certWarnings.join('；') }}</p>
          </div>
        </HavlineCard>
      </section>
        </n-tab-pane>

      <!-- ② 环境与依赖 -->
        <n-tab-pane name="deps" tab="环境与依赖">
      <section class="dep-grid" aria-label="环境与依赖">
        <HavlineCard class="section-card" title="Havline Agent">
          <div class="section-body">
            <div class="field-grid">
              <div><label>Agent 版本</label><span class="mono">{{ agentVersionText(agentStatus?.agent_version) }}</span></div>
              <div><label>凭据状态</label><span class="field-tags">
                <StatusBadge :kind="server.agent_configured ? 'success' : 'warning'" :text="server.agent_configured ? '已配置' : '未配置'" />
              </span></div>
              <div><label>SSH 凭据</label><span class="field-tags">
                <StatusBadge :kind="server.ssh_configured ? 'success' : 'disabled'" :text="server.ssh_configured ? '已配置' : '未配置'" />
              </span></div>
              <div><label>Agent 地址</label><span class="mono mono-wrap">{{ server.agent_url || '未配置' }}</span></div>
            </div>
            <div class="agent-actions">
              <n-button size="small" :loading="busy === 'probe'" :disabled="busy !== null && busy !== 'probe'" @click="probe(server)">探测环境</n-button>
            </div>
          </div>
        </HavlineCard>

        <HavlineCard class="section-card" title="Nginx（公网反代依赖）">
          <div class="section-body">
            <div class="field-grid">
              <div><label>可用性</label><span class="field-tags"><StatusBadge :kind="nginxKind" :text="nginxText" /></span></div>
              <div><label>进程</label><span>{{ agentStatus?.nginx_running ? '运行中' : '未运行' }}</span></div>
              <div><label>版本</label><span class="mono">{{ agentStatus?.nginx_version || '—' }}</span></div>
              <div><label>可执行文件</label><span class="mono mono-wrap">{{ agentStatus?.nginx_path || agentStatus?.nginx_binary || '—' }}</span></div>
              <div class="field-grid__wide"><label>vhost 配置目录</label><span class="mono mono-wrap">{{ agentStatus?.nginx_conf_dir || '—' }}</span></div>
            </div>
            <p v-if="agentStatus?.nginx_error" class="form-hint nginx-error mono">{{ agentStatus.nginx_error }}</p>
            <p v-if="reloadHint" class="form-hint">{{ reloadHint }}</p>
            <div class="agent-actions">
              <n-popconfirm
                v-if="agentStatus?.nginx_running"
                positive-text="停止"
                negative-text="取消"
                @positive-click="nginxAction('stop')"
              >
                <template #trigger>
                  <n-button size="small" :loading="actingNginx" :disabled="busy !== null">停止 Nginx</n-button>
                </template>
                停止后 VPS 上所有经 Nginx 反代的域名会立即无法访问，需要重新启动才能恢复。确定停止？
              </n-popconfirm>
              <n-button
                v-else
                size="small"
                type="primary"
                :loading="actingNginx"
                :disabled="busy !== null || !agentStatus?.nginx_path"
                @click="nginxAction('start')"
              >启动 Nginx</n-button>
              <n-button size="small" :loading="reloadingNginx" :disabled="busy !== null" @click="reloadNginx">重载 Nginx</n-button>
              <router-link to="/public-proxy"><n-button size="small" quaternary>去「公网反代」处理</n-button></router-link>
            </div>
          </div>
        </HavlineCard>

        <HavlineCard class="section-card" title="frps 运行时">
          <div class="section-body">
            <div class="field-grid">
              <div><label>已安装版本</label><span class="field-tags"><StatusBadge :kind="agentStatus?.frps_version ? 'success' : 'warning'" :text="installedFrpsVersion || '未安装'" /></span></div>
              <div><label>开机自启</label><span class="field-tags"><StatusBadge :kind="agentStatus?.frps_enabled ? 'success' : 'warning'" :text="agentStatus?.frps_enabled ? '已启用' : '未启用'" /></span></div>
              <div><label>二进制路径</label><span class="mono mono-wrap">{{ agentStatus?.frps_binary || '—' }}</span></div>
              <div><label>配置文件</label><span class="mono mono-wrap">{{ agentStatus?.frps_config_path || '—' }}</span></div>
              <div class="field-grid__wide"><label>启动命令</label><span class="mono mono-wrap">{{ agentStatus?.frps_unit_exec || '—' }}</span></div>
              <div><label>启动一致性</label><span class="field-tags"><StatusBadge :kind="frpsConsistency.kind" :text="frpsConsistency.text" /></span></div>
            </div>
            <p v-if="!frpsConsistency.ok && frpsConsistency.detail" class="form-hint">{{ frpsConsistency.detail }}</p>
          </div>
        </HavlineCard>
      </section>
        </n-tab-pane>

      <!-- ④ Agent 生命周期（SSH） + ③ frps 控制：两列并排（生命周期在左） -->
        <n-tab-pane name="control" tab="运行控制">
      <div class="control-grid">
      <HavlineCard class="section-card" title="Agent 生命周期（SSH）">
        <div class="section-body">
          <p class="form-hint">
            首次安装、升级、卸载需要 SSH（在本页「编辑服务端 → 远程管理 → 公网 Agent 管理」区填写）；安装成功后自动建立 SSH 隧道，不会默认暴露公网 7700。
            SSH 信息仅用于生命周期和隧道，凭据加密存储、不回显。
          </p>
          <div class="field-grid agent-transport-grid">
            <div><label>传输方式</label><span class="field-tags"><StatusBadge :kind="agentTransportKind(server)" :text="agentTransportText(server)" /></span></div>
            <div><label>隧道状态</label><span>{{ agentTransportStateText(server) }}</span></div>
            <div><label>本地入口</label><span class="mono">{{ server.agent_transport === 'tunnel' && server.agent_local_port ? `127.0.0.1:${server.agent_local_port}` : '—' }}</span></div>
            <div><label>Agent 监听</label><span class="mono">{{ server.agent_listen_addr || '—' }}</span></div>
            <div v-if="server.agent_transport === 'https-pin'"><label>HTTPS 端口</label><span class="mono">{{ agentHTTPSPort(server) || '—' }}</span></div>
            <div v-if="server.agent_transport === 'https-pin'"><label>证书到期</label><span>{{ server.agent_tls_not_after ? formatDate(server.agent_tls_not_after) : '—' }}</span></div>
          </div>
          <n-alert v-if="server.agent_transport === 'http'" type="warning" :bordered="false" class="agent-transport-warning">
            当前仍是公网明文 HTTP。点击「升级 Agent」可自动迁移到 SSH 隧道安全模式。
          </n-alert>
          <n-alert v-if="server.agent_transport === 'https-pin'" type="info" :bordered="false" class="agent-transport-warning">
            HTTPS 直连使用证书指纹校验，证书到期前 30 天会自动轮换。请确认云安全组已放行 {{ agentHTTPSPort(server) || 'HTTPS' }} 端口，Havline 不会自动修改云安全组。
          </n-alert>
          <p v-if="server.agent_transport === 'https-pin' && server.agent_firewall_state" class="form-hint">主机防火墙：{{ server.agent_firewall_state }}</p>
          <p v-if="server.agent_transport_error" class="form-hint agent-transport-error">{{ server.agent_transport_error }}</p>
          <div class="agent-actions">
            <n-button size="small" :loading="busy === 'agent-install'" :disabled="busy !== null && busy !== 'agent-install'" @click="installAgent(server)">{{ server.agent_configured ? '升级 Agent' : '安装 Agent' }}</n-button>
            <n-button v-if="server.agent_transport === 'tunnel'" size="small" :loading="busy === 'tunnel-restart'" :disabled="busy !== null && busy !== 'tunnel-restart'" @click="restartAgentTunnel(server)">重启隧道</n-button>
            <n-popconfirm v-if="server.agent_transport === 'tunnel'" @positive-click="enableAgentHTTPS(server)">
              <template #trigger>
                <n-button size="small" :loading="busy === 'transport-https'" :disabled="busy !== null && busy !== 'transport-https'">切换 HTTPS 直连</n-button>
              </template>
              将自动从 TCP 7443 / 8443 / 9443 中选择可用端口启用自签名 HTTPS。请确保云安全组已放行其中之一，否则切换后 Agent 会不可达。继续？
            </n-popconfirm>
            <n-button v-if="server.agent_transport === 'https-pin'" size="small" :loading="busy === 'tls-rotate'" :disabled="busy !== null && busy !== 'tls-rotate'" @click="rotateAgentCert(server)">轮换证书</n-button>
            <n-button v-if="server.agent_transport === 'https-pin'" size="small" :loading="busy === 'firewall-check'" :disabled="busy !== null && busy !== 'firewall-check'" @click="checkAgentFirewall(server)">检测主机防火墙</n-button>
            <n-popconfirm v-if="server.agent_transport === 'https-pin' && firewallInfo && firewallInfo.serverId === server.id && !firewallInfo.data.port_open" @positive-click="allowAgentFirewall(server)">
              <template #trigger>
                <n-button size="small" :loading="busy === 'firewall-allow'" :disabled="busy !== null && busy !== 'firewall-allow'">放行主机防火墙</n-button>
              </template>
              只会对 ufw / firewalld 执行放行，不会修改云安全组。继续？
            </n-popconfirm>
            <n-popconfirm v-if="server.agent_transport === 'https-pin'" @positive-click="switchAgentToTunnel(server)">
              <template #trigger>
                <n-button size="small" :loading="busy === 'transport-tunnel'" :disabled="busy !== null && busy !== 'transport-tunnel'">切回 SSH 隧道</n-button>
              </template>
              将关闭 VPS 上的 HTTPS Agent 入口并恢复 SSH 隧道。继续？
            </n-popconfirm>
            <n-button size="small" :loading="busy === 'diagnose'" :disabled="busy !== null && busy !== 'diagnose'" @click="diagnoseAgent(server)">SSH 诊断</n-button>
            <n-button size="small" quaternary :loading="busy === 'agent-uninstall'" :disabled="busy !== null && busy !== 'agent-uninstall'" @click="uninstallAgent(server, true)">卸载（保留路由/证书）</n-button>
            <n-popconfirm @positive-click="uninstallAgent(server, false)">
              <template #trigger>
                <n-button size="small" type="warning" :disabled="busy !== null">卸载（含数据）</n-button>
              </template>
              将删除 VPS 上的 havline-agent、frps 单元与 /var/lib/havline-agent（已注册的路由与证书一并丢失），确认继续？
            </n-popconfirm>
          </div>
        </div>
      </HavlineCard>

      <HavlineCard class="section-card" title="frps 控制">
        <div class="section-body">
          <div class="agent-actions">
            <n-button v-if="agentStatus?.frps_active" size="small" type="warning" :loading="busy === 'frps-stop'" :disabled="busy !== null && busy !== 'frps-stop'" @click="frpsAction(server, 'stop')">停止 frps</n-button>
            <n-button v-else size="small" type="success" :loading="busy === 'frps-start'" :disabled="busy !== null && busy !== 'frps-start'" @click="frpsAction(server, 'start')">启动 frps</n-button>
            <n-button size="small" :loading="busy === 'frps-restart'" :disabled="busy !== null && busy !== 'frps-restart'" @click="frpsAction(server, 'restart')">重启 frps</n-button>
            <n-tooltip :disabled="!!dashboardURL">
              <template #trigger>
                <span class="tooltip-trigger">
                  <n-button size="small" quaternary :disabled="!dashboardURL" @click="openDashboard">frps 管理面板</n-button>
                </span>
              </template>
              未配置 frps 管理接口：在本页「编辑服务端 → 服务端配置 → frps 管理接口」填写地址与端口后保存
            </n-tooltip>
            <n-button size="small" :loading="busy === 'push-config'" :disabled="busy !== null && busy !== 'push-config'" @click="pushAgentConfig(server)">下发 frps 配置</n-button>
            <n-button size="small" quaternary :loading="configLoading" :disabled="busy !== null" @click="viewConfig(server)">配置查看</n-button>
          </div>
          <div class="frps-install-row">
            <n-select
              v-model:value="frpsInstallVersion"
              :options="frpsVersionOptions"
              placeholder="选择 frps 官方版本"
              class="frps-version-select"
              filterable
              :loading="releasesLoading"
            />
            <n-button size="small" type="success" :loading="busy === 'frps-install'" :disabled="(busy !== null && busy !== 'frps-install') || !releases.length" @click="installFrps(server)">安装 / 升级 frps</n-button>
            <span v-if="!releases.length" class="form-hint">正在获取官方版本列表…（获取失败时可手动输入版本号）</span>
            <span v-else class="form-hint">VPS 无 frps 二进制时使用：自动下载并注册 frps.service，完成后自动下发同源配置</span>
          </div>
        </div>
      </HavlineCard>

      </div>
        </n-tab-pane>

      <!-- ⑤ 输出与日志 -->
        <n-tab-pane name="logs" tab="日志与输出">
      <HavlineCard class="section-card" title="输出与日志">
        <template #header>
          <n-button size="small" quaternary :disabled="!opOutput.length" @click="opOutput = []">清空操作输出</n-button>
        </template>
        <div class="section-body">
          <n-tabs type="line" animated>
            <n-tab-pane name="ops" tab="操作输出">
              <div v-if="opOutput.length" class="op-list" aria-live="polite">
                <div v-for="(item, index) in opOutput" :key="index" class="op-item" :class="{ 'op-item--fail': !item.ok }">
                  <div class="op-item__head">
                    <StatusBadge :kind="item.ok ? 'success' : 'error'" :text="item.ok ? '成功' : '失败'" />
                    <span class="op-item__source">{{ item.source }}</span>
                    <span class="op-item__time">{{ formatRelativeTime(item.at) }}</span>
                  </div>
                  <pre class="code-block">{{ item.text }}</pre>
                </div>
              </div>
              <EmptyState v-else title="暂无操作输出" description="执行探测、安装、重启等操作后，这里会按时间倒序显示来源、时间与结果。" />
            </n-tab-pane>
            <n-tab-pane name="logs" tab="frps 日志">
              <pre v-if="agentStatus?.frps_log_tail" class="code-block">{{ agentStatus.frps_log_tail }}</pre>
              <EmptyState v-else title="暂无 frps 日志" description="VPS 上 frps 服务输出为空，或 frps 尚未启动。" />
            </n-tab-pane>
          </n-tabs>
        </div>
      </HavlineCard>
        </n-tab-pane>
      </n-tabs>
        </div>
      </div>

      <!-- 配置查看弹窗：展示即将下发的 frps.toml -->
      <n-modal v-model:show="configModal" style="width: 640px">
        <div class="config-modal">
          <div class="config-modal__header">
            <h3 class="modal-title">frps 配置（即将下发的内容）</h3>
            <n-button size="small" quaternary @click="configModal = false">
              <template #icon><n-icon :component="CloseOutline" /></template>
            </n-button>
          </div>
          <div class="config-modal__body">
            <p class="form-hint">
              由 Havline 本地生成；点「下发 frps 配置」后写入 VPS 的
              <span class="mono">{{ agentStatus?.frps_config_path || '/etc/frp/frps.toml' }}</span>，重启 frps 后生效。
              <br />
              密钥类字段（如 <span class="mono">auth.token</span>）在这里显示为掩码，下发时仍使用真实值。
            </p>
            <pre class="code-block config-modal__code">{{ configContent || '# 暂无内容' }}</pre>
          </div>
          <div class="config-modal__footer">
            <n-button size="small" :disabled="!configContent" @click="copyConfig">复制（已脱敏）</n-button>
            <n-button size="small" type="primary" @click="configModal = false">关闭</n-button>
          </div>
        </div>
      </n-modal>
    </template>
  </template>

  <n-modal
    v-model:show="showServerModal"
    :mask-closable="false"
    transform-origin="center"
  >
    <div class="proxy-modal frp-proxy-modal" :class="{ 'frp-proxy-modal--creating': isCreatingServer }">
      <div class="proxy-modal__form">
        <div class="proxy-modal__header">
          <h3 class="modal-title">{{ editingServer ? '编辑服务端' : '添加服务端' }}</h3>
          <n-button size="small" quaternary @click="showServerModal = false">
            <template #icon><n-icon :component="CloseOutline" /></template>
          </n-button>
        </div>
        <template v-if="isCreatingServer">
          <div class="server-wizard">
            <n-steps :current="addStep" size="small" class="server-wizard__steps">
              <n-step title="连接 frps" description="名称、地址与认证" />
              <n-step title="接入方式" description="可选安装 Agent" />
              <n-step title="确认保存" description="检查摘要并保存" />
            </n-steps>

            <div v-if="addStep === 1" class="server-wizard__body">
              <n-form label-placement="top" class="server-wizard__form" @submit.prevent="addNextStep">
                <n-form-item label="快速预设">
                  <n-select :value="serverPreset" :options="serverPresetOptions" @update:value="applyServerPreset" />
                </n-form-item>
                <div class="form-grid form-grid--wide">
                  <n-form-item label="服务端名称" required><n-input v-model:value="serverForm.name" placeholder="例如：东京 VPS" /></n-form-item>
                  <n-form-item label="主机地址" required><n-input v-model:value="serverForm.server_addr" placeholder="例如：frps.example.com" /></n-form-item>
                  <n-form-item label="控制端口" required><n-input-number v-model:value="serverForm.server_port" :min="1" :max="65535" /></n-form-item>
                </div>
                <n-form-item label="认证方式" required>
                  <n-radio-group v-model:value="serverForm.options.auth_method" name="add-auth-method">
                    <n-radio-button value="none">无</n-radio-button>
                    <n-radio-button value="token">令牌</n-radio-button>
                    <n-radio-button value="oidc">OIDC</n-radio-button>
                  </n-radio-group>
                </n-form-item>
                <div v-if="serverForm.options.auth_method === 'token'" class="form-grid server-wizard__grid">
                  <n-form-item label="用户" required>
                    <n-input v-model:value="serverForm.options.user" placeholder="与 frps auth.user 一致" />
                  </n-form-item>
                  <n-form-item label="令牌" required>
                    <ConfiguredSecretField v-model="serverForm.token" label="令牌" :configured="false" placeholder="请输入与 frps 一致的令牌" hide-label />
                  </n-form-item>
                </div>
                <n-form-item label="启用 TLS">
                  <div class="server-wizard__switch-inline">
                    <n-switch v-model:value="serverForm.tls_enabled" />
                    <span>{{ serverForm.tls_enabled ? '默认开启' : '已关闭' }}</span>
                  </div>
                </n-form-item>
                <template v-if="serverForm.options.auth_method === 'oidc'">
                  <div class="form-grid">
                    <n-form-item label="客户端 ID（Client ID）" required><n-input v-model:value="serverForm.options.oidc_client_id" placeholder="身份提供商分配的客户端标识" /></n-form-item>
                    <n-form-item label="客户端密钥（Client Secret）" required>
                      <ConfiguredSecretField v-model="serverForm.oidc_client_secret" label="客户端密钥（Client Secret）" :configured="false" placeholder="请输入客户端密钥" hide-label />
                    </n-form-item>
                    <n-form-item label="令牌端点（Token Endpoint）" required><n-input v-model:value="serverForm.options.oidc_token_endpoint_url" placeholder="https://issuer.example.com/token" /></n-form-item>
                  </div>
                </template>
              </n-form>
            </div>

            <div v-else-if="addStep === 2" class="server-wizard__body">
              <p class="form-hint">Agent 不是连接 frps 的必需条件，可以稍后在本页安装。</p>
              <n-radio-group :value="agentSetupMode" class="server-wizard__agent-choice" @update:value="setAgentSetupMode">
                <n-radio-button v-for="item in agentSetupOptions" :key="item.value" :value="item.value">{{ item.label }}</n-radio-button>
              </n-radio-group>
              <div v-if="agentSetupMode === 'ssh'" class="form-grid server-wizard__grid">
                <n-form-item label="SSH 地址" required><n-input v-model:value="serverForm.ssh_host" placeholder="VPS 公网 IP 或域名" /></n-form-item>
                <n-form-item label="SSH 端口"><n-input-number v-model:value="serverForm.ssh_port" :min="1" :max="65535" /></n-form-item>
                <n-form-item label="SSH 用户" required><n-input v-model:value="serverForm.ssh_user" placeholder="root 或具备 sudo 权限的用户" /></n-form-item>
                <n-form-item label="SSH 认证"><n-select v-model:value="serverForm.ssh_auth" :options="[{ label: '私钥', value: 'key' }, { label: '密码', value: 'password' }]" /></n-form-item>
                <n-form-item :show-label="false" :show-feedback="false" class="form-grid__wide">
                  <ConfiguredSecretField
                    v-model="serverForm.ssh_secret"
                    :label="serverForm.ssh_auth === 'key' ? 'SSH 私钥' : 'SSH 密码'"
                    :configured="false"
                    :placeholder="serverForm.ssh_auth === 'key' ? '粘贴私钥内容' : '输入 SSH 密码'"
                  />
                </n-form-item>
              </div>
              <div v-else-if="agentSetupMode === 'existing'" class="form-grid server-wizard__grid">
                <n-form-item label="Agent API 地址" required><n-input v-model:value="serverForm.agent_url" placeholder="安装后自动回填；隧道模式可用 127.0.0.1 地址" /></n-form-item>
                <n-form-item label="Agent Token">
                  <ConfiguredSecretField v-model="serverForm.agent_token" label="Agent Token" :configured="false" placeholder="留空保持不变" hide-label />
                </n-form-item>
              </div>
              <n-alert v-else type="info" :bordered="false">暂不安装 Agent 时，只保存 frps 连接信息；之后可在本页补装 Agent。</n-alert>
            </div>

            <div v-else class="server-wizard__body">
              <div class="server-wizard__summary">
                <div v-for="item in addSummary" :key="item.label"><span>{{ item.label }}</span><strong>{{ item.value }}</strong></div>
              </div>
              <n-collapse class="server-wizard__advanced">
                <n-collapse-item title="高级服务端参数（可选）" name="advanced">
                  <div class="form-grid">
                    <n-form-item label="HTTP 回源端口"><n-input-number v-model:value="serverForm.options.vhost_http_port" :min="1" :max="65535" /></n-form-item>
                    <n-form-item label="HTTPS 回源端口"><n-input-number v-model:value="serverForm.options.vhost_https_port" :min="1" :max="65535" /></n-form-item>
                    <n-form-item label="端口白名单（allowPorts）"><n-input v-model:value="serverForm.options.allow_ports" placeholder="如 6000,6005-6010；留空不限制" /></n-form-item>
                    <n-form-item label="管理地址">
                      <n-input v-model:value="serverForm.options.dashboard_addr" placeholder="留空则不采集流量">
                        <template #suffix><n-button size="tiny" quaternary @click="serverForm.options.dashboard_addr = serverForm.server_addr">用主机地址</n-button></template>
                      </n-input>
                    </n-form-item>
                    <n-form-item label="管理端口"><n-input-number v-model:value="serverForm.options.dashboard_port" :min="0" :max="65535" placeholder="例如 7500" /></n-form-item>
                  </div>
                </n-collapse-item>
              </n-collapse>
            </div>

            <div class="server-wizard__footer">
              <n-button v-if="addStep > 1" @click="addPrevStep">上一步</n-button>
              <div class="server-wizard__footer-right">
                <n-button @click="showServerModal = false">取消</n-button>
                <n-button v-if="addStep < 3" type="primary" :disabled="(addStep === 1 && !canProceedAddStep1) || (addStep === 2 && !canProceedAddStep2)" @click="addNextStep">下一步</n-button>
                <template v-else>
                  <n-button v-if="agentSetupMode === 'existing'" :loading="savingServer" @click="saveServer(true)">保存并下发</n-button>
                  <n-button type="primary" :loading="savingServer" @click="saveServer(false)">保存</n-button>
                </template>
              </div>
            </div>
          </div>
        </template>

        <template v-else>
          <div class="proxy-modal__tabbar">
            <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': serverFormTab === 'basic' }" @click="serverFormTab = 'basic'">连接</button>
            <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': serverFormTab === 'server' }" @click="serverFormTab = 'server'">服务端参数</button>
            <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': serverFormTab === 'remote' }" @click="serverFormTab = 'remote'">Agent 与远程</button>
            <button type="button" class="proxy-modal__tab" :class="{ 'proxy-modal__tab--active': serverFormTab === 'advanced' }" @click="serverFormTab = 'advanced'">高级</button>
          </div>
          <div v-if="editingServer" class="server-edit-summary">
            <div class="server-edit-summary__identity">
              <strong>{{ editingServer.name }}</strong>
              <span class="mono">{{ serverForm.server_addr }}:{{ serverForm.server_port }}</span>
            </div>
            <div class="server-edit-summary__badges">
              <StatusBadge :kind="serverForm.tls_enabled ? 'success' : 'warning'" :text="serverForm.tls_enabled ? 'TLS 已启用' : 'TLS 未启用'" />
              <StatusBadge :kind="editingServer.agent_configured ? 'success' : 'disabled'" :text="editingServer.agent_configured ? 'Agent 已配置' : 'Agent 未配置'" />
              <StatusBadge :kind="editingServer.ssh_configured ? 'success' : 'disabled'" :text="editingServer.ssh_configured ? 'SSH 已配置' : 'SSH 未配置'" />
            </div>
          </div>
          <div class="proxy-modal__scroll">
            <n-form label-placement="top" class="proxy-modal__pane" @submit.prevent="saveServer">
            <div v-show="serverFormTab === 'basic'" class="frp-server-modal__basic">
      <section class="form-section">
        <h3><n-icon :component="InformationCircleOutline" /> 基本信息</h3>
        <div class="form-grid form-grid--wide">
          <n-form-item label="服务端名称" required><n-input v-model:value="serverForm.name" placeholder="例如：生产服务器" /></n-form-item>
          <n-form-item label="主机地址" required><n-input v-model:value="serverForm.server_addr" placeholder="例如：frps.example.com" /></n-form-item>
          <n-form-item label="端口" required><n-input-number v-model:value="serverForm.server_port" :min="1" :max="65535" /></n-form-item>
        </div>
        <div class="switch-row"><span>启用服务端</span><n-switch v-model:value="serverForm.enabled" /></div>
      </section>

      <section class="form-section">
        <h3><n-icon :component="LockClosedOutline" /> 认证</h3>
        <n-form-item label="认证方式" required>
          <n-radio-group v-model:value="serverForm.options.auth_method" name="auth-method">
            <n-radio-button value="none">无</n-radio-button>
            <n-radio-button value="token">令牌</n-radio-button>
            <n-radio-button value="oidc">OIDC（开放身份连接）</n-radio-button>
          </n-radio-group>
        </n-form-item>
        <div v-if="serverForm.options.auth_method === 'token'" class="form-grid">
          <n-form-item label="用户"><n-input v-model:value="serverForm.options.user" placeholder="例如：admin" /></n-form-item>
          <n-form-item>
            <template #label>
              <span class="configured-secret-form-label">
                <span>令牌</span>
                <n-tag v-if="editingServer?.has_token" size="small" type="success" :bordered="false">{{ CONFIGURED_SECRET_TAG }}</n-tag>
              </span>
            </template>
            <ConfiguredSecretField
              v-model="serverForm.token"
              label="令牌"
              :configured="!!editingServer?.has_token"
              placeholder="请输入与 frps 一致的令牌"
              hide-label
            />
          </n-form-item>
        </div>
        <n-form-item v-else label="用户"><n-input v-model:value="serverForm.options.user" placeholder="例如：admin" /></n-form-item>
        <template v-if="serverForm.options.auth_method === 'oidc'">
          <p class="form-hint">OIDC（OpenID Connect，开放身份连接）会向身份提供商申请访问令牌，再用于连接 FRP 服务端。</p>
          <div class="form-grid">
            <n-form-item label="客户端 ID（Client ID）" required><n-input v-model:value="serverForm.options.oidc_client_id" placeholder="身份提供商分配的客户端标识" /></n-form-item>
            <n-form-item>
              <template #label>
                <span class="configured-secret-form-label">
                  <span>客户端密钥（Client Secret）</span>
                  <n-tag v-if="editingServer?.has_oidc_client_secret" size="small" type="success" :bordered="false">{{ CONFIGURED_SECRET_TAG }}</n-tag>
                </span>
              </template>
              <ConfiguredSecretField
                v-model="serverForm.oidc_client_secret"
                label="客户端密钥（Client Secret）"
                :configured="!!editingServer?.has_oidc_client_secret"
                placeholder="请输入客户端密钥"
                hide-label
              />
            </n-form-item>
            <n-form-item label="令牌端点（Token Endpoint）" required class="form-grid__wide"><n-input v-model:value="serverForm.options.oidc_token_endpoint_url" placeholder="https://issuer.example.com/token" /></n-form-item>
          </div>
          <n-collapse class="oidc-advanced-collapse">
            <n-collapse-item name="oidc-advanced">
              <template #header>OIDC 高级选项</template>
              <div class="form-grid">
                <n-form-item label="受众（Audience）"><n-input v-model:value="serverForm.options.oidc_audience" placeholder="可选，用于限定令牌的目标服务" /></n-form-item>
                <n-form-item label="额外权限范围（Scope）"><n-input v-model:value="serverForm.options.oidc_scope" placeholder="例如：openid profile" /></n-form-item>
                <n-form-item label="OIDC 请求代理地址" class="form-grid__wide"><n-input v-model:value="serverForm.options.oidc_proxy_url" placeholder="http://user:pass@192.168.1.1:8080" /></n-form-item>
              </div>
              <div class="switch-row"><span>跳过 OIDC TLS 校验</span><n-switch v-model:value="serverForm.options.oidc_insecure_skip_verify" /></div>
            </n-collapse-item>
          </n-collapse>
        </template>
      </section>

      <section class="form-section">
        <h3><n-icon :component="DocumentOutline" /> 描述</h3>
        <n-form-item label="描述"><n-input v-model:value="serverForm.options.remark" type="textarea" :rows="2" placeholder="添加一些关于此服务器的备注" /></n-form-item>
      </section>
            </div>
            <div v-show="serverFormTab === 'server'" class="frp-server-modal__server">
      <n-alert type="info" :bordered="false" class="server-config-notice">
        frps 参数修改后需保存并「下发 frps 配置」，再重启 frps 才会生效。
      </n-alert>
      <n-collapse class="server-config-collapse" :default-expanded-names="['vhost']">
        <n-collapse-item name="vhost">
          <template #header><n-icon :component="SwapHorizontalOutline" /> frps vhost 与域名反代</template>
      <section class="form-section">
        <p class="form-hint">frps 的 vhostHTTPPort / vhostHTTPSPort 是 Nginx 反代的回源端口（proxy_pass http://127.0.0.1:端口），同时包含 subDomainHost、tcpmux 等域名入口配置。修改后需「下发 frps 配置」并重启 frps，且重新部署各规则的反代。留空用默认 8080 / 8443。</p>
        <n-alert
          v-if="serverForm.options.vhost_http_port === 80 || serverForm.options.vhost_https_port === 443"
          type="warning"
          :show-icon="false"
          style="margin-bottom: 12px"
        >
          80/443 通常由 VPS 上的 Nginx 占用。继续使用前请先停止占用该端口的服务，保存时会再次弹窗确认；端口仍被占用时 frps 会因冲突启动失败。若 VPS 上还要保留 Nginx，请改用 8080/8443。
        </n-alert>
        <div class="form-grid">
          <n-form-item label="HTTP 回源端口"><n-input-number v-model:value="serverForm.options.vhost_http_port" :min="1" :max="65535" placeholder="默认 8080" /></n-form-item>
          <n-form-item label="HTTPS 回源端口"><n-input-number v-model:value="serverForm.options.vhost_https_port" :min="1" :max="65535" placeholder="默认 8443" /></n-form-item>
        </div>
        <div class="form-grid">
          <n-form-item label="子域名根（subDomainHost）"><n-input v-model:value="serverForm.options.sub_domain_host" placeholder="例如：frps.example.com，规则填 subdomain 后生成为 sub.frps.example.com" /></n-form-item>
          <n-form-item label="tcpmux 监听端口"><n-input-number v-model:value="serverForm.options.tcpmux_http_connect_port" :min="1" :max="65535" placeholder="存在 tcpmux 规则时必填" /></n-form-item>
        </div>
        <div class="form-grid">
          <n-form-item label="自定义 404 页面（VPS 路径）"><n-input v-model:value="serverForm.options.custom_404_page" placeholder="例如 /var/www/404.html；留空用默认" /></n-form-item>
          <div class="switch-row"><span>强制 TLS（frps 只接受 TLS 连接）</span><n-switch v-model:value="serverForm.options.tls_force" /></div>
        </div>
        <div class="switch-row"><span>frps 写文件日志（/var/log/frps.log）</span><n-switch v-model:value="serverForm.options.frps_log_to_file" /></div>
        <n-form-item v-if="serverForm.options.frps_log_to_file" label="日志保留天数"><n-input-number v-model:value="serverForm.options.frps_log_max_days" :min="1" :max="365" placeholder="默认 3" class="narrow-input" /></n-form-item>
      </section>
        </n-collapse-item>

        <n-collapse-item name="ports">
          <template #header><n-icon :component="GitNetworkOutline" /> TCP/UDP 端口分配</template>
      <section class="form-section">
        <p class="form-hint">allowPorts 只限制 tcp / udp 规则使用的 remotePort，不控制 HTTP/HTTPS 的 vhost 回源端口。留空表示不限制，但实际远程端口仍需在云安全组和 VPS 主机防火墙放行。</p>
        <div class="form-grid">
          <n-form-item label="端口白名单（allowPorts）"><n-input v-model:value="serverForm.options.allow_ports" placeholder="如 6000,6005-6010,7000-8000；留空不限制" /></n-form-item>
          <n-form-item label="每客户端端口上限"><n-input-number v-model:value="serverForm.options.max_ports_per_client" :min="0" :max="65535" placeholder="0 表示不限" /></n-form-item>
        </div>
      </section>
        </n-collapse-item>

        <n-collapse-item name="dashboard">
          <template #header><n-icon :component="ServerOutline" /> frps 管理接口</template>
      <section class="form-section">
        <p class="form-hint">用于 Havline 拉取穿透规则的实时连接与流量信息；需在 frps 的 frps.toml 中开启 webServer（dashboard），与 frpc 连接用的令牌无关。</p>
        <div class="form-grid">
          <n-form-item label="管理地址"><n-input v-model:value="serverForm.options.dashboard_addr" placeholder="frps 机器的 IP 或域名，留空则不采集流量" /></n-form-item>
          <n-form-item label="管理端口"><n-input-number v-model:value="serverForm.options.dashboard_port" :min="0" :max="65535" placeholder="例如：7500" /></n-form-item>
        </div>
        <div class="form-grid">
          <n-form-item label="管理用户"><n-input v-model:value="serverForm.options.dashboard_user" placeholder="webServer.user，未启用认证可留空" /></n-form-item>
          <n-form-item>
            <template #label>
              <span class="configured-secret-form-label">
                <span>管理密码</span>
                <n-tag v-if="editingServer?.dashboard_has_pwd" size="small" type="success" :bordered="false">{{ CONFIGURED_SECRET_TAG }}</n-tag>
              </span>
            </template>
            <ConfiguredSecretField
              v-model="serverForm.dashboard_password"
              label="管理密码"
              :configured="!!editingServer?.dashboard_has_pwd"
              placeholder="webServer.password"
              hide-label
            />
          </n-form-item>
        </div>
        <n-button v-if="editingServer?.dashboard_has_pwd" size="small" quaternary type="error" @click="serverForm.clear_dashboard_password = true">清除管理密码</n-button>
      </section>
        </n-collapse-item>
      </n-collapse>
            </div>
            <div v-show="serverFormTab === 'remote'" class="frp-server-modal__remote">
      <section class="form-section">
        <h3><n-icon :component="CloudOutline" /> 公网 Agent 管理</h3>
        <p class="form-hint">Agent 部署在公网 VPS 上，保存 HTTP/HTTPS 规则后可由 Agent 自动生成反代；「公网 Agent」页提供完整管理面板。首次安装/升级需要 SSH，日常管理走 Agent API。Agent 地址通常由 SSH 隧道或 HTTPS 固定自动回填，不建议将 7700 明文暴露到公网。</p>
        <div class="form-grid">
          <n-form-item label="Agent API 地址"><n-input v-model:value="serverForm.agent_url" placeholder="安装后自动回填；也可填 http://127.0.0.1:7700（SSH 隧道）" /></n-form-item>
          <n-form-item>
            <template #label>
              <span class="configured-secret-form-label">
                <span>Agent Token</span>
                <n-tag v-if="editingServer?.agent_configured" size="small" type="success" :bordered="false">{{ CONFIGURED_SECRET_TAG }}</n-tag>
              </span>
            </template>
            <ConfiguredSecretField v-model="serverForm.agent_token" label="Agent Token" :configured="!!editingServer?.agent_configured" placeholder="留空保持不变" hide-label />
          </n-form-item>
        </div>
        <div class="agent-mgmt-option">
          <div class="agent-mgmt-option__text">
            <div class="agent-mgmt-option__label">启用 Agent 日常管理</div>
            <div class="agent-mgmt-option__hint">关闭后仅保留 SSH 生命周期管理，不通过 Agent API 管理 VPS。</div>
          </div>
          <n-switch v-model:value="serverForm.mgmt_enabled" />
        </div>
        <n-collapse class="agent-ssh-collapse" :default-expanded-names="editingServer?.ssh_configured ? ['ssh'] : []">
          <n-collapse-item name="ssh">
            <template #header><n-icon :component="ServerOutline" /> SSH 安装与升级（可选）</template>
            <div class="form-grid">
              <n-form-item label="SSH 地址"><n-input v-model:value="serverForm.ssh_host" placeholder="用于安装/升级/卸载 agent" /></n-form-item>
              <n-form-item label="SSH 端口"><n-input-number v-model:value="serverForm.ssh_port" :min="1" :max="65535" /></n-form-item>
              <n-form-item label="SSH 用户"><n-input v-model:value="serverForm.ssh_user" placeholder="root 或具备 sudo 权限的用户" /></n-form-item>
              <n-form-item label="SSH 认证"><n-select v-model:value="serverForm.ssh_auth" :options="[{ label: '私钥', value: 'key' }, { label: '密码', value: 'password' }]" /></n-form-item>
              <n-form-item :show-label="false" :show-feedback="false"><ConfiguredSecretField v-model="serverForm.ssh_secret" :label="serverForm.ssh_auth === 'key' ? 'SSH 私钥' : 'SSH 密码'" :configured="!!editingServer?.ssh_configured" :placeholder="serverForm.ssh_auth === 'key' ? '粘贴私钥内容，用于 Agent 安装与升级' : '输入 SSH 密码，用于 Agent 安装与升级'" /></n-form-item>
            </div>
          </n-collapse-item>
        </n-collapse>
        <div v-if="editingServer?.agent_configured" class="agent-action-panel">
          <div class="agent-action-panel__head">
            <div class="agent-action-panel__title">Agent 操作</div>
            <div class="agent-action-panel__hint">验证连接，或把最新 frps 配置下发到 VPS</div>
          </div>
          <div class="agent-action-panel__buttons">
            <n-button size="small" :loading="agentActionId === editingServer.id" @click="testAgent(editingServer)">测试连接</n-button>
            <n-button size="small" type="primary" :loading="busy === 'push-config'" :disabled="busy !== null && busy !== 'push-config'" @click="pushAgentConfig(editingServer)">下发 frps 配置</n-button>
            <n-button size="small" quaternary @click="showServerModal = false; selectedId = editingServer.id">查看服务端详情</n-button>
          </div>
          <p class="agent-action-panel__note">「下发 frps 配置」使用已保存的配置；若刚修改了上方参数，请先保存再下发。</p>
        </div>
      </section>
            </div>
            <div v-show="serverFormTab === 'advanced'" class="frp-server-modal__advanced">
      <section class="form-section">
        <h3><n-icon :component="DocumentTextOutline" /> 日志</h3>
        <div class="form-grid">
          <n-form-item label="日志级别"><n-select v-model:value="serverForm.options.log_level" :options="logLevelOptions" /></n-form-item>
          <n-form-item label="最大天数"><n-input-number v-model:value="serverForm.options.log_max_days" :min="0" :max="3650" /></n-form-item>
        </div>
      </section>

      <section class="form-section">
        <h3><n-icon :component="GitNetworkOutline" /> 传输</h3>
        <n-form-item label="协议"><n-select v-model:value="serverForm.options.protocol" :options="transportOptions" /></n-form-item>
        <div class="switch-row"><span>启用 TLS</span><n-switch v-model:value="serverForm.tls_enabled" /><span>禁用自定义 TLS 首字节</span><n-switch v-model:value="serverForm.options.tls_disable_custom_first_byte" /></div>
        <div v-if="serverForm.tls_enabled" class="tls-panel">
          <n-form-item label="服务器名称（SNI）"><n-input v-model:value="serverForm.tls_server_name" placeholder="例如：frps.example.com" /></n-form-item>
          <n-collapse class="tls-cert-collapse" :default-expanded-names="hasCustomTlsMaterial ? ['cert'] : []">
            <n-collapse-item name="cert">
              <template #header>自定义 TLS 证书（可选）</template>
              <div class="tls-file-field">
                <div class="tls-file-field__head"><span>证书文件内容</span><n-button size="small" quaternary @click="openTLSFilePicker('certificate')">从文件加载</n-button></div>
                <n-input v-model:value="serverForm.tls_certificate" type="textarea" :rows="3" placeholder="粘贴客户端证书 PEM 内容或从文件加载；留空保持现有文件" />
              </div>
              <div class="tls-file-field">
                <div class="tls-file-field__head"><span>密钥文件内容</span><n-button size="small" quaternary @click="openTLSFilePicker('key')">从文件加载</n-button></div>
                <n-input v-model:value="serverForm.tls_key" type="textarea" :rows="3" placeholder="粘贴客户端私钥 PEM 内容或从文件加载；留空保持现有文件" />
              </div>
              <div class="tls-file-field">
                <div class="tls-file-field__head"><span>受信任的 CA 内容</span><n-button size="small" quaternary @click="openTLSFilePicker('ca')">从文件加载</n-button></div>
                <n-input v-model:value="serverForm.tls_trusted_ca" type="textarea" :rows="3" placeholder="粘贴 CA PEM 内容或从文件加载；留空保持现有文件" />
              </div>
              <div class="asset-actions">
                <n-button v-if="editingServer?.options.tls_certificate_configured" size="small" quaternary type="error" @click="serverForm.clear_tls_certificate = true">清除证书</n-button>
                <n-button v-if="editingServer?.options.tls_key_configured" size="small" quaternary type="error" @click="serverForm.clear_tls_key = true">清除密钥</n-button>
                <n-button v-if="editingServer?.options.tls_trusted_ca_configured" size="small" quaternary type="error" @click="serverForm.clear_tls_trusted_ca = true">清除 CA</n-button>
              </div>
            </n-collapse-item>
          </n-collapse>
          <input ref="certificateFileInput" class="file-input" type="file" accept=".pem,.crt,.cer,.key,.txt" @change="loadTLSFile('certificate', $event)" />
          <input ref="keyFileInput" class="file-input" type="file" accept=".pem,.key,.txt" @change="loadTLSFile('key', $event)" />
          <input ref="caFileInput" class="file-input" type="file" accept=".pem,.crt,.cer,.ca,.txt" @change="loadTLSFile('ca', $event)" />
        </div>
        <n-collapse class="advanced-collapse">
          <n-collapse-item title="高级连接设置" name="advanced">
            <div class="form-grid">
              <n-form-item label="代理 URL"><n-input v-model:value="serverForm.options.proxy_url" placeholder="http://user:pass@192.168.1.1:8080" /></n-form-item>
              <n-form-item label="连接池数量"><n-input-number v-model:value="serverForm.options.pool_count" :min="0" :max="100" /></n-form-item>
              <n-form-item label="拨号超时（秒）"><n-input-number v-model:value="serverForm.options.dial_server_timeout" :min="0" :max="600" /></n-form-item>
              <n-form-item label="TCP 保活（秒）"><n-input-number v-model:value="serverForm.options.dial_server_keepalive" :min="0" :max="86400" /></n-form-item>
              <n-form-item label="心跳间隔（秒）"><n-input-number v-model:value="serverForm.options.heartbeat_interval" :min="0" :max="3600" /></n-form-item>
              <n-form-item label="心跳超时（秒）"><n-input-number v-model:value="serverForm.options.heartbeat_timeout" :min="0" :max="3600" /></n-form-item>
              <n-form-item label="TCP Mux 保活（秒）"><n-input-number v-model:value="serverForm.options.tcp_mux_keepalive_interval" :min="0" :max="3600" /></n-form-item>
              <n-form-item label="指定本地源 IP"><n-input v-model:value="serverForm.options.connect_server_local_ip" placeholder="可选" /></n-form-item>
              <n-form-item label="DNS 服务器"><n-input v-model:value="serverForm.options.dns_server" placeholder="可选，例如：1.1.1.1" /></n-form-item>
            </div>
            <div class="switch-row"><span>TCP 多路复用</span><n-switch v-model:value="serverForm.options.tcp_mux" /><span>登录失败即退出</span><n-switch v-model:value="serverForm.options.login_fail_exit" /><span>自动连接</span><n-switch v-model:value="serverForm.options.auto_start" /></div>
          </n-collapse-item>
        </n-collapse>
      </section>
            </div>
            <div class="modal-actions">
              <n-button @click="showServerModal = false">取消</n-button>
              <n-button v-if="editingServer?.agent_configured" :loading="savingServer" @click="saveServer(true)">保存并下发</n-button>
              <n-button type="primary" :loading="savingServer" @click="saveServer(false)">保存</n-button>
            </div>
          </n-form>
        </div>
        </template>
      </div>
      <div class="proxy-modal__help">
        <h4>{{ serverHelpTitle }}</h4>
        <ol>
          <li v-for="item in serverHelpItems" :key="item.title"><strong>{{ item.title }}</strong>：{{ item.text }}</li>
        </ol>
        <div class="proxy-modal__tip">
          <n-icon :component="InformationCircleOutline" class="proxy-modal__tip-icon" />
          <span>{{ serverHelpTip }}</span>
        </div>
      </div>
    </div>
  </n-modal>

  <n-modal v-model:show="showServerLogs">
    <div class="frp-card-modal">
      <div class="proxy-modal__header">
        <h3 class="modal-title">frps 服务日志 - {{ serverLogsTarget?.name ?? '' }}</h3>
        <n-button size="small" quaternary @click="showServerLogs = false">关闭</n-button>
      </div>
      <div class="proxy-modal__scroll">
    <div class="logs-toolbar">
      <span class="table-muted">VPS 上的 frps 服务日志（journalctl，最近 20 行、新的在上）· {{ serverLogsTarget?.server_addr }}:{{ serverLogsTarget?.server_port }}</span>
    </div>
      <pre class="logs-box">{{ agentStatus?.frps_log_tail || '暂无 frps 日志：可能是 VPS 上 frps 未启动，或 agent 版本低于 0.12.3' }}</pre>
      </div>
      <div class="modal-footer"><n-button @click="showServerLogs = false">关闭</n-button></div>
    </div>
  </n-modal>

  <n-modal v-model:show="showServerDetail" preset="card" class="frp-detail-card" :title="viewingServer?.name || '服务端详情'" :style="{ width: 'min(980px, 94vw)', maxHeight: 'calc(100vh - 40px)' }" :content-style="{ maxHeight: 'calc(100vh - 120px)', overflowY: 'auto' }">
    <div v-if="viewingServer" class="server-detail">
      <div class="server-detail__top"><div><h2>{{ viewingServer.name }}</h2><p>服务端详情和运行监测</p></div><n-button v-if="viewingServer.boot_status === 'running'" size="small" type="error" ghost @click="stopServer(viewingServer)"><template #icon><n-icon :component="StopOutline" /></template>停止</n-button><n-button v-else size="small" type="success" ghost @click="startServer(viewingServer)"><template #icon><n-icon :component="PlayOutline" /></template>启动</n-button><n-button size="small" @click="openFRPSConfig(viewingServer)"><template #icon><n-icon :component="DocumentOutline" /></template>frps.toml</n-button></div>
      <div class="server-detail__grid">
        <section class="server-detail__panel"><h3>基本信息</h3><div class="detail-fields"><div><label>主机地址</label><strong class="mono">{{ viewingServer.server_addr }}</strong></div><div><label>端口</label><strong>{{ viewingServer.server_port }}</strong></div><div><label>用户</label><strong>{{ viewingServer.options.user || '未设置' }}</strong></div><div><label>位置</label><strong class="server-detail__location"><n-icon :component="LocationOutline" /> {{ locationText(viewingServer) }}</strong></div><div><label>状态</label><StatusBadge :kind="serverStartupKind(viewingServer)" :text="serverStartupText(viewingServer)" /></div><div><label>协议</label><strong>{{ viewingServer.options.protocol.toUpperCase() }}</strong></div><div><label>创建时间</label><span>{{ formatDate(viewingServer.created_at) }}</span></div><div><label>最后更新</label><span>{{ formatDate(viewingServer.updated_at) }}</span></div></div></section>
        <section class="server-detail__panel">
          <h3>配置</h3>
          <div class="detail-fields detail-fields--single">
            <div><label>描述</label><span>{{ viewingServer.options.remark || '未提供描述。' }}</span></div>
          </div>
          <div class="detail-subgroup">
            <div class="detail-subgroup__title">连接与认证</div>
            <div class="detail-fields">
              <div><label>TLS</label><n-tag size="small" :type="viewingServer.tls_enabled ? 'success' : 'warning'" :bordered="false">{{ viewingServer.tls_enabled ? '已启用' : '未启用' }}</n-tag></div>
              <div><label>认证</label><span>{{ authMethodText(viewingServer.options.auth_method) }}{{ viewingServer.has_token ? '（Token 已配置）' : '' }}</span></div>
              <div><label>客户端</label><span>{{ runtime.active_version || runtime.detected_version || '当前版本' }}</span></div>
              <div><label>期望状态</label><span>{{ serverStartupText(viewingServer) }}</span></div>
              <div><label>frpc 进程</label><span>{{ processStateText(viewingServer) }}{{ viewingServer.process_pid ? `（PID ${viewingServer.process_pid}）` : '' }}</span></div>
              <div><label>FRP 连接</label><span>{{ connectionStateText(viewingServer) }}</span></div>
              <div><label>状态来源</label><span>{{ viewingServer.state_source || '未知' }}</span></div>
            </div>
          </div>
          <div class="detail-subgroup">
            <div class="detail-subgroup__title">诊断</div>
            <span v-if="(serverDiagnostics[viewingServer.id]?.checks ?? []).length" class="diag-checks"><n-tag v-for="check in serverDiagnostics[viewingServer.id]?.checks ?? []" :key="check.name" size="small" :type="checkTagType(check.state)" :bordered="false" :title="check.detail">{{ checkLabel(check.name) }}·{{ checkStateText(check.state) }}</n-tag></span>
            <span v-else class="detail-muted">暂无诊断数据，可点「立即诊断」获取</span>
          </div>
        </section>
      </div>
      <section class="server-detail__panel server-status-panel"><div class="server-detail__section-title"><h3>服务器状态</h3></div><div class="server-status-summary"><div class="server-status-summary__item server-status-summary__item--state"><StatusBadge :kind="detailStatusKind" :text="detailStatusText" /></div><div class="server-status-summary__item"><label>可达率</label><strong>{{ detailAvailability.toFixed(1) }}%</strong></div><div class="server-status-summary__item"><label>平均延迟</label><strong>{{ detailAverageLatency }}ms</strong></div><div class="server-status-summary__item"><label>峰值延迟</label><strong>{{ detailPeakLatency }}ms</strong></div></div><div v-if="detailStatusBars.length" class="status-bars"><div v-for="(sample, index) in detailStatusBars" :key="`${sample.checked_at}-${index}`" class="status-bar" :class="sample.available ? 'status-bar--ok' : 'status-bar--failed'" :data-tooltip="statusBarTooltip(sample)" :aria-label="statusBarTooltip(sample)" role="img" tabindex="0" /></div><div v-else class="status-bars-empty">暂无最近 30 分钟诊断采样</div><div class="status-bars-footer"><span>30 分钟前</span><span><i /> 现在</span></div><div class="detail-status-meta"><span>最近检测：{{ formatDate(detailStatus.last_checked_at) || '尚未检测' }}</span><span v-if="detailStatus.last_error" class="detail-status-error">{{ detailStatus.last_error }}</span></div></section>
      <section class="server-detail__panel latency-panel"><div class="server-detail__section-title"><h3>延迟曲线</h3><span class="latency-panel__hint">最近 30 分钟</span></div><VChart v-if="detailSamples.length" class="latency-chart" autoresize :option="detailChartOption" /><div v-else class="detail-empty">暂无最近 30 分钟诊断采样</div><div class="detail-chart-footer"><span>延迟（ms）</span><span><i /> 可连接 <b class="detail-legend-failed" /> 失败</span></div></section>
      <section class="server-detail__panel"><div class="server-detail__section-title"><h3>代理</h3><n-button size="small" quaternary @click="loadDetailData"><template #icon><n-icon :component="RefreshOutline" /></template>刷新</n-button></div><n-data-table :columns="detailProxyColumns" :data="detailProxies" :bordered="false" :single-line="false" size="small" /></section>
      <section class="server-detail__panel"><div class="server-detail__section-title"><h3>连接日志</h3><n-button size="small" quaternary @click="loadDetailData">刷新</n-button></div><pre class="detail-logs">{{ detailLogs.length ? detailLogs.join('\n') : '暂无该服务端连接日志' }}</pre></section>
    </div>
  </n-modal>

  <n-modal v-model:show="showFRPSConfig">
    <div class="frp-card-modal">
      <div class="proxy-modal__header">
        <h3 class="modal-title">frps.toml 同源配置 - {{ frpsConfigTarget?.name || '' }}</h3>
        <n-button size="small" quaternary @click="showFRPSConfig = false">关闭</n-button>
      </div>
      <div class="proxy-modal__scroll">
        <p class="form-hint">与该服务端的 frpc 配置同源生成（bindPort / 认证 / 管理接口一致）；复制到 VPS 保存为 frps.toml 后执行 ./frps -c frps.toml。包含真实 Token，请勿外传。</p>
                <p v-if="tcpRemotePorts.length" class="form-hint">
          已启用的 TCP/UDP 隧道远程端口：<span class="mono">{{ tcpRemotePorts.join('、') }}</span>
          —— 请确认 VPS 防火墙 / 安全组已放行这些端口（frps 侧 allowPorts 已按此生成，未放行时隧道会连不上）。
        </p>
        <p v-if="frpsCompare" class="form-hint" :class="{ 'frps-compare--warn': !frpsCompare.ok }">
          {{ frpsCompare.text }}
        </p>
        <pre class="logs-box">{{ frpsConfigContent || '生成中…' }}</pre>
      </div>
      <div class="modal-footer">
        <n-button size="small" :loading="comparingFrps" :disabled="!frpsConfigTarget" @click="compareFrpsConfig">
          比对 VPS 实际文件
        </n-button>
        <n-button @click="showFRPSConfig = false">关闭</n-button>
      </div>
    </div>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import type { DataTableColumns } from 'naive-ui'
import { NButton, NDropdown, NIcon, NTag, useDialog, useMessage } from 'naive-ui'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { LineChart, ScatterChart } from 'echarts/charts'
import { CanvasRenderer } from 'echarts/renderers'
import {
  AddOutline, CheckmarkCircleOutline, CloseCircleOutline, CloseOutline, CloudOutline, DocumentOutline,
  DocumentTextOutline, EllipsisHorizontalOutline, FlashOutline, GitNetworkOutline, InformationCircleOutline,
  LayersOutline, LocationOutline, LockClosedOutline, PlayOutline, RefreshOutline, ServerOutline, StopOutline,
  SwapHorizontalOutline,
} from '@vicons/ionicons5'
import EmptyState from '../components/EmptyState.vue'
import ConfiguredSecretField from '../components/ConfiguredSecretField.vue'
import HavlineCard from '../components/HavlineCard.vue'
import LoadError from '../components/LoadError.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { CONFIGURED_SECRET_TAG } from '../constants/secretField'
import { api, asList } from '../api/client'
import type {
  FrpAgentStatus, FrpProxy, FrpRelease, FrpRuntimeInfo, FrpServer, FrpServerDetail, FrpServerDiagnostics,
  FrpServerOptions, FrpServerPayload, FrpServerStatusDetail,
} from '../api/types'
import { formatBytes, formatDate, formatRelativeTime, formatUptime } from '../utils/format'
import type { StatusKind } from '../utils/status'
import { copyText } from '../utils/clipboard'
import { agentVersionText } from '../utils/agent'
import { useVisibilityPolling } from '../composables/useVisibilityPolling'

use([CanvasRenderer, LineChart, ScatterChart, GridComponent, TooltipComponent])

// 操作动作：按动作粒度控制 loading，保证"只有被点的按钮转圈"
type Action =
  | 'probe' | 'agent-install' | 'agent-uninstall' | 'diagnose'
  | 'tunnel-restart' | 'transport-https' | 'transport-tunnel'
  | 'tls-rotate' | 'firewall-check' | 'firewall-allow'
  | 'frps-start' | 'frps-stop' | 'frps-restart' | 'frps-install' | 'push-config'

interface OpOutput { at: string; source: string; text: string; ok: boolean }

const message = useMessage()
const dialog = useDialog()
const servers = ref<FrpServer[]>([])
// 深链：/frp-agent?server=<id> 直接选中该服务端（来自内网穿透页服务端行的「Agent 管理」）；
// id 不在列表里时由 load() 回落为第一台
const route = useRoute()
function serverIdFromQuery(): number | null {
  const raw = Number(route.query.server ?? '')
  return Number.isInteger(raw) && raw > 0 ? raw : null
}
const selectedId = ref<number | null>(serverIdFromQuery())
const loadError = ref('')
const agentStatus = ref<FrpAgentStatus | null>(null)
const agentStatusOk = ref(false)
const statusLoaded = ref(false)
const statusLoading = ref(false)
const busy = ref<Action | null>(null)
const firewallInfo = ref<{ serverId: number; data: Record<string, any> } | null>(null)
const opOutput = ref<OpOutput[]>([])
const frpsInstallVersion = ref<string | null>(null)
const releases = ref<FrpRelease[]>([])
const releasesLoading = ref(false)
const configModal = ref(false)
const configContent = ref('')
const configLoading = ref(false)

const watchedPorts = ['80', '443', '7000', '8080'] as const

// 端口含义（P0-1）：让端口矩阵直接可读，不用再去查哪个端口是谁的
const portMeanings: Record<string, string> = {
  '80': '公网 HTTP 入口（Nginx）',
  '443': '公网 HTTPS 入口（Nginx）',
  '7000': 'frps 控制端口（frpc 连接用）',
  '8080': 'frps vhost HTTP 端口（Nginx 回源到 frps）',
}

function portMeaning(port: string) {
  return `${portMeanings[port] ?? '端口'}：${portOn(port) ? '正在监听' : '未监听'}`
}

// frps 运行一致性（P0-3）：systemd 实际执行的启动命令，是否用的是 Havline 下发的二进制与配置文件。
// 不一致意味着：手工改过 unit，或者 Havline 写入的 frps.toml 并不是 frps 实际加载的那一份。
const frpsConsistency = computed(() => {
  const exec = agentStatus.value?.frps_unit_exec ?? ''
  if (!exec) return { ok: false, kind: 'warning' as const, text: '未获取', detail: '' }
  const binary = agentStatus.value?.frps_binary ?? ''
  const config = agentStatus.value?.frps_config_path ?? ''
  const problems: string[] = []
  if (binary && !exec.includes(binary)) problems.push(`启动命令未使用配置的二进制（${binary}）`)
  if (config && !exec.includes(config)) problems.push(`启动命令未使用 Havline 下发的配置文件（${config}）`)
  if (problems.length > 0) {
    return { ok: false, kind: 'warning' as const, text: '不一致', detail: problems.join('；') }
  }
  return { ok: true, kind: 'success' as const, text: '一致', detail: '启动命令与配置的二进制、配置文件一致' }
})

const server = computed(() => servers.value.find((item) => item.id === selectedId.value) ?? null)
const installedFrpsVersion = computed(() => (agentStatus.value?.frps_version || '').trim())
const routesCount = computed(() => agentStatus.value?.routes?.length ?? 0)
const frpsVersionOptions = computed(() => releases.value.map((release) => ({ label: release.version, value: release.version })))

// 主区页签：只改变信息呈现顺序，不改变任何数据加载逻辑
type AgentTab = 'overview' | 'deps' | 'control' | 'logs'
const activeTab = ref<AgentTab>('overview')

// 状态总览卡：先给结论，再给传输方式和地址，避免首屏需要在多张指标卡之间来回扫视
const agentHeroClass = computed(() => {
  if (!statusLoaded.value) return 'agent-hero--pending'
  if (agentStatusOk.value) return 'agent-hero--on'
  if (agentStatus.value?.configured === false || agentStatus.value?.mgmt_enabled === false) return 'agent-hero--warn'
  return 'agent-hero--off'
})
const agentHeroTitle = computed(() => {
  if (!server.value) return '未选择服务端'
  if (!statusLoaded.value) return statusLoading.value ? '正在读取 Agent 状态' : '等待读取 Agent 状态'
  if (agentStatus.value?.configured === false) return '尚未安装 Agent'
  if (agentStatus.value?.mgmt_enabled === false) return 'Agent 管理未启用'
  if (agentStatus.value?.token_mismatch) return 'Agent Token 不匹配'
  return agentStatusOk.value ? 'Agent 已连接' : 'Agent 未连接'
})
const agentHeroSubtitle = computed(() => {
  const current = server.value
  if (!current) return ''
  return `${agentTransportText(current)} · ${agentTransportStateText(current)} · ${current.agent_url || '未配置 Agent 地址'}`
})

// 更多操作菜单：把低频操作收进下拉，只保留启动/停止与编辑在主工具条
const serverMoreOptions = computed(() => [
  { label: '查看详情', key: 'detail' },
  { label: '测速', key: 'measure' },
  { label: '校验', key: 'test' },
  { label: '运行日志', key: 'logs' },
  { type: 'divider', key: 'divider' },
  { label: '删除服务端', key: 'delete' },
])

function handleServerMoreSelect(key: string | number) {
  const current = server.value
  if (!current) return
  if (key === 'detail') return openServerDetail(current)
  if (key === 'measure') return void measureServer(current)
  if (key === 'test') return void testServer(current)
  if (key === 'logs') return openServerLogs(current)
  if (key === 'delete') return confirmDeleteServer(current)
}

// O：最后检查时间（PageHeader 副标题，判断数据新鲜度）
const headerDescription = computed(() => {
  if (!server.value) return '管理公网 VPS 服务端（frps）与其上的 havline-agent'
  if (!statusLoaded.value) return '正在读取 VPS 状态…'
  const checkedAt = agentStatus.value?.checked_at
  return checkedAt ? `管理部署在公网 VPS 上的 havline-agent 与 frps · 检查于 ${formatRelativeTime(checkedAt)}` : '管理部署在公网 VPS 上的 havline-agent 与 frps 服务'
})

// 三态：未加载 / 加载中 / 失败——避免把"没拉到"显示成"未连接"
const agentConnectionText = computed(() => {
  if (!statusLoaded.value) return statusLoading.value ? '读取中…' : '—'
  if (agentStatus.value?.configured === false) return '未配置'
  if (agentStatus.value?.token_mismatch) return 'Token 不匹配'
  return agentStatusOk.value ? '在线' : '未连接'
})
const frpsStateText = computed(() => {
  if (!statusLoaded.value) return '—'
  return agentStatus.value?.frps_active ? '运行中' : '未运行'
})
const portIconClass = computed(() => {
  if (!statusLoaded.value) return 'stat-card__icon--blue'
  return watchedPorts.some((port) => portOn(port)) ? 'stat-card__icon--green' : 'stat-card__icon--amber'
})
const nginxKind = computed<'success' | 'warning' | 'error' | 'disabled'>(() => {
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

// 状态提示：互斥，最多一条（error 未连接 > warning frps 未运行）
const statusAlert = computed<{ type: 'error' | 'warning'; text: string; action?: { label: string; run: () => void } } | null>(() => {
  const current = server.value
  if (!current || !statusLoaded.value) return null
  if (!agentStatusOk.value) {
    if (agentStatus.value?.configured === false) {
      return {
        type: 'warning',
        text: '尚未安装 Agent：请先填写 SSH 信息，再点击下方「安装 Agent」。',
        action: { label: '安装 Agent', run: () => installAgent(current) },
      }
    }
    if (agentStatus.value?.mgmt_enabled === false) {
      return {
        type: 'warning',
        text: 'Agent 已配置，但「启用 Agent 日常管理」未开启。',
        action: { label: '编辑服务端', run: () => openServerModal(current) },
      }
    }
    if (agentStatus.value?.token_mismatch) {
      return {
        type: 'error',
        text: 'Agent Token 不匹配：Havline 保存的 Token 与 VPS 上的 Agent 配置不一致。',
        action: { label: '升级 Agent', run: () => installAgent(current) },
      }
    }
    return {
      type: 'error',
      text: 'Agent 未连接：请确认 VPS 上 havline-agent 正在运行，且 Agent 地址与 Token 与页面一致。',
      action: { label: '探测环境', run: () => probe(current) },
    }
  }
  if (!agentStatus.value?.frps_active) {
    return {
      type: 'warning',
      text: 'frps 未运行：反代与隧道都不会生效，可先启动；未安装则用下方「安装 / 升级 frps」。',
      action: { label: '启动 frps', run: () => frpsAction(current, 'start') },
    }
  }
  return null
})

const dashboardURL = computed(() => {
  const current = server.value
  if (!current) return ''
  const addr = (current.options.dashboard_addr || '').trim()
  const port = current.options.dashboard_port || 0
  if (!addr || !port) return ''
  const host = addr === '0.0.0.0' || addr === '::' ? (current.server_addr || '127.0.0.1') : addr
  return `http://${host}:${port}`
})

function portOn(port: string) {
  return agentStatus.value?.ports?.[port] === true
}

function stateClass(ok: boolean, loaded: boolean) {
  if (!loaded) return 'stat-card__icon--blue'
  return ok ? 'stat-card__icon--green' : 'stat-card__icon--red'
}

function pushOutput(source: string, text: string, ok = true) {
  const trimmed = (text || '').trim()
  if (!trimmed) return
  opOutput.value = [{ at: new Date().toISOString(), source, text: trimmed, ok }, ...opOutput.value].slice(0, 20)
}

// 统一的操作执行器：按动作粒度 loading + 串行化（按钮已 disabled，这里只是安全网）
async function run<T>(action: Action, source: string, fn: () => Promise<T>, describe?: (result: T) => { text: string; ok: boolean }) {
  if (busy.value) return
  busy.value = action
  try {
    const result = await fn()
    const described = describe ? describe(result) : { text: '操作完成', ok: true }
    pushOutput(source, described.text, described.ok)
    if (!described.ok) message.error(`${source}失败，详见「输出与日志」`)
    return result
  } catch (error) {
    const text = error instanceof Error ? error.message : `${source}失败`
    pushOutput(source, text, false)
    message.error(`${source}失败，详见「输出与日志」`)
    return undefined
  } finally { busy.value = null }
}

async function load() {
  loadError.value = ''
  try {
    // 本页现在是服务端的「家」：列表展示全部服务端（未配 Agent 的也要能编辑 / 删除）
    const [nextServers, nextProxies] = await Promise.all([api.listFrpServers(), api.listFrpProxies()])
    servers.value = asList(nextServers)
    proxies.value = asList(nextProxies)
    if (selectedId.value === null || !servers.value.some((item) => item.id === selectedId.value)) {
      selectedId.value = servers.value[0]?.id ?? null
    }
    void loadServerDiagnostics(servers.value)
    void loadRuntimeVersion()
    if (server.value) await refreshAll({ silent: false })
    void loadHavlineCerts()
  } catch (error) { loadError.value = error instanceof Error ? error.message : '读取服务端失败' }
}

// VPS 资源（需 agent ≥ 0.10.0）：老版本 agent 没有该端点，错误信息里已带升级提示
const agentMetrics = ref<import('../api/types').FrpAgentMetrics | null>(null)
const metricsError = ref('')
const metricsLoading = ref(false)

async function loadMetrics(serverID: number) {
  metricsLoading.value = true
  try {
    agentMetrics.value = await api.getFrpAgentMetrics(serverID)
    metricsError.value = ''
  } catch (error) {
    agentMetrics.value = null
    metricsError.value = error instanceof Error ? error.message : 'VPS 资源获取失败'
  } finally {
    metricsLoading.value = false
  }
}

function usagePercentOf(used?: number, total?: number) {
  if (!used || !total || total <= 0) return 0
  return Math.min(100, (used / total) * 100)
}

const metricsRows = computed(() => {
  const data = agentMetrics.value
  if (!data) return []
  const rows = [
    { label: 'CPU', percent: Math.min(100, Math.max(0, data.cpu_percent ?? 0)), detail: '整机占用率' },
    {
      label: '内存',
      percent: usagePercentOf(data.mem_used_bytes, data.mem_total_bytes),
      detail: `${formatBytes(data.mem_used_bytes)} / ${formatBytes(data.mem_total_bytes)}`,
    },
    {
      label: '磁盘',
      percent: usagePercentOf(data.disk_used_bytes, data.disk_total_bytes),
      detail: `${formatBytes(data.disk_used_bytes)} / ${formatBytes(data.disk_total_bytes)}`,
    },
  ]
  return rows.map((row) => ({
    ...row,
    value: `${Math.round(row.percent)}%`,
    tone: row.percent >= 90 ? 'danger' : row.percent >= 75 ? 'warn' : 'ok',
  }))
})

const metricsLoadText = computed(() =>
  agentMetrics.value?.load_avg?.length ? agentMetrics.value.load_avg.map((value) => value.toFixed(2)).join(' / ') : '—',
)

const metricsUptimeText = computed(() =>
  agentMetrics.value?.uptime_seconds ? formatUptime(agentMetrics.value.uptime_seconds) : '—',
)

// VPS 证书清单（需 agent ≥ 0.11.0）：与 Havline 证书库交叉核对，宝塔自签的证书也能看到
const agentCerts = ref<import('../api/types').FrpAgentCertificate[]>([])
const certWarnings = ref<string[]>([])
const certsError = ref('')
const certsLoading = ref(false)
const havlineCertDomains = ref<Set<string>>(new Set())

async function loadAgentCerts(serverID: number) {
  certsLoading.value = true
  try {
    const result = await api.getFrpAgentCerts(serverID)
    agentCerts.value = result.certs ?? []
    certWarnings.value = result.warnings ?? []
    certsError.value = ''
  } catch (error) {
    agentCerts.value = []
    certWarnings.value = []
    certsError.value = error instanceof Error ? error.message : 'VPS 证书读取失败'
  } finally {
    certsLoading.value = false
  }
}

// Havline 证书库里的域名（含通配符），用于判断「VPS 上有但 Havline 没有」
async function loadHavlineCerts() {
  try {
    const records = await api.listCertificates()
    const domains = new Set<string>()
    for (const record of records ?? []) {
      domains.add(record.domain)
      for (const item of record.domains ?? []) domains.add(item)
    }
    havlineCertDomains.value = domains
  } catch {
    havlineCertDomains.value = new Set()
  }
}

// Nginx 显式重载（需 agent ≥ 0.11.0）：把校验 / 重载失败原因直接显示出来
const reloadingNginx = ref(false)
const reloadHint = ref('')
// Nginx 启停（需 agent ≥ 0.15.2）：按钮文案随进程状态切换
const actingNginx = ref(false)

async function reloadNginx() {
  const target = server.value
  if (!target) return
  reloadingNginx.value = true
  reloadHint.value = ''
  try {
    const result = await api.reloadFrpAgentNginx(target.id)
    if (result.ok) {
      reloadHint.value = 'Nginx 校验通过并已重载'
    } else if (!result.test_ok) {
      reloadHint.value = `Nginx 配置校验失败：${result.test_error || '未返回原因'}`
    } else {
      reloadHint.value = `Nginx 重载失败：${result.reload_error || '未返回原因'}`
    }
  } catch (error) {
    reloadHint.value = error instanceof Error ? error.message : '重载失败'
  } finally {
    reloadingNginx.value = false
  }
}

// 启停后刷新状态，让卡片上的「进程」与按钮文案同步
async function nginxAction(action: 'start' | 'stop' | 'restart') {
  const target = server.value
  if (!target) return
  actingNginx.value = true
  reloadHint.value = ''
  try {
    const result = await api.actionFrpAgentNginx(target.id, action)
    await refreshAll({ silent: true })
    reloadHint.value = result.nginx_running ? 'Nginx 已启动' : 'Nginx 已停止'
  } catch (error) {
    reloadHint.value = error instanceof Error ? error.message : 'Nginx 操作失败'
  } finally {
    actingNginx.value = false
  }
}

// 轮询走静默（不触碰 busy）；首次加载显示 statusLoading
async function refreshAll(options: { silent?: boolean } = { silent: true }) {
  const current = server.value
  if (!current) return
  if (options.silent === false) statusLoading.value = true
  try {
    const status = await api.getFrpAgentStatus(current.id)
    agentStatus.value = status
    agentStatusOk.value = status.configured !== false && status.mgmt_enabled !== false && status.token_mismatch !== true
    statusLoaded.value = true
    if (agentStatusOk.value) {
      // 资源采集单独走，不阻断状态加载
      void loadMetrics(current.id)
      void loadAgentCerts(current.id)
    } else {
      agentMetrics.value = null
      metricsError.value = ''
      havlineCertDomains.value = new Set()
    }
  } catch (error) {
    agentStatusOk.value = false
    statusLoaded.value = true
    if (options.silent === false) {
      message.error(error instanceof Error ? error.message : 'Agent 状态获取失败')
    }
  } finally { statusLoading.value = false }
}

function probe(target: FrpServer) {
  return run('probe', '探测环境', () => api.probeFrpAgent(target.id), (result) => ({
    text: Object.entries(result as Record<string, unknown>).map(([key, value]) => `${key}: ${value}`).join('\n'),
    ok: true,
  }))
}

function isSSHHostKeyChangedError(error: unknown) {
  return error instanceof Error && error.message.includes('SSH 主机密钥与首次记录不一致')
}

function confirmTrustSSHHostKey(): Promise<boolean> {
  return new Promise((resolve) => {
    dialog.warning({
      title: 'SSH 主机密钥已变化',
      content: '该服务器可能已重装系统。确认新主机密钥可信后，Havline 将更新记录并重新安装 Agent。',
      positiveText: '信任并重装',
      negativeText: '取消',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
    })
  })
}

async function installAgentWithTrust(target: FrpServer) {
  try {
    return await api.installFrpAgent(target.id)
  } catch (error) {
    if (!isSSHHostKeyChangedError(error)) throw error
    if (!(await confirmTrustSSHHostKey())) {
      throw new Error('已取消信任新的 SSH 主机密钥')
    }
    return api.installFrpAgent(target.id, true)
  }
}

function installAgent(target: FrpServer) {
  return run('agent-install', target.agent_configured ? '升级 Agent' : '安装 Agent',
    () => installAgentWithTrust(target),
    () => ({ text: 'Agent 安装/升级完成，已自动回填 Agent 地址与 Token', ok: true }))
    .then(async (result) => { if (result !== undefined) await load() })
}

function uninstallAgent(target: FrpServer, keepData: boolean) {
  const source = keepData ? '卸载 Agent（保留路由/证书）' : '卸载 Agent（含数据）'
  return run('agent-uninstall', source, () => api.uninstallFrpAgent(target.id, keepData),
    (result) => ({ text: result?.output || '已卸载', ok: true }))
    .then(async (result) => { if (result !== undefined) await load() })
}

function diagnoseAgent(target: FrpServer) {
  return run('diagnose', 'SSH 诊断', () => api.sshDiagnoseFrpAgent(target.id),
    (result) => ({ text: result?.output || '无输出', ok: true }))
}

function frpsAction(target: FrpServer, action: 'start' | 'stop' | 'restart') {
  const actionText = action === 'stop' ? '停止' : action === 'start' ? '启动' : '重启'
  return run(`frps-${action}` as Action, `${actionText} frps`,
    () => api.frpsServerActionRaw(target.id, action),
    (result) => result && result.ok === false
      ? { text: result.error || `frps ${actionText}后未进入预期状态`, ok: false }
      : { text: `frps 已${actionText}`, ok: true })
    .then(async () => { await refreshAll() })
}

function pushAgentConfig(target: FrpServer) {
  return run('push-config', '下发 frps 配置', () => api.pushFrpConfigToAgent(target.id),
    () => ({ text: 'frps 同源配置已下发到 VPS，点「重启 frps」后生效', ok: true }))
}

// 配置查看：只读，展示 Havline 本地生成的 frps.toml（即「下发」会写入 VPS 的内容）
async function viewConfig(target: FrpServer) {
  configLoading.value = true
  try {
    const result = await api.getFrpFRPSConfig(target.id)
    configContent.value = result?.content || ''
    configModal.value = true
  } catch (error) {
    message.error(error instanceof Error ? error.message : '读取 frps 配置失败')
  } finally { configLoading.value = false }
}

async function copyConfig() {
  if (await copyText(configContent.value)) message.success('已复制 frps 配置')
  else message.error('复制失败，请手动选择文本复制')
}

function installFrps(target: FrpServer) {
  const version = frpsVersionOptions.value[0]?.value ? (frpsInstallVersion.value?.trim() || frpsVersionOptions.value[0].value) : ''
  return run('frps-install', '安装 / 升级 frps',
    () => api.installFrpServer(target.id, version, ''),
    (result) => {
      const notes = Array.isArray(result?.notes) ? result.notes.join('\n') : ''
      return { text: `frps ${result?.version ?? version} 安装完成\n${notes}`.trim(), ok: true }
    })
    .then(async () => { await refreshAll() })
}

function openDashboard() {
  if (dashboardURL.value) window.open(dashboardURL.value, '_blank', 'noopener')
}

async function loadReleases() {
  if (releases.value.length) return
  releasesLoading.value = true
  try { releases.value = await api.listFrpReleases() } catch { /* 拉取失败时按钮保持禁用并提示 */ }
  finally {
    releasesLoading.value = false
    if (!frpsInstallVersion.value && releases.value.length) frpsInstallVersion.value = releases.value[0].version
  }
}

// 切换服务端必须重置，避免上一台的状态与输出串台
watch(selectedId, () => {
  activeTab.value = 'overview'
  opOutput.value = []
  agentStatus.value = null
  agentStatusOk.value = false
  statusLoaded.value = false
  void refreshAll({ silent: false })
})

// ── 公网服务端管理（列表 / 表单 / 日志 / 详情 / frps.toml 同源配置）──
// 2026-09-24 由内网穿透页（FrpView.vue）迁入：服务端的「家」在本页，列表与详情同屏。

const proxies = ref<FrpProxy[]>([])

// 服务端详情弹窗「客户端」一行显示本机 frpc 版本，与内网穿透页同源
const runtime = ref<FrpRuntimeInfo>({ platform: '', configured_bin: '', download_proxy: '', config_path: '', config_exists: false })

async function loadRuntimeVersion() {
  try { runtime.value = await api.getFrpRuntime() } catch { /* 读取失败时该行回落为「当前版本」 */ }
}

// 已启用的 TCP/UDP 隧道远程端口：用于提示在 VPS 防火墙放行（与 frps 的 allowPorts 同源）
const tcpRemotePorts = computed(() => proxies.value
  .filter((proxy) => proxy.enabled && (proxy.type === 'tcp' || proxy.type === 'udp') && proxy.remote_port)
  .map((proxy) => proxy.remote_port as number))

const serverDiagnostics = ref<Record<number, FrpServerDiagnostics>>({})

const testingId = ref<number | null>(null)
const serverStartingId = ref<number | null>(null)
const agentActionId = ref<number | null>(null)
const measuringId = ref<number | null>(null)
const savingServer = ref(false)
// 80/443 回源端口的二次确认结果，按弹窗会话记录，避免每次保存都重复弹窗
const confirmedPrivilegedVhost = ref('')

const showServerModal = ref(false)

const showServerDetail = ref(false)

// frps.toml 同源配置弹窗：按服务端生成，与 frpc 侧端口/认证一致
const showFRPSConfig = ref(false)
const frpsConfigContent = ref('')
// frps.toml 与 VPS 实际文件的比对结果（P1-3，需 agent ≥ 0.10.0）
const frpsCompare = ref<{ ok: boolean; text: string } | null>(null)
const comparingFrps = ref(false)

// 比对前统一去注释、空行与首尾空白：注释和排版差异不算「被手工修改」
function normalizedConfigLines(text: string) {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '' && !line.startsWith('#'))
}

async function compareFrpsConfig() {
  const target = frpsConfigTarget.value
  if (!target) return
  comparingFrps.value = true
  frpsCompare.value = null
  try {
    const remote = await api.getFrpAgentFRPSConfig(target.id)
    if (!remote.exists) {
      frpsCompare.value = { ok: false, text: `VPS 上还没有 ${remote.path || 'frps.toml'}，可直接下发写入` }
      return
    }
    const expected = normalizedConfigLines(frpsConfigContent.value)
    const actual = normalizedConfigLines(remote.content)
    const index = expected.findIndex((line, i) => actual[i] !== line)
    if (index === -1 && expected.length === actual.length) {
      frpsCompare.value = { ok: true, text: `与 VPS 上的 ${remote.path} 一致（已忽略注释与空行）` }
      return
    }
    const at = index === -1 ? Math.min(expected.length, actual.length) : index
    frpsCompare.value = {
      ok: false,
      text: `不一致：第 ${at + 1} 行 下发「${expected[at] ?? '（无）'}」/ VPS「${actual[at] ?? '（无）'}」——VPS 上的 frps.toml 可能被手工改过，重新下发会覆盖`,
    }
  } catch (error) {
    frpsCompare.value = { ok: false, text: error instanceof Error ? error.message : '比对失败' }
  } finally {
    comparingFrps.value = false
  }
}
const frpsConfigTarget = ref<FrpServer | null>(null)
const frpsConfigLoading = ref(false)

const serverFormTab = ref<'basic' | 'advanced' | 'server' | 'remote'>('basic')

const editingServer = ref<FrpServer | null>(null)
const viewingServer = ref<FrpServer | null>(null)
const detailProxies = ref<FrpProxy[]>([])
const detailLogs = ref<string[]>([])
const detailStatus = ref<FrpServerStatusDetail>({ available: false, availability: 0, average_latency_ms: 0, peak_latency_ms: 0, samples: [] })

const certificateFileInput = ref<HTMLInputElement | null>(null)
const keyFileInput = ref<HTMLInputElement | null>(null)
const caFileInput = ref<HTMLInputElement | null>(null)

let diagnosticsAt = 0

function defaultServerOptions(): FrpServerOptions {
  return {
    user: '',
    location: '',
    auth_method: 'token', log_level: 'info', log_max_days: 3, protocol: 'tcp', dial_server_timeout: 10,
    dial_server_keepalive: 7200, pool_count: 0, tcp_mux: true, tcp_mux_keepalive_interval: 60,
    heartbeat_interval: 30, heartbeat_timeout: 90, login_fail_exit: true, tls_disable_custom_first_byte: false, auto_start: false,
    proxy_url: '', connect_server_local_ip: '', dns_server: '',
    vhost_http_port: 8080, vhost_https_port: 8443, sub_domain_host: '', custom_404_page: '',
    tls_force: false, allow_ports: '', max_ports_per_client: 0, tcpmux_http_connect_port: 0,
    frps_log_to_file: false, frps_log_max_days: 3,
    dashboard_addr: '', dashboard_port: 0, dashboard_user: '',
    oidc_client_id: '', oidc_audience: '', oidc_scope: '', oidc_token_endpoint_url: '',
    oidc_insecure_skip_verify: false, oidc_proxy_url: '', remark: '',
  }
}

const serverForm = reactive({
  name: '', server_addr: '', server_port: 7000, tls_enabled: true, tls_server_name: '', enabled: true, token: '',
  agent_url: '', agent_token: '', ssh_host: '', ssh_port: 22, ssh_user: 'root', ssh_auth: 'key', ssh_secret: '', mgmt_enabled: true,
  oidc_client_secret: '', dashboard_password: '', clear_dashboard_password: false, tls_certificate: '', tls_key: '', tls_trusted_ca: '', clear_tls_certificate: false,
  clear_tls_key: false, clear_tls_trusted_ca: false, options: defaultServerOptions(),
})

type ServerAddStep = 1 | 2 | 3
type AgentSetupMode = 'none' | 'ssh' | 'existing'
type ServerPreset = 'standard' | 'public-proxy' | 'existing-agent'

const addStep = ref<ServerAddStep>(1)
const agentSetupMode = ref<AgentSetupMode>('none')
const serverPreset = ref<ServerPreset>('standard')
const isCreatingServer = computed(() => editingServer.value === null)

const serverPresetOptions = [
  { label: '标准 frps', value: 'standard' },
  { label: '需要公网反代', value: 'public-proxy' },
  { label: '已有 Agent', value: 'existing-agent' },
]
const agentSetupOptions = [
  { label: '暂不安装 Agent', value: 'none' },
  { label: '通过 SSH 安装 Agent', value: 'ssh' },
  { label: '已有 Agent，手动填写地址', value: 'existing' },
]

const canProceedAddStep1 = computed(() => {
  if (!serverForm.name.trim()) return false
  if (!serverForm.server_addr.trim()) return false
  if (!serverForm.server_port || serverForm.server_port < 1 || serverForm.server_port > 65535) return false
  if (serverForm.options.auth_method === 'token' && !serverForm.token.trim()) return false
  if (serverForm.options.auth_method === 'token' && !serverForm.options.user?.trim()) return false
  if (serverForm.options.auth_method === 'oidc') {
    if (!serverForm.options.oidc_client_id?.trim() || !serverForm.options.oidc_token_endpoint_url?.trim() || !serverForm.oidc_client_secret.trim()) return false
  }
  return true
})

const canProceedAddStep2 = computed(() => {
  if (agentSetupMode.value === 'ssh') {
    return !!serverForm.ssh_host.trim() && !!serverForm.ssh_user.trim() && !!serverForm.ssh_secret.trim()
  }
  if (agentSetupMode.value === 'existing') return !!serverForm.agent_url.trim()
  return true
})

const addSummary = computed(() => [
  { label: '服务端', value: `${serverForm.name || '未命名'} · ${serverForm.server_addr || '未填写地址'}:${serverForm.server_port || '—'}` },
  { label: '认证', value: authMethodText(serverForm.options.auth_method) },
  { label: '用户', value: serverForm.options.auth_method === 'token' ? (serverForm.options.user || '未填写') : '—' },
  { label: 'TLS', value: serverForm.tls_enabled ? '已启用' : '未启用' },
  { label: '接入方式', value: agentSetupMode.value === 'none' ? '暂不安装 Agent' : agentSetupMode.value === 'ssh' ? '通过 SSH 安装 Agent' : '已有 Agent' },
])

const hasCustomTlsMaterial = computed(() => Boolean(
  editingServer.value?.options.tls_certificate_configured
  || editingServer.value?.options.tls_key_configured
  || editingServer.value?.options.tls_trusted_ca_configured,
))

const serverHelpItems = computed(() => {
  if (isCreatingServer.value) {
    if (addStep.value === 1) return [
      { title: '连接信息', text: '服务端名称只用于 Havline 内部识别；主机地址和端口对应 frps 的 bindAddr / bindPort，默认端口通常是 7000。' },
      { title: '认证方式', text: '必须与 frps 的 auth 配置一致；Token 模式填写 auth.user 和 auth.token，OIDC 模式填写客户端 ID、密钥和令牌端点。' },
      { title: 'TLS', text: '默认开启，用于加密 frpc 与 frps 的连接；如果 frps 明确关闭了 TLS，再手动关闭。' },
      { title: '快速预设', text: '标准 frps 适合只连接 frps；需要公网反代会保留 8080/8443 回源端口；已有 Agent 会直接进入 Agent 地址填写。' },
    ]
    if (addStep.value === 2) return [
      { title: '暂不安装 Agent', text: '只保存 frps 连接信息。之后可在本页补填 SSH 或 Agent 地址并安装。' },
      { title: '通过 SSH 安装 Agent', text: '需要 SSH 地址、用户和密码/私钥；安装成功后 Agent 地址和 Token 会自动回填。' },
      { title: '已有 Agent', text: '适合 VPS 上已经运行 Agent 的场景；填写 Agent API 地址和 Token 即可。' },
    ]
    return [
      { title: '摘要', text: '确认服务端地址、认证方式和接入方式无误后再保存。' },
      { title: '高级服务端参数', text: 'vhost 回源端口、端口白名单和 dashboard 都是可选项；不需要可以保持折叠。' },
      { title: '保存与下发', text: '已有 Agent 时可直接保存并下发 frps 配置；其它接入方式保存后再安装 Agent。' },
    ]
  }
  if (serverFormTab.value === 'advanced') return [
    { title: '日志', text: 'frpc 运行日志的级别与保留天数；排查连接问题时建议临时调到 debug。' },
    { title: '传输协议', text: 'TCP 兼容性最好；KCP/QUIC 在丢包链路上更流畅，WebSocket/WSS 适合经过反向代理或 CDN 的场景，需 frps 侧支持。' },
    { title: 'TLS', text: '默认启用；证书与密钥用于双向认证，受信任的 CA 用于校验 frps 证书，SNI 与 frps 侧证书域名一致。' },
    { title: '连接调优', text: '心跳与 TCP 多路复用保持默认即可；网络不稳定时可加大超时，连接池可降低高并发时的建连延迟。' },
  ]
  if (serverFormTab.value === 'server') return [
    { title: 'frps vhost 与域名反代', text: 'vhostHTTPPort/vhostHTTPSPort 是 Nginx 反代的回源端口，同时决定 subDomainHost、tcpmux 等域名入口；留空用默认 8080/8443。80/443 通常与 VPS 上的 Nginx 冲突，确需使用时保存前会二次确认，请先释放该端口。修改后需「下发 frps 配置」并重启 frps，再重新部署各规则。' },
    { title: 'TCP/UDP 端口分配', text: 'allowPorts 只限制 tcp/udp 规则使用的 remotePort，不控制 vhost 回源端口；实际端口仍需在云安全组与 VPS 主机防火墙放行。' },
    { title: 'frps 管理接口', text: 'frps 开启 webServer（dashboard）后填写，Havline 借此拉取穿透规则的实时连接与流量；地址与端口需成对填写，与连接认证无关。' },
  ]
  if (serverFormTab.value === 'remote') return [
    { title: '公网 Agent 管理', text: 'Agent 部署在 VPS 上，保存 HTTP/HTTPS 规则后自动生成反代；首次安装/升级需 SSH，日常管理走 Agent API，Token 与 SSH 凭据加密存储、不回显。' },
  ]
  const common = [
    { title: '基本信息', text: '主机地址和端口对应 frps 的 bindAddr / bindPort，多个服务端可并行配置，规则保存时选择归属。' },
    { title: '认证方式', text: '与 frps 的 auth 配置保持一致：无认证、令牌（auth.token）或 OIDC；令牌与密钥加密存储，编辑时留空保持不变。' },
    { title: '描述', text: '仅用于在 Havline 中区分多台服务端的备注信息。' },
  ]
  if (serverForm.options.auth_method === 'oidc') return [...common, { title: 'OIDC', text: '客户端 ID、密钥和令牌端点由身份提供商提供；受众与权限范围需与 frps 的 OIDC 校验配置匹配。' }]
  return common
})
const serverHelpTitle = computed(() => {
  if (isCreatingServer.value) {
    return ['连接 frps', '接入方式', '确认保存'][addStep.value - 1]
  }
  return { basic: '连接说明', advanced: '高级配置说明', server: '服务端参数说明', remote: 'Agent 与远程说明' }[serverFormTab.value]
})
const serverHelpTip = computed(() => {
  if (isCreatingServer.value) {
    if (addStep.value === 1) return '先填写最小连接信息即可继续，高级参数可以稍后在编辑服务端时调整。'
    if (addStep.value === 2) return 'Agent 不是连接 frps 的必要条件；不确定时选择“暂不安装 Agent”。'
    return '保存后可在服务端详情中测试连接、安装 Agent，或前往公网反代配置域名规则。'
  }
  if (serverFormTab.value === 'advanced') return '传输参数需与 frps 服务端配置匹配；修改后保存即可生效。'
  if (serverFormTab.value === 'server') return 'frps 参数修改后需保存并「下发 frps 配置」，再重启 frps 才会生效。'
  if (serverFormTab.value === 'remote') return '首次安装/升级需 SSH；安装成功后建议使用 SSH 隧道或 HTTPS 固定接入，不要长期暴露公网 7700。'
  if (serverForm.options.auth_method === 'oidc') return 'OIDC 需要身份提供商支持客户端凭证模式，且 frps 侧已启用相同的 OIDC 校验。'
  return '请先在 VPS 上部署 frps 并放行对应端口，再保存服务端配置。'
})

const logLevelOptions = ['trace', 'debug', 'info', 'warn', 'error'].map((value) => ({ label: value.toUpperCase(), value }))
const transportOptions = [
  { label: 'TCP', value: 'tcp' }, { label: 'KCP', value: 'kcp' }, { label: 'QUIC', value: 'quic' },
  { label: 'WebSocket', value: 'websocket' }, { label: 'WSS', value: 'wss' },
]

// 详情弹窗「代理」表里的公网入口：与内网穿透页同源（TCP/UDP 显示远程端口，密钥类隧道无公网入口）
function proxyPublicEndpoint(proxy: FrpProxy) {
  if (proxy.type === 'tcp' || proxy.type === 'udp') return `:${proxy.remote_port ?? '-'}`
  if (proxy.type === 'stcp' || proxy.type === 'sudp' || proxy.type === 'xtcp') return '密钥访问'
  return proxy.custom_domains.join(', ') || proxy.options?.subdomain || '未设置入口'
}

const detailProxyColumns = computed<DataTableColumns<FrpProxy>>(() => [
  { title: '名称', key: 'name', minWidth: 130 },
  { title: '类型', key: 'type', width: 80, render: (row) => h('span', { class: 'type-chip' }, row.type.toUpperCase()) },
  { title: '本地目标', key: 'local', minWidth: 150, render: (row) => h('code', `${row.local_ip}:${row.local_port}`) },
  { title: '公网入口', key: 'public', minWidth: 180, render: (row) => h('code', proxyPublicEndpoint(row)) },
  { title: '状态', key: 'enabled', width: 90, render: (row) => h('span', { class: row.enabled ? 'detail-status--on' : 'detail-status--off' }, row.enabled ? '已启用' : '已停用') },
])
const detailSamples = computed(() => detailStatus.value.samples)
const detailStatusBars = computed(() => {
  const samples = detailSamples.value
  if (samples.length <= 30) return samples
  const step = (samples.length - 1) / 29
  return Array.from({ length: 30 }, (_, index) => samples[Math.round(index * step)]).filter(Boolean)
})
const detailAvailability = computed(() => detailStatus.value.availability)
const detailAverageLatency = computed(() => detailStatus.value.average_latency_ms)
const detailPeakLatency = computed(() => detailStatus.value.peak_latency_ms)
const detailStatusText = computed(() => detailStatus.value.available ? '正常' : detailSamples.value.length ? '异常' : '待检测')
const detailStatusKind = computed(() => detailStatus.value.available ? 'success' : detailSamples.value.length ? 'error' : 'warning')
const detailChartOption = computed(() => {
  const samples = detailSamples.value
  const times = samples.map((sample) => sample.checked_at)
  const latency = samples.map((sample) => sample.available ? sample.latency_ms ?? 0 : null)
  const failures = samples.map((sample, index) => sample.available ? null : [index, 0])
  return {
    animation: false,
    grid: { left: 42, right: 18, top: 20, bottom: 32 },
    tooltip: { trigger: 'axis', formatter: (params: Array<{ dataIndex: number }>) => formatChartTooltip(samples, params[0]?.dataIndex ?? 0) },
    xAxis: { type: 'category', boundaryGap: false, data: times, axisLabel: { formatter: (value: string) => formatChartTime(value) } },
    yAxis: { type: 'value', min: 0, name: 'ms', nameTextStyle: { padding: [0, 0, 0, 8] } },
    series: [
      { name: '延迟', type: 'line', smooth: 0.25, connectNulls: false, data: latency, symbol: 'circle', symbolSize: 6, lineStyle: { color: '#18a058', width: 2 }, itemStyle: { color: '#18a058' }, areaStyle: { color: 'rgba(24,160,88,0.12)' } },
      { name: '失败', type: 'scatter', data: failures, symbolSize: 9, itemStyle: { color: '#d03050' }, tooltip: { formatter: () => '连接失败' } },
    ],
  }
})

function formatChartTime(value: string) {
  const date = new Date(value.includes('T') ? value : value.replace(' ', 'T') + 'Z')
  return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function formatChartTooltip(samples: FrpServerDetail['status']['samples'], index: number) {
  const sample = samples[index]
  if (!sample) return ''
  const time = formatChartTime(sample.checked_at)
  if (sample.available) return time + '<br/>延迟：' + (sample.latency_ms ?? 0) + ' ms'
  return time + '<br/>连接失败' + (sample.error ? '：' + sample.error : '')
}

function statusBarTooltip(sample: FrpServerDetail['status']['samples'][number]) {
  const time = formatChartTime(sample.checked_at)
  if (sample.available) return `${time}：可连接，延迟 ${sample.latency_ms ?? 0} ms`
  return `${time}：连接失败${sample.error ? `：${sample.error}` : ''}`
}

function resetServerForm() {
  Object.assign(serverForm, {
    name: '', server_addr: '', server_port: 7000, tls_enabled: true, tls_server_name: '', enabled: true, token: '',
    agent_url: '', agent_token: '', ssh_host: '', ssh_port: 22, ssh_user: 'root', ssh_auth: 'key', ssh_secret: '', mgmt_enabled: false,
    oidc_client_secret: '', dashboard_password: '', clear_dashboard_password: false, tls_certificate: '', tls_key: '', tls_trusted_ca: '', clear_tls_certificate: false,
    clear_tls_key: false, clear_tls_trusted_ca: false,
  })
  // 整体替换 options，避免上一次编辑的字段（如 vhost/日志/端口）残留到新增表单
  serverForm.options = defaultServerOptions()
}

function applyServerPreset(value: ServerPreset) {
  serverPreset.value = value
  serverForm.options.vhost_http_port = 8080
  serverForm.options.vhost_https_port = 8443
  if (value === 'existing-agent') {
    agentSetupMode.value = 'existing'
    serverForm.mgmt_enabled = true
    return
  }
  agentSetupMode.value = 'none'
  serverForm.mgmt_enabled = false
}

function setAgentSetupMode(value: AgentSetupMode) {
  agentSetupMode.value = value
  serverForm.mgmt_enabled = value !== 'none'
}

function addNextStep() {
  if (addStep.value === 1 && !canProceedAddStep1.value) {
    message.error('请先填写服务端名称、地址、端口和认证信息')
    return
  }
  if (addStep.value === 2 && !canProceedAddStep2.value) {
    message.error('请先补充 Agent 接入所需信息')
    return
  }
  addStep.value = Math.min(3, addStep.value + 1) as ServerAddStep
}

function addPrevStep() {
  addStep.value = Math.max(1, addStep.value - 1) as ServerAddStep
}

function openServerModal(server?: FrpServer) {
  editingServer.value = server ?? null
  serverFormTab.value = 'basic'
  addStep.value = 1
  agentSetupMode.value = server?.agent_configured ? 'existing' : server?.ssh_configured ? 'ssh' : 'none'
  serverPreset.value = 'standard'
  confirmedPrivilegedVhost.value = ''
  resetServerForm()
  if (server) {
    Object.assign(serverForm, { ...server, token: '', oidc_client_secret: '', agent_token: '', ssh_secret: '' })
    Object.assign(serverForm.options, server.options)
    agentSetupMode.value = server.agent_configured ? 'existing' : server.ssh_configured ? 'ssh' : 'none'
  }
  showServerModal.value = true
}

function openServerDetail(server: FrpServer) {
  viewingServer.value = server
  detailStatus.value = { available: false, availability: 0, average_latency_ms: 0, peak_latency_ms: 0, samples: [] }
  detailProxies.value = []
  detailLogs.value = []
  showServerDetail.value = true
  void loadDetailData()
}

async function loadDetailData() {
  const server = viewingServer.value
  if (!server) return
  try {
    const [detail, nextLogs] = await Promise.all([api.getFrpServerDetail(server.id), api.getFrpLogs(300)])
    viewingServer.value = detail.server
    detailStatus.value = detail.status
    serverDiagnostics.value = { ...serverDiagnostics.value, [server.id]: {
      server_id: server.id, available: detail.status.available, latency_ms: detail.status.average_latency_ms,
      location: serverDiagnostics.value[server.id]?.location, error: detail.status.last_error,
    } }
    detailProxies.value = asList(detail.proxies)
    const matchingLogs = asList(nextLogs).filter((line) => line.includes(server.server_addr) || line.includes(`[${server.name}]`))
    detailLogs.value = (matchingLogs.length ? matchingLogs : asList(nextLogs)).slice(-100)
  } catch (error) { message.error(error instanceof Error ? error.message : '读取服务端详情失败') }
}

const showServerLogs = ref(false)
const serverLogsTarget = ref<FrpServer | null>(null)
const serverLogLines = ref<string[]>([])
const serverLogsLoading = ref(false)

function openServerLogs(server: FrpServer) {
  serverLogsTarget.value = server
  showServerLogs.value = true
  void loadServerLogs()
}

async function loadServerLogs() {
  const server = serverLogsTarget.value
  if (!server) return
  serverLogsLoading.value = true
  try { serverLogLines.value = asList(await api.getFrpLogs(200, server.id)) }
  catch (error) { message.error(error instanceof Error ? error.message : '读取服务端日志失败') }
  finally { serverLogsLoading.value = false }
}

async function loadServerDiagnostics(items: FrpServer[]) {
  if (items.length === 0 || Date.now() - diagnosticsAt < 30_000) return
  diagnosticsAt = Date.now()
  const results = await Promise.all(items.map(async (server) => {
    try { return await api.getFrpServerDiagnostics(server.id) }
    catch { return { server_id: server.id, available: false, error: '不可达' } }
  }))
  serverDiagnostics.value = Object.fromEntries(results.map((item) => [item.server_id, item]))
}

function latencyText(server: FrpServer) {
  const diagnostics = serverDiagnostics.value[server.id]
  if (!diagnostics) return '未测速'
  if (!diagnostics.available) return diagnostics.error?.includes('本机网络策略') ? '受限' : '不可达'
  return `${diagnostics.latency_ms ?? 0} ms`
}

function latencyDetail(server: FrpServer) {
  return serverDiagnostics.value[server.id]?.error || ''
}

function locationText(server: FrpServer) {
  return serverDiagnostics.value[server.id]?.location || server.server_addr || '定位中'
}

function authMethodText(method: FrpServerOptions['auth_method']) {
  return { none: '无认证', token: 'Token', oidc: 'OIDC（开放身份连接）' }[method] ?? method
}

function serverStartupText(server: FrpServer) {
  const labels = { starting: '启动中', running: '运行中', stopped: '已停止', error: '异常' }
  return labels[server.boot_status] ?? '已停止'
}

function serverStartupType(server: FrpServer) {
  if (server.boot_status === 'running') return 'success'
  if (server.boot_status === 'starting') return 'warning'
  if (server.boot_status === 'error') return 'error'
  return 'default'
}

// StatusBadge 的 kind 只认 success/warning/error/disabled/unknown，n-tag 的 'default' 要映射成 unknown
function serverStartupKind(server: FrpServer): StatusKind {
  if (server.boot_status === 'running') return 'success'
  if (server.boot_status === 'starting') return 'warning'
  if (server.boot_status === 'error') return 'error'
  return 'unknown'
}

function restartAgentTunnel(target: FrpServer) {
  return run('tunnel-restart', '重启 SSH 隧道', () => api.restartFrpAgentTunnel(target.id),
    () => ({ text: 'SSH 隧道已重建并完成连接检查', ok: true }))
    .then(async (result) => { if (result !== undefined) await load() })
}

function enableAgentHTTPS(target: FrpServer) {
  return run('transport-https', '切换 HTTPS 直连', () => api.setFrpAgentTransport(target.id, 'https-pin', 0),
    () => ({ text: 'HTTPS 直连已启用并通过证书指纹验证（已自动选择端口）', ok: true }))
    .then(async (result) => { if (result !== undefined) await load() })
}

function agentHTTPSPort(server: FrpServer): number {
  const addr = server.agent_listen_addr ?? ''
  const index = addr.lastIndexOf(':')
  if (index < 0) return 0
  const port = Number(addr.slice(index + 1))
  return Number.isInteger(port) && port > 0 ? port : 0
}

function rotateAgentCert(target: FrpServer) {
  return run('tls-rotate', '轮换 HTTPS 证书', () => api.rotateFrpAgentTLSCert(target.id),
    () => ({ text: 'HTTPS 证书已轮换，过渡期内同时接受新旧指纹', ok: true }))
    .then(async (result) => { if (result !== undefined) await load() })
}

function checkAgentFirewall(target: FrpServer) {
  const port = agentHTTPSPort(target)
  if (!port) {
    message.warning('请先切换到 HTTPS 直连')
    return
  }
  return run('firewall-check', '检测主机防火墙', () => api.getFrpAgentFirewall(target.id, port),
    (result) => ({ text: `主机防火墙：${result.tool ?? '未知'}，端口${result.port_open ? '已放行' : '未放行'}`, ok: true }))
    .then(async (result) => {
      if (result) firewallInfo.value = { serverId: target.id, data: result }
    })
}

function allowAgentFirewall(target: FrpServer) {
  const port = agentHTTPSPort(target)
  if (!port) {
    message.warning('请先切换到 HTTPS 直连')
    return
  }
  return run('firewall-allow', '放行主机防火墙', () => api.allowFrpAgentFirewallPort(target.id, port),
    () => ({ text: `已在主机防火墙放行 TCP ${port}`, ok: true }))
    .then(async (result) => {
      if (result) firewallInfo.value = { serverId: target.id, data: result.status ?? result }
      await load()
    })
}

function switchAgentToTunnel(target: FrpServer) {
  return run('transport-tunnel', '切回 SSH 隧道', () => api.setFrpAgentTransport(target.id, 'tunnel'),
    () => ({ text: '已关闭 HTTPS 直连并恢复 SSH 隧道', ok: true }))
    .then(async (result) => { if (result !== undefined) await load() })
}

function agentTransportText(server: FrpServer) {
  if (server.agent_transport === 'tunnel') return 'SSH 隧道'
  if (server.agent_transport === 'https-pin') return 'HTTPS + 证书固定'
  return '公网 HTTP（兼容）'
}

function agentTransportKind(server: FrpServer): StatusKind {
  if (server.agent_transport === 'tunnel' || server.agent_transport === 'https-pin') return 'success'
  return 'warning'
}

function agentTransportStateText(server: FrpServer) {
  if (server.agent_transport === 'http') return '不适用'
  if (server.agent_transport_state === 'connected') return '已连接'
  if (server.agent_transport_state === 'reconnecting') return '重连中'
  if (server.agent_transport_state === 'failed') return '连接失败'
  return server.agent_transport === 'https-pin' ? '待验证' : '未建立'
}

function processStateText(server: FrpServer) {
  return ({ starting: '启动中', running: '运行中', stopped: '已停止', error: '异常', unknown: '未知' } as Record<string, string>)[server.process_state] ?? '未知'
}

function connectionStateText(server: FrpServer) {
  return ({ connected: '已连接', connecting: '连接中', disconnected: '未连接', error: '连接异常', unknown: '连接未知' } as Record<string, string>)[server.connection_state] ?? '连接未知'
}

function serverStateTooltip(server: FrpServer) {
  return `期望：${serverStartupText(server)}；frpc：${processStateText(server)}；FRP：${connectionStateText(server)}${server.last_error ? `；${server.last_error}` : ''}`
}

function validateServerForm(): string {
  const form = serverForm
  if (!form.name.trim()) return '请填写服务端名称'
  if (!form.server_addr.trim()) return '请填写主机地址'
  if (!form.server_port || form.server_port < 1 || form.server_port > 65535) return '端口需在 1-65535 之间'

  if (form.options.auth_method === 'token' && !editingServer.value && !form.token.trim()) {
    return '认证方式为令牌时，请填写与 frps 一致的令牌'
  }
  if (form.options.auth_method === 'oidc') {
    if (!form.options.oidc_client_id?.trim()) return 'OIDC 需填写客户端 ID'
    if (!form.options.oidc_token_endpoint_url?.trim()) return 'OIDC 需填写令牌端点'
    if (!editingServer.value && !form.oidc_client_secret.trim()) return 'OIDC 需填写客户端密钥'
  }

  // 80/443 不再硬拦：由 saveServer 弹窗二次确认，后端要求同一次请求带上确认标记
  const ports: Array<[string, number]> = [
    ['控制端口', form.server_port],
    ['HTTP 回源端口', form.options.vhost_http_port || 8080],
    ['HTTPS 回源端口', form.options.vhost_https_port || 8443],
  ]
  if (form.options.dashboard_port) ports.push(['管理端口', form.options.dashboard_port])
  if (form.options.tcpmux_http_connect_port) ports.push(['tcpmux 端口', form.options.tcpmux_http_connect_port])
  const seen = new Map<number, string>()
  for (const [label, port] of ports) {
    const prev = seen.get(port)
    if (prev) return `${prev}与${label}不能使用同一端口（${port}）`
    seen.set(port, label)
  }

  const hasDashboardAddr = !!form.options.dashboard_addr?.trim()
  const hasDashboardPort = !!form.options.dashboard_port
  if (hasDashboardAddr !== hasDashboardPort) return 'frps 管理接口的地址与端口需同时填写或同时留空'

  if (form.ssh_host.trim()) {
    if (!form.ssh_user.trim()) return '填写了 SSH 地址后，需同时填写 SSH 用户'
    if (!editingServer.value?.ssh_configured && !form.ssh_secret.trim()) return '填写了 SSH 地址后，需填写 SSH 密码或私钥'
  }

  if (form.options.tls_force && !form.tls_enabled) return '已勾选「强制 TLS」，请在高级配置中启用 TLS'

  const allowPorts = form.options.allow_ports?.trim()
  if (allowPorts && !/^\d+(-\d+)?(\s*,\s*\d+(-\d+)?)*$/.test(allowPorts)) {
    return '端口白名单格式应为 6000,6005-6010 这样的单端口或范围'
  }

  return ''
}

// 当前表单里用到的 80/443 回源端口；空数组表示无需确认。
function privilegedVhostPorts(): number[] {
  const ports: number[] = []
  if (serverForm.options.vhost_http_port === 80) ports.push(80)
  if (serverForm.options.vhost_https_port === 443) ports.push(443)
  return ports
}

// 80/443 通常已被 VPS 上的 Nginx 占用，放行前必须让用户确认自己已经释放了端口。
function confirmPrivilegedVhostPorts(ports: number[]): Promise<boolean> {
  return new Promise((resolve) => {
    dialog.warning({
      title: `确认使用 ${ports.join(' / ')} 作为 frps 回源端口`,
      content: `${ports.join(' / ')} 通常已被 VPS 上的 Nginx 占用。继续前请先停止占用该端口的服务，否则 frps 会因端口冲突启动失败，隧道和公网反代都会不可用。保存后需在服务端详情「下发 frps 配置」并重启 frps 才生效。`,
      positiveText: '端口已释放，继续保存',
      negativeText: '取消',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
    })
  })
}

async function saveServer(pushAfterSave = false) {
  const invalid = validateServerForm()
  if (invalid) {
    message.error(invalid)
    return
  }
  const privileged = privilegedVhostPorts()
  if (privileged.length > 0 && confirmedPrivilegedVhost.value !== privileged.join(',')) {
    const confirmed = await confirmPrivilegedVhostPorts(privileged)
    if (!confirmed) return
    confirmedPrivilegedVhost.value = privileged.join(',')
  }
  if (serverForm.mgmt_enabled && !serverForm.agent_url.trim()) {
    message.warning('未填写 Agent 地址：保存后可在本页通过 SSH 安装 Agent 自动回填')
  }
  savingServer.value = true
  try {
    const payload: FrpServerPayload = {
      name: serverForm.name, server_addr: serverForm.server_addr, server_port: serverForm.server_port,
      agent_url: serverForm.agent_url, agent_token: serverForm.agent_token, ssh_host: serverForm.ssh_host,
      ssh_port: serverForm.ssh_port, ssh_user: serverForm.ssh_user, ssh_auth: serverForm.ssh_auth,
      ssh_secret: serverForm.ssh_secret, mgmt_enabled: serverForm.mgmt_enabled,
      tls_enabled: serverForm.tls_enabled, tls_server_name: serverForm.tls_server_name, enabled: serverForm.enabled,
      token: serverForm.token, options: { ...serverForm.options }, oidc_client_secret: serverForm.oidc_client_secret,
      dashboard_password: serverForm.dashboard_password, clear_dashboard_password: serverForm.clear_dashboard_password,
      tls_certificate: serverForm.tls_certificate, tls_key: serverForm.tls_key, tls_trusted_ca: serverForm.tls_trusted_ca,
      clear_tls_certificate: serverForm.clear_tls_certificate, clear_tls_key: serverForm.clear_tls_key,
      clear_tls_trusted_ca: serverForm.clear_tls_trusted_ca,
      allow_privileged_vhost_ports: privileged.length > 0,
    }
    const saved = editingServer.value
      ? await api.updateFrpServer(editingServer.value.id, payload)
      : await api.createFrpServer(payload)
    let savedMessage = '服务端配置已保存'
    if (pushAfterSave) {
      if (saved.agent_configured) {
        await api.pushFrpConfigToAgent(saved.id)
        savedMessage = '服务端已保存，frps 配置已下发'
      } else {
        message.warning('服务端已保存；Agent 安装完成后再下发 frps 配置')
      }
    }
    showServerModal.value = false
    message.success(savedMessage)
    await load()
    if (!editingServer.value) {
      dialog.info({
        title: '服务端已保存',
        content: '下一步：安装并测试 Agent，然后前往「公网反代」部署域名规则。',
        positiveText: '知道了',
      })
    }
  } catch (error) { message.error(error instanceof Error ? error.message : '保存失败') }
  finally { savingServer.value = false }
}

async function testAgent(server: FrpServer) {
  agentActionId.value = server.id
  try { await api.testFrpAgent(server.id); message.success('Agent 连接正常') }
  catch (error) { message.error(error instanceof Error ? error.message : 'Agent 连接失败') }
  finally { agentActionId.value = null }
}

async function startServer(server: FrpServer) {
  serverStartingId.value = server.id
  try {
    const updated = await api.startFrpServer(server.id)
    servers.value = servers.value.map((item) => item.id === updated.id ? updated : item)
    viewingServer.value = updated
    message.success('服务端已标记为启动状态')
  } catch (error) { message.error(error instanceof Error ? error.message : '启动服务端失败') }
  finally { serverStartingId.value = null }
}

async function stopServer(server: FrpServer) {
  serverStartingId.value = server.id
  try {
    const updated = await api.stopFrpServer(server.id)
    servers.value = servers.value.map((item) => item.id === updated.id ? updated : item)
    viewingServer.value = updated
    message.success('服务端已停止')
  } catch (error) { message.error(error instanceof Error ? error.message : '停止服务端失败') }
  finally { serverStartingId.value = null }
}

async function measureServer(server: FrpServer) {
  measuringId.value = server.id
  try {
    const diagnostics = await api.getFrpServerDiagnostics(server.id)
    serverDiagnostics.value = { ...serverDiagnostics.value, [server.id]: diagnostics }
    if (!diagnostics.available) {
      message.error(diagnostics.error || '控制端口不可达')
      return
    }
    // TCP 可达不等于可用：登录失败等关键项单独提示
    const login = diagnostics.checks?.find((check) => check.name === 'frpc_login')
    if (login?.state === 'failed') {
      message.warning(`控制端口可达，但 ${login.detail ?? 'frpc 登录失败'}`)
      return
    }
    message.info(`控制端口延迟 ${diagnostics.latency_ms ?? 0} ms`)
  } catch (error) { message.error(error instanceof Error ? error.message : '测速失败') }
  finally { measuringId.value = null }
}

const diagnosticCheckLabels: Record<string, string> = {
  tcp_reachable: '控制端口', frpc_login: 'frpc 登录', proxy_reachable: '发布入口', dns_resolution: 'DNS 解析', location: '定位',
}
function checkLabel(name: string) {
  return diagnosticCheckLabels[name] ?? name
}
function checkStateText(state: string) {
  return { ok: '正常', failed: '异常', unknown: '未知' }[state] ?? state
}
function checkTagType(state: string) {
  return state === 'ok' ? 'success' : state === 'failed' ? 'error' : 'warning'
}

async function testServer(server: FrpServer) {
  testingId.value = server.id
  try { message.success((await api.testFrpServer(server.id)).message) }
  catch (error) { message.error(error instanceof Error ? error.message : '校验失败') }
  finally { testingId.value = null }
}

function confirmDeleteServer(server: FrpServer) {
  dialog.warning({ title: '删除服务端', content: `将删除「${server.name}」及其规则；不会停止本机 frpc。是否继续？`, positiveText: '删除', negativeText: '取消', onPositiveClick: async () => { try { await api.deleteFrpServer(server.id); message.success('服务端已删除'); await load() } catch (error) { message.error(error instanceof Error ? error.message : '删除失败') } } })
}

async function openFRPSConfig(server: FrpServer) {
  frpsConfigTarget.value = server
  showFRPSConfig.value = true
  frpsCompare.value = null
  frpsConfigLoading.value = true
  try { frpsConfigContent.value = (await api.getFrpFRPSConfig(server.id)).content }
  catch (error) {
    showFRPSConfig.value = false
    message.error(error instanceof Error ? error.message : '生成 frps.toml 失败')
  }
  finally { frpsConfigLoading.value = false }
}

function openTLSFilePicker(target: 'certificate' | 'key' | 'ca') {
  const inputs = { certificate: certificateFileInput, key: keyFileInput, ca: caFileInput }
  inputs[target].value?.click()
}

async function loadTLSFile(target: 'certificate' | 'key' | 'ca', event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (file.size > 1 << 20) {
    message.error('证书文件不能超过 1MB')
    return
  }
  const content = (await file.text()).trim()
  if (!content.includes('-----BEGIN')) {
    message.error('请选择 PEM 格式的证书、私钥或 CA 文件')
    return
  }
  if (target === 'certificate') serverForm.tls_certificate = content
  if (target === 'key') serverForm.tls_key = content
  if (target === 'ca') serverForm.tls_trusted_ca = content
  message.success(`已加载 ${file.name}`)
}

onMounted(() => {
  void load()
  void loadReleases()
})

useVisibilityPolling(() => {
  if (!server.value) return
  void refreshAll()
}, 10_000)
</script>

<style scoped>
/* 分区：① 运行状态 → ② 环境与依赖 → ③ frps 控制 → ④ 生命周期 → ⑤ 输出与日志 */
.page-alert { margin-bottom: var(--havline-space-3); }
.page-alert__body { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-4); }
.entrance-row { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-4); margin-bottom: var(--havline-space-4); padding: 10px var(--havline-space-5); border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); background: var(--havline-info-soft); }
.entrance-row__text { font-size: 13px; color: var(--havline-text); }

.stats-row { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--havline-space-4); margin-bottom: var(--havline-space-4); }
.stat-card { background: var(--havline-surface); border: 1px solid var(--havline-border); border-radius: var(--havline-radius); box-shadow: var(--havline-shadow); padding: var(--havline-space-4) var(--havline-space-5); min-height: 118px; }
.stat-card__icon { font-size: 18px; margin-bottom: 4px; }
.stat-card__icon--blue { color: var(--havline-info); }
.stat-card__icon--green { color: var(--havline-brand); }
.stat-card__icon--amber { color: var(--havline-warning); }
.stat-card__icon--red { color: var(--havline-error); }
.stat-card__value { margin-top: 4px; font-size: 30px; font-weight: 700; line-height: 1.1; color: var(--havline-text); letter-spacing: -0.03em; }
.stat-card__value--sm { font-size: 18px; }
.stat-card__label { margin-top: 8px; font-size: 13px; font-weight: 600; color: var(--havline-text); }
.stat-card__sub { margin-top: 4px; font-size: 12px; color: var(--havline-text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.port-list { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
.port-item { display: inline-flex; align-items: center; gap: 4px; font-family: var(--havline-mono); font-size: 12.5px; padding: 1px 6px; border-radius: 999px; background: var(--havline-bg-muted); color: var(--havline-text-muted); }
.port-item--on { background: var(--havline-brand-soft); color: var(--havline-brand-text); }

.dep-grid { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: var(--havline-space-4); margin-bottom: var(--havline-space-4); }
/* 前四张卡每行三张（各占 2/6），VPS 资源与 VPS 证书各占 3/6 并排 */
.dep-grid > .section-card { grid-column: span 2; }
/* 三列下卡片较窄：短字段（版本/徽标）双列并排，长路径与命令用 .field-grid__wide 整行，控制卡片高度 */
.dep-grid .field-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--havline-space-2) var(--havline-space-3); }
/* frps 控制与 Agent 生命周期两列并排，卡片等高 */
.control-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--havline-space-4); margin-bottom: var(--havline-space-4); }
.control-grid .section-card { margin-bottom: 0; }
.section-card { margin-bottom: var(--havline-space-4); }
.section-body { padding: var(--havline-space-2) var(--havline-space-5) var(--havline-space-4); }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--havline-space-3) var(--havline-space-4); }
.field-grid > div { display: grid; gap: 4px; min-width: 0; }
.field-grid__wide { grid-column: 1 / -1; }
.field-grid label { color: var(--havline-text-secondary); font-size: 12px; }
.field-tags { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.agent-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: var(--havline-space-3); }
.agent-transport-grid { margin-top: var(--havline-space-3); }
.agent-transport-warning { margin-top: var(--havline-space-3); }
.agent-transport-error { color: var(--havline-error); }
.tooltip-trigger { display: inline-flex; }
.frps-install-row { display: flex; align-items: center; gap: 10px; margin-top: var(--havline-space-3); flex-wrap: wrap; }
.frps-version-select { width: 200px; }
.nginx-error { margin-top: var(--havline-space-2); color: var(--havline-error); overflow-wrap: anywhere; }

/* 配置查看弹窗（只读） */
.config-modal { background: var(--havline-surface); border: 1px solid var(--havline-border); border-radius: var(--havline-radius); box-shadow: var(--havline-shadow-md); padding: var(--havline-space-4) var(--havline-space-5); }
.config-modal__header { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); }
.config-modal__body { margin-top: var(--havline-space-3); }
.config-modal__code { max-height: 380px; }
.config-modal__footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: var(--havline-space-4); }
.modal-title { margin: 0; font-size: 15px; font-weight: 600; color: var(--havline-text); }

.op-list { display: grid; gap: var(--havline-space-3); }
.op-item { padding: var(--havline-space-3); border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); background: var(--havline-surface); }
.op-item--fail { border-color: var(--havline-error); }
.op-item__head { display: flex; align-items: center; gap: 8px; }
.op-item__source { font-size: 13px; font-weight: 600; color: var(--havline-text); }
.op-item__time { margin-left: auto; font-size: 12px; color: var(--havline-text-muted); }

.code-block {
  margin: var(--havline-space-2) 0 0;
  padding: var(--havline-space-3) var(--havline-space-4);
  background: var(--havline-bg-muted);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  font-family: var(--havline-mono);
  font-size: 12.5px;
  line-height: 1.65;
  color: var(--havline-text);
  overflow: auto;
  max-height: 240px;
  white-space: pre-wrap;
  word-break: break-all;
}
.form-hint { color: var(--havline-text-secondary); font-size: 12.5px; }
.mono { font-family: var(--havline-mono); font-size: 13px; }
.mono-wrap { white-space: normal; word-break: break-all; }

/* 首屏一次性入场；用户偏好减少动效时关闭 */
@keyframes havline-fade-up {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: none; }
}
.stats-row, .dep-grid, .section-card, .entrance-row { animation: havline-fade-up 160ms ease-out both; }
@media (prefers-reduced-motion: reduce) {
  .stats-row, .dep-grid, .section-card, .entrance-row { animation: none; }
}

@media (max-width: 1199px) {
  .stats-row { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .dep-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  /* 窄屏恢复每卡一格：span 在列数变少后会撑破网格 */
  .dep-grid > .section-card, .dep-grid > .vps-resource-card { grid-column: auto; }
}
@media (max-width: 900px) {
  .dep-grid { grid-template-columns: 1fr; }
  .control-grid { grid-template-columns: 1fr; }
  .field-grid { grid-template-columns: 1fr; }
}
@media (max-width: 760px) {
  .stats-row { grid-template-columns: 1fr; }
  .agent-actions { width: 100%; }
  .agent-actions .n-button { min-height: 44px; flex: 1 1 auto; }
  .entrance-row { flex-direction: column; align-items: flex-start; }
}
/* 概览页签：指标卡一行，VPS 资源与证书并排；窄屏再纵向排列 */
.overview-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--havline-space-4);
  margin-bottom: var(--havline-space-4);
}

.res-strip {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  align-items: center;
  gap: var(--havline-space-4);
}

.res-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  grid-template-areas:
    'label value'
    'detail detail'
    'bar bar';
  align-items: center;
  gap: var(--havline-space-2);
  font-size: 13px;
}

.res-row__label {
  grid-area: label;
  color: var(--havline-text-secondary);
}

.res-row__detail {
  grid-area: detail;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--havline-text-muted);
}

.res-row__value {
  grid-area: value;
  font-weight: 600;
  color: var(--havline-text);
  font-variant-numeric: tabular-nums;
}

.res-bar {
  grid-area: bar;
  display: block;
  height: 4px;
  border-radius: 2px;
  background: var(--havline-border);
  overflow: hidden;
}

.res-bar__fill {
  display: block;
  height: 100%;
  border-radius: 2px;
  background: var(--havline-brand);
  transition: width 0.3s ease;
}

.res-bar--warn .res-bar__fill {
  background: var(--havline-warning);
}

.res-bar--danger .res-bar__fill {
  background: var(--havline-error);
}

.res-meta {
  grid-column: 1 / -1;
  display: flex;
  gap: var(--havline-space-4);
  padding-top: var(--havline-space-2);
  border-top: 1px solid var(--havline-border);
  font-size: 12px;
  color: var(--havline-text-muted);
}

@media (max-width: 900px) {
  .res-strip { grid-template-columns: 1fr; }
  .res-meta { flex-direction: column; gap: 4px; }
}
.cert-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.cert-row {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) 76px 76px 92px;
  align-items: center;
  gap: var(--havline-space-2);
  font-size: 13px;
}

.cert-row--head {
  font-size: 12px;
  color: var(--havline-text-muted);
}

.cert-domain {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cert-muted {
  font-size: 12px;
  color: var(--havline-text-muted);
}

.cert-days--warn {
  color: var(--havline-warning);
}

.cert-days--error {
  color: var(--havline-error);
  font-weight: 600;
}

@media (max-width: 900px) {
  .cert-row { grid-template-columns: minmax(0, 1fr) 80px; }
  .cert-row span:nth-child(3),
  .cert-row span:nth-child(4) { display: none; }
}

/* ── 以下样式随「公网服务端」列表与弹窗从内网穿透页迁入（2026-09-24）── */


.section-actions, .switch-row, .modal-actions, .logs-toolbar { display: flex; align-items: center; gap: var(--havline-space-2); flex-wrap: wrap; }

.logs-toolbar { display: flex; align-items: center; justify-content: flex-end; gap: var(--havline-space-3); }

.frp-proxy-modal { display: flex; width: min(1120px, 96vw); max-width: 96vw; max-height: 90vh; background: var(--havline-surface); border-radius: var(--havline-radius); overflow: hidden; box-shadow: var(--havline-shadow-md); }
.frp-proxy-modal--creating { width: min(1180px, 96vw); }
.server-edit-summary { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-4); margin: var(--havline-space-4) var(--havline-space-5) 0; padding: 14px 16px 16px; background: var(--havline-bg); border: 1px solid var(--havline-border); border-radius: 10px; flex-shrink: 0; }
.server-edit-summary__identity { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--havline-space-3); min-width: 0; }
.server-edit-summary__identity strong { color: var(--havline-text); font-size: 14px; }
.server-edit-summary__identity span { color: var(--havline-text-secondary); font-size: 12.5px; }
.server-edit-summary__badges { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
.server-config-notice { margin-bottom: var(--havline-space-3); }
.server-config-collapse, .agent-ssh-collapse, .tls-cert-collapse, .oidc-advanced-collapse { width: 100%; border: 1px solid var(--havline-border); border-radius: 10px; overflow: hidden; background: var(--havline-surface); }
.agent-ssh-collapse { margin-top: var(--havline-space-3); }
.oidc-advanced-collapse { margin-top: var(--havline-space-3); }
.server-config-collapse :deep(.n-collapse-item), .agent-ssh-collapse :deep(.n-collapse-item), .tls-cert-collapse :deep(.n-collapse-item), .oidc-advanced-collapse :deep(.n-collapse-item) { margin: 0 !important; border: none !important; border-radius: 0 !important; }
.server-config-collapse :deep(.n-collapse-item + .n-collapse-item), .agent-ssh-collapse :deep(.n-collapse-item + .n-collapse-item), .tls-cert-collapse :deep(.n-collapse-item + .n-collapse-item), .oidc-advanced-collapse :deep(.n-collapse-item + .n-collapse-item) { border-top: 1px solid var(--havline-border) !important; }
.server-config-collapse :deep(.n-collapse-item__header), .agent-ssh-collapse :deep(.n-collapse-item__header), .tls-cert-collapse :deep(.n-collapse-item__header), .oidc-advanced-collapse :deep(.n-collapse-item__header) { min-height: 44px; padding: 12px 14px !important; box-sizing: border-box; font-weight: 600; background: var(--havline-bg); }
.server-config-collapse :deep(.n-collapse-item:first-child > .n-collapse-item__header), .agent-ssh-collapse :deep(.n-collapse-item:first-child > .n-collapse-item__header), .tls-cert-collapse :deep(.n-collapse-item:first-child > .n-collapse-item__header), .oidc-advanced-collapse :deep(.n-collapse-item:first-child > .n-collapse-item__header) { padding-top: 12px !important; }
.server-config-collapse :deep(.n-collapse-item__content-inner), .agent-ssh-collapse :deep(.n-collapse-item__content-inner), .tls-cert-collapse :deep(.n-collapse-item__content-inner), .oidc-advanced-collapse :deep(.n-collapse-item__content-inner) { padding: 0 14px 14px; }
.server-config-collapse .form-section { padding-top: var(--havline-space-4); }
.agent-mgmt-option { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-4); padding: 12px 0; border-bottom: 1px solid var(--havline-border); }
.agent-mgmt-option__text { display: grid; gap: 2px; min-width: 0; }
.agent-mgmt-option__label { color: var(--havline-text); font-size: 14px; font-weight: 500; }
.agent-mgmt-option__hint { color: var(--havline-text-muted); font-size: 12px; line-height: 1.45; }
.agent-action-panel { margin-top: var(--havline-space-3); padding: 14px 16px; border: 1px solid var(--havline-border); border-radius: 10px; background: var(--havline-bg); }
.agent-action-panel__head { display: flex; align-items: baseline; justify-content: space-between; gap: var(--havline-space-3); flex-wrap: wrap; }
.agent-action-panel__title { color: var(--havline-text); font-size: 14px; font-weight: 600; }
.agent-action-panel__hint { color: var(--havline-text-muted); font-size: 12px; }
.agent-action-panel__buttons { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--havline-border); }
.agent-action-panel__note { margin: 10px 0 0; color: var(--havline-text-muted); font-size: 12px; line-height: 1.5; }
.frp-proxy-modal .proxy-modal__form { flex: 1 1 790px; min-width: 790px; display: flex; flex-direction: column; max-height: 90vh; }
.frp-proxy-modal .proxy-modal__header { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); padding: var(--havline-space-5) var(--havline-space-5) 0; }
.frp-proxy-modal .modal-title { margin: 0; font-size: 18px; font-weight: 600; }
.frp-proxy-modal .proxy-modal__tabbar { display: flex; gap: var(--havline-space-5); padding: var(--havline-space-3) var(--havline-space-5) 0; border-bottom: 1px solid var(--havline-border); flex-shrink: 0; }
.frp-proxy-modal .proxy-modal__tab { margin: 0 0 -1px; padding: 8px 2px 10px; border: 0; border-bottom: 2px solid transparent; background: none; color: var(--havline-text-secondary); font: inherit; font-size: 14px; cursor: pointer; }
.frp-proxy-modal .proxy-modal__tab:hover { color: var(--havline-text); }
.frp-proxy-modal .proxy-modal__tab--active { color: var(--havline-brand-text); font-weight: 600; border-bottom-color: var(--havline-brand); }
.frp-proxy-modal .proxy-modal__scroll { flex: 1; min-height: 0; overflow-y: auto; }
.frp-proxy-modal .proxy-modal__pane { width: 100%; box-sizing: border-box; padding: var(--havline-space-4) var(--havline-space-5) 0; }
.frp-proxy-modal .proxy-modal__pane > .modal-actions { margin-top: var(--havline-space-5); padding-bottom: var(--havline-space-5); }

.form-section { padding: var(--havline-space-4) 0; border-bottom: 1px solid var(--havline-border); }
.form-section:first-child { padding-top: 0; }
.form-section:last-of-type { border-bottom: 0; }
.form-section h3 { display: flex; align-items: center; gap: 8px; margin: 0 0 var(--havline-space-4); font-size: 16px; }
.form-section h3 :deep(.n-icon) { display: grid; place-items: center; width: 26px; height: 26px; border-radius: var(--havline-radius-sm); background: var(--havline-brand); color: #fff; font-size: 15px; }

.narrow-input { max-width: 200px; }
/* 统一卡片式弹窗外壳（运行日志 / 导出配置 / frps 同源配置）：与 .proxy-modal 体系一致的头部 + 滚动区 + 页脚 */
.frp-card-modal { display: flex; flex-direction: column; width: min(860px, calc(100vw - 32px)); max-height: 90vh; background: var(--havline-surface); border-radius: var(--havline-radius); overflow: hidden; box-shadow: var(--havline-shadow-md); }
.frp-card-modal .proxy-modal__header { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); padding: var(--havline-space-5) var(--havline-space-5) 0; flex-shrink: 0; }
.frp-card-modal .modal-title { margin: 0; font-size: 18px; font-weight: 600; color: var(--havline-text); }
.frp-card-modal .proxy-modal__scroll { flex: 1; min-height: 0; overflow-y: auto; padding: var(--havline-space-4) var(--havline-space-5) 0; }
.frp-card-modal .modal-footer { display: flex; justify-content: flex-end; gap: var(--havline-space-3); padding: var(--havline-space-4) var(--havline-space-5) var(--havline-space-5); border-top: 1px solid var(--havline-border); margin-top: var(--havline-space-2); flex-shrink: 0; }

.frp-detail-card :deep(.n-card-header) { padding: var(--havline-space-5) var(--havline-space-5) 0; }
.frp-detail-card :deep(.n-card-header__main) { font-size: 18px; font-weight: 600; color: var(--havline-text); }
.frp-detail-card :deep(.n-card__content) { padding: var(--havline-space-4) var(--havline-space-5) var(--havline-space-5); }

.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 var(--havline-space-4); }
.configured-secret-form-label { display: inline-flex; align-items: center; gap: 8px; min-height: 22px; }
.frp-proxy-modal :deep(.n-form-item-label) { min-height: 22px; align-items: center; }
.form-grid--wide > :first-child { grid-column: 1 / -1; }
.tls-panel { padding: var(--havline-space-4); border-radius: var(--havline-radius-sm); background: var(--havline-bg-muted); }
.tls-cert-collapse { margin-top: var(--havline-space-3); }
.tls-file-field { margin-bottom: var(--havline-space-4); }
.tls-file-field__head { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); margin-bottom: var(--havline-space-2); font-size: 14px; font-weight: 500; }
.file-input { display: none; }
.asset-actions { display: flex; gap: var(--havline-space-2); margin-top: calc(-1 * var(--havline-space-2)); }
.advanced-collapse { margin-top: var(--havline-space-3); }
.switch-row { padding: 4px 0 var(--havline-space-5); }
.switch-row span:nth-of-type(2) { margin-left: var(--havline-space-4); }
.modal-actions { justify-content: flex-end; }
.logs-box { margin: var(--havline-space-3) 0 0; padding: var(--havline-space-4); min-height: 260px; max-height: 480px; overflow: auto; border: 1px solid var(--havline-border); background: var(--havline-bg-muted); color: var(--havline-text); white-space: pre-wrap; word-break: break-word; font: 12px/1.55 var(--havline-mono); }

.table-muted { margin-top: 4px; color: var(--havline-text-muted); font-size: 12px; }
.type-chip { font-family: var(--havline-mono); font-size: 12px; color: var(--havline-brand-text); }

.server-detail { display: grid; gap: var(--havline-space-4); }
.server-detail__top, .server-detail__section-title { display: flex; align-items: center; justify-content: space-between; gap: var(--havline-space-3); }
.server-detail__top h2 { margin: 0; font-size: 24px; }
.server-detail__top p { margin: 4px 0 0; color: var(--havline-text-secondary); font-size: 13px; }
.server-detail__grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--havline-space-4); }
.server-detail__panel { padding: var(--havline-space-4); border: 1px solid var(--havline-border); border-radius: var(--havline-radius-sm); background: var(--havline-bg); }
.server-detail__panel h3 { margin: 0 0 var(--havline-space-4); font-size: 16px; }
.detail-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--havline-space-4); }
.detail-fields--single { grid-template-columns: 1fr; }
.detail-fields > div { display: grid; gap: 5px; min-width: 0; }
.detail-fields label, .detail-stats label { color: var(--havline-text-secondary); font-size: 12px; }
.detail-fields strong, .detail-fields span { overflow-wrap: anywhere; }
.detail-subgroup { margin-top: var(--havline-space-3); padding-top: var(--havline-space-3); border-top: 1px solid var(--havline-border); }
.detail-subgroup__title { margin-bottom: var(--havline-space-2); font-size: 12px; font-weight: 600; color: var(--havline-text-secondary); }
.detail-muted { color: var(--havline-text-secondary); font-size: 12px; }
.diag-checks { display: inline-flex; flex-wrap: wrap; gap: 6px; }
.server-detail__location { display: flex; align-items: center; gap: 5px; }
.detail-stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--havline-space-4); margin-bottom: var(--havline-space-4); }
.detail-stats > div { display: grid; gap: 5px; }
.detail-stats strong { font-size: 20px; color: var(--havline-success); }
.server-status-summary { display: grid; grid-template-columns: minmax(150px, 1.3fr) repeat(3, minmax(110px, 1fr)); align-items: center; gap: var(--havline-space-4); }
.server-status-summary__item { display: grid; gap: 6px; min-width: 0; }
.server-status-summary__item label { color: var(--havline-text-secondary); font-size: 12px; }
.server-status-summary__item strong { color: var(--havline-success); font-size: 18px; line-height: 1.1; font-variant-numeric: tabular-nums; }
.server-status-summary__item--state { display: flex; align-items: center; min-height: 28px; padding-right: var(--havline-space-4); border-right: 1px solid var(--havline-border); }
.status-bars { display: grid; grid-template-columns: repeat(30, minmax(0, 1fr)); gap: 4px; margin-top: var(--havline-space-5); min-height: 36px; }
.status-bar { position: relative; min-width: 0; height: 36px; border-radius: 4px; background: var(--havline-bg-muted); cursor: help; outline: none; transition: transform 160ms ease, filter 160ms ease, box-shadow 160ms ease; }
.status-bar--ok { background: linear-gradient(180deg, color-mix(in srgb, var(--havline-success) 75%, white), color-mix(in srgb, var(--havline-success) 48%, white)); }
.status-bar--failed { background: color-mix(in srgb, var(--havline-error) 68%, white); }
.status-bar:hover, .status-bar:focus-visible { z-index: 2; transform: translateY(-2px); filter: brightness(1.1); box-shadow: var(--havline-shadow-md); }
.status-bar::after { content: attr(data-tooltip); position: absolute; left: 50%; bottom: calc(100% + 9px); z-index: 3; max-width: 240px; padding: 6px 8px; border-radius: 6px; background: var(--havline-text); color: var(--havline-surface); font-size: 12px; line-height: 1.2; white-space: nowrap; pointer-events: none; opacity: 0; transform: translate(-50%, 4px); transition: opacity 160ms ease, transform 160ms ease; }
.status-bar::before { content: ''; position: absolute; left: 50%; bottom: calc(100% + 4px); z-index: 3; border: 5px solid transparent; border-top-color: var(--havline-text); pointer-events: none; opacity: 0; transform: translate(-50%, 4px); transition: opacity 160ms ease, transform 160ms ease; }
.status-bar:hover::after, .status-bar:hover::before, .status-bar:focus-visible::after, .status-bar:focus-visible::before { opacity: 1; transform: translate(-50%, 0); }
.status-bars-empty { display: grid; place-items: center; min-height: 36px; margin-top: var(--havline-space-5); border: 1px dashed var(--havline-border); color: var(--havline-text-muted); font-size: 12px; }
.status-bars-footer { display: flex; justify-content: space-between; margin-top: var(--havline-space-3); color: var(--havline-text-muted); font-size: 12px; }
.status-bars-footer span:last-child { display: inline-flex; align-items: center; gap: 5px; }
.status-bars-footer i { width: 6px; height: 6px; border-radius: 50%; background: color-mix(in srgb, var(--havline-success) 35%, white); }
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

.frps-compare--warn {
  color: var(--havline-warning);
}
/* 服务端表单沿用内网穿透页的提示排版；本页既有的 .form-hint 服务于详情卡，保持不动 */
.frp-proxy-modal .form-hint { margin: 0 0 var(--havline-space-4); font-size: 13px; line-height: 1.55; }

@media (max-width: 1200px) {
  .frp-proxy-modal .proxy-modal__form { flex-basis: auto; min-width: 0; }
}

/* 新增服务端向导：只保留连接 frps 的最小字段，低频参数留到确认步骤折叠 */
.server-wizard {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.server-wizard__steps {
  padding: var(--havline-space-4) var(--havline-space-5) 0;
  flex-shrink: 0;
}

.server-wizard__body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--havline-space-4) var(--havline-space-5);
}

.server-wizard__form {
  max-width: 720px;
}

.server-wizard__agent-choice {
  display: flex;
  flex-wrap: wrap;
  gap: var(--havline-space-2);
  margin: var(--havline-space-3) 0;
}

.server-wizard__grid {
  margin-top: var(--havline-space-3);
}

.server-wizard__switch-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
}

.server-wizard__switch-inline span {
  color: var(--havline-text-secondary);
  font-size: 12.5px;
}

.server-wizard__summary {
  display: grid;
  gap: var(--havline-space-3);
  padding: var(--havline-space-4);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg-muted);
}

.server-wizard__summary > div {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.server-wizard__summary span {
  color: var(--havline-text-secondary);
  font-size: 12px;
}

.server-wizard__summary strong {
  color: var(--havline-text);
  overflow-wrap: anywhere;
}

.server-wizard__advanced {
  margin-top: var(--havline-space-4);
}

.server-wizard__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--havline-space-3);
  padding: var(--havline-space-4) var(--havline-space-5) var(--havline-space-5);
  border-top: 1px solid var(--havline-border);
  flex-shrink: 0;
}

.server-wizard__footer-right {
  display: flex;
  align-items: center;
  gap: var(--havline-space-2);
  margin-left: auto;
}

@media (max-width: 760px) {
  .frp-proxy-modal { display: block; max-height: 92vh; overflow-y: auto; }
  .frp-proxy-modal .proxy-modal__form { max-height: none; }
  .frp-proxy-modal .proxy-modal__scroll { overflow: visible; }
  .frp-proxy-modal .proxy-modal__help { width: auto; border-top: 1px solid var(--havline-border); border-left: 0; }
  .server-wizard__body { overflow: visible; }
  .server-wizard__footer { align-items: stretch; flex-direction: column; }
  .server-wizard__footer-right { justify-content: flex-end; margin-left: 0; }
}
@media (max-width: 700px) {
  .server-detail__grid, .detail-fields, .form-grid { grid-template-columns: 1fr; }
  .server-status-summary { grid-template-columns: 1fr; }
  .server-status-summary__item--state { min-height: 0; padding-right: 0; padding-bottom: var(--havline-space-3); border-right: 0; border-bottom: 1px solid var(--havline-border); }
  .status-bars { gap: 3px; }
  .status-bar { height: 28px; }
  .switch-row span:nth-of-type(2) { margin-left: 0; }
}
/* 服务端表单弹窗的说明栏 / 高级折叠 / 服务端选项区：与内网穿透页同源，随弹窗一并迁入 */
.logs-toolbar__filter { width: 200px; }
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
/* 左列表 + 右详情：与 DDNS 页（.ddns-layout / .task-sidebar）同构 */
.agent-layout { display: grid; grid-template-columns: minmax(260px, 320px) minmax(0, 1fr); gap: var(--havline-space-4); align-items: start; }
.agent-detail { min-width: 0; }

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
.server-item__meta span { display: inline-flex; align-items: center; gap: 4px; min-width: 0; }
.server-item__meta :deep(.n-icon) { flex: 0 0 auto; font-size: 14px; }
.server-item__tags { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }

.server-toolbar { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--havline-space-3); margin-bottom: var(--havline-space-4); padding: var(--havline-space-3) var(--havline-space-4); background: var(--havline-surface); border: 1px solid var(--havline-border); border-radius: var(--havline-radius); box-shadow: var(--havline-shadow); }
.server-toolbar__info { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--havline-space-3); min-width: 0; }
.server-toolbar__name { font-size: 15px; font-weight: 600; color: var(--havline-text); }
.server-toolbar__endpoint { font-size: 12.5px; color: var(--havline-text-secondary); }
.server-toolbar__actions { display: flex; align-items: center; flex-wrap: wrap; gap: var(--havline-space-2); }

/* Agent 状态总览：结论、链路、指标、端口按阅读顺序排列 */
.agent-hero-card {
  margin-bottom: var(--havline-space-4);
}

.agent-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--havline-space-4);
  padding: 14px;
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
}

.agent-hero--on {
  border-color: color-mix(in srgb, var(--havline-success) 35%, var(--havline-border));
  background: color-mix(in srgb, var(--havline-success) 6%, var(--havline-bg));
}

.agent-hero--warn {
  border-color: color-mix(in srgb, var(--havline-warning) 35%, var(--havline-border));
  background: color-mix(in srgb, var(--havline-warning) 6%, var(--havline-bg));
}

.agent-hero--off {
  border-color: var(--havline-border);
  background: color-mix(in srgb, var(--havline-text-muted) 6%, var(--havline-bg));
}

.agent-hero__main {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  min-width: 0;
}

.agent-hero__icon {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  flex-shrink: 0;
  border-radius: 10px;
  background: var(--havline-surface);
  color: var(--havline-text-secondary);
  font-size: 22px;
}

.agent-hero--on .agent-hero__icon {
  color: var(--havline-success);
}

.agent-hero--warn .agent-hero__icon {
  color: var(--havline-warning);
}

.agent-hero__text {
  min-width: 0;
}

.agent-hero__title {
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
  color: var(--havline-text);
}

.agent-hero__sub {
  margin-top: 4px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--havline-text-muted);
  overflow-wrap: anywhere;
}

.agent-hero__aside {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
  flex-shrink: 0;
}

.agent-hero__time {
  font-size: 12px;
  color: var(--havline-text-muted);
}

.agent-hero__flow {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: var(--havline-space-4);
  padding: 12px;
  border: 1px dashed color-mix(in srgb, var(--havline-info) 28%, var(--havline-border));
  border-radius: var(--havline-radius-sm);
  background: var(--havline-info-soft);
}

.agent-hero__flow-step {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--havline-text-secondary);
}

.agent-hero__flow-step--accent {
  color: var(--havline-brand-text);
  font-weight: 600;
}

.agent-hero__flow-arrow {
  color: var(--havline-text-muted);
}

.agent-hero__metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--havline-space-3);
  margin-top: var(--havline-space-4);
  padding-top: var(--havline-space-4);
  border-top: 1px solid var(--havline-border);
}

.agent-hero__metrics > div {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.agent-hero__metrics span {
  font-size: 12px;
  color: var(--havline-text-secondary);
}

.agent-hero__metrics strong {
  font-size: 15px;
  color: var(--havline-text);
  overflow-wrap: anywhere;
}

.agent-hero__ports {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: var(--havline-space-3);
}

.agent-hero__ports-label {
  margin-right: 4px;
  font-size: 12px;
  color: var(--havline-text-secondary);
}

.agent-tabs :deep(.n-tabs-nav) {
  margin-bottom: var(--havline-space-4);
}

.agent-tabs :deep(.n-tabs-tab) {
  font-weight: 500;
}

@media (max-width: 1100px) {
  .agent-layout { grid-template-columns: 1fr; }
  .server-sidebar__list { max-height: none; }
}

@media (max-width: 1199px) {
  .overview-grid { grid-template-columns: 1fr; }
  .agent-hero__metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

@media (max-width: 760px) {
  .agent-hero {
    flex-direction: column;
  }

  .agent-hero__aside {
    align-items: flex-start;
  }

  .agent-hero__flow {
    align-items: flex-start;
    flex-direction: column;
  }

  .agent-hero__flow-arrow {
    transform: rotate(90deg);
  }

  .agent-hero__metrics {
    grid-template-columns: 1fr;
  }
}
</style>
