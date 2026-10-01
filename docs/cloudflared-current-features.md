# Cloudflare 隧道当前功能说明

> 日期：2026-10-01
> 状态：v1.9.0 实测功能基线。本文描述**当前代码实际实现的能力**，不是早期规划稿。
> 相关文档：
> - `cloudflared-integration-plan.md`：早期融合方案
> - `cloudflared-feature-plan.md`：官方与参考项目功能全集，用于查漏
> - `cloudflared-fonu-fit-plan.md`：Fonu 适配版阶段计划

---

## 一、模块定位

Cloudflare 隧道在 Fonu 中不是独立的 cloudflared 管理面板，而是公网访问的第二条出口：

- 本机 Nginx：适合有公网 IP 或已有端口映射的场景。
- FRP：适合需要 TCP/UDP、自控 VPS 或大流量的场景。
- Cloudflare 隧道：适合没有公网 IP、主要发布 HTTP/HTTPS Web 服务的场景。

当前模块已经融入 Fonu 的规则、监控、通知、状态页、日志、备份、诊断和升级体系。

---

## 二、已实现的入口与页面

### 2.1 Cloudflare 隧道列表页

页面：`web/src/views/CloudflareView.vue`

已实现：

- 隧道列表显示：
  - 隧道名称
  - 状态
  - 副本 / cloudflared HA 连接数
  - cloudflared 版本
  - 路由数量
  - 指标端口
- 全局托管开关：
  - 关闭：停止全部隧道。
  - 开启：立即启动全部隧道。
  - 应用重启：全局托管开启时自动恢复全部隧道。
- 新建隧道弹窗：
  - 自动托管模式。
  - 手工隧道模式。
- 手工隧道暴露规则：
  - 子域名输入。
  - 域从 Cloudflare Zone 列表选择。
  - HTTP / HTTPS / TCP / SSH 服务类型。
  - `://` 与 URL 分离显示。
  - 实时显示完整主机名。
- 隧道操作：
  - 详情。
  - 配置页。
  - 启动 / 停止 / 重启。
  - 删除。
- 应用配置：
  - Account ID。
  - API Token。
  - Token 验证。
  - cloudflared 下载镜像。
  - 默认网络参数。

### 2.2 隧道配置页

页面：`web/src/views/CloudflareTunnelConfigView.vue`

已实现：

- 独立路由页面，不再使用编辑弹窗：
  - `/cloudflare/:id/config`
- 指标卡：
  - 活动副本
  - 路由数
  - 状态
  - 运行时间
- 基础配置：
  - 页面只显示“编辑基础配置”按钮。
  - 点击后弹窗编辑名称、子域名、Zone、服务类型、URL、自动启动。
- 路由列表：
  - 顺序。
  - 目标域名。
  - 路径。
  - 服务。
  - 来源。
  - 操作。
- 拓扑图：
  - 域名节点。
  - Cloudflare 出口节点。
  - 隧道节点。
  - 服务节点。
  - 点阵画布和曲线连接。
- 路由操作：
  - 添加路由。
  - 编辑路由。
  - 删除路由。
  - 上移 / 下移。
  - 复制路由。
  - 测试源服务 TCP 连通性。
- 路由弹窗：
  - 子域名 + Zone 下拉。
  - HTTP / HTTPS / TCP / SSH + URL。
  - 完整主机名实时预览。
- 全部捕获规则：
  - 默认显示 `http_status:404`。
- 运行控制：
  - 启动、停止、重启。

自动托管隧道的路由只读，来源显示为“反向代理规则”，并提示到“反向代理”页维护。

---

## 三、Cloudflare 账号与二进制

### 3.1 账号配置

实现文件：

- `internal/cloudflared/cloudflare_api.go`
- `internal/cloudflared/service.go`

已实现：

- 保存 Account ID + API Token。
- API Token 加密存储。
- Token 掩码回显。
- 调用账号级 `tokens/verify` 验证 Token。
- 查询当前 Token 可访问的 Zone。
- 根据域名向上查找所属 Zone。
- 创建、读取、删除 Cloudflare 隧道。
- 获取隧道 Token。
- 读取和覆盖远端 ingress。
- 创建、更新、删除 DNS CNAME。
- 只读检测 Cloudflare Access。

### 3.2 cloudflared 二进制

实现文件：

- `internal/cloudflared/binary.go`

