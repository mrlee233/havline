# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### 变更

- **品牌全量切换为 Havline**：项目显示名、Go module、命令目录、二进制、环境变量、数据库、Cookie、Docker/Compose、Agent、Updater、脚本和当前文档统一切换到 `Havline / havline / HAVLINE_`
- **AgentVersion 提升到 0.15.0**：Agent 安装路径、配置目录、数据目录、systemd unit、Nginx 托管标记和 API 服务标识全部切换到 `havline-agent`
- **历史文档保留原品牌名**：`CHANGELOG.md` 历史版本、旧品牌评估文档和带日期的历史分析报告保留原始名称，作为项目沿革和 GPL 来源记录

## [v1.9.0] - 2026-10-01

### 新增

- **Cloudflare 隧道出口抽象**：反向代理规则新增 `local` / `cloudflare` 出口选择，支持同时使用本机 Nginx 与 Cloudflare 隧道；托管隧道从服务发布规则自动派生 ingress，规则新增、修改、删除、启停时自动同步 Cloudflare 云端与 DNS
- **Cloudflare 独立配置页**：隧道「编辑」改为独立普通页面 `/cloudflare/:id/config`，包含指标、基础配置、拓扑图、路由列表和运行控制；基础配置通过按钮打开弹窗，不再直接堆叠在页面中
- **Cloudflare 路由管理增强**：新增路由添加、编辑、删除、上移、下移、复制和源服务 TCP 连通性测试；通配路径 `*` 只在界面显示，不再作为非法正则发送给 Cloudflare
- **Cloudflare 接入向导与预检**：新增 cloudflared、Account ID、API Token、Zone 权限检查，并在 Cloudflare 页面提供接入向导入口
- **Cloudflare 调和中心**：隧道列表显示“调和”状态；检测本地规则与云端 ingress 漂移，支持以本地规则一键覆盖修复
- **Cloudflare 运维能力**：新增隧道健康巡检、24 小时可用率、断连/恢复通知、状态页事故记录、仪表盘摘要、日志中心分页、漂移检测、一键诊断、Access 只读检测、Zone 列表和 DNS 记录归属管理
- **cloudflared 二进制管理**：支持查询最新版本、指定版本下载、保留上一版回滚；Cloudflare 页面显示当前版本并支持检查更新
- **NAS 服务发现增强**：服务发现目录新增 Home Assistant、Immich、AdGuard Home、Synology DSM 等常见 NAS 应用；扫描后可按 Cloudflare 出口预填服务发布规则
- **备份恢复增强**：备份归档加入 `cloudflared/tunnels`，保留隧道 `config.yml`、`creds.json` 与相关数据库记录
- **Cloudflare 拓扑图**：按“域名 → 出口 → 隧道 → 服务”展示服务发布链路，支持点阵画布、节点和曲线连接

### 修复

- **Cloudflare 全局启动策略**：全局托管开启或 Fonu 启动时，默认启动全部隧道；关闭时停止全部隧道并阻止应用重启自动拉起
- **Cloudflare 路由保存提示**：规则本体保存成功但 Cloudflare 同步失败时返回 `sync_warning` 并在前端提示，不再把已保存的规则判为完全失败
- **Cloudflare metrics 端口识别**：兼容 `Starting metrics server...` 大小写日志格式，修复 metrics 端口、连接数、传输协议无法采集的问题
- **Cloudflare DNS 安全清理**：只删除 Fonu 接管或创建的 DNS 记录，不覆盖用户已有的其他目标 CNAME
- **Cloudflare 隧道删除保护**：仍被反向代理规则引用的托管隧道不允许删除或切换为手工隧道

### 测试

- 新增 Cloudflare 出口默认值与持久化、托管规则生成 ingress、健康状态与可用率、路由解析、通配 path、DNS 记录归属、漂移比较、备份包含隧道配置等测试
- `go test ./...`、`go vet ./...`、`npm run typecheck`、`npm test -- --run`、`npm run build` 通过

### 变更

- 版本号提升到 1.9.0；`AgentVersion` 保持 **0.14.1**，本次无 Agent 行为变化

## [v1.8.0] - 2026-09-30

### 新增

- **Cloudflare 隧道全局托管开关**：页面顶部新增「全局托管」开关：
  - 关闭时立即停止所有隧道进程，应用重启后不再自动拉起；单条隧道的「启动」按钮同时禁用
  - 开启时只恢复标记了 `auto_start` 的隧道，其余保持停止
  - 只影响本机 cloudflared 进程托管，不删除本地配置、凭据、云端隧道或 DNS 记录
- 新增 `cf_settings.enabled` 字段（迁移 `034`）与 `POST /api/cloudflare/enabled` 接口

### 变更

- 版本号提升到 1.8.0；`AgentVersion` 保持 **0.14.1**

### 测试

- 新增「全局停用时拒绝启动」单元测试
- `go test ./...`、`go vet ./...`、`npm run typecheck`、`npm test`、`npm run build` 通过

## [v1.7.0] - 2026-09-28

### 新增

- **自动创建 Cloudflare DNS 记录**：创建隧道时，Fonu 会为每个暴露域名查找所属 Cloudflare Zone（从 hostname 逐级向上）并幂等创建 / 更新 CNAME：
  - 目标为 `<tunnel-id>.cfargotunnel.com`
  - 自动开启代理（橙云）
  - 同名 CNAME 已存在时更新而不是重复创建
  - 编辑隧道时同步 DNS；创建失败只在页面提示警告，不影响隧道本身
- API Token 权限说明更新为：`Account: Cloudflare Tunnel: Edit`、`Zone: Zone: Read`、`Zone: DNS: Edit`

### 变更

- 版本号提升到 1.7.0；`AgentVersion` 保持 **0.14.1**

### 测试

- `go test ./...`、`go vet ./...`、`npm run typecheck`、`npm test`、`npm run build` 通过

## [v1.6.9] - 2026-09-28

### 修复

- **测试连接未使用当前输入的 Account ID**：测试连接原先只读取数据库中已保存的 Account ID，未保存直接点测试会误报「请先填写 Account ID」。现在前端会把输入框里的 Account ID 与 API Token 一起提交，后端优先使用输入值，留空时才回退到已保存配置

### 变更

- 版本号提升到 1.6.9；`AgentVersion` 保持 **0.14.1**

### 测试

- `go test ./...`、`go vet ./...`、`npm run typecheck`、`npm test`、`npm run build` 通过

## [v1.6.8] - 2026-09-28

### 变更

- **Cloudflare 凭据模型改为 Account ID + API Token**：应用配置不再要求 Tunnel Token；Fonu 用 Account ID + API Token 调用 Cloudflare API：
  - 「测试连接」调用 `GET /accounts/{account_id}/tokens/verify`
  - 新建隧道时自动创建 `config_src=local` 的命名隧道、获取凭据、生成本地 `config.yml` 与 `creds.json`
  - 删除隧道时同步删除云端隧道；编辑时同步推送 ingress
- 隧道弹窗去掉 Token 与模式选择，只保留名称、暴露域名、回源 service 与自动启动
- 新增 `cf_settings.account_id` 字段（迁移 `033`）

### 变更

- 版本号提升到 1.6.8；`AgentVersion` 保持 **0.14.1**

### 测试

- `go test ./...`、`go vet ./...`、`npm run typecheck`、`npm test`、`npm run build` 通过

## [v1.6.7] - 2026-09-28

### 新增

- **API Token 连接验证**：应用配置的 Token 区域拆分为两个输入：
  - 隧道 Token（Tunnel Token）：用于 cloudflared 运行
  - API Token（`cfat_...`）：用于连接验证，调用 Cloudflare 官方 `GET /user/tokens/verify` 接口
- 「测试连接」优先使用 API Token；未配置 API Token 时回退到隧道 Token 的临时连接测试；输入以 `cfat_` 开头时也会自动走 API 验证
- 新增 `cf_settings.api_token_enc` 字段（迁移 `032`），API Token 独立加密存储与掩码回显

### 变更

- 版本号提升到 1.6.7；`AgentVersion` 保持 **0.14.1**

### 测试

- `go test ./...`、`go vet ./...`、`npm run typecheck`、`npm test`、`npm run build` 通过

## [v1.6.6] - 2026-09-28

### 修复

- **Token 解码兼容 URL-safe base64**：Cloudflare 面板生成的 Tunnel Token 使用 URL-safe base64（含 `-` / `_`），原先只按标准 base64 解码，会报 `illegal base64 data at input byte N` 并返回 502。现在同时兼容标准 / URL-safe 与有无填充四种形式，失败时给出更明确的提示

### 变更

- 版本号提升到 1.6.6；`AgentVersion` 保持 **0.14.1**

### 测试

- 新增 URL-safe 与无填充 token 解码测试

## [v1.6.5] - 2026-09-28

### 新增

- **Token 配置连接入口**：应用配置的 Token 区域新增两个入口：
  - 「测试连接」：后端用 cloudflared 临时启动一次连接，最多等待 25 秒，确认 token 能注册到 Cloudflare 边缘；成功后显示隧道 ID，失败给出明确原因
  - 「获取 Token」：直达 Cloudflare Zero Trust 面板
- 新增 `POST /api/cloudflare/settings/test` 接口；请求体 token 留空时使用已保存的默认 token

### 变更

- 版本号提升到 1.6.5；`AgentVersion` 保持 **0.14.1**

### 测试

- `go test ./...`、`go vet ./...`、`npm run typecheck`、`npm test`、`npm run build` 通过

## [v1.6.4] - 2026-09-28

### 变更

- **新建 / 编辑隧道弹窗去重**：移除与应用配置重复的 Token 输入与网络调优字段；Token、传输协议、边缘地址族、HA 连接数、代理模式统一在「应用配置」中维护，保存时自动应用到新建或编辑的隧道
- 新建隧道前若未配置默认 Token，弹窗内直接提示并阻止提交；编辑已有隧道不受影响
- 隧道弹窗现在只保留名称、模式、暴露域名、回源 service 与自动启动

### 变更

- 版本号提升到 1.6.4；`AgentVersion` 保持 **0.14.1**

### 测试

- `npm run typecheck`、`npm test`、`npm run build` 通过

## [v1.6.3] - 2026-09-28

### 新增

- **Cloudflare 应用配置**：「cloudflared 二进制」弹窗升级为「应用配置」，包含三块：
  - 程序安装：当前版本、下载镜像、下载并安装
  - Token 配置：保存模块级默认 token，掩码回显；新建隧道时可以留空，直接使用默认 token
  - 默认网络设置：传输协议、边缘地址族、HA 连接数、代理模式，作为新建隧道的默认值
- 新增 `cf_settings` 表（迁移 `031`）与 `GET / PUT /api/cloudflare/settings` 接口

### 变更

- 版本号提升到 1.6.3；`AgentVersion` 保持 **0.14.1**

### 测试

- 新增应用配置与默认 token 的端到端单元测试
- `go test ./...`、`go vet ./...`、`npm run typecheck`、`npm test`、`npm run build` 通过

## [v1.6.2] - 2026-09-28

### 修复

- **Cloudflare 隧道弹窗宽度**：`n-modal` 的宽度原先写在 scoped class 上，没有传到 naive-ui 的 modal 根节点，部分视口下弹窗被拉成 100%。现在改为与其他页面一致的 `:style` 写法：编辑弹窗 `min(720px, 96vw)`、详情弹窗 `min(760px, 96vw)`、二进制弹窗 `min(560px, 94vw)`

### 变更

- 版本号提升到 1.6.2；`AgentVersion` 保持 **0.14.1**

### 测试

- `npm run typecheck`、`npm run build` 通过

## [v1.6.1] - 2026-09-28

### 修复

- **Cloudflare 隧道弹窗完善**：编辑弹窗回填已配置的暴露域名与回源 service（后端从 `config.yml` 解析）；按「基本信息 / 暴露规则 / 网络调优」分组，增加模式说明；云端规则模式隐藏本地域名配置并提示规则由 Cloudflare 面板管理
- **详情弹窗**：改为状态卡 + `config.yml` / 日志分页，日志每 3 秒自动刷新，支持一键复制配置，刷新按钮带加载状态
- **二进制弹窗**：显示当前版本与目标文件；镜像列表为空时回退官方源
- **保存校验**：提交前校验名称、token 与本地规则模式的暴露域名

### 变更

- 版本号提升到 1.6.1；`AgentVersion` 保持 **0.14.1**，本次无 Agent 行为变化

### 测试

- `go test ./...`、`go vet ./...`、`npm run typecheck`、`npm test`、`npm run build` 通过

## [v1.6.0] - 2026-09-28

### 新增

- **Cloudflare 隧道独立模块（阶段 1–2）**：新增 `internal/cloudflared` 包，覆盖 token 解码、`creds.json` / `config.yml` 生成、ingress 生成器、SQLite 存储、cloudflared 二进制下载（镜像表 + 大小与可执行校验）、子进程托管、日志追加、metrics 端口发现与健康抓取
- 新增 `cf_tunnels` 表与 `/api/cloudflare/*` 接口：列表、创建、更新、删除、启动、停止、重启、状态、日志、配置预览、二进制信息与下载、ingress 候选域名
- 前端新增「Cloudflare 隧道」页面：隧道列表、创建 / 编辑、启停、配置预览、日志、二进制下载与网络调优设置
- 支持 `token-local`（Fonu 生成本地 ingress）与 `token-remote`（Cloudflare 面板管理规则）两种模式；界面显示模式差异
- 网络调优默认值：`http2` / IPv4 / HA 2 / 跟随系统代理；ingress 锁定 catch-all、`127.0.0.1` 归一化与稳定排序三条不变式

### 变更

- 版本号提升到 1.6.0；`AgentVersion` 保持 **0.14.1**，本次无 Agent 行为变化

### 测试

- `internal/cloudflared`：token 解码、ingress 不变式、config.yml、metrics 解析、启动参数、镜像 URL、代理禁用单元测试
- `go test ./...`、`go vet ./...`、`npm run typecheck`、`npm test`、`npm run build` 通过

