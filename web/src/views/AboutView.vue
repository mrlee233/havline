<template>
  <PageHeader title="关于项目" description="查看版本、项目仓库、发布记录与开源信息">
    <template #actions>
      <n-button :loading="refreshing" @click="refreshVersion">
        <template #icon><n-icon :component="RefreshOutline" /></template>
        刷新版本信息
      </n-button>
    </template>
  </PageHeader>

  <section class="about-hero">
    <div class="about-hero__main">
      <img class="about-hero__logo" :src="brandLogo" alt="Havline" />
      <div class="about-hero__copy">
        <p class="about-kicker">ABOUT HAVLINE</p>
        <h2>让每一台 NAS，都能安全地连接世界。</h2>
        <p>
          Havline 面向家庭 NAS 与自托管环境，集中管理服务的公网访问入口。
          从域名解析、HTTPS 证书到反向代理、FRP 和 Cloudflare 隧道，
          将分散的配置与日常运维集中到一处，支持容器化与本地化部署。
        </p>
        <div class="about-actions">
          <a :href="GITHUB_REPO_URL" target="_blank" rel="noopener noreferrer">
            <n-icon :component="LogoGithub" />
            查看项目源码
          </a>
          <a :href="GITHUB_RELEASES_URL" target="_blank" rel="noopener noreferrer">
            <n-icon :component="CloudDownloadOutline" />
            发布版本
          </a>
          <a :href="GPL_URL" target="_blank" rel="noopener noreferrer">
            <n-icon :component="DocumentTextOutline" />
            GPL-3.0
          </a>
        </div>
      </div>
    </div>

    <div class="about-hero__meta">
      <div class="about-meta-item">
        <span>当前版本</span>
        <strong class="mono">v{{ version }}</strong>
      </div>
      <div class="about-meta-item">
        <span>项目仓库</span>
        <strong>{{ GITHUB_REPO }}</strong>
      </div>
      <div class="about-meta-item">
        <span>开源协议</span>
        <strong>GPL-3.0</strong>
      </div>
      <div class="about-meta-item">
        <span>最近检查</span>
        <strong>{{ checkedAt || '刚刚' }}</strong>
      </div>
    </div>
  </section>

  <div class="about-sections">
  <HavlineCard title="项目定位" subtitle="Havline 解决什么问题">
    <div class="about-lead">
      <p>
        NAS 的价值不只在存储，更在于把文件、媒体、自动化工具和家庭服务安全地带到需要它们的地方。
        Havline 不试图替代专业网关或企业级运维平台，而是把 NAS 用户最常用、最容易出错的公网访问能力
        做成一个可理解、可维护、可回滚的 Web 控制台。
      </p>
    </div>
    <div class="about-points">
      <article class="about-point">
        <h3>减少手工配置</h3>
        <p>自动生成 Nginx 配置、管理证书续期，并统一处理 DDNS 与访问入口。</p>
      </article>
      <article class="about-point">
        <h3>覆盖多种出口</h3>
        <p>支持本机反向代理、FRP 内网穿透、公网 VPS Agent 和 Cloudflare Tunnel。</p>
      </article>
      <article class="about-point">
        <h3>保留可控性</h3>
        <p>本地配置与运行数据由自己管理，支持配置回滚和备份恢复；DNS、证书与隧道按所选服务接入。</p>
      </article>
    </div>
  </HavlineCard>

  <HavlineCard title="来源与致谢" subtitle="站在巨人的肩膀上继续前行">
    <div class="about-origin">
      <blockquote>开源不是重新发明轮子，而是站在巨人的肩膀上，把一件事继续做得更好。</blockquote>
      <p>
        Havline 是基于开源项目
        <a :href="UPSTREAM_REPO_URL" target="_blank" rel="noopener noreferrer">{{ UPSTREAM_REPO }}</a>
        <strong>1.0.2 版本</strong>进行二次开发的项目。原始项目已经完成了反向代理、DDNS、证书、
        内网穿透等核心能力，为 Havline 提供了坚实的工程基础。
      </p>
      <p>
        Havline 在此基础上持续完善多服务端管理、公网 Agent、Cloudflare 隧道和部署兼容性。
        当前源码与版本发布由 {{ GITHUB_REPO }} 维护，保留上游来源与贡献者致谢，并继续遵循
        <a :href="GPL_URL" target="_blank" rel="noopener noreferrer">GPL-3.0</a>
        开源协议。
      </p>
    </div>
  </HavlineCard>

  <HavlineCard title="核心能力" subtitle="从入口到证书，从内网到公网">
    <div class="about-features">
      <article v-for="feature in features" :key="feature.title" class="about-feature">
        <span class="about-feature__icon">
          <n-icon :component="feature.icon" />
        </span>
        <div>
          <h3>{{ feature.title }}</h3>
          <p>{{ feature.description }}</p>
        </div>
      </article>
    </div>
  </HavlineCard>

  <HavlineCard title="适用场景" subtitle="根据网络条件选择服务入口">
    <div class="about-points">
      <article class="about-point">
        <h3>有公网访问条件的 NAS</h3>
        <p>通过 DDNS 跟随公网 IP，使用 Nginx 反向代理和 HTTPS 证书发布服务。实际访问仍取决于路由器转发、防火墙、运营商限制与 IPv4 / IPv6 连通性。</p>
      </article>
      <article class="about-point">
        <h3>通过公网服务器接入</h3>
        <p>使用 FRP 连接自有 VPS，并通过公网 Agent 管理服务器上的 Nginx、frps 和服务路由。VPS、域名与服务器权限由使用者自行准备和管理。</p>
      </article>
      <article class="about-point">
        <h3>不开放本机入站端口</h3>
        <p>通过 Cloudflare Tunnel 发布内网 Web 服务，管理隧道、域名路由与 DNS。此方式依赖 Cloudflare 账户、授权和网络连接，适用范围受其服务规则约束。</p>
      </article>
    </div>
  </HavlineCard>

  <HavlineCard title="部署与运行" subtitle="一份业务源码，多种交付方式">
    <div class="about-detail-grid">
      <article class="about-detail-item">
        <span class="about-detail-item__eyebrow">Docker</span>
        <h3>Compose 管理容器与数据</h3>
        <p>适合通用 Linux、NAS 和已有 Docker 环境。数据通过命名卷或绑定目录持久化，配有 updater 的 Compose 部署支持受控升级。</p>
      </article>
      <article class="about-detail-item">
        <span class="about-detail-item__eyebrow">自托管部署</span>
        <h3>按环境选择运行方式</h3>
        <p>可根据设备与运维习惯选择容器化或本地化部署，数据结构保持一致，便于迁移、备份和恢复。</p>
      </article>
      <article class="about-detail-item">
        <span class="about-detail-item__eyebrow">按需组件</span>
        <h3>首次使用时下载</h3>
        <p>FRP 客户端和 cloudflared 不预置在基础运行环境中，启用对应功能时需要联网下载并保存到数据目录。</p>
      </article>
      <article class="about-detail-item">
        <span class="about-detail-item__eyebrow">外部依赖</span>
        <h3>域名、证书与服务器由用户准备</h3>
        <p>DNS、ACME 证书、公网 VPS 和 Cloudflare Tunnel 需要相应账户、凭据或服务器，应用不会自动获得公网 IP 或第三方授权。</p>
      </article>
    </div>
  </HavlineCard>

  <HavlineCard title="数据与安全" subtitle="本地管理不等于无需防护">
    <div class="about-points about-points--security">
      <article class="about-point">
        <h3>完整备份，而非只备份数据库</h3>
        <p>数据库、会话与凭据加密密钥、证书私钥和运行配置共同组成实例数据。迁移或升级前应备份完整数据目录；使用环境变量提供密钥时，还需安全保留原值。</p>
      </article>
      <article class="about-point">
        <h3>管理入口与服务入口分离</h3>
        <p>对外发布业务服务时，不应顺带开放管理后台、Docker Socket 或 Agent 管理端口。按需配置认证、HTTPS、防火墙和访问限制，并定期检查授权范围。</p>
      </article>
      <article class="about-point">
        <h3>保护凭据与备份归档</h3>
        <p>DNS Token、服务器凭据和证书私钥属于敏感数据。凭据加密不能替代文件权限与备份保护；已有数据后不要随意更换加密主密钥，以免失去解密能力。</p>
      </article>
    </div>
  </HavlineCard>

  <HavlineCard title="项目维护与反馈" subtitle="以当前源码和发布记录为准">
    <div class="about-detail-grid about-detail-grid--feedback">
      <article class="about-detail-item">
        <span class="about-detail-item__eyebrow">源码与版本</span>
        <h3>跟随项目仓库发布</h3>
        <p>当前源码由 <a :href="GITHUB_REPO_URL" target="_blank" rel="noopener noreferrer">{{ GITHUB_REPO }}</a> 维护，已发布版本与更新说明见 <a :href="GITHUB_RELEASES_URL" target="_blank" rel="noopener noreferrer">发布记录</a>。生产环境请固定版本并保留备份。</p>
      </article>
      <article class="about-detail-item">
        <span class="about-detail-item__eyebrow">问题反馈</span>
        <h3>提供可复现的信息</h3>
        <p>通过 <a :href="`${GITHUB_REPO_URL}/issues`" target="_blank" rel="noopener noreferrer">GitHub Issues</a> 反馈时，请附版本、部署方式、设备架构、复现步骤和脱敏日志。</p>
      </article>
      <article class="about-detail-item about-detail-item--wide">
        <span class="about-detail-item__eyebrow">开源协作</span>
        <h3>保留来源，持续改进</h3>
        <p>欢迎提交问题复现、兼容性反馈和代码改进。Havline 保留上游贡献者的版权与来源说明，持续按 GPL-3.0 发布；请勿上传密码、Token、私钥、完整数据库或包含敏感配置的备份归档。</p>
      </article>
    </div>
  </HavlineCard>

  <HavlineCard title="技术栈" subtitle="保持简单、透明、易部署">
    <div class="about-tech">
      <section v-for="group in techGroups" :key="group.title" class="about-tech__group">
        <h3>{{ group.title }}</h3>
        <div class="about-tags">
          <span v-for="item in group.items" :key="item">{{ item }}</span>
        </div>
      </section>
    </div>
  </HavlineCard>

  <HavlineCard title="开源与许可" subtitle="尊重版权，也尊重每一位使用者">
    <div class="about-license">
      <div class="about-license__item">
        <n-icon :component="DocumentTextOutline" />
        <div>
          <h3>GPL-3.0</h3>
          <p>项目继续采用 GNU General Public License v3.0 开源。</p>
        </div>
      </div>
      <div class="about-license__item">
        <n-icon :component="LogoGithub" />
        <div>
          <h3>上游来源</h3>
          <p>基于 {{ UPSTREAM_REPO }} 继续开发，保留原作者版权、来源说明与许可证。</p>
        </div>
      </div>
      <div class="about-license__item">
        <n-icon :component="ShieldCheckmarkOutline" />
        <div>
          <h3>无担保声明</h3>
          <p>软件按“原样”提供，请在部署前做好备份、权限和网络访问控制。</p>
        </div>
      </div>
    </div>
  </HavlineCard>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton, NIcon, useMessage } from 'naive-ui'
