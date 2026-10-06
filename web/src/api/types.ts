export interface ProxyHost {
  id: number
  hostname: string
  listen_port?: number | null
}

export interface BasicAuthConfig {
  enabled?: boolean
  username?: string
  has_password?: boolean
}

export interface RateLimitConfig {
  enabled?: boolean
  rate?: number
  burst?: number
}

export interface ConnLimitConfig {
  enabled?: boolean
  max?: number
}

export interface ProxySecurityConfig {
  ip_blacklist?: string[]
  ip_whitelist?: string[]
  ip_whitelist_mode?: boolean
  china_only?: boolean
  basic_auth?: BasicAuthConfig
  rate_limit?: RateLimitConfig
  conn_limit?: ConnLimitConfig
  proxy_ssl_verify_off?: boolean
  proxy_host_upstream?: boolean
  tls_min_13_only?: boolean
  security_headers?: boolean
}

export interface ProxyRule {
  id: number
  domain: string
  upstream: string
  listen_port: number
  listen_ipv4: boolean
  listen_ipv6: boolean
  hosts: ProxyHost[]
  https_enabled: boolean
  http_redirect: boolean
  enabled: boolean
  nginx_mode?: string
  name: string
  sort_order: number
  security: ProxySecurityConfig
  exits: string[]
  cf_tunnel_id: number
  sync_warning?: string
  created_at: string
  updated_at: string
}

export interface ChinaCIDRStatus {
  updated_at?: string
  entry_count_v4: number
  entry_count_v6: number
  last_error?: string
  updating?: boolean
  ready?: boolean
}

export interface ProxyTraffic {
  rule_id: number
  upload_total: number
  download_total: number
  upload_rate: number
  download_rate: number
  connections: number
}

export interface ProxyClientConn {
  ip: string
  last_seen: string
}

export interface ProxySecurityPayload {
  ip_blacklist?: string[]
  ip_whitelist?: string[]
  ip_whitelist_mode?: boolean
  ip_blacklist_text?: string
  ip_whitelist_text?: string
  china_only?: boolean
  basic_auth?: {
    enabled?: boolean
    username?: string
    password?: string
  }
  rate_limit?: RateLimitConfig
  conn_limit?: ConnLimitConfig
  proxy_ssl_verify_off?: boolean
  proxy_host_upstream?: boolean
  tls_min_13_only?: boolean
  security_headers?: boolean
}

export interface ProxySavePayload {
  upstream: string
  listen_port?: number
  listen_ipv4?: boolean
  listen_ipv6?: boolean
  hosts: string[]
  https_enabled?: boolean
  http_redirect?: boolean
  enabled?: boolean
  name?: string
  security?: ProxySecurityPayload
  exits?: string[]
  cf_tunnel_id?: number
}

export interface AuthStatus {
  initialized: boolean
  authenticated: boolean
}

export interface AppVersion {
  version: string
  nginx_http_port: number
  nginx_https_port: number
}

export interface UpdateStatus {
  enabled: boolean
  mode: string
  current_version?: string
  latest_version?: string
  update_available: boolean
  busy: boolean
  phase: 'idle' | 'scheduled' | 'running' | 'success' | 'failed' | string
  message: string
  log?: string[]
  checked_at?: string
}

export interface ApiError {
  error: string
}

export interface DashboardStatus {
  public_ipv4: string
  public_ipv6: string
  public_ip_source?: 'ddns' | 'detect' | 'none'
  ddns_status: string
  ddns_count: number
  ddns_last_updated?: string
  certificate_status: string
  certificate_days: number
  certificate_count: number
  certificate_summary?: string
  proxy_count: number
  nginx_status: string
  request_today: number
  request_yesterday?: number
  request_trend?: number | null
  requests_hourly: number[]
  requests_hourly_labels?: string[]
  error_today: number
  avg_response_ms: number
  started_at: string
  uptime_seconds: number
  cpu_percent: number
  mem_used_bytes: number
  mem_total_bytes: number
  disk_used_bytes: number
  disk_total_bytes: number
  cloudflare_count: number
  cloudflare_online: number
  cloudflare_status: string
  cloudflare_message?: string
}

export interface DDNSDomainRecord {
  domain: string
  ipv4?: string
  ipv6?: string
  status: string
  message?: string
}