### 未实施

- 账号模式（`cert.pem`）与云端 ingress 的 fetch / push
- 边缘 IP 优选、快速隧道、自动 DNS

## [v1.5.4] - 2026-09-28

### 变更

- **P1-5 统一前端轮询组合式函数**：`useVisibilityPolling` 重写为支持 `enabled` / `immediate` / `onError` 选项，加入 in-flight 互斥，返回 `start` / `stop` / `busy` / `lastError`；核心逻辑抽成 `createVisibilityPoller`，可在无 DOM 环境下测试
- Dashboard、Proxy、Logs、Settings、Frp、UpdateModal、Agent 的轮询全部迁移到组合式函数；仅 AppLayout 时钟保留原生计时器
- 慢接口不再堆积：上一轮未结束时跳过下一轮；页面隐藏时暂停，回到前台立即补一次

### 测试

- 新增 6 项组合式函数单元测试：隐藏不执行、回到前台补一次、in-flight 互斥、`enabled=false`、卸载清理、`onError` 后继续轮询
- `npm run typecheck`、`npm test`（25 项）、`npm run build` 通过

### 变更

- 版本号提升到 1.5.4；`AgentVersion` 保持 **0.14.1**，本次无 Agent 行为变化

## [v1.5.3] - 2026-09-28

### 新增

- **P1-4 路由鉴权覆盖测试**：API 路由收敛为可遍历路由表，新增 `internal/api/router_auth_test.go` 遍历 100+ 受保护路由，匿名请求必须返回 401，公开路由不被鉴权误拦；Agent 路由收敛为 `agentRoutes` 声明，新增 `internal/agent/router_auth_test.go` 遍历全部 `/api/v1/*` 验证无 Token 必须 401
- **配置回滚测试**：本机新增 `internal/nginx/versions_test.go`，覆盖版本列表 / 读取、非法版本名拒绝路径上跳、语法校验失败不覆盖当前配置、成功回滚保留当前版本；Agent 新增写 vhost 前保存版本、校验失败恢复上一可用版本、首次部署失败删除新文件、回滚后执行 reload 的测试
- **Agent 版本兼容测试**：新增 `internal/frp/agent_versions_compat_test.go`，覆盖 404 / 405 判定端点缺失并提示最低版本与「升级 Agent」、400 / 500 不误判、状态接口 `agent_version` 解析
- **前端版本字段降级测试**：新增 `agentVersionText` 工具与测试，旧 Agent 缺少 `agent_version` 时显示占位符而不是空白

### 变更

- 版本号提升到 1.5.3；`AgentVersion` 保持 **0.14.1**，本次无 Agent 行为变化
- Agent 的 nginx 校验与 reload 增加测试注入钩子；生产路径仍调用真实 nginx，行为不变

### 测试

- `go test ./...`、`go vet ./...` 通过
- `npm run typecheck`、`npm test`（19 项）、`npm run build` 通过

## [v1.5.2] - 2026-09-28

### 修复

- **升级 Agent 不再强制切回 SSH 隧道**：HTTPS 直连模式下点击「升级 Agent」原先会无条件建立隧道并把 transport 改回 `tunnel`，用户升级后需要重新切换 HTTPS。现在升级前是 `https-pin` 时会保持 HTTPS 直连，只更新 Token 密文并重新验证连通
- **清理隧道模式下残留的 HTTPS 反代**：旧升级流程切回隧道时没有关闭 Agent 上的 `fonuagent-https.conf`，导致 7443 等端口继续暴露。现在从 HTTPS 迁移到隧道、应用启动恢复隧道、手动「重启隧道」时都会通过隧道调用 `DisableHTTPS` 清理残留

### 变更

- 版本号提升到 1.5.2；`AgentVersion` 保持 **0.14.1**，本次是中央端修复，不需要重新升级 Agent

### 测试

- `internal/frp` 现有单元测试与编译校验通过；升级分支依赖真实 SSH/Agent 环境，未新增本地集成测试

## [v1.5.1] - 2026-09-28

### 变更

- **P1-1 共享 Nginx 模板包**：新增纯生成包 `internal/nginxtmpl`，统一 TLS 指令、安全响应头、allow / deny、WebSocket 头、限流指令与 zone 命名；本机 `internal/nginx` 与公网 Agent 均改为调用共享函数，重复模板实现已删除
- **安全响应头对齐**：两端统一输出 HSTS、`X-Content-Type-Options`、`X-Frame-Options`、`Referrer-Policy`，本机此前缺少 `Referrer-Policy`
- **访问控制顺序统一**：统一为黑名单 `deny` → 仅大陆规则 → 白名单 `allow` → `deny all`，黑名单优先匹配
- **TLS 指令显式化**：两端统一显式输出 `ssl_protocols TLSv1.2 TLSv1.3` 或 `TLSv1.3`
- 版本号提升到 1.5.1；`AgentVersion` 0.14.0 → **0.14.1**，内嵌 amd64 / arm64 二进制已重编；VPS 上需点一次「升级 Agent」才会生效

### 测试

- `internal/nginxtmpl`：新增共享函数的 TLS、安全头、访问控制、限流、zone 与 WebSocket 特征化测试
- `internal/nginx`：本机生成测试继续通过；`internal/agent`：新增 vhost 顺序与共享语义测试

## [v1.5.0] - 2026-09-28

### 新增

- **HTTPS 直连端口自动选择**：Agent 在 `7443` / `8443` / `9443` 中逐个尝试可用端口，每轮都会写配置、reload 并做本机证书自检；全部失败时返回每个端口的具体原因。显式传入端口时仍按指定值执行，`443` 可通过接口显式指定
- **证书自动轮换**：证书有效期 365 天，剩余 30 天时后台自动生成新证书并通过当前连接安装；过渡期 7 天内客户端同时接受新旧 pin，避免轮换瞬间连接失败。旧记录缺少到期信息时，先从 Agent 读取当前证书到期时间再决定是否轮换
- **主机防火墙辅助**：Agent 新增防火墙检测与显式放行接口，支持 `ufw` / `firewalld` 自动放行；`nftables` / `iptables` 只检测和提示，不自动修改规则。Agent 页增加「检测主机防火墙 / 放行主机防火墙」
- **隧道后台保活与重连**：隧道启动后每 30 秒保活，断线按 1 秒到 60 秒指数退避重建，不再只在有流量时才尝试重连
- **传输详细状态接口**：`GET /api/frpmulti/servers/{id}/agent/transport` 返回 transport、状态、端口、pin、证书到期时间与防火墙状态
- **审计记录**：transport 切换、证书轮换、防火墙放行写入 AUDIT 日志，包含 admin_id、server_id 与来源地址
- **证书 SAN 校验**：pin 校验同时校验证书 SAN 与连接主机一致

### 变更

- 版本号提升到 1.5.0；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 0.13.3 → **0.14.0**，内嵌 amd64 / arm64 二进制已重编；VPS 上需点一次「升级 Agent」才会生效
- Agent 页轮询改用 `useVisibilityPolling`，页面隐藏时暂停

### 测试

- `internal/frp`：新增多 pin 过渡、SAN 拒绝、端口解析与轮换兼容测试
- `internal/agent`：新增 ufw 解析与候选端口测试
- `internal/frp`：隧道退避策略单元测试

## [v1.4.6] - 2026-09-28

### 修复

- **HTTPS 直连证书指纹不匹配可定位**：nginx 配置校验通过、但公网 `7443` 返回的证书与刚写入的不一致时，原来只提示“证书指纹不匹配”，无法判断是 nginx 未生效还是端口被其他服务接管。现在 HTTPS server 使用 `default_server` 绑定，Agent 会在 reload 后从本机 `127.0.0.1:<端口>` 握手自检并重试 5 秒；指纹不一致时回滚，并回报实际指纹与 `ss -tlnp` 端口占用进程
- **公网链路错误提示**：主程序在 Agent 本机自检通过、但公网地址校验失败时，会明确提示检查 `7443` 的端口转发、防火墙或其他服务接管

### 变更

- 版本号提升到 1.4.6；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 0.13.2 → **0.13.3**，内嵌 amd64 / arm64 二进制已重编；VPS 上需重新点一次「升级 Agent」才会生效

### 测试

- `internal/agent`：新增证书 PEM 指纹计算测试

## [v1.4.5] - 2026-09-28

### 修复

- **Agent 探测阶段无法自愈 nginx 残留目录**：0.13.1 的迁移只在配置目录已知或 `nginx -t` 校验前执行；当 `nginx -T` 在探测目录阶段就因 `.versions` 目录失败时，Agent 仍返回 `nginx -T 执行失败: exit status 1`。现在会先从 `nginx -T` 报错文本解析出 `<配置>.versions` 路径并迁移，再重试一次；安装 / 升级 Agent 的脚本也会先把 `sites-enabled`、`conf.d` 下的旧 `.versions` 目录移出 include 范围

### 变更

- 版本号提升到 1.4.5；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 0.13.1 → **0.13.2**，内嵌 amd64 / arm64 二进制已重编；VPS 上需重新点一次「升级 Agent」才会生效

### 测试

- `internal/agent`：新增从 nginx 报错文本解析并迁移旧版本目录的测试
- `scripts/agent-install.sh`：`bash -n` 语法校验通过

## [v1.4.4] - 2026-09-28

### 修复

- **Agent nginx 版本目录被误加载**：切换 HTTPS 直连时，配置备份会写到 `<配置文件>.versions` 目录；该目录位于 `sites-enabled` 时会被 nginx 的 `include .../sites-enabled/*` 当成配置文件读取，导致 `nginx -t` 报 `pread() ".../fonuagent-https.conf.versions" failed (21: Is a directory)`。现在版本目录改为隐藏目录 `.<文件名>.versions`，`include *` 不再匹配；旧目录会在读取、备份或 `nginx -t` 前自动迁移，历史版本内容保留

### 变更

- 版本号提升到 1.4.4；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 0.13.0 → **0.13.1**，内嵌 amd64 / arm64 二进制已重编；VPS 上需点一次「升级 Agent」才会生效

### 测试

- `internal/fsutil`：新增隐藏版本目录与旧目录迁移测试
- `internal/agent`：新增 `migrateLegacyVersionDirs` 迁移测试

## [v1.4.3] - 2026-09-28

### 修复

- **HTTPS 直连旧地址自愈**：从 SSH 隧道切换到「HTTPS + 证书固定」后，历史实例的 `agent_url` 可能仍为已失效的 `http://127.0.0.1:17701`，隧道关闭后直接报 `connection refused`。现在读取 HTTPS transport 时会依据 `ssh_host` 与 `agent_listen_addr` 自动重建 `https://<host>:<port>`，并在地址变化时回写数据库；切回 SSH 隧道也使用同一重建逻辑，无需手工修改地址

### 变更

- 版本号提升到 1.4.3；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.13.0

### 测试

- `internal/frp`：新增 `TestAgentHTTPSBaseURL`，覆盖旧隧道地址重建、已是 HTTPS 地址保持、默认端口回退与缺少 SSH 主机的情况

## [v1.4.2] - 2026-09-28

### 修复

- **切换 HTTPS 后 Agent URL 未更新**：启用 HTTPS 直连时只更新了 transport 和证书 pin，数据库里的 `agent_url` 仍保留旧的 `http://127.0.0.1:17701`。隧道关闭后仍访问旧本地地址，导致 `connect: connection refused`。现在 transport、URL、端口和 pin 同批更新，并在切换后清理旧 AgentClient 缓存
- **切回 SSH 隧道时同步恢复本地 URL**：恢复隧道会重新写入 `http://127.0.0.1:<localPort>`，避免残留 HTTPS 地址影响后续状态读取

### 变更

- 版本号提升到 1.4.2；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.13.0

### 测试

- `internal/frp`：transport 切换与缓存失效编译验证

## [v1.4.1] - 2026-09-28

### 修复

- **SSH 隧道端口分配竞态**：原来先探测端口再关闭 listener，随后才真正启动隧道，两个安装动作或残留占用可能在两步之间抢到同一端口，报 `bind: Only one usage of each socket address`。现在改为分配端口时直接持有 listener 并交给隧道使用；17700 被占用时自动尝试后续端口

### 变更

- 版本号提升到 1.4.1；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.13.0

### 测试

- `internal/frp`：隧道 listener 复用、端口占用与释放回归测试

## [v1.4.0] - 2026-09-28

### 新增

- **Agent 默认监听回环**：`fonu-agent` 默认改为 `127.0.0.1:7700`；监听非回环地址必须显式设置 `FONU_AGENT_ALLOW_REMOTE=1`，状态接口新增监听地址、监听范围和传输安全字段
- **SSH 隧道自动接入**：中央 Fonu 安装或升级 Agent 后自动分配 `17700-18700` 范围内的本地回环端口，通过现有 SSH 凭据建立隧道访问 VPS 的 `127.0.0.1:7700`，无需新增公网端口、证书或防火墙规则
- **隧道生命周期管理**：应用启动后自动恢复 tunnel transport；应用退出时统一关闭；每个服务端支持单独的「重启隧道」操作
- **旧公网 HTTP 自动迁移**：旧 Agent 点击「升级 Agent」后安装脚本会备份旧 `agent.env`、改写为回环监听并自动建立隧道；隧道验证失败时恢复旧监听配置
- **可选 HTTPS 直连 + 证书指纹固定**：安装后可从 SSH 隧道切换到 HTTPS 直连。Fonu 自动生成 ECDSA 自签名证书、通过现有安全连接下发，VPS Agent 自动写入独立 Nginx TLS 反代；后续连接按证书 SHA-256 pin 校验，不使用 `InsecureSkipVerify` 裸跳过
- **防火墙提示边界**：HTTPS 模式明确提示云安全组需放行 TCP 7443；Fonu 不自动修改云安全组，主机防火墙自动处理仍留在后续阶段

### 变更

