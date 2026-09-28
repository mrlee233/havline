# Fonu Web 一键升级方案

> 实施版本：v1.3.0  
> 适用范围：`docker-compose.yml`、`docker-compose.hub.yml`  
> 不适用范围：`docker-compose.host.yml`、原生 systemd 安装、Windows 直接运行

## 1. 设计目标

在管理后台检测到新版本后，用户点击「一键升级」即可完成：

1. 读取远端最新版本；
2. 构建新的 `fonu` 镜像；
3. 只重建 `fonu` 容器；
4. 等待 `/api/version` 返回目标版本；
5. 失败时尝试回滚到升级前的镜像 tag；
6. 把升级过程和结果返回给前端。

## 2. 安全边界

主程序容器**不挂载** `/var/run/docker.sock`。

只有 `fonu-updater` sidecar 持有：

- Docker Socket；
- Docker CLI 与 Compose；
- 项目目录；
- Git。

主程序与 sidecar 通过一个共享 Unix Socket 通信：

```text
浏览器
  → fonu 后端 /api/system/update/*
  → /run/fonu-updater/updater.sock
  → fonu-updater
  → docker compose build/up
```

这表示：

- 管理后台被攻破时，攻击者仍需通过 sidecar 的固定接口才能触发升级；
- sidecar 只接受固定的 Compose 文件名和固定的 `fonu` 服务名；
- 不提供任意命令执行接口；
- 不支持自定义 shell 命令。

## 3. 两种升级模式

| Compose 文件 | 模式 | 行为 |
| --- | --- | --- |
| `docker-compose.yml` | `local` | 在项目目录检查 Git 工作区，干净时执行 `git pull --ff-only`，再构建 |
| `docker-compose.hub.yml` | `git` | Compose 的 build context 直接从 Git 仓库拉取，updater 只负责构建与重建 |

两种模式都会：

- 读取目标版本；
- 构建 `fonu` 镜像；
- 执行 `docker compose up -d --no-deps --force-recreate fonu`；
- 轮询 `/api/version` 验证升级结果。

## 4. 首次部署

一键升级依赖 `fonu-updater` 已经运行。

旧版本升级到 v1.3.0 时，需要先手动执行一次：

```bash
# 本地源码部署
docker compose up -d --build

# 单文件 Git 构建部署
docker compose -f docker-compose.hub.yml up -d --build
```

完成这一轮后，后续版本即可在 Web 页面一键升级。

## 5. 环境变量

### 主程序

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `FONU_UPDATER_SOCKET` | `/run/fonu-updater/updater.sock` | 与 sidecar 通信的 Unix Socket |

### updater sidecar

| 变量 | 说明 |
| --- | --- |
| `FONU_UPDATER_SOCKET` | sidecar 监听的 Unix Socket |
| `FONU_UPDATE_PROJECT_DIR` | Compose 项目目录，容器内为 `/workspace` |
| `FONU_UPDATE_COMPOSE_FILE` | 只允许 `docker-compose.yml`、`docker-compose.hub.yml`、`docker-compose.host.yml` |
| `FONU_UPDATE_SERVICE` | 要重建的服务，默认 `fonu` |
| `FONU_UPDATE_MODE` | `local` 或 `git` |
| `FONU_UPDATE_REPO` | `git` 模式下的仓库地址 |
| `FONU_UPDATE_BRANCH` | `git` 模式下的分支，默认 `main` |
| `FONU_UPDATE_APP_URL` | 健康检查地址，默认 `http://fonu:6893` |
| `DOCKER_SOCK` | 宿主机 Docker Socket，默认 `/var/run/docker.sock` |
| `FONU_UPDATE_HOST_DIR` | 宿主机上 Compose 文件所在目录；留空时自动从 Compose 容器标签发现，设置后作为显式覆盖 |

## 6. 前端行为

侧栏版本区在满足以下条件时显示「一键升级」：

- sidecar 可用；
- 远端版本高于当前版本；
- 当前没有正在执行的升级任务。

点击后弹窗显示：

- 当前版本；
- 目标版本；
- 升级方式；
- 当前阶段；
- 最近日志。

升级开始后，页面会持续轮询升级状态。容器重建期间请求会短暂失败，前端会显示「服务正在重启，等待恢复」，恢复成功后自动刷新页面。

## 7. 回滚

升级前会记录当前容器镜像。升级后健康检查超时，sidecar 会尝试：

```bash
FONU_VERSION=<上一个版本> docker compose up -d --no-deps --force-recreate fonu
```

回滚是否成功会写入升级日志。

## 8. 常见问题

### 侧栏没有「一键升级」

检查：

