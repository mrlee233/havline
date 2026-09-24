# Fonu 部署安装文档

> 适用版本：v1.1.0（源码包 `fonu-1.1.0-src.zip`）。本文覆盖从拿到安装包到服务上线的完整流程：环境要求 → 打包 → 三种部署方式 → 首次配置 → FRP/DDNS/证书联动 → 升级与回滚 → 故障排查。
> 功能细节与组合场景请配合项目内两份文档：`README.md`（功能总览）与 FRP 页面右上「使用说明」（内置十个组合场景详解）。

## 0. 交付物清单与安装路径速查

| 交付物 | 内容 | 适用 | 安装方式 |
| --- | --- | --- | --- |
| `fonu-1.1.0-linux-amd64.tar.gz`（14MB） | 编译好的 Linux 二进制（前端已内嵌）+ migrations + systemd + 一键脚本 | **裸机 Linux 服务器**，目标机只需系统 nginx | 解压 → `sudo bash install.sh`（见 3.4） |
| `fonu-1.1.0-image.tar.gz`（约 100-150MB） | 构建好的 Docker 镜像 `fonu:local`（含 nginx + frpc） | 目标机有 Docker、要离线安装 | `docker load` → `docker run`（见 3.3 方式 C） |
| `fonu-1.1.0-src.zip`（11MB） | 完整源码 + Dockerfile/compose + 部署文档 | 要自行构建 / 二次开发 / 交叉编译 | `docker compose build` 后按方式 A/B 部署（见第 2 节） |

**怎么选**：飞牛 fnOS 首选镜像包导入（图形界面，见第 3 节末「飞牛 fnOS 部署详解」），要在 NAS 上构建则用源码包 + host 模式 compose；普通 Linux 服务器要省事用 Linux 运行包（免 Docker）；目标机不方便联网构建用镜像包离线导入。三者的功能完全一致——容器里跑的也是同一份二进制 + 系统 nginx。

## 1. 包内容与环境要求

### 1.1 源码包内容

`fonu-1.1.0-src.zip`（约 11MB，304 项）为完整可构建源码：

| 目录/文件 | 说明 |
| --- | --- |
| `cmd/` `internal/` | Go 后端源码（含已迁移的多服务端 FRP 模块） |
| `web/` | Vue 3 + TypeScript 前端源码 |
| `migrations/` | SQLite 数据库迁移 001–022（启动时自动按序应用） |
| `Dockerfile` / `docker-compose*.yml` | 本地构建镜像与三种部署编排 |
| `VERSION` / `CHANGELOG.md` | 版本号单一来源（当前 `1.1.0`）与版本记录 |
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

一次构建同时产出：前端 `dist`、Go 后端二进制与运行镜像（frpc 不预装，运行期用「应用配置」按需下载），标签为 `fonu:local`。

**版本号**：读取根目录 `VERSION` 文件（当前 `1.1.0`）自动注入前后端显示；要改版本号只需编辑该文件后重新 build。

**网络受限环境**可覆盖基础镜像与 frpc 下载源（默认值见 `Dockerfile` 顶部注释）：

```bash
docker build \
  --build-arg NODE_IMAGE=docker.m.daocloud.io/library/node:22-alpine \
  --build-arg GO_IMAGE=docker.m.daocloud.io/library/golang:1.23-bookworm \
  --build-arg RUNTIME_IMAGE=docker.m.daocloud.io/library/debian:bookworm-slim \
  -t fonu:local .
```

## 3. 三种部署方式（按环境二选一）

### 方式 A：飞牛 fnOS / 已占用 80/443 的环境 —— host 模式（推荐）

飞牛系统自带 Nginx 占用 80/443，Fonu 使用 host 网络 + 高位反代端口避免冲突。

```bash
FONU_SESSION_SECRET=请改成一串随机字符 docker compose -f docker-compose.host.yml up -d --build
```