- 数据库新增 Agent transport / 本地端口 / TLS pin / 监听地址 / 连接状态 / 连接错误字段（迁移 028）
- 前端「公网 Agent」页显示传输方式、隧道状态、本地入口和 Agent 监听地址；旧公网 HTTP 显示迁移警告
- 版本号提升到 1.4.0；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 0.12.9 → **0.13.0**，内嵌 amd64 / arm64 二进制已重编；VPS 上需升级 Agent

### 测试

- `internal/agent`：回环默认值与远程监听保护
- `internal/frp` / `internal/app`：隧道生命周期、transport 存储、证书生成与 pin 校验测试

## [v1.3.9] - 2026-09-28

### 修复

- **限流状态损坏时拒绝覆盖**：`readLimitState` 原来忽略 JSON 解析错误并返回空 map，保存任一规则时可能把其他域名的限流配置清空。现在文件不存在仍视为空状态，读取失败、空文件或 JSON 损坏则返回错误，`applyLimitState` / `clearLimitState` 会保留原文件与现有 Nginx zone，不再覆盖
- **一键升级宿主机目录自动发现**：当 Compose 标签里的 `working_dir` 是 `/workspace` 这类容器路径时，不再直接把它当宿主机目录；优先使用 `project.config_files` 推导真实目录，无法得到宿主机路径时明确报错，避免 helper 挂载 `/workspace:/workspace` 后找不到 Compose 文件

### 变更

- 版本号提升到 1.3.9；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 0.12.8 → **0.12.9**，内嵌 amd64 / arm64 二进制已重编；VPS 上需点一次「升级 Agent」才会生效

### 测试

- `internal/agent`：新增限流状态文件缺失、正常解析、空文件、截断 JSON、null 内容与失败后原文件不变的测试

## [v1.3.8] - 2026-09-28

### 新增

- **示例页面 v1.3.8 验证标识**：页面版本提示、「本次升级验证」区块、页面状态和验证标识同步更新为 `v1.3.8-example-page`，用于再次验证线上升级后的前端资源切换

### 变更

- 版本号提升到 1.3.8；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.12.8

### 测试

- 前端类型检查、单元测试与生产构建

## [v1.3.7] - 2026-09-28

### 新增

- **示例页面新增独立升级验证区块**：页面增加「本次升级验证」高亮区块，显示 `v1.3.7`，验证标识更新为 `v1.3.7-example-page`，用于直观确认升级后的前端资源已切换

### 修复

- **复用原 Compose 项目名重建容器**：updater 在通过 helper 容器执行 `docker compose` 时，先从现有 `fonu` 容器读取 `com.docker.compose.project` 标签，并显式传入同一个项目名，避免 Compose 将现有容器视为另一个项目后重新创建网络、数据卷并撞上 `/fonu` 容器名冲突。现有容器不是 Compose 管理时，updater 会拒绝升级并提示按 Compose 方式重新部署，避免误建空数据卷

### 变更

- 版本号提升到 1.3.7；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.12.8

### 测试

- 前端类型检查、单元测试与生产构建

## [v1.3.6] - 2026-09-28

### 修复

- **自动定位宿主机 Compose 目录**：当 updater 容器内的 `/workspace` 没有挂载到真实 Compose 目录时，不再直接失败。updater 会从 `fonu` 容器的 `com.docker.compose.project.working_dir` / `project.config_files` 标签读取宿主机路径，并通过临时 helper 容器挂载真实目录执行 `docker compose`。`FONU_UPDATE_HOST_DIR` 仍可作为显式覆盖
- 示例页验证标识更新为 `v1.3.6-example-page`

### 变更

- 版本号提升到 1.3.6；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.12.8

### 测试

- `internal/updater`：覆盖 Compose 文件直挂载与宿主机目录 helper 模式

## [v1.3.5] - 2026-09-28

### 新增

- **示例页面升级验证内容**：页面版本提示与验证标识更新为 `v1.3.5-example-page`，新增「页面状态」统计卡和「本次示例页变化」说明，便于确认升级后的前端资源已经切换

### 变更

- 版本号提升到 1.3.5；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.12.8

### 测试

- 前端类型检查、单元测试与生产构建

## [v1.3.4] - 2026-09-28

### 修复

- **一键升级找不到 Compose 文件**：fnOS 或 Docker UI 启动 Compose 时，updater 的 `.:/workspace` 可能解析到错误目录，构建时报 `open /workspace/docker-compose.hub.yml: no such file or directory`。Compose 文件现支持用 `FONU_UPDATE_HOST_DIR` 显式指定宿主机 Compose 目录，并在状态接口中提前报告缺少 Compose 文件，避免排到升级后才失败
- **示例页验证标识更新为 v1.3.4-example-page**，用于确认升级后的前端资源确实生效

### 变更

- 版本号提升到 1.3.4；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.12.8

### 测试

- `internal/updater`：补充 Compose 文件缺失检查

## [v1.3.3] - 2026-09-27

### 新增

- **示例页面升级验证标识**：示例页面新增固定的 `v1.3.3-example-page` 验证标识与展示项，升级后可直接通过标识区分新旧前端资源，确认页面、路由和静态资源均已更新

### 变更

- 版本号提升到 1.3.3；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.12.8，本次无 Agent 行为变化

### 测试

- 前端类型检查、单元测试与生产构建

## [v1.3.2] - 2026-09-27

### 修复

- **一键升级无法读取远端版本**：`fonu-updater` 检查更新时原来会 `git clone --depth 1` 整个仓库，而状态接口的父级超时只有 20 秒，克隆未完成即被 `signal: killed`，表现为 `fatal: early EOF`，前端因此一直不显示「一键升级」。现改为优先通过 Gitea API / raw 地址或 GitHub raw 读取 `VERSION`，显著降低检查耗时，不再依赖完整克隆

### 变更

- 版本号提升到 1.3.2；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.12.8，本次无 Agent 行为变化
- `fonu-updater` 位于 sidecar，不能通过自身升级自身；已有部署需要先手动重建一次 `updater` 服务，之后该修复才生效

### 测试

- `internal/updater`：新增 Gitea API / raw 地址解析、文本与 base64 JSON 版本内容解析测试

## [v1.3.1] - 2026-09-27

### 新增

- **示例页面（在线升级验证页）**：新增 `/example` 页面与侧栏「示例页面」入口，页面实时读取 `/api/version`，展示当前版本、验证步骤和验证结果，用于确认一键升级后前端路由、菜单与后端版本均已更新

### 变更

- 版本号提升到 1.3.1；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.12.8，本次无 Agent 行为变化

### 测试

- 前端类型检查、单元测试与生产构建

## [v1.3.0] - 2026-09-27

### 新增

- **Web 一键升级（Docker Compose）**：新增受限的 `fonu-updater` 升级 sidecar，支持 `docker-compose.yml`（本机源码构建）与 `docker-compose.hub.yml`（Git 仓库构建）两种部署。主程序只通过共享 Unix Socket 调用 sidecar，不直接挂载 Docker Socket；升级时只重建 `fonu` 服务，构建或健康检查失败会自动尝试回滚到升级前的镜像 tag
- 前端侧栏在检测到可升级版本时显示「一键升级」入口，升级弹窗展示当前版本、目标版本、升级方式、阶段消息与最近日志；服务重启期间浏览器会自动等待恢复
- 新增 `docs/system-upgrade.md`，说明首次部署 sidecar、安全边界、升级流程、回滚和常见问题

### 变更

- 版本号提升到 1.3.0；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 保持 0.12.8，本次无 Agent 行为变化

### 注意

- 已经运行旧版本的实例，需要先手动更新 Compose 文件并执行一次 `docker compose up -d --build`，让 `fonu-updater` sidecar 启动。此后才可以在 Web 页面一键升级

### 测试

- `internal/updater`：版本比较、compose 路径白名单、配置校验

## [v1.2.21] - 2026-09-27

### 修复

- **Agent Bearer Token 改为常量时间比较**：`internal/agent` 原来用普通字符串比较校验 Token，现将 Authorization 解析与 Token 比较拆分，改用 `crypto/subtle.ConstantTimeCompare`，避免认证路径上的长度与内容时序泄漏

### 变更

- 版本号提升到 1.2.21；`web/package.json` 与 `web/package-lock.json` 同步对齐
- `AgentVersion` 0.12.7 → **0.12.8**，内嵌 amd64 / arm64 二进制已重编；VPS 上需点一次「升级 Agent」才会生效

### 测试

- `internal/agent`：新增认证测试，覆盖正确 Token、大小写 Bearer、多余空白、错误 Token、长度不同、缺失 Authorization、错误认证方案

## [v1.2.20] - 2026-09-27

### 修复

- **DDNS 页 FRP 提示恢复**：`DdnsView.vue` 原来调用已退役的 `/api/frp`，请求失败后会把 `frpEnabled` 固定为 `false`，导致“FRP 内网穿透已启用”提示不再出现。现改用 `/api/frpmulti/status`，仅在“已配置服务端且存在启用规则”时显示，并补充判定函数单元测试

### 变更

- 版本号提升到 1.2.20；`web/package.json` 与 `web/package-lock.json` 同步对齐
- 本轮不涉及 Go 与 agent 行为，`AgentVersion` 仍为 0.12.7

## [v1.2.19] - 2026-09-27

### 新增

- **数据目录可选（默认行为不变）**：三个 compose 的挂载改为 `${FONU_DATA_PATH:-fonu-data}:/data`——默认仍是命名卷 `fonu-data`，**老用户升级后不受影响**；在同层 `.env` 里写 `FONU_DATA_PATH=/vol1/docker/fonu-data` 即可让数据落到指定宿主机目录（以 `/` 或 `.` 开头的写法会被 compose 当成 bind mount，其余字符串仍是命名卷）
- **新增 `docs/container-deploy-and-data.md`（容器部署与数据指南）**：三个编排文件的分工与选择、干净机器单文件部署、数据目录（默认卷 / 指定目录 / 从卷迁移）、备份与恢复的三条注意、升级与回滚、环境变量速查、常见问题（401 加速器、`FONU_SESSION_SECRET` 报错、`fonu Pulling` 超时、换目录后「数据没了」、仪表盘三张卡为空）、与原生安装之间的迁移

### 变更

- 版本号提升到 1.2.19；`web/package.json` 同步对齐
- 本轮不涉及 Go 代码（只改 compose 与文档），因此安装包未重出，`dist/` 里 1.2.17 的包仍可用；`AgentVersion` 仍为 0.12.7

## [v1.2.18] - 2026-09-27

### 修复

- **国内加速器上构建失败（401 Unauthorized）**：`Dockerfile` 顶部原来显式写了 `# syntax=docker/dockerfile:1`，这会让 BuildKit 额外去拉 `docker/dockerfile` 镜像，而在飞牛的 `docker.fnnas.com` 这类加速器上会被拦成 401（`resolve image config for docker-image://docker/dockerfile:1` 失败）。该行已移除：`RUN --mount=type=cache` 由 Docker 内建的 BuildKit frontend 已支持，不需要显式指定，缓存挂载保持不变
- **compose 无谓地尝试拉镜像**：`docker-compose.hub.yml` 增加 `pull_policy: build`，跳过日志里那条 "fonu Pulling"（国内直连 Docker Hub 的超时告警），直接构建

### 变更

- 版本号提升到 1.2.18；`web/package.json` 同步对齐
- 本轮**不涉及 Go 代码**（只改 Dockerfile 与 compose），因此安装包未重出，`dist/fonu-1.2.17-linux-*.tar.gz` 仍可用；`AgentVersion` 仍为 0.12.7

## [v1.2.17] - 2026-09-27

### 新增

- **会话密钥自动生成**：`FONU_SESSION_SECRET` 未配置时，应用首次启动会在数据目录里生成 32 字节随机密钥（64 字符）并持久化到 `<数据目录>/session_secret`（权限 0600，先写临时文件再改名；读到损坏内容会重新生成）。启动日志会写明密钥来源（环境变量 / 文件路径 / 仍在使用历史默认值的提示）
- **compose 改为「留空即自动」**：三个 compose 统一 `FONU_SESSION_SECRET: ${FONU_SESSION_SECRET:-}`；`docker-compose.hub.yml` 的单文件部署现在只要 `docker compose up -d --build`，**不再需要预先准备 `.env`**

### 变更

- `FONU_SESSION_SECRET` 的默认值由固定的 `change-me-in-production` 改为空（空 = 未配置 → 自动生成）
- **不会自动替换已配置的值**：这个密钥同时是 `secret.NewBox` 的加密密钥，替换会让库里已加密的凭据（frp token、通知密码等）无法解密；对仍在使用历史默认值的实例只提示、不改数据
- `.gitignore` 补上 `.env` 与 `.env.local`（`.env.example` 这类模板仍可入库）
- 版本号提升到 1.2.17；`web/package.json` 同步对齐
- 本轮不含 agent 行为变更，`AgentVersion` 仍为 0.12.7

### 测试

- `internal/config`：密钥生成与复用、文件权限（类 Unix 上校验 0600）、显式配置优先且不写文件、历史默认值保留并提示、损坏文件重新生成

## [v1.2.16] - 2026-09-27

### 改进

- **`docker-compose.hub.yml` 改为「单文件从仓库拉取源码构建」**：`build.context` 直接用 Git 地址（`http://45.207.223.190:10082/mrlee/-Fonu.git#main`），由 BuildKit 自己克隆后再构建——干净机器上**只需要这一个 compose 文件 + 一个 `.env`**，不必先手动 `git clone`。升级也是回该目录重跑 `docker compose up -d --build`（会重新拉 `main` 的当前代码）。文件头补齐了完整用法
- **会话密钥不再静默回退**：这一份把 `FONU_SESSION_SECRET` 写成必填（`${FONU_SESSION_SECRET:?...}`），没在 `.env` 里给值就直接拒绝启动并给出生成命令，避免用 `change-me-in-production` 直接跑在公网

### 变更

- 版本号提升到 1.2.16；`web/package.json` 同步对齐
- 本轮不含 agent 行为变更，`AgentVersion` 仍为 0.12.7
- 三个 compose 的分工：`docker-compose.yml`（本地源码上下文）、`docker-compose.hub.yml`（**从仓库拉取**）、`docker-compose.host.yml`（host 网络，仍需本地源码）

