# Cloudflare 隧道：Havline 适配版功能计划

> 日期：2026-09-30
> 定位：`cloudflared-feature-plan.md` 是「官方 + 参考项目的功能全集」；本文是**按 Havline 产品定位筛选后的适配版计划**，实施以本文为准。
> 核心原则：Havline 不是 cloudflared 管理面板，而是「NAS / 家庭服务器的公网访问管理面板」。Cloudflare 隧道只是它的第二条出口。

---

## 一、先明确 Havline 是什么

### 1.1 产品定位

Havline 面对的是家庭 NAS、小主机、家用服务器的用户，核心价值是：

- 用域名访问内网服务，而不是记 IP 和端口；
- 自动处理证书、反代、穿透、DDNS；
- 尽量少配置、少暴露、少维护。

Havline 已经有三条主线：

1. **反向代理**：域名 → 本机 Nginx → 内网服务；
2. **FRP 穿透**：本机 frpc → VPS frps → 域名；
3. **公网 Agent**：管理 VPS 上的 frps 与 Nginx。

Cloudflare 隧道的加入，是为了解决一类现有链路解决不了的人：

> 没有公网 IP、也不想长期租 VPS，但想让内网 Web 服务通过域名安全访问。

### 1.2 因此有三条不可动摇的原则

1. **CF 隧道是第二条出口，不是替代 FRP**：TCP/UDP、自主可控、大流量仍走 FRP。
2. **规则同源**：用户维护的是「服务发布规则」，不是「隧道 ingress」。CF 只是一个出口选项。
3. **融入 Havline 体系**：监控、通知、状态页、日志、备份、升级、审计都复用现有模块，不另造一套面板。

---

## 二、目标用户与场景

### 2.1 适合

- 家宽没有公网 IPv4，或运营商封 80/443；
- 有域名托管在 Cloudflare，或愿意把域名 NS 切到 Cloudflare；
- 主要暴露 HTTP/HTTPS 服务：NAS 面板、PhotoPrism、Jellyfin 管理页、Home Assistant、qBittorrent WebUI、OpenWrt 面板等；
- 希望「一次配置好，之后自动跟随反代规则」。

### 2.2 不适合

- 需要裸 TCP/UDP 端口（游戏服、非 HTTP 协议）；
- 需要完全自主可控、流量不经过第三方；
- 大文件/视频持续分发（Cloudflare 非 Enterprise 条款限制）；
- 企业级 Zero Trust 私有网络、VPN 替代。

---

## 三、借鉴与不借鉴

参考项目 `Tunnel-Cloudflared-Manager 1.0.1` 是一个独立的 cloudflared 面板。以下判断按「是否符合 Havline 产品形态」给出：

### 3.1 值得借鉴

| 参考项目做法 | Havline 该怎么用 |
| --- | --- |
| token 解码成本地凭据 | 已用 Account ID + API Token 替代；保留解码能力用于导入 |
| metrics 端口从日志发现 | 已实现，继续沿用 |
| ingress 三条不变式 | 已实现，继续沿用 |
| 云端 / 本地配置互相同步 | 值得做，但要简化为「diff → 拉取或覆盖」 |
| 端点级增删改 | 值得做，但必须挂在 Havline 的「服务发布规则」下 |
| 快速隧道 | 只做「临时演示」，不做正式功能 |
| 网络诊断 | 值得做，做成 Havline 的一键诊断 |
| 镜像与下载进度 | 值得做，但复用 Havline 现有镜像表，不做用户自定义镜像管理 |
| Windows 隐藏控制台 | 已实现，继续沿用 |

### 3.2 不借鉴

| 参考项目做法 | 不借鉴的理由 |
| --- | --- |
| 自带 JWT + 账号体系 | Havline 已有会话 + API Token，重复建设 |
| 独立主题、多语言 | 与 Havline 全局设置重复 |
| 多账号 cert.pem 登录 | 与 Havline 的 Account ID + API Token 模型冲突；远期若有需求再评估 |
| 私有网络 / WARP routing / virtual network | 面向企业 Zero Trust，NAS 用户不需要 |
| Kubernetes / Ansible / Terraform / 云厂商部署指南 | 不是 Havline 的交付形态 |
| 负载均衡 | 家用单机场景不需要 |
| Workers VPC / Hyperdrive / Spectrum / Keyless SSL | 超出 Havline 定位 |
| WebSocket 实时日志 | Havline 已有 SSE 日志模式，不为 CF 单独引入 WSS |
| 自定义镜像管理（增删改测） | Havline 只需内置镜像表 + 自定义 URL，不需要用户维护镜像源 |
| 独立 systemd / Windows service 安装 | Havline 自己托管进程，不接管外部服务 |
| 快速隧道作为正式功能 | 官方明确不保证 SLA，只做演示入口 |