export interface DDNSConfig {
  id: number
  provider: string
  root_domain: string
  record_name: string
  record_names?: string[]
  ipv4_enabled: boolean
  ipv6_enabled: boolean
  enabled: boolean
  has_token: boolean
  last_ipv4?: string
  last_ipv6?: string
  last_status?: string
  last_error?: string
  custom_ipv4?: string
  custom_ipv6?: string
  domain_records?: DDNSDomainRecord[]
  last_updated_at?: string
}

export interface CertificateCAOption {
  value: string
  label: string
}

export interface CertificateJobDone {
  ok: boolean
  error?: string
  domain?: string
  cert_path?: string
  key_path?: string
  cert_dir?: string
  expires_at?: string
}

export interface CertificateJobEvent {
  type: 'log' | 'done'
  level?: string
  message?: string
  result?: CertificateJobDone
}

export interface CertificateRecord {
  id: number
  domain: string
  domains?: string[]
  wildcard: boolean
  acme_ca?: string
  status: string
  days_left: number
  last_error?: string
  expires_at?: string
  last_renew_at?: string
}

export interface AccessLogEntry {
  time: string
  domain: string
  server_port?: number
  method: string
  path: string
  status: number
  response_time: number
  client_ip: string
  upstream: string
}

export interface SystemLogEntry {
  time: string
  level: string
  module: string
  message: string
}

export type NotifyType = '' | 'email' | 'webhook' | 'telegram'
export type WebhookProvider = 'bark' | 'ntfy' | 'gotify' | 'wecom' | 'dingtalk' | 'feishu' | 'custom'

export interface NotifyEmailConfig {
  host: string
  port: number
  username: string
  from: string
  to: string[]
  tls: boolean
  has_password?: boolean
}

export interface NotifyWebhookConfig {
  provider: WebhookProvider
  server: string
  key: string
  topic: string
  url: string
  has_secret?: boolean
}

export interface NotifyTelegramConfig {
  chat_id: string
  proxy_url: string
  has_bot_token?: boolean
}

export interface NotifyTestPayload {
  type: NotifyType
  email: NotifyEmailConfig
  webhook: NotifyWebhookConfig
  telegram: NotifyTelegramConfig
  on_ddns_ip_change: boolean
  on_ddns_failure: boolean
  on_cert_expiry: boolean
  on_cert_renew_success: boolean
  on_cert_renew_failure: boolean
  on_ip_frequent_access: boolean
  on_login_failure: boolean
  on_nginx_reload_failure: boolean
  ip_frequent_threshold: number
  ip_frequent_window_sec: number
  login_failure_threshold: number
  login_failure_window_sec: number
  smtp_password?: string
  webhook_secret?: string
  telegram_token?: string
}

export type SettingsMap = Record<string, string>

export interface DiscoveredService {
  name: string
  port: number
  host: string
  upstream: string
  detected: boolean
  platform: string
  title?: string
  suggestion?: string
}

export interface DDNSSavePayload {
  provider?: string
  root_domain?: string
  record_name?: string
  record_names?: string[]
  domains?: string[]
  ipv4_enabled?: boolean
  ipv6_enabled?: boolean
  enabled?: boolean
  custom_ipv4?: string
  custom_ipv6?: string
  api_token?: string
  api_token_id?: string
  api_secret?: string
}

export interface DDNSTestPayload {
  config_id?: number
  provider?: string
  api_token?: string
  api_token_id?: string
  api_secret?: string
}

export interface FRPStatus {
  enabled: boolean
  connected: boolean
  message: string
  last_error?: string
  server_endpoint?: string
  connected_at?: string
  uptime_seconds?: number
  client_version?: string
  http_gateway_enabled?: boolean
  https_gateway_enabled?: boolean
  synced_domain_count?: number
}

export interface FRPConfig {
  enabled: boolean
  server_addr: string
  server_port: number
  auth_token: string
  has_auth_token: boolean
  tls_enabled: boolean
  custom_domains: string[]
}

export interface FRPResponse {
  enabled: boolean
  server_addr: string
  server_port: number
  auth_token: string
  has_auth_token: boolean
  tls_enabled: boolean
  custom_domains: string[]
  status: FRPStatus
  frps_config: string
  nginx_http_port: number
  nginx_https_port: number
}