## [v1.2.15] - 2026-09-27

### 新增

- **仪表盘「实时流量」支持切换数据源（本机 / 公网）**：本机读 Fonu 管理的本机 Nginx 访问日志；公网读 frps 侧逐规则的 `today_traffic_in/out` 与 `cur_conns`（速率由后端两次采样算出，口径与「公网反代」页的当前上传/下载一致）。切换时会清空 5 分钟曲线，避免两个来源的采样混在一张图里；还没有穿透规则时「公网」按钮禁用并给出说明

### 改进

- **容器交付：升级改为在部署机（NAS）上直接构建**（不再需要 save / 传文件 / load 镜像包）
  - `Dockerfile`：加 `# syntax=docker/dockerfile:1` + 四处 `RUN --mount=type=cache`（npm 与 Go 模块/编译缓存跨次构建复用，重复构建从几分钟降到几十秒）
  - 三个 compose 统一 `name: fonu`（否则改了目录名/项目名后 compose 会把它当成新项目，`fonu-data` 变成新空卷，现象就是「数据丢了」）与 `image: fonu:${FONU_VERSION:-local}`（镜像 tag 随版本走，便于回滚）；`.host.yml` 补上原本缺失的 `image:`
  - 新增 `scripts/upgrade.sh`：一条命令完成「拉代码 → 构建 → 重建 → 健康检查」，支持 `--no-pull`、`--rollback <tag>`（不重新构建）与 `COMPOSE_FILE=`（host 网络那仃）

### 变更

- 版本号提升到 1.2.15；`web/package.json` 同步对齐
- 本轮不含 agent 行为变更，`AgentVersion` 仍为 0.12.7

## [v1.2.14] - 2026-09-27

### 新增

- **证书颁发机构：LiteSSL 替换 Buypass**。LiteSSL（亚数 TrustAsia 的公益免费 DV 证书，90 天，官方支持单域名 / 通配符 / 多域名）已接入，ACME 目录 `https://acme.litessl.com/acme/v2/directory`。该目录里 `meta.externalAccountRequired = true`，所以**强制 EAB**：在 FreeSSL 平台「证书自动化 → EAB 管理」创建后，把 Kid 与 HMAC 填进设置即可（不需要像 ZeroSSL 那样用 API Key 去换取）。EAB HMAC 只回掩码。
  Buypass 保留常量与展示元数据（历史证书记录仍能正常显示），但不再可选，直接提交会被拦下并提示替代项。域名若有 CAA 记录，需允许 `trustasia.com`。

### 修复

- **frps 安装一直报 HTTP 404**：frp 官方的校验文件名始终是 `frp_sha256_checksums.txt`（不带版本号，v0.52.3 / v0.58.1 / v0.62.1 / v0.71.0 实测均如此），而代码拼的是 `frp_<版本>_sha256sums.txt`——任何版本、任何网络环境下都装不上。现改为按候选名依次尝试（旧名保留兜底），失败信息里列出尝试过的文件名。
  **同一条 bug 也在本机下载 frpc 的路径上，一并修复**；SHA-256 校验逻辑本身一行未改。
- **Nginx 探测两处（均在 agent）**：
  - `nginxPath()` 探测不到时不再回落到配置里的裸名字 `nginx`：那会让「未安装」被误判为「配置校验未通过（nginx -t 执行失败）」，且「VPS 未安装 Nginx」告警与一键安装按钮**永远不会出现**。
  - vhost 目录探测不再要求被加载文件以 `.conf` 结尾：Debian/Ubuntu 的 `sites-enabled/default` 不带后缀、而 `nginx.conf` 里是 `include .../sites-enabled/*`，过滤掉它之后只剩主配置目录可试，报「在 1 个被加载目录中均无法放置完整 server 配置」；现在失败信息会带上最后一次的具体原因与常见显式指定目录。
- **前端申请证书弹窗的颁发机构卡片**改为前端可控清单：原先硬编码为 Let's Encrypt / ZeroSSL / Buypass，后端 `/api/certs/options` 的结果不参与渲染，导致后端增删 CA 时弹窗不变（本次 LiteSSL 替换就是这幺被发现的）。现在卡片为 Let's Encrypt / ZeroSSL / LiteSSL。

### 测试

- `internal/frp`：校验文件候选名顺序、sha256sum 格式解析（含 `*` 前缀与不存在的文件负例）
- `internal/acme`：LiteSSL 目录地址 / 展示名 / 别名归一化、通配符与多域名不被拦、Buypass 停用提示里含替代项

### 变更

- `AgentVersion` 0.12.4 → **0.12.7**（0.12.5 修 frps 校验文件名；0.12.6 修 nginxPath；0.12.7 修 vhost 目录探测），双架构内嵌二进制已重编；**VPS 上需点一次「升级 Agent」**
- 版本号提升到 1.2.14；`web/package.json` 同步对齐

## [v1.2.13] - 2026-09-27

### 新增

- **Linux 原生安装包**：`scripts/build-linux.ps1` 一条命令产出 `fonu-<版本>-linux-{amd64,arm64}.tar.gz`（内含二进制、migrations、systemd 安装脚本、前端运行时资源与 sha256 校验文件，无需 Docker/WSL，依赖纯 Go 的 modernc.org/sqlite）；`scripts/fonu-install.sh` 负责目标机安装与卸载：装到 `/opt/fonu`、数据在 `/var/lib/fonu`、环境变量在 `/etc/fonu/fonu.env`（首次生成随机会话密钥，重装保留）并注册 `fonu.service`
- **`--reset-admin`**：忘了管理员口令时清空 `admins`/`sessions` 后重启，启动流程会重新生成随机口令；装完也会直接打印初始口令（以前只提示访问地址，用户找不到口令）
- **Cloudflare Tunnel 融合方案文档** `docs/cloudflared-integration-plan.md`：同类项目对比、两种托管模型、可复用面盘点、参考实现分析（含用户自有项目 cf-tunnel-manager）与 IP 优选可行性边界
- 设置页「ACME 证书」新增 **DNS 解析器（高级）**设置项

### 修复

- **证书申请：DNS-01 传播检查用错解析器**：lego 默认读系统 `/etc/resolv.conf`，容器里那是 Docker 的 `127.0.0.11`，它不返回权威 NS，于是报 `could not determine authoritative nameservers`。现在显式传入 `dns01.AddRecursiveNameservers`（默认 `223.5.5.5:53,1.1.1.1:53,8.8.8.8:53`，可用 `acme_dns_resolvers` 覆盖，填 `system` 回到系统解析器；配置无效时报明确错误而不是静默降级）
- **Buypass 下线**：该 CA 已于 2025-10-15 停止签发、2026-04-15 终止 ACME 服务，已从可选 CA 列表移除；历史记录仍可展示，直接提交会被拦下并给出中文原因（不再让用户面对一个看不懂的 ACME 报错）
- **ZeroSSL 报错信息不全**：返回 422 时把 ZeroSSL 的 `error.type` / `error.code` 带进错误；非 JSON 响应退回压平截断的正文，便于定位
- **密钥泄露**：ZeroSSL EAB 请求失败时，`url.Error` 里带 `access_key` 的完整 URL 会被写进任务日志与 `<数据目录>/logs/app.log`；现在只保留底层错误原因

### 测试

- `internal/acme`：递归解析器列表解析（分隔符/自动补 53/上限 8 条/默认值可用）；ZeroSSL EAB 四条（成功、422 带错误类型、非 JSON 退回正文、网络错误不泄露密钥）

### 变更

- 版本号提升到 1.2.13；`web/package.json` 同步对齐
- 本轮不含 agent 行为变更，`AgentVersion` 仍为 0.12.4

## [v1.2.12] - 2026-09-25

### 修复

- **frps.toml 同源配置预览泄露明文密钥**：`FRPSServerConfig` 是对外**预览**入口，之前直接把 builder 输出返回，而模板里含解密后的 `auth.token`——「即将下发的内容」弹窗拿到的是明文，与同文件「密钥不入其他 API 响应」的约定矛盾。现在预览统一走 `maskFRPSSecrets`（`auth.token` / `auth.oidc.clientSecret` 的值换成 `********`，键名与行结构保留、注释行不动）；**真正下发路径不受影响**（`PushFRPSConfigToAgent` 与安装流程仍用未脱敏内容）。预览弹窗补了说明，「复制」改为「复制（已脱敏）」
- 规则详情「日志」页签改为**倒序**：加载后反转，最新一条在最上面（与日志页「FRP 日志」页签一致）

### 改进

- 日志页「FRP 日志」的**来源**列不再显示 frpc 的 Go 源码文件行号（如 `client/control.go:174`），改为中文模块名（登录 / 连接 / 代理 / 服务 / 配置 / 健康检查 / 服务端）；认不出来的值（包括回落到的组件名）原样保留
- 「公网服务端」日志弹窗改为显示 **VPS 上的 frps 服务日志**（该页职责是管理 frps 端），标题与说明同步；本机 frpc 日志仍在日志页「FRP 日志」页签
- frps 日志从写死 6 行改为**最近 20 行、最新在最上面**，并把 `2026-09-22T18:43:34+08:00 RainYun-XXX systemd[1]: …` 重写成 `2026-09-22 18:43:34 systemd[1]: …`（去掉 T 分隔、时区偏移与每行重复的主机名）；`frps/action` 失败时带回的日志一并统一格式

### 测试

- `internal/frp`：`maskFRPSSecrets` 脱敏（明文不出现、非敏感行不被误伤）与注释行不被改写

### 变更

- **agent 行为变更**：`AgentVersion` 0.12.2 → **0.12.4**（0.12.3：20 行 + 倒序 + ISO；0.12.4：重写日期格式），双架构内嵌二进制已重编；**需在 VPS 上点一次「升级 Agent」**才生效
- 版本号提升到 1.2.12；`web/package.json` 同步对齐

## [v1.2.11] - 2026-09-25

### 新增

- **定时自动备份 + 保留策略**：每天自动备份一份到 `<DataDir>/backups`，超过保留份数自动清理最旧的；重启不会重复备份（按「最新备份是否在 24 小时内」判定）。设置页「数据」卡可开关、改份数、立即备份，并列出/下载/恢复/删除备份，**恢复前先校验归档**
- **健康检查持续化 + 公开状态页**：规则的四段健康结论按**状态变更**落库（采样间隔即巡检轮次），由此推导可用率与不可用区间；设置页「高级」新增「公开状态页」——默认关闭，可设访问码（只存 SHA-256、不下发明文），按 IP 限流，只暴露域名与可用率
- **指标历史与趋势图**：新增纵向点表 `metrics_samples`，记录本机 CPU / 内存 / 磁盘与每条规则的上行/下行速率（采样跟随巡检，30 天保留）；仪表盘新增「资源与流量趋势」卡，支持 1 小时 / 24 小时 / 7 天
- 告警事件新增「自动备份失败」（默认开启）

### 修复

- **隧道探活：「判不了」不再当成「不通」**：`RouteHealthCandidate` 新增 `tunnel_known`，frps 管理接口未配置或不可用时不再把 `tunnel_ok=false` 当成故障——此前会对这类用户误报「隧道连续不通」
- 健康汇总同样区分「无法判定」与「不通」：新增 `tunnel_unknown` 计数，规则表徽标用警告色并说明「不代表隧道不通」，「无法判定」不再算作不健康

### 改进

- 巡检每轮只采一次主机资源，同时喂给「资源阀值告警」与「指标历史」（CPU 占用率靠两次采样的差值，采两次会互相干扰）
- 流量聚合抽出 `aggregateHostKeys`，端点聚合与按域名聚合共用同一套速率算法，避免两处漂移
- 仪表盘：新增趋势卡；「运行状态」并入顶部统计区（运行时长补进「服务」卡）；「性能」并入系统资源那一行，整行 4 列
- 趋势图：图例与绘图区留白调整；点少时画点（此前只有一两个点时线根本画不出来）；空桶断开而不是补 0

### 测试

- `internal/backup`：导出 + 校验往返、校验拒绝非 gzip、列表/剪枝/名称白名单、到期判断
- `internal/metrics`：分桶平均与空桶不产点、窗口外的点、退化输入
- `internal/frp`：可用率推导（区间 + unknown、仍在持续、无记录不出结果）、健康汇总的 unknown 用例
- `internal/api`：状态页限流与访问码哈希
- `internal/app`：monitor 与 frp 两侧状态字面量的翻译

### 变更

- 新增迁移 `026_route_health.sql`、`027_metrics_samples.sql`（启动时自动执行）
- agent 无改动，`AgentVersion` 保持 **0.12.2**，本轮不需要在 VPS 上重新升级 Agent
- 版本号提升到 1.2.11；`web/package.json` 同步对齐

## [v1.2.10] - 2026-09-25

### 新增

- **告警事件新增三项巡检**（默认关闭，设置页「告警事件 → 系统」）：
  - **公网服务端掉线**：服务端进程没在跑、或隧道连不上 frps 时通知，恢复时再通知一次；只对「用户要它运行」的服务端判定，手动停止的不报
  - **公网 agent 不可达**：VPS 上的 fonu-agent 连续无响应（重启后没起来最常见），恢复时再通知一次；没配 agent 的服务端不参与判定
  - **配置漂移**：VPS 上的 vhost 与 Fonu 记录不一致（被手工改过、或 agent 重装后没重部署）时通知，漂移清空时补一条恢复
- 通知图标与分类补齐：📈 系统（资源占用）、🔌🛑📡 隧道（隧道 / 服务端 / agent）、🧭 配置（漂移）
- 通知方式的 `Webhook` 改名为「Webhook / IM 机器人」：企业微信、钉钉、飞书机器人挂在 Webhook 预设下，原标签没说明白

### 改进

