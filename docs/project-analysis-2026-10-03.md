# Havline 项目结构与实现分析（v1.9.0）

> 分析日期：2026-10-03  
> 分析对象：`F:\Fonu-main\havline`  
> 版本来源：根目录 `VERSION`，当前为 `1.9.0`  
> 分析方式：重新扫描工作区目录，阅读启动入口、应用装配、路由、配置、Docker 构建、Compose 编排、前端路由、数据库迁移和现有文档。本文是当前目录的结构快照，不替代带日期的历史方案文档。

## 一、结论摘要

Havline 是面向飞牛 fnOS 和家庭 NAS 的公网访问管理控制台。项目以 Go 单体应用作为本地控制面，以 Vue 3 管理后台作为操作界面，以 SQLite 和 `/data` 数据目录保存状态，并通过 Nginx、FRP、Cloudflare Tunnel、DDNS 和 ACME 证书把内网服务发布到公网。

当前版本已经从单一的本机反向代理工具扩展为多角色运维系统：

1. 本机 Nginx 反向代理与配置版本管理；
2. 多 DNS 服务商 DDNS 与记录同步；
3. ACME 证书申请、续期、导入和下载；
4. FRP 多服务端、代理规则、运行状态、流量和诊断；
5. 公网 `havline-agent` 对 VPS 上的 frps、Nginx、证书和路由进行远程管理；
6. Cloudflare Tunnel 的本地托管、路由同步、DNS、健康和漂移管理；
7. 日志、指标、通知、备份恢复、服务发现和一键升级。

从工程形态看，项目的核心不是简单 CRUD，而是「控制面 + 外部进程编排 + 远程节点管理 + 配置生成与回滚 + 周期任务」的组合系统。复杂度最高的区域仍然是 FRP/Agent；Cloudflare Tunnel 已成为第二个需要持续关注的外部系统集成边界。

## 二、目录边界与工作区现状

外层 `F:\Fonu-main` 是工作区容器，包含多个交付形态和临时产物；当前主源码目录是 `havline`，而不是主动文件路径中显示的历史 `Fonu-1.0.2` 目录。

```text
F:\Fonu-main
├─ havline/                  当前主源码与 Git 项目内容
│  ├─ cmd/                   Go 程序入口
│  ├─ internal/              后端领域模块
│  ├─ migrations/            SQLite 数据库迁移 001–036
│  ├─ web/                   Vue 3 + TypeScript 前端
│  ├─ docs/                  当前项目文档、方案和审查记录
│  ├─ scripts/               构建、安装、升级脚本
│  ├─ Dockerfile             多阶段 Docker 构建
│  ├─ docker-compose*.yml    三种 Compose 部署形态
│  ├─ VERSION                版本单一来源，当前 1.9.0
│  └─ CHANGELOG.md           版本变更记录
├─ fnos/                     fnOS 打包交付物
├─ fnos-native/              fnOS 原生应用工程及其 app-src 镜像
├─ havline-docs/             独立 Nuxt 文档站点
├─ docs/                     外层临时资料和 FRP 迁移资料
└─ .tmp-* / 日志 / 数据库    本地构建、测试和运行产物
```

`havline` 目录内当前可见的结构规模：

| 区域 | 当前观察 |
| --- | --- |
| Go 后端 | `internal` 下约 281 个 Go 文件，包含业务代码和测试代码 |
| 数据库迁移 | 36 个 SQL 迁移，从 `001_init.sql` 到 `036_cf_health.sql` |
| 前端页面 | 14 个 Vue 页面，覆盖登录、仪表盘、代理、FRP、Cloudflare、日志和设置 |
| 前端技术 | Vue 3.5、TypeScript、Vite、Naive UI、ECharts、CodeMirror、Vitest |
| 后端技术 | Go 1.23、标准库 HTTP、SQLite、ACME、SSH、多个 DNS/Cloudflare SDK |
| 交付架构 | Docker bridge、Docker host、Git URL 构建、Linux 原生部署、fnOS 应用打包 |