---

## 四、符合 Havline 的功能全景

按「用户价值」而不是「参考项目有什么」重新分组。

### 4.1 出口抽象（Havline 的核心差异化）

**F-1 服务发布规则增加「出口」字段**

- 一条规则描述「域名 → 内网服务」，出口可选：本机 Nginx / FRP / Cloudflare 隧道。
- 现有反向代理规则作为基础，新增出口选择，不新建第二套规则模型。
- 验收：同一条规则可以切换出口，切换后域名与服务保持不变。

**F-2 CF 隧道 ingress 自动跟随反代规则**

- 反代规则新增/修改/删除域名时，自动同步到该隧道的 ingress。
- 只同步 HTTP/HTTPS 规则；TCP/UDP 规则在出口选择里标为「Cloudflare 不支持」。
- 验收：改一条反代规则，CF 隧道配置与云端 ingress 自动更新，无需手工编辑。

**F-3 一条规则可以同时走 FRP 与 CF**

- 出口支持多选（FRP + Cloudflare），用于灰度或双保险。
- 验收：同一域名可同时存在 FRP 与 CF 入口，互不影响。

### 4.2 自动化（延续 Havline「自动为主」）

**F-4 一次配置，后续全自动**（大部分已完成）

- Account ID + API Token 一次保存；
- 自动创建隧道、写 `creds.json` / `config.yml`、推送 ingress、创建 CNAME；
- 自动启动、自动重连、崩溃重启。

**F-5 反代规则变化自动同步**

- 规则增删改 → 生成新 ingress → 本地与云端同步 → 需要时重载配置。
- 验收：用户不需要理解 ingress，也不需要手工点同步。

**F-6 自动 DNS 管理**

- 创建域名时自动 CNAME 到 `<tunnel-id>.cfargotunnel.com`；
- 删除域名或隧道时，删除「指向该隧道」的 DNS 记录（只删自己创建的）。
- 验收：域名增删与 DNS 一致，不误删用户其他记录。

**F-7 自动健康与自愈**

- 进程退出自动重启；
- `/ready` 与 `ha_connections` 判活；
- 连续失败进入 Havline 告警；恢复后发通知。

### 4.3 融入 Havline 现有体系

**F-8 监控与可用率**

- 新增 cloudflared 的 `monitor.Source`；
- 隧道健康进入 Havline 现有可用率与历史指标。

**F-9 通知**

- 隧道断连、恢复、证书/DNS 失败复用 `notify`；
- 通知文案面向 NAS 用户，而不是 cloudflared 术语。

**F-10 状态页**

- 公开状态页增加 CF 隧道域名与可用率；
- 只展示域名与状态，不泄露隧道 ID、本机地址。

**F-11 日志中心**

- CF 隧道日志进入 Havline 日志页；
- 复用 SSE 增量推送，不单独做 WebSocket。

**F-12 仪表盘**

- 仪表盘增加「Cloudflare 隧道」卡片：隧道数、在线数、最近错误。

**F-13 备份与迁移**

- `cf_tunnels` 与凭据纳入 Havline 备份/恢复流程；
- 恢复后自动重启隧道。

**F-14 一键升级与版本策略**

- cloudflared 版本跟随 Havline 升级流程；
- 提供「检查更新 / 更新 cloudflared / 回滚」；
- 不要求用户手工替换二进制。

**F-15 审计**

- 记录谁在何时修改了服务发布规则、出口、CF 凭据、全局开关；
- 复用 Havline 的日志/审计基础设施。

### 4.4 NAS 场景体验

**F-16 Docker 服务发现一键发布**