- 无端口映射，容器直接使用宿主机网络栈；
- 反代端口由环境变量 `FONU_NGINX_HTTP_PORT=18080`、`FONU_NGINX_HTTPS_PORT=9443` 决定（文件内已内置）；
- 入口：管理后台 `http://<NAS-IP>:6893`，HTTP 反代 `http://<NAS-IP>:18080`，HTTPS 反代 `https://<NAS-IP>:9443`；
- 数据持久化在命名卷 `fonu-data`（挂载到容器 `/data`）。

### 方式 B：其他环境 / 空闲 80/443 —— Bridge 端口映射

```bash
FONU_SESSION_SECRET=请改成一串随机字符 docker compose -f docker-compose.hub.yml up -d --build
```

- 端口映射 `6893:6893`、`18080:80`、`9443:443`（宿主机 18080/9443 → 容器 80/443）；
- 数据同样在命名卷 `fonu-data`。

### 方式 C：docker run（不想用 compose）

```bash
docker run -d --name fonu \
  --net=host \
  -v ./data:/data \
  -e FONU_SESSION_SECRET=请改成一串随机字符 \
  --restart unless-stopped \
  fonu:local
```

host 模式下反代端口用环境变量控制：`-e FONU_NGINX_HTTP_PORT=18080 -e FONU_NGINX_HTTPS_PORT=9443`。Bridge 环境改为 `-p 6893:6893 -p 18080:80 -p 9443:443`（去掉 `--net=host`）。

**生成随机 SESSION_SECRET**：`openssl rand -hex 32`（Linux）或 PowerShell：`-join ((48..57)+(97..122) | Get-Random -Count 32 | % {[char]$_})`。

> **`FONU_SESSION_SECRET` 是什么**：Fonu 的主密钥，两个用途——① 签名/加密登录会话 Cookie（决定登录态是否可被伪造）；② 作为 `secret.Box` 加密存储中敏感凭据（DDNS API Token、FRP Token、SMTP 密码等）。默认值 `change-me-in-production` 是公开的：拿到它即可伪造管理员会话、解密数据库里的全部凭据，**生产环境必须替换**。⚠️ 换密钥的时机：首次启动前设置；若之后更换，旧密钥加密的凭据将无法解密（应用会提示重新保存），需到对应页面重录 Token。

### Linux 服务器完整部署步骤（Ubuntu / Debian / CentOS）

```bash
# 0. 安装 Docker（已装跳过）
curl -fsSL https://get.docker.com | sh
sudo systemctl enable --now docker

# 1. 解压源码包并进入目录
unzip fonu-1.1.0-src.zip -d fonu && cd fonu

# 2. 构建镜像（约 3-10 分钟，取决于网络与机器性能）
docker compose build

# 3. 生成密钥并启动（Bridge 模式，映射 6893/18080/9443）
echo "FONU_SESSION_SECRET=$(openssl rand -hex 32)" > .env
docker compose -f docker-compose.hub.yml up -d

# 4. 防火墙放行（按需）
sudo ufw allow 6893/tcp     # 管理后台
sudo ufw allow 80/tcp 443/tcp   # 反代（Bridge 模式容器内监听 80/443）
# firewalld 用: firewall-cmd --permanent --add-port=6893/tcp && firewall-cmd --reload

# 5. 验证
docker logs fonu | grep -i password   # 取 admin 初始密码
curl -I http://127.0.0.1:6893/login   # 应返回 200
```

- **开机自启**：compose 内已含 `restart: unless-stopped`，且第 0 步已 `enable docker`，重启服务器自动拉起；
- **80/443 被系统 Nginx 占用**：改用 `docker-compose.host.yml`（host 网络 + 18080/9443 高位端口），或改 `docker-compose.hub.yml` 里的宿主端口映射；
- **离线部署**（目标机不方便联网构建）：在有网的机器上 `docker compose build` 后 `docker save fonu:local | gzip > fonu.tar.gz`，拷贝到目标机 `docker load < fonu.tar.gz`，再用方式 C 的 `docker run` 启动（无需再构建）；
- **数据位置**：命名卷 `fonu-data`（`docker volume inspect fonu-data` 查看宿主机实际路径），备份该卷即备份全部配置与证书。

