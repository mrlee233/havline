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
          : '请求超时，请检查 Fonu 服务或 Nginx 状态',
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
  getChinaCIDRStatus: () => request<ChinaCIDRStatus>('/api/settings/china-cidr'),
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
  probeFrpAgent: (id: number) => request<Record<string, any>>(`/api/frpmulti/servers/${id}/agent/probe`, { method: 'POST' }),
  installFrpAgent: (id: number) => request<FrpServer>(`/api/frpmulti/servers/${id}/agent/install`, { method: 'POST' }),
  uninstallFrpAgent: (id: number, keepData: boolean) => request<{ ok: boolean; output: string }>(`/api/frpmulti/servers/${id}/agent/uninstall?keep_data=${keepData}`, { method: 'POST' }),
  sshDiagnoseFrpAgent: (id: number) => request<{ output: string }>(`/api/frpmulti/servers/${id}/agent/ssh-diagnose`, { method: 'POST' }),
  getFrpAgentStatus: (id: number) => request<import('./types').FrpAgentStatus>(`/api/frpmulti/servers/${id}/agent/status`),
  getFrpAgentRoutes: (id: number) => request<import('./types').FrpAgentRouteDetail[]>(`/api/frpmulti/servers/${id}/agent/routes`),
  getFrpAgentRouteCandidates: (id: number) => request<import('./types').FrpProxy[]>(`/api/frpmulti/servers/${id}/agent/route-candidates`),
  deployFrpAgentRoute: (serverId: number, proxyId: number) => request<{ ok: boolean }>(`/api/frpmulti/servers/${serverId}/agent/routes/${proxyId}`, { method: 'POST' }),
  getFrpAgentRouteConf: (serverId: number, proxyId: number) =>
    request<{ proxy_id: number; domain: string; content: string }>(`/api/frpmulti/servers/${serverId}/agent/routes/${proxyId}/conf`),
  installFrpAgentNginx: (serverId: number) =>
    request<{ ok: boolean; manager?: string; output?: string; path?: string; error?: string }>(`/api/frpmulti/servers/${serverId}/agent/nginx/install`, { method: 'POST' }),
  getFrpAgentRouteHealth: (serverId: number) =>
    request<import('./types').FrpAgentRouteHealth[]>(`/api/frpmulti/servers/${serverId}/agent/routes/health`),
  getFrpRouteHealthSummary: () =>
    request<import('./types').FrpRouteHealthSummary>('/api/frpmulti/routes/summary'),
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
    link.download = `fonu-backup-${new Date().toISOString().slice(0, 10)}.tar.gz`
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
}