- 复用现有 `discovery`，发现本机 Docker 服务；
- 选择服务 → 填域名 → 自动创建反代规则 + CF 出口。

**F-17 常用 NAS 应用预设**

- NAS 面板、PhotoPrism、Jellyfin、Home Assistant、qBittorrent 等端口预设；
- 用户只需选应用与域名。

**F-18 域名与 Zone 选择器**

- 通过 API Token 列出账号下的 Zone 与可用域名；
- 用户从列表选择，而不是手输。

**F-19 证书提示**

- 使用 CF 出口时，CF 边缘自动提供 HTTPS；
- 页面提示「该域名无需在 Havline 申请证书」，避免用户重复操作。

### 4.5 运维与安全

**F-20 端点级编辑**

- 在 Havline 规则详情里编辑 hostname、path、service、协议；
- 提供 `ingress validate` 与 `ingress rule <url>` 校验。

**F-21 云端漂移检测**

- 定期比较本地 ingress 与云端配置；
- 发现漂移后提示「以本地为准覆盖」或「以云端为准拉取」。

**F-22 一键诊断**

- `tunnel info`、`tunnel diag`、网络预检（region1/region2、7844、443）、系统要求检查；
- 结果转成 Havline 风格的可读结论与建议。

**F-23 令牌轮换**

- 提供「重新签发隧道凭据」入口；
- 轮换过程尽量不中断，失败自动回滚。

**F-24 全局托管开关**（已完成）

- 一键停止全部隧道，重启不自动拉起；
- 开启时只恢复 auto_start 的隧道。

**F-25 条款与用量提示**

- 页面常驻说明：不适合大文件/视频分发；
- 不采集、不上报用户流量到外部。

**F-26 Access 保护状态检测（只读）**

- 检测域名是否启用 Cloudflare Access；
- 只提示，不代配策略。

### 4.6 可选增强

**F-27 快速隧道（临时演示）**

- 临时把某个本地端口发到 `trycloudflare.com`；
- 明确标注不保证 SLA、URL 每次变化、仅用于测试。

**F-28 多副本**

- 高级选项，用于「主备两台设备跑同一条隧道」；
- 默认关闭，不打扰普通用户。

**F-29 边缘 IP 优选**

- 只影响「本机 → CF 边缘」一段；
- 页面展示优选建议，不代写 hosts；
- 面向国内链路质量差的用户。

**F-30 账号模式（cert.pem）**

- 仅当用户明确需要「多账号 / 团队权限」时提供；
- 默认仍推荐 Account ID + API Token。

---

## 五、明确不做的功能

| 功能 | 结论 | 理由 |
| --- | --- | --- |
| 独立登录、JWT、多用户体系 | 不做 | Havline 已有认证与 API Token |
| 独立主题、多语言 | 不做 | 属于 Havline 全局设置 |
| 私有网络 / WARP routing | 不做（远期评估） | 面向 Zero Trust 企业，NAS 用户不需要 |
| 负载均衡 | 不做 | 家用单机场景不需要 |
| Kubernetes / Ansible / Terraform 指南 | 不做 | 不是 Havline 交付形态 |
| Workers VPC / Hyperdrive / Spectrum | 不做 | 超范围 |
| WebSocket 实时日志 | 不做 | 复用 Havline SSE |
| 自定义镜像管理 | 不做 | 内置镜像表 + 自定义 URL 足够 |
| 接管外部 systemd / Windows service | 不做 | Havline 只托管自己启动的进程 |
| 快速隧道作为正式功能 | 不做 | 官方不保证 SLA，只做演示 |
| 代配 Access 策略 | 不做 | 高风险，只做只读检测 |
| 自动扩权 / 自动改系统防火墙 | 不做 | 保持 Havline 现有安全边界 |

---

## 六、与现有模块的整合点