外层还存在 `.tmp-guide-data`、`.tmp-native`、`.tmp-push`、`.data` 和日志等运行或验证产物。它们适合排查本地环境，不应被当成源码、正式数据迁移或发布文档的来源。

## 三、总体架构

```mermaid
flowchart LR
    Browser[浏览器] -->|HTTP :6893| App[Havline 控制面]
    App --> DB[(SQLite /data/havline.db)]
    App --> FS[/data 数据目录]
    App --> Nginx[Nginx 配置与进程]
    Nginx --> NAS[NAS 内网服务]
    App --> DNS[DNS 服务商]
    App --> ACME[ACME CA]
    App --> Scheduler[后台调度器]
    App --> FRPC[本机 frpc]
    FRPC --> FRPS[公网 VPS frps]
    App --> Agent[公网 havline-agent]
    Agent --> VPSNginx[VPS Nginx]
    App --> Cloudflared[本机 cloudflared]
    Cloudflared --> Cloudflare[Cloudflare Edge]
    App --> Updater[受限 updater sidecar]
```

### 3.1 运行角色

| 角色 | 位置 | 主要职责 |
| --- | --- | --- |
| `havline` | NAS、Docker 或 Linux 主机 | 管理后台、本机 Nginx、DDNS、证书、FRP、Cloudflare、调度器和数据 |
| `havline-agent` | 公网 VPS | 远程管理 VPS Nginx、frps、证书、路由和诊断 |
| Nginx | NAS 与 VPS 两端 | 提供本机反代、HTTPS 终止和公网入口 |
| frpc / frps | NAS / VPS | 在没有公网入站条件时建立 FRP 隧道 |
| cloudflared | NAS | 将选定的服务通过 Cloudflare Tunnel 发布 |
| updater | Docker sidecar | 持有 Docker Socket，通过 Unix Socket 接收受限升级请求 |
| SQLite | `/data/havline.db` | 保存配置、账号、运行状态、健康事件和历史数据 |

### 3.2 典型访问链路

本机反代链路：

```text
公网客户端 → NAS Nginx → NAS 内网服务
```

FRP 链路：

```text
公网客户端 → VPS DNS/Nginx → frps → frpc → NAS Nginx → 内网服务
```

Cloudflare 链路：

```text
公网客户端 → Cloudflare Edge → cloudflared → NAS 内网服务
```

管理链路：

```text
浏览器 → Havline HTTP API → 领域 Service → SQLite / 文件系统 / 外部进程 / 远程 Agent
```

## 四、启动与依赖装配

主入口是 `cmd/havline/main.go`，启动过程较集中：

1. `config.Load()` 读取监听地址、数据目录、Nginx、frpc 和 updater 配置；
2. `app.ResolveMigrationsDir()` 从当前目录及上级候选目录寻找迁移文件；
3. 通过 `go:embed` 将 `web/dist` 嵌入主程序；
4. `app.New()` 创建数据目录和日志目录；
5. 解析会话密钥；
6. 打开 SQLite，按序执行迁移；
7. 创建加密盒、管理员服务、各领域 Store/Service；
8. 注册 API 路由和静态 SPA；
9. 启动 FRP Agent 隧道、Cloudflare 隧道、定时调度、流量采集；
10. 初始化 Nginx 配置，恢复 FRP 运行状态并执行自动启动；
11. 监听 `SIGINT` / `SIGTERM`，按超时上下文关闭 HTTP、Nginx、FRP 和数据库。

`internal/app/app.go` 是整个后端的组合根。它的优点是依赖关系可直接追踪；风险是任何新模块都需要修改装配代码，长期可能继续增大启动函数的耦合度。

## 五、后端模块职责

### 5.1 API 层

