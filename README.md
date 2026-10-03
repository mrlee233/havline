<p align="center">
  <img src="web/src/assets/brand/havline-logo.svg" alt="Havline" width="96" />
</p>

<h1 align="center">Havline</h1>

<p align="center"><strong>连接你的服务，掌握你的入口。</strong></p>

<p align="center">
  面向家庭 NAS 与自托管环境的公网访问控制台<br />
  域名解析 · HTTPS 证书 · 反向代理 · FRP · Cloudflare Tunnel
</p>

<p align="center">
  <a href="https://github.com/mrlee233/havline/releases"><img src="https://img.shields.io/github/v/release/mrlee233/havline?style=flat-square" alt="发布版本" /></a>
  <img src="https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.23" />
  <img src="https://img.shields.io/badge/Vue-3-4FC08D?style=flat-square&logo=vuedotjs&logoColor=white" alt="Vue 3" />
  <img src="https://img.shields.io/badge/架构-amd64%20%7C%20arm64-2496ED?style=flat-square" alt="amd64 / arm64" />
  <a href="LICENSE"><img src="https://img.shields.io/badge/许可-GPL--3.0-16825d?style=flat-square" alt="GPL-3.0" /></a>
</p>

<p align="center">
  <a href="#快速部署">快速部署</a> ·
  <a href="#功能地图">功能地图</a> ·
  <a href="https://github.com/mrlee233/havline/releases">发布记录</a> ·
  <a href="https://github.com/mrlee233/havline/issues">问题反馈</a>
</p>

---

## 不只是把服务放到公网

Havline 将域名、证书、代理规则和隧道集中到一个 Web 控制台，让服务发布与日常运维有据可查。你可以自动生成 Nginx 配置、跟随公网 IP 更新 DNS、续期证书，也可以借助自己的 VPS 或 Cloudflare Tunnel 建立访问链路。

**发布、观察、恢复，是同一件事的三个环节。**

- **发布**：为内网服务配置域名、证书与访问出口。
- **观察**：通过日志、流量、健康检查和通知了解运行状态。
- **恢复**：保留配置版本与数据备份，在出错时有可追溯的恢复路径。

Havline 是管理工具，不是公网资源提供商。域名、VPS、DNS 授权和第三方隧道账户需要自行准备；最终连通性仍取决于网络、权限、防火墙与服务商规则。

## 选择你的访问链路

| 网络条件 | 建议入口 | 需要准备 |
| --- | --- | --- |
| 有可用公网 IPv4 或 IPv6 | DDNS + 本机 Nginx + HTTPS | 域名、DNS 凭据、可达端口 |
| 无法直接入站，但有公网服务器 | FRP + VPS，按需配置公网 Agent | VPS、frps、域名及服务器权限 |
| 不希望开放本机公网入站端口 | Cloudflare Tunnel | Cloudflare 账户、授权和可用域名 |

```text
直连访问       访客 → NAS Nginx → 内网服务
FRP 中转       访客 → VPS 入口 → frps / frpc → 内网服务
Cloudflare     访客 → Cloudflare Edge → cloudflared → 内网服务
```

不同链路可以按服务组合使用，不必把所有服务绑定在同一种出口上。

## 功能地图

### 发布服务

| 能力 | 当前支持 |
| --- | --- |
| 反向代理 | 多域名、多端口、HTTP / HTTPS、重定向、配置生成与重载 |
| DDNS | Cloudflare、DNSPod、阿里云、腾讯云、火山引擎；多域名任务与公网 IP 同步 |
| HTTPS 证书 | ACME DNS-01 申请与续期、通配符证书、手动导入和下载 |
| FRP | 多服务端、TCP / UDP / HTTP / HTTPS 等代理类型、运行控制与诊断 |
| 公网 Agent | 通过 SSH 安装与维护，管理 VPS 上的 frps、Nginx、证书和路由 |
| Cloudflare Tunnel | 隧道与路由管理、DNS 同步、运行监控、配置漂移检测与修复 |

### 维护实例

| 能力 | 当前支持 |
| --- | --- |
| 仪表盘与日志 | 服务状态、请求与流量趋势；系统、访问及错误日志 |
| 健康与通知 | 健康巡检；邮件、Webhook、Telegram、钉钉、飞书及企业微信通知 |
| 配置与备份 | 配置版本、回滚、数据导出与恢复、自动备份 |
| 访问控制 | 管理员认证、API Token、敏感凭据加密与代理安全选项 |
| 更新管理 | 指定 Compose 部署下的 updater sidecar，构建、重建和版本检查 |

