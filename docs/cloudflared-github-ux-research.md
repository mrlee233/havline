# Cloudflare 隧道同类项目调研与 Fonu 体验优化建议

> 日期：2026-09-30
> 调研范围：GitHub 上 Cloudflare Tunnel / cloudflared 管理面板、反向代理管理器、自托管访问平台、状态监测与流量检查器。
> 目标：不是照搬功能，而是提取适合 Fonu 的自动化方式、页面结构和长期运维体验。

---

## 一、调研结论摘要

同类项目大致分成四类：

1. **cloudflared 单容器 Web UI**
   - 代表：Cloudflared-web
   - 重点：快速设置 Token、启停隧道、查看运行状态。
2. **Cloudflare 路由与 Access 自动化平台**
   - 代表：DockFlare
   - 重点：Docker 标签、期望状态调和、DNS、ingress、Access 策略统一管理。
3. **Homelab 反向代理 / 私有访问平台**
   - 代表：Nginx Proxy Manager、Pangolin、Cosmos Server
   - 重点：可视化路由、证书、访问控制、资源入口页、审计。
4. **监控、状态页与流量调试**
   - 代表：Uptime Kuma、Tunnel Inspector
   - 重点：可用率、状态页、事故记录、请求级观测、重放和抓包体验。

对 Fonu 最重要的启发不是“再加多少按钮”，而是：

- 用期望状态和自动调和代替手工点同步。
- 让规则、隧道、DNS、证书、监控在同一套模型中管理。
- 配置页要面向“服务发布”，而不是面向 cloudflared 参数。
- 把诊断、修复、日志和状态页做成闭环。
- 高级能力可以保留，但默认路径必须非常短。

---

## 二、代表性项目

以下 Star 数据为 2026-09-30 GitHub 查询结果，仅作为热度参考。

