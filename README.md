<p align="center">
  <img src="web/src/assets/brand/logo.png" alt="Fonu Logo" width="96" />
</p>

<h1 align="center">Fonu</h1>

<p align="center"><strong>让 NAS 访问更简单</strong></p>

<p align="center">
  面向 <strong>飞牛 fnOS</strong> 的轻量公网访问管理工具<br />
  反向代理 · DDNS · HTTPS 证书 · 内网穿透 · 一站式 Web 管理
</p>

<p align="center">
  <img src="https://img.shields.io/badge/目标平台-飞牛_fnOS-22c55e?style=flat-square" alt="飞牛 fnOS" />
  <img src="https://img.shields.io/badge/其他环境-未测试-94a3b8?style=flat-square" alt="其他环境未测试" />
  <img src="https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Vue-3-4FC08D?style=flat-square&logo=vuedotjs&logoColor=white" alt="Vue 3" />
  <img src="https://img.shields.io/badge/Nginx-内置-009639?style=flat-square&logo=nginx&logoColor=white" alt="Nginx" />
  <a href="https://www.gnu.org/licenses/gpl-3.0.html"><img src="https://img.shields.io/badge/license-GPL--3.0-blue?style=flat-square" alt="GPL-3.0" /></a>
</p>

---

## 简介

**Fonu** 是一款主要为 **飞牛 fnOS** 编写的反代与管理工具，帮助你在 NAS 上快速搭建稳定的公网访问能力：通过 Web 界面管理 Nginx 反向代理、自动同步 DDNS、申请与续期 Let's Encrypt 证书，并在一个仪表盘里查看服务状态、访问日志与流量统计。

无需手写 Nginx 配置，也无需在多个工具之间切换。当前开发与测试环境以飞牛 fnOS 为主，**群晖、威联通、自建 Linux 等其他环境尚未充分测试**，部署前请自行验证。

## 功能特性

| 模块           | 说明                                                                         |
| -------------- | ---------------------------------------------------------------------------- |
| **反向代理**   | 多域名、多端口、HTTP/HTTPS、自动重定向；实时访问日志、流量统计、当前连接 IP  |
| **DDNS**       | 支持 Cloudflare、DNSPod、阿里云、腾讯云、火山引擎 DNS；单任务多根域名，自动同步公网 IP |
| **HTTPS 证书** | ACME 自动申请与续期；证书/私钥/ZIP 下载；申请进度实时日志                    |
| **仪表盘**     | 公网 IP、域名、证书、服务状态一览；请求趋势与运行健康度                      |
| **日志中心**   | 系统日志、Nginx 访问日志、Nginx 错误日志；分页筛选与自动刷新                   |
| **内网穿透**   | FRP 客户端（Nginx 网关穿透）：无公网 IP 时经 VPS 中转，反代与安全策略不变    |
| **系统设置**   | 常规 / 通知 / 安全 / 高级分栏；邮件/Webhook/Telegram 告警、备份与恢复        |
| **其他**       | 深色模式、配置导出/导入、GitHub Release 更新提示                             |


## 技术栈

| 层级         | 技术                                                  |
| ------------ | ----------------------------------------------------- |
| **后端**     | Go 1.23、标准库 HTTP、SQLite（modernc.org/sqlite）    |
| **反向代理** | 内置 Nginx（动态生成配置、热重载）                    |
| **证书**     | go-acme/lego（Let's Encrypt）                         |
| **DDNS**     | Cloudflare / DNSPod / 阿里云 / 腾讯云 / 火山引擎 DNS API |
| **内网穿透** | FRP 客户端（多服务端）：无公网 IP 时经 VPS 中转，8 种穿透类型与插件，frps.toml 同源生成 |
| **前端**     | Vue 3、TypeScript、Vite、Naive UI、ECharts            |
| **部署**     | Docker 镜像（amd64 / arm64），本地源码构建 |

## 部署

