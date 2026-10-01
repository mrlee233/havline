# Fonu → Havline 品牌改名实施与影响评估方案

> 日期：2026-10-01
> 当前版本：`1.9.0`
> 当前 Agent 版本：`0.14.1`
> 扫描范围：仓库源码、部署文件、脚本、前端、文档、GitHub Actions、迁移与生成配置
> 扫描结果：**227 个包含 `Fonu / fonu / FONU` 的文件，共 4032 处匹配**
> 扫描排除：`.git`、`web/node_modules`、`web/dist`、`cmd/fonu/web/dist`、`output`、`.playwright-cli`、Havline Logo 预览页
> 说明：本方案文档本身不计入上述扫描统计。
> 执行记录：2026-10-01 已按“无兼容迁移”模式执行全量切换。本文中的兼容迁移设计作为历史设计记录保留，不再代表当前实施状态。

---

## 0. 结论速览

品牌改名可以做，但**不能做一次无脑全局替换**。

真正危险的不是页面文字，而是下面这些会直接影响老用户升级的契约：

| 风险项 | 当前值 | 改名后的风险 |
| --- | --- | --- |
| 数据文件名 | `fonu.db` | 裸改名后应用会新建空库，用户会认为数据丢失 |
| 环境变量前缀 | `FONU_*` | 裸改名后老 `.env`、compose、systemd 配置全部失效 |
| Docker Volume | `fonu-data` | 裸改名后会创建新空卷 |
| Compose 项目名 | `name: fonu` | 项目名变化会让容器、网络、卷全部重新创建 |
| Agent 路径 | `/etc/fonu-agent`、`/var/lib/fonu-agent` | 服务端与 VPS Agent 版本错配时探测和升级失败 |
| Agent systemd | `fonu-agent.service` | 新老 Agent 可能同时存在或互相抢端口 |
| 会话 Cookie | `fonu_session` | 裸改名会让所有已登录用户掉线 |
| 一键升级 | `fonu`、`fonu-updater` 服务名与镜像 | 改名后 updater 找不到目标服务或镜像 |
| 前端版本检查 | `O96u/Fonu` | 仓库改名或常量未同步后，在线版本检查失效 |

推荐采用：

**分层渐进改名 + 一个稳定版本周期的兼容层。**

推荐目标命名：

| 形态 | 目标值 |
| --- | --- |
| 显示名 | `Havline` |
| 小写标识 | `havline` |
| 环境变量前缀 | `HAVLINE_` |
| Go module | `github.com/havline/havline` |
| 主程序 | `havline` |
| Agent | `havline-agent` |
| Updater | `havline-updater` |
| Docker 镜像 | `havline:<version>` |
| Updater 镜像 | `havline-updater:<version>` |
| Compose 项目 | `name: havline` |
| Compose 服务 | `havline`、`updater` |
| 数据卷 | `havline-data` |
| 数据库 | `havline.db` |

---

## 1. 当前工作区前提

当前工作区不是干净状态，执行正式改名之前必须先处理基线问题。

已存在的未提交或未跟踪内容：

| 类型 | 路径 | 与本次改名关系 |
| --- | --- | --- |
| 已修改 | `web/src/layouts/AppLayout.vue` | 侧栏已切到 `havline-logo.svg`，属于品牌展示层改动 |
| 已修改 | `web/src/assets/brand/logo.png` | 旧品牌资源，改名时必须统一处理 |
| 已修改 | `web/public/favicon.png` | 旧 favicon，改名时必须统一处理 |
| 已新增 | `web/src/assets/brand/havline-logo.svg` | 当前侧栏使用的第一版 SVG |
| 已新增 | `web/src/assets/brand/havline-logo-v1.svg` | 备选或历史 SVG |
| 已新增 | `web/src/assets/brand/havline-logo-v2.svg` | 当前推荐的第二版 SVG，尚未接入侧栏 |
| 已新增 | `web/assets/image/havline-logo.svg` | 镜像内资源目录的 SVG |
| 已新增 | `docs/havline-logo-preview.html` | 第一版 Logo 预览 |
| 已新增 | `docs/havline-logo-v2-preview.html` | 第二版 Logo 预览 |
| 未跟踪 | `docs/fnos-app-packaging-plan.md` | 飞牛应用打包方案，与品牌名有关 |
| 已删除 | `internal/frp-1.0.2/**` | 非本次改名逻辑的一部分，执行前需确认是否保留该删除 |
| 已删除 | `web/src/views/FrpView-1.0.2.vue` | 非本次改名逻辑的一部分，执行前需确认是否保留该删除 |

建议：

1. 先把 Logo V2 和侧栏资源作为独立提交合入。
2. 再确认 `internal/frp-1.0.2` 与 `FrpView-1.0.2.vue` 的删除是否属于当前基线。
3. 然后从干净、可回滚的基线开始兼容层和全仓改名。

---

## 2. 全仓扫描结果

### 2.1 数量统计

| 分类 | 文件数 |
| --- | ---: |
| Go、module、迁移 | 144 |
| 前端 Web | 40 |
| 部署、Docker、脚本 | 9 |
| 文档、README、CHANGELOG | 31 |
| GitHub Actions、`.gitignore` | 3 |
| 合计 | 227 |