| 项目 | 类型 | 技术栈 | Star | 最值得借鉴 |
| --- | --- | --- | ---: | --- |
| [cloudflare/cloudflared](https://github.com/cloudflare/cloudflared) | 官方客户端 | Go | 15970 | 官方能力边界、TryCloudflare、版本支持策略 |
| [DockFlare](https://github.com/ChrispyBacon-dev/DockFlare) | Tunnel + Access 自动化 | Python | 2471 | Docker 标签、期望状态调和、多主 Agent、备份恢复 |
| [Nginx Proxy Manager](https://github.com/NginxProxyManager/nginx-proxy-manager) | 反向代理面板 | JavaScript | 高热度 | Host / Streams / Access Lists / SSL / 用户与审计 |
| [cloudflare-tunnel-ingress-controller](https://github.com/STRRL/cloudflare-tunnel-ingress-controller) | Tunnel Ingress Controller | Go | 1197 | 声明式路由、Ingress 到 Tunnel/DNS 自动生成 |
| [cftunnel](https://github.com/qingchencloud/cftunnel) | 全协议穿透 CLI | Go | 345 | 快速隧道、Cloud + FRP 双引擎、模板、分享、自动更新 |
| [Cloudflared-web](https://github.com/WisdomSky/Cloudflared-web) | 极简 Web UI | Vue | 465 | 单容器安装、Token 管理、启停、低学习成本 |
| [Tunnel Cloudflared Desktop](https://github.com/maskodingku/Tunnel-Cloudflared-Desktop) | 桌面管理器 | Tauri + Rust | 55 | 多账号、`cert.pem`、双向同步、实时指标、Quick Tunnel |
| [Tunnel Inspector](https://github.com/BlockbusterAndy/cloudflare-tunnel-inspector) | 流量检查器 | TypeScript | 6 | 请求/响应捕获、SSE 实时流、搜索筛选、重放、敏感头脱敏 |
| [Pangolin](https://github.com/fosrl/pangolin) | 自托管访问平台 | Go / Web | 高热度 | 资源入口页、统一身份、按资源授权、浏览器访问 SSH/RDP/VNC |
| [Uptime Kuma](https://github.com/louislam/uptime-kuma) | 监控与状态页 | Vue / Node | 高热度 | 多状态页、事故记录、证书信息、通知渠道、2FA |

---

## 三、项目逐个分析

### 3.1 Cloudflared-web

项目定位：

- 一个 Docker 镜像同时打包 cloudflared 和简单 Web UI。
- 主要面向“只想管理 Token 和启停隧道”的用户。

核心体验：

- Docker 启动后直接打开固定端口网页。
- 填写 Token，一键启停。
- 支持 BASIC AUTH。
- 支持环境变量配置协议、边缘 IP、metrics、重试等。
- 数据持久化到 `/config`。

对 Fonu 的启示：

- 初次配置应该更短：Account ID、API Token、Zone、Tunnel 四步内完成。
- 所有高级网络参数应该折叠到“高级选项”，默认值直接可用。
- 当前 Fonu 已覆盖它的主要能力，并且多了 API 托管、DNS、规则同步和监控。

不建议照搬：

- 只有 Token 和启停，模型太薄。
- 不适合 Fonu 的“服务发布规则”产品定位。

### 3.2 DockFlare

项目定位：

- 面向多变自托管环境的 Cloudflare Tunnel / DNS / Access 控制平面。
- 强调“定义一次意图，然后持续调和”。

核心能力：

- Docker 标签自动发现服务。
- Web UI 手工规则。
- 远程 Agent 多主机管理。
- Tunnel ingress、DNS、Access 统一调和。
- Access group 和可复用策略。
- 加密配置、运行时状态和数据的备份恢复。
- 多语言 UI。

对 Fonu 的启示：

- 当前 Fonu 已经有“反向代理规则派生 Cloudflare ingress”，方向正确。
- 下一步应补“期望状态”和“调和结果”：
  - 本地期望配置。
  - Cloudflare 实际配置。
  - 最后同步时间。
  - 漂移原因。
  - 一键修复。
- Docker 标签自动发布非常适合 NAS。
- 多主机 Agent 适合进阶用户，但不应优先于单机体验。
- Access 自动化风险较高，Fonu 暂时保持只读更稳。

### 3.3 Tunnel Ingress Controller

项目定位：

- 把 Kubernetes Ingress 转成 Cloudflare Tunnel 和 DNS 配置。

核心思想：

- 用户只维护 Kubernetes 原生声明。
- Controller 持续把声明同步到 Cloudflare。
- 不要求用户手工编辑 cloudflared ingress。

对 Fonu 的启示：

- Fonu 的“反向代理规则就是服务入口声明”是同一思路的非 K8s 版本。
- 无需引入 K8s，但应借鉴：
  - 声明与运行状态分离。
  - 调和结果可见。
  - 失败原因可追踪。
  - 不应让用户理解底层 ingress 文件。

### 3.4 Tunnel Cloudflared Desktop

项目定位：

- 面向桌面用户的 Cloudflare Tunnel 管理器。

核心能力：

- Quick Tunnel。
- 多账号 `cert.pem`。
- 本地与 Cloudflare Dashboard 双向同步。
- 实时指标。
- 实时日志。
- 自动下载 cloudflared。

对 Fonu 的启示：

- Quick Tunnel 适合“临时演示”，不建议作为正式发布方式。
- 双向同步是 Fonu 当前明显缺口：
  - 当前支持检测漂移并“本地覆盖云端”。
  - 缺少“云端覆盖本地”。
- 多账号模式对 Fonu 属于进阶能力，可放到远期。
- 实时日志和指标已具备，继续加强即可。

### 3.5 cftunnel

项目定位：

- 全协议内网穿透 CLI。
- Cloud 模式使用 Cloudflare Tunnel。
- Relay 模式使用自建 frps。

核心体验：

- `quick <port>` 生成临时公网地址。
- 可加 Basic Auth。
- 自动 DNS。
- 预设模板。
- 一键分享二维码、地址、Telegram。
- 自动更新。

对 Fonu 的启示：

- “快速隧道”可以作为临时验证入口，但必须标明不保证 SLA。
- 场景模板非常值得借鉴：
  - Webhook
  - Home Assistant
  - 面板
  - 相册
  - Jellyfin 管理页
- “复制访问地址 / 二维码分享”适合手机用户。
- Cloud + FRP 双引擎的概念与 Fonu 的出口抽象一致。

### 3.6 Nginx Proxy Manager

项目定位：

- 最经典的自托管反向代理 Web 面板。

核心体验：

- Hosts、Streams、Access Lists、SSL Certificates、Users 分栏。
- 每条代理规则有独立状态、SSL、访问控制、高级配置。
- 证书申请和续签直接在规则旁完成。
- 管理界面简单，新手可按表单完成部署。

对 Fonu 的启示：

- 反向代理规则的“编辑页”应成为服务发布中心。
- Cloudflare 出口应该是一个选项，而不是另一套孤立页面。
- SSL、Access、健康检查、日志应围绕同一条规则组织。
- Fonu 当前已有这一方向，但页面还可以更像“资源详情页”。

### 3.7 Pangolin

项目定位：

- 开源自托管 SASE / Zero Trust 访问平台。

核心体验：

- 资源入口页。
- 用户、角色、资源级授权。
- 浏览器直接访问 SSH、RDP、VNC。
- 对每个资源做审计和会话记录。
- 多 connector 和站点冗余。

对 Fonu 的启示：

- 普通 NAS 用户更需要“访问我的服务”，不是“管理网络隧道”。
- 可以借鉴资源入口页：
  - 按应用、域名、出口分组。
  - 一键打开服务。
  - 显示维护状态和最近访问。
- 不建议 Fonu 当前做企业 RBAC、SSO、PAM；成本过高。

### 3.8 Uptime Kuma

项目定位：

- 自托管监控和状态页。

核心体验：

- 多状态页。
- 不同域名绑定不同状态页。
- Ping 图表、证书信息。
- 通知渠道丰富。
- 2FA。

对 Fonu 的启示：

- 当前公开状态页已有域名和可用率。
- 下一步最值得补：
  - 事故记录。
  - 维护窗口。
  - 最近 24h / 7d / 30d 状态条。
  - 多个状态页。
  - 自定义标题、Logo、说明。
- 这些比继续增加隧道参数更直接影响用户体验。

### 3.9 Tunnel Inspector

项目定位：

- Cloudflare Tunnel 的本地 HTTP 流量检查器。

核心体验：

- 实时请求列表。
- 方法和状态码筛选。
- 请求/响应头、Body、耗时、TTFB。
- 响应流式透传。
- 敏感头脱敏。
- 请求重放和 Copy as cURL。
- SSE 实时推送。

对 Fonu 的启示：

- 适合做成“Cloudflare 出口调试模式”，不要默认常驻。
- 可提供：
  - 最近请求。
  - 上游状态码。
  - 回源耗时。
  - 错误请求。
  - 一键复制 curl。
- 需要严格限制：
  - 仅本机访问。
  - 默认脱敏。
  - 默认关闭。
  - 明确数据不离开本机。

---

## 四、跨项目共同的最佳体验

### 4.1 安装向导与预检

优秀项目不会让用户直接面对复杂配置。

建议 Fonu 增加：

1. 检查 cloudflared 是否安装。
2. 检查 Account ID。
3. 检查 API Token 权限。
4. 列出可用 Zone。
5. 检查目标根域名是否在 Zone 内。
6. 检查 DNS 是否已被其他目标占用。
7. 检查 7844 / 443 连通性。
8. 生成一条测试隧道并验证访问。

### 4.2 期望状态与调和中心

用户看到的不应只是“同步按钮”，而是：

```text
期望状态：已发布
Cloudflare 实际：缺失
最后同步：2 分钟前
原因：DNS CNAME 目标不一致
操作：修复 / 查看差异
```

这比单纯提示“保存失败”更有价值。

### 4.3 可视化拓扑

很多用户不理解 Nginx、frps、cloudflared、DNS、Access 的关系。

建议：

```text
域名 → 出口 → 隧道 → 内网服务
```

例如：

```text
nas.example.com
  → Cloudflare
  → tunnel: home-nas
  → http://192.168.1.10:8001
```

拓扑图可以直接显示：

- 域名。
- 出口。
- DNS 状态。
- 隧道状态。
- 回源状态。
- 证书来源。

### 4.4 路由表体验

当前 Fonu 已有路由表。建议继续补：

- 拖拽排序。
- 启用/停用。
- 复制路由。
- 正则路径提示。
- “测试此路由”按钮。
- 最后匹配结果。
- 路由来源标签：
  - 手工。
  - 反向代理规则。
  - 导入。
- 修改历史和回滚。

### 4.5 状态页与事故

建议把公开状态页从“当前状态列表”升级为：

- 当前状态。
- 最近事故。
- 事故持续时间。
- 维护窗口。
- 最近 24h / 7d / 30d 状态条。
- 服务分组。

### 4.6 流量检查模式

建议作为高级诊断功能：

- 默认关闭。
- 开启后显示最近请求。
- 自动过滤静态资源。
- 显示 4xx/5xx。
- 回源错误和边缘错误分开。
- 可复制为 curl。
- 可查看请求/响应头。
- 默认隐藏 Authorization、Cookie、Token。

### 4.7 快速隧道

适合做“临时演示”：

- 只暴露单个本地端口。
- 生成 `trycloudflare.com` 地址。
- 明确标注：
  - 地址会变化。
  - 不保证 SLA。
  - 不用于正式服务。
- 可复制地址或生成二维码。

### 4.8 一键分享与移动体验

对 NAS / 手机用户很有价值：

- 复制访问链接。
- 生成二维码。
- 分享到 Telegram / 微信。
- 移动端查看状态页。
- PWA 支持可作为远期项。

### 4.9 多账号与 `cert.pem`

桌面管理器和 DockFlare 都把多账号作为进阶能力。

建议 Fonu：

- 当前继续以 Account ID + API Token 为默认。
- 远期可支持多账号。
- `cert.pem` 不应替代 API Token，只作为高级兼容模式。
- 多账号必须有明确的账号隔离，避免路由误绑。

### 4.10 审计与回滚

自托管平台越复杂，回滚越重要。

建议：

- 记录规则和出口变更前后值。
- 记录操作主体。
- 记录 Cloudflare 同步结果。
- 支持手动回滚配置。
- 敏感字段只记录“已修改”，不记录明文。

---

## 五、Fonu 不该照搬的功能

| 功能 | 结论 | 原因 |
| --- | --- | --- |
| Kubernetes Controller | 不做 | 不是 NAS 用户场景 |
| 企业 SSO / PAM / RBAC 平台 | 暂不做 | 成本高，偏离 Fonu 定位 |
| Cloudflare 邮件套件 | 不做 | 与访问管理无关 |
| 多语言 13 种起步 | 暂不做 | 先保持中文产品体验 |
| 多主机 Agent 优先 | 后置 | 先做好单机 NAS |
| 自动配置 Access 策略 | 后置 | 权限风险高 |
| 默认常驻流量抓包 | 不做 | 隐私和性能风险 |
| 用户自定义 cloudflared 镜像管理 | 不做 | 增加维护复杂度 |

---

## 六、Fonu 建议路线

> 实施状态：截至 2026-10-01，本节的 P0 已完成第一轮落地，详见 `cloudflared-current-features.md`。

### P0：最值得先做

1. **Cloudflare 接入向导**
   - Account、Token、Zone、Tunnel 一步检查。
2. **调和中心**
   - 展示期望状态、云端状态、漂移原因、最后同步时间。
3. **路由表增强**
   - 排序、启停、复制、测试、来源标签。
4. **一键修复**
   - DNS、ingress、进程、二进制四类修复动作。
5. **状态页事故记录**
   - 事故时间线、持续时长、维护窗口。

### P1：用户体验提升

6. **拓扑图**
   - 域名 → 出口 → 隧道 → 服务。
7. **Docker 标签发布**
   - 容器标签自动生成 Fonu 规则和 Cloudflare 出口。
8. **场景模板**
   - Home Assistant、Immich、Jellyfin、qBittorrent、AdGuard。
9. **访问二维码与分享**
   - 手机用户高频需求。
10. **云端覆盖本地**
    - 完成双向同步，不只支持本地覆盖云端。

### P2：高级能力

11. **流量检查模式**
12. **多 Cloudflare 账号**
13. **Access 策略只读增强**
14. **审计日志**
15. **Quick Tunnel 临时演示**
16. **外部 Agent 多主机发布**

### P3：远期

17. PWA / 移动端体验。
18. 团队用户与资源级授权。
19. 浏览器 SSH / RDP / VNC 网关。
20. 多 connector 主备。

---

## 七、针对当前 Fonu 页面的具体改版建议

### 7.1 Cloudflare 总览页

建议结构：

```text
PageHeader
接入状态卡
隧道总览卡
待处理问题
最近变更
快速操作
```

待处理问题可以包括：

- API Token 权限不足。
- 根域名不在 Zone。
- DNS 被其他记录占用。
- 隧道离线。
- 本地与云端漂移。
- cloudflared 有新版本。

### 7.2 隧道配置页

建议分为：

```text
概览
路由
副本 / 指标
DNS
诊断
日志
高级
```

不要把所有内容放在一个长页面里。

### 7.3 反向代理规则页

在规则详情中增加：

```text
出口：本机 Nginx + Cloudflare
Cloudflare 状态：已同步
DNS：已创建
访问保护：未启用
最近错误：无
```

用户不需要离开规则详情就能知道整条链路是否健康。

### 7.4 仪表盘

增加：

- Cloudflare 在线隧道。
- 异常路由。
- DNS 漂移。
- 最近恢复 / 最近故障。
- cloudflared 可更新提示。

---

## 八、最终建议

Fonu 不应继续横向堆叠 cloudflared 参数。更值得投入的是：

1. 把 Cloudflare 接入做成向导。
2. 把“规则 → 出口 → 隧道 → DNS → 监控”做成同一条链路。
3. 把漂移、诊断和修复做成可操作闭环。
4. 把状态页做成真正的可分享服务状态页。
5. 把 Docker / NAS 服务发布做到自动发现。
6. 把高级能力放到高级页，不干扰默认路径。

这会让 Fonu 与普通 cloudflared 面板产生明确差异：

> 普通面板管理 cloudflared；Fonu 管理“我的服务如何安全、稳定地访问”。