export interface FRPSaveResponse {
  message: string
  config: FRPConfig
  status: FRPStatus
  frps_config: string
  nginx_http_port: number
  nginx_https_port: number
}

export interface FRPSavePayload {
  enabled: boolean
  server_addr: string
  server_port: number
  auth_token?: string
  tls_enabled: boolean
  custom_domains: string[]
}

// ===== FRP 多服务端模块（/api/frpmulti，0.1.25 核心迁移）=====

export interface FrpServer {
  agent_url?: string
  agent_transport: 'tunnel' | 'https-pin' | 'http' | string
  agent_local_port?: number
  agent_tls_pin?: string
  agent_tls_pin_prev?: string
  agent_tls_pin_prev_until?: string
  agent_tls_not_after?: string
  agent_firewall_state?: string
  agent_listen_addr?: string
  agent_transport_state: string
  agent_transport_error?: string
  agent_configured?: boolean
  ssh_configured?: boolean
  mgmt_enabled?: boolean
  ssh_host?: string
  ssh_port?: number
  ssh_user?: string
  ssh_auth?: string
  id: number
  name: string
  server_addr: string
  server_port: number
  tls_enabled: boolean
  tls_server_name?: string
  enabled: boolean
  has_token: boolean
  has_oidc_client_secret: boolean
  dashboard_has_pwd?: boolean
  options: FrpServerOptions
  boot_status: 'starting' | 'running' | 'stopped' | 'error'
  desired_state: 'starting' | 'running' | 'stopped' | 'error' | string
  process_state: 'starting' | 'running' | 'stopped' | 'error' | 'unknown' | string
  connection_state: 'connected' | 'connecting' | 'disconnected' | 'error' | 'unknown' | string
  state_source: string
  process_pid?: number
  exit_code?: number
  last_connected_at?: string
  last_disconnected_at?: string
  last_error?: string
  last_started_at?: string
  created_at: string
  updated_at: string
}

export interface FrpServerOptions {
  user?: string
  location?: string
  /** frps 的 vhost 监听端口（http/https 反代回源端口），与 frps.toml 和 Nginx upstream 同源；留空用默认 8080/8443 */
  vhost_http_port?: number
  vhost_https_port?: number
  /** frps 进阶：子域名根（规则 subdomain 生效前提）*/
  sub_domain_host?: string
  /** HTTP vhost 404 页面路径（VPS 本地路径）*/
  custom_404_page?: string
  /** 强制 TLS：只接受 TLS 连接（frpc 必须开 transport.tls.enable）*/
  tls_force?: boolean
  /** 端口白名单：逗号分隔的单端口或范围，如 6000,6005-6010；留空不限制 */
  allow_ports?: string
  /** 每客户端可绑定端口上限；0/留空不限 */
  max_ports_per_client?: number
  /** tcpmux 规则的 HTTP CONNECT 监听端口 */
  tcpmux_http_connect_port?: number
  /** frps 写文件日志 /var/log/frps.log（journalctl 之外）*/
  frps_log_to_file?: boolean
  /** 文件日志保留天数，默认 3 */
  frps_log_max_days?: number
  /** frps 管理接口（webServer），仅用于 Havline 拉取穿透流量等只读信息 */
  dashboard_addr?: string
  dashboard_port?: number
  dashboard_user?: string
  dashboard_has_pwd?: boolean
  auth_method: 'none' | 'token' | 'oidc'
  oidc_client_id?: string
  oidc_audience?: string
  oidc_scope?: string
  oidc_token_endpoint_url?: string
  oidc_trusted_ca_path?: string
  oidc_insecure_skip_verify?: boolean
  oidc_proxy_url?: string
  log_level: 'trace' | 'debug' | 'info' | 'warn' | 'error'
  log_max_days: number
  protocol: 'tcp' | 'kcp' | 'quic' | 'websocket' | 'wss'
  proxy_url?: string
  dial_server_timeout: number
  dial_server_keepalive: number
  connect_server_local_ip?: string
  dns_server?: string
  pool_count: number
  tcp_mux: boolean
  tcp_mux_keepalive_interval: number
  heartbeat_interval: number
  heartbeat_timeout: number
  login_fail_exit: boolean
  tls_disable_custom_first_byte: boolean
  auto_start: boolean
  remark?: string
  tls_certificate_configured?: boolean
  tls_key_configured?: boolean
  tls_trusted_ca_configured?: boolean
}