匹配总量：**4032 处**。

### 2.2 重复率最高的文件

| 文件 | 匹配数 | 说明 |
| --- | ---: | --- |
| `web/src/views/AgentView.vue` | 214 | Agent 管理页，包含大量文案、变量和 `FonuCard` 引用 |
| `web/src/views/FrpView.vue` | 159 | 内网穿透页，包含品牌文案、变量和组件 |
| `web/src/views/ProxyView.vue` | 133 | 反向代理页，主要是 CSS 变量与组件名 |
| `web/src/views/ProxyRemoteView.vue` | 97 | 公网反代页 |
| `web/src/views/DashboardView.vue` | 97 | 仪表盘 |
| `web/src/views/SettingsView.vue` | 87 | 设置页 |
| `web/src/views/CertificatesView.vue` | 80 | 证书页 |
| `web/src/views/DdnsView.vue` | 77 | DDNS 页 |
| `docs/fnos-app-packaging-plan.md` | 65 | 飞牛应用方案 |
| `docs/improvement-roadmap-2026-09-27.md` | 65 | 改进路线图 |
| `docs/cloudflared-integration-plan.md` | 62 | Cloudflare 集成方案 |
| `CHANGELOG.md` | 57 | 历史版本记录 |
| `docs/brand-rename-plan.md` | 57 | 上一版改名评估文档 |
| `docs/DEPLOYMENT.md` | 56 | 部署文档 |
| `web/src/styles/variables.css` | 51 | CSS 品牌变量定义 |
| `internal/agent/server.go` | 48 | Agent 行为与远端路径 |

---

## 3. 命名映射表

### 3.1 用户可见名称

| 当前 | 目标 | 处理方式 |
| --- | --- | --- |
| `Fonu` | `Havline` | 页面标题、侧栏、登录页、通知、状态页 |
| `github.com/O96u/Fonu` | 待确认的新 GitHub 地址 | 版本检查、Release 链接 |
| Gitea `mrlee/-Fonu.git` | 待确认的新仓库地址 | Compose Git 构建、Updater |
| `Fonu Agent` | `Havline Agent` | UI、日志、systemd Description |
| `Fonu 自托管网关管理面板` | `Havline 自托管网关管理面板` | systemd unit 描述 |

### 3.2 代码与构建标识

| 类型 | 当前 | 目标 | 风险 |
| --- | --- | --- | --- |
| Go module | `github.com/fonu/fonu` | `github.com/havline/havline` | 中 |
| 主程序目录 | `cmd/fonu` | `cmd/havline` | 中 |
| Agent 目录 | `cmd/fonu-agent` | `cmd/havline-agent` | 高 |
| Updater 目录 | `cmd/fonu-updater` | `cmd/havline-updater` | 高 |
| 前端包名 | `fonu-web` | `havline-web` | 低 |
| 嵌入目录 | `cmd/fonu/web/dist` | `cmd/havline/web/dist` | 中 |
| Agent 嵌入文件 | `fonu-agent-linux-amd64` | `havline-agent-linux-amd64` | 高 |
| 构建版本注入 | `github.com/fonu/fonu/internal/version.Version` | `github.com/havline/havline/internal/version.Version` | 高 |
| Docker 镜像 | `fonu:<version>` | `havline:<version>` | 高 |
| Updater 镜像 | `fonu-updater:<version>` | `havline-updater:<version>` | 高 |

### 3.3 运行期契约

| 类型 | 当前 | 目标 | 兼容要求 |
| --- | --- | --- | --- |
| 环境变量 | `FONU_*` | `HAVLINE_*` | 新前缀优先，旧前缀兜底 |
| 数据库 | `fonu.db` | `havline.db` | 自动探测并接管旧库 |
| Cookie | `fonu_session` | `havline_session` | 新 Cookie 优先，旧 Cookie 兜底 |
| 本地主题 | `fonu-theme` | `havline-theme` | 读旧值，写新值 |
| Compose 项目 | `name: fonu` | `name: havline` | 需要卷迁移说明 |
| Compose 服务 | `fonu` | `havline` | Updater 变量必须同步 |
| Updater Socket | `/run/fonu-updater/updater.sock` | `/run/havline-updater/updater.sock` | Compose 两侧同步 |
| Updater Socket 卷 | `fonu-updater-sock` | `havline-updater-sock` | 需要卷迁移 |
| 数据卷 | `fonu-data` | `havline-data` | 不能直接改名，需要复制或保留旧卷名 |
| Agent 响应字段 | `"service": "fonu-agent"` | `"service": "havline-agent"` | 新服务端同时识别新旧值 |

### 3.4 Agent 远端契约

