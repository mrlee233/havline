<p align="center">
  <img src="web/src/assets/brand/havline-logo.svg" alt="Havline Logo" width="96" />
</p>

<h1 align="center">Havline</h1>

<p align="center"><strong>让 NAS 访问更简单</strong></p>

<p align="center">
  面向 <strong>飞牛 fnOS</strong> 的轻量公网访问管理工具<br />
  反向代理 · DDNS · HTTPS 证书 · 内网穿透 · Cloudflare Tunnel · 一站式 Web 管理
</p>

<p align="center">
  <a href="https://github.com/mrlee233/havline/releases"><img src="https://img.shields.io/badge/version-1.9.0-22c55e?style=flat-square" alt="发布版本" /></a>
  <img src="https://img.shields.io/badge/目标平台-飞牛_fnOS-22c55e?style=flat-square" alt="飞牛 fnOS" />
  <img src="https://img.shields.io/badge/其他环境-未测试-94a3b8?style=flat-square" alt="其他环境未测试" />
  <img src="https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Vue-3-4FC08D?style=flat-square&logo=vuedotjs&logoColor=white" alt="Vue 3" />
  <img src="https://img.shields.io/badge/Nginx-内置-009639?style=flat-square&logo=nginx&logoColor=white" alt="Nginx" />
  <a href="https://www.gnu.org/licenses/gpl-3.0.html"><img src="https://img.shields.io/badge/license-GPL--3.0-blue?style=flat-square" alt="GPL-3.0" /></a>
</p>

---

## 简介

**Havline** 是一款主要为 **飞牛 fnOS** 编写的反代与管理工具，帮助你在 NAS 上快速搭建稳定的公网访问能力：通过 Web 界面管理 Nginx 反向代理、自动同步 DDNS、申请与续期 Let's Encrypt 证书，并在一个仪表盘里查看服务状态、访问日志与流量统计。

无需手写 Nginx 配置，也无需在多个工具之间切换。当前开发与测试环境以飞牛 fnOS 为主，**群晖、威联通、自建 Linux 等其他环境尚未充分测试**，部署前请自行验证。