### Linux 运行包安装（免 Docker，裸机部署）

适用：不想装 Docker 的 Linux 服务器。前提：系统已装 nginx（Fonu 复用它做反代），root 权限。

```bash
# 1. 解压
tar xzf fonu-1.1.0-linux-amd64.tar.gz -C fonu && cd fonu

# 2. 一键安装（自动建 fonu 系统用户、装到 /opt/fonu、生成随机密钥、注册 systemd 开机自启）
sudo bash install.sh

# 3. 验证
systemctl status fonu
curl -I http://127.0.0.1:6893/login
journalctl -u fonu | grep -i password    # 取 admin 初始密码
```

包内容：`fonu` 二进制（前端已 `go:embed` 内嵌，无需单独部署 dist）、`migrations/`（从二进制同级目录自动查找）、`deploy/nginx/mime.types`、`fonu.service`（systemd 模板）、`install.sh`、`DEPLOYMENT.md`。

关键路径（install.sh 自动配置）：

| 项 | 值 |
| --- | --- |
| 二进制 | `/opt/fonu/fonu` |
| 数据目录 | `/var/lib/fonu`（数据库、证书、日志全在这） |
| 会话密钥 | `/etc/default/fonu`（重复安装保留原值） |
| 服务 | `systemctl restart fonu` 生效配置改动 |

**备份/迁移**：停服务后打包 `/var/lib/fonu` 即可（`tar czf fonu-data.tar.gz /var/lib/fonu`），新机器解压到同路径 + 装同版本二进制即完成迁移。

**手动安装**（不用脚本）：按包内 `README.txt` 要点——建 `fonu` 系统用户、复制二进制与 migrations 到 `/opt/fonu`、`EnvironmentFile` 写入随机密钥、`systemctl enable --now fonu`。

> 镜像包（`fonu-1.1.0-image.tar.gz`）来源：在任意装了 Docker 的机器上对源码包执行 `docker compose build` 后 `docker save fonu:local | gzip > fonu-1.1.0-image.tar.gz`；目标机 `docker load < fonu-1.1.0-image.tar.gz` 后按方式 C 启动。

### 飞牛 fnOS 部署详解（图形界面，不敲命令）

前置：fnOS 应用中心安装 Docker 应用（较新版本自带）。以下菜单名以 fnOS 当前版本实际界面为准，大致路径一致。

**方式一：导入现成镜像（推荐，5 分钟）**

1. **上传文件**：把 `fonu-1.1.0-image.tar.gz` 通过文件管理 / SMB 上传到 NAS，如共享目录 `docker/fonu/`；
2. **导入镜像**：打开 Docker 应用 → 镜像 → 导入（从文件加载）→ 选择该 tar.gz → 等待镜像列表出现 `fonu:local`（若界面只接受 .tar，先在电脑上解压出内层的 .tar 再导入）；
3. **创建容器**：容器 → 创建：

| 配置项 | 填写 |
| --- | --- |
| 名称 | `fonu` |
| 镜像 | `fonu:local` |
| 网络 | **host**（飞牛 80/443 被系统占用，host + 高位端口最省事；UI 不支持 host 时选 Bridge 并映射 6893/18080/9443） |
| 存储挂载 | NAS 建目录（如 `/vol1/…/docker/fonu/data`）→ 映射到容器 `/data`（读写） |
| 环境变量 | `FONU_SESSION_SECRET=随机串`、`FONU_DATA_DIR=/data`、`FONU_LISTEN=:6893`、`FONU_NGINX_HTTP_PORT=18080`、`FONU_NGINX_HTTPS_PORT=9443` |
| 重启策略 | 自动重启 / unless-stopped（开机自启） |

4. **启动并取初始密码**：容器详情 → 日志，搜 `password`；浏览器打开 `http://<NAS-IP>:6893` 登录。

**方式二：Compose 项目（用源码包，在 NAS 上构建）**

