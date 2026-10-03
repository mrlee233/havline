<p align="center">
  <img src="web/src/assets/brand/havline-logo.svg" alt="Havline Logo" width="112" />
</p>

<h1 align="center">Havline</h1>

<p align="center"><strong>让每一台 NAS，都能安全地连接世界。</strong></p>

<p align="center">
  面向 <strong>飞牛 fnOS</strong> 等 NAS 环境的轻量公网访问管理控制台<br />
  反向代理 · DDNS · HTTPS 证书 · 内网穿透 · Cloudflare Tunnel · 一键升级
</p>

<p align="center">
  <img src="https://img.shields.io/badge/目标平台-飞牛_fnOS-22c55e?style=flat-square" alt="飞牛 fnOS" />
  <img src="https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.23" />
  <img src="https://img.shields.io/badge/Vue-3-4FC08D?style=flat-square&logo=vuedotjs&logoColor=white" alt="Vue 3" />
  <img src="https://img.shields.io/badge/Docker-amd64_%7C_arm64-2496ED?style=flat-square&logo=docker&logoColor=white" alt="Docker" />
  <a href="https://www.gnu.org/licenses/gpl-3.0.html">
    <img src="https://img.shields.io/badge/license-GPL--3.0-blue?style=flat-square" alt="GPL-3.0" />
  </a>
</p>