`internal/api/router.go` 集中注册 HTTP API，并通过 `routeSpec` 标记公开路由和受保护路由。当前公开接口主要包括版本、登录状态、登录、登出和可选的公开状态页；业务接口默认经过 Session Middleware 和 `RequireAuth`。

API 领域覆盖：

- `/api/proxies`：本机反向代理、日志流、客户端和 Nginx 规则；
- `/api/ddns`：DDNS 任务、测试和立即同步；
- `/api/certificates`：证书列表、申请、导入、续期和下载；
- `/api/frpmulti`：FRP 服务端、代理、运行时、Agent、诊断、指标和路由；
- `/api/cloudflare`：隧道、路由、DNS、健康、漂移、二进制和设置；
- `/api/backup`：导出、归档、下载、恢复和删除；
- `/api/logs`、`/api/metrics`：日志流、分页日志和指标历史；
- `/api/system/update`：升级状态和执行升级；
- `/api/settings`：系统设置、通知、Token 和中国 IP 段。

路由注册集中有利于鉴权覆盖测试，但文件本身已经较长。新增 API 时应同时补充处理器测试、鉴权覆盖和前端 API 类型，避免只注册后端路径而遗漏客户端契约。

### 5.2 领域模块

| 包 | 主要职责 |
| --- | --- |
| `config` | 环境变量、数据目录、默认端口、会话密钥解析 |
| `db` | SQLite 打开、迁移和数据库基础操作 |
| `auth` | 管理员、会话、密码和 API Token |
| `secret` | 敏感配置的加密存储 |
| `proxy` / `service` | 代理规则、出口、端口校验和业务编排 |
| `nginx` / `nginxtmpl` | Nginx 配置生成、语法检查、启动、重载、版本和回滚 |
| `ddns` | Cloudflare、DNSPod、阿里云、腾讯云、火山引擎等 DNS 服务商 |
| `acme` / `certificate` | ACME DNS-01、证书任务、导入、下载和元数据 |
| `frp` | 多服务端 FRP、frpc 进程、frps 配置、Agent、健康、漂移和流量 |
| `agent` | 公网 Agent HTTP 服务、路由、版本、传输和防火墙 |
| `cloudflared` | Cloudflare API、隧道进程、ingress、DNS、健康和漂移 |
| `monitor` / `metrics` | 周期巡检、健康事件和指标历史 |
| `scheduler` | DDNS、ACME、中国 CIDR、巡检和自动备份等周期任务 |
| `backup` | 数据导出、归档、恢复和自动备份保留策略 |
| `notify` | 邮件、Webhook、Telegram、钉钉、飞书和企业微信 |
| `logstore` / `traffic` | 应用日志、Nginx 日志、访问解析、流量和连接统计 |
| `discovery` | NAS 常见服务发现与候选配置 |
| `updater` | 主程序与升级 sidecar 之间的 Unix Socket 协议 |

### 5.3 外部进程与文件系统边界

Havline 不只读写数据库，还会生成和维护以下运行资源：

- `/data/nginx/nginx.conf`、规则目录、自定义配置和配置备份；
- `/data/certs` 证书和私钥；
- `/data/frp/frpc.toml`、PID 文件和运行日志；
- Cloudflare Tunnel 的二进制、配置和凭据；
- `/data/logs` 应用、Nginx 和 FRP 日志；
- `/data/ssh_known_hosts`，用于 SSH 主机密钥 TOFU 校验；
- `/data/session_secret`，用于未显式配置时持久化会话密钥。

这意味着备份、升级和恢复不能只关注 SQLite。完整实例恢复至少需要数据库、证书、Nginx 自定义配置、FRP/Cloudflare 运行文件和会话密钥保持一致。

## 六、数据与安全模型

### 6.1 数据持久化

Compose 默认把命名卷 `havline-data` 挂载到容器 `/data`，也可通过 `HAVLINE_DATA_PATH` 改为绑定目录。主数据库为 `/data/havline.db`，迁移已覆盖：