export interface FrpProxy {
  id: number
  server_id: number
  name: string
  type: 'tcp' | 'udp' | 'http' | 'https' | 'tcpmux' | 'stcp' | 'sudp' | 'xtcp'
  local_ip: string
  local_port: number
  remote_port?: number | null
  custom_domains: string[]
  host_header_rewrite?: string
  options: FrpProxyOptions
  enabled: boolean
  remark?: string
  created_at: string
  updated_at: string
}

export interface FrpProxyOptions {
  subdomain?: string
  /** WebSocket 开关：仅作用于公网 Nginx 反代模板（加 Upgrade/Connection 头） */
  websocket?: boolean
  /** 公网反代：HTTP 80 强制 301 跳转 HTTPS（需公网证书已部署）*/
  redirect_https?: boolean
  /** 公网反代：IP 白名单（allow/deny），空则不限制 */
  allow_ips?: string[]
  /** 公网反代：Basic Auth 用户名；密码由 agent 首次部署自动生成（见配置注释）*/
  basic_auth_user?: string
  /** 公网反代：IP 黑名单（deny），与白名单可同时使用 */
  deny_ips?: string[]
  /** 公网反代：Basic Auth 自定义密码；留空则首次部署自动生成 */
  basic_auth_password?: string
  /** 公网反代：显式关闭 HTTPS（证书已推送也不监听 443）；缺省 = 启用 */
  https_disabled?: boolean
  /** 公网反代：客户端上传大小限制，如 50m */
  client_max_body_size?: string
  /** 公网反代：代理读超时，如 3600s（长轮询 / 流媒体 / 大文件必需）*/
  proxy_read_timeout?: string
  /** 公网反代：安全响应头（HSTS / X-Frame-Options / X-Content-Type-Options / Referrer-Policy）*/
  security_headers?: boolean
  /** 公网反代：仅允许 TLS 1.3 */
  tls13_only?: boolean
  /** 公网反代：请求限流（次/秒）；0 = 不限制 */
  rate_limit_rate?: number
  /** 公网反代：限流突发允许量 */
  rate_limit_burst?: number
  /** 公网反代：每 IP 并发连接数上限；0 = 不限制 */
  conn_limit_max?: number
  /** 公网反代：仅允许中国大陆 IP（部署时自动下发本地维护的 IP 段）*/
  china_only?: boolean
  locations?: string[]
  route_by_http_user?: string
  http_user?: string
  http_password?: string
  multiplexer?: string
  secret_key?: string
  allow_users?: string[]
  transport: {
    bandwidth_limit?: string
    bandwidth_limit_mode?: 'client' | 'server' | string
    use_encryption?: boolean
    use_compression?: boolean
    proxy_protocol_version?: 'v1' | 'v2' | string
  }
  health_check: {
    type?: 'tcp' | 'http' | string
    path?: string
    interval_seconds?: number
    max_failed?: number
    timeout_seconds?: number
    http_headers?: Array<{ name: string; value: string }>
  }
  load_balancer: { group?: string; group_key?: string }
  metadatas?: Record<string, string>
  request_headers?: Record<string, string>
  response_headers?: Record<string, string>
  plugin?: {
    type: string
    unix_path?: string
    local_path?: string
    strip_prefix?: string
    local_addr?: string
    http_user?: string
    http_password?: string
    username?: string
    password?: string
    crt_path?: string
    key_path?: string
    host_header_rewrite?: string
    destination_ip?: string
  }
  nat_traversal: { disable_assisted_addrs?: boolean }
}

/** frps 侧单条穿透规则的流量快照；速率为两次采样间隔内的均值 */
export interface FrpProxyTraffic {
  traffic_in: number
  traffic_out: number
  cur_conns: number
  traffic_in_rate: number
  traffic_out_rate: number
}

/** 按 frp 规则 ID 聚合的流量数据；errors 记录采集失败的服务端/规则错误 */
export interface FrpProxiesTraffic {
  items: Record<string, FrpProxyTraffic>
  errors?: Record<string, string>
  sampled_at: string
}

export interface FrpStatus {
  status: 'running' | 'stopped' | 'unknown'
  pid?: number
  frpc_version?: string
  last_started_at?: string
  last_reloaded_at?: string
  last_error?: string
  configured: boolean
  enabled_proxies: number
}

