# Havline 部署安装文档

> 适用版本：v1.9.0。本文覆盖 Docker / 飞牛 fnOS / Linux 原生部署、数据持久化、敏感数据保护、updater sidecar、一键升级、回滚和故障排查。文中旧版本包名仅用于说明历史交付物，不代表当前发布包名。
>
> 当前发布源：`https://jayhub.top/mrlee/Havline.git`，分支：`main`。`docker-compose.hub.yml` 和 updater sidecar 都固定使用这组仓库地址与分支；如果实际发布仓库发生变化，必须同步修改 Compose 文件、updater 环境变量和本节说明后再部署。
> 功能细节与组合场景请配合项目内两份文档：`README.md`（功能总览）与 FRP 页面右上「使用说明」（内置十个组合场景详解）。

## 0. 交付物清单与安装路径速查

| 交付物 | 内容 | 适用 | 安装方式 |
| --- | --- | --- | --- |
| `havline-1.9.0-linux-amd64.tar.gz`（大小以实际发布包为准） | 编译好的 Linux 二进制（前端已内嵌）+ migrations + systemd + 一键脚本 | **裸机 Linux 服务器**，目标机只需系统 nginx | 解压 → `sudo bash install.sh` |
| `havline-1.9.0-image.tar.gz`（大小以实际发布包为准） | 构建好的 Docker 镜像 `havline:local`（含 Nginx，不预装 frpc） | 目标机有 Docker、要离线安装 | `docker load` → `docker run` |
| `havline-1.9.0-src.zip`（大小以实际发布包为准） | 完整源码 + Dockerfile/compose + 部署文档 | 要自行构建 / 二次开发 / 交叉编译 | `docker compose build` 后按方式 A/B 部署（见第 2 节） |

**怎么选**：飞牛 fnOS 首选镜像包导入（图形界面，见第 3 节末「飞牛 fnOS 部署详解」），要在 NAS 上构建则用源码包 + host 模式 compose；普通 Linux 服务器要省事用 Linux 运行包（免 Docker）；目标机不方便联网构建用镜像包离线导入。三者的功能完全一致——容器里跑的也是同一份二进制 + 系统 nginx。

## 1. 包内容与环境要求

### 1.1 源码包内容

`havline-1.9.0-src.zip`（文件大小与条目数以实际发布包为准）为完整可构建源码：

| 目录/文件 | 说明 |
| --- | --- |
| `cmd/` `internal/` | Go 后端源码（含已迁移的多服务端 FRP 模块） |
| `web/` | Vue 3 + TypeScript 前端源码 |
| `migrations/` | SQLite 数据库迁移 001–036（启动时自动按序应用） |
| `Dockerfile` / `docker-compose*.yml` | 本地构建镜像与三种部署编排 |
| `VERSION` / `CHANGELOG.md` | 版本号单一来源（当前 `1.9.0`）与版本记录 |
| `deploy/nginx/mime.types` | 容器内 Nginx 依赖的 MIME 类型文件 |
| `docs/screenshots/` | README 引用的界面截图 |

**已排除**（打包时即剔除，无需自行处理）：`.data/`（本地运行数据）、`web/node_modules`（构建时自动安装）、`internal/frp-1.0.2/` 与 `FrpView-1.0.2.vue`（弃用归档）、`*.db`。

### 1.2 部署机要求

| 项 | 要求 |
| --- | --- |
| 系统 | 飞牛 fnOS（主要目标平台）；其他 Linux x86_64 / arm64 理论可用，未充分测试 |
| Docker | 20.10+（含 compose v2） |
| 内存 | ≥ 512MB 空闲（构建期建议 ≥ 2GB，前端编译吃内存） |
| 磁盘 | ≥ 2GB（镜像约 300MB + 数据目录按日志量增长） |
| 网络 | 构建期需访问 npm registry 与 Go module proxy；运行期需访问 GitHub（按需下载 frpc）与你的 DNS 服务商 API（DDNS/证书） |
| 端口 | 管理端口 6893；反代端口默认 80/443（Bridge）或 18080/9443（飞牛 host 模式） |

## 2. 构建镜像（所有部署方式的前提）

解压源码包后进入项目根目录执行：