已实现：

- 内置多种下载镜像。
- 下载并原子替换本机 cloudflared。
- 校验文件大小和可执行性。
- 显示当前版本。
- 查询 GitHub 最新版本。
- 下载指定版本。
- 保留上一版并支持回滚。

版本显示统一为语义版本，例如：

```text
2026.9.3
```

---

## 四、隧道生命周期

实现文件：

- `internal/cloudflared/service.go`
- `internal/cloudflared/manager.go`
- `internal/cloudflared/store.go`

已实现：

- 通过 Cloudflare API 创建命名隧道。
- 生成本地凭据：
  - `creds.json`
  - `config.yml`
  - `cloudflared.log`
- 支持隧道模式模型：
  - `account-local`
  - `token-local`
  - `token-remote`
- 进程托管：
  - 启动。
  - 停止。
  - 重启。
  - 崩溃后读取日志尾部便于诊断。
- 开机启动：
  - 全局托管开启时，应用启动会恢复全部隧道。
  - “随 Fonu 自动启动”开关保留为隧道级选项。
- 全局总开关：
  - 开启时启动全部隧道。
  - 关闭时停止全部隧道。
- 凭据刷新：
  - 重新获取隧道 Token。
  - 更新本地 `creds.json`。
  - 运行中时自动重启。

---

## 五、出口抽象与反向代理规则

实现文件：

- `internal/proxy/proxy.go`
- `internal/service/proxy.go`
- `internal/cloudflared/service.go`

迁移：

- `migrations/035_proxy_exits.sql`

已实现：

- 反向代理规则增加出口字段：
  - `local`
  - `cloudflare`
- 支持同时选择：
  - 本机 Nginx + Cloudflare。
- Cloudflare 出口增加目标隧道：
  - `cf_tunnel_id`
- 规则变化自动同步：
  - 新增规则。
  - 修改规则。
  - 删除规则。
  - 启停规则。
- 托管隧道 ingress 从规则派生：
  - 每条域名使用自己的回源地址。
  - 仅同步启用的 Cloudflare 出口规则。
- 仅 Cloudflare 出口的规则不生成无用的本机 Nginx server。
- 规则保存成功但 Cloudflare 同步失败时：
  - 本地规则仍保存。
  - 返回 `sync_warning`。
  - 前端显示警告而不是把整次保存判为失败。

---

## 六、路由与 ingress

实现文件：

- `internal/cloudflared/routes.go`
- `internal/cloudflared/config.go`

已实现：

- 手工隧道路由来自本地 `config.yml`。
- 托管隧道路由来自反向代理规则。
- 路由列表支持：
  - 添加。
  - 编辑。
  - 删除。
  - 上移 / 下移。
  - 复制。
  - 源服务连通性测试。
- 保存路由时：
  - 重新生成本地 `config.yml`。
  - 覆盖 Cloudflare 云端 ingress。
  - 同步 DNS。
  - 运行中时重启 cloudflared。
- 末条 catch-all 固定为：
  ```text
  http_status:404
  ```
- 通配路径处理：
  - UI 显示 `*`。
  - 发送到 Cloudflare 时省略 `path`。
  - 避免 `*` 被当作非法正则。
- 支持正则 path 的后端字段，但当前页面主要使用通配路径。

### P0：接入与调和

- Cloudflare 页面新增“接入向导”。
- 接入预检检查：
  - cloudflared 二进制。
  - Account ID。
  - API Token。
  - Zone 权限。
- 隧道列表新增“调和”列：
  - 已同步。
  - 有漂移。
  - 手工隧道。
- 有漂移时可一键修复：
  - 以本地规则覆盖 Cloudflare 云端配置。

---

## 七、DNS 管理

实现文件：

- `internal/cloudflared/cloudflare_api.go`
- `internal/cloudflared/service.go`
- `internal/cloudflared/store.go`

迁移：

- `migrations/035_proxy_exits.sql`

已实现：

- 自动创建 CNAME：
  ```text
  <hostname> -> <tunnel-id>.cfargotunnel.com
  ```
- 开启 Cloudflare 代理（橙云）。
- 记录 Fonu 创建或接管的 DNS 记录：
  - `cf_dns_records`
- 删除域名或移除出口时：
  - 只删除 Fonu 自己接管的记录。
- 不覆盖指向其他目标的用户 DNS 记录。
- 域名从一条隧道迁移到另一条隧道时支持接管迁移。