- 管理员、会话和系统设置；
- 本机代理及多 Host、端口、安全、流量和出口；
- DDNS 任务、域名记录和自定义公网 IP；
- 证书、ACME CA 和证书域名；
- FRP 服务端、选项、代理、运行状态、诊断、Agent 和健康；
- API Token、路由健康、指标采样；
- Cloudflare Tunnel、全局设置、API Token、Account ID 和健康事件。

截至当前目录，迁移编号到 `036_cf_health.sql`，说明 Cloudflare 健康事件已经进入数据库持久化边界。

### 6.2 会话密钥

`Config.ResolveSessionSecret()` 的实际优先级是：

1. 显式环境变量 `HAVLINE_SESSION_SECRET`；
2. 数据目录中的 `session_secret`；
3. 不存在或明显损坏时生成 32 字节随机值，以 64 个十六进制字符写入并设置为 `0600`。

该密钥同时用于会话签名和敏感凭据加密。已有数据后更换密钥会导致已保存的 DDNS、FRP、通知和 Cloudflare 等凭据无法解密，因此部署文档必须把密钥持久化和迁移作为数据恢复的一部分。

### 6.3 鉴权边界

管理 API 默认通过会话中间件和认证检查；API Token 用于自动化访问。公开状态页是唯一明确设计为匿名读取的业务页面，代码中还保留按 IP 限流和访问码控制的语义。远程 Agent 使用独立 Bearer Token、SSH 安装/诊断和可选的传输层能力。

主要安全关注点：

- updater sidecar 挂载 Docker Socket，必须限制其 Unix Socket 调用面和宿主机项目目录；
- Agent 若监听非回环地址，必须配合 Token、TLS/隧道和防火墙策略；
- 证书私钥、DNS 凭据、FRP Token 和 Cloudflare Token 需要同时保护数据库、数据卷和备份归档；
- 状态页和日志接口不能意外泄露内部域名、凭据或运行细节；
- Compose 文件中的 Git 构建地址、更新仓库和项目目录必须与实际发布源保持一致。

## 七、前端结构

前端入口通过 Vue Router 将页面分为公开区域和登录后的 AppLayout 区域：

```text
/login                         登录
/status                        公开状态页
/dashboard                     仪表盘
/ddns                          DDNS
/certificates                  证书管理
/proxies                       本机反向代理
/frp                           内网穿透
/frp-agent                     公网 Agent
/cloudflare                    Cloudflare 隧道列表
/cloudflare/:id/config         隧道独立配置页
/public-proxy                  公网反代
/logs                          日志中心
/settings                      系统设置
/about                         关于项目
```

前端职责主要集中在页面组件、API Client、类型定义、状态/格式化工具和布局组件。技术栈包含：

- Vue 3.5 与 Vue Router 4；
- TypeScript 5.7 与 `vue-tsc`；
- Vite 6；
- Naive UI；
- ECharts 与 `vue-echarts`；
- CodeMirror 6，用于配置编辑；
- SortableJS，用于规则排序；
- Vitest，用于单元测试。

需要持续控制的维护成本是页面组件体量。代理、设置、Agent 和 Cloudflare 页面承载了较多状态、弹窗、表单、配置编辑和运行操作，后续新增功能应优先抽取可复用的表单、状态卡片、日志流和配置编辑子组件，避免继续扩大单文件复杂度。

## 八、部署与构建

### 8.1 Dockerfile

当前 Dockerfile 是多阶段构建：

1. 从 `VERSION` 读取版本；
2. 使用 Node 镜像构建前端，并将版本注入 `VITE_APP_VERSION`；
3. 使用 Go 镜像下载依赖并交叉编译 amd64/arm64 的 `havline-agent`；
4. 将前端 `dist` 复制到 `cmd/havline/web/dist`，由 Go `embed` 打入主程序；
5. 编译主程序并通过 `-ldflags` 注入后端版本；
6. 单独构建 updater sidecar；
7. 运行时镜像安装 Nginx、CA 证书和 tzdata，默认暴露 6893、80、443。