| 类型 | 当前 | 目标 | 风险 |
| --- | --- | --- | --- |
| 二进制 | `/usr/local/bin/fonu-agent` | `/usr/local/bin/havline-agent` | 高 |
| 配置目录 | `/etc/fonu-agent` | `/etc/havline-agent` | 高 |
| 数据目录 | `/var/lib/fonu-agent` | `/var/lib/havline-agent` | 高 |
| systemd unit | `fonu-agent.service` | `havline-agent.service` | 高 |
| 环境变量 | `FONU_AGENT_*` | `HAVLINE_AGENT_*` | 高 |
| Nginx 标记 | `# Managed by fonu-agent.` | `# Managed by havline-agent.` | 高 |
| Nginx 探针 | `fonu-probe.conf` | `havline-probe.conf` | 高 |
| Nginx 残留 | `fonu-*.conf` | `havline-*.conf` | 高 |
| 证书目录 | `/etc/fonu-agent/tls` | `/etc/havline-agent/tls` | 高 |

---

## 4. 分层影响分析

### 4.1 展示层

主要范围：

| 区域 | 文件 |
| --- | --- |
| 浏览器标题 | `web/index.html` |
| 侧栏品牌 | `web/src/layouts/AppLayout.vue` |
| 登录页 | `web/src/views/LoginView.vue` |
| 公开状态页 | `internal/api/status_page_handler.go`、`internal/api/status_handler.go` |
| 通知 | `internal/notify/format.go`、`internal/notify/service.go`、`internal/notify/push.go` |
| Agent 页面 | `web/src/views/AgentView.vue` |
| FRP 页面 | `web/src/views/FrpView.vue` |
| 设置页 | `web/src/views/SettingsView.vue` |
| 提示文案 | `web/src/constants/notifyEvents.ts`、`web/src/router/index.ts` |

高风险文案：

| 路径 | 当前内容 |
| --- | --- |
| `web/src/layouts/AppLayout.vue` | `Fonu`、`让 NAS 访问更简单` |
| `web/src/views/LoginView.vue` | `Fonu` 标题、旧 Logo、`alt="Fonu"` |
| `web/index.html` | `<title>Fonu</title>` |
| `web/src/views/AgentView.vue` | `Fonu Agent`、`fonu-agent`、删除确认文案 |
| `web/src/constants/notifyEvents.ts` | `VPS 上的 fonu-agent 连续无响应` |
| `internal/notify/*` | 邮件、推送中的品牌前缀 |
| `internal/api/status_*` | 公开状态页标题、页脚 |

### 4.2 前端资源层

当前品牌资源：

| 路径 | 用途 | 建议 |
| --- | --- | --- |
| `web/src/assets/brand/logo.png` | 侧栏/登录页旧 Logo | 保留至替换完成，随后删除 |
| `web/src/assets/brand/logo00.png` | 历史/备用 Logo | 确认无引用后处理 |
| `web/src/assets/brand/havline-logo.svg` | 当前第一版 SVG | 如决定 V2，替换引用 |
| `web/src/assets/brand/havline-logo-v1.svg` | 第一版 SVG 备份 | 归档或删除 |
| `web/src/assets/brand/havline-logo-v2.svg` | 推荐第二版 SVG | 作为正式 Logo 候选 |
| `web/assets/image/logo.png` | 镜像内旧 Logo | 替换为 SVG 或导出 PNG |
| `web/assets/image/havline-logo.svg` | 镜像内 SVG | 确认与 `src/assets` 保持一致 |
| `web/assets/image/banner.png` | 品牌 Banner | 需要重绘 |
| `web/src/assets/brand/banner.png` | 前端 Banner | 需要重绘 |
| `web/public/favicon.png` | 浏览器 favicon | 换成 Havline SVG/PNG |
| `web/public/111favicon.png` | 本地备用文件 | 确认是否应该入库 |

### 4.3 CSS 与组件层

CSS 变量数量：`web/src/styles/variables.css` 中 51 处 `--fonu-*`，且被大量 `.vue` 文件引用。

建议：

| 当前 | 目标 |
| --- | --- |
| `--fonu-brand` | `--havline-brand` |
| `--fonu-surface` | `--havline-surface` |
| `--fonu-text` | `--havline-text` |
| `--fonu-space-*` | `--havline-space-*` |
| `--fonu-radius*` | `--havline-radius*` |

组件重命名：

| 当前 | 目标 |
| --- | --- |
| `web/src/components/FonuCard.vue` | `web/src/components/HavlineCard.vue` |
| `.fonu-card` | `.havline-card` |
| `FonuCard` import | `HavlineCard` import |

影响文件：

`web/src/views/AgentView.vue`、`CertificatesView.vue`、`CloudflareTunnelConfigView.vue`、`CloudflareView.vue`、`DashboardView.vue`、`DdnsView.vue`、`ExampleView.vue`、`FrpView.vue`、`LogsView.vue`、`ProxyRemoteView.vue`、`ProxyView.vue`、`SettingsView.vue`。

### 4.4 构建与发布层