---

## 八、状态、指标与日志

实现文件：

- `internal/cloudflared/manager.go`
- `internal/cloudflared/health.go`

已实现：

- 从 cloudflared metrics 端口读取：
  - HA 连接数。
  - 延迟。
  - 上下行速率。
  - 传输协议。
- 隧道列表实时状态。
- 配置页指标卡。
- 日志中心新增“Cloudflare 隧道”页签：
  - 选择隧道。
  - 查看 200 / 500 / 2000 行日志。
  - 关键词筛选。

---

## 九、健康巡检、通知与状态页

迁移：

- `migrations/036_cf_health.sql`

实现文件：

- `internal/cloudflared/health.go`
- `internal/monitor/monitor.go`
- `internal/app/monitor_source.go`

已实现：

- 记录 Cloudflare 域名健康状态变化：
  - `up`
  - `down`
  - `unknown`
- 24 小时可用率。
- 不可用区间。
- 巡检复用现有：
  - 路由健康开关。
  - 隧道断开通知开关。
  - 连续失败阈值。
- 连续不通和恢复复用通知事件 `tunnel_down`。
- 公开状态页合并：
  - FRP 路由。
  - Cloudflare 路由。
- 状态页展示每条路由的不可用区间：
  - 开始时间。
  - 持续分钟数。
  - 原因。
- 状态页只显示域名、来源、状态和可用率，不泄露隧道 ID 与内网地址。

---

## 十、仪表盘

实现文件：

- `internal/api/status_handler.go`
- `web/src/views/DashboardView.vue`

已实现：

- `/api/status` 增加：
  - Cloudflare 隧道总数。
  - 在线数。
  - 状态。
  - 摘要消息。
- 仪表盘健康列表增加“Cloudflare 隧道”条目。

---

## 十一、备份与恢复

实现文件：

- `internal/backup/service.go`

已实现：

- 备份内容包含：
  - `fonu.db`
  - `certs`
  - `nginx`
  - `cloudflared/tunnels`
- `cloudflared/tunnels` 包含：
  - `config.yml`
  - `creds.json`
- 数据库中的 `cf_tunnels`、`cf_settings`、`cf_dns_records` 随 `fonu.db` 恢复。
- 恢复后仍建议重启 Fonu，由启动流程恢复全部隧道。

---

## 十二、漂移检测与诊断

实现文件：

- `internal/cloudflared/operations.go`

已实现：

### 12.1 漂移检测

- 比较本地规则派生 ingress 与 Cloudflare 云端 ingress。
- 检出：
  - 本地有、云端缺失。
  - 云端有、本地已移除。
  - 回源地址不一致。
- 支持“以本地覆盖”：
  ```text
  POST /api/cloudflare/tunnels/{id}/sync
  ```
- 当前未实现“以云端为准拉取”。

### 12.2 一键诊断

检查项：

- cloudflared 二进制。
- 本地 `config.yml`。
- 隧道进程状态。
- Cloudflare API 与云端配置可访问性。
- Fonu 接管的 DNS 记录。
- Cloudflare edge region1。
- Cloudflare edge region2。

---

## 十三、NAS 场景能力

实现文件：

- `internal/discovery/service.go`
- `web/src/views/ProxyView.vue`

已实现：

- 服务发现目录包含：
  - 飞牛 fnOS
  - AList
  - Jellyfin
  - PhotoPrism
  - qBittorrent
  - Portainer
  - Home Assistant
  - Immich
  - AdGuard Home
  - Synology DSM
- 反向代理页“扫描服务”打开发现列表。
- 点击“发布”会：
  - 预填服务名称。
  - 预填内网 upstream。
  - 若已有托管隧道，自动选择 Cloudflare 出口。
  - 打开规则编辑表单等待填写域名。

当前不是完全一键创建，仍需要用户填写域名并确认保存。

---

## 十四、安全与保护提示

已实现：

- Cloudflare Tunnel 条款提示：
  - 不适合持续大文件/视频分发。
- 仅 Cloudflare 出口时：
  - 提示无需申请本机 HTTPS 证书。
- Access 只读检测：
  - 检测域名是否已有 Access 应用。
  - 不代配 Access 策略。
- API Token 权限提示：
  - `Account: Cloudflare Tunnel: Edit`
  - `Zone: Zone: Read`
  - `Zone: DNS: Edit`
  - Zone Resources 必须包含待发布根域名。