当前运行镜像不预装 `frpc`，内网穿透需要在应用内按需下载并保存到数据目录。文档若仍写成“镜像内置 frpc”，应按当前 Dockerfile 修正。

### 8.2 Compose 形态

| 文件 | 场景 | 特征 |
| --- | --- | --- |
| `docker-compose.yml` | 本地源码构建 | bridge 网络、主服务和 updater sidecar |
| `docker-compose.host.yml` | fnOS 等已占用 80/443 | host 网络，默认反代端口 18080/9443 |
| `docker-compose.hub.yml` | 只保留编排文件部署 | BuildKit 从 Git URL 拉源码构建，适合干净机器 |

三个 Compose 文件都使用固定项目名 `havline`，并支持 `HAVLINE_DATA_PATH` 将默认命名卷替换为绑定目录。`docker-compose.hub.yml` 当前从 `https://jayhub.top/mrlee/Havline.git#main` 构建和升级，部署环境需要确认该地址、分支和实际发布仓库的关系。

### 8.3 一键升级

Docker 模式由主服务和 updater sidecar 组成：主服务只通过共享 Unix Socket 请求升级，sidecar 才挂载 Docker Socket。升级过程涉及拉取/构建目标代码、重建服务、健康检查和失败回滚。

该设计降低了主业务进程直接控制 Docker 的权限，但 updater 仍然拥有高权限宿主机能力。升级协议、项目目录挂载、Compose 文件名、服务名、镜像标签和回滚条件必须作为一个整体测试。

## 九、关键业务流程

### 9.1 新建本机反向代理

```text
前端表单
  → POST /api/proxies
  → Proxy Handler / Proxy Service
  → 写入 SQLite
  → 生成 Nginx 规则与证书引用
  → 语法检查
  → 原子写入和备份
  → Nginx reload
  → 可选同步 Cloudflare ingress/DNS
```

### 9.2 DDNS 与证书

DDNS 任务由 scheduler 周期触发，也可通过 API 手动执行。证书申请使用 ACME DNS-01，凭据从加密存储读取，申请过程通过 Job 和日志流反馈给前端，成功后更新证书元数据并供 Nginx 或远程 Agent 使用。

### 9.3 FRP 与 Agent

本地 Havline 保存服务端和代理定义，负责生成 `frpc.toml`、下载/激活 FRP 二进制、启动和停止本地进程。公网 Agent 通过 SSH 安装或管理，随后由 Havline 调用 Agent API 完成 frps、Nginx、证书、路由和诊断操作。路由漂移、健康、版本和配置回滚使该链路具备运维闭环，但也使错误恢复路径较长。

### 9.4 Cloudflare Tunnel

反向代理规则可以选择本机出口或 Cloudflare 出口。托管隧道从规则派生 ingress，并负责本地配置、Cloudflare 云端路由、DNS 记录、隧道进程、健康事件和漂移修复。隧道配置页单独呈现运行指标、拓扑、路由和控制操作，说明该功能已经从实验性集成进入主要业务边界。

## 十、现有文档与实现的偏差

本次读取发现，正式代码已经进入 v1.9.0，但部分历史文档仍停留在早期版本，使用前应区分“历史快照”和“当前操作说明”：

1. `docs/DEPLOYMENT.md` 顶部仍标注适用版本 v1.1.0，迁移数量、包名和若干功能描述明显早于当前实现；
2. 该部署文档仍强调手工设置会话密钥，并按历史固定默认值描述风险，而当前代码在未配置时会生成并持久化随机密钥；
3. `docs/README.md` 已将 `project-analysis-2026-09-27.md` 标记为 v1.2.19 快照，不能代表当前 v1.9.0 的完整 Cloudflare 和品牌状态；
4. `CHANGELOG.md` 的 `[Unreleased]` 仍包含品牌全量切换说明，而 v1.9.0 已记录 Cloudflare 功能，版本状态需要在发布流程中明确；
5. 当前代码、Compose 和环境变量已统一使用 `HAVLINE_*`，旧 `FONU_*` 只应在历史文档中保留，并需要避免被新部署步骤继续复制；
6. `fnos-native/app-src` 是另一个包含源码副本的交付边界，修改主 `havline` 后若需要发布 fnOS 原生包，必须明确同步策略，不能默认认为两个目录自动一致。