Havline 基于开源项目 [O96u/Fonu](https://github.com/O96u/Fonu) **1.0.2** 继续开发，保留上游在反向代理、DDNS、证书和内网穿透上的基础能力，并继续扩展公网 Agent、Cloudflare Tunnel、一键升级和飞牛应用交付。

## 功能特性

| 模块           | 说明                                                                                           |
| -------------- | ---------------------------------------------------------------------------------------------- |
| **反向代理**   | 多域名、多端口、HTTP/HTTPS、自动重定向；实时访问日志、流量统计、当前连接 IP                    |
| **DDNS**       | 支持 Cloudflare、DNSPod、阿里云、腾讯云、火山引擎 DNS；单任务多根域名，自动同步公网 IP         |
| **HTTPS 证书** | ACME 自动申请与续期；证书/私钥/ZIP 下载；申请进度实时日志                                      |
| **仪表盘**     | 公网 IP、域名、证书、服务状态一览；请求趋势与运行健康度                                        |
| **日志中心**   | 系统日志、Nginx 访问日志、Nginx 错误日志；分页筛选与自动刷新                                   |
| **内网穿透**   | FRP 多服务端管理，支持 TCP/UDP/HTTP/HTTPS 等代理类型；客户端按需下载、配置与运行诊断 |
| **公网 Agent** | 通过 SSH 安装和维护 VPS Agent，远程管理 frps、Nginx、证书和路由 |
| **Cloudflare Tunnel** | cloudflared 按需下载、隧道与路由管理、DNS 同步、健康检查和漂移修复 |
| **一键升级** | 含 updater 的 Compose 部署支持镜像构建、主服务重建和版本健康检查 |
| **系统设置**   | 常规 / 通知 / 安全 / 高级分栏；邮件/Webhook/Telegram 告警、备份与恢复                          |
| **其他**       | 深色模式、配置导出/导入、版本更新提示                                                           |

## 相比上游 Fonu 的主要增量

| 方向 | Havline 增量 |
| --- | --- |
| **公网服务端** | 通过 SSH 安装、升级和维护 VPS 上的 `havline-agent`，远程管理 frps、Nginx、证书和公网反代 |
| **FRP 管理** | 多服务端管理、FRP 客户端按需下载、代理规则校验、运行诊断、路由健康与漂移检测 |
| **Cloudflare Tunnel** | cloudflared 按需下载、Account/API Token 预检、隧道与路由管理、DNS 同步、健康巡检和漂移修复 |
| **一键升级** | Compose + updater sidecar，支持构建/重建/健康检查，并在部分失败场景下尝试回滚 |
| **飞牛交付** | 提供 Docker 离线镜像包与 Native 应用两种形态，支持统一网关 `/app/havline` 接入 |
| **文档体系** | 增加图文操作手册、部署/升级/迁移文档、飞牛打包方案和 Cloudflare/FRP 专题方案 |

## 界面展示

<table>
  <tr>
    <td align="center" width="50%">
      <b>仪表盘</b><br>
      <a href="docs/screenshots/guide-dashboard.png"><img src="docs/screenshots/guide-dashboard.png" alt="仪表盘" width="100%" /></a>
    </td>
    <td align="center" width="50%">
      <b>反向代理</b><br>
      <a href="docs/screenshots/guide-proxy.png"><img src="docs/screenshots/guide-proxy.png" alt="反向代理" width="100%" /></a>
    </td>
  </tr>
  <tr>
    <td align="center">
      <b>新建反向代理规则</b><br>
      <a href="docs/screenshots/guide-dialog-proxy-create.png"><img src="docs/screenshots/guide-dialog-proxy-create.png" alt="新建反向代理规则" width="100%" /></a>
    </td>
    <td align="center">
      <b>DDNS 添加任务</b><br>
      <a href="docs/screenshots/guide-dialog-ddns-add.png"><img src="docs/screenshots/guide-dialog-ddns-add.png" alt="DDNS 添加任务" width="100%" /></a>
    </td>
  </tr>
  <tr>
    <td align="center">
      <b>证书申请</b><br>
      <a href="docs/screenshots/guide-dialog-cert-apply.png"><img src="docs/screenshots/guide-dialog-cert-apply.png" alt="证书申请" width="100%" /></a>
    </td>
    <td align="center">
      <b>内网穿透</b><br>
      <a href="docs/screenshots/guide-frp.png"><img src="docs/screenshots/guide-frp.png" alt="内网穿透" width="100%" /></a>
    </td>
  </tr>
  <tr>
    <td align="center">
      <b>公网服务端</b><br>
      <a href="docs/screenshots/guide-agent.png"><img src="docs/screenshots/guide-agent.png" alt="公网服务端" width="100%" /></a>
    </td>
    <td align="center">
      <b>Cloudflare 隧道</b><br>
      <a href="docs/screenshots/guide-cloudflare.png"><img src="docs/screenshots/guide-cloudflare.png" alt="Cloudflare 隧道" width="100%" /></a>
    </td>
  </tr>
  <tr>
    <td align="center">
      <b>Cloudflare 新建隧道</b><br>
      <a href="docs/screenshots/guide-dialog-cf-create.png"><img src="docs/screenshots/guide-dialog-cf-create.png" alt="Cloudflare 新建隧道" width="100%" /></a>
    </td>
    <td align="center">
      <b>系统设置</b><br>
      <a href="docs/screenshots/guide-settings.png"><img src="docs/screenshots/guide-settings.png" alt="系统设置" width="100%" /></a>
    </td>
  </tr>
</table>

更多页面、弹窗和逐步操作说明见 [`docs/user-guide.md`](docs/user-guide.md)。

## 技术栈

| 层级         | 技术                                                                   |
| ------------ | ---------------------------------------------------------------------- |
| **后端**     | Go 1.23、标准库 HTTP、SQLite（modernc.org/sqlite）                     |
| **反向代理** | 内置 Nginx（动态生成配置、热重载）                                     |
| **证书**     | go-acme/lego（ACME DNS-01）                                          |
| **DDNS**     | Cloudflare / DNSPod / 阿里云 / 腾讯云 / 火山引擎 DNS API               |
| **内网穿透** | FRP 客户端按需下载；多服务端和代理规则管理，支持公网 Agent |
| **隧道** | Cloudflare Tunnel / cloudflared |
| **前端**     | Vue 3、TypeScript、Vite、Naive UI、ECharts                             |
| **部署**     | Docker 多架构镜像（amd64 / arm64）、GitHub Actions CI                  |

## 项目结构

| 路径 | 说明 |
| --- | --- |
| `cmd/havline` | 主程序入口，提供 Web 管理后台、API、调度和内置 Nginx/FRP 管理 |
| `cmd/havline-agent` | 公网 VPS 上的 Agent，接收主程序下发并管理 frps、Nginx、证书和路由 |
| `cmd/havline-updater` | 一键升级 sidecar，仅它挂载 Docker Socket，并通过 Unix Socket 与主程序通信 |
| `internal/api` | HTTP 路由、鉴权中间件、请求处理器和 API 响应封装 |
| `internal/app` | 应用启动、服务装配、统一网关和运行生命周期 |
| `internal/nginx`、`internal/nginxtmpl` | Nginx 配置生成、共享模板、安全/限流/geo/TLS 片段和热重载 |
| `internal/proxy`、`internal/service` | 反向代理规则、证书绑定、访问策略和 Nginx 配置编排 |
| `internal/frp` | FRP 多服务端、规则、运行进程、Agent 安装、诊断、健康和漂移检测 |
| `internal/cloudflared` | Cloudflare Tunnel 设置、隧道、路由、DNS、健康事件和漂移修复 |
| `internal/agent` | Agent 服务端实现，包括鉴权、监听策略、防火墙提示、Nginx/frps 操作 |
| `internal/updater` | updater 服务端、客户端、执行器和远端版本读取 |
| `internal/ddns`、`internal/certificate`、`internal/acme` | DDNS 服务商、证书存储、导入和 ACME 申请续期 |
| `internal/backup`、`internal/notify`、`internal/monitor`、`internal/metrics` | 备份恢复、通知渠道、监控采集和指标导出 |
| `web/src` | Vue 3 前端，包含 API 客户端、组合式函数、组件、路由、样式和页面视图 |
| `migrations` | SQLite 数据库迁移 |
| `docs` | 项目方案、部署、升级、操作手册和飞牛应用文档 |
| `docker-compose*.yml` | Host、Bridge 和单文件 Git 构建部署编排 |

## 部署

源码仓库：[https://github.com/mrlee233/havline](https://github.com/mrlee233/havline) · [发布版本](https://github.com/mrlee233/havline/releases)

以下方式均从源码构建，不使用上游 Docker 镜像。

| 场景 | 推荐方式 | 说明 |
| --- | --- | --- |
| 飞牛 fnOS 常规安装 | Host 模式 Compose | 避开系统已占用的 `80/443`，默认使用 `6893/18080/9443` |
| 非 host 网络或自定义端口映射 | Bridge 模式 Compose | 主服务和 updater 通过端口映射访问，适合测试或自建 Linux |
| 只拿一个 Compose 文件部署 | 单文件 Git 构建 | updater 从远端仓库读取版本并构建，本地不需要完整源码 |
| 飞牛应用商店安装 | Docker 离线包或 Native FPK | 见下方「飞牛 fnOS 应用」章节 |

### 快速部署（推荐 · 飞牛 fnOS · Host 模式）

飞牛系统已占用 **80/443**，使用 **host 网络** + **高位反代端口**，避免与系统 Nginx 冲突。

```bash
git clone --branch main https://github.com/mrlee233/havline.git
cd havline
docker compose -f docker-compose.host.yml up -d --build
```

| 入口               | 地址                    |
| ------------------ | ----------------------- |
| 管理后台           | `http://<NAS-IP>:6893`  |
| HTTP 反代（默认）  | `http://<NAS-IP>:18080` |
| HTTPS 反代（默认） | `https://<NAS-IP>:9443` |

- 默认命名卷挂载到 `/data`，存放数据库、Nginx 配置、证书与日志；可通过 `HAVLINE_DATA_PATH` 选择绑定目录，已有数据须先迁移
- Host 编排设置反代默认端口为 `18080`、`9443`，可通过环境变量修改；该编排不包含 updater

### 其他部署方式

<details>
<summary>本地源码 + Bridge 网络</summary>

在已克隆的源码目录中执行：

```bash
umask 077
# 只在尚未配置时追加，保留已有环境变量和密钥
if ! test -f .env || ! grep -q "^HAVLINE_UPDATER_TOKEN=" .env; then
  token=$(openssl rand -hex 32) && printf "\nHAVLINE_UPDATER_TOKEN=%s\n" "$token" >> .env
fi
docker compose -f docker-compose.yml up -d --build
```

默认映射 `6893:6893`、`18080:80`、`9443:443`。主服务与 updater 共享 Token，只有 updater 挂载 Docker Socket。

</details>

<details>
<summary>单文件 Git 构建（不需要本地源码）</summary>

```bash
mkdir -p /opt/havline && cd /opt/havline
curl -fsSLo docker-compose.hub.yml \
  https://github.com/mrlee233/havline/raw/branch/main/docker-compose.hub.yml
umask 077
# 只在尚未配置时追加，保留已有环境变量和密钥
if ! test -f .env || ! grep -q "^HAVLINE_UPDATER_TOKEN=" .env; then
  token=$(openssl rand -hex 32) && printf "\nHAVLINE_UPDATER_TOKEN=%s\n" "$token" >> .env
fi
docker compose -f docker-compose.hub.yml up -d --build
```

BuildKit 从当前仓库 `main` 分支构建，默认使用 Bridge 端口映射。`HAVLINE_VERSION` 只控制镜像标签，不锁定源码提交。

</details>

<details>
<summary>手动构建镜像</summary>

```bash
docker build -t havline:local .
```

在源码根目录执行；构建镜像不会自动启动容器，部署时仍需选择合适的 Compose 编排。

</details>

### 首次登录

1. 启动后查看容器/运行日志，获取自动生成的 **admin 初始密码**
2. 访问 `http://<NAS-IP>:6893/login` 登录
3. 在 **设置** 中立即修改管理员密码

### FRP 内网穿透（无公网 IP）

适用于无法端口映射、无公网 IPv4 的场景。Havline 在 **内网穿透** 页面统一管理多个服务端与代理规则，**frpc 按需下载**：

- **Web 网关**：外网 HTTP/HTTPS 经 VPS 上的 frps 中转至本地 Havline Nginx（默认 `18080` / `9443`），域名分流、证书、访问控制、日志仍由 Nginx 处理
- **端口转发（TCP/UDP）**：将 VPS 远程端口映射到内网服务（如 SSH、数据库、DNS/游戏等 UDP）；可与 Web 网关同时启用，纯端口穿透可不配置 Web 域名
- **服务端设置**：填写 frps 地址、端口、认证和 TLS 选项；保存后添加代理规则，校验配置再启动

**Web 流量路径**：用户 → DNS（解析到 VPS）→ frps → frpc → Havline Nginx → 内网服务

**端口转发流量路径**：用户 → VPS 公网 IP:远程端口 → frps → frpc → 内网 `IP:端口`（TCP 或 UDP）

#### 1. VPS 部署 frps

在具有公网 IP 的 VPS 上安装 [frp](https://github.com/fatedier/frp)，选择与本地 frpc 兼容的版本。在服务端详情中查看生成的 `frps.toml` 参考，也可通过公网 Agent 管理 frps。

典型 Web 网关示例：

```toml
bindAddr = "0.0.0.0"
bindPort = 7000

auth.method = "token"
auth.token = "your-secret-token"

vhostHTTPPort = 80
vhostHTTPSPort = 443
```

启动：`frps -c frps.toml`。安全组/防火墙需放行 **7000**（控制连接）、**80/443**（Web 虚拟主机），以及 **隧道远程端口**（在 Havline 添加规则后按提示放行；UDP 隧道需在防火墙同时放行对应端口的 UDP）。

#### 2. Havline 配置 frpc

在侧栏 **内网穿透** 页面：

1. 添加服务端，填写服务器地址、端口、认证信息和 TLS 选项，与 VPS 上 frps 保持一致。
2. 添加 HTTP / HTTPS 代理时填写域名和本机 Nginx 目标端口；Host 编排默认使用 `18080` / `9443`，Bridge 容器内通常为 `80` / `443`，以实际规则为准。
3. 添加 TCP / UDP 代理时填写 VPS 远程端口及内网目标地址；开放对应安全组和防火墙端口。
4. 在应用配置中下载或激活 FRP 客户端，校验配置后启动，通过服务端诊断和日志检查运行状态。

控制连接成功不等于业务已连通，还需检查代理端口、内网目标、DNS 和 Nginx 路由。

#### 3. DNS 与证书

- **DDNS**：FRP 访问域名应解析到 **VPS 公网 IP**，而非 NAS IP；按需使用自定义 IP 或在 DNS 服务商处配置
- **HTTPS 证书**：Web 网关推荐继续使用 **DNS-01** 验证（Cloudflare、阿里云、腾讯云、DNSPod、火山引擎等已支持）；证书仍在 NAS 侧 Nginx 终结
- **纯端口穿透**：无需为 TCP/UDP 隧道单独配置域名证书

### 公网 Agent 与 Cloudflare Tunnel

- **公网 Agent**：在公网 Agent 页面配置 VPS 信息，按需通过 SSH 安装并测试连接，再管理 frps、Nginx 和证书；需要公网反代时部署对应路由。限制 Agent 管理通道访问，不要仅凭业务端口可用判断管理通道安全。
- **Cloudflare Tunnel**：配置 Account ID 和 API Token，预检授权并按需下载 cloudflared；创建隧道后，在反向代理规则中选择 Cloudflare 出口和目标隧道，检查路由、DNS 与连接状态。托管模式从本地规则派生 ingress，修复漂移前先确认本地配置。

## 飞牛 fnOS 应用

Havline 提供两种飞牛应用形态，均支持统一网关入口 `/app/havline`。

### Docker 应用（离线镜像包）

- 独立仓库：<https://github.com/mrlee233/havline>
- 安装包：`havline-1.9.0.fpk`
- 包内包含 amd64 / arm64 离线镜像，安装时执行 `docker load`
- Compose 使用本地镜像并设置 `pull_policy: never`，不依赖外部 Registry
- 管理界面通过统一网关 `/app/havline` 访问


## 环境变量

| 变量                    | 默认值                    | 说明                                       |
| ----------------------- | ------------------------- | ------------------------------------------ |
| `HAVLINE_LISTEN`           | `:6893`                   | 管理后台监听地址                           |
| `HAVLINE_DATA_DIR`         | `/data`                   | 数据目录（数据库、Nginx 配置、证书、日志） |
| `HAVLINE_SESSION_SECRET` | 空，自动生成 | 会话与凭据加密密钥，保存在数据目录；已有数据后不要更换 |
| `HAVLINE_INITIAL_ADMIN_PASSWORD` | 空，自动生成 | 首次初始化管理员时使用的密码；不设置时会生成随机密码并输出到首次启动日志 |
| `HAVLINE_UPDATER_TOKEN` | 无 | 含 updater 的 Compose 部署必填随机共享鉴权值 |
| `HAVLINE_UPDATER_SOCKET` | `/run/havline-updater/updater.sock` | 主程序与 updater sidecar 通信的 Unix Socket |
| `HAVLINE_DATA_PATH` | `havline-data` | Compose 挂载来源，可配置宿主机目录 |
| `HAVLINE_NGINX_HTTP_PORT`  | `80`                      | 新建 HTTP 反代规则的默认监听端口           |
| `HAVLINE_NGINX_HTTPS_PORT` | `443`                     | 新建 HTTPS 反代规则的默认监听端口          |
| `HAVLINE_NGINX_BIN`        | `nginx`                   | Nginx 可执行文件路径（本地开发可指定）     |
| `HAVLINE_FRPC_BIN`         | `frpc`                    | frpc 可执行文件路径（运行时可按需下载）     |
| `HAVLINE_FRP_PID`          | `{DATA_DIR}/frp/frpc.pid` | frpc 进程 PID 文件路径                     |
| `HAVLINE_GATEWAY_SOCKET` | 空 | 飞牛统一网关接入时使用的 Unix Socket |
| `HAVLINE_GATEWAY_PREFIX` | 空 | 统一网关访问前缀，例如 `/app/havline` |
| `HAVLINE_AGENT_LISTEN` | `127.0.0.1:7700` | Agent 默认监听地址 |
| `HAVLINE_AGENT_ALLOW_REMOTE` | `0` | 是否允许 Agent 监听非回环地址；启用前应确认防火墙和 HTTPS/隧道策略 |
| `HAVLINE_AGENT_TOKEN` | 安装时生成 | Agent Bearer Token |
| `HAVLINE_UPDATE_PROJECT_DIR` | `/workspace` | updater 容器内 Compose 项目目录 |
| `HAVLINE_UPDATE_COMPOSE_FILE` | `docker-compose.yml` | updater 允许操作的 Compose 文件 |
| `HAVLINE_UPDATE_MODE` | `local` | 升级模式，支持 `local` 或 `git` |
| `HAVLINE_UPDATE_REPO` | 无 | `git` 模式下的远端仓库地址 |
| `HAVLINE_UPDATE_BRANCH` | `main` | `git` 模式下的分支 |
| `HAVLINE_UPDATE_HOST_DIR` | 自动发现 | 宿主机 Compose 文件目录；自动发现失败时必须显式设置 |

### 数据目录速查

| 路径 | 内容 |
| --- | --- |
| `/data/havline.db` | 管理员、会话、代理、DDNS、FRP、Cloudflare、健康事件等 SQLite 数据 |
| `/data/session_secret` | 会话签名与敏感凭据加密主密钥；已有数据后不可随意更换 |
| `/data/nginx/` | 生成的 Nginx 配置、日志和相关运行文件 |
| `/data/certs/` | ACME 证书与私钥 |
| `/data/frp/` | frpc 配置、二进制、PID 和运行状态 |

## 注意事项

1. **密钥持久化**：会话密钥未显式配置时自动生成并写入数据目录；已有数据不要更换密钥。含 updater 的编排必须另行配置 `HAVLINE_UPDATER_TOKEN`。
2. **端口冲突**：宿主机已有 Nginx（如飞牛系统）占用 80/443 时，不要让 Havline 反代规则监听 80/443；使用 `18080/9443` 或 host 模式 + 环境变量。
3. **已有反代规则**：修改 `HAVLINE_NGINX_HTTP_PORT` / `HAVLINE_NGINX_HTTPS_PORT` 后，**不会自动更新**数据库中已有规则的端口，需在反代页手动修改并保存。
4. **访问日志**：仅统计经 Havline Nginx 反代的域名流量，不包含管理后台 `:6893` 的请求。
5. **证书与 DNS**：DNS-01 申请不要求域名指向 NAS，但需确保 DNS 凭证能写入对应根域的验证记录，并等待传播。业务访问仍需正确解析、端口和路由。
6. **数据持久化**：Docker 部署请挂载 volume 到 `HAVLINE_DATA_DIR`，避免容器重建后配置丢失。
7. **备份**：可通过设置页导出数据；文件级备份前停止服务，保留整个数据目录（数据库、密钥、证书私钥和运行配置）；显式环境密钥需另行安全保留。
8. **升级**：含 updater 的 Compose 实例可使用一键升级；仅重建后版本健康检查失败时尝试镜像回滚，构建失败和数据库迁移回退需人工处理。升级前保留备份和旧镜像。

## 常见问题

| 现象 | 优先检查 |
| --- | --- |
| 无法登录或忘记初始密码 | 首次启动日志中的 `admin` 密码；若日志已丢失，停止服务后从备份恢复，或按部署文档处理数据库 |
| 启动后提示端口被占用 | 飞牛系统通常占用 `80/443`；Host 模式请使用 `6893/18080/9443`，Bridge 模式检查端口映射 |
| 修改反代端口后旧规则不生效 | `HAVLINE_NGINX_HTTP_PORT` / `HAVLINE_NGINX_HTTPS_PORT` 只影响新规则，已有规则需在反代页手动修改 |
| 证书申请失败 | 检查 DNS 服务商凭证、根域权限、ACME 邮箱、DNS 传播和 CA 限制；泛域名必须使用 DNS-01 |
| DDNS 没有更新 | 检查 DNS API 权限、记录是否存在、公网 IP 探测结果；FRP 场景域名应解析到 VPS 公网 IP |
| FRP 客户端启动失败 | 确认 frps 地址/端口/Token/TLS 一致，VPS 安全组已放行控制端口和远程端口 |
| Cloudflare DNS 未自动创建 | 确认根域名在同一个 Account ID 下，API Token 至少具备 Zone:Read 和 DNS:Edit |
| Cloudflare 隧道 502 或 Token 错误 | 使用 API Token 预检；Tunnel Token 与 Account API Token 不是同一种凭据 |
| 升级时找不到 Compose 文件 | 设置 `HAVLINE_UPDATE_HOST_DIR` 指向宿主机 Compose 所在目录，并确认容器名和 Compose 项目标签来自同一部署 |
| 升级后数据异常 | 恢复升级前备份，确认 `HAVLINE_DATA_DIR`、`session_secret` 和数据库迁移状态未变化 |
| Agent 直连失败 | 确认 Agent 监听地址、端口、防火墙和 Token；默认策略应优先使用 SSH 隧道或 HTTPS，不建议直接暴露管理端口 |
| 公网访问失败 | 按 DNS → 端口/隧道 → Nginx 路由 → 内网服务逐段排查，不要只看 FRP 或 Tunnel 是否在线 |

## 本地开发

```powershell
# 构建前端并准备嵌入目录
cd web
npm ci
npm run build
cd ..
New-Item -ItemType Directory -Force cmd/havline/web/dist | Out-Null
Copy-Item web/dist/* cmd/havline/web/dist -Recurse -Force

# 运行后端（需要可用 Nginx）
go run ./cmd/havline
```

前端热更新在另一终端执行 `cd web`、`npm run dev`。测试命令：

```powershell
go test ./...
go vet ./...
cd web
npm run typecheck
npm test
```

版本来源为根目录 `VERSION`；Docker 构建注入前后端，本地 `go run` 未注入时显示 `dev`。

## 开源协议

本项目采用 [GPL-3.0](https://www.gnu.org/licenses/gpl-3.0.html) 开源。

Havline 基于 [O96u/Fonu](https://github.com/O96u/Fonu) **1.0.2** 版本继续开发，感谢上游作者及贡献者。保留原作者版权、来源与 GPL-3.0 下适用的源代码提供义务。

问题反馈：[Havline Issues](https://github.com/mrlee233/havline/issues)。请提供版本、部署方式、复现步骤和脱敏日志，不上传密码、Token、私钥或完整备份。