本地项目：三个 compose 文件均从源码构建镜像（`build: .`），不拉取任何仓库镜像，构建产物标签为 `fonu:local`。

> **详细部署安装文档**：[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) —— 环境要求、镜像构建、三种部署方式、首次配置、FRP/DDNS/证书联动、升级回滚与故障排查。
> **FRPS 服务端管理方案**：[`docs/frps服务端管理方案.md`](frps服务端管理方案.md) —— 公网 VPS 部署 `fonu-agent`，SSH 从本地 Fonu 安装/升级/卸载，日常通过本地管理面板调用 API 自动编排 Nginx、frps 与证书。

### 快速部署（推荐 · 飞牛 fnOS · Host 模式）

飞牛系统已占用 **80/443**，使用 **host 网络** + **高位反代端口**，避免与系统 Nginx 冲突。

```bash
docker run -d \
  --name fonu \
  --net=host \
  -v ./data:/data \
  --restart unless-stopped \
  fonu:local
```

| 入口               | 地址                    |
| ------------------ | ----------------------- |
| 管理后台           | `http://<NAS-IP>:6893`  |
| HTTP 反代（默认）  | `http://<NAS-IP>:18080` |
| HTTPS 反代（默认） | `https://<NAS-IP>:9443` |

- `./data` 挂载后自动作为数据目录，存放数据库、Nginx 配置、证书与日志
- 飞牛系统已占用 80/443，容器默认反代端口为 `18080`、`9443`，可通过环境变量修改

### 其他部署方式

<details>
<summary>Docker Compose</summary>

```bash
FONU_SESSION_SECRET=your-secret docker compose -f docker-compose.host.yml up -d
```

</details>

<details>
<summary>Bridge 网络（端口映射，非飞牛环境可自行尝试）</summary>

```bash
FONU_SESSION_SECRET=your-secret docker compose -f docker-compose.hub.yml up -d --build
```

</details>

<details>
<summary>本地构建</summary>

```bash
docker compose up -d --build
```

</details>

### Web 一键升级

`docker-compose.yml` 与 `docker-compose.hub.yml` 会同时启动受限的 `fonu-updater` sidecar。检测到新版本后，侧栏会出现「一键升级」入口，可在管理页面完成构建、重建与健康检查。

> 从旧版本升级时，需要先手动更新 Compose 文件并执行一次 `docker compose up -d --build` 启动 sidecar。详见 [`docs/system-upgrade.md`](docs/system-upgrade.md)。

### 首次登录

1. 启动后查看容器/运行日志，获取自动生成的 **admin 初始密码**
2. 访问 `http://<NAS-IP>:6893/login` 登录
3. 在 **设置** 中立即修改管理员密码

### FRP 内网穿透（无公网 IP）

适用于无法端口映射、无公网 IPv4 的场景。Fonu 内置 **frpc**，支持**多台公网服务端**与全部 8 种穿透类型（tcp / udp / http / https / tcpmux / stcp / sudp / xtcp）及插件（static_file / socks5 / https2http 等）。

**典型流量路径**（tcp 规则 + 本地反代）：用户 → DNS（解析到 VPS）→ frps → frpc → Fonu Nginx → 内网服务

#### 1. VPS 部署 frps

在服务端「详情」弹窗点 **「frps.toml」** 一键生成与 frpc 侧同源的服务端配置（bindPort / 认证 / webServer / 传输参数 / vhost 端口与防火墙清单全部对齐），复制到 VPS 保存后 `frps -c frps.toml` 启动，按文件末尾清单放行防火墙端口。

#### 2. Fonu 配置 frpc

在侧栏 **FRP 穿透** 页面：

- **添加服务端**：主机地址 / 端口（frps bindPort，默认 7000）/ 认证方式与 Token，与 frps 一致；可选填 frps 管理接口（webServer）用于拉取穿透流量；
- **添加穿透规则**：按需选择类型——tcp / udp 填远程端口，http / https / tcpmux 填自定义域名，stcp / sudp / xtcp 凭 secretKey 访问；
- 保存后「应用配置」弹窗内校验并应用；服务端「详情」含五项诊断与独立日志。

