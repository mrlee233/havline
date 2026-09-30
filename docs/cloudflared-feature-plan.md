# Cloudflare 隧道模块全功能规划

> 调研日期：2026-09-30
> 调研对象：Cloudflare 官方文档（developers.cloudflare.com，2026-08/09 更新）+ `F:\WWW\cf\Tunnel-Cloudflared-linux`（Tunnel-Cloudflared-Manager 1.0.1）+ Fonu 1.8.0 现有实现
> 目标：把 Fonu 的「Cloudflare 隧道」从「可用的创建与托管」推进到「覆盖官方主要能力、可长期运维」的完整模块。

---

## 一、官方文档调研结论

Cloudflare 把 Tunnel 分成两条产品线，Fonu 目前主要落在第一条，规划时需要同时兼顾两条：

| 产品线 | 文档入口 | 典型用途 |
| --- | --- | --- |
| Zero Trust / 私有网络 | [cloudflare-one Tunnel](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/) | 私有网络、SSH/RDP、VPN 替代、Access 保护 |
| 公开发布应用 | [tunnel](https://developers.cloudflare.com/tunnel/) | 把公网域名指向自建服务，替代 VPS 与端口暴露 |

### 1.1 核心概念（官方）

- **Tunnel**：一条以 UUID 标识的持久对象，是源站与 Cloudflare 之间的逻辑链路。
- **Connector**：一个 `cloudflared` 进程；同一条隧道可以运行多个 connector。
- **Replica**：同一隧道的多个 `cloudflared` 实例，用于高可用与零停机更新。
- **Remotely-managed**：隧道由 Cloudflare 面板 / API 管理，规则在云端，`cloudflared` 用 token 启动。
- **Locally-managed**：规则在本机 `config.yml`，用 `credentials-file` 启动。
- **Quick tunnel**：`trycloudflare.com` 临时隧道，无需账号，官方明确仅用于测试与开发。
- **Virtual network**：私有网络中用于隔离不同网络的路由域。

### 1.2 官方关键能力清单

**隧道与连接器**

- 创建、删除、列出、按名称或 UUID 查询隧道
- 获取隧道 token；token 轮换
- 同一隧道部署多个 replica
- `cloudflared tunnel info` 查看连接器与连接状态
- 每个 connector 默认建立 4 条出站连接，分布在至少两个数据中心
- 支持 post-quantum 加密（`--post-quantum`）

**路由**

- Published application：`hostname`（可带 `path`）→ `service`
- 支持服务协议：`http`、`https`、`ssh`、`rdp`、`smb`、`tcp`、`unix`、`http_status`、`http_host`、TCP over WebSocket 等
- DNS：创建 CNAME 指向 `<tunnel-id>.cfargotunnel.com`，需开启代理
- 私有网络：CIDR route、WARP routing、virtual network
- Load Balancing：公开负载均衡与私有负载均衡

**配置**

- `config.yml` 顶层：`tunnel`、`credentials-file`、`ingress`、`warp-routing`、`originRequest`、`protocol`、`edge-ip-version`、`ha-connections`、`retries`、`grace-period`、`post-quantum`、`region`、`tag`、`logfile`、`loglevel`、`metrics`、`pidfile`、`token`、`token-file`
- `ingress` 必须末条 catch-all（如 `http_status:404`），按顺序匹配 hostname / path
- Origin 参数：TLS（`originServerName`、`matchSNItoHost`、`caPool`、`noTLSVerify`、`tlsTimeout`、`http2Origin`）、HTTP（`httpHostHeader`、`disableChunkedEncoding`）、连接（`connectTimeout`、`noHappyEyeballs`、`proxyType`、`proxyAddress`、`proxyPort`、`keepAliveTimeout`、`keepAliveConnections`、`tcpKeepAlive`）、Access（`access`）
- 校验工具：`cloudflared tunnel ingress validate`、`cloudflared tunnel ingress rule <url>`

**观测**

- Tunnel health：面板隧道健康、`cloudflared tunnel info`
- Cloudflare Notifications：隧道健康事件通知
- Logs：Cloudflare 侧日志、remote log streaming、network session logs、本机 `cloudflared` 日志
- Metrics：`/metrics`（Prometheus 格式）、`/ready`、`/healthcheck`
- Diagnostic logs：`cloudflared tunnel diag` 生成诊断文件

**运维与部署**

- Linux systemd、macOS launch agent、Windows service
- Docker、Kubernetes、Ansible、Terraform、AWS/Azure/GCP 指南
- 更新 cloudflared；多实例零停机更新
- 系统要求：出站 7844（TCP/UDP）与 443；ulimit；容量计算
- 文件权限要求：`cert.pem`、`<UUID>.json`、`config.yml`

**权限与安全**

- API Token 权限：`Account > Cloudflare Tunnel > Edit`、`Zone > DNS > Edit`（查询 Zone 还需 `Zone > Zone > Read`）
- Access、Service Auth、mTLS、Keyless SSL、Spectrum、Workers VPC、Hyperdrive 等集成
- Tunnel ownership、account-scoped roles、撤销账号证书

### 1.3 官方版本要点（2026-09）

- 运行参数文档最近更新 2026-09；`post-quantum`、`token-file`、`region`、`tag`、`dns-resolver-addrs`、`edge-bind-address` 等参数已进入官方参考。
- `edge-ip-version` 官方默认值曾由 `4` 调整为 `auto`；Fonu 已显式下发 `4`，与国内链路建议一致。
- 官方把「公开发布应用」与「Zero Trust 私有网络」拆成两条文档线，Fonu 需要明确自己的默认定位是公开发布应用。

---

## 二、参考项目功能清单（Tunnel-Cloudflared-Manager 1.0.1）

参考项目是一个独立面板，功能面比 Fonu 当前实现更宽，但缺少与 Fonu 的规则、健康、通知、状态页体系融合。

### 2.1 隧道管理

- 命名隧道：token 启动、config-file 启动
- 快速隧道：`--url` 启动、从日志解析 `*.trycloudflare.com` URL
- 启动 / 停止 / 停止全部 / 运行列表 / URL 查询
- 创建隧道（账号模式，`tunnel create`）
- 初始化 token 隧道：解码 token → 写 `creds.json` + `config.yml`
- 删除云端隧道

### 2.2 路由与云端同步

- 端点列表：读取本地 `config.yml` 的 ingress
- 新增 / 更新 / 删除端点（本地）
- 新增端点时自动调用 `tunnel route dns` 创建 DNS
- 从云端拉取 ingress 到本地
- 把本地 ingress 推送到云端
- ingress 三条不变式：catch-all、`localhost` 归一化、插入在 catch-all 之前

### 2.3 账号模式

- `cert.pem` 登录与 finalize
- 列出账号下的隧道
- 同步云端隧道
- 删除账号

### 2.4 二进制与镜像

- 检查 / 下载 / 替换 cloudflared
- 下载进度
- 镜像管理：列出、新增、更新、删除、测试
- 内置官方与多个 gh-proxy 镜像

### 2.5 指标与日志

- 每 2 秒抓取 `/metrics`
- QUIC：`quic_client_receive_bytes`、`quic_client_sent_bytes`、`quic_client_smoothed_rtt`
- HTTP/2：RPC latency、主动 HEAD 探测；带宽用进程 socket 统计（Windows 需要管理员）
- 连接数：`cloudflared_tunnel_ha_connections`
- 实时日志：WebSocket 推送
- 清空日志

### 2.6 网络与调优

- `protocol`（auto/http2/quic）、代理环境探测与回落
- `edge-ip-version`、`ha-connections`
- `originRequest`：`caPool`、`noTLSVerify`、`httpHostHeader`
- 网络诊断接口
- Windows 子进程隐藏控制台

### 2.7 运维

- 版本检查与应用自更新
- JWT + bcrypt 登录、失败锁定
- 多语言、主题
- Docker 与 Linux 安装脚本

---

## 三、Fonu 当前实现（v1.8.0）

| 能力 | 状态 |
| --- | --- |
| Account ID + API Token 凭据 | ✅ 已实现 |
| API Token 验证（`/accounts/{id}/tokens/verify`） | ✅ 已实现 |
| 创建 / 删除云端隧道 | ✅ 已实现 |
| 获取隧道 token、写 `creds.json` / `config.yml` | ✅ 已实现 |
| 推送 ingress 到云端 | ✅ 已实现 |
| 自动创建 CNAME DNS | ✅ 已实现 |
| 二进制下载与版本检查 | ✅ 已实现（多镜像、大小与可执行校验） |
| 进程托管、启停、重启、日志文件 | ✅ 已实现 |
| metrics 端口发现、`/ready`、`ha_connections`、QUIC 带宽/RTT | ✅ 已实现（基础） |
| 全局托管开关 | ✅ 已实现 |
| auto_start | ✅ 已实现 |
| 前端页面与弹窗 | ✅ 已实现 |
| 云端隧道列表同步 / 导入已有隧道 | ❌ 未实现 |
| 端点级 CRUD（path / 协议 / 多服务） | ❌ 未实现 |
| 从云端拉取 ingress（pull） | ❌ 未实现 |
| 多副本 / 零停机更新 | ❌ 未实现 |
| 快速隧道 | ❌ 未实现 |
| 账号模式（cert.pem） | ❌ 未实现 |
| 实时日志流（WebSocket/SSE） | ❌ 未实现 |
| 诊断日志、网络预检、隧道 info | ❌ 未实现 |
| 接入 Fonu monitor / 通知 / 状态页 | ❌ 未实现 |
| cloudflared 自动更新 | ❌ 未实现 |
| 自定义镜像管理、下载进度 | ❌ 未实现 |
| Origin 高级参数 UI | ❌ 未实现 |
| 令牌轮换、权限审计 | ❌ 未实现 |
| 私有网络 / WARP routing / Access / 负载均衡 | ❌ 未实现 |
| 边缘 IP 优选、post-quantum、region/tag | ❌ 未实现 |

---

## 四、差距与机会

### 4.1 必须补齐（否则模块不完整）

1. **云端隧道列表与导入**：目前 Fonu 只能管理自己创建的隧道，已有隧道无法纳管。
2. **端点级 CRUD**：现在是整体覆盖 ingress，无法单独增删改一条 hostname/path。
3. **云端配置拉取与漂移检测**：云端被手动修改后，Fonu 无法发现。
4. **健康接入 Fonu 体系**：隧道状态没有进入 monitor、通知、状态页、可用率。
5. **实时日志**：轮询文件在故障排查时体验差。
6. **诊断能力**：缺 `tunnel info`、`tunnel diag`、网络预检、端口要求检查。

### 4.2 高价值增强

1. 快速隧道（临时演示、开发调试）
2. 多副本与零停机更新
3. cloudflared 自动更新与版本策略
4. 令牌轮换
5. Origin 高级参数（HTTPS 回源、自签证书、Host 头）
6. 自定义镜像与下载进度
7. 审计日志（谁在何时修改了隧道、端点、DNS）

### 4.3 远期能力

1. 账号模式（cert.pem）与多账号管理
2. 私有网络 / WARP routing / virtual network
3. Cloudflare Access / Service Auth 集成
4. 负载均衡集成
5. 边缘 IP 优选
6. Workers VPC / Hyperdrive / Spectrum 等生态集成

---

## 五、全量功能规划

### P0：模块完整性（先补齐）

**P0-1 云端隧道同步与导入**

- 目标：列出 Cloudflare 账号下的全部隧道，把未纳管的隧道导入 Fonu。
- 实现：`GET /accounts/{id}/cfd_tunnel` 列表；导入时读取 token、写 `creds.json`/`config.yml`、落库。
- 验收：账号下已有隧道能在页面看到并导入；重复导入不产生重复记录。

**P0-2 端点级 CRUD 与 ingress 校验**

- 目标：不再整体覆盖 ingress，支持单条 hostname/path/service 的增删改。
- 实现：本地 `config.yml` 解析与增量修改；云端 `PUT configurations` 同步；`cloudflared tunnel ingress validate` 与 `ingress rule <url>` 自检。
- 验收：新增、修改、删除单个端点后本地与云端一致；非法规则被拒绝。

**P0-3 云端配置拉取与漂移检测**

- 目标：云端被手动修改后，Fonu 能发现差异并选择拉取或覆盖。
- 实现：`GET /accounts/{id}/cfd_tunnel/{tunnel}/configurations`；与本地 config 做 diff；提供「拉取到本地」「推送到云端」。
- 验收：diff 可见；两个方向都能执行并留痕。

**P0-4 健康接入 Fonu 体系**

- 目标：隧道断连、恢复进入现有 monitor、通知、状态页与可用率。
- 实现：新增 cloudflared 的 `monitor.Source`；`/ready` + `ha_connections` 判活；断连/恢复事件复用 notify；状态页增加隧道条目。
- 验收：停掉隧道后产生告警；恢复后产生恢复通知；状态页可见。

**P0-5 实时日志（SSE）**

- 目标：日志页实时滚动，替代轮询。
- 实现：Fonu 现有日志 SSE 模式复用；cloudflared 日志文件 tail + 增量推送；保留轮询回退。
- 验收：页面实时看到新日志；切换页面/重连不丢关键行。

**P0-6 诊断能力**

- 目标：一键完成常见故障排查。
- 实现：`cloudflared tunnel info <name>`、`cloudflared tunnel diag`、`cloudflared tunnel ingress validate`、网络预检（region1/region2、7844、443）、端口与系统要求检查。
- 验收：诊断页给出结构化结果与建议，不只是原始输出。

### P1：运维与安全

**P1-1 快速隧道**

- 目标：临时把本地端口发布到 `trycloudflare.com`，用于演示与开发。
- 实现：`--url` 启动、日志解析公共 URL、进程随 Fonu 生命周期、默认关闭。
- 边界：明确标注不保证 SLA、URL 每次变化。

**P1-2 多副本与零停机更新**

- 目标：同一隧道支持多个 connector，更新时不中断。
- 实现：replica 配置与进程管理；逐个重启；`tunnel info` 确认新连接注册后再停旧实例。
- 验收：更新过程中隧道可用率不下降。

**P1-3 cloudflared 自动更新与版本策略**

- 目标：可检查新版本、按策略升级、支持回滚。
- 实现：官方 Release 检查；保留旧版本目录；升级前校验；失败回滚。
- 验收：升级成功可回滚；版本信息可追溯。

**P1-4 令牌轮换**

- 目标：按官方 `tunnel token` 支持重新签发 token。
- 实现：`GET /accounts/{id}/cfd_tunnel/{tunnel}/token`；轮换后更新 `creds.json` 并重启连接器；双凭据过渡窗口。
- 验收：轮换过程不中断或只短暂闪断。

**P1-5 Origin 高级参数**

- 目标：支持 HTTPS 回源、自签证书、Host 头等官方 origin 参数。
- 实现：`originRequest` 的 `originServerName`、`matchSNItoHost`、`caPool`、`noTLSVerify`、`httpHostHeader`、`connectTimeout`、`keepAliveTimeout` 等。
- 验收：HTTPS 回源与自签证书场景可用；参数写入本地与云端配置。

**P1-6 审计日志**

- 目标：记录谁在何时修改了隧道、端点、DNS、令牌与全局开关。
- 实现：复用 Fonu 日志/审计基础设施；操作前记录旧值、操作后记录新值。
- 验收：关键变更可查询、可导出。

### P2：体验与增强

**P2-1 自定义镜像与下载进度**

- 目标：镜像可增删改测，下载有进度。
- 实现：镜像表（URL 模板）+ 连通性测试；SSE 推送下载进度。

**P2-2 指标图表与历史**

- 目标：展示隧道延迟、带宽、连接数、可用率历史。
- 实现：复用 `metrics_samples`；前端图表；区分 QUIC/HTTP2 可用指标。

**P2-3 配置版本、diff 与回滚**

- 目标：隧道 `config.yml` 与云端 ingress 的版本化。
- 实现：复用 `fsutil.BackupVersioned`；变更前留版本；支持 diff 与回滚。

**P2-4 日志导出与清理**

- 目标：导出、清理单隧道日志；限制日志体积。
- 实现：日志文件轮转、下载、清空；保留期设置。

**P2-5 系统要求与容量检查**

- 目标：按官方要求检查 ulimit、出站 7844/443、系统资源。
- 实现：启动前检查与页面提示；不做自动修改系统参数。

**P2-6 网络调优扩展**

- 目标：暴露官方 run parameters。
- 实现：`post-quantum`、`region`、`tag`、`dns-resolver-addrs`、`edge-bind-address`、`retries`、`grace-period`、`loglevel`、`logfile`、`pidfile`。

### P3：平台化

**P3-1 账号模式（cert.pem）与多账号管理**

- 支持 `cloudflared tunnel login`、cert.pem 导入、多账号切换、账号下隧道同步。

**P3-2 私有网络（WARP routing / CIDR / virtual network）**

- 支持 `warp-routing: enabled: true`、CIDR route、virtual network、私有主机名路由。

**P3-3 Access / Service Auth**

- 只做诊断与提示，或在用户显式配置后生成 Access 应用；不代配高风险策略。

**P3-4 负载均衡集成**

- 公开/私有负载均衡池与隧道关联；只做只读展示与跳转。

**P3-5 边缘 IP 优选**

- 探测官方 IPv4 段的 TCP 7844 延迟，生成两个 region 的优选建议；页面展示，不代写 hosts。

**P3-6 生态集成**

- Workers VPC、Hyperdrive、Spectrum、Keyless SSL、Data Localization 等按需评估。

---

## 六、推荐实施顺序

| 阶段 | 内容 | 预计 | 完成标志 |
| --- | --- | --- | --- |
| 阶段 1 | P0-1 ~ P0-6 | 1-2 周 | 云端同步、端点 CRUD、漂移检测、健康接入、实时日志、诊断全部可用 |
| 阶段 2 | P1-1 ~ P1-6 | 2-3 周 | 快速隧道、多副本、自动更新、令牌轮换、Origin 高级参数、审计 |
| 阶段 3 | P2-1 ~ P2-6 | 2 周 | 镜像、图表、版本回滚、日志治理、系统检查、网络调优扩展 |
| 阶段 4 | P3-1 ~ P3-6 | 按需 | 账号模式、私有网络、Access、负载均衡、边缘优选、生态集成 |

建议先做阶段 1，因为它直接补齐「已经有隧道但无法纳管」「云端被改无法发现」「故障无法定位」这三类现实问题。

---

## 七、数据模型与 API 变更

### 7.1 数据模型

- `cf_tunnels`：增加 `source`（created/imported）、`cloud_status`、`connector_count`、`last_sync_at`。
- 新增 `cf_tunnel_endpoints`：`tunnel_id`、`hostname`、`path`、`service`、`protocol`、`origin_json`。
- 新增 `cf_tunnel_events`：健康事件、审计事件、同步事件。
- `cf_settings`：扩展 `enabled`、`auto_update`、`update_channel`、`mirror`。

### 7.2 API

- `GET /api/cloudflare/cloud-tunnels`：列出云端隧道
- `POST /api/cloudflare/cloud-tunnels/{id}/import`：导入
- `GET/POST/PUT/DELETE /api/cloudflare/tunnels/{id}/endpoints`：端点 CRUD
- `POST /api/cloudflare/tunnels/{id}/pull`：从云端拉取
- `GET /api/cloudflare/tunnels/{id}/drift`：漂移检测
- `GET /api/cloudflare/tunnels/{id}/logs/stream`：实时日志
- `GET /api/cloudflare/tunnels/{id}/diagnostics`：诊断
- `POST /api/cloudflare/tunnels/{id}/token/rotate`：令牌轮换
- `GET/POST /api/cloudflare/mirrors`：镜像管理

### 7.3 前端

- 隧道详情增加端点列表、diff、诊断、实时日志、指标图表。
- 新增「云端隧道」页签或导入入口。
- 应用配置增加自动更新策略、镜像管理、系统要求检查结果。

---

## 八、测试与验收

**单元测试**

- ingress 增删改与三条不变式
- 云端配置 diff
- token 解码、轮换
- metrics 解析与降级
- 诊断结果解析

**集成测试**

- 创建 → 启动 → 健康 → 停止 → 删除全流程
- 云端修改后用 diff 发现并拉取
- 多副本零停机更新
- 自动 DNS 创建/更新

**前端**

- 端点编辑、diff 视图、诊断面板、实时日志

**真机矩阵**

- Windows、Linux、Docker 容器内、host 网络
- 代理环境、QUIC 被拦、7443/7844 被占用、无 IPv6
- Cloudflare Access 与无 Access

---

## 九、风险与边界

- **官方接口演进**：Cloudflare API 与 cloudflared 参数会变；所有参数需要可配置 + 默认值 + 可恢复默认。
- **权限边界**：API Token 权限不足时只提示，不自动扩权。
- **网络边界**：只做本机到 CF 边缘的调优；访客侧不可控。
- **合规边界**：大文件/视频分发不在推荐用法内。
- **安全边界**：不代配 Access 高风险策略；不写 hosts；不改系统 ulimit/防火墙。
- **运行边界**：Fonu 只托管自己启动的 cloudflared 进程；不接管外部 systemd 服务。
- **数据边界**：凭据加密存储；预览脱敏；审计日志不记录明文 token。

---

## 十、结论

Fonu 的 Cloudflare 隧道模块已经具备「创建、运行、托管、DNS」的闭环。下一阶段的目标是把它从「能用」推进到「可长期运维」：

1. 先补齐云端同步、端点 CRUD、漂移检测、健康接入、实时日志、诊断（阶段 1）。
2. 再补快速隧道、多副本、自动更新、令牌轮换、Origin 高级参数、审计（阶段 2）。
3. 最后做镜像、图表、版本回滚、日志治理、系统检查与网络调优扩展（阶段 3）。
4. 私有网络、Access、负载均衡、边缘优选等平台化能力按需推进（阶段 4）。

这样 Fonu 才能在官方能力、参考项目功能与自身产品体系之间取得平衡，而不是停留在「进程开关 + 配置预览」。