| 文件 | 影响 |
| --- | --- |
| `Dockerfile` | Agent 交叉编译、嵌入目录、主程序路径、Updater 二进制、环境变量 |
| `docker-compose.yml` | 项目名、服务名、镜像、容器名、卷、环境变量 |
| `docker-compose.hub.yml` | Git 构建地址、镜像、Updater、Socket、卷、服务名 |
| `docker-compose.host.yml` | 项目名、镜像、容器名、卷、host 网络环境变量 |
| `scripts/build-linux.ps1` | 发布物命名、`cmd/fonu`、agent 输出路径、ldflags |
| `scripts/fonu-install.sh` | `/opt/fonu`、`/var/lib/fonu`、`/etc/fonu`、`fonu.service` |
| `scripts/agent-install.sh` | Agent 二进制、目录、systemd unit、环境变量 |
| `scripts/upgrade.sh` | 容器名、镜像名、`FONU_VERSION` |
| `scripts/upgrade-hub.sh` | Gitea 地址、容器名、镜像名、回滚命令 |
| `.github/workflows/ci.yml` | `cmd/fonu/web/dist` 同步路径 |
| `.github/workflows/docker-release.yml` | Docker Hub 镜像 `DOCKERHUB_USERNAME}/fonu` |
| `.github/workflows/github-release.yml` | 虽无直接旧名，但 Release 流程需要回归 |

### 4.5 Go 代码标识层

`go.mod`：

```text
github.com/fonu/fonu
```

一旦修改 module path，所有 Go 文件中的 import 都需要更新。

当前受影响 Go 文件包括：

| 目录 | 文件数量 |
| --- | ---: |
| `cmd` | 3 |
| `internal/acme` | 7 |
| `internal/agent` | 5 |
| `internal/api` | 27 |
| `internal/app` | 3 |
| `internal/auth` | 3 |
| `internal/backup` | 4 |
| `internal/cloudflared` | 10 |
| `internal/config` | 2 |
| `internal/frp` | 13 |
| `internal/nginx` | 23 |
| `internal/notify` | 7 |
| `internal/updater` | 6 |
| 其他 internal | 17 |
| `migrations` | 2 |

### 4.6 运行契约层

| 文件 | 当前契约 | 改名风险 |
| --- | --- | --- |
| `internal/config/config.go` | `FONU_*`、`fonu.db` | 环境变量和数据库路径 |
| `internal/api/middleware.go` | `fonu_session` | 登录态 |
| `internal/api/auth_handler.go` | 设置/清除 `fonu_session` | 登录态 |
| `web/src/composables/useTheme.ts` | `fonu-theme` | 主题偏好 |
| `internal/backup/service.go` | 备份名、`fonu.db` 检查 | 备份兼容 |
| `internal/backup/archive.go` | `fonu-backup-*`、`fonu.db` | 导入旧备份 |
| `internal/updater/runner.go` | `FONU_VERSION`、`fonu:<version>` | 一键升级 |
| `cmd/fonu-updater/main.go` | Updater 环境变量与默认服务名 | 一键升级 |
| `internal/frp/agent_installer.go` | `/etc/fonu-agent`、`/usr/local/bin/fonu-agent` | Agent 安装与探测 |
| `internal/frp/ssh_installer.go` | 上传 `/tmp/fonu-agent` | Agent 安装 |
| `internal/agent/server.go` | Agent 版本、远端路径、Nginx 标记 | Agent 兼容 |
| `internal/agent/transport.go` | `/etc/fonu-agent/tls` | HTTPS 直连 |
| `internal/monitor/monitor.go` | `fonu-agent` 状态文案与判定 | 监控与通知 |

### 4.7 数据与备份层

| 数据 | 当前值 | 处理策略 |
| --- | --- | --- |
| SQLite 主库 | `<DataDir>/fonu.db` | 检测旧库，优先原子重命名，失败则回退旧路径 |
| Session Secret | `<DataDir>/session_secret` | 文件名不含品牌，不需要改名 |
| 备份文件名 | `fonu-backup-YYYYMMDD-HHMMSS-N.tar.gz` | 新名生成，旧备份仍允许导入 |
| 备份识别 | 必须存在 `fonu.db` | 同时兼容 `havline.db` 与 `fonu.db` |
| Docker 卷 | `fonu-data` | 不自动删旧卷，提供复制/挂载迁移 |
| Updater 卷 | `fonu-updater-sock` | 与 Compose 同步迁移 |
| Agent 数据 | `/var/lib/fonu-agent` | 安装脚本迁移到新目录，旧目录保留一个版本 |
| Agent 配置 | `/etc/fonu-agent/agent.env` | 新目录优先，旧目录兼容读取 |

---

## 5. 兼容迁移设计

### 5.1 环境变量

当前读取逻辑：

```go
envOr("FONU_DATA_DIR", defaultDataDir())
```

目标逻辑：

```go
envOrHavline("HAVLINE_DATA_DIR", "FONU_DATA_DIR", defaultDataDir())
```

规则：

1. `HAVLINE_*` 优先。
2. `FONU_*` 作为 fallback。
3. 命中旧变量时启动日志给出一条迁移提示。
4. 至少保留一个 stable 版本后再删除旧前缀。

必须兼容的环境变量：