> **Havline 是基于开源项目 [O96u/Fonu](https://github.com/O96u/Fonu) 1.0.2 版本进行二次开发的项目。**
> 原项目已经完成了反向代理、DDNS、证书和内网穿透等核心能力。Havline 在此基础上进行品牌重塑、
> 功能迭代、界面调整、部署流程整理和文档重构，并继续遵循 GPL-3.0 开源协议。

---

## 项目简介

NAS 的价值不只在存储，更在于把文件、媒体、自动化工具和家庭服务安全地带到需要它们的地方。

**Havline** 把 NAS 用户最常用、也最容易出错的公网访问能力收进一个 Web 控制台：

- 管理 Nginx 反向代理，不必手工维护复杂配置；
- 自动同步 DDNS，保持域名始终指向当前公网 IP；
- 使用 ACME DNS-01 申请与续期 HTTPS 证书；
- 通过 FRP 在无公网 IP 时经 VPS 中转服务；
- 通过公网 Agent 远程管理 VPS 上的 frps 与 Nginx；
- 使用 Cloudflare Tunnel 发布服务，不必开放公网端口；
- 在一个仪表盘中查看状态、日志、流量和运行健康度。

当前开发与测试环境以 **飞牛 fnOS** 为主。群晖、威联通、自建 Linux 等其他环境尚未充分测试，
部署前请自行验证端口、权限和数据目录。

## 界面预览

| 仪表盘 | 反向代理 |
| --- | --- |
| ![仪表盘](docs/screenshots/dashboard.png) | ![反向代理](docs/screenshots/proxy.png) |

| DDNS | 证书管理 |
| --- | --- |
| ![DDNS](docs/screenshots/ddns.png) | ![证书管理](docs/screenshots/certificates.png) |

## 功能总览

| 模块 | 能力 |
| --- | --- |
| **反向代理** | 多域名、多端口、HTTP/HTTPS、自动跳转、Nginx 配置生成与热重载 |
| **DDNS** | Cloudflare、DNSPod、阿里云、腾讯云、火山引擎；单任务多域名与自动同步 |
| **HTTPS 证书** | ACME 自动申请与续期、通配符证书、手动导入、证书与私钥下载 |
| **内网穿透** | FRP 多服务端、8 种代理类型、插件、规则下发、流量与连接数监控 |
| **公网服务端** | SSH 安装与升级 `havline-agent`，远程管理 VPS 上的 frps 与 Nginx |
| **Cloudflare Tunnel** | cloudflared 安装、隧道、路由、DNS、健康检查、漂移修复 |
| **仪表盘** | 公网 IP、域名、证书、代理状态、请求趋势、流量和运行健康度 |
| **日志中心** | 系统日志、Nginx 访问日志、错误日志；分页、筛选与自动刷新 |
| **通知与备份** | 邮件、Webhook、Telegram、钉钉、飞书、企业微信；配置导出与恢复 |
| **一键升级** | Docker Compose sidecar 构建、重建、健康检查与失败回滚 |

## 快速开始

### 推荐：飞牛 fnOS Host 模式

飞牛系统通常已经占用 `80/443`，推荐使用 **host 网络 + 高位反代端口**：

```bash
docker run -d \
  --name havline \
  --net=host \
  -v ./data:/data \
  --restart unless-stopped \
  havline:local
```

| 入口 | 默认地址 |
| --- | --- |
| 管理后台 | `http://<NAS-IP>:6893` |
| HTTP 反向代理 | `http://<NAS-IP>:18080` |
| HTTPS 反向代理 | `https://<NAS-IP>:9443` |

### Docker Compose

```bash
# 飞牛 fnOS / host 网络
HAVLINE_SESSION_SECRET=your-secret \
  docker compose -f docker-compose.host.yml up -d

# Bridge 网络
HAVLINE_SESSION_SECRET=your-secret \
  docker compose -f docker-compose.yml up -d --build
```

### 首次登录

1. 查看容器或系统日志，获取自动生成的 `admin` 初始密码；
2. 打开 `http://<NAS-IP>:6893/login`；
3. 登录后立即在 **系统设置 → 管理员账户** 中修改密码。

> 完整部署、权限、端口、数据目录、迁移和故障排查请阅读
> [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) 与
> [`docs/container-deploy-and-data.md`](docs/container-deploy-and-data.md)。

## 内网穿透与公网服务端

没有公网 IPv4 或无法端口映射时，可以使用 FRP 经 VPS 中转：

```text
用户 → DNS(VPS) → frps → frpc → Havline Nginx → 内网服务
```

公网 VPS 上可以安装 `havline-agent`，由本地 Havline 通过 SSH 管理 agent、frps、Nginx 和证书。

| 主题 | 文档 |
| --- | --- |
| FRP 使用与场景 | [`docs/FRP公网暴露操作手册.md`](docs/FRP公网暴露操作手册.md) |
| 公网服务端管理 | [`docs/frps服务端管理方案.md`](docs/frps服务端管理方案.md) |
| Agent 监听与安全 | [`docs/agent-listen-strategy-plan.md`](docs/agent-listen-strategy-plan.md) |

## Cloudflare Tunnel

Havline 支持将 Cloudflare Tunnel 作为服务发布出口。反向代理规则可选择本机 Nginx 或
Cloudflare 出口，并自动派生隧道 ingress、同步 DNS、检查健康状态和修复云端漂移。

| 主题 | 文档 |
| --- | --- |
| 当前功能 | [`docs/cloudflared-current-features.md`](docs/cloudflared-current-features.md) |
| 实施方案 | [`docs/cloudflared-havline-fit-plan.md`](docs/cloudflared-havline-fit-plan.md) |
| UX 调研 | [`docs/cloudflared-github-ux-research.md`](docs/cloudflared-github-ux-research.md) |

## 一键升级

`docker-compose.yml` 与 `docker-compose.hub.yml` 会启动受限的 `havline-updater` sidecar。
检测到新版本后，侧栏会出现「一键升级」入口，可在页面中完成：

1. 拉取或构建目标版本镜像；
2. 重建 `havline` 服务；
3. 执行健康检查；
4. 失败时尝试回滚到升级前镜像。

详细设计、安全边界、故障排查和验证步骤见 [`docs/system-upgrade.md`](docs/system-upgrade.md)。

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HAVLINE_LISTEN` | `:6893` | 管理后台监听地址 |
| `HAVLINE_DATA_DIR` | `/data` | 数据目录，包含数据库、Nginx、证书与日志 |
| `HAVLINE_SESSION_SECRET` | 空，自动生成 | 会话签名与凭据加密密钥；有数据后不要更换 |
| `HAVLINE_NGINX_BIN` | `nginx` | Nginx 可执行文件路径 |
| `HAVLINE_NGINX_HTTP_PORT` | `80` | 新建 HTTP 反代规则默认监听端口 |
| `HAVLINE_NGINX_HTTPS_PORT` | `443` | 新建 HTTPS 反代规则默认监听端口 |
| `HAVLINE_FRPC_BIN` | `frpc` | frpc 可执行文件路径 |
| `HAVLINE_FRP_PID` | `{DATA_DIR}/frp/frpc.pid` | frpc PID 文件 |
| `HAVLINE_UPDATER_SOCKET` | `/run/havline-updater/updater.sock` | 一键升级 sidecar Socket |
| `HAVLINE_AGENT_LISTEN` | `127.0.0.1:7700` | Agent 默认监听地址 |
| `HAVLINE_AGENT_ALLOW_REMOTE` | `0` | 是否允许 Agent 监听非回环地址 |
| `HAVLINE_AGENT_TOKEN` | 安装时生成 | Agent Bearer Token |

## 技术架构

| 层级 | 技术 |
| --- | --- |
| 后端 | Go 1.23、标准库 HTTP、SQLite |
| 数据存储 | `modernc.org/sqlite`，单文件数据库 |
| 反向代理 | Nginx，配置动态生成与热重载 |
| 证书 | go-acme/lego，ACME DNS-01 |
| 内网穿透 | FRP 客户端 / 服务端与自研管理 Agent |
| 隧道 | Cloudflare Tunnel / cloudflared |
| 前端 | Vue 3、TypeScript、Vite、Naive UI、ECharts |
| 部署 | Docker、Docker Compose、Linux systemd |
| 架构 | amd64 / arm64 |

## 本地开发

```bash
# 前端依赖与热更新
cd web
npm ci
npm run dev

# 构建前端并同步到 Go embed 目录
npm run build
cd ..
Copy-Item -Path web/dist/* -Destination cmd/havline/web/dist -Recurse -Force

# 运行后端
go run ./cmd/havline

# 测试
go test ./...
go vet ./...
cd web && npm test
```

`VERSION` 是版本号单一来源。Docker 构建会读取并注入前后端；直接 `go run` 时未注入版本会显示 `dev`。

## 文档导航

| 文档 | 说明 |
| --- | --- |
| [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) | 裸机、Docker、飞牛 host 模式部署 |
| [`docs/container-deploy-and-data.md`](docs/container-deploy-and-data.md) | 容器部署与数据目录 |
| [`docs/system-upgrade.md`](docs/system-upgrade.md) | 一键升级与回滚 |
| [`docs/FRP公网暴露操作手册.md`](docs/FRP公网暴露操作手册.md) | FRP 使用手册 |
| [`docs/frps服务端管理方案.md`](docs/frps服务端管理方案.md) | 公网服务端与 Agent |
| [`docs/cloudflared-current-features.md`](docs/cloudflared-current-features.md) | Cloudflare Tunnel 当前能力 |
| [`docs/fnos-app-packaging-plan.md`](docs/fnos-app-packaging-plan.md) | 飞牛应用打包方案 |
| [`docs/README.md`](docs/README.md) | 全部文档索引 |

## 开源许可与致谢

> 开源不是重新发明轮子，而是站在巨人的肩膀上，把一件事继续做得更好。

Havline 基于开源项目 [**O96u/Fonu**](https://github.com/O96u/Fonu) **1.0.2** 版本进行二次开发。
感谢原项目及其贡献者提供的基础实现和工程积累。

本项目继续采用 [GPL-3.0](https://www.gnu.org/licenses/gpl-3.0.html) 开源。品牌名称调整不会改变
原项目及其衍生版本在 GPL-3.0 下的版权、署名和源代码提供义务。

软件按“原样”提供，不附带任何担保。请在部署前做好数据备份、访问控制和网络安全配置。