| Havline 模块 | 整合方式 |
| --- | --- |
| 反向代理规则 | 增加「出口」字段；CF 隧道从规则派生 |
| FRP 多服务端 | 与 CF 出口并列，互不影响 |
| 证书管理 | CF 出口域名提示无需本地证书 |
| DDNS | CF 橙色云代理后本地 IP 变化不再影响访问，可提示用户 |
| 监控 / 可用率 | 新增 cloudflared Source |
| 通知 | 断连、恢复、DNS 失败复用 |
| 公开状态页 | 增加 CF 隧道条目 |
| 日志中心 | CF 日志进入现有日志页 |
| 仪表盘 | 新增 CF 隧道卡片 |
| Docker 服务发现 | 一键发布到 CF |
| 备份 / 恢复 | cf_tunnels 与凭据纳入 |
| 一键升级 | cloudflared 版本更新纳入 |
| 审计 | 规则、出口、凭据变更记录 |

---

## 七、数据模型与 API 变更

### 7.1 数据模型

- `frp_proxies`（或服务发布规则表）增加 `exit` 字段：`local` / `frp` / `cloudflare` / 多选；
- `cf_tunnels` 增加 `rule_id`（可选，关联服务发布规则）、`source`、`last_sync_at`；
- 新增 `cf_endpoint_drift` 或复用现有漂移检测表；
- `cf_settings` 增加 `auto_update`、`update_channel`。

### 7.2 API

- `PUT /api/proxies/{id}/exit`：切换出口；
- `GET /api/cloudflare/tunnels/{id}/drift`：漂移检测；
- `POST /api/cloudflare/tunnels/{id}/pull`：从云端拉取；
- `POST /api/cloudflare/tunnels/{id}/diagnostics`：一键诊断；
- `GET /api/cloudflare/zones`：Zone / 域名列表；
- `POST /api/cloudflare/tunnels/{id}/token/rotate`：令牌轮换；
- `GET /api/cloudflare/tunnels/{id}/logs/stream`：SSE 日志；
- `GET /api/discovery/docker`：Docker 服务发现（若尚未提供）。

### 7.3 前端

- 反向代理规则表单增加「出口」选择器；
- 规则详情增加「Cloudflare 隧道」状态与操作；
- CF 页面聚焦「隧道状态 / 诊断 / 日志 / 全局开关」，弱化「手工建隧道」；
- 仪表盘、状态页、日志中心增加 CF 条目。

---

## 八、阶段计划

| 阶段 | 内容 | 目标 |
| --- | --- | --- |
| 阶段 1 | F-1 ~ F-3 出口抽象；F-5 规则同步；F-6 DNS 管理完善 | 让 CF 成为反代规则的一个出口，而不是独立模块 |
| 阶段 2 | F-8 ~ F-15 体系整合；F-20 ~ F-22 编辑与诊断 | 让 CF 隧道进入 Havline 监控、通知、状态页、日志、备份、审计 |
| 阶段 3 | F-16 ~ F-19 NAS 体验；F-23 ~ F-26 运维安全 | 一键发布、域名选择、令牌轮换、条款提示、Access 检测 |
| 阶段 4 | F-27 ~ F-30 可选增强 | 快速隧道、多副本、边缘优选、账号模式按需 |

**优先级建议**：阶段 1 是产品形态的关键，先做；阶段 2 决定「能不能长期运维」；阶段 3 决定「好不好用」；阶段 4 是加分项。

---

## 九、验收标准

- 用户只维护「域名 → 服务」的规则，出口切换不需要改域名或端口；
- 反代规则变化后，CF 隧道与 DNS 自动跟随；
- 隧道断连能进 Havline 告警与状态页；
- 一条规则可以同时走 FRP 与 CF，互不影响；
- CF 出口域名不需要本地证书；
- 备份恢复后隧道能自动恢复；
- 所有 CF 凭据加密存储、预览脱敏；
- 明确不做的功能不会出现在界面里，避免用户误解。

---

## 十、结论

Havline 需要的不是「另一个 cloudflared 面板」，而是：

> 把 Cloudflare 隧道做成反向代理规则的一个出口选项，并让监控、通知、状态页、日志、备份、升级、审计自动覆盖它。

按本文阶段 1-3 实施，Havline 的 CF 模块会形成与参考项目不同的产品形态：参考项目面向「管理 cloudflared」，Havline 面向「管理我的服务怎么上公网」。

本文与 `cloudflared-feature-plan.md` 的关系：

- `cloudflared-feature-plan.md`：官方 + 参考项目的功能全集，用于查漏；
- 本文：按 Havline 定位筛选后的实施计划，**实施以本文为准**。