```text
FONU_DATA_DIR
FONU_LISTEN
FONU_SESSION_SECRET
FONU_NGINX_BIN
FONU_NGINX_PID
FONU_NGINX_MIME_TYPES
FONU_NGINX_HTTP_PORT
FONU_NGINX_HTTPS_PORT
FONU_FRPC_BIN
FONU_FRP_PID
FONU_UPDATER_SOCKET
FONU_AGENT_LISTEN
FONU_AGENT_ALLOW_REMOTE
FONU_AGENT_TOKEN
FONU_AGENT_NGINX_CONF_DIR
FONU_AGENT_NGINX_LOG_DIR
FONU_AGENT_NGINX_BIN
FONU_AGENT_CERTS_DIR
FONU_AGENT_DATA_DIR
FONU_AGENT_FRPS_CONFIG
FONU_AGENT_FRPS_BIN
FONU_UPDATE_PROJECT_DIR
FONU_UPDATE_COMPOSE_FILE
FONU_UPDATE_SERVICE
FONU_UPDATE_MODE
FONU_UPDATE_REPO
FONU_UPDATE_BRANCH
FONU_UPDATE_APP_URL
FONU_UPDATE_HOST_DIR
FONU_UPDATE_PROJECT_NAME
FONU_UPDATE_SELF_IMAGE
FONU_UPDATE_DOCKER_SOCK
FONU_VERSION
FONU_DATA_PATH
```

### 5.2 数据库接管

判断顺序：

1. `havline.db` 存在：直接使用。
2. `havline.db` 不存在且 `fonu.db` 存在：尝试原子重命名。
3. 重命名失败：继续使用 `fonu.db`，日志提示迁移。
4. 两者都不存在：创建 `havline.db`。

禁止行为：

1. 不能在旧库存在时直接创建新空库。
2. 不能删除旧库。
3. 不能修改 `session_secret`，否则已有加密凭据无法解密。

### 5.3 Cookie 与本地偏好

| 项目 | 读策略 | 写策略 |
| --- | --- | --- |
| Session Cookie | `havline_session` 优先，回退 `fonu_session` | 只写 `havline_session` |
| 主题偏好 | `havline-theme` 优先，回退 `fonu-theme` | 读到旧值后回写新键 |

### 5.4 Docker Compose 与 Volume

不能直接把 `fonu-data` 改成 `havline-data` 就结束。

推荐策略：

1. 新 compose 使用 `name: havline`。
2. 新卷名为 `havline-data`。
3. 文档提供旧卷到新卷的复制命令。
4. 升级前先备份旧卷。
5. 首版兼容可以直接把 `havline-data` 显式指向旧卷进行阶段性过渡。

示例：

```yaml
volumes:
  havline-data:
    name: fonu-data
```

上述做法只用于过渡版本。完全切换完成后再恢复为 `name: havline-data`。

### 5.5 Updater 迁移

一键升级涉及四组变量：

```text
FONU_UPDATE_SERVICE=fonu
FONU_UPDATE_APP_URL=http://fonu:6893
FONU_UPDATE_SELF_IMAGE=fonu-updater:<version>
FONU_UPDATER_SOCKET=/run/fonu-updater/updater.sock
```

改名时必须一起处理：

| 当前 | 目标 |
| --- | --- |
| Service `fonu` | Service `havline` |
| App URL `http://fonu:6893` | `http://havline:6893` |
| Updater 镜像 `fonu-updater` | `havline-updater` |
| Updater socket 目录 | `/run/havline-updater/updater.sock` |
| Socket volume | `havline-updater-sock` |

### 5.6 Agent 迁移

推荐 Agent 安装脚本流程：

1. 检测旧 `/etc/fonu-agent/agent.env`。
2. 停止并禁用 `fonu-agent.service`。
3. 安装新 `havline-agent` 二进制。
4. 创建 `/etc/havline-agent`、`/var/lib/havline-agent`。
5. 迁移或复制旧 token、证书和 frps 配置。
6. 安装 `havline-agent.service`。
7. 保留旧目录一个版本，便于回滚。
8. 服务端探测同时检查新旧二进制、目录和 systemd unit。

Agent 版本必须递增。当前 `AgentVersion = 0.14.1`，品牌与路径变更建议递增到 `0.15.0`。

---

## 6. 实施阶段

### 阶段 0：冻结名称与外部身份

需要确认：

1. GitHub 仓库新地址。
2. Gitea 仓库新地址。
3. Docker Hub 命名空间与仓库名。
4. 是否保留旧仓库重定向。
5. 是否发布旧镜像名兼容标签。
6. 域名与商标状态。

### 阶段 1：兼容层先行

先不替换全仓品牌，只增加兼容能力：

1. `internal/config/config.go` 支持 `HAVLINE_*` 与 `FONU_*` 双读。
2. 数据库支持 `havline.db` 与 `fonu.db`。
3. Session Cookie 支持新旧双读。
4. `useTheme.ts` 支持新旧 key。
5. Agent 探测支持新旧路径。
6. Backup 导入支持新旧数据库名。
7. Updater 支持新旧服务名与镜像名过渡。

### 阶段 2：展示层改名

1. `web/index.html` 标题。
2. `AppLayout.vue` 品牌名、标语、Logo。
3. `LoginView.vue` 标题、Logo、说明文案。
4. 公开状态页和通知品牌名。
5. `web/src/constants/notifyEvents.ts` 和路由 meta。
6. 前端资源与 favicon。
7. `FonuCard.vue` -> `HavlineCard.vue`。
8. `--fonu-*` -> `--havline-*`。

### 阶段 3：代码标识改名