---

## 十五、当前 API

### 隧道

```text
GET    /api/cloudflare/tunnels
POST   /api/cloudflare/tunnels
GET    /api/cloudflare/tunnels/{id}
PUT    /api/cloudflare/tunnels/{id}
DELETE /api/cloudflare/tunnels/{id}
POST   /api/cloudflare/tunnels/{id}/start
POST   /api/cloudflare/tunnels/{id}/stop
POST   /api/cloudflare/tunnels/{id}/restart
POST   /api/cloudflare/tunnels/{id}/credentials/refresh
GET    /api/cloudflare/tunnels/{id}/status
GET    /api/cloudflare/tunnels/{id}/logs
GET    /api/cloudflare/tunnels/{id}/config
GET    /api/cloudflare/tunnels/{id}/routes
PUT    /api/cloudflare/tunnels/{id}/routes
POST   /api/cloudflare/routes/test
GET    /api/cloudflare/tunnels/{id}/drift
POST   /api/cloudflare/tunnels/{id}/sync
GET    /api/cloudflare/tunnels/{id}/diagnostics
```

### 二进制与配置

```text
GET    /api/cloudflare/binary
POST   /api/cloudflare/binary/download
GET    /api/cloudflare/binary/latest
POST   /api/cloudflare/binary/rollback
GET    /api/cloudflare/settings
PUT    /api/cloudflare/settings
POST   /api/cloudflare/settings/test
POST   /api/cloudflare/enabled
GET    /api/cloudflare/preflight
```

### Zone、候选与 Access

```text
GET    /api/cloudflare/ingress-candidates
GET    /api/cloudflare/zones
GET    /api/cloudflare/access-status
```

---

## 十六、数据表

### `cf_tunnels`

保存隧道基础信息：

- 名称。
- 模式。
- Tunnel ID。
- Account Tag。
- 加密后的凭据。
- 配置、凭据、日志路径。
- 网络设置。
- 运行状态。
- metrics 端口。
- 自动启动选项。
- 是否自动托管。

### `cf_settings`

保存应用级配置：

- Account ID。
- 加密后的默认 Token。
- 加密后的 API Token。
- 下载镜像。
- 默认网络设置。
- 全局托管开关。

### `cf_dns_records`

保存 Fonu 接管的 DNS 记录：

- 本地隧道 ID。
- Zone ID。
- DNS Record ID。
- Hostname。

### `cf_health_events`

保存 Cloudflare 域名健康状态变化：

- 隧道 ID。
- 域名。
- 状态。
- 原因。
- 详情。
- 时间。

### `proxy_rules` 扩展

- `exits_json`
- `cf_tunnel_id`

---

## 十七、当前未完成或仅部分实现

以下能力尚未完成，不能按“已实现”宣传：

1. 完整审计日志
   - 尚未记录“谁在何时修改了规则、出口、凭据和设置”。

2. 云端配置拉取
   - 漂移检测支持“本地覆盖云端”。
   - 尚不支持“云端覆盖本地”。

3. 完整无感令牌轮换
   - 当前是“刷新/重新获取隧道凭据”。
   - 尚未实现创建新隧道后无缝迁移全部规则。

4. Access 策略管理
   - 只做只读检测。
   - 不创建、不修改 Access 应用。

5. 完全一键服务发布
   - 当前是扫描、预填、打开表单。
   - 仍需要用户填写域名并确认保存。

6. 正则路径 UI
   - 后端路由模型支持 `path`。
   - 当前页面主要使用通配路径，尚未提供独立的 path 高级编辑控件。

7. 多 Cloudflare 账号
   - 当前应用配置只支持一个 Account ID + API Token。
   - `cert.pem` / 多账号模式尚未实现。

---

## 十八、验证基线

当前实现覆盖以下验证：

```text
go test ./... -count=1
go vet ./...
cd web && npm run typecheck
cd web && npm test -- --run
cd web && npm run build
```

新增或重点测试：

- 出口默认值、持久化与过滤。
- Cloudflare 按规则生成 ingress。
- Cloudflare 健康状态与可用率计算。
- 手工 config 路由解析。
- 通配 path 不发送非法正则。
- DNS 记录归属。
- 备份包含 cloudflared 隧道配置。
- 漂移比较。
