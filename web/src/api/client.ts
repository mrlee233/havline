import type {
  AccessLogEntry,
  ApiError,
  AppVersion,
  AuthStatus,
  CertificateCAOption,
  CertificateRecord,
  DashboardStatus,
  DDNSConfig,
  DDNSSavePayload,
  DDNSTestPayload,
  FRPResponse,
  FRPSavePayload,
  FRPSaveResponse,
  FRPStatus,
  FrpBinary,
  FrpDownloadTask,
  FrpProxy,
  FrpProxyPayload,
  FrpProxiesTraffic,
  FrpRelease,
  FrpRuntimeInfo,
  FrpServer,
  FrpServerDiagnostics,
  FrpServerDetail,
  FrpServerPayload,
  FrpStatus,
  DiscoveredService,
  ProxyClientConn,
  ChinaCIDRStatus,
  ProxyRule,
  ProxySavePayload,
  ProxyTraffic,
  GlobalNginxView,
  RuleNginxView,
  SettingsMap,
  SystemLogEntry,
  NotifyTestPayload,
  UpdateStatus,
  CfMirror,
  CfRuntimeStatus,
  CfTunnel,
  CfTunnelPayload,
  CfAppSettings,
  CfAppSettingsInput,
  CfDriftResult,
  CfDiagnosticsResult,
  CfAccessStatus,
  CfZone,
  CfTunnelRoutes,
  CfPreflightResult,
  CfRouteTestResult,
} from './types'

export function asList<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : []
}

const REQUEST_TIMEOUT_MS = 30_000
const CERT_REQUEST_TIMEOUT_MS = 10 * 60_000

async function request<T>(path: string, init?: RequestInit, timeoutMs = REQUEST_TIMEOUT_MS): Promise<T> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), timeoutMs)
  try {
    return await requestWithSignal<T>(path, init, controller.signal)
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new Error(
        timeoutMs > REQUEST_TIMEOUT_MS
          ? '证书操作超时，请稍后在证书列表查看状态，或在「日志 → 实时日志」查看 Nginx 输出'
          : '请求超时，请检查 Havline 服务或 Nginx 状态',
      )
    }
    throw error
  } finally {
    clearTimeout(timeout)
  }
}

async function requestWithSignal<T>(path: string, init?: RequestInit, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    signal,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })

  if (!response.ok) {
    let message = '请求失败'
    try {
      const body = (await response.json()) as ApiError
      if (body.error) {
        message = body.error
      }
    } catch {
      // ignore
    }
    throw new Error(message)
  }

  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