```bash
docker compose build
```

一次构建同时产出：前端 `dist`、Go 后端二进制与运行镜像（frpc 不预装，运行期用「应用配置」按需下载），标签为 `havline:local`。

**版本号**：读取根目录 `VERSION` 文件（当前 `1.9.0`）自动注入前后端显示；要改版本号只需编辑该文件后重新 build。

**网络受限环境**可覆盖基础镜像与 frpc 下载源（默认值见 `Dockerfile` 顶部注释）：

```bash
docker build \
  --build-arg NODE_IMAGE=docker.m.daocloud.io/library/node:22-alpine \
  --build-arg GO_IMAGE=docker.m.daocloud.io/library/golang:1.23-bookworm \
  --build-arg RUNTIME_IMAGE=docker.m.daocloud.io/library/debian:bookworm-slim \
  -t havline:local .
```

## 3. 三种部署方式（按环境二选一）

### 方式 A：飞牛 fnOS / 已占用 80/443 的环境 —— host 模式（推荐）

飞牛系统自带 Nginx 占用 80/443，Havline 使用 host 网络 + 高位反代端口避免冲突。

```bash
# host 编排不启用 updater sidecar；HAVLINE_SESSION_SECRET 留空时由应用生成并持久化
# 如需显式密钥，请提前写入同层 .env
docker compose -f docker-compose.host.yml up -d --build
```

- 无端口映射，容器直接使用宿主机网络栈；
- 反代端口由环境变量 `HAVLINE_NGINX_HTTP_PORT=18080`、`HAVLINE_NGINX_HTTPS_PORT=9443` 决定（文件内已内置）；
- 入口：管理后台 `http://<NAS-IP>:6893`，HTTP 反代 `http://<NAS-IP>:18080`，HTTPS 反代 `https://<NAS-IP>:9443`；
- 数据持久化在命名卷 `havline-data`（挂载到容器 `/data`）。

### 方式 B：其他环境 / 空闲 80/443 —— Bridge 端口映射

```bash
docker compose -f docker-compose.hub.yml up -d --build
```

- 端口映射 `6893:6893`、`18080:80`、`9443:443`（宿主机 18080/9443 → 容器 80/443）；
- 数据同样在命名卷 `havline-data`；
- 主程序首次启动时会在共享卷中自动生成 updater Token，sidecar 等待并读取同一文件。旧部署如显式设置 `HAVLINE_UPDATER_TOKEN`，仍会优先使用该值。

### 方式 C：docker run（不想用 compose）

```bash
docker run -d --name havline \
  --net=host \
  -v ./data:/data \
  -e HAVLINE_SESSION_SECRET=可选的固定密钥 \
  --restart unless-stopped \
  havline:local
```

host 模式下反代端口用环境变量控制：`-e HAVLINE_NGINX_HTTP_PORT=18080 -e HAVLINE_NGINX_HTTPS_PORT=9443`。Bridge 环境改为 `-p 6893:6893 -p 18080:80 -p 9443:443`（去掉 `--net=host`）。

**生成随机密钥**：Linux 使用 `openssl rand -hex 32`；PowerShell 可使用 `-join ((48..57)+(97..102) | Get-Random -Count 64 | % {[char]$_})`。

> **`HAVLINE_SESSION_SECRET`** 是会话签名和敏感凭据加密密钥。当前代码在它为空时，会在 `/data/session_secret` 生成 32 字节随机值并以 `0600` 权限持久化。已有数据后不要更换，否则数据库中已加密的 DDNS、FRP、通知、Cloudflare 等凭据无法解密。
>
> **`HAVLINE_UPDATER_TOKEN`** 是主程序与 updater sidecar 的共享鉴权密钥。包含 updater 的 Compose 会由主程序在共享卷中自动生成 Token 文件，默认不需要手动配置。旧部署仍可用环境变量覆盖；Token 文件不得提交到仓库、截图、工单或公开备份说明。

### Linux 服务器完整部署步骤（Ubuntu / Debian / CentOS）