1. `go.mod` module path。
2. 全仓 Go import。
3. `cmd/fonu` -> `cmd/havline`。
4. `cmd/fonu-agent` -> `cmd/havline-agent`。
5. `cmd/fonu-updater` -> `cmd/havline-updater`。
6. Dockerfile ldflags。
7. Agent 嵌入文件。
8. CI `cmd/fonu/web/dist` 路径。

### 阶段 4：交付与升级链改名

1. Docker 镜像名。
2. Updater 镜像名。
3. Compose service、container、volume。
4. `scripts/upgrade.sh`、`scripts/upgrade-hub.sh`。
5. 一键升级配置。
6. Docker Release workflow。

### 阶段 5：Agent 全量切换

1. 新 Agent 二进制。
2. 新 systemd unit。
3. 新目录和环境变量。
4. Nginx 标记兼容。
5. 旧 Agent 探测、升级、回滚。
6. Agent 版本递增与双架构重编。

### 阶段 6：文档与历史清理

1. README、DEPLOYMENT、容器文档、飞牛打包文档。
2. CHANGELOG 只追加，不改历史发布记录。
3. 删除旧兼容代码。
4. 移除旧 Logo 和旧资源。
5. 全仓 grep 验收。

---

## 7. 验证矩阵

### 7.1 构建验证

```powershell
go build ./...
go vet ./...
go test ./... -count=1
cd web
npx vue-tsc --noEmit
npx vite build
npm test
```

### 7.2 Docker 验证

1. `docker-compose.yml` 新建部署。
2. `docker-compose.hub.yml` 单文件部署。
3. `docker-compose.host.yml` host 网络部署。
4. 旧 `fonu-data` 卷启动新版本。
5. 旧 `.env` 中 `FONU_*` 继续生效。
6. 新 `.env` 中 `HAVLINE_*` 优先。
7. 一键升级成功。
8. 回滚到旧镜像成功。

### 7.3 数据验证

1. 旧 `fonu.db` 自动接管。
2. 旧备份导入。
3. 新备份导出与恢复。
4. `session_secret` 不变，凭据仍能解密。
5. 旧 Cookie 能继续登录一次并迁移到新 Cookie。
6. 旧主题偏好迁移。

### 7.4 Agent 验证

1. 旧 Agent -> 新服务端。
2. 新 Agent -> 旧服务端。
3. 新 Agent -> 新服务端。
4. 旧 `/etc/fonu-agent` 升级到新目录。
5. 旧 `fonu-agent.service` 被正确停止和替换。
6. 旧 `fonu-probe.conf` 和 `fonu-*.conf` 能被识别和清理。

---

## 8. 回滚策略

| 阶段 | 回滚方式 |
| --- | --- |
| 展示层 | Git revert 文案与图片 |
| CSS/组件 | Git revert 组件重命名 |
| 环境变量双读 | 保留旧逻辑，无需回滚 |
| 数据库 | 优先使用旧库，反向重命名需单独文档 |
| Docker Volume | 保留旧卷，不自动删除 |
| Updater | 恢复旧 compose 与旧镜像 |
| Agent | 保留旧目录与旧 unit，重新启用旧服务 |
| Module path | 整体 revert，避免混合 import |

---

## 9. 建议提交拆分

| 提交 | 内容 |
| --- | --- |
| 1 | 增加 Havline Logo 资产与预览 |
| 2 | 品牌改名方案文档 |
| 3 | 环境变量、DB、Cookie、主题兼容层 |
| 4 | 前端展示层与资源替换 |
| 5 | CSS 变量与组件重命名 |
| 6 | Go module、cmd 目录、Agent 嵌入路径 |
| 7 | Docker、Compose、Updater、脚本 |
| 8 | Agent 远端安装与迁移 |
| 9 | 文档全面更新 |
| 10 | 兼容层清理 |

每个提交都必须能独立构建和回滚，不要把 10 个阶段压成一个提交。

---

## 10. 完整文件清单

以下文件为当前扫描到包含 `Fonu / fonu / FONU` 的全部文件。生成目录、`node_modules`、`.git` 不在此列表中。

### 10.1 GitHub Actions 与忽略规则

```text
.github/workflows/ci.yml
.github/workflows/docker-release.yml
.gitignore
```

### 10.2 Go、module 与迁移