- 巡检改为 Source 接口（Routes / Servers / Agents / Drift），`internal/app/monitor_source.go` 负责把 frp 服务适配进来，`internal/monitor` 因此不依赖 frp 包
- 「连续 N 次失败」抽成共用的 `failureTransition`（原 `tunnelTransition` 泛化）：三条新巡检与原有隧道巡检共用「刚好达标报一次、恢复报一次」；漂移不是连续失败型，单独用冷却（默认 24 小时，可配）
- 避免误报的三条约定：服务端只在「用户要它运行」时判定；`ConnectionState` 为 `unknown` 按健康处理；没配 agent 的服务端不参与可达性判定
- 开关关闭时连数据采集都跳过（关掉的功能不该有后台开销）；所有 `notify.Alert` 一律在锁外调用
- 新增设置项：`notify_on_frps_down` / `notify_on_agent_unreachable` / `notify_on_route_drift`，以及 `notify_frps_fail_threshold`(2) / `notify_agent_fail_threshold`(3) / `notify_drift_cooldown_hours`(24)

### 测试

- `internal/monitor/monitor_test.go` 增加 `serverDown`（手动停止不报、进程未运行或连接断开要报、连接状态未知不误报）、`serverLabel`、`stateOr` 用例

### 变更

- 版本号提升到 1.2.10（版本单一来源仍是根目录 `VERSION`）；`web/package.json` 同步对齐
- agent 无改动，`AgentVersion` 保持 **0.12.2**，本轮不需要在 VPS 上重新升级 Agent

## [v1.2.9] - 2026-09-24

### 新增

- **配置版本历史 / 对比 / 回滚**：主 `nginx.conf` 每次保存前留一份版本（`<path>.versions/`，保留最近 10 次**内容有变化**的改动，内容相同自动去重）；设置页「Nginx 全局配置」内可看版本列表、逐行差异并一键回滚——回滚先过语法校验与 `nginx -t`，不过关不覆盖当前配置
- 公网反代页的「查看配置」弹窗内可看该域名在 VPS 上的历史版本、与当前配置做差异对比并回滚（需 agent ≥ 0.12.2）
- **通知渠道补齐**：新增企业微信机器人、钉钉机器人、飞书机器人（钉钉正文以「Fonu · 」开头，机器人用「自定义关键词 = Fonu」即可命中；加签方式暂不支持）
- **阈值型告警**（默认关闭）：CPU / 内存 / 磁盘持续超阈值、隧道连续 N 次未在 frps 上注册时通知，恢复时再通知一次；阈值与开关在设置页「告警事件」里配置
- **API Token**：新增 `api_tokens` 表与 `Authorization: Bearer <token>` 认证，供脚本 / HomeAssistant / 手机快捷指令调用；设置页「高级」可签发（明文只显示一次）、查看最近使用时间与撤销

### 修复

- agent 写 vhost 时 `nginx -t` 校验失败**不再直接删文件**（那等于把在跑的站点从磁盘上抹掉），改为恢复上一个可用版本
- 配置版本名做路径校验，拒绝带 `/` 或 `..` 的名称，避免接口被用来读版本目录以外的文件
- 通知：切换 Webhook 预设时服务地址自动换成该预设默认值（此前会把 Bark 的 `api.day.app` 带到企业微信上）
- API Token 的令牌管理、改密、退出**只允许登录会话**——否则一个 write 令牌能给自己提权或无限续期

### 改进

- `internal/fsutil` 新增 `BackupVersioned` / `ListVersions` / `ReadVersion`；`internal/nginx` 的回滚与 `Apply` 共用抽出的 `activate()`，两条路径行为一致
- 新增 `internal/monitor` 巡检包（每 2 分钟一轮，注册进 `internal/scheduler`）；资源类告警同一项 30 分钟静默期，回落到阈值以下即清冷却，开关关闭时连采集与探测都跳过
- `providerUsesKey()` 收敛「凭据放 Key 还是 WebhookSecret」的判定；前端新增 `ConfigDiffView.vue` 与 `utils/diff.ts`（LCS 行级差异，超长文本退化为整体替换）
- API Token 的 `last_used_at` 限频回写（1 分钟一次），单连接 SQLite 上不做每请求写

### 测试

- `internal/auth/token_test.go`（令牌生命周期 / 过期 / 非法输入 / 明文不落盘）、`internal/api/middleware_test.go`（只读令牌只读、写令牌可写、令牌不能管理令牌、错误凭据 401）
- `internal/monitor/monitor_test.go`（连续失败只报一次、恢复报一次、未达阈值静默）
- `internal/notify/push_test.go` 增加企业微信 / 钉钉 / 飞书三家的端点与 payload 用例；`internal/fsutil` 增加版本列表 / 内容去重 / 路径校验用例

### 变更

- `AgentVersion` 0.12.1 → **0.12.2**（域名 vhost 的版本列表 / 对比 / 回滚端点，写配置前留版本）；内嵌 `fonu-agent-linux-amd64` 与 `-arm64` 已重编，VPS 上需点「升级 Agent」才会生效
- 版本号提升到 1.2.9（版本单一来源仍是根目录 `VERSION`）；`web/package.json` 同步对齐
- 新增 `docs/feature-benchmark-2026-09-24.md`：同类项目对标与增补建议，含 §九 本轮 P0 实施状态

## [v1.2.8] - 2026-09-24

### 修复

- 公网服务端：`frps` 启停失败不再假成功——之前忽略 `systemctl` 的 err 后无条件写「frps.service 已启动」，现在失败时返回 `ok:false` 与 stderr，运维不会误以为服务在跑
- frps 配置写入：`.bak` 备份失败时立即中止并删除 `.verify`，不再覆盖唯一副本（旧配置得留退路）
- 安装 frps 后下发同源 `frps.toml` 失败不再静默吞掉：结果里带 `config_warning` 并追加到 `notes`
- 请求体解析：证书申请 / 续期、DDNS 测试改为严格 400；frps 安装允许空请求体（走默认版本）但拒绝 JSON 语法错误
- 会话与登录：同一 IP 在 5 分钟内失败 5 次后返回 429（成功后清零，带过期清理与上限）；改密码只失效该账号的**其他**会话，不再把当前会话一并踢掉；会话 cookie 按 `r.TLS` / `X-Forwarded-Proto` 决定是否加 `Secure`（默认明文部署不加，否则浏览器丢 cookie 导致登不进去），`clearSessionCookie` 补 `SameSite=Lax`
- 后台下载改为继承应用 ctx（新增 `Service.SetBaseContext`），App 关闭时随之中止；`scheduler` 每个任务包 `recover()`，单个任务 panic 不再带走整个进程，且建表时跳过非正间隔（`time.NewTicker` 会 panic）
- 两个 `http.Server` 补 `ReadTimeout` / `IdleTimeout`（agent 60s/2min；应用 5min/2min，**不设 `WriteTimeout`**——否则日志 SSE 会被中途切断）
- 公网反代页：`refreshAll` 的 7 个独立接口由串行 `await` 改为 `Promise.all`，每次刷新不再等于 7 个往返相加
- 内网穿透页：删除 74 条随服务端 UI 迁走的死样式；公网服务端页合并逐字重复的样式块
- 文档与工程：修正 README / DEPLOYMENT / `frps_template.go` 中「Docker 镜像内置 frpc 0.71.0」的不准确说法（镜像不预装、按需下载）；`.dockerignore` 删掉两行已失效的归档排除项并修掉 ` docs` 的前导空格

### 改进

- 新增 `internal/fsutil`：`WriteFileAtomic`（同目录随机名临时文件 + rename + 显式 chmod）与 `Backup`（文件不存在视为无需备份），收敛 agent / frp / nginx / chinacidr 各处自建的 `.tmp` 与 `.bak` 逻辑（其中 nginx 原来用固定 `.tmp` 名，并发写同一路径会互相踩）
- 流量采集：`notify.RecordAccessHit`（写库 + 判断通知）从写锁内移到锁外，不再顶住 `/api/proxies/traffic` 与 `/clients` 的读
- `internal/frp` 的 agent 客户端改为按 `serverID` 缓存（指纹 = `AgentURL` + Token 密文），批量同步 N 条路由不再新建 N 个连接池与 TLS 握手
- 前端新增 `useVisibilityPolling()`：标签页隐藏时暂停轮询、回前台立即补一次；公网反代页与内网穿透页已接入，仪表盘流量轮询加 in-flight 互斥避免堆积并发请求
- 新增 `docs/README.md` 文档索引；给 `agent-capability-plan` / `agent-view-design` / `-v2` / `upstream-sync-plan` 加实施状态与时效说明

### 测试

- Go 新增 4 个测试文件：`backup`（`../` 逃逸与 `../data-other/` 同前缀绕过、还原权限固定）、`scheduler`（非正间隔跳过、panic 后继续）、`frp`（`shellPath` 单引号转义、`isELFBinary`）、`agent`（`validateRoute` 表驱动、`nginxRoute` 关键行、`fileSHA256`）
- 前端引入 vitest（`npm test` = `vitest run`），新增 13 个单测覆盖 `utils/format.ts` 与 `utils/status.ts`（含一条守住「n-tag 的 type 值不能当状态值」的回归用例）；CI 增加「Run web unit tests」一步

### 变更

- `AgentVersion` 0.12.0 → **0.12.1**（frps 启停失败如实回报、HTTP 服务补超时）；内嵌 `fonu-agent-linux-amd64` 与 `-arm64` 已重编，VPS 上需点「升级 Agent」才会生效
- 版本号提升到 1.2.8（版本单一来源仍是根目录 `VERSION`）；`web/package.json` 的版本同步对齐，本地构建不再显示 1.0.2

## [v1.2.7] - 2026-09-24

### 修复

- 安全（P0）：备份恢复的上传文件名不再拼进临时路径，改用 `os.CreateTemp`——此前 multipart 文件名完全可控，`../../root/.ssh/authorized_keys` 之类可逃出临时目录（`internal/api/backup_handler.go`）
- 安全（P0）：备份还原的条目路径校验从「字符串前缀比较」改为 `filepath.Rel` 判定并拒绝 `..`——此前 `DataDir=/data` 时条目 `../data-other/x` 能通过前缀匹配写到 DataDir 之外；同时不再照搬归档里的权限位（固定文件 `0o640` / 目录 `0o750`），避免构造的归档造出 0777 或带执行位的文件
- 安全（P0）：agent 安装 frps 时 `proxy` 参数改走白名单（与「FRP 下载加速代理」同一组前缀），且改为先取官方 `frp_<version>_sha256sums.txt`、下载后比对 SHA-256 通过才解压安装——此前该参数未校验就拼进下载地址，等于可让 VPS 拉取任意地址的二进制并装成 frps
- 安全（P0）：SSH 不再 `InsecureIgnoreHostKey()`，改为 known_hosts + TOFU——首次连接把主机指纹记到 `<DataDir>/ssh_known_hosts`（新增 `config.SSHKnownHostsPath()`），之后指纹不一致直接拒绝连接，错误里带上「期望 / 实际」指纹与处理提示
- 安全（P0）：`GET /api/settings` 不再回传 ZeroSSL API Key 明文，改为只回 `********` 掩码（前端以「非空」判断是否已配置，保存时仅在重新输入才会提交该字段）
- 安全（P0）：agent 环境探测不再回传 Agent Token 明文，改为 `agent_token_configured` + `agent_token_hint`（末 4 位）
- 安全（P0）：反代自动生成的 Basic Auth 密码不再写进 nginx 配置注释（此前会被「查看配置」永久读到），改为在部署响应里一次性返回、页面上提示「仅显示这一次」；部署失败会丢弃刚生成的凭据
- 公网服务端页：服务端详情弹窗的状态点恒为灰色——`serverStartupType()` 返回的是 n-tag 的 type 值，被当成状态值传给 `StatusBadge :value` 后落到 unknown；新增返回 `StatusKind` 的 `serverStartupKind()` 并改用 `:kind`
- 仪表盘：域名卡与证书卡的徽标固定显示「正常」，后端返回异常状态时会出现「红/灰点 + 正常」的矛盾，改为按状态值推导文案并在无值时隐藏

### 变更

- `AgentVersion` 0.11.0 → **0.12.0**（frps 安装校验 + Basic Auth 密码处理），内嵌 `fonu-agent-linux-amd64`（10.4 MB）与 `-arm64`（9.7 MB）已重编，VPS 上需点「升级 Agent」才会生效
- 新增 `.github/workflows/ci.yml`：push / PR 触发，按「前端构建 → 同步 dist 到 `cmd/fonu/web/dist` → `go vet` → `go test ./... -count=1` → `go build ./...`」执行（顺序不可颠倒：`cmd/fonu` 用 `//go:embed all:web/dist`，干净检出缺该目录时编译直接失败）；gofmt 卡口待全仓格式化后再开
- `web/package.json` 版本由 1.0.2 对齐到 `VERSION`，并声明 `packageManager: npm@10.9.4`（与 Dockerfile 的 `node:22-alpine` 一致）；删除 `web/pnpm-workspace.yaml`（占位残留，仓库并无 pnpm 锁文件）
- 版本号提升到 1.2.7（版本单一来源仍是根目录 `VERSION`）

## [v1.2.6] - 2026-09-24

### 改进

- 公网服务端页（`/frp-agent`）改为 DDNS 页那样的**左列表 + 右详情**：左侧是服务端列表（名称 / 启动状态 / `主机:端口` / 延迟与位置 / TLS·Token·Agent·SSH 标签，点行即选中并高亮），右侧是选中服务端的详情；原列表行内操作（启动·停止 / 查看 / 日志 / 测速 / 校验 / 编辑 / 删除）移到右侧工具条，顶部「选择服务端」下拉去掉（左侧列表本身就是选择器），页面标题改「公网服务端」，≤1100px 收成一列
- 内网穿透页（`/frp`）改为**左：服务端列表 / 右：该服务端的穿透规则**：左侧点服务端即筛选（显示规则数与启用数），右侧保留原来的穿透规则表格（`:data` 按选中服务端过滤、去掉冗余的「服务端」列、`:scroll-x` 2020 → 1890），工具条显示当前服务端与规则条数；规则详情仍是弹窗（表格「详情」按钮），实时速率采样只在弹窗打开期间进行
- 服务端列表从内网穿透页整体并入公网服务端页：服务端表单 / 服务端日志 / 服务端详情 / frps.toml 同源配置四个弹窗随列表一起迁移；内网穿透页不再持有服务端的增删改与启停
- 内网穿透页规则区在「尚未配置服务端」时给出指向「公网服务端」页的空态（原来只剩一个被禁用的「添加规则」按钮）
- 日志中心「公网 FRP」页签改为时间倒序（新日期在前），分页与筛选跟着一致