import {
  CloudDownloadOutline,
  CloudOutline,
  DocumentTextOutline,
  GitNetworkOutline,
  GridOutline,
  HardwareChipOutline,
  LockClosedOutline,
  LogoGithub,
  RefreshOutline,
  ShieldCheckmarkOutline,
  SwapHorizontalOutline,
} from '@vicons/ionicons5'
import { api } from '../api/client'
import brandLogo from '../assets/brand/havline-logo.svg'
import HavlineCard from '../components/HavlineCard.vue'
import PageHeader from '../components/PageHeader.vue'
import {
  GITHUB_RELEASES_URL,
  GITHUB_REPO,
  GITHUB_REPO_URL,
  UPSTREAM_REPO,
  UPSTREAM_REPO_URL,
} from '../constants/app'
import { formatDate } from '../utils/format'

const GPL_URL = 'https://www.gnu.org/licenses/gpl-3.0.html'
const message = useMessage()
const version = ref(__APP_VERSION__)
const checkedAt = ref('')
const refreshing = ref(false)

const features = [
  {
    title: '反向代理',
    description: '通过 Web 界面管理 Nginx 域名与端口规则，自动生成配置并热重载。',
    icon: SwapHorizontalOutline,
  },
  {
    title: 'DDNS',
    description: '对接 Cloudflare、DNSPod、阿里云、腾讯云和火山引擎，自动同步公网 IP。',
    icon: CloudOutline,
  },
  {
    title: 'HTTPS 证书',
    description: '使用 ACME DNS-01 自动申请与续期证书，支持通配符和手动导入。',
    icon: LockClosedOutline,
  },
  {
    title: '内网穿透',
    description: '内置 FRP 多服务端管理，在没有公网 IP 时把服务安全暴露到 VPS。',
    icon: GitNetworkOutline,
  },
  {
    title: '公网 Agent',
    description: '通过 SSH 安装和维护 VPS 上的 havline-agent，远程编排 frps 与 Nginx。',
    icon: HardwareChipOutline,
  },
  {
    title: 'Cloudflare Tunnel',
    description: '管理 cloudflared 隧道、路由与 DNS，适合不愿意开放公网端口的场景。',
    icon: CloudOutline,
  },
  {
    title: '日志与监控',
    description: '查看系统、访问和错误日志，观察流量趋势、服务状态与运行健康度。',
    icon: GridOutline,
  },
  {
    title: '安全与备份',
    description: '提供访问控制、凭据加密、配置备份和恢复，降低误操作与数据丢失风险。',
    icon: ShieldCheckmarkOutline,
  },
]