1. 把源码包 zip 上传到 NAS 并解压（文件管理支持解压，或电脑解压后整目录上传）；
2. Docker 应用 → 项目（Compose）→ 新建：指定项目目录为解压后的源码目录、使用 `docker-compose.host.yml`（或直接把该文件内容粘贴到 Compose 编辑器）；
3. 在源码目录同层放一个 `.env` 文件，内容一行：`FONU_SESSION_SECRET=随机串`；
4. 构建并启动。⚠️ NAS 上构建耗时较长（前端编译），低配机器建议改用方式一；
5. 后续升级：上传新源码覆盖 → 项目重新构建即可，数据在卷里不受影响。

**Compose 构建报 401（docker.fnnas.com）**：fnOS 的 Docker 设置里默认配了自家加速地址，但它对 `library/` 官方镜像拒绝匿名拉取。三种解法任选：

1. **改用镜像包（最省事）**：跳过构建，按方式一导入 `fonu-1.1.0-image.tar.gz`；
2. **覆盖基础镜像源**：在源码目录的 `.env` 里加三行（已有 FONU_SESSION_SECRET 就追加在下面）：
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

1. 启动后查看初始密码：`docker logs fonu | grep -i password`（自动生成的 admin 初始密码会打印在日志里）；
2. 浏览器打开 `http://<NAS-IP>:6893/login`，用 `admin` + 初始密码登录；
3. **立即**在「设置 → 安全」修改管理员密码；
4. 确认「设置 → 常规」中 `FONU_SESSION_SECRET` 已改为非默认值（compose 文件中已传入则跳过）；
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

1. **Fonu 添加服务端**：FRP 穿透页 → 添加服务端 → 填 VPS 地址、bindPort（默认 7000）、认证方式与 Token（与 frps 一致）；
2. **生成 frps 配置**：服务端「详情」→「frps.toml」→ 复制生成内容到 VPS 保存为 `frps.toml`，执行 `frps -c frps.toml`；按文件末尾的防火墙清单在 VPS 安全组放行端口；
3. **添加穿透规则**：8 种类型按需选——tcp/udp 填远程端口；http/https/tcpmux 填自定义域名（走 frps vhost 端口）；stcp/sudp/xtcp 凭 secretKey（无公网监听）；
4. **应用并验证**：「应用配置」→「校验并应用配置」；服务端「详情」看五项诊断（TCP 可达 / frpc 登录 / 代理可达 / DNS 解析 / 归属地）与独立日志。

## 8. 升级与回滚

### 升级

```bash
# 1. 备份数据（最重要）
docker run --rm -v fonu-data:/data -v ${PWD}:/backup alpine tar czf /backup/fonu-data-backup.tar.gz /data
# 2. 解压新版源码包覆盖项目目录（VERSION 与 migrations 会更新）
# 3. 重建并启动（迁移 015–022 会在启动时自动应用）
docker compose -f docker-compose.host.yml up -d --build
# 4. 看启动日志确认迁移与版本
docker logs -f fonu
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
| 容器起来但 6893 打不开 | `docker logs fonu` 看启动报错；`docker port fonu`（Bridge 模式）确认映射 |
| 提示「会话加密密钥」警告 | 未设置 `FONU_SESSION_SECRET`，compose 传入即可 |
| 反代 502 | 目标内网应用未启动或地址填错；日志中心看 Nginx error 日志 |
| 证书申请失败 | DDNS 凭证是否有效、域名 DNS 是否已生效、ACME 邮箱是否已设置 |
| frpc 连不上 frps | 两端 Token/端口不一致；用服务端「详情 → frps.toml」重新生成对齐 |
| 穿透流量显示 0 | frps 未配置 webServer 或 Fonu 服务端表单未填管理地址（非必须功能） |
| 页面改动未生效 | 确认访问的是容器端口而非开发服务器；`docker compose up -d --build` 重建 |

## 10. 卸载

```bash
docker compose down          # 停止并删除容器（数据卷保留）
docker volume rm fonu-data   # 彻底删除数据（不可逆，先备份）
docker rmi fonu:local        # 删除镜像
```