```text
go.mod
cmd/fonu/main.go
cmd/fonu-agent/main.go
cmd/fonu-updater/main.go
internal/acme/dns_provider.go
internal/acme/dns_provider_test.go
internal/acme/domains.go
internal/acme/download.go
internal/acme/job.go
internal/acme/service.go
internal/acme/volcengine_dns.go
internal/agent/insights.go
internal/agent/route_test.go
internal/agent/server.go
internal/agent/transport.go
internal/agent/versions.go
internal/agent/versions_test.go
internal/api/auth_handler.go
internal/api/auth_handler_test.go
internal/api/backup_handler.go
internal/api/cert_handler.go
internal/api/chinacidr_handler.go
internal/api/cloudflare_handler.go
internal/api/ddns_handler.go
internal/api/deps.go
internal/api/discovery_handler.go
internal/api/frpmulti_handler.go
internal/api/logs_handler.go
internal/api/logs_handler_test.go
internal/api/metrics_handler.go
internal/api/middleware.go
internal/api/middleware_test.go
internal/api/nginx_handler.go
internal/api/notify_handler.go
internal/api/proxy_handler.go
internal/api/proxy_security.go
internal/api/router.go
internal/api/router_auth_test.go
internal/api/settings_handler.go
internal/api/status_handler.go
internal/api/status_page_handler.go
internal/api/token_handler.go
internal/api/update_handler.go
internal/api/version_handler.go
internal/api/version_handler_test.go
internal/app/app.go
internal/app/monitor_source.go
internal/app/monitor_source_test.go
internal/auth/auth.go
internal/auth/auth_test.go
internal/auth/token_test.go
internal/backup/archive.go
internal/backup/auto.go
internal/backup/auto_test.go
internal/backup/service.go
internal/chinacidr/service.go
internal/cloudflared/cloudflare_api.go
internal/cloudflared/config.go
internal/cloudflared/config_test.go
internal/cloudflared/health_test.go
internal/cloudflared/manager.go
internal/cloudflared/model.go
internal/cloudflared/operations.go
internal/cloudflared/routes.go
internal/cloudflared/service.go
internal/cloudflared/settings_test.go
internal/cloudflared/store.go
internal/config/config.go
internal/config/config_test.go
internal/db/db_test.go
internal/ddns/service.go
internal/frp/agent_client.go
internal/frp/agent_insights.go
internal/frp/agent_installer.go
internal/frp/agentbin/embed.go
internal/frp/frps_template.go
internal/frp/manager.go
internal/frp/model.go
internal/frp/route_drift.go
internal/frp/runtime.go
internal/frp/service.go
internal/frp/ssh_installer.go
internal/frp/ssh_installer_test.go
internal/fsutil/fsutil_test.go
internal/logstore/dashboard_access_test.go
internal/logstore/reader.go
internal/monitor/monitor.go
internal/nginx/certs.go
internal/nginx/comments.go
internal/nginx/custom.go
internal/nginx/custom_test.go
internal/nginx/endpoints.go
internal/nginx/endpoints_test.go
internal/nginx/errors.go
internal/nginx/errors_test.go
internal/nginx/generate.go
internal/nginx/generate_china_bypass_test.go
internal/nginx/generate_china_cidr_path_test.go
internal/nginx/generate_comments_test.go
internal/nginx/generate_https_no_cert_test.go
internal/nginx/generate_https_with_cert_test.go
internal/nginx/generate_multi_test.go
internal/nginx/generate_order_test.go
internal/nginx/generate_security_test.go
internal/nginx/generate_seed_mix_test.go
internal/nginx/generate_test.go
internal/nginx/global_framework_test.go
internal/nginx/htpasswd.go
internal/nginx/htpasswd_test.go
internal/nginx/nginx.go
internal/nginx/nginx_integration_test.go
internal/nginx/nginx_unavailable_test.go
internal/nginx/pid_test.go
internal/nginx/realip.go
internal/nginx/security.go
internal/nginx/versions.go
internal/nginx/versions_test.go
internal/nginxtmpl/nginxtmpl_test.go
internal/notify/format.go
internal/notify/format_test.go
internal/notify/push.go
internal/notify/push_test.go
internal/notify/service.go
internal/notify/smtp_test.go
internal/notify/store.go
internal/notify/store_test.go
internal/proxy/custom_endpoints_test.go
internal/proxy/proxy.go
internal/proxy/security.go
internal/proxy/security_test.go
internal/service/nginx_config.go
internal/service/proxy.go
internal/service/proxy_opts.go
internal/sysinfo/sysinfo_windows.go
internal/traffic/collector.go
internal/traffic/collector_test.go
internal/updater/client.go
internal/updater/model.go
internal/updater/remote_version_test.go
internal/updater/runner.go
internal/updater/server.go
internal/updater/server_test.go
migrations/023_frp_server_agent.sql
migrations/035_proxy_exits.sql
```

### 10.3 前端 Web

```text
web/index.html
web/package.json
web/package-lock.json
web/src/App.vue
web/src/main.ts
web/src/api/client.ts
web/src/api/types.ts
web/src/components/ConfigDiffView.vue
web/src/components/EmptyState.vue
web/src/components/FonuCard.vue
web/src/components/LoadError.vue
web/src/components/MetricsTrendPanel.vue
web/src/components/NginxCodeEditor.vue
web/src/components/PageHeader.vue
web/src/components/ProxyAccessLogBox.vue
web/src/components/StatCard.vue
web/src/components/StatusBadge.vue
web/src/components/TaskProviderForm.vue
web/src/components/UpdateModal.vue
web/src/composables/useTheme.ts
web/src/constants/app.ts
web/src/constants/notifyEvents.ts
web/src/layouts/AppLayout.vue
web/src/router/index.ts
web/src/styles/variables.css
web/src/utils/clipboard.ts
web/src/views/AgentView.vue
web/src/views/CertificatesView.vue
web/src/views/CloudflareTunnelConfigView.vue
web/src/views/CloudflareView.vue
web/src/views/DashboardView.vue
web/src/views/DdnsView.vue
web/src/views/ExampleView.vue
web/src/views/FrpView.vue
web/src/views/LoginView.vue
web/src/views/LogsView.vue
web/src/views/ProxyRemoteView.vue
web/src/views/ProxyView.vue
web/src/views/SettingsView.vue
web/src/views/StatusPageView.vue
```