const techGroups = [
  {
    title: '后端',
    items: ['Go 1.23', 'SQLite', 'Nginx', 'FRP', 'ACME / lego'],
  },
  {
    title: '前端',
    items: ['Vue 3', 'TypeScript', 'Vite', 'Naive UI', 'ECharts'],
  },
  {
    title: '部署',
    items: ['Docker', 'Docker Compose', 'Linux systemd', 'amd64 / arm64'],
  },
]

async function refreshVersion() {
  refreshing.value = true
  try {
    const result = await api.getVersion()
    version.value = result.version || version.value
    checkedAt.value = formatDate(new Date().toISOString())
  } catch (error) {
    message.error(error instanceof Error ? error.message : '读取版本失败')
  } finally {
    refreshing.value = false
  }
}

onMounted(() => {
  void refreshVersion()
})
</script>

<style scoped>
.about-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: var(--havline-space-6);
  align-items: center;
  margin-bottom: var(--havline-space-5);
  padding: var(--havline-space-6);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius);
  background:
    linear-gradient(135deg, var(--havline-brand-soft), transparent 58%),
    var(--havline-surface);
}

.about-hero__main {
  display: flex;
  align-items: flex-start;
  gap: var(--havline-space-5);
  min-width: 0;
}

.about-hero__logo {
  width: 76px;
  height: 76px;
  flex: 0 0 auto;
}