### 变更

- 「Agent 管理」跳转改为深链：内网穿透页点某台服务端直接落到该台详情（`/frp-agent?server=<id>`，id 不存在时回落为第一台）
- 服务端列表不再按「已配 Agent / SSH」过滤——未配的服务端也要能在本页编辑与删除
- 版本号提升到 1.2.6（版本单一来源仍是根目录 `VERSION`）

## [v1.2.5] - 2026-09-24

### 新增

- 公网 Agent 能力扩展（agent 0.11.0）：新增 `GET /api/v1/certs`（VPS 证书清单——遍历证书目录、用标准库 `crypto/x509` 解析 `fullchain.pem` 的到期时间、按剩余天数排序、解析失败的单条记入 `warnings` 不中断整轮）与 `POST /api/v1/nginx/reload`（显式校验并重载 Nginx，分别回传 `nginx -t` 与 reload 的失败原因；成功后让可用性探测缓存失效，避免页面继续显示旧结论）
- 公网 Agent 页：新增「VPS 证书」卡（域名 / 剩余天数 / **Fonu 证书库** / 到期时间），与 Fonu 证书库交叉核对（含通配符匹配），宝塔自行申请的证书也能看到；剩余不足 30 天转橙、已过期转红加粗
- 公网 Agent 页：Nginx 卡片新增「重载 Nginx」按钮，校验 / 重载失败原因（`nginx -t` 输出或 reload 的 stderr）直接显示在卡内

### 改进

- 公网 Agent 页布局：「VPS 资源」与「VPS 证书」改为并排各占半行——`dep-grid` 由 3 列改为 6 列并按 span 分配（原有三张卡仍每行三张），半宽下资源三项指标等分、平均负载与运行时长落到卡底一行，证书表列宽收紧；≤1199px 复位跨度避免 `span` 在列数变少后撑破网格
- 应用侧对 agent 端点的版本要求提升到 **≥ 0.11.0**（旧 agent 仍按 404/405 统一降级为「请升级 Agent」提示）

### 变更

- `AgentVersion` 0.10.0 → **0.11.0**；内嵌 `fonu-agent-linux-amd64`（10.0 MB）与 `-arm64`（9.3 MB）已重编，VPS 上需点「升级 Agent」才会生效
- 版本号提升到 1.2.5（版本单一来源仍是根目录 `VERSION`）

## [v1.2.4] - 2026-09-24

### 新增

- 公网 Agent 能力扩展（agent 0.10.0）：新增三个只读端点——`GET /api/v1/metrics`（VPS 主机资源：CPU / 内存 / 磁盘 / 负载 / 运行时长，CPU 用两次采样差值，1 秒内的重复请求直接复用上次结果）、`GET /api/v1/logs/nginx`（access / error 日志尾部，路径只从 `nginx -T` 实际生效配置、`FONU_AGENT_NGINX_LOG_DIR` 或默认目录取；按 32KB 块倒读，单行 4KB 截断，上限 2000 行）、`GET /api/v1/frps/config`（frps.toml 回读，256KB 上限，文件不存在返回 `exists: false`）
- 公网 Agent 页：新增「VPS 资源」横排带（CPU / 内存 / 磁盘三项并排 + 进度条 + 右侧平均负载与运行时长），占满「环境与依赖」整行，不做单独占格
- 日志中心：新增「公网 Nginx」页签（服务端 / 访问或错误日志 / 行数 200·500·2000 + 关键词搜索 + 分页），并显示 agent 解析出的实际日志路径与「仅显示尾部」标记
- 公网服务端「frps.toml 同源配置」弹窗：新增「比对 VPS 实际文件」——去注释与空行后逐行比对，一致给出绿色说明，不一致指出第一个差异行（下发值 / VPS 值）

### 改进

- `internal/sysinfo` 补 `LoadAvg()` 与 `UptimeSeconds()`：Linux 读 `/proc/loadavg`、`/proc/uptime`，Windows 用 `GetTickCount64`，其它平台返回空值
- 老版本 agent 的降级：三个新端点遇到 HTTP 404/405 时统一转成「VPS 上的 agent 版本过低，不支持该接口（需 ≥ 0.10.0），请在服务端详情里升级 Agent」的可操作提示，而不是原始 HTTP 错误

### 变更

- `AgentVersion` 0.9.2 → **0.10.0**；内嵌 `fonu-agent-linux-amd64`（9.9 MB）与 `-arm64`（9.2 MB）已用新代码重编，VPS 上需点「升级 Agent」才会生效
- 版本号提升到 1.2.4（版本单一来源仍是根目录 `VERSION`）

## [v1.2.3] - 2026-09-24

### 新增

- 仪表盘：新增「公网穿透」统计卡，并在「运行状态」面板增加「公网反代」行——数据来自新增的 `GET /api/frpmulti/routes/summary`（逐台服务端采集四段健康后聚合，单台失败只记名不中断）
- 仪表盘：新增「性能」面板（今日异常 / 平均响应，`/api/status` 早已返回但页面从未展示），「实时流量」面板新增「流量 Top 5」（直接复用已有的逐规则流量轮询）
- 仪表盘：新增一行三卡「系统资源 / 中国 IP 段 / 通知通道」；系统资源来自新增的跨平台采集包 `internal/sysinfo`（Linux `/proc`、Windows kernel32，无新增依赖；容器内为宿主机视角）
- 公网反代：新增部署漂移清单（未部署 / 孤儿配置 / 未生效 / 内容不一致）与「重新部署全部」——逐条下发、单条失败不中断；新增 `GET /api/frpmulti/servers/{id}/agent/routes/drift` 与 `POST .../agent/routes/redeploy`
- 公网 Agent：端口矩阵补端口含义提示；「frps 运行时」新增「启动一致性」核对（systemd 实际启动命令 vs 配置的二进制与配置文件，检出「Fonu 写入的 frps.toml 不是 frps 实际加载的那份」）
- 新增 `docs/agent-capability-plan.md`：fonu-agent 能力扩展方案（12 个端点现状盘点、P0–P2 分批设计、agent 侧与应用侧改动流程、验收与风险回滚）

### 改进

- 仪表盘「服务」卡：原先写死「正常 / 4 个」，现按 Fonu / Nginx / DDNS / 证书四项实际状态聚合，异常时点名并把徽标降级为 warning/error（此前 Nginx 停止、DDNS 异常时该卡仍显示「正常」，与右侧「运行状态」面板自相矛盾）
- 仪表盘轮询：`/api/status` 每 15 秒刷新一次（CPU 占用率需要两次采样才有意义），并在页面隐藏时暂停「实时流量」与「状态」两个轮询
- 公网反代健康列：证书徽标补「证书来源」——`Fonu 证书库：有（剩余 N 天）` 或 `无（需先在证书管理申请）`，用于区分「没有证书可推送」与「VPS 上证书过期或未覆盖」

### 修复

- FRP：frpc 版本读不出来——`RuntimeStatus.Version` 带 `omitempty`，`frpc --version` 探测失败时字段被整体省略，前端只能显示「未检测 / 未知」；现回落为按 `<FrpDir>/bin/<版本>/` 目录名推导（与「FRP 应用配置」弹窗的 `active_version` 同源），并改为逐台服务端探测版本（原先只探测默认那台）
- 仪表盘：删除死样式（`.dashboard` / `.stat-highlight` / `.proxy-metric*`）

### 变更

- 版本号提升到 1.2.3（版本单一来源仍是根目录 `VERSION`）

## [v1.2.2] - 2026-09-23

### 新增

- 日志中心：新增「FRP 日志」Tab——把 frpc 运行日志解析为 时间 / 级别 / 来源 / 消息 四列，支持关键词与级别筛选、分页、错误/警告行着色，与系统日志、访问日志、Nginx 日志同一套交互（对齐上游 1.0.3）
- 新增 `web/src/utils/clipboard.ts`：统一的剪贴板复制工具（异步剪贴板 API 优先，失败或非安全上下文时回退 `execCommand`）

### 改进

- 剪贴板复制在非 HTTPS 环境可用：Fonu 常以 `http://内网IP:6893` 访问，属非安全上下文，此时 `navigator.clipboard` 为 undefined，复制必然失败；公网反代（复制域名 / 复制配置）、公网 Agent（配置查看）、DDNS、本地反向代理（复制访问链接）共 5 处调用点统一改用上述工具（对齐上游 1.0.4）
- 容器时区：`Dockerfile` 新增 `TZ=Asia/Shanghai` 并在运行时阶段安装 `tzdata`（只设 `TZ` 而镜像内无 zoneinfo 时 Go 仍会回落 UTC），三个 compose 文件均支持 `TZ` 覆盖——通知消息时间按系统时区显示（对齐上游 1.0.4）
- 反代配置查看弹窗：补齐内容区内边距（与头部 24px 对齐），新增来源说明（由 VPS 上的 fonu-agent 生成、文件名 `fonu-<域名>.conf`、手工修改会被下次部署覆盖），并改为说明固定 + 代码块内部滚动
- 反向代理：复制规则不再自动追加「-复制」后缀，保留原名称与域名、由用户在表单里确认修改（原先一点复制就得到一个改过名的副本，容易误建同名规则）（对齐上游 1.0.5）
- FRP 穿透页：frps 同源配置弹窗新增「VPS 防火墙 / 安全组放行 TCP 远程端口」提示，端口列表由已启用的 TCP/UDP 规则实时推导（与 frps 的 `allowPorts` 同源；未放行时隧道连不上，但日志不会点明原因）（对齐上游 1.0.3）
- 通知：Telegram 代理地址只填 `host:port` 时自动补全 `http://`，并校验 scheme 仅支持 http / https / socks5，错误信息带上原始地址便于自查（对齐上游 1.0.3）

### 修复

- 通知：单个通道密文解密失败不再让整份配置加载失败——原先 SMTP / Webhook / Telegram 任一密文损坏（例如更换过加密密钥）都会直接返回错误，导致其它已配置通道一并无法告警；现改为仅把该通道标记为「未配置」并继续解析其它通道（对齐上游 1.0.5）
- 反向代理：监听占用冲突提示写明占用方——优先规则备注名，其次该规则首个域名，最后回落 `规则 #id`；原先只说「已被其他规则使用」，用户得自己翻列表找是哪一条
- 反向代理：手动（custom）Nginx 配置的监听占用纳入校验。custom 配置的正本在文件系统（数据库只记 `nginx_mode` 开关），其 `listen` / `server_name` 原先完全不参与校验，与其它规则「同域名 + 同端口」的冲突只会在 Nginx reload 时静默失效（第一个 server 块生效、后面的被忽略）；现于保存 / 回滚时解析并写入新表 `proxy_custom_endpoints`，与表单规则共用同一条判定（对齐上游 1.0.5）
- 反向代理：并发下修改监听端口可能绕过保存前校验（`UPDATE proxy_hosts` 的唯一约束冲突此前直接抛出原始 SQL 错误），现改为按域名复检并给出带规则名的提示

### 变更

- 新增 `docs/upstream-sync-plan.md`：上游 1.0.2 → 1.0.5 的逐版本对照方案，含 8 项待核对项的代码级核实结论、不照搬项及原因、以及「可增加项清单」（11 项，按价值排序）
- 新增迁移 `024_proxy_custom_endpoints.sql`（手动 Nginx 配置的占用表，`UNIQUE(rule_id, hostname, listen_port)` + `ON DELETE CASCADE`），解析器 `internal/nginx/endpoints.go`（`ParseServerEndpoints`，保守解析：`include` 片段 / 正则 `server_name` / `listen unix:` 一律忽略，宁可漏报不误报）与 `internal/proxy/custom_endpoints.go`
- 补测试：`internal/nginx/endpoints_test.go`（多 server 块、`[::]` 写法、通配与正则、无 `listen` 默认 80）、`internal/proxy/custom_endpoints_test.go`（走真实迁移建内存库：冲突含规则名、排除自身、同域名不同端口放行、custom 占用对表单规则可见、级联清理）
- `docs/upstream-sync-plan.md` 补 §八「第 5 项详细整理」：Nginx 端口占用语义（一条规则占用哪些端口）、现有校验链逐跳、三处真实缺口与方案 A/B/C 对比、实施记录与未覆盖边界


## [v1.2.1] - 2026-09-23

### 改进