export interface FrpRuntimeInfo {
  platform: string
  configured_bin: string
  active_bin?: string
  active_version?: string
  detected_version?: string
  download_proxy: string
  config_path: string
  config_paths?: string[]
  config_exists: boolean
}

export interface FrpRelease {
  version: string
  published_at?: string
  asset_name: string
  size: number
}

export interface FrpBinary {
  version: string
  path: string
  size: number
  installed_at: string
  active: boolean
}

/** 单项诊断结果；state 为 ok/failed/unknown，unknown 表示无法确认而非失败 */
export interface FrpDiagnosticCheck {
  name: 'tcp_reachable' | 'frpc_login' | 'proxy_reachable' | 'dns_resolution' | 'location' | string
  state: 'ok' | 'failed' | 'unknown' | string
  detail?: string
  latency_ms?: number
}

export interface FrpServerDiagnostics {
  server_id: number
  available: boolean
  latency_ms?: number
  location?: string
  checks?: FrpDiagnosticCheck[]
  error?: string
}

export interface FrpServerSample {
  checked_at: string
  available: boolean
  latency_ms?: number
  error?: string
}

export interface FrpServerStatusDetail {
  available: boolean
  availability: number
  average_latency_ms: number
  peak_latency_ms: number
  samples: FrpServerSample[]
  last_checked_at?: string
  last_error?: string
}

export interface FrpServerDetail {
  server: FrpServer
  status: FrpServerStatusDetail
  proxies: FrpProxy[]
}

export interface FrpDownloadTask {
  version?: string
  status: 'idle' | 'queued' | 'downloading' | 'verifying' | 'extracting' | 'completed' | 'failed'
  message: string
  downloaded: number
  total: number
  error?: string
  updated_at?: string
}

export interface FrpServerPayload {
  agent_url?: string
  agent_token?: string
  clear_agent_token?: boolean
  ssh_host?: string
  ssh_port?: number
  ssh_user?: string
  ssh_auth?: string
  ssh_secret?: string
  clear_ssh_secret?: boolean
  mgmt_enabled?: boolean
  name: string
  server_addr: string
  server_port: number
  tls_enabled?: boolean
  tls_server_name?: string
  enabled?: boolean
  token?: string
  clear_token?: boolean
  options: FrpServerOptions
  oidc_client_secret?: string
  clear_oidc_client_secret?: boolean
  dashboard_password?: string
  clear_dashboard_password?: boolean
  tls_certificate?: string
  tls_key?: string
  tls_trusted_ca?: string
  clear_tls_certificate?: boolean
  clear_tls_key?: boolean
  clear_tls_trusted_ca?: boolean
}

export interface FrpAgentRouteCandidate {
  id: number
  name: string
  type: 'http' | 'https'
  custom_domains: string[]
  local_ip: string
  local_port: number
  enabled: boolean
}

export interface FrpAgentRouteHealth {
  proxy_id: number
  domain: string
  dns_ok: boolean
  dns_detail?: string
  cert_ok: boolean
  cert_detail?: string
  tunnel_ok: boolean
  /** 隧道状态是否判得出来：frps 管理接口不可用时 tunnel_ok 恒为 false，但那不代表隧道不通 */
  tunnel_known?: boolean
  tunnel_name?: string
  tunnel_detail?: string
  service_ok: boolean
}

export interface FrpRouteHealthSummary {
  servers: number
  routes: number
  healthy: number
  dns_failed: number
  cert_missing: number
  tunnel_down: number
  /** frps 管理接口不可用、隧道状态判不出来的条数 */
  tunnel_unknown?: number
  service_down: number
  failed_servers?: string[]
}

export interface FrpAgentCertificate {
  domain: string
  cert_path: string
  key_path: string
  not_after?: string
  days_left: number
}

export interface FrpAgentCerts {
  certs: FrpAgentCertificate[]
  warnings?: string[]
  dir?: string
  checked_at: string
}

export interface FrpAgentNginxReload {
  ok: boolean
  test_ok?: boolean
  test_error?: string
  reload_ok?: boolean
  reload_error?: string
  checked_at: string
}