```bash
# 0. 安装 Docker（已装跳过）
curl -fsSL https://get.docker.com | sh
sudo systemctl enable --now docker

# 1. 解压源码包并进入目录
unzip havline-1.9.0-src.zip -d havline && cd havline

# 2. 构建镜像（约 3-10 分钟，取决于网络与机器性能）
docker compose build

# 3. 可选：生成会话密钥并启动（Bridge 模式，映射 6893/18080/9443）
echo "HAVLINE_SESSION_SECRET=$(openssl rand -hex 32)" > .env
docker compose -f docker-compose.hub.yml up -d

# 4. 防火墙放行（按需）
sudo ufw allow 6893/tcp     # 管理后台
sudo ufw allow 80/tcp 443/tcp   # 反代（Bridge 模式容器内监听 80/443）
# firewalld 用: firewall-cmd --permanent --add-port=6893/tcp && firewall-cmd --reload

# 5. 验证
docker logs havline | grep -i password   # 取 admin 初始密码
curl -I http://127.0.0.1:6893/login   # 应返回 200
```

- **开机自启**：compose 内已含 `restart: unless-stopped`，且第 0 步已 `enable docker`，重启服务器自动拉起；
- **80/443 被系统 Nginx 占用**：改用 `docker-compose.host.yml`（host 网络 + 18080/9443 高位端口），或改 `docker-compose.hub.yml` 里的宿主端口映射；
- **离线部署**（目标机不方便联网构建）：在有网的机器上 `docker compose build` 后 `docker save havline:local | gzip > havline.tar.gz`，拷贝到目标机 `docker load < havline.tar.gz`，再用方式 C 的 `docker run` 启动（无需再构建）；
- **数据位置**：命名卷 `havline-data`（`docker volume inspect havline-data` 查看宿主机实际路径），备份该卷即备份全部配置与证书。

### Linux 运行包安装（免 Docker，裸机部署）

适用：不想装 Docker 的 Linux 服务器。前提：系统已装 nginx（Havline 复用它做反代），root 权限。

```bash
# 1. 解压
tar xzf havline-1.9.0-linux-amd64.tar.gz -C havline && cd havline

# 2. 一键安装（自动建 havline 系统用户、装到 /opt/havline、生成随机密钥、注册 systemd 开机自启）
sudo bash install.sh

# 3. 验证
systemctl status havline
curl -I http://127.0.0.1:6893/login
journalctl -u havline | grep -i password    # 取 admin 初始密码
```

包内容：`havline` 二进制（前端已 `go:embed` 内嵌，无需单独部署 dist）、`migrations/`（从二进制同级目录自动查找）、`deploy/nginx/mime.types`、`havline.service`（systemd 模板）、`install.sh`、`DEPLOYMENT.md`。

关键路径（install.sh 自动配置）：

| 项 | 值 |
| --- | --- |
| 二进制 | `/opt/havline/havline` |
| 数据目录 | `/var/lib/havline`（数据库、证书、日志全在这） |
| 会话密钥 | `/etc/default/havline`（重复安装保留原值） |
| 服务 | `systemctl restart havline` 生效配置改动 |

**备份/迁移**：停服务后打包 `/var/lib/havline` 即可（`tar czf havline-data.tar.gz /var/lib/havline`），新机器解压到同路径 + 装同版本二进制即完成迁移。

**手动安装**（不用脚本）：按包内 `README.txt` 要点——建 `havline` 系统用户、复制二进制与 migrations 到 `/opt/havline`、`EnvironmentFile` 写入随机密钥、`systemctl enable --now havline`。

> 镜像包（`havline-1.9.0-image.tar.gz`）来源：在任意装了 Docker 的机器上对源码包执行 `docker compose build` 后 `docker save havline:local | gzip > havline-1.9.0-image.tar.gz`；目标机 `docker load < havline-1.9.0-image.tar.gz` 后按方式 C 启动。

### 飞牛 fnOS 部署详解（图形界面，不敲命令）

前置：fnOS 应用中心安装 Docker 应用（较新版本自带）。以下菜单名以 fnOS 当前版本实际界面为准，大致路径一致。

**方式一：导入现成镜像（推荐，5 分钟）**