- 公网 Agent 页：按「功能清单」重排为五段分区（运行状态 / 环境与依赖 / frps 控制 / Agent 生命周期 / 输出与日志）；4 张状态卡统一为标准 `.stats-row` + `.stat-card` 规格；补齐 5 项「数据已返回但页面未显示」的能力——Agent 凭据状态、8080 端口、已注册反代路由数、最后检查时间、Nginx 可用性
- 公网 Agent 页：「环境与依赖」改为三列并排（Fonu Agent / Nginx / frps 运行时），卡内短字段双列、长路径整行；「frps 控制」与「Agent 生命周期」两列并排；新增「配置查看」按钮（只读弹窗展示即将下发的 frps 配置，含复制），卸载（含数据）增加二次确认
- 公网反代页：规则台账改用 `n-data-table`（与本地反向代理页一致，表头与数据天然对齐），列为 规则/域名（含复制与打开按钮）、协议、上游/回源、健康（DNS·证书·隧道·服务）、连接、当前上传、当前下载、总上传、总下载、更新时间、操作
- 公网反代页：新增统计区（可部署规则 / 已生效 / 健康通过 / 当前连接）与环境与 Nginx 卡（4 列）；「使用说明」整卡改为页面级提示与入口行；部署设置弹窗对齐本地反向代理页规格（900px、三段式、页脚分隔线、开关行底色）并新增右侧「配置说明」栏
- 公网反代页：协议列改为按部署设置的「启用 HTTPS」自动识别（规则 type 恒为 http，不能代表入口协议），HTTPS 绿色 / HTTP 灰色并带悬停说明
- FRP 穿透页：4 张状态卡统一为标准统计区；公网服务端列表由自建网格改为 `n-data-table`（8 列，含启动/停止/查看/日志/测速/校验/Agent 管理/编辑/删除）
- FRP 穿透页：6 个弹窗统一到同一外壳（`.frp-card-modal` / `.frp-detail-card`），运行日志、导出配置、frps 同源配置、FRP 应用配置的头部/滚动区/页脚一致
- 统一设计令牌：清理硬编码色值（公网反代页 6 处、FRP 穿透页 13 处）与内联样式，删除死样式；统计区、弹窗、表格单元格全部走 `--fonu-*` 令牌与共享组件（`StatusBadge` / `EmptyState` / `LoadError` / `FonuCard`）
- 表格与列表交互：数字列右对齐、操作按钮右对齐并按行禁用、长文本换行、单元格内容垂直居中；窄屏按 1200 / 900 / 760 断点降列

### 修复

- FRP 穿透页「运行中」状态显示为灰色：`statusKind` 返回 `'success'` 却被当作 `value` 传给 `StatusBadge`（`utils/status.ts` 不识别该值，落到 `unknown`），且其中含非法的 `'default'`；现改为返回合法 `StatusKind` 并改用 `:kind`，服务端详情弹窗同类问题一并修复
- 表格单元格样式整体失效：`n-data-table` 的 render 函数用 `h()` 创建的节点不带 scoped 属性，导致 `table-muted` / `type-chip` / `mono` 与各 `cell-*` 样式未生效（表现为「上游/回源」两行变一行、域名与按钮错位、徽标无间距）；统一改用 `:deep()` 选择器
- 公网反代页刷新失败静默：`loadErrorMsg` 此前写入但从未渲染，现改为顶部可见提示，并让单个接口失败不再中断整轮数据加载
- 公网反代页切换服务端残留上一台数据、轮询在页面隐藏时仍请求、loading 粒度粗（任一操作全页按钮转圈）等问题修复

### 变更

- 新增设计文档：`docs/agent-view-design-v2.md`（公网 Agent 页）、`docs/proxy-remote-design.md`（公网反代页），以及被取代留档的 `docs/agent-view-design.md`、`docs/agent-view-refactor-plan.md`
- 新增页面 UI 标准 skill（工作区级 `.breezell/skills/fonu-page-ui`，不在仓库内）：页面骨架、共享组件契约、设计令牌、类名约定、弹窗标准结构、脚本约定与验收清单

## [v1.2.0] - 2026-09-23

### 新增

- FRP 公网反代：独立「公网反代」页（`/public-proxy`）——服务端选择、规则列表（名称 / 域名 / 内网目标 / 回源端口）、部署与移除、配置查看、15 秒自动刷新
- FRP 公网反代：**部署后回源自检**——agent 写入配置后模拟 Nginx 回源（`Host: 域名` → `127.0.0.1:<vhost 端口>`）并分级提示：0=连接不上 vhost、404=隧道未注册、5xx=内网服务异常，不再出现「部署成功但打开 502」
- FRP 公网反代：**状态徽章实时化**——基于 `nginx -T` 同时判定「文件存在 / 语法通过 / 实际被加载」，展示已生效 / 已写入未加载 / 语法错误 / 未部署
- FRP 公网反代：**配置查看**——`GET /api/v1/routes/{domain}/conf` 直读 VPS 上生成的 nginx 配置，排障无需 SSH
- FRP 公网反代：**五段健康检查**（逐规则）——DNS 指向、证书覆盖与剩余天数、隧道注册、内网服务探活、反代生效态；任一断点红标 + 悬停显示原因
- FRP 公网反代：**流量列**——当前连接 / 当前上传 / 当前下载 / 总上传 / 总下载（取自 frps dashboard 聚合）
- FRP 公网反代：「部署设置」弹窗（与本地反向代理弹窗同构，三 Tab）
  - 基础：启用 HTTPS（显式开关）、HTTP 强制跳转、WebSocket、上传大小限制、代理读超时
  - 安全：IP 白名单、IP 黑名单、Basic Auth（用户名 + 自定义密码，留空首次自动生成）、仅中国大陆 IP
  - 高级：安全响应头（HSTS / X-Frame-Options / X-Content-Type-Options / Referrer-Policy）、仅 TLS 1.3、请求限流（速率 / 突发）、每 IP 连接数上限
- FRP 公网反代：限流与连接数落地——agent 生成 `fonuagent-limits.conf`（http 上下文 zone 定义，由状态文件驱动增删），server 内输出 `limit_req_status 429` / `limit_conn_status 503`
- FRP 公网反代：仅中国大陆 IP——Fonu 把本地维护的中国 IP 段下发到 agent（`PUT /api/v1/china-cidr`），agent 生成 `geo $fonu_china_client`；IP 段未更新时部署显式报错，不产出半成品配置
- FRP 公网 Agent：新增独立「公网 Agent」页（`/frp-agent`）——agent 状态面板、frps 配置下发、frps 版本安装 / 升级、frps 启停与重启、frps 管理入口、日志、SSH 生命周期与诊断
- FRP 公网 Agent：**Nginx 状态卡片**——状态徽标（正常 / 未安装 / 配置校验未通过 / 已安装未运行）、版本、可执行路径、vhost 配置目录、`nginx -t` 结果、80/443 监听、反代配置文件数
- FRP 公网 Agent：**一键安装 Nginx**——agent 识别 apt-get / dnf / yum / apk 非交互安装并启动，回写探测结果（需 agent 以 root 运行）
- FRP 公网反代：兼容 frps v0.68 与 v0.71+——新版走 `GET /api/v2/proxies`，旧版走 `GET /api/proxies/{user.name}` 逐条查询

### 改进

- FRP 公网反代：部署选项（WebSocket / 强制跳转 / IP 白名单 / Basic Auth 等）从穿透规则弹窗迁移到公网反代页「部署设置」，语义归位（这些是公网反代属性，与隧道无关）
- FRP 公网反代：隧道匹配优先按域名（`spec.http.customDomains`），比按代理名猜测可靠；同时兼容 `user` 前缀（`admin.<规则名>`）与裸规则名
- FRP 公网反代：frps 管理接口调用失败时把具体原因透出到前端悬停（请求 URL、状态码、响应体、密码读取 / 解密环节），不再笼统提示「接口不可用」
- FRP 公网反代：部署预检——未安装 Nginx 时直接提示「VPS 未安装 Nginx，请先安装（apt install nginx / yum install nginx）」，替代 `nginx config rejected` 技术堆栈
- FRP：agent 部署接口由 10 个位置参数改为 `RouteSpec` 结构体（`SyncRouteSpec`），`SyncRouteEx` 保留为兼容包装
- FRP：agent 版本递增至 0.9.1，状态新增 `nginx_ok` / `nginx_missing` / `nginx_running` / `nginx_path` / `nginx_version` / `nginx_error`
- FRP：agent 0.9.2——Basic Auth 哈希改为**纯 Go apr1**（与 `openssl passwd -apr1` 输出逐字节一致，已用参考向量做单元测试），安装脚本 token 改用 `/dev/urandom`：agent 与安装流程彻底去除对 VPS 上 openssl 的依赖（原先缺 openssl 时会静默写入空 token、导致 Fonu 无法认证 agent）

### 修复

- FRP 公网反代：隧道状态误报 ✗——`RouteHealth` 静默忽略 dashboard 密码解密错误导致空密码 401，现逐环节记录并透出
- FRP 公网反代：隧道检测端点错配——旧代码请求 `/api/proxies`（405）或按类型子端点（404），现按 frps 版本自适应并解析 V2 的 `data.items`
- FRP 公网反代：流量恒为 0——V2 返回的键是 `admin.ssssss` 而代码只查裸规则名，现按「裸名 / user 前缀名 / 域名索引」三级匹配
- FRP 公网反代：列表健康徽标样式丢失（此前无样式渲染）
- 修复被外部进程清空的 `internal/frp/store.go`、`internal/api/ddns_handler.go`、`web/src/views/DdnsView.vue`

### 变更

- FRP 公网反代管理从「公网 Agent」页拆分为独立页面（`/public-proxy`），侧栏新增「公网反代」
- 新增 agent 端点：`GET /api/v1/routes/{domain}/conf`、`PUT /api/v1/china-cidr`、`POST /api/v1/nginx/install`
- 新增 Fonu 接口：`GET /api/frpmulti/servers/{id}/agent/routes/health`、`GET /api/frpmulti/servers/{id}/agent/routes/{proxyID}/conf`、`POST /api/frpmulti/servers/{id}/agent/nginx/install`
- 明确不做（已写入 `docs/agent-proxy-settings-plan.md`）：公网反代不使用「目标 Host 头」（会破坏 frps vhost 路由导致 404）、不做「忽略后端 TLS 校验」（回源为明文 HTTP）、不做公网侧自定义 upstream 覆盖（会绕过隧道）

## [v1.1.0] - 2026-09-19

### 新增

- FRP：新增公网 `fonu-agent` 服务端管理基础能力——VPS 侧提供 Bearer Token API，Fonu 本地可测试 Agent、读取状态、下发 frps.toml，并在 HTTP/HTTPS 规则保存后同步公网 Nginx 反代路由
- FRP：服务端新增「Agent 管理」弹窗——API 状态面板（连接/frps/Nginx/80-443 端口监听/已注册路由）与 SSH 生命周期操作（探测环境、一键安装/升级/卸载、SSH 诊断），安装后自动回填 Agent 地址与 Token；agent 二进制内嵌随主程序分发，无需单独下载
- FRP：模块整体升级为多服务端管理——支持添加多台 frps（独立启动 / 停止 / 日志 / 测速 / 校验），穿透规则支持全部 8 种类型（tcp / udp / http / https / tcpmux / stcp / sudp / xtcp）与插件（static_file / socks5 / https2http 等，按插件类型校验必填字段）
- FRP：服务端详情五项独立诊断（TCP 可达 / frpc 登录 / 代理可达 / DNS 解析 / 归属地），含最近 30 分钟可用率与延迟曲线
- FRP：frps.toml 同源生成——服务端「详情」一键生成与 frpc 侧配置同源的服务端配置（bindPort / 认证 / webServer / 传输参数 / vhost 端口 / 防火墙放行清单）；支持 token / 无认证 / OIDC 三种方式；格式对齐官方 `frps_full_example.toml`（v0.52.0+ TOML，v0.63.0+ 均适用）
- FRP：穿透规则实时流量与连接数展示（需在 frps 配置 webServer 管理接口）
- FRP：「应用配置」旁新增「使用说明」——配置流程、DDNS / 反向代理 / 证书组合说明与十个组合场景详解（双标签页 + 手风琴折叠展示）
- FRP：frpc 客户端多版本管理（「应用配置」弹窗内下载 / 启用 / 删除 / 配置下载代理）
- DDNS：新增「自定义 IP」——按任务设置固定 IPv4 / IPv6（如公网服务器 IP），更新时直接使用该 IP，不再检测本机出口 IP；须为公网地址，否则拒绝更新

### 改进

- FRP：脱敏令牌完整闭环——Token / OIDC 客户端密钥 / frps 管理密码掩码往返保护，回传掩码不会覆盖真实凭据；表单统一「已配置」标识
- FRP：服务端详情「配置」面板重新分组排版（描述 / 连接与认证 / 诊断）
- 前端：使用说明弹窗对齐「添加穿透规则」弹窗的自绘外壳样式

### 变更

- FRP：旧「Nginx 网关模式」退役——原 5 条 `/api/frp` 接口移除，由 `/api/frpmulti` 接管；旧实现归档为 `internal/frp-1.0.2`（不参与运行、可整目录删除）；旧网关配置（settings 键值）保留但不再读取，升级后需在「FRP 穿透」页重新配置服务端
- 数据库：新增迁移 015–021（FRP 多服务端核心表）与 022（DDNS 自定义 IP 字段），升级后首次启动自动应用

## [v1.0.2] - 2026-09-18

### 新增

- 证书：支持火山引擎 DNS 的 ACME DNS-01 验证

## [v1.0.1] - 2026-09-18

### 新增

- DDNS：支持火山引擎 DNS（AccessKey / Secret Access Key）
- DDNS：委托子域与多层子域名 Zone 自动匹配（最长后缀优先，如 `a.zzz.com`）

### 改进

- DDNS：解析记录统一展示完整域名；任务列表仅显示记录条数
- DDNS：保存时校验域名格式（非法字符、标签长度、连字符位置）
- 证书：火山引擎 DNS 任务申请证书时给出明确不支持 DNS-01 的提示

### 修复

- DDNS：多层子域名（如 `aaaa.vvv.bbb.com`、`www.a.zzz.com`）保存后被截断的问题
- DDNS：同一任务混用多根域/委托子域时 Zone 解析错误

## [v1.0.0] - 2026-09-18

### 新增

- 通知：支持 SMTP 邮件、Webhook 预设（Bark / ntfy / Gotify / 自定义）与 Telegram 机器人（可选 HTTP 代理），全局仅可启用一种方式
- 设置页新增「测试通知」按钮，无需保存即可验证当前配置
- 告警事件：DDNS / 证书 / 安全 / 系统分组，含 IP 变更、续签成功/失败、登录异常、Nginx 重载失败等
- 设置页改为「常规 / 通知 / 安全 / 高级」分栏，减轻单页信息密度
- 日志中心：系统日志、访问日志、Nginx 错误日志三 Tab；移除实时日志 SSE
- 凭据类字段统一「已配置」标识与「留空表示不修改」占位（SMTP / Webhook / Telegram / DDNS / FRP / Basic Auth 等）