.about-hero__copy {
  min-width: 0;
}

.about-kicker {
  margin: 0 0 8px;
  color: var(--havline-brand-text);
  font-family: var(--havline-mono);
  font-size: 11px;
  letter-spacing: 0.14em;
}

.about-hero h2 {
  margin: 0 0 12px;
  color: var(--havline-text);
  font-size: clamp(24px, 3vw, 34px);
  line-height: 1.22;
}

.about-hero__copy > p:not(.about-kicker) {
  max-width: 720px;
  margin: 0;
  color: var(--havline-text-secondary);
  font-size: 14px;
  line-height: 1.8;
}

.about-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--havline-space-3);
  margin-top: var(--havline-space-5);
}

.about-actions a {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  color: var(--havline-brand-text);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-surface);
  text-decoration: none;
}

.about-hero__meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: color-mix(in srgb, var(--havline-surface) 92%, transparent);
  overflow: hidden;
}

.about-meta-item {
  min-width: 0;
  padding: 16px;
  border-right: 1px solid var(--havline-border);
  border-bottom: 1px solid var(--havline-border);
}

.about-meta-item:nth-child(2n) {
  border-right: 0;
}

.about-meta-item:nth-last-child(-n + 2) {
  border-bottom: 0;
}

.about-meta-item span,
.about-meta-item strong {
  display: block;
}

.about-meta-item span {
  margin-bottom: 6px;
  color: var(--havline-text-muted);
  font-size: 12px;
}

.about-meta-item strong {
  overflow: hidden;
  color: var(--havline-text);
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.about-sections {
  display: grid;
  gap: var(--havline-space-5);
  min-width: 0;
}

.about-sections > :deep(.havline-card) {
  min-width: 0;
}

.about-lead {
  max-width: 860px;
  color: var(--havline-text-secondary);
  font-size: 14px;
  line-height: 1.9;
}

.about-lead p {
  margin: 0;
}

.about-points {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--havline-space-4);
  margin-top: var(--havline-space-5);
}