1. **上传文件**：把 `havline-1.9.0-image.tar.gz` 通过文件管理 / SMB 上传到 NAS，如共享目录 `docker/havline/`；
2. **导入镜像**：打开 Docker 应用 → 镜像 → 导入（从文件加载）→ 选择该 tar.gz → 等待镜像列表出现 `havline:local`（若界面只接受 .tar，先在电脑上解压出内层的 .tar 再导入）；
3. **创建容器**：容器 → 创建：

| 配置项 | 填写 |
| --- | --- |
| 名称 | `havline` |
| 镜像 | `havline:local` |
| 网络 | **host**（飞牛 80/443 被系统占用，host + 高位端口最省事；UI 不支持 host 时选 Bridge 并映射 6893/18080/9443） |
| 存储挂载 | NAS 建目录（如 `/vol1/…/docker/havline/data`）→ 映射到容器 `/data`（读写） |
| 环境变量 | `HAVLINE_DATA_DIR=/data`、`HAVLINE_LISTEN=:6893`、`HAVLINE_NGINX_HTTP_PORT=18080`、`HAVLINE_NGINX_HTTPS_PORT=9443`；`HAVLINE_SESSION_SECRET` 可留空自动生成 |
| 重启策略 | 自动重启 / unless-stopped（开机自启） |

4. **启动并取初始密码**：容器详情 → 日志，搜 `password`；浏览器打开 `http://<NAS-IP>:6893` 登录。

**方式二：Compose 项目（用源码包，在 NAS 上构建）**

1. 把源码包 zip 上传到 NAS 并解压（文件管理支持解压，或电脑解压后整目录上传）；
2. Docker 应用 → 项目（Compose）→ 新建：指定项目目录为解压后的源码目录、使用 `docker-compose.host.yml`（或直接把该文件内容粘贴到 Compose 编辑器）；
3. 在源码目录同层放一个 `.env` 文件；host 编排不需要 updater Token，`HAVLINE_SESSION_SECRET` 可留空，应用会在数据目录生成并持久化；
4. 构建并启动。⚠️ NAS 上构建耗时较长（前端编译），低配机器建议改用方式一；
5. 后续升级：上传新源码覆盖 → 项目重新构建即可，数据在卷里不受影响。

**Compose 构建报 401（docker.fnnas.com）**：fnOS 的 Docker 设置里默认配了自家加速地址，但它对 `library/` 官方镜像拒绝匿名拉取。三种解法任选：

1. **改用镜像包（最省事）**：跳过构建，按方式一导入 `havline-1.9.0-image.tar.gz`；
2. **覆盖基础镜像源**：在源码目录的 `.env` 里加三行（已有 HAVLINE_SESSION_SECRET 就追加在下面）：
   ```
   NODE_IMAGE=docker.m.daocloud.io/library/node:22-alpine
   GO_IMAGE=docker.m.daocloud.io/library/golang:1.23-bookworm
   RUNTIME_IMAGE=docker.m.daocloud.io/library/debian:bookworm-slim
   GOPROXY=https://goproxy.cn,direct
   ```

   （Dockerfile/compose 现已内置这些默认值，无需 .env；此处列出供手动 docker build 参考或覆盖。`GOPROXY` 解决 go mod download 访问 proxy.golang.org 超时——国内默认 goproxy.cn，海外可覆盖回 direct。）
   然后 Compose 重新构建——compose 文件已支持这三个变量透传；
3. **换/关掉 fnOS 加速地址**：Docker 应用 → 设置 → 镜像源，把 `docker.fnnas.com` 换成可用加速源（如 `docker.m.daocloud.io`）或清空，再重新构建。

**fnOS 注意事项**：

- 80/443 已被 fnOS 系统 Nginx 占用——**不要**把反代规则监听 80/443，用默认 18080/9443；
- host 模式下容器端口即宿主机端口，fnOS 防火墙（如有启用）放行 6893 / 18080 / 9443；
- 数据目录建议放在存储池挂载路径下（`/vol1/…`，以文件管理器显示的实际路径为准），系统重装时数据不随系统分区丢失；
- 升级：方式一重新导入新镜像 → 停止并删除旧容器 → 用相同挂载/环境变量重建（数据在映射目录，不丢）；方式二直接重新构建。

## 4. 首次登录与安全配置