export interface FrpAgentMetrics {
  cpu_percent: number
  mem_used_bytes: number
  mem_total_bytes: number
  disk_used_bytes: number
  disk_total_bytes: number
  disk_path?: string
  load_avg?: number[]
  uptime_seconds?: number
  checked_at: string
}

export interface FrpAgentNginxLogs {
  type: 'access' | 'error'
  path: string
  lines: string[]
  truncated: boolean
  checked_at: string
}

export interface FrpAgentFRPSConfig {
  path: string
  exists: boolean
  content: string
  truncated?: boolean
  mod_time?: string
  checked_at: string
}

export interface FrpRouteDriftItem {
  domain: string
  kind: 'missing' | 'orphan' | 'not_loaded' | 'content_mismatch'
  detail: string
  expected?: string
  actual?: string
}

export interface FrpRouteDriftReport {
  server_id: number
  expected: number
  deployed: number
  loaded: number
  items: FrpRouteDriftItem[]
  checked_at: string
}

export interface FrpRedeployRouteResult {
  proxy_id: number
  name: string
  domains: string
  ok: boolean
  error?: string
}

export interface FrpRedeployAllResult {
  server_id: number
  total: number
  succeeded: number
  failed: number
  results: FrpRedeployRouteResult[]
}

export interface FrpAgentRouteDetail {
  domain: string
  file_exists: boolean
  syntax_ok: boolean
  loaded: boolean
}

export interface FrpAgentStatus {
  configured?: boolean
  available?: boolean
  message?: string
  agent_version?: string
  listen_addr?: string
  listen_scope?: string
  remote_access_enabled?: boolean
  transport_security?: string
  https_proxy_configured?: boolean
  https_proxy_port?: number
  ports?: Record<string, boolean>
  frps_active?: boolean
  frps_enabled?: boolean
  frps_version?: string
  frps_log_tail?: string
  frps_unit_exec?: string
  dashboard_addr?: string
  service: string
  nginx_binary: string
  nginx_conf_dir: string
  /** nginx -t 是否通过 */
  nginx_ok?: boolean
  /** 未找到 nginx 可执行文件 */
  nginx_missing?: boolean
  /** 解析到的 nginx 路径 */
  nginx_path?: string
  /** 有 nginx 进程在运行 */
  nginx_running?: boolean
  /** nginx 版本，如 nginx/1.24.0 */
  nginx_version?: string
  /** 探测失败原因（缺失或配置有误） */
  nginx_error?: string
  frps_binary: string
  frps_config_path: string
  routes: string[]
  checked_at: string
}

export interface FrpProxyPayload {
  server_id: number
  name: string
  type: FrpProxy['type']
  local_ip: string
  local_port: number
  remote_port?: number | null
  custom_domains?: string[]
  host_header_rewrite?: string
  options?: FrpProxyOptions
  enabled?: boolean
  remark?: string
}

export interface NginxBackupEntry {
  name: string
  created_at: string
}

export interface RuleNginxView {
  mode: 'auto' | 'custom'
  enabled: boolean
  active: boolean
  generated: string
  content: string
  backups: NginxBackupEntry[]
}

export interface GlobalNginxView {
  mode: 'auto' | 'custom'
  generated_framework: string
  generated_snippet: string
  content: string
  backups: NginxBackupEntry[]
}

// 配置版本（本机主配置与公网 vhost 共用同一形状）
export interface NginxConfigVersionEntry {
  name: string
  size: number
  created_at: string
}

export interface NginxConfigVersionsView {
  message?: string
  content: string
  versions: NginxConfigVersionEntry[]
}

export interface NginxConfigVersionView {
  name: string
  content: string
}

// 公网 VPS 上某个域名 vhost 的版本历史（需 agent ≥ 0.12.2）
export interface FrpAgentRouteVersions {
  domain: string
  content: string
  versions: NginxConfigVersionEntry[]
}

export interface FrpAgentRouteVersion {
  domain: string
  name: string
  content: string
}

// API Token：明文只在创建响应里出现一次
// （令牌本身不含密钥材料，是随机串的 SHA-256 存库）
export interface ApiToken {
  id: number
  name: string
  scope: 'read' | 'write'
  expires_at?: string
  last_used_at?: string
  created_at: string
}

export interface CreatedApiToken extends ApiToken {
  token: string
}

// 留在服务器上的备份（自动备份 + 手动「立即备份」）
export interface BackupArchive {
  name: string
  size: number
  mod_time: string
}