#### 3. DNS 与证书

- **DDNS**：IP 来源**留空**=检测本机（域名指家庭宽带）；填**自定义 IP**=VPS 的 IP（域名直接指 VPS，Web 服务经 VPS 对外时使用）；VPS 动态 IP 场景把服务端地址填 DDNS 域名即可自动跟随；
- **HTTPS 证书**：ACME DNS-01 验证（复用 DDNS 服务商凭证，含火山引擎）；「统一入口」方案证书在本地 Nginx 终结，「直通」方案在 VPS 侧终止。

> 组合场景详解已内置页面：FRP 页右上「使用说明」→「组合场景详解」标签，覆盖十个典型部署场景与故障排查。

## 环境变量

| 变量                    | 默认值                    | 说明                                       |
| ----------------------- | ------------------------- | ------------------------------------------ |
| `FONU_LISTEN`           | `:6893`                   | 管理后台监听地址                           |
| `FONU_DATA_DIR`         | `/data`                   | 数据目录（数据库、Nginx 配置、证书、日志） |
| `FONU_SESSION_SECRET`   | 空（自动生成）            | 会话与凭据加密密钥；有数据后不要更换       |
| `FONU_NGINX_HTTP_PORT`  | `80`                      | 新建 HTTP 反代规则的默认监听端口           |
| `FONU_NGINX_HTTPS_PORT` | `443`                     | 新建 HTTPS 反代规则的默认监听端口          |
| `FONU_NGINX_BIN`        | `nginx`                   | Nginx 可执行文件路径（本地开发可指定）     |
| `FONU_FRPC_BIN`         | `frpc`                    | frpc 可执行文件路径（镜像不预装，用「应用配置」按需下载或自行放置） |
| `FONU_FRP_PID`          | `{DATA_DIR}/frp/frpc.pid` | frpc 进程 PID 文件路径                     |
| `FONU_UPDATER_SOCKET`   | `/run/fonu-updater/updater.sock` | 一键升级 sidecar 的 Unix Socket；未挂载时自动禁用 |

## 注意事项

1. **生产环境务必设置** `FONU_SESSION_SECRET`，不要使用默认值。
2. **端口冲突**：宿主机已有 Nginx（如飞牛系统）占用 80/443 时，不要让 Fonu 反代规则监听 80/443；使用 `18080/9443` 或 host 模式 + 环境变量。
3. **已有反代规则**：修改 `FONU_NGINX_HTTP_PORT` / `FONU_NGINX_HTTPS_PORT` 后，**不会自动更新**数据库中已有规则的端口，需在反代页手动修改并保存。
4. **访问日志**：仅统计经 Fonu Nginx 反代的域名流量，不包含管理后台 `:6893` 的请求。
5. **证书与 DNS**：HTTPS 申请前请确保域名 DNS 已解析到本机，且 DDNS 凭证能管理对应根域。
6. **数据持久化**：Docker 部署请挂载 volume 到 `FONU_DATA_DIR`，避免容器重建后配置丢失。
7. **备份**：可通过设置页导出配置；重要证书与数据库建议定期备份 `FONU_DATA_DIR` 目录。

## 本地开发

```bash
# 前端热更新
cd web && npm ci && npm run dev

# 后端（需先把 web/dist 复制到 cmd/fonu/web/dist）
cd web && npm run build
go run ./cmd/fonu

# 测试
go test ./...
cd web && npm run typecheck
```

发版版本号以根目录 `VERSION` 文件为单一来源，`docker compose build` 自动读取并注入前后端；本地 `go run` / 未注入时默认显示 `dev`。

## 开源协议

本项目采用 [GPL-3.0](https://www.gnu.org/licenses/gpl-3.0.html) 开源。