> FRP 客户端和 cloudflared 按需下载，首次启用相关功能需要联网。管理后台的统计范围与所选出口有关，本机 Nginx 数据不等同于所有公网入口的总流量。

## 快速部署

源码仓库：[mrlee233/havline](https://github.com/mrlee233/havline)。以下命令使用当前仓库提供的 Compose 文件，本地构建镜像；不假设存在可直接拉取的 Docker Hub 镜像。

### 路径一：本地源码 + Host 网络

适合 NAS 宿主机已占用 80 / 443 的环境。

```bash
git clone --branch main https://github.com/mrlee233/havline.git
cd havline
docker compose -f docker-compose.host.yml up -d --build
```

| 入口 | 默认地址 |
| --- | --- |
| 管理后台 | `http://<NAS-IP>:6893` |
| HTTP 反向代理 | `http://<NAS-IP>:18080` |
| HTTPS 反向代理 | `https://<NAS-IP>:9443` |

此编排使用宿主机网络，不包含 updater sidecar。更新源码并重新构建即可升级；生产部署前应确认目标提交和备份。

### 路径二：单文件 Git 构建 + Bridge 网络

适合只在部署目录保存编排文件的环境。BuildKit 从 GitHub 的 `main` 分支获取源码，主程序和 updater 都从相同仓库构建。

```bash
mkdir -p havline && cd havline
curl -fsSLo docker-compose.hub.yml \
  https://raw.githubusercontent.com/mrlee233/havline/main/docker-compose.hub.yml

# 首次部署：仅在尚未配置 Token 时追加随机值，不覆盖已有 .env
umask 077
if ! test -f .env || ! grep -q "^HAVLINE_UPDATER_TOKEN=" .env; then
  printf "\nHAVLINE_UPDATER_TOKEN=%s\n" "$(openssl rand -hex 32)" >> .env
fi

docker compose -f docker-compose.hub.yml up -d --build
```

默认映射为 `6893:6893`、`18080:80`、`9443:443`，访问地址与上表相同。容器内访问 NAS 宿主机服务时，请使用容器可达的宿主机地址，而不是把 `127.0.0.1` 当成 NAS。

**已有源码、希望使用 Bridge 网络？** 在源码根目录准备同样的 updater Token，改用：

```bash
docker compose -f docker-compose.yml up -d --build
```

> `HAVLINE_UPDATER_TOKEN` 与会话密钥不同：前者是主程序和升级 sidecar 的共享鉴权值，启用 sidecar 时必填；`HAVLINE_SESSION_SECRET` 可留空，由应用在数据目录自动生成。已有实例请沿用原会话密钥，不要为了升级生成新值。

### 首次登录

1. 在容器日志中查找首次启动生成的管理员口令：`docker logs havline`。
2. 打开 `http://<NAS-IP>:6893/login`，使用 `admin` 登录。
3. 登录后修改管理员密码，再配置 DNS 凭据、证书与服务入口。

初始口令只在首次初始化时生成。日志可能含敏感信息，反馈问题前务必脱敏。其他设备与系统环境的兼容性需要自行验证，尤其是端口占用、文件权限及架构。

## 数据归你管理，也需要完整保护

数据不在镜像里。三份 Compose 默认将命名卷挂载到 `/data`，数据库为 `/data/havline.db`；固定项目名 `havline` 时，默认完整卷名为 `havline_havline-data`，实际挂载以 `docker inspect havline` 为准。

| 数据 | 为什么要保留 |
| --- | --- |
| `havline.db` | 管理员、服务规则、设置与历史状态 |
| `session_secret` 或显式环境密钥 | 会话与敏感凭据加密；缺失或更换会影响解密 |
| `certs/` | HTTPS 证书与私钥 |
| `nginx/`、`frp/`、`cloudflared/` | 运行配置、隧道凭据及组件数据 |
| `logs/`、备份归档 | 故障追溯与恢复材料，也可能包含敏感信息 |

- **备份完整数据目录**，不要只复制数据库；使用显式密钥时另行安全保留原值。
- **停服务后做文件级备份**，或使用应用备份流程，避免数据库与配置处于不一致状态。
- **迁移前确认挂载与项目名**。只修改 `HAVLINE_DATA_PATH` 而不迁移数据，会启动一个空实例。
- **管理后台不要直接暴露公网**。按需保护代理入口、Agent 管理通道和 Docker Socket。
- **备份也是敏感资产**，请限制访问并使用可信的加密存储。

如需绑定宿主机目录，在 Compose 同层 `.env` 配置 `HAVLINE_DATA_PATH=/你的数据目录`；已有数据必须先完成迁移。

## 升级的边界

`docker-compose.yml` 与 `docker-compose.hub.yml` 提供 updater sidecar。主程序通过带 Token 鉴权的 Unix Socket 请求升级，只有 sidecar 挂载 Docker Socket。**Docker Socket 仍是高权限能力**，不能把 sidecar 的存在理解为完全隔离。

升级会构建目标镜像、重建主服务并检查 `/api/version`。仅在重建成功但目标版本健康检查失败时尝试回滚镜像；构建失败、重建失败、sidecar 异常和数据库结构回退不由该流程完整覆盖。

升级前保留整目录备份和旧镜像，升级后检查版本、日志和业务连通性。`HAVLINE_VERSION` 是镜像标签，不是源码版本锁；hub 编排中的 `#main` 仍指向可变分支。

## 配置速查

| 变量 | 默认值或行为 |
| --- | --- |
| `HAVLINE_DATA_PATH` | Compose 数据挂载来源，默认 `havline-data` |
| `HAVLINE_DATA_DIR` | Compose 中固定为 `/data` |
| `HAVLINE_LISTEN` | 管理 HTTP 监听地址，默认 `:6893` |
| `HAVLINE_SESSION_SECRET` | 留空时读取或生成数据目录密钥；已有数据不要更换 |
| `HAVLINE_UPDATER_TOKEN` | 启用升级 sidecar 时必填的随机共享 Token |
| `HAVLINE_UPDATER_SOCKET` | 默认 `/run/havline-updater/updater.sock` |
| `HAVLINE_UPDATE_HOST_DIR` | sidecar 挂载的宿主机项目目录，默认当前目录 |
| `HAVLINE_NGINX_HTTP_PORT` / `HAVLINE_NGINX_HTTPS_PORT` | Host 编排默认 `18080` / `9443`；应用默认 `80` / `443` |
| `NODE_IMAGE` / `GO_IMAGE` / `RUNTIME_IMAGE` / `UPDATER_IMAGE` | 构建镜像源，按所用编排覆盖 |
| `GOPROXY` | Go 模块下载代理，默认 `https://goproxy.cn,direct` |
| `TZ` | Compose 默认 `Asia/Shanghai` |

修改应用监听地址或数据目录时，需同时核对端口映射、健康检查地址和实际挂载。

## 开发与贡献

后端使用 Go、标准库 HTTP 与 SQLite，前端使用 Vue 3、TypeScript、Naive UI 和 ECharts；Nginx、FRP 与 cloudflared 承担不同的数据访问链路。

```text
cmd/          主程序、Agent、updater 入口
internal/     API、业务服务、进程与配置管理
web/          管理界面
migrations/   数据库向前迁移
deploy/       部署资源
scripts/      安装、构建与升级脚本
```

### 本地运行（PowerShell）

需要 Go、Node.js / npm，以及可运行的 Nginx。先构建前端并准备嵌入资源：

```powershell
cd web
npm ci
npm run build
cd ..
New-Item -ItemType Directory -Force cmd/havline/web/dist | Out-Null
Copy-Item -Path web/dist/* -Destination cmd/havline/web/dist -Recurse -Force
go run ./cmd/havline
```

前端热更新可在另一个终端执行 `cd web` 后运行 `npm run dev`。未通过构建参数注入版本的后端会显示 `dev`；Docker 构建从根目录 `VERSION` 注入前后端版本。

### 验证改动

```powershell
go test ./...
go vet ./...
cd web
npm run typecheck
npm test
npm run build
```

在 [Issues](https://github.com/mrlee233/havline/issues) 提交问题时，请附上版本、部署方式、设备架构、复现步骤与脱敏日志。欢迎提交兼容性反馈、问题修复和改进建议；请勿上传 Token、密码、私钥、完整数据库或备份归档。

## 来源与许可

Havline 基于 [O96u/Fonu](https://github.com/O96u/Fonu) **1.0.2** 版本继续开发。感谢上游作者与贡献者提供反向代理、DDNS、证书和内网穿透的基础实现。Havline 在此基础上完善多服务端管理、公网 Agent、Cloudflare Tunnel 与部署兼容性，并以自己的仓库维护源码与发布记录。

本项目继续采用 [GPL-3.0](LICENSE)；项目更名与二次开发不改变上游版权、署名及相应源代码提供义务。第三方服务与依赖仍适用各自的许可证和条款。

软件按“原样”提供，不附带任何担保。请在部署前做好备份、权限和网络访问控制。