> `docker-compose.host.yml` 不包含 updater sidecar；`docker-compose.yml` 和 `docker-compose.hub.yml` 会由主程序自动生成共享 Token。不要把 host 模式的“自动生成会话密钥”和 updater 的“自动生成共享 Token”混为一谈。


1. 启动后查看初始密码：`docker logs havline | grep -i password`（自动生成的 admin 初始密码会打印在日志里）；
2. 浏览器打开 `http://<NAS-IP>:6893/login`，用 `admin` + 初始密码登录；
3. **立即**在「设置 → 安全」修改管理员密码；
4. 确认数据目录中存在 `session_secret`，并将其与数据库、证书和配置一起纳入备份；不要在已有数据后更换它；
5. 建议在「设置 → 通知」配置一种告警方式（邮件 / Webhook / Telegram），DDNS 失败、证书续签失败、登录异常等事件会推送。

## 5. 反向代理与 HTTPS 证书

1. **反向代理页 → 添加规则**：填前端域名（如 `app.example.com`）、后端目标（内网应用地址）、HTTPS 开关与证书；
2. **证书页 → 申请证书**：ACME DNS-01 验证，需先在 DDNS 页配置对应服务商凭证（Cloudflare / DNSPod / 阿里云 / 腾讯云 / 火山引擎），通配符证书 `*.example.com` 可覆盖整组域名；
3. DNS 记录指向本机 IP：家庭宽带场景由 DDNS 自动跟随（见第 6 节）；
4. 验证：浏览器访问域名，证书页看剩余有效期，日志中心看访问/错误日志。

## 6. DDNS（域名解析自动跟随）

DDNS 页 → 添加任务：选服务商 → 填 API 凭证（可「测试连接」）→ 添加解析记录（支持 `@`、子域、完整域名）→ 开 IPv4/IPv6 → 保存。

**IP 来源两种模式**（任务详情 → 「自定义 IP」按钮设置）：

| 模式 | 填法 | 适用 |
| --- | --- | --- |
| 自动检测（默认） | 自定义 IP 留空 | 域名指向家庭宽带出口 IP |
| 自定义 IP | 填固定公网 IP（如 VPS 的 IP） | 域名直接指向公网服务器 |

自定义 IP 必须为公网地址，填内网 IP 会被拒绝更新。VPS 动态 IP 场景：自定义 IP 填 VPS 当前 IP，FRP 服务端地址填 DDNS 域名，VPS 换 IP 后自动跟随。

## 7. FRP 内网穿透（无公网 IP 时）

前置：一台有公网 IP 的 VPS。

1. **Havline 添加服务端**：FRP 穿透页 → 添加服务端 → 填 VPS 地址、bindPort（默认 7000）、认证方式与 Token（与 frps 一致）；
2. **生成 frps 配置**：服务端「详情」→「frps.toml」→ 复制生成内容到 VPS 保存为 `frps.toml`，执行 `frps -c frps.toml`；按文件末尾的防火墙清单在 VPS 安全组放行端口；
3. **添加穿透规则**：8 种类型按需选——tcp/udp 填远程端口；http/https/tcpmux 填自定义域名（走 frps vhost 端口）；stcp/sudp/xtcp 凭 secretKey（无公网监听）；
4. **应用并验证**：「应用配置」→「校验并应用配置」；服务端「详情」看五项诊断（TCP 可达 / frpc 登录 / 代理可达 / DNS 解析 / 归属地）与独立日志。

## 8. 数据保护、升级与回滚

### 8.1 敏感数据保护边界

Havline 的持久化数据不是只有 SQLite。生产环境应把整个 `/data` 数据目录视为高敏感资产，并限制宿主机目录或 Docker volume 的读权限。以下内容不得提交到 Git、写入公开日志或上传到公开工单：