// 规则可用率：由后台巡检落库的状态变更推导，不是页面现算
// （不可用区间与判不出来的时长分开给，unknown 不计入可用率分母）
export interface FrpRouteUptimeIncident {
  from: string
  to?: string
  reason?: string
  detail?: string
  minutes: number
}

export interface FrpRouteUptime {
  server_id: number
  domain: string
  state: string
  uptime_pct: number
  down_minutes: number
  unknown_minutes: number
  covered_minutes: number
  incidents: FrpRouteUptimeIncident[]
}

// 指标历史：scope=host 是本机，scope=route:<serverID>:<domain> 是单条规则的流量
export interface MetricsPoint {
  at: string
  value: number
}

export interface MetricsSeries {
  scope: string
  metric: string
  points: MetricsPoint[]
}

export interface MetricsHistory {
  hours: number
  buckets: number
  series: MetricsSeries[]
}

// 公开状态页（匿名只读）：只有域名与可用率，不含内网地址/端口/证书路径
export interface StatusPageRoute {
  domain: string
  state: string
  source?: 'frp' | 'cloudflare' | string
  uptime_pct: number
  down_minutes: number
  unknown_minutes: number
  incidents: StatusPageIncident[]
}

export interface StatusPageIncident {
  from: string
  to?: string
  reason?: string
  detail?: string
  minutes: number
}

export interface StatusPagePayload {
  title: string
  code_required: boolean
  checked_at: string
  up: number
  down: number
  unknown: number
  routes: StatusPageRoute[]
}

// Cloudflare Tunnel 独立模块
export interface CfNetworkSettings {
  transport_protocol: string
  edge_ip_version: string
  ha_connections: number
  proxy_mode: string
  origin_ca_pool?: string
  no_tls_verify?: boolean
  http_host_header?: string
}

export interface CfTunnel {
  id: number
  name: string
  mode: string
  tunnel_id: string
  account_tag: string
  config_path: string
  creds_path: string
  log_path: string
  network: CfNetworkSettings
  status: string
  last_error?: string
  metrics_port?: number
  auto_start: boolean
  managed: boolean
  hostnames?: string[]
  origin_service?: string
  dns_warning?: string
  created_at: string
  updated_at: string
}

export interface CfRuntimeStatus {
  running: boolean
  status: string
  last_error?: string
  metrics_port?: number
  connections: number
  latency_ms: number
  bandwidth_in: number
  bandwidth_out: number
  transport: string
  bandwidth_notice?: string
}

export interface CfMirror {
  id: string
  name: string
  base: string
}

export interface CfZone {
  id: string
  name: string
}

export interface CfTunnelPayload {
  name: string
  mode: string
  token?: string
  hostnames: string[]
  origin_service?: string
  network: CfNetworkSettings
  auto_start: boolean
  managed: boolean
}

export interface CfAppSettings {
  account_id: string
  enabled: boolean
  default_token_configured: boolean
  default_token_masked: string
  api_token_configured: boolean
  api_token_masked: string
  mirror: string
  network: CfNetworkSettings
}

export interface CfAppSettingsInput {
  account_id: string
  enabled?: boolean
  token: string
  api_token: string
  mirror: string
  network: CfNetworkSettings
}

export interface CfDriftResult {
  tunnel_id: number
  managed: boolean
  has_drift: boolean
  items: string[]
}

export interface CfRouteView {
  id?: number
  hostname: string
  path: string
  service: string
  source: 'rule' | 'config' | string
}

export interface CfTunnelRoutes {
  managed: boolean
  routes: CfRouteView[]
  catch_all: string
}

export interface CfDiagnosticCheck {
  name: string
  status: 'ok' | 'warning' | 'error' | string
  message: string
}

export interface CfDiagnosticsResult {
  tunnel_id: number
  generated_at: string
  checks: CfDiagnosticCheck[]
}

export interface CfPreflightCheck {
  name: string
  status: 'ok' | 'error' | string
  message: string
  action?: string
}

export interface CfPreflightResult {
  ready: boolean
  checks: CfPreflightCheck[]
}

export interface CfRouteTestResult {
  ok: boolean
  message: string
  latency_ms: number
}

export interface CfAccessStatus {
  enabled: boolean
  name?: string
}
