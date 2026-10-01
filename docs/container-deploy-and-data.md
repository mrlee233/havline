# Havline 容器部署与数据指南

> 适用版本：v1.2.19 及以后（Docker / 飞牛 fnOS 等 NAS 环境）
> 相关文件：`docker-compose.yml`、`docker-compose.hub.yml`、`docker-compose.host.yml`、`Dockerfile`、`scripts/upgrade.sh`

---

## 一、三个编排文件怎么选

| 文件 | 构建方式 | 适用场景 |
| --- | --- | --- |
| `docker-compose.hub.yml` | **从 Git 仓库拉取源码**构建（`build.context` 是仓库地址） | **干净机器首选**：只要这一个文件，不需要先 `git clone` |
| `docker-compose.yml` | 用**本地源码**构建（`context: .`） | 已经有源码目录（改代码后直接重建最方便） |
| `docker-compose.host.yml` | 本地源码构建 + `network_mode: host` | 80/443 已被别的服务占用（如飞牛、群晖自带反代） |

三份都写了 `name: havline`：**不要删掉这行**。不写它，改了目录名/项目名后 compose 会把它当成新项目，`havline-data` 变成新的空卷——现象就是「数据全没了」。

---

## 二、干净机器部署（推荐路径）

```bash
mkdir -p /opt/havline && cd /opt/havline
# 从仓库取这一份编排文件（用你仓库页面上的「原始文件」链接，或 git clone 后复制）
#   curl -fsSLo docker-compose.hub.yml <仓库里该文件的原始地址>
docker compose up -d --build
docker compose logs -f havline      # 首次启动会初始化数据库、生成会话密钥
```

首次访问 `http://<机器IP>:6893`，用日志里的初始口令登录。初始口令只在**首次启动**生成一次：

```bash
docker compose logs havline 2>&1 | grep -i password | tail -n 3
```

80/443 被占用的环境改用 host 那份：

```bash
COMPOSE_FILE=docker-compose.host.yml docker compose up -d --build
# 反代端口默认 18080/9443（可用 HAVLINE_NGINX_HTTP_PORT / HAVLINE_NGINX_HTTPS_PORT 覆盖）
```

---

## 三、数据存在哪

数据**不在镜像里**，全部在数据目录中（容器内固定为 `/data`，由 `HAVLINE_DATA_DIR` 指定）：

| 路径 | 内容 |
| --- | --- |
| `havline.db` | SQLite 主库：反代规则、穿透规则与服务端、DDNS、证书记录、设置、API Token、健康事件、指标历史 |
| `session_secret` | 会话与**凭据加密密钥**（v1.2.17 起未配置时自动生成，0600） |
| `logs/` | `app.log`（应用日志）、`access.log` / `error.log`（本机 Nginx） |
| `certs/` | 证书与私钥 |
| `nginx/` | Havline 生成的 Nginx 配置与 pid |
| `frp/` | frpc 二进制、各服务端的 `frpc-server-N.toml` 与日志 |
| `ssh_known_hosts` | SSH 主机密钥 TOFU 记录 |
| `backups/` | 应用内「数据备份」导出的归档 |

### 3.1 默认：命名卷

默认挂在命名卷 `havline-data`（compose 项目名是 `havline`，所以完整卷名是 `havline_havline-data`），由 Docker 管理，位置随 Docker 数据根。

```bash
docker volume ls | grep havline
docker volume inspect havline_havline-data --format '{{.Mountpoint}}'
docker compose exec havline ls -l /data        # 容器内视角，最直观
```

飞牛这类 NAS 的 Docker 数据根常在 `/vol1/docker`，于是实际落在
`/vol1/docker/volumes/havline_havline-data/_data`。**以 `docker volume inspect` 为准**，各家不同。

### 3.2 指定宿主机目录（bind mount）

在同层 `.env` 里写一行即可，以 `/` 或 `.` 开头的路径会被 compose 当成 bind mount：

```bash
echo 'HAVLINE_DATA_PATH=/vol1/docker/havline-data' >> .env
```

从**已有命名卷**迁过去（迁移前先停服务）：

```bash
docker compose down
sudo mkdir -p /vol1/docker/havline-data
docker run --rm -v havline_havline-data:/from -v /vol1/docker/havline-data:/to alpine sh -c 'cp -av /from/. /to/'
docker compose up -d
docker compose exec havline ls -l /data          # 确认数据还在
```

反过来（bind → 卷）同理，把两边路径对调即可。**不要**只做一边的拷贝就改配置，否则会看到「空实例」。

---

## 四、备份与恢复

**整目录备份**（推荐，含密钥与证书）：

```bash
docker run --rm -v havline_havline-data:/data -v "$PWD":/backup alpine \
  tar czf /backup/havline-data-$(date +%F).tgz -C /data .
```

三个必须知道的点：

1. 千万别只拷 `havline.db`：`session_secret` 是凭据的加密密钥，缺了它库里存的 frp token、通知密码都解不开；
2. 不要在服务运行时手改 `havline.db`（SQLite 单连接，容易被写坏）——要改先 `docker compose stop havline`；
3. 应用内「设置 → 数据」也有备份/恢复按钮，归档落在 `backups/`，恢复后需要重启容器。

---

## 五、升级与回滚

### 5.1 一条命令（推荐）

仓库自带 `scripts/upgrade.sh`（在部署机上执行）：

```bash
sudo bash scripts/upgrade.sh                     # 拉代码 → 构建 → 重建 → 健康检查
sudo bash scripts/upgrade.sh --no-pull           # 用当前源码重建
sudo bash scripts/upgrade.sh --rollback 1.2.18   # 回滚到本地已有的镜像 tag（不重建）
COMPOSE_FILE=docker-compose.host.yml sudo bash scripts/upgrade.sh   # host 网络那份
```