.about-points--security {
  margin-top: 0;
}

.about-detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--havline-space-4);
}

.about-detail-item {
  min-width: 0;
  padding: var(--havline-space-4);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
}

.about-detail-item--wide {
  grid-column: 1 / -1;
}

.about-detail-item__eyebrow {
  display: block;
  margin-bottom: var(--havline-space-2);
  color: var(--havline-brand-text);
  font-family: var(--havline-mono);
  font-size: 11px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.about-detail-item h3 {
  margin: 0 0 var(--havline-space-2);
  color: var(--havline-text);
  font-size: 15px;
}

.about-detail-item p {
  margin: 0;
  color: var(--havline-text-secondary);
  font-size: 13px;
  line-height: 1.75;
}

.about-detail-item a {
  color: var(--havline-brand-text);
  text-decoration: none;
}

.about-point {
  padding-left: var(--havline-space-4);
  border-left: 2px solid var(--havline-brand);
}

.about-point h3,
.about-feature h3,
.about-tech__group h3,
.about-license__item h3 {
  margin: 0 0 6px;
  color: var(--havline-text);
  font-size: 15px;
}

.about-point p,
.about-feature p,
.about-license__item p {
  margin: 0;
  color: var(--havline-text-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.about-origin {
  display: grid;
  gap: var(--havline-space-4);
  color: var(--havline-text-secondary);
  font-size: 14px;
  line-height: 1.85;
}

.about-origin blockquote {
  margin: 0;
  padding: var(--havline-space-4) var(--havline-space-5);
  color: var(--havline-brand-text);
  border-left: 3px solid var(--havline-brand);
  background: var(--havline-brand-soft);
  border-radius: 0 var(--havline-radius-sm) var(--havline-radius-sm) 0;
  font-size: 16px;
  font-weight: 600;
}

.about-origin p {
  margin: 0;
}

.about-origin a {
  color: var(--havline-brand-text);
  text-decoration: none;
}

.about-features {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--havline-space-4);
}

.about-feature {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr);
  gap: var(--havline-space-3);
  min-width: 0;
  padding: var(--havline-space-4);
  border: 1px solid var(--havline-border);
  border-radius: var(--havline-radius-sm);
  background: var(--havline-bg);
}

.about-feature__icon {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  color: var(--havline-brand-text);
  border-radius: 10px;
  background: var(--havline-brand-soft);
  font-size: 19px;
}

.about-tech {
  display: grid;
  gap: var(--havline-space-5);
}

.about-tech__group {
  display: grid;
  grid-template-columns: 100px minmax(0, 1fr);
  gap: var(--havline-space-4);
  align-items: start;
}

.about-tech__group h3 {
  margin: 0;
  padding-top: 5px;
}

.about-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.about-tags span {
  padding: 5px 10px;
  color: var(--havline-text-secondary);
  border: 1px solid var(--havline-border);
  border-radius: 999px;
  background: var(--havline-bg);
  font-size: 12px;
}

.about-license {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--havline-space-4);
}

.about-license__item {
  display: grid;
  grid-template-columns: 22px minmax(0, 1fr);
  gap: var(--havline-space-3);
  min-width: 0;
}

.about-license__item > .n-icon {
  margin-top: 2px;
  color: var(--havline-brand-text);
  font-size: 19px;
}

@media (max-width: 1100px) {
  .about-hero,
  .about-features {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .about-hero__main {
    grid-column: 1 / -1;
  }

  .about-hero__meta {
    grid-column: 1 / -1;
  }
}

@media (max-width: 760px) {
  .about-hero {
    grid-template-columns: 1fr;
    padding: var(--havline-space-5);
  }

  .about-hero__main {
    display: grid;
  }

  .about-hero__logo {
    width: 64px;
    height: 64px;
  }

  .about-points,
  .about-features,
  .about-license,
  .about-detail-grid {
    grid-template-columns: 1fr;
  }

  .about-detail-item--wide {
    grid-column: auto;
  }

  .about-tech__group {
    grid-template-columns: 1fr;
    gap: var(--havline-space-2);
  }

  .about-meta-item {
    border-right: 0;
  }

  .about-meta-item:nth-child(odd) {
    border-right: 1px solid var(--havline-border);
  }
}
</style>