| 数据 | 位置或来源 | 保护要求 |
| --- | --- | --- |
| 管理员、会话、代理、DDNS、FRP、Cloudflare 和健康记录 | `/data/havline.db` | 限制文件权限；不要在服务运行时直接编辑；备份需加密并限制访问 |
| 会话签名与凭据加密主密钥 | `/data/session_secret` 或 `HAVLINE_SESSION_SECRET` | 与数据库一起备份；已有数据后不可随意更换；文件权限应为 `0600` |
| ACME 证书和私钥 | `/data/certs/` | 私钥按密钥材料保护；备份和迁移必须保密 |
| Nginx、FRP、Cloudflare 配置 | `/data/nginx/`、`/data/frp/` 及 Cloudflare 数据目录 | 配置可能包含域名、Token 引用和内部地址，不应公开 |
| API Token、FRP Token、DNS/SMTP/Webhook 凭据 | 数据库加密字段及备份归档 | 依赖会话密钥解密；不要只恢复数据库而遗漏 `session_secret` |
| updater 共享鉴权 Token | 共享卷中的 `/run/havline-updater/updater.token` | 只给主程序和 sidecar；旧部署可用 `HAVLINE_UPDATER_TOKEN` 覆盖 |
| Docker Socket | updater sidecar 的宿主机挂载 | 等同高权限 Docker 控制权；仅允许受信任管理员操作宿主机 |

推荐备份整个数据目录，并在服务停止或应用备份流程完成后复制，避免 SQLite 和配置文件处于不一致状态：

```bash
# 先确认当前 Compose 项目和数据卷名称；不要盲目使用旧卷名
 docker compose -f docker-compose.hub.yml stop havline
 docker volume ls | grep havline
 docker run --rm \
   -v havline_havline-data:/data:ro \
   -v "$PWD":/backup \
   alpine tar czf /backup/havline-data-$(date +%F).tgz -C /data .
 docker compose -f docker-compose.hub.yml start havline
```

如果使用 `HAVLINE_DATA_PATH` 绑定目录，应备份该绑定目录而不是假设命名卷仍存在。备份文件本身应存放在受限目录，最好再进行离线加密；恢复前先停止服务，恢复后检查 `session_secret`、数据库迁移、证书权限和 Nginx/FRP 配置。

### 8.2 updater sidecar 的安全边界与验证

`docker-compose.yml` 和 `docker-compose.hub.yml` 中，主程序只挂载共享 Unix Socket，不挂载 Docker Socket；只有 updater sidecar 挂载 `${DOCKER_SOCK:-/var/run/docker.sock}`。Socket 目录通过 `havline-updater-sock` 共享，文件权限为 `0660`，HTTP 接口还必须携带 `X-Havline-Updater-Token`，Token 由 `.env` 注入两侧。sidecar 缺少 Token 时拒绝启动。

sidecar 的 Compose 项目目录边界如下：

- `/workspace` 只映射 `${HAVLINE_UPDATE_HOST_DIR:-.}`；
- `HAVLINE_UPDATE_PROJECT_DIR` 固定为容器内 `/workspace`；
- Compose 文件名由编排固定为 `docker-compose.yml` 或 `docker-compose.hub.yml`；
- 服务名固定为 `havline`；
- git 模式仓库和分支固定为 `https://jayhub.top/mrlee/Havline.git` / `main`；
- updater 会读取现有容器的 Compose 项目标签，无法确认原项目时拒绝升级；
- 本地模式会拒绝带未提交改动的 Git 工作区；
- helper 模式只接受位于宿主机项目目录内的 Compose 文件，不能通过 `..` 越界。

部署后应执行以下检查：

```bash
# 1. 确认两个容器都在运行，且 updater 挂载了 Socket、主程序没有 Docker Socket
 docker compose -f docker-compose.hub.yml ps
 docker inspect havline-updater --format '{{json .Mounts}}'
 docker inspect havline --format '{{json .Mounts}}'

# 2. 确认 Compose 项目、工作目录和配置文件标签来自同一个部署项目
 docker inspect havline --format '{{ index .Config.Labels "com.docker.compose.project" }}'
 docker inspect havline --format '{{ index .Config.Labels "com.docker.compose.project.working_dir" }}'
 docker inspect havline --format '{{ index .Config.Labels "com.docker.compose.project.config_files" }}'

# 3. 不要把 Token 打印到终端；只确认 Token 文件已生成
 docker compose -f docker-compose.hub.yml exec havline test -s /run/havline-updater/updater.token && echo updater-token-ready

# 4. 从主程序页面检查 updater status；手工请求 Unix Socket 无 Token 应返回 401
 docker compose -f docker-compose.hub.yml logs --tail 100 havline-updater
```