脚本会打印升级前后的镜像 tag，并轮询 `http://127.0.0.1:6893/api/version` 确认起来。

### 5.2 手动

```bash
cd /opt/havline && docker compose pull 2>/dev/null; docker compose up -d --build
docker images havline                    # 看历史上留下哪些 tag，回滚时用
```

镜像 tag 由 `HAVLINE_VERSION` 决定（`scripts/upgrade.sh` 会从仓库 `VERSION` 自动取），不设时是 `havline:local`。

镜像是本地构建的，所以升级不需要 `docker pull`；上面那条 `up` 会带上 `--build` 重新构建。

### 5.3 只有编排文件时（`docker-compose.hub.yml` 部署，推荐路径）

构建上下文是 Git 地址，所以**每次 `--build` 都会重新解析仓库分支**（日志里那句 `load git source ... refs/heads/main` 就是它），不需要 `git pull`：

```bash
cd /opt/havline
docker compose up -d --build        # 重新克隆 main → 构建 → 按新镜像重建容器
```

建议**带上版本号**，给镜像留一个固定 tag（否则一直是 `havline:local`，回滚时找不到旧版）：

```bash
HAVLINE_VERSION=1.2.19 docker compose up -d --build
# 回滚：镜像还在本地，不用重新构建
HAVLINE_VERSION=1.2.18 docker compose up -d
```

版本号从仓库的 `CHANGELOG.md` 或 `VERSION` 文件取；面板左侧栏底部也会显示当前运行的版本。

这套部署方式下，`scripts/upgrade.sh` 不太适用（它需要本地源码目录来 `git pull` 与读 `VERSION`）——如果你希望也能一条命令搞定，可以把这两个文件（`scripts/upgrade.sh` + `VERSION`）也放到部署目录，或告诉我，我给 hub 变体加一个专用脚本。

### 5.4 升级前后的两件小事

- **升级前备份**（整目录，含密钥）：见 §四那条 `tar` 命令；遇到异常可整个目录回滚。
- **升级后确认**：`docker compose logs -f havline` 看启动是否正常（会打印「会话密钥已就绪」），再刷新面板看版本与功能。

---

## 六、环境变量速查

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `HAVLINE_DATA_DIR` | `/data` | 容器内数据目录，一般不用改；要换位置改**挂载** |
| `HAVLINE_DATA_PATH` | `havline-data` | **宿主机侧**数据位置：命名卷名，或绝对路径（bind mount） |
| `HAVLINE_LISTEN` | `:6893` | 面板监听地址 |
| `HAVLINE_SESSION_SECRET` | 空 | 留空即自动生成并持久化；**它是凭据加密密钥，有数据后不要更换** |
| `HAVLINE_NGINX_HTTP_PORT` / `HAVLINE_NGINX_HTTPS_PORT` | `80` / `443` | 反代监听端口（host 网络那份默认 18080/9443） |
| `HAVLINE_UPDATER_SOCKET` | `/run/havline-updater/updater.sock` | 一键升级 sidecar 的 Unix Socket |
| `DOCKER_SOCK` | `/var/run/docker.sock` | 宿主机 Docker Socket；rootless Docker 或自定义路径时覆盖 |
| `TZ` | `Asia/Shanghai` | 通知与日志显示用的时区 |
| `NODE_IMAGE` / `GO_IMAGE` / `RUNTIME_IMAGE` / `UPDATER_IMAGE` | 国内镜像源 | 构建用的基础镜像，被加速器拦截时用来覆盖 |

---

## 七、常见问题

**构建报 `401 Unauthorized`（`resolve image config for docker-image://docker/dockerfile:1`）**
国内加速器（如飞牛的 `docker.fnnas.com`）会拦下 BuildKit 的 frontend 镜像。Dockerfile 已不再显式指定 `# syntax=`，正常不会再遇到；若仍报错，改用加速器能拉的地址：`# syntax=docker.m.daocloud.io/docker/dockerfile:1`，或去掉四处 `--mount=type=cache`。

**`error while interpolating ... HAVLINE_SESSION_SECRET is missing a value`**
这是旧版 compose（把该变量写成必填）留下的行为。新版本是 `${HAVLINE_SESSION_SECRET:-}`：留空即由应用自动生成。重新取一份编排文件即可。

**构建时 `havline Pulling` 之后超时告警**
有 `build:` 段时 compose 仍会先尝试拉同名镜像；`.hub.yml` 已加 `pull_policy: build` 跳过。其余两份可忽略这条警告。

**换目录后「数据没了」**
几乎没有真的丢：要么新目录是空的（迁移没做），要么项目名变了导致卷名变了。先用 `docker volume ls` 找到旧卷，按 §3.2 拷过去。

**面板能开但「今日请求/实时流量/性能」没数据**
这三张卡读的是**本机 Nginx 访问日志**（`<数据目录>/logs/access.log`），只有在 Havline 里配了本机反代规则并产生访问后才有数据；VPS 侧的流量在「公网反代」页与「日志中心 → 公网 Nginx」。

---

## 八、与原生安装的分工

不想用容器时可以用 `scripts/build-linux.ps1` 打出 `havline-<版本>-linux-<arch>.tar.gz`，在目标机用 `sudo bash install.sh` 安装（装到 `/opt/havline`，数据在 `/var/lib/havline`，systemd 单元 `havline.service`）。两者的数据目录结构完全一致，因此**可以互相迁移**：把一方的数据目录内容整个拷到另一方即可，注意带上 `session_secret`。
