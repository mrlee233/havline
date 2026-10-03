import { createRouter, createWebHistory } from 'vue-router'
import { api } from '../api/client'
import AppLayout from '../layouts/AppLayout.vue'
import LoginView from '../views/LoginView.vue'
import ProxyView from '../views/ProxyView.vue'
import DashboardView from '../views/DashboardView.vue'
import DdnsView from '../views/DdnsView.vue'
import CertificatesView from '../views/CertificatesView.vue'
import LogsView from '../views/LogsView.vue'
import SettingsView from '../views/SettingsView.vue'
import FrpView from '../views/FrpView.vue'
import CloudflareView from '../views/CloudflareView.vue'
import CloudflareTunnelConfigView from '../views/CloudflareTunnelConfigView.vue'
import AgentView from '../views/AgentView.vue'
import ProxyRemoteView from '../views/ProxyRemoteView.vue'
import StatusPageView from '../views/StatusPageView.vue'
import AboutView from '../views/AboutView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', redirect: '/login' },
    { path: '/login', name: 'login', component: LoginView, meta: { public: true } },
    // 公开状态页：挂到公网给别人看的，不经过登录守卫
    { path: '/status', name: 'status', component: StatusPageView, meta: { public: true } },
    {
      path: '/',
      component: AppLayout,
      meta: { requiresAuth: true },
      children: [
        { path: '', redirect: '/dashboard' },
        {
          path: 'dashboard',
          name: 'dashboard',
          component: DashboardView,
          meta: { title: '仪表盘', description: '查看 Havline 运行状态与关键指标' },
        },
        {
          path: 'ddns',
          name: 'ddns',
          component: DdnsView,
          meta: { title: 'DDNS', description: '自动同步公网 IP 到 DNS 解析' },
        },
        {
          path: 'certificates',
          name: 'certificates',
          component: CertificatesView,
          meta: { title: '证书管理', description: '通过 ACME 自动申请和续签证书' },
        },
        {
          path: 'proxies',
          name: 'proxies',
          component: ProxyView,
          meta: { title: '反向代理', description: '管理通过域名访问的 NAS 服务' },
        },
        {
          path: 'frp',
          name: 'frp',
          component: FrpView,
          meta: { title: '内网穿透', description: '通过 FRP 将公网流量转发到 Havline Nginx' },
        },
        {
          path: 'frp-agent',
          name: 'frp-agent',
          component: AgentView,
          meta: { title: '公网 Agent', description: '管理部署在公网 VPS 上的 havline-agent 与 frps' },
        },
        {
          path: 'cloudflare',
          name: 'cloudflare',
          component: CloudflareView,
          meta: { title: 'Cloudflare 隧道', description: '通过 cloudflared 把内网 Web 服务发布到 Cloudflare 边缘' },
        },
        {
          path: 'cloudflare/:id/config',
          name: 'cloudflare-tunnel-config',
          component: CloudflareTunnelConfigView,
          meta: { title: 'Cloudflare 隧道配置', description: '配置隧道、查看副本指标与已发布路由' },
        },
        {
          path: 'public-proxy',
          name: 'public-proxy',
          component: ProxyRemoteView,
          meta: { title: '公网反代', description: '管理 VPS 上 Nginx 反代的部署/移除与安全选项' },
        },
        {
          path: 'logs',
          name: 'logs',
          component: LogsView,
          meta: { title: '日志中心', description: '查看系统、访问与 Nginx 错误日志' },
        },
        {
          path: 'settings',
          name: 'settings',
          component: SettingsView,
          meta: { title: '系统设置', description: '配置系统参数与运行策略' },
        },
        {
          path: 'about',
          name: 'about',
          component: AboutView,
          meta: { title: '关于项目', description: '查看 Havline 版本、源开源项目和技术信息' },
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  // 公开页（状态页）不登录也能打开，也不必为它去问一次登录状态
  if (to.meta.public && to.name !== 'login') return true
  const status = await api.authStatus()
  if (to.meta.requiresAuth && !status.authenticated) return { name: 'login' }
  if (to.name === 'login' && status.authenticated) return { name: 'dashboard' }
  return true
})

export default router
