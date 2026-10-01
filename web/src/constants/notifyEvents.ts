export interface NotifyEventOption {
  key: string
  label: string
  hint?: string
  hasThreshold?: boolean
  thresholdKey?: 'ip_frequent' | 'login_failure' | 'resource' | 'tunnel_fail' | 'frps_fail' | 'agent_fail' | 'drift_cooldown'
}

export interface NotifyEventGroup {
  title: string
  events: NotifyEventOption[]
}

export const NOTIFY_EVENT_GROUPS: NotifyEventGroup[] = [
  {
    title: 'DDNS',
    events: [
      {
        key: 'on_ddns_ip_change',
        label: 'IP 变更',
        hint: '公网 IP 变更并成功同步到 DNS',
      },
      {
        key: 'on_ddns_failure',
        label: '更新失败',
      },
    ],
  },
  {
    title: '证书',
    events: [
      {
        key: 'on_cert_expiry',
        label: '即将到期',
        hint: '进入续签阈值后每天提醒一次',
      },
      {
        key: 'on_cert_renew_success',
        label: '续签成功',
      },
      {
        key: 'on_cert_renew_failure',
        label: '续签失败',
      },
    ],
  },
  {
    title: '安全',
    events: [
      {
        key: 'on_ip_frequent_access',
        label: 'IP 频繁访问',
        hint: '同一 IP 在统计窗口内超过阈值',
        hasThreshold: true,
        thresholdKey: 'ip_frequent',
      },
      {
        key: 'on_login_failure',
        label: '登录异常',
        hint: '同一 IP 在窗口内多次登录失败',
        hasThreshold: true,
        thresholdKey: 'login_failure',
      },
    ],
  },
  {
    title: '系统',
    events: [
      {
        key: 'on_nginx_reload_failure',
        label: 'Nginx 重载失败',
        hint: '保存反代或证书后 Nginx 未能成功重载',
      },
      {
        key: 'on_backup_failure',
        label: '自动备份失败',
        hint: '定时备份导出或校验失败——没有备份这件事，往往到需要它的那天才发现',
      },
      {
        key: 'on_resource_threshold',
        label: '资源占用过高',
        hint: 'CPU / 内存 / 磁盘持续超过阈值，同一项每 30 分钟最多提醒一次',
        hasThreshold: true,
        thresholdKey: 'resource',
      },
      {
        key: 'on_tunnel_down',
        label: '隧道连续不通',
        hint: '连续多次巡检未在 frps 上注册，恢复时再通知一次',
        hasThreshold: true,
        thresholdKey: 'tunnel_fail',
      },
      {
        key: 'on_frps_down',
        label: '公网服务端掉线',
        hint: '服务端进程没在跑或隧道连不上 frps（手动停止的不报），恢复时再通知一次',
        hasThreshold: true,
        thresholdKey: 'frps_fail',
      },
      {
        key: 'on_agent_unreachable',
        label: '公网 agent 不可达',
        hint: 'VPS 上的 havline-agent 连续无响应（重启后没起来最常见），恢复时再通知一次',
        hasThreshold: true,
        thresholdKey: 'agent_fail',
      },
      {
        key: 'on_route_drift',
        label: '配置漂移',
        hint: 'VPS 上的 vhost 与 Havline 记录不一致（被手工改过或重装后未重部署），同一台按提醒间隔重复提醒',
        hasThreshold: true,
        thresholdKey: 'drift_cooldown',
      },
    ],
  },
]

export const DEFAULT_IP_FREQUENT_THRESHOLD = 100
export const DEFAULT_IP_FREQUENT_WINDOW_SEC = 60
export const DEFAULT_LOGIN_FAILURE_THRESHOLD = 5
export const DEFAULT_LOGIN_FAILURE_WINDOW_SEC = 300
export const DEFAULT_CPU_THRESHOLD = 90
export const DEFAULT_MEMORY_THRESHOLD = 90
export const DEFAULT_DISK_THRESHOLD = 85
export const DEFAULT_TUNNEL_FAIL_THRESHOLD = 3
export const DEFAULT_FRPS_FAIL_THRESHOLD = 2
export const DEFAULT_AGENT_FAIL_THRESHOLD = 3
export const DEFAULT_DRIFT_COOLDOWN_HOURS = 24