建议把文档分成三类：当前部署手册、当前功能基线、历史分析/方案。历史文件保留原内容，但标题和索引应明确版本和时效，避免用户把 v1.1.0 的命令用于 v1.9.0。

## 十一、风险与改进优先级

### P0：发布与数据安全

- 更新部署文档，使会话密钥、数据卷、备份和当前 Compose 变量与代码一致；
- 验证 updater sidecar 的 Socket 鉴权、项目目录边界、升级失败回滚和异常重启；
- 对数据库、证书、Token、私钥和备份归档建立统一的敏感数据保护说明；
- 明确 `docker-compose.hub.yml` 的 Git 地址、分支和发布仓库，避免升级拉到非预期源。

### P1：复杂集成稳定性

- 为 FRP/Agent/Cloudflare 的“配置写入 → 进程重载 → 健康检查 → 漂移修复”建立端到端测试；
- 把远程 Agent 调用的超时、重试、版本兼容和错误恢复状态统一建模；
- 验证 Cloudflare DNS 清理只影响 Havline 接管的记录；
- 检查迁移从已有实例升级到 `036` 的完整性，并验证备份恢复后凭据和运行文件可用。

### P2：可维护性

- 拆分大型 Vue 页面和大型 FRP/Agent 服务文件；
- 统一 API 类型、错误码、异步 Job、日志流和状态映射；
- 继续保持路由鉴权覆盖测试，并为每个外部进程管理器补充失败注入测试；
- 将重复的配置版本、原子写入和回滚逻辑沉淀到已有 `fsutil` 或明确的领域接口中。

### P3：交付一致性

- 明确 `havline`、`fnos-native/app-src`、fnOS `.fpk` 和独立文档站点之间的发布关系；
- 在 CI 中增加版本源、Compose 变量、文档版本和嵌入前端资源的一致性检查；
- 清理或隔离外层临时数据库、日志和构建产物，减少分析时误把运行状态当源码事实的风险。

## 十二、建议的后续验证清单

在把本分析作为发布依据前，建议在 `havline` 目录执行：

```bash
go test ./... -count=1
go vet ./...
cd web
npm run typecheck
npm test
npm run build
```

部署验证至少覆盖：

- 空数据目录首次启动并确认随机 `session_secret` 权限；
- 旧数据卷升级并确认数据库迁移全部完成；
- Compose bridge、host、hub 三种形态分别启动；
- Nginx 配置写入、语法错误、回滚和重载失败；
- FRP 本地进程、VPS Agent 和路由漂移；
- Cloudflare 隧道、DNS、ingress 同步、健康事件和恢复；
- 备份导出、全量恢复、凭据解密和证书/配置文件恢复；
- updater sidecar 升级成功、健康检查失败和回滚。

## 十三、最终判断

当前 `havline` 已经是一个功能较完整的 NAS 公网访问控制面，代码、部署和文档均已完成从 Fonu 到 Havline 的大部分迁移。架构主线清晰：Go 负责控制面和外部系统编排，Vue 负责交互，SQLite 负责状态，文件系统负责运行配置和密钥，Nginx/FRP/Cloudflare 负责不同公网出口。

项目下一阶段的主要工作不在于继续堆叠入口，而在于收敛交付事实：同步当前部署文档、强化高权限升级边界、保证数据库与文件数据的一致备份恢复、降低 FRP/Agent/Cloudflare 集成的故障恢复成本，并把历史方案与当前实现明确分层。