1. 当前是否使用 `docker-compose.yml` 或 `docker-compose.hub.yml`；
2. `fonu-updater` 容器是否在运行；
3. `FONU_UPDATER_SOCKET` 是否在两侧指向同一路径；
4. 共享卷 `fonu-updater-sock` 是否存在。

如果状态接口返回：

```text
读取远端版本失败：git clone ... signal: killed ... fatal: early EOF
```

说明运行中的 updater 仍是旧版本，仍在用完整克隆读取 `VERSION`。升级到 v1.3.2 后，updater 会改用 Gitea API / raw 地址读取版本文件。由于 sidecar 不能升级自身，需要先手动重建一次：

```bash
docker compose -f docker-compose.hub.yml up -d --build updater

# 本地源码部署
docker compose up -d --build updater
```

### `docker-compose.yml` 模式提示存在未提交改动

升级前的 Git 工作区检查要求干净，避免覆盖用户修改。先处理工作区，或手动执行：

```bash
docker compose up -d --build
```

### 升级时报 `/workspace/docker-compose.hub.yml: no such file or directory`

v1.3.6 起 updater 会自动从 `fonu` 容器的 Compose 标签读取宿主机目录，并通过 helper 容器执行 Compose。如果自动发现仍失败，可以显式设置目录：

先查询运行中 `fonu` 容器的 Compose 工作目录：

```bash
docker inspect fonu --format '{{ index .Config.Labels "com.docker.compose.project.working_dir" }}'
```

把输出目录写入 Compose 同层 `.env` 作为覆盖：

```dotenv
FONU_UPDATE_HOST_DIR=/实际/的/Compose/目录
```

然后只重建 updater：

```bash
docker compose -f /实际/的/Compose/目录/docker-compose.hub.yml up -d --build updater
```

状态接口会明确提示 Compose 文件缺失，修复挂载或设置覆盖后按钮才会重新显示。

### 报 `container name "/fonu" is already in use`

这表示 Compose 没有识别到当前容器所属的原项目，可能把升级当成新项目并尝试新建网络与数据卷。v1.3.7 的更新版 updater 会读取现有容器的 `com.docker.compose.project` 标签，并显式复用原项目名。

如果现有 `fonu` 容器不是由 Compose 管理，updater 会拒绝升级，避免新建 `fonu_fonu-data` 等空数据卷。应先按 Compose 方式重新部署，再使用一键升级。

### helper 命令里出现 `-v /workspace:/workspace`

说明 Compose 标签只返回了容器内路径。更新后的 updater 会优先从 `project.config_files` 标签推导宿主机目录，并忽略 `/workspace` 这类容器路径。若标签仍无法提供真实宿主机路径，应检查部署工具的 Compose 标签是否完整；正常情况下无需手工设置目录。

### `docker-compose.hub.yml` 模式无法读取远端版本

检查 sidecar 容器内是否可以访问 Git 仓库：

```bash
docker exec -it fonu-updater sh -lc 'git ls-remote http://45.207.223.190:10082/mrlee/-Fonu.git main'
```

### 升级卡在 `running`

查看 sidecar 日志：

```bash
docker logs -f fonu-updater
```

以及 Compose 输出：

```bash
docker compose ps
docker compose logs --tail 100 fonu
```

### 回滚失败

确认升级前的镜像 tag 仍然存在：

```bash
docker images fonu
```

然后用明确的版本 tag 手动切回：

```bash
FONU_VERSION=<旧版本> docker compose up -d --no-deps --force-recreate fonu
```

## 9. 暂不支持

- `docker-compose.host.yml`：需要确认 host 网络与端口占用后再设计；
- 原生 systemd 安装：需要单独的受控升级 helper；
- 回滚到远端不存在的镜像：需要额外发布流程；
- 升级 `fonu-updater` 自身：当前只重建 `fonu` 服务，sidecar 更新仍需一次手动 Compose 操作。

## 10. 在线升级验证

示例页面用于验证升级结果。v1.6.8 的验证流程：

1. 确认当前线上实例为 v1.3.0，且 `fonu-updater` sidecar 已运行。
2. 确认远端 `main` 分支的 `VERSION` 为 `1.6.8`。
3. 在侧栏看到「一键升级」后点击并确认。
4. 等待升级弹窗显示成功，页面自动刷新。
5. 打开侧栏「示例页面」，确认：
   - 页面路由为 `/example`；
   - 页面显示的版本为 `1.6.8`；
   - 页面显示验证标识 `v1.6.8-example-page`；
   - 页面显示独立的「本次升级验证」区块；
   - 「升级验证」显示为「通过」。

如果示例页面可见且版本正确，说明镜像构建、容器重建、后端版本注入和前端静态资源更新均已生效。