export const api = {
  getVersion: () => request<AppVersion>('/api/version'),
  getUpdateStatus: () => request<UpdateStatus>('/api/system/update/status'),
  applyUpdate: () => request<UpdateStatus>('/api/system/update/apply', { method: 'POST' }),
  authStatus: () => request<AuthStatus>('/api/auth/status'),
  login: (username: string, password: string) =>
    request<{ message: string }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<{ message: string }>('/api/auth/logout', { method: 'POST' }),
  changePassword: (old_password: string, new_password: string) =>
    request<{ message: string }>('/api/auth/password', {
      method: 'POST',
      body: JSON.stringify({ old_password, new_password }),
    }),

  getStatus: () => request<DashboardStatus>('/api/status'),
  listProxies: () => request<ProxyRule[]>('/api/proxies'),
  createProxy: (payload: ProxySavePayload) =>
    request<ProxyRule>('/api/proxies', { method: 'POST', body: JSON.stringify(payload) }),
  updateProxy: (id: number, payload: Partial<ProxySavePayload>) =>
    request<ProxyRule>(`/api/proxies/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProxy: (id: number) => request<void>(`/api/proxies/${id}`, { method: 'DELETE' }),
  reorderProxies: (ids: number[]) =>
    request<void>('/api/proxies/reorder', { method: 'PUT', body: JSON.stringify({ ids }) }),
  getProxyTraffic: () => request<ProxyTraffic[]>('/api/proxies/traffic'),
  getProxyClients: (id: number) => request<ProxyClientConn[]>(`/api/proxies/${id}/clients`),
  getProxyNginx: (id: number) => request<RuleNginxView>(`/api/proxies/${id}/nginx`),
  saveProxyNginx: (id: number, payload: { mode: 'auto' | 'custom'; content?: string }) =>
    request<RuleNginxView>(`/api/proxies/${id}/nginx`, { method: 'PUT', body: JSON.stringify(payload) }),
  rollbackProxyNginx: (id: number, backup?: string) =>
    request<RuleNginxView>(`/api/proxies/${id}/nginx/rollback`, {
      method: 'POST',
      body: JSON.stringify(backup ? { backup } : {}),
    }),

  listDDNS: () => request<DDNSConfig[]>('/api/ddns'),
  listDDNSLite: () => request<DDNSConfig[]>('/api/ddns?lite=1'),
  createDDNS: (payload: DDNSSavePayload) =>
    request<DDNSConfig>('/api/ddns', { method: 'POST', body: JSON.stringify(payload) }),
  updateDDNS: (id: number, payload: DDNSSavePayload) =>
    request<DDNSConfig>(`/api/ddns/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteDDNS: (id: number) => request<void>(`/api/ddns/${id}`, { method: 'DELETE' }),
  testDDNS: (payload: DDNSTestPayload) =>
    request<{ message: string }>('/api/ddns/test', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  updateAllDDNS: () => request<DDNSConfig[]>('/api/ddns/update', { method: 'POST' }),
  updateDDNSOne: (id: number) => request<DDNSConfig>(`/api/ddns/${id}/update`, { method: 'POST' }),

  listCertificates: () => request<CertificateRecord[]>('/api/certificates'),
  listCertificateCAOptions: () => request<CertificateCAOption[]>('/api/certificates/ca-options'),
  applyCertificate: (payload: {
    dns_zone?: string
    ddns_config_id?: number
    domains: string[]
    ca?: string
    email?: string
  }) =>
    request<{ job_id: string }>('/api/certificates/apply', {
      method: 'POST',
      body: JSON.stringify({
        dns_zone: payload.dns_zone ?? '',
        ddns_config_id: payload.ddns_config_id ?? 0,
        domains: payload.domains,
        ca: payload.ca ?? '',
        email: payload.email ?? '',
      }),
    }),
  certificateApplyStreamURL: (jobId: string) => `/api/certificates/jobs/${encodeURIComponent(jobId)}/stream`,
  importCertificate: (payload: {
    certificate?: string
    private_key?: string
    cert_path?: string
    key_path?: string
  }) =>
    request<CertificateRecord>('/api/certificates/import', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  async downloadCertificate(domain: string, part: 'zip' | 'cert' | 'key' = 'zip') {
    const response = await fetch(
      `/api/certificates/${encodeURIComponent(domain)}/download?part=${part}`,
      { credentials: 'include' },
    )
    if (!response.ok) {
      let message = '下载证书失败'
      try {
        const body = (await response.json()) as ApiError
        if (body.error) message = body.error
      } catch {
        // ignore
      }
      throw new Error(message)
    }
    const blob = await response.blob()
    const disposition = response.headers.get('Content-Disposition') ?? ''
    const match = disposition.match(/filename="?([^";\n]+)"?/)
    const filename = match?.[1] ?? `${domain}.zip`
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    link.click()
    URL.revokeObjectURL(url)
  },
  deleteCertificate: (domain: string) =>
    request<{ message: string }>(`/api/certificates/${encodeURIComponent(domain)}`, { method: 'DELETE' }),
  renewCertificate: (domain?: string, ca?: string) =>
    request<CertificateRecord>(
      '/api/certificates/renew',
      {
        method: 'POST',
        body: JSON.stringify({ domain: domain ?? '', ca: ca ?? '' }),
      },
      CERT_REQUEST_TIMEOUT_MS,
    ),

  getSettings: () => request<SettingsMap>('/api/settings'),
  saveSettings: (payload: SettingsMap) =>
    request<SettingsMap>('/api/settings', { method: 'PUT', body: JSON.stringify(payload) }),
  getGlobalNginx: () => request<GlobalNginxView>('/api/settings/nginx/global'),
  saveGlobalNginx: (payload: { mode: 'auto' | 'custom'; content?: string }) =>
    request<GlobalNginxView>('/api/settings/nginx/global', { method: 'PUT', body: JSON.stringify(payload) }),
  rollbackGlobalNginx: (backup?: string) =>
    request<GlobalNginxView>('/api/settings/nginx/global/rollback', {
      method: 'POST',
      body: JSON.stringify(backup ? { backup } : {}),
    }),
  getNginxConfigVersions: () =>
    request<import('./types').NginxConfigVersionsView>('/api/settings/nginx/config/versions'),
  getNginxConfigVersion: (name: string) =>
    request<import('./types').NginxConfigVersionView>(`/api/settings/nginx/config/versions/${encodeURIComponent(name)}`),
  rollbackNginxConfigVersion: (name: string) =>
    request<import('./types').NginxConfigVersionsView>(
      `/api/settings/nginx/config/versions/${encodeURIComponent(name)}/rollback`,
      { method: 'POST' },
    ),
  getChinaCIDRStatus: () => request<ChinaCIDRStatus>('/api/settings/china-cidr'),
  listApiTokens: () => request<{ tokens: import('./types').ApiToken[] }>('/api/settings/tokens'),
  createApiToken: (payload: { name: string; scope: 'read' | 'write'; expires_in_days: number }) =>
    request<import('./types').CreatedApiToken>('/api/settings/tokens', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  deleteApiToken: (id: number) =>
    request<{ ok: boolean }>(`/api/settings/tokens/${id}`, { method: 'DELETE' }),
  refreshChinaCIDR: () =>
    request<ChinaCIDRStatus>('/api/settings/china-cidr/refresh', { method: 'POST' }),
  testNotify: (payload: NotifyTestPayload) =>
    request<{ message: string }>('/api/settings/notify/test', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  getFRP: () => request<FRPResponse>('/api/frp'),
  saveFRP: (payload: FRPSavePayload) =>
    request<FRPSaveResponse>('/api/frp', {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
  getFRPStatus: () => request<FRPStatus>('/api/frp/status'),
  syncFRPDomains: () =>
    request<FRPSaveResponse & { domains: string[] }>('/api/frp/sync-domains', {
      method: 'POST',
    }),
  getFRPLogs: (limit = 200) => request<string[]>(`/api/frp/logs?limit=${limit}`),

  // ===== FRP 多服务端模块（/api/frpmulti）=====
  getFrpStatus: () => request<FrpStatus>('/api/frpmulti/status'),
  getFrpRuntime: () => request<FrpRuntimeInfo>('/api/frpmulti/runtime'),
  setFrpDownloadProxy: (proxy: string) => request<FrpRuntimeInfo>('/api/frpmulti/runtime/download-proxy', { method: 'PUT', body: JSON.stringify({ proxy }) }),
  listFrpReleases: () => request<FrpRelease[]>('/api/frpmulti/releases'),
  listFrpBinaries: () => request<FrpBinary[]>('/api/frpmulti/binaries'),
  getFrpDownloadStatus: () => request<FrpDownloadTask>('/api/frpmulti/download'),
  downloadFrpRelease: (version: string) => request<FrpDownloadTask>(`/api/frpmulti/releases/${encodeURIComponent(version)}/download`, { method: 'POST' }),
  activateFrpBinary: (version: string) => request<FrpStatus>(`/api/frpmulti/binaries/${encodeURIComponent(version)}/activate`, { method: 'POST' }),
  deleteFrpBinary: (version: string) => request<void>(`/api/frpmulti/binaries/${encodeURIComponent(version)}`, { method: 'DELETE' }),
  listFrpServers: () => request<FrpServer[]>('/api/frpmulti/servers'),
  getFrpServerDiagnostics: (id: number) => request<FrpServerDiagnostics>(`/api/frpmulti/servers/${id}/diagnostics`),
  getFrpServerDetail: (id: number) => request<FrpServerDetail>(`/api/frpmulti/servers/${id}/detail`),
  startFrpServer: (id: number) => request<FrpServer>(`/api/frpmulti/servers/${id}/start`, { method: 'POST' }),
  stopFrpServer: (id: number) => request<FrpServer>(`/api/frpmulti/servers/${id}/stop`, { method: 'POST' }),
  createFrpServer: (payload: FrpServerPayload) =>
    request<FrpServer>('/api/frpmulti/servers', { method: 'POST', body: JSON.stringify(payload) }),
  updateFrpServer: (id: number, payload: FrpServerPayload) =>
    request<FrpServer>(`/api/frpmulti/servers/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteFrpServer: (id: number) => request<void>(`/api/frpmulti/servers/${id}`, { method: 'DELETE' }),
  testFrpServer: (id: number) => request<{ message: string }>(`/api/frpmulti/servers/${id}/test`, { method: 'POST' }),
  listFrpProxies: () => request<FrpProxy[]>('/api/frpmulti/proxies'),
  getFrpProxyLogs: (id: number, limit = 200) => request<string[]>(`/api/frpmulti/proxies/${id}/logs?limit=${limit}`),
  listFrpProxiesTraffic: () => request<FrpProxiesTraffic>('/api/frpmulti/proxies/traffic'),
  createFrpProxy: (payload: FrpProxyPayload) =>
    request<FrpProxy>('/api/frpmulti/proxies', { method: 'POST', body: JSON.stringify(payload) }),
  updateFrpProxy: (id: number, payload: FrpProxyPayload) =>
    request<FrpProxy>(`/api/frpmulti/proxies/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteFrpProxy: (id: number) => request<void>(`/api/frpmulti/proxies/${id}`, { method: 'DELETE' }),
  startFrp: () => request<FrpStatus>('/api/frpmulti/start', { method: 'POST' }),
  stopFrp: () => request<FrpStatus>('/api/frpmulti/stop', { method: 'POST' }),
  reloadFrp: () => request<FrpStatus>('/api/frpmulti/reload', { method: 'POST' }),
  getFrpLogs: (limit = 200, serverID?: number) => {
    const query = new URLSearchParams({ limit: String(limit) })
    if (serverID) query.set('server_id', String(serverID))
    return request<string[]>(`/api/frpmulti/logs?${query.toString()}`)
  },
  exportFrpConfig: () => request<{ content: string }>('/api/frpmulti/config/export'),
  getFrpFRPSConfig: (id: number) => request<{ content: string }>(`/api/frpmulti/servers/${id}/frps-config`),
  testFrpAgent: (id: number) => request<{ ok: boolean }>(`/api/frpmulti/servers/${id}/agent/test`, { method: 'POST' }),
  restartFrpAgentTunnel: (id: number) =>
    request<FrpServer>(`/api/frpmulti/servers/${id}/agent/tunnel/restart`, { method: 'POST' }),
  setFrpAgentTransport: (id: number, transport: 'tunnel' | 'https-pin', port = 0) =>
    request<FrpServer>(`/api/frpmulti/servers/${id}/agent/transport`, {
      method: 'POST',
      body: JSON.stringify({ transport, port }),
    }),
  getFrpAgentTransport: (id: number) =>
    request<Record<string, any>>(`/api/frpmulti/servers/${id}/agent/transport`),
  rotateFrpAgentTLSCert: (id: number) =>
    request<FrpServer>(`/api/frpmulti/servers/${id}/agent/transport/rotate`, { method: 'POST' }),
  getFrpAgentFirewall: (id: number, port: number) =>
    request<Record<string, any>>(`/api/frpmulti/servers/${id}/agent/firewall?port=${port}`),
  allowFrpAgentFirewallPort: (id: number, port: number) =>
    request<Record<string, any>>(`/api/frpmulti/servers/${id}/agent/firewall/allow`, {
      method: 'POST',
      body: JSON.stringify({ port }),
    }),
  probeFrpAgent: (id: number) => request<Record<string, any>>(`/api/frpmulti/servers/${id}/agent/probe`, { method: 'POST' }),
  installFrpAgent: (id: number) => request<FrpServer>(`/api/frpmulti/servers/${id}/agent/install`, { method: 'POST' }),
  uninstallFrpAgent: (id: number, keepData: boolean) => request<{ ok: boolean; output: string }>(`/api/frpmulti/servers/${id}/agent/uninstall?keep_data=${keepData}`, { method: 'POST' }),
  sshDiagnoseFrpAgent: (id: number) => request<{ output: string }>(`/api/frpmulti/servers/${id}/agent/ssh-diagnose`, { method: 'POST' }),
  getFrpAgentStatus: (id: number) => request<import('./types').FrpAgentStatus>(`/api/frpmulti/servers/${id}/agent/status`),
    getFrpAgentCerts: (id: number) =>
    request<import('./types').FrpAgentCerts>(`/api/frpmulti/servers/${id}/agent/certs`),
  reloadFrpAgentNginx: (id: number) =>
    request<import('./types').FrpAgentNginxReload>(`/api/frpmulti/servers/${id}/agent/nginx/reload`, {
      method: 'POST',
    }),
  getFrpAgentMetrics: (id: number) =>
    request<import('./types').FrpAgentMetrics>(`/api/frpmulti/servers/${id}/agent/metrics`),
  getFrpAgentNginxLogs: (id: number, type: 'access' | 'error', lines = 200) =>
    request<import('./types').FrpAgentNginxLogs>(
      `/api/frpmulti/servers/${id}/agent/logs/nginx?type=${type}&lines=${lines}`,
    ),
  getFrpAgentFRPSConfig: (id: number) =>
    request<import('./types').FrpAgentFRPSConfig>(`/api/frpmulti/servers/${id}/agent/frps-config`),
  getFrpAgentRouteDrift: (id: number) =>
    request<import('./types').FrpRouteDriftReport>(`/api/frpmulti/servers/${id}/agent/routes/drift`),
  redeployFrpAgentRoutes: (id: number) =>
    request<import('./types').FrpRedeployAllResult>(`/api/frpmulti/servers/${id}/agent/routes/redeploy`, {
      method: 'POST',
    }),
  getFrpAgentRoutes: (id: number) => request<import('./types').FrpAgentRouteDetail[]>(`/api/frpmulti/servers/${id}/agent/routes`),
  getFrpAgentRouteCandidates: (id: number) => request<import('./types').FrpProxy[]>(`/api/frpmulti/servers/${id}/agent/route-candidates`),
  deployFrpAgentRoute: (serverId: number, proxyId: number) => request<{ ok: boolean; basic_auth_user?: string; basic_auth_password?: string }>(`/api/frpmulti/servers/${serverId}/agent/routes/${proxyId}`, { method: 'POST' }),
  getFrpAgentRouteConf: (serverId: number, proxyId: number) =>
    request<{ proxy_id: number; domain: string; content: string }>(`/api/frpmulti/servers/${serverId}/agent/routes/${proxyId}/conf`),
  getFrpAgentRouteVersions: (serverId: number, proxyId: number) =>
    request<import('./types').FrpAgentRouteVersions>(`/api/frpmulti/servers/${serverId}/agent/routes/${proxyId}/versions`),
  getFrpAgentRouteVersion: (serverId: number, proxyId: number, name: string) =>
    request<import('./types').FrpAgentRouteVersion>(
      `/api/frpmulti/servers/${serverId}/agent/routes/${proxyId}/versions/${encodeURIComponent(name)}`,
    ),
  rollbackFrpAgentRouteVersion: (serverId: number, proxyId: number, name: string) =>
    request<{ ok: boolean; domain: string; name: string }>(
      `/api/frpmulti/servers/${serverId}/agent/routes/${proxyId}/versions/${encodeURIComponent(name)}/rollback`,
      { method: 'POST' },
    ),
  installFrpAgentNginx: (serverId: number) =>
    request<{ ok: boolean; manager?: string; output?: string; path?: string; error?: string }>(`/api/frpmulti/servers/${serverId}/agent/nginx/install`, { method: 'POST' }),
  getFrpAgentRouteHealth: (serverId: number) =>
    request<import('./types').FrpAgentRouteHealth[]>(`/api/frpmulti/servers/${serverId}/agent/routes/health`),
  getFrpRouteHealthSummary: () =>
    request<import('./types').FrpRouteHealthSummary>('/api/frpmulti/routes/summary'),
  getFrpRouteUptime: (hours = 24) =>
    request<{ hours: number; items: import('./types').FrpRouteUptime[] }>(`/api/frpmulti/routes/uptime?hours=${hours}`),

  // 指标历史：窗口内的序列已按桶取平均，拿到就能直接画
  getMetricsHistory: (hours = 24, buckets = 48) =>
    request<import('./types').MetricsHistory>(`/api/metrics/history?hours=${hours}&buckets=${buckets}`),

  // 匿名状态页：未开启或访问码不对时后端回 404
  getStatusPage: (code?: string) =>
    request<import('./types').StatusPagePayload>(
      `/api/status-page${code ? `?code=${encodeURIComponent(code)}` : ''}`,
    ),
  removeFrpAgentRoute: (serverId: number, proxyId: number) => request<{ ok: boolean }>(`/api/frpmulti/servers/${serverId}/agent/routes/${proxyId}`, { method: 'DELETE' }),
  pushFrpConfigToAgent: (id: number) => request<{ ok: boolean }>(`/api/frpmulti/servers/${id}/agent/frps-config`, { method: 'POST' }),
  frpsServerAction: (id: number, action: 'start' | 'stop' | 'restart') =>
    request<{ ok: boolean }>(`/api/frpmulti/servers/${id}/agent/frps-action`, { method: 'POST', body: JSON.stringify({ action }) }),
  frpsServerActionRaw: (id: number, action: 'start' | 'stop' | 'restart') =>
    request<{ ok: boolean; error?: string }>(`/api/frpmulti/servers/${id}/agent/frps-action`, { method: 'POST', body: JSON.stringify({ action }) }),
  installFrpServer: (id: number, version: string, proxy: string) =>
    request<Record<string, any>>(`/api/frpmulti/servers/${id}/agent/frps-install`, { method: 'POST', body: JSON.stringify({ version, proxy }) }),

  getAccessLogs: (params?: { limit?: number; keyword?: string; status?: number }) => {
    const q = new URLSearchParams()
    if (params?.limit) q.set('limit', String(params.limit))
    if (params?.keyword) q.set('keyword', params.keyword)
    if (params?.status) q.set('status', String(params.status))
    return request<AccessLogEntry[]>(`/api/logs/access?${q}`)
  },
  getErrorLogs: (params?: { limit?: number; keyword?: string }) => {
    const q = new URLSearchParams()
    if (params?.limit) q.set('limit', String(params.limit))
    if (params?.keyword) q.set('keyword', params.keyword)
    return request<string[]>(`/api/logs/error?${q}`)
  },
  getSystemLogs: (params?: { limit?: number; level?: string; keyword?: string }) => {
    const q = new URLSearchParams()
    if (params?.limit) q.set('limit', String(params.limit))
    if (params?.level) q.set('level', params.level)
    if (params?.keyword) q.set('keyword', params.keyword)
    return request<SystemLogEntry[]>(`/api/logs/system?${q}`)
  },

  scanDiscovery: (host?: string) => {
    const q = new URLSearchParams()
    if (host) q.set('host', host)
    return request<DiscoveredService[]>(`/api/discovery/scan?${q}`)
  },

  async exportBackup() {
    const response = await fetch('/api/backup/export', { credentials: 'include' })
    if (!response.ok) {
      throw new Error('导出备份失败')
    }
    const blob = await response.blob()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `havline-backup-${new Date().toISOString().slice(0, 10)}.tar.gz`
    link.click()
    URL.revokeObjectURL(url)
  },

  listBackupArchives: () =>
    request<{ archives: import('./types').BackupArchive[]; keep: number }>('/api/backup/archives'),
  createBackupArchive: () =>
    request<{ name: string; removed: number }>('/api/backup/archives', { method: 'POST' }),
  restoreBackupArchive: (name: string) =>
    request<{ message: string }>(`/api/backup/archives/${encodeURIComponent(name)}/restore`, { method: 'POST' }),
  deleteBackupArchive: (name: string) =>
    request<{ ok: boolean }>(`/api/backup/archives/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  async downloadBackupArchive(name: string) {
    const response = await fetch(`/api/backup/archives/${encodeURIComponent(name)}`, { credentials: 'include' })
    if (!response.ok) {
      throw new Error('下载备份失败')
    }
    const blob = await response.blob()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = name
    link.click()
    URL.revokeObjectURL(url)
  },

  async restoreBackup(file: File) {
    const form = new FormData()
    form.append('file', file)
    const response = await fetch('/api/backup/restore', {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
    if (!response.ok) {
      let message = '恢复备份失败'
      try {
        const body = (await response.json()) as ApiError
        if (body.error) message = body.error
      } catch {
        // ignore
      }
      throw new Error(message)
    }
    return (await response.json()) as { message: string }
  },

  listCfTunnels: () => request<CfTunnel[]>('/api/cloudflare/tunnels'),
  getCfTunnel: (id: number) => request<CfTunnel>(`/api/cloudflare/tunnels/${id}`),
  createCfTunnel: (payload: CfTunnelPayload) =>
    request<CfTunnel>('/api/cloudflare/tunnels', { method: 'POST', body: JSON.stringify(payload) }),
  updateCfTunnel: (id: number, payload: CfTunnelPayload) =>
    request<CfTunnel>(`/api/cloudflare/tunnels/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteCfTunnel: (id: number) =>
    request<{ ok: boolean }>(`/api/cloudflare/tunnels/${id}`, { method: 'DELETE' }),
  startCfTunnel: (id: number) =>
    request<CfTunnel>(`/api/cloudflare/tunnels/${id}/start`, { method: 'POST' }),
  stopCfTunnel: (id: number) =>
    request<CfTunnel>(`/api/cloudflare/tunnels/${id}/stop`, { method: 'POST' }),
  restartCfTunnel: (id: number) =>
    request<CfTunnel>(`/api/cloudflare/tunnels/${id}/restart`, { method: 'POST' }),
  refreshCfCredentials: (id: number) =>
    request<CfTunnel>(`/api/cloudflare/tunnels/${id}/credentials/refresh`, { method: 'POST' }),
  getCfTunnelStatus: (id: number) =>
    request<CfRuntimeStatus>(`/api/cloudflare/tunnels/${id}/status`),
  getCfTunnelLogs: (id: number, lines = 200) =>
    request<{ output: string }>(`/api/cloudflare/tunnels/${id}/logs?lines=${lines}`),
  getCfTunnelConfig: (id: number) =>
    request<{ content: string }>(`/api/cloudflare/tunnels/${id}/config`),
  getCfTunnelRoutes: (id: number) =>
    request<CfTunnelRoutes>(`/api/cloudflare/tunnels/${id}/routes`),
  replaceCfTunnelRoutes: (id: number, routes: Array<{ hostname: string; path: string; service: string }>) =>
    request<CfTunnelRoutes>(`/api/cloudflare/tunnels/${id}/routes`, {
      method: 'PUT',
      body: JSON.stringify({ routes }),
    }),
  getCfTunnelDrift: (id: number) => request<CfDriftResult>(`/api/cloudflare/tunnels/${id}/drift`),
  syncCfTunnelRoutes: (id: number) =>
    request<{ ok: boolean }>(`/api/cloudflare/tunnels/${id}/sync`, { method: 'POST' }),
  getCfTunnelDiagnostics: (id: number) =>
    request<CfDiagnosticsResult>(`/api/cloudflare/tunnels/${id}/diagnostics`),
  getCfBinary: () => request<{ version: string; mirrors: CfMirror[] }>('/api/cloudflare/binary'),
  downloadCfBinary: (mirror: string, version = '') =>
    request<{ ok: boolean; version: string }>('/api/cloudflare/binary/download', {
      method: 'POST',
      body: JSON.stringify({ mirror, version }),
    }),
  getLatestCfBinary: () => request<{ version: string }>('/api/cloudflare/binary/latest'),
  rollbackCfBinary: () =>
    request<{ ok: boolean; version: string }>('/api/cloudflare/binary/rollback', { method: 'POST' }),
  getCfIngressCandidates: () =>
    request<{ hostnames: string[] }>('/api/cloudflare/ingress-candidates'),
  getCfZones: () => request<{ zones: CfZone[] }>('/api/cloudflare/zones'),
  getCfAccessStatus: (hostname: string) =>
    request<CfAccessStatus>(`/api/cloudflare/access-status?hostname=${encodeURIComponent(hostname)}`),
  getCfPreflight: () => request<CfPreflightResult>('/api/cloudflare/preflight'),
  testCfRoute: (service: string) =>
    request<CfRouteTestResult>('/api/cloudflare/routes/test', {
      method: 'POST',
      body: JSON.stringify({ service }),
    }),
  getCfSettings: () => request<CfAppSettings>('/api/cloudflare/settings'),
  saveCfSettings: (payload: CfAppSettingsInput) =>
    request<CfAppSettings>('/api/cloudflare/settings', { method: 'PUT', body: JSON.stringify(payload) }),
  testCfToken: (accountId: string, token: string, kind: 'tunnel' | 'api') =>
    request<{ ok: boolean; message: string }>('/api/cloudflare/settings/test', {
      method: 'POST',
      body: JSON.stringify({ account_id: accountId, token, kind }),
    }, 60_000),
  setCfEnabled: (enabled: boolean) =>
    request<CfAppSettings>('/api/cloudflare/enabled', {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
}