### 8.3 升级、健康检查与回滚事实

升级流程是：读取远端 `VERSION` → 记录当前镜像 → 构建目标镜像 → 只重建 `havline` → 在 `HAVLINE_UPDATE_APP_URL` 上轮询 `/api/version` → 成功才标记完成。当前实现的自动回滚触发条件是“容器重建成功，但新版本在健康检查超时内未返回目标版本”；回滚使用升级前记录的镜像 tag。

以下情况**不会**由当前 updater 自动恢复：

- 构建失败；
- Compose 重建失败；
- updater sidecar 自己退出、Token 错误或 Docker Socket 不可用；
- 升级前已经没有可识别的旧镜像 tag；
- 数据库迁移已经向前执行后，旧版本代码不兼容新结构。

因此升级前必须保留整目录备份和旧镜像 tag：

```bash
# 记录可回滚镜像，不要只记录 VERSION 文本
 docker image inspect "havline:${HAVLINE_VERSION:-local}" --format '{{.RepoTags}}'
 docker images havline

# hub 模式从固定 main 分支构建；先备份，再执行升级
 docker compose -f docker-compose.hub.yml up -d --build
 docker compose -f docker-compose.hub.yml logs --tail 200 havline-updater
 curl -fsS http://127.0.0.1:6893/api/version

# 自动回滚未成功时，使用实际存在的旧 tag 手动重建
 HAVLINE_VERSION=<旧版本> docker compose -f docker-compose.hub.yml up -d --no-deps --force-recreate havline
```

异常重启由两个服务的 `restart: unless-stopped` 覆盖：主程序或 updater 进程退出后 Docker 会尝试重启。但这不等于升级成功；必须同时检查容器状态、日志、`/api/version`、数据库迁移和数据目录内容。


### 升级

```bash
# 1. 备份数据（最重要）
docker run --rm -v havline-data:/data -v ${PWD}:/backup alpine tar czf /backup/havline-data-backup.tar.gz /data
# 2. 解压新版源码包覆盖项目目录（VERSION 与 migrations 会更新）
# 3. 重建并启动（迁移 015–022 会在启动时自动应用）
docker compose -f docker-compose.host.yml up -d --build
# 4. 看启动日志确认迁移与版本
docker logs -f havline
```

### 回滚

```bash
docker compose down
# 还原上一版源码目录后：
docker compose -f docker-compose.host.yml up -d --build
# 数据目录如需还原：解压第 1 步的备份 tar 到挂载卷
```

> 注意：数据库迁移只向前应用（`schema_migrations` 记录版本号）。跨大版本回滚前必须先做第 1 步的数据备份，否则旧版代码遇到新表结构可能启动异常。

## 9. 故障排查速查

| 现象 | 处理 |
| --- | --- |
| 容器起来但 6893 打不开 | `docker logs havline` 看启动报错；`docker port havline`（Bridge 模式）确认映射 |
| updater 启动失败或一直等待 Token | 检查主程序日志，以及共享卷中的 `/run/havline-updater/updater.token` 是否存在；旧部署可用 `HAVLINE_UPDATER_TOKEN` 覆盖 |
| 反代 502 | 目标内网应用未启动或地址填错；日志中心看 Nginx error 日志 |
| 证书申请失败 | DDNS 凭证是否有效、域名 DNS 是否已生效、ACME 邮箱是否已设置 |
| frpc 连不上 frps | 两端 Token/端口不一致；用服务端「详情 → frps.toml」重新生成对齐 |
| 穿透流量显示 0 | frps 未配置 webServer 或 Havline 服务端表单未填管理地址（非必须功能） |
| 页面改动未生效 | 确认访问的是容器端口而非开发服务器；`docker compose up -d --build` 重建 |
| 升级失败后服务未恢复 | 先查看 `docker logs havline-updater`；只有重建成功但新版本健康检查失败时才会自动回滚，构建失败或 updater 自身异常需要人工按镜像 tag 回滚 |

## 10. 卸载

```bash
docker compose down          # 停止并删除容器（数据卷保留）
docker volume rm havline-data   # 彻底删除数据（不可逆，先备份）
docker rmi havline:local        # 删除镜像
```