### 10.4 部署、Docker 与脚本

```text
Dockerfile
docker-compose.yml
docker-compose.hub.yml
docker-compose.host.yml
scripts/agent-install.sh
scripts/build-linux.ps1
scripts/fonu-install.sh
scripts/upgrade.sh
scripts/upgrade-hub.sh
```

### 10.5 文档与历史

```text
CHANGELOG.md
README.md
docs/DEPLOYMENT.md
docs/FRP公网暴露操作手册.md
docs/README.md
docs/agent-capability-plan.md
docs/agent-listen-strategy-plan.md
docs/agent-proxy-roadmap.md
docs/agent-proxy-settings-plan.md
docs/agent-view-design-v2.md
docs/agent-view-design.md
docs/agent-view-refactor-plan.md
docs/brand-rename-plan.md
docs/cloudflared-current-features.md
docs/cloudflared-feature-plan.md
docs/cloudflared-fonu-fit-plan.md
docs/cloudflared-github-ux-research.md
docs/cloudflared-integration-plan.md
docs/container-deploy-and-data.md
docs/feature-benchmark-2026-09-24.md
docs/fnos-app-packaging-plan.md
docs/frp-server-list-merge-plan.md
docs/frps-features.md
docs/frps服务端管理方案.md
docs/frps管理方案.md
docs/improvement-roadmap-2026-09-27.md
docs/project-analysis-2026-09-27.md
docs/project-audit-2026-09-24.md
docs/proxy-remote-design.md
docs/system-upgrade.md
docs/upstream-sync-plan.md
```

### 10.6 不在 grep 结果中但需要同步处理

| 路径 | 原因 |
| --- | --- |
| `web/src/assets/brand/havline-logo-v2.svg` | 新 Logo 资源，需要决定最终接入口径 |
| `web/src/assets/brand/havline-logo-v1.svg` | Logo 历史版本 |
| `web/src/assets/brand/havline-logo.svg` | 当前已接入侧栏 |
| `web/src/assets/brand/logo00.png` | 历史/备用 Logo |
| `web/src/assets/brand/banner.png` | 旧品牌 Banner |
| `web/src/assets/brand/banner-art.png` | 旧品牌 Banner 素材 |
| `web/assets/image/havline-logo.svg` | 镜像资源目录中的 SVG |
| `web/assets/image/logo.png` | 旧品牌图片，需要替换 |
| `web/assets/image/banner.png` | 旧品牌 Banner，需要重绘 |
| `web/public/favicon.png` | 旧 favicon，需要替换 |
| `web/public/111favicon.png` | 未确认用途 |
| `docs/havline-brand-rename-plan.md` | 本方案文档本身 |
| `docs/havline-logo-preview.html` | 第一版 Logo 预览 |
| `docs/havline-logo-v2-preview.html` | 第二版 Logo 预览 |
| `cmd/fonu/web/dist/**` | 生成目录，改名后由构建重新生成 |
| `internal/frp/agentbin/fonu-agent-linux-amd64` | 生成/嵌入二进制，改名后重新编译 |
| `internal/frp/agentbin/fonu-agent-linux-arm64` | 生成/嵌入二进制，改名后重新编译 |
| `fonu-agent.exe` | 本地构建产物 |

---

## 11. 最终执行顺序

推荐顺序：

1. 确认最终 Logo、GitHub、Gitea、Docker Hub 身份。
2. 增加兼容层。
3. 替换展示层。
4. 替换 CSS 与组件。
5. 替换 Go module 与 cmd 目录。
6. 替换 Docker、Compose、Updater。
7. 替换 Agent 远端安装与探测逻辑。
8. 全量迁移文档。
9. 跑完整回归。
10. 保留兼容层一个版本。
11. 删除旧兼容代码与旧资源。

---

## 12. 验收 grep

改名过程中可重复执行：

```powershell
rg --hidden -in "fonu" `
  --glob '!web/node_modules/**' `
  --glob '!web/dist/**' `
  --glob '!cmd/havline/web/dist/**' `
  --glob '!.git/**' `
  --glob '!output/**' `
  --glob '!.playwright-cli/**'
```

分类验收：

```powershell
rg -n "FONU_"
rg -n "fonu\.db|fonu_session|fonu-theme"
rg -n "fonu-agent|fonu-probe|/etc/fonu|/var/lib/fonu"
rg -n "github\.com/fonu/fonu|O96u/Fonu|/fonu:|fonu-data"
rg -n -- "--fonu-"
```

最终目标：

1. 源码中不再出现旧品牌展示文案。
2. 运行时代码不再依赖旧环境变量、旧 Cookie、旧数据库文件名。
3. 兼容层只保留明确记录的旧值 fallback。
4. 文档历史记录不改写，只新增迁移说明。
5. 新安装、旧数据升级、旧 Agent 升级、旧 Compose 回滚四条路径全部通过。