### 改进

- 旧版 `notify_webhook_url` 自动迁移为「自定义 Webhook」
- 通知密钥加密存储，接口不再返回明文；保存后表单状态与服务端同步
- 系统日志中文展示（历史英文日志兼容翻译）
- 仪表盘与反代详情「最近访问」展示反代服务名而非域名
- README 界面截图更新为原图，两列排版展示

### 修复

- 通知 / 系统设置保存后刷新丢失的问题

## [v0.1.31] - 2026-09-18

### 新增

- 反向代理 Nginx：支持单条规则自动/手动编辑、备份与回滚；详情页只读预览，编辑弹窗内配置
- 设置页 Nginx 全局片段：http 块自定义编辑，默认上传大小 50M 片段与框架说明
- Nginx 编辑器：CodeMirror 语法高亮；自动生成配置附带中文注释
- 保存 Nginx 时基础语法校验（无 nginx 可执行文件时亦可拦截明显错误）

### 改进

- 反代日志面板：移除「隐藏轮询」开关，简化日志工具栏

## [v0.1.30] - 2026-09-17

### 修复

- 反向代理 Basic Auth：htpasswd 文件补充 nginx 可读权限与 bcrypt 前缀兼容，修复认证后 500 错误
- 反代连接数：按「域名 + 监听端口」统计，修复同域名多规则互相串数的问题
- 仪表盘「今日请求」：排除错误页与 favicon 噪音，趋势改为较昨日对比

### 新增

- 反代详情日志：支持全屏查看实时访问日志

## [v0.1.29] - 2026-09-17

### 修复

- 反向代理 Basic Auth：修复密码哈希在 Nginx 重载前被剥离导致认证不生效的问题
- 反代上传：全局默认 `client_max_body_size 50m`，修复大文件上传 413 错误

### 改进

- README 更新界面截图（v0.1.28）并补充开源地址与协议说明

## [v0.1.28] - 2026-09-16

### 修复

- 仪表盘「今日请求」：改为今日 0–24 时固定坐标轴（00:00–22:00），未来时段为空

### 改进

- FRP Token：保存后显示「已保存」状态，说明不显示明文的原因
- FRP Web 穿透：移除重复域名输入框，统一在「已同步域名」弹窗中编辑保存

## [v0.1.27] - 2026-09-16

### 修复

- FRP：移除无效的 `pidFile` 配置项，修复 frpc 启动即退出（exit status 1）
- FRP：关闭穿透时保存按钮不可用，导致无法真正关闭
- FRP：frps 配置随连接设置实时生成，不再展示静态示例
- 仪表盘「今日请求」：改为滚动最近 24 小时统计，修复当前时段无数据

### 改进

- Docker 镜像内置 frpc 升级至 v0.71.0

## [v0.1.26] - 2026-09-16

### 新增

- FRP 内网穿透（Nginx 网关模式）：独立「内网穿透」页面配置 frps 连接，自动穿透本地 Nginx HTTP/HTTPS 端口
- Docker 镜像内置 frpc v0.66.x（amd64 / arm64）
- DDNS 页：FRP 启用时提示将域名解析到 VPS 公网 IP

## [v0.1.25] - 2026-09-15

### 新增

- DDNS 与证书 DNS-01：支持腾讯云 DNS（SecretId + SecretKey，区别于 DNSPod ID+Token）

## [v0.1.24] - 2026-09-14

### 修复

- 中国 IP 段更新后 nginx 重载失败：校验用临时文件，应用配置时改用正式的 `china_cidr.conf`
- 仪表盘「今日请求」图表：从完整 access.log 按小时统计，不再仅用最近 10 条访问记录

## [v0.1.23] - 2026-09-14

### 修复

- 自定义错误页插图无法显示：图片路径改为公开 location，浏览器可正常加载 `error.png` / `429.png`

## [v0.1.22] - 2026-09-14

### 改进

- 仅中国大陆：内网 IP（10/8、172.16/12、192.168/16 等）默认放行，无需手动加白名单
- 自定义错误页图片内嵌进二进制，Docker 镜像补充 `web/assets/image` 资源目录

### 修复

- 错误页目录权限：生成后自动 chmod/chown，避免 nginx（www-data）无法读取导致回退默认 403 页
- 中国 IP 段未就绪时，仅中国大陆规则改为只拦截非公网内网地址

## [v0.1.21] - 2026-09-14

### 新增

- 反代安全设置：IP 黑/白名单、仅中国大陆、Basic Auth、速率/连接数限制
- 设置页：信任代理（CDN Real IP）、全局 IP 黑名单、中国 IP 段定时更新
- 自定义 Nginx 错误页（403 / 404 / 429 / 500 / 503）
- 高级选项：上游 TLS 校验跳过、目标 Host 头、TLS 1.3 仅、HTTPS 安全响应头

### 改进

- 中国 IP 段：同步更新 API、ghfast 镜像回退、多源下载与状态展示
- 反代编辑/详情弹窗 UI 重构；列表域名访问链接与复制
- 仅中国大陆 + 白名单：白名单 IP 正确豁免大陆限制（geo + 变量组合）

### 修复

- 白名单模式开启但未填 IP 时的校验
- API 测试依赖 settings 包导入

## [v0.1.20] - 2026-09-13

### 改进

- 反代详情改为弹窗展示，概览布局重排（信息条 + 图表/指标 + 最近访问）
- 标题栏显示当前连接数；最近访问列表支持纵向滚动
- 反代详情日志结构化展示（时间、方法、路径、状态码等分列）

### 文档

- README 默认反代端口更新为 `18080` / `9443`

## [v0.1.19] - 2026-09-13

### 修复

- 反代详情日志：修复无法滚动的问题，悬停显示滚动条，默认跟随最新日志

## [v0.1.18] - 2026-09-13

### 改进

- 反代详情：概览 / 日志 Tab 切换，默认概览；日志区域撑满侧栏
- 反代列表：状态列改为开关，可直接启用/停用
- 反代「复制」改为填入新建表单（名称加 `-复制` 后缀）
- README Docker 徽章按 semver 排序，避免显示 `-amd64` 标签

### 移除

- 反代详情「设置」Tab 与拖动排序提示文案

## [v0.1.17] - 2026-09-13

### 新增

- 反代规则「备注」改为「名称」，列表以名称优先展示
- 反代规则复制当前信息到剪贴板
- 反代列表拖动排序（无筛选时）

## [v0.1.16] - 2026-09-13

### 改进

- 证书申请：首次未配置 ACME 邮箱时提示填写，前后端校验邮箱格式
- 证书列表空状态与反代/DDNS 页风格统一

## [v0.1.15] - 2026-09-13

### 修复

- Docker 发版：amd64/arm64 均构建完成后再推送 `latest` 与版本多架构 manifest
- 补全 `web/assets/image` 品牌图片资源，同步仪表盘 banner

### 改进

- arm64 镜像构建：前端与 Go 在 BUILDPLATFORM 编译，缩短 QEMU 构建时间

## [v0.1.14] - 2026-09-13

### 新增

- ZeroSSL 证书申请支持
- 反代规则备注字段；域名+端口唯一约束
- 可配置默认反代监听端口（`FONU_NGINX_HTTP_PORT` / `FONU_NGINX_HTTPS_PORT`），适配飞牛 host 部署
- `docker-compose.host.yml` 飞牛 host 模式部署示例
- 全新 Web UI：品牌视觉、仪表盘、证书/DDNS/反代/设置页重构

### 改进

- README 重写：飞牛 fnOS 部署说明、界面截图、Docker Hub 徽章
- `/api/version` 返回默认反代端口配置
- 移除仓库内 `scripts/`、`cmd/seed` 本地开发辅助

## [v0.1.13] - 2026-09-13

### 修复

- 修复证书页 TypeScript 类型错误导致 Docker 构建失败

## [v0.1.12] - 2026-09-12

### 新增

- DDNS：单任务支持多根域名，列表按域名展示解析 IP 与同步状态
- 证书：申请进度弹窗与实时日志（SSE 异步任务）；列表支持下载证书/私钥/ZIP
- 反代：按规则实时访问日志、流量统计（累计/速率）与当前连接 IP 查看
- 日志：访问/错误/运行日志分页；实时日志预载最近 100 条 Nginx 记录

### 修复

- 实时日志 SSE 因中间件未转发 Flush 导致始终为空
- Vite 开发代理对 SSE 流的缓冲问题
- 证书申请：同一 DDNS 凭证管理多个根域（如 roven.cc + chiak.cc）时校验失败
- 首次启动自动创建管理员并在运行日志输出初始密码（移除独立初始化页）
- 证书申请失败写入 `last_error` 并在列表展示
- 系统日志过滤无意义 API 轮询噪音

### 改进

- 仪表盘/证书/DDNS 列表加载优化（轻量接口与缓存刷新）
- Nginx 访问日志增加 `request_length` / `bytes_sent` 供流量统计
- 反代与证书页操作按钮横排布局

## [v0.1.11] - 2026-09-12

### 修复

- Vite 开发服务器允许经 Nginx 反代用自定义域名访问（`server.allowedHosts`）

## [v0.1.10] - 2026-09-12

### 修复

- Windows 本地 Nginx：证书/日志使用绝对路径，HTTPS 块先于 HTTP 重定向，修复 `ssl_certificate` 校验失败
- Windows 进程检测与启动：修复 `isPIDAlive` 误判、启动阻塞及重复拉起 Nginx 导致端口冲突
- 系统日志写入 `app.log`，API 请求记录状态码；实时日志连接时预载最近条目
- 未设置 `FONU_DATA_DIR` 时默认使用当前目录 `.data`

### 改进

- 新增 `scripts/dev.ps1`、`scripts/clean-data.ps1` 便于 Windows 本地开发
- 测试与文档示例域名统一为 `example.com`，移除个人域名硬编码

## [v0.1.9] - 2026-09-12

### 修复

- 反代保存/删除不再因 Nginx 重载卡住 30 秒：优先 SIGHUP 信号重载，失败时快速强制重启
- 保存/更新规则先落库再重载 Nginx，避免超时后规则未写入
- 部分成功（规则已保存但 Nginx 失败）时前端仍关闭弹窗并刷新列表

## [v0.1.8] - 2026-09-12

### 修复

- 修复反代规则删除无反应：删除失败时显示错误提示；Nginx 重载失败时不再回滚已删除的规则

## [v0.1.7] - 2026-09-12

### 改进

- 界面左下角版本号随发版标签注入，与 Docker 镜像版本一致（不再固定显示 v0.1.0）
- 新增 `GET /api/version` 接口；发版时更新根目录 `VERSION` 文件

## [v0.1.6] - 2026-09-12

### 修复

- 为 Nginx 校验/重载/启动命令增加 15 秒超时，避免反代保存时无限转圈
- 前端 API 请求增加 30 秒超时，超时后显示明确错误提示
- Nginx 重载失败时先尝试优雅停止再重启，降低端口占用导致的卡死

## [v0.1.5] - 2026-09-12

### 修复

- 反代证书匹配改为读取证书库记录与磁盘文件，导入证书可正确用于 Nginx HTTPS
- 启用 HTTPS 但域名未匹配证书时，返回明确错误（含当前证书覆盖范围）而非 Nginx 校验失败
- 修复 HTTPS 配置生成时可能遗漏 HTTP 回退的问题

## [v0.1.4] - 2026-09-12

### 修复

- 修复启用 HTTPS 但证书文件不可用时，Nginx 生成 `listen ... ssl` 却缺少 `ssl_certificate` 导致反代保存失败
- 仅在证书文件完整存在时生成 HTTPS 配置，否则自动降级为 HTTP

## [v0.1.3] - 2026-09-12

### 新增

- **反代多域名**：一条规则支持多个前端域名、自定义监听端口，以及 IPv4/IPv6 监听开关
- **单域名独立端口**：前端地址可写 `example.com:6893` 覆盖规则默认端口

### 改进

- 本地开发未安装 Nginx 时，保存反代规则仍写入配置并跳过校验，便于调试 UI

### 数据库迁移

- `006_proxy_multi_host.sql` — 反代规则多域名与监听配置

## [v0.1.2] - 2026-09-12

### 修复

- 修复 Nginx PID 文件为空或进程已退出时仍尝试重载，导致反代规则保存失败

## [v0.1.1] - 2026-09-12

### 修复

- 修复全新部署时 API 返回 `null` 导致仪表盘/日志等页面空白、`map` 报错
- 修复 Nginx 重载使用错误 PID 路径（`/run/nginx.pid`）导致反代保存失败
- 公网 IP 检测增加多个备用服务，降低 Docker 环境下单点失败概率
- 修复 SPA 路由刷新时未正确回退到 `index.html`

## [v0.1.0] - 2026-09-12

### 新增

- **DDNS 多域名**：支持多条 DDNS 配置，独立管理根域名与记录
- **证书多域名**：申请/续签支持多域名与通配符（`*.example.com`）
- **ACME 颁发机构**：申请与续签时可选择 Let's Encrypt 正式/测试环境
- **证书导入**：支持粘贴 PEM 或指定文件路径导入自签/已有证书
- **Docker 发版**：推送 `v*` 标签自动分架构推送镜像（`latest` / `0.1.0` / `0.1.0-amd64` / `0.1.0-arm64`）
- **表格操作规范**：所有列表操作按钮平铺展示（`renderTableRowActions`）

### 改进

- **仪表盘**：全新布局，ECharts 图表，状态卡片与日志预览
- **日志页**：分页（20 条/页），支持 `?tab=` 跳转，统一时间格式
- **证书页**：概览统计、申请弹窗优化、表格自适应宽度
- **布局**：顶栏时间靠左、操作靠右，内容区铺满主区域
- **主题**：深色模式与组件样式统一

### 数据库迁移

- `003_ddns_multi.sql` — DDNS 多配置
- `004_cert_acme_ca.sql` — 证书 ACME CA 字段
- `005_cert_domains.sql` — 证书多域名
