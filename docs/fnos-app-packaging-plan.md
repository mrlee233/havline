# Havline 飞牛 fnOS 应用打包方案

> 日期：2026-10-01
> 依据：飞牛应用开放平台文档：<https://developer.fnnas.com/docs/guide/>
> 目标：将 Havline 从 Docker Compose / 裸机部署，改造为可安装、可升级、可卸载的飞牛 fnOS `.fpk` 应用。

---

## 一、官方文档结论

飞牛 fnOS 应用包的核心组成：

```text
manifest
config/privilege
config/resource
app/
cmd/
wizard/
ICON.PNG
ICON_256.PNG
```

官方提供三种常见形态：

1. **CGI 应用**
   - 适合静态页面和轻量入口。
   - 不支持 WebSocket，不适合 Havline 这种常驻服务。
2. **统一网关应用**
   - 复用 fnOS 访问域名和登录态。
   - 服务监听 Unix Socket，由 fnOS 网关转发。
   - 支持 WebSocket，但 Havline 需要新增 Unix Socket 监听能力。
3. **Docker 应用**
   - 使用 `app/docker/docker-compose.yaml`。
   - 在 `config/resource` 中声明 `docker-project`。
   - 最适合 Havline 当前形态。

**首选方案：飞牛 Docker 应用模板。**

原因：

- Havline 已经是完整 Docker 应用。
- 镜像内已包含 Nginx、Go 主程序和数据库运行环境。
- Cloudflare Tunnel、FRP、Agent、证书管理都依赖常驻进程。
- fnOS Docker 项目资源可以托管启动、停止和运行状态。
- 避免把 Havline 的 Go、Nginx、SQLite 和 shell 依赖重新拆进原生应用生命周期。

---

## 二、当前 Havline 的部署形态

### 2.1 当前主要组成

```text
havline
├── Go 主程序
├── 内嵌 Web 前端
├── 内置 Nginx
├── SQLite
├── FRP 客户端 / Agent 管理
├── Cloudflare Tunnel / cloudflared
├── ACME / DDNS / 通知
└── 可选 havline-updater sidecar
```

### 2.2 当前关键目录

```text
/data
├── havline.db
├── session_secret
├── nginx/
├── certs/
├── logs/
├── frp/
└── cloudflared/
```

### 2.3 当前端口

| 端口 | 用途 |
| --- | --- |
| `6893` | Havline 管理界面/API |
| `80` | 本机 Nginx HTTP |
| `443` | 本机 Nginx HTTPS |

飞牛 fnOS 通常已占用 `80/443`，因此 fnOS 包应默认使用：

```text
管理端口：6893
Nginx HTTP：18080
Nginx HTTPS：9443
```

端口应允许安装向导修改。

### 2.4 当前 updater 的限制

Havline 当前的一键升级依赖：

- Docker Socket
- updater sidecar
- 宿主机 Compose 项目目录
- 对宿主 Docker 执行 `compose build/up` 的权限

在 fnOS 应用包内继续保留这条链路会带来：

- 容器内 path mapping 复杂；
- fnOS 应用目录路径和宿主路径不一致；
- 应用中心升级与 Havline 内部升级冲突；
- 需要给应用额外 Docker Socket 权限。

**方案：fnOS 首发版本默认关闭 Havline 内置 updater，统一由飞牛应用中心升级。**

后续如果需要，可做“高级模式”单独启用。

---

## 三、目标包结构

建议在仓库新增：

```text
fnos/
├── manifest
├── ICON.PNG
├── ICON_256.PNG
├── config/
│   ├── privilege
│   └── resource
├── app/
│   ├── docker/
│   │   └── docker-compose.yaml
│   └── ui/
│       ├── config
│       └── images/
│           ├── icon_64.png
│           └── icon_256.png
├── cmd/
│   ├── main
│   ├── install_init
│   ├── install_callback
│   ├── upgrade_init
│   ├── upgrade_callback
│   ├── uninstall_init
│   ├── uninstall_callback
│   ├── config_init
│   └── config_callback
└── wizard/
    ├── install
    ├── upgrade
    ├── config
    └── uninstall
```

构建脚本负责：

1. 复制 `fnos/` 模板到临时目录。
2. 用 `VERSION` 替换 `manifest.version`。
3. 生成对应的 `docker-compose.yaml`。
4. 调用 `fnpack build`。
5. 输出 `havline-<version>.fpk`。

---

## 四、Manifest 草案

```ini
appname=havline
version=1.9.0
display_name=Havline
desc=NAS 公网访问管理：反向代理、FRP、Cloudflare Tunnel、DDNS 与证书
source=thirdparty
platform=all
maintainer=Havline
maintainer_url=https://example.com
os_min_version=1.0.0
desktop_uidir=ui
desktop_applaunchname=havline.main
service_port=6893
checkport=true
ctl_stop=true
changelog=本次更新
```

字段说明：

- `appname=havline`：应用唯一标识。
- `platform=all`：包内不含架构二进制，实际架构由 multi-arch 镜像决定。
- `service_port=6893`：桌面入口默认访问端口。
- `ctl_stop=true`：应用中心显示启动、停止和运行状态。
- `desktop_applaunchname=havline.main`：应用卡片默认入口。

### 多架构要求

发布时必须提供：

```text
linux/amd64
linux/arm64
```

否则 `platform=all` 只在镜像本身支持目标架构时成立。

---

## 五、权限与资源

### 5.1 `config/privilege`

```json
{
  "defaults": {
    "run-as": "package"
  },
  "username": "havline",
  "groupname": "havline"
}
```

原则：

- 长期运行的主程序使用包用户。
- 不默认给 Root。
- 不默认挂载 Docker Socket。
- 不默认加入 `docker` 用户组。

如果未来需要启用内部 updater，应单独评估 Docker Socket 的权限边界。

### 5.2 `config/resource`

```json
{
  "docker-project": {
    "projects": [
      {
        "name": "havline",
        "path": "docker"
      }
    ]
  }
}
```

可选共享目录：

```json
{
  "data-share": {
    "shares": [
      {
        "name": "havline/backups"
      }
    ]
  }
}
```

建议首发版本只暴露备份目录。

不要暴露：

- `havline.db`
- `session_secret`
- `certs/`
- `nginx/`
- `cloudflared/`
- `frp/`

这些内容包含访问控制、证书和加密凭据。

---

## 六、Docker Compose 草案

建议新建：

```text
fnos/app/docker/docker-compose.yaml
```

内容：

```yaml
name: havline

services:
  havline:
    image: ghcr.io/example/havline:${TRIM_APPVER}
    container_name: havline
    restart: unless-stopped
    ports:
      - "${TRIM_SERVICE_PORT}:6893"
      - "${wizard_nginx_http_port}:18080"
      - "${wizard_nginx_https_port}:9443"
    volumes:
      - "${TRIM_PKGVAR}/data:/data"
    environment:
      HAVLINE_DATA_DIR: /data
      HAVLINE_LISTEN: ":6893"
      HAVLINE_NGINX_HTTP_PORT: "${wizard_nginx_http_port}"
      HAVLINE_NGINX_HTTPS_PORT: "${wizard_nginx_https_port}"
      HAVLINE_SESSION_SECRET: ""
      TZ: "${wizard_timezone}"
      HAVLINE_UPDATER_SOCKET: ""
```

说明：

- `TRIM_SERVICE_PORT` 默认映射管理界面。
- 使用 bridge 网络，避免与 fnOS 现有 host 网络服务直接冲突。
- Nginx 使用 18080/9443，避免占用 fnOS 的 80/443。
- 数据全部落在 `TRIM_PKGVAR/data`。
- 不挂载 Docker Socket。
- 不启动 updater sidecar。

### 为什么首发不直接用 host 网络

host 网络确实更贴近 Havline 裸机模式，但：

- fnOS 本身和其他应用可能占用大量端口；
- 应用中心无法明确管理端口冲突；
- bridge + 端口映射更容易解释和回滚。

首发建议 bridge。

后续可提供“高级安装模式：host 网络”。

---

## 七、桌面入口

`app/ui/config`：

```json
{
  ".url": {
    "havline.main": {
      "title": "Havline",
      "icon": "images/icon_{0}.png",
      "type": "iframe",
      "protocol": "http",
      "port": "${TRIM_SERVICE_PORT}",
      "url": "/",
      "allUsers": false,
      "control": {
        "accessPerm": "readonly"
      }
    }
  }
}
```

考虑：

- Havline 当前是单管理员产品，默认只让管理员可见。
- 使用端口服务，不依赖 fnOS 网关。
- 后续若新增统一网关，可把入口改成 `gatewayPrefix/gatewaySocket`，无需端口。

---

## 八、生命周期脚本

### `cmd/main`

Docker 项目的启停由 fnOS Docker 资源处理，`main` 主要提供状态检查。

```bash
#!/bin/bash
CONTAINER_NAME="havline"

is_running() {
  docker inspect "$CONTAINER_NAME" 2>/dev/null | grep -q '"Status": "running"'
}

case "$1" in
  start|stop)
    exit 0
    ;;
  status)
    if is_running; then
      exit 0
    fi
    exit 3
    ;;
  *)
    echo "Unknown command: $1" > "$TRIM_TEMP_LOGFILE"
    exit 1
    ;;
esac
```

### `cmd/install_callback`

职责：

- 创建 `TRIM_PKGVAR/data`
- 修正目录权限
- 如果用户选择了迁移，执行数据复制
- 输出安装摘要

### `cmd/upgrade_callback`

职责：

- 检查数据库可读写
- 保留已有的 `session_secret`
- 记录升级前后版本
- 可选：导入旧版本迁移文件

### `cmd/config_callback`

职责：

- 配置端口或时区变化时，验证新端口
- 如果只是环境变量变化，尽量交给 Docker Compose 重建
- 输出清晰的失败原因到 `TRIM_TEMP_LOGFILE`

### `cmd/uninstall_callback`

默认行为：

- 删除容器和程序
- 保留应用数据

只有用户在卸载向导中明确选择“删除全部数据”时，才删除：

```text
${TRIM_PKGVAR}/data
```

---

## 九、安装向导

### `wizard/install`

建议字段：

```json
[
  {
    "stepTitle": "Havline 访问端口",
    "items": [
      {
        "type": "text",
        "field": "wizard_nginx_http_port",
        "label": "Nginx HTTP 端口",
        "initValue": "18080",
        "rules": [
          {
            "required": true,
            "message": "请输入端口"
          },
          {
            "pattern": "^[0-9]+$",
            "message": "端口必须是数字"
          }
        ]
      },
      {
        "type": "text",
        "field": "wizard_nginx_https_port",
        "label": "Nginx HTTPS 端口",
        "initValue": "9443",
        "rules": [
          {
            "required": true,
            "message": "请输入端口"
          },
          {
            "pattern": "^[0-9]+$",
            "message": "端口必须是数字"
          }
        ]
      }
    ]
  },
  {
    "stepTitle": "基础设置",
    "items": [
      {
        "type": "select",
        "field": "wizard_timezone",
        "label": "时区",
        "initValue": "Asia/Shanghai",
        "options": [
          {
            "label": "Asia/Shanghai",
            "value": "Asia/Shanghai"
          },
          {
            "label": "UTC",
            "value": "UTC"
          }
        ]
      }
    ]
  }
]
```

### 管理员初始化

当前 Havline 首次启动会自动生成随机管理员密码并写入日志。

对于 fnOS 应用，建议改造为：

1. 安装向导收集初始管理员密码；或
2. 首次打开 Havline 时进入创建管理员页面；或
3. 读取 `HAVLINE_INITIAL_ADMIN_PASSWORD` 进行一次性初始化。

**不建议继续把管理员密码写入日志。**

---

## 十、环境变量映射

| fnOS 变量 | 映射到 Havline | 说明 |
| --- | --- | --- |
| `TRIM_APPVER` | 镜像 tag | 应用版本 |
| `TRIM_SERVICE_PORT` | 6893 映射 | 桌面入口 |
| `TRIM_PKGVAR` | `/data` | 持久化数据 |
| `TRIM_APPDEST` | 应用 target | 只读程序目录 |
| `wizard_nginx_http_port` | `HAVLINE_NGINX_HTTP_PORT` | Nginx HTTP |
| `wizard_nginx_https_port` | `HAVLINE_NGINX_HTTPS_PORT` | Nginx HTTPS |
| `wizard_timezone` | `TZ` | 时区 |

Havline 数据目录内应继续保留：

```text
${TRIM_PKGVAR}/data/session_secret
```

不要每次安装或升级重新生成。

---

## 十一、更新策略

### 首发方案

统一使用飞牛应用中心升级：

- 更新 `.fpk`
- 拉取新 multi-arch 镜像
- 保留 `${TRIM_PKGVAR}/data`
- 重启 Docker Compose 项目

### 禁用 Havline 自带 updater

fnOS 包内：

```text
HAVLINE_UPDATER_SOCKET=""
```

并且 compose 不启动：

```text
havline-updater
```

原因：

- App Center 已经是唯一更新入口。
- 避免 Docker Socket 权限。
- 避免应用目录映射错误。
- 避免升级 Recursive / 自更新冲突。

### 后续可选

如果确实需要应用内更新，再增加“高级更新模式”：

- 显式让用户开启；
- 显式挂载 Docker Socket；
- 显式配置宿主项目路径；
- 失败时回滚到上一版镜像。

---

## 十二、数据迁移方案

### 从 Docker Compose 迁移

旧环境可能有：

```text
/opt/havline/data
```

或 Docker 命名卷：

```text
havline-data
```

迁移步骤：

1. 停止旧 Havline。
2. 备份旧数据目录。
3. 将旧目录复制到：

```text
${TRIM_PKGVAR}/data
```

4. 确认 `session_secret` 和 `havline.db` 都存在。
5. 启动 fnOS 应用。
6. 登录并验证：
   - 用户与设置
   - 反向代理规则
   - FRP 服务端
   - Cloudflare 隧道
   - 证书
   - 备份列表

### 从裸机迁移

裸机默认数据目录通常是：

```text
./.data
```

迁移内容一致，重点是保持 `session_secret` 不变。

否则：

- 已加密的 API Token
- DDNS 凭据
- 通知凭据
- Cloudflare 凭据

可能无法解密。

---

## 十三、安全设计

### 默认安全基线

- `run-as=package`
- 不启用 Root
- 不挂载 Docker Socket
- 不暴露证书、数据库和加密密钥目录
- 数据目录仅应用用户可读写
- 仅暴露管理端口、Nginx 高位的 HTTP/HTTPS 端口
- 管理员入口默认只对 fnOS 管理员可见

### 首次管理员密码

必须改造当前“随机密码写日志”行为。

推荐优先级：

1. 首次进入创建管理员页面
2. 安装向导收集密码
3. 环境变量一次性初始化

### Cloudflare Token

仍由 Havline 独立加密保存：

```text
${TRIM_PKGVAR}/data/session_secret
```

不放入 fnOS 共享目录。

---

## 十四、构建与发布流程

### 1. 构建 multi-arch 镜像

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION_FILE=VERSION \
  -t registry.example.com/havline:${VERSION} \
  --push .
```

### 2. 生成 fnOS 项目

从以下模板生成：

```text
fnos/
```

替换：

- `manifest.version`
- Compose 镜像 tag
- 应用说明
- `changelog`

### 3. 打包

```bash
fnpack build --directory dist/fnos/havline
```

### 4. 测试设备安装

```bash
appcenter-cli install-fpk havline-${VERSION}.fpk --env config.env
```

### 5. 发布前验证

在至少一台 x86 和一台 ARM 测试设备验证：

- 安装
- 启动
- 桌面入口
- 端口映射
- 数据持久化
- 应用中心升级
- 卸载保留数据
- 卸载删除数据

---

## 十五、分阶段实施

### 阶段 1：可用包

- 新增 `fnos/` 目录
- 编写 manifest
- Docker Compose 模板
- 安装/卸载/状态生命周期
- 管理入口
- 端口向导
- 使用固定镜像 tag

目标：可以在飞牛测试设备安装并打开 Havline。

### 阶段 2：数据与升级

- `TRIM_PKGVAR` 持久化
- 旧 Docker Compose 数据迁移脚本
- 应用中心升级验证
- 保留/删除数据选项
- `session_secret` 兼容检查

目标：数据不丢，可稳定升级。

### 阶段 3：fnOS 体验

- 首次管理员初始化页面
- 端口冲突检测
- 应用入口图标完善
- 备份目录共享
- 时区和系统信息适配

目标：符合飞牛应用中心体验。

### 阶段 4：高级集成

- 可选统一网关
- 可选 Docker Socket updater
- fnOS 开放 API 集成
- 应用中心审核材料
- ARM 实机完整验证

---

## 十六、风险与处理

| 风险 | 影响 | 处理 |
| --- | --- | --- |
| `80/443` 被 fnOS 占用 | Havline Nginx 无法启动 | 默认改用 18080/9443，向导可改 |
| 镜像只有 amd64 | ARM 设备无法运行 | CI 构建 amd64 + arm64 |
| fnOS 应用升级与内置 updater 冲突 | 版本回退或容器失控 | 首发禁用内置 updater |
| `session_secret` 丢失 | 加密凭据无法解密 | 持久化到 `TRIM_PKGVAR`，迁移时必须保留 |
| Docker Socket 权限过大 | 安全风险 | 默认不挂载 |
| 数据目录权限错误 | 数据库无法写入 | `install_callback` 修正权限 |
| 管理员密码写日志 | 凭据泄露 | 安装向导或首次设置页 |
| 端口冲突 | 应用无法启动 | 向导配置，安装时检测 |
| ARM 镜像无 cloudflared | Cloudflare 隧道不可用 | 镜像内校验架构，或运行时下载对应 binary |

---

## 十七、官方文档参考

- 欢迎加入：<https://developer.fnnas.com/docs/guide/>
- 快速开始：
  - <https://developer.fnnas.com/docs/quick-started/prerequisites/>
  - <https://developer.fnnas.com/docs/quick-started/create-application/>
  - <https://developer.fnnas.com/docs/quick-started/test-application/>
  - <https://developer.fnnas.com/docs/quick-started/publish-application/>
- 核心概念：
  - <https://developer.fnnas.com/docs/core-concepts/manifest/>
  - <https://developer.fnnas.com/docs/core-concepts/privilege/>
  - <https://developer.fnnas.com/docs/core-concepts/resource/>
  - <https://developer.fnnas.com/docs/core-concepts/runtime/>
  - <https://developer.fnnas.com/docs/core-concepts/environment-variables/>
  - <https://developer.fnnas.com/docs/core-concepts/app-entry/>
  - <https://developer.fnnas.com/docs/core-concepts/gateway-registration/>
  - <https://developer.fnnas.com/docs/core-concepts/wizard/>
- 工具：
  - <https://developer.fnnas.com/docs/cli/fnpack/>
  - <https://developer.fnnas.com/docs/cli/appcentercli/>
- Docker 案例：
  - <https://developer.fnnas.com/docs/examples/docker/>

---

## 十八、结论

Havline 适合做飞牛 Docker 应用，不适合首发做成 CGI 或原生应用。

首发目标：

```text
飞牛应用中心安装
→ Docker Compose 启动 Havline
→ 桌面入口打开管理界面
→ 数据持久化到 TRIM_PKGVAR
→ 应用中心升级
```

最需要先解决的代码问题：

1. 管理员初始化不再写日志。
2. 默认 updater 改为可关闭，fnOS 包中关闭。
3. 增加 multi-arch 镜像发布。
4. 增加 `fnos/` 打包模板和构建脚本。
5. 验证 `TRIM_PKGVAR/data` 数据迁移。

---

## 十九、官方文档复核与实施修正（2026-10-01）

本轮已逐页复核飞牛应用开放平台文档中的以下内容：

- 快速开始：准备工作、创建应用、测试应用、上架应用
- 核心概念：`manifest`、权限、资源、运行时、环境变量、应用入口、统一网关、用户向导
- 开发工具：`fnpack`、`appcenter-cli`
- Docker 应用案例：`fnpack create --template docker`、Compose、资源声明、状态检查、桌面入口

### 19.1 已确认正确的方案

现有方案的大方向与官方 Docker 应用案例一致：

1. 使用 Docker 应用模板，而不是首发做 CGI 或原生应用。
2. 包结构使用 `manifest`、`config/privilege`、`config/resource`、`app/docker/docker-compose.yaml`、`app/ui/config`、`cmd/main` 和 `wizard/`。
3. `config/resource` 声明 Docker 项目资源的格式正确：

```json
{
  "docker-project": {
    "projects": [
      {
        "name": "havline",
        "path": "docker"
      }
    ]
  }
}
```

4. `cmd/main` 的 `start` 和 `stop` 可以空操作，Docker 项目启停由飞牛应用中心处理；`status` 检查容器状态，运行中返回 `0`，未运行返回 `3`。
5. 首发关闭 Havline 自带 updater，避免 Docker Socket、宿主机路径映射和飞牛应用中心升级冲突。
6. 默认使用 `run-as=package`，不使用 Root，不挂载 Docker Socket。
7. 使用 `TRIM_SERVICE_PORT`、`TRIM_APPDEST`、`TRIM_PKGVAR` 等变量，而不是硬编码安装路径。

### 19.2 需要修正或补充的实现细节

1. **以 `fnpack` 模板为起点**

   官方 Docker 案例推荐先执行：

   ```bash
   fnpack create havline --template docker
   cd havline
   ```

   因此 `fnos/` 不应完全手写。正确做法是：

   - 先用模板生成标准骨架；
   - 再将 Havline 的 Compose、入口和生命周期脚本替换进去；
   - 让构建脚本只维护差异部分，避免未来官方模板升级时难以同步。

2. **`cmd/main` 的状态检查必须与 Compose 一致**

   Compose 中有 `container_name: havline` 时，`cmd/main` 应检查该名称：

   ```bash
   CONTAINER_NAME="havline"
   ```

   如果以后移除 `container_name`，`cmd/main` 需要按 Compose 实际生成的容器名检查，否则应用中心会一直显示未运行。

3. **向导密钥字段必须使用 `password` 类型**

   官方向导支持 `password` 类型，不能把管理员初始密码或 Token 用普通 `text` 收集：

   ```json
   {
     "type": "password",
     "field": "wizard_initial_admin_password",
     "label": "初始管理员密码",
     "rules": [
       { "required": true, "message": "请输入初始管理员密码" }
     ]
   }
   ```

   向导值会变成同名环境变量，生命周期脚本使用前必须再次校验，并禁止写入日志。

4. **自定义向导字段统一使用 `wizard_` 前缀**

   官方建议：

   - 自定义字段使用 `wizard_` 前缀；
   - 字段名一旦发布应保持稳定；
   - 不要使用 `TRIM_` 前缀，因为该前缀保留给系统变量。

   Havline 建议字段：

   ```text
   wizard_service_port
   wizard_nginx_http_port
   wizard_nginx_https_port
   wizard_timezone
   wizard_initial_admin_password
   ```

5. **生命周期脚本错误必须写入 `TRIM_TEMP_LOGFILE`**

   官方要求生命周期脚本以非零状态退出前，将用户可见错误写入：

   ```text
   TRIM_TEMP_LOGFILE
   ```

   安装、升级、配置和卸载脚本都应遵循该约定，不能只输出到 stdout。

6. **`app/ui/config` 的 `port` 必须三处一致**

   以下三个值必须保持一致：

   ```text
   manifest.service_port
   app/ui/config 的 port
   docker-compose.yaml 中 ${TRIM_SERVICE_PORT}:6893
   ```

   否则应用卡片可能打开失败，或应用中心显示运行但入口不可访问。

7. **Docker Compose 支持官方路径变量**

   官方明确指出 Docker Compose 可以使用：

   ```text
   TRIM_SERVICE_PORT
   TRIM_APPDEST
   TRIM_PKGVAR
   TRIM_DATA_SHARE_PATHS
   ```

   Havline 推荐继续使用：

   ```yaml
   volumes:
     - "${TRIM_PKGVAR}:/data"
   ```

   不要额外拼出无意义的 `${TRIM_PKGVAR}/data`，除非确实要在数据根下再做子目录。

8. **共享目录只暴露明确需要的路径**

   `config/resource` 的 `data-share` 会让用户在文件管理器中看到路径。首发只能暴露备份目录，绝不能暴露：

   ```text
   session_secret
   havline.db
   certs/
   nginx/
   frp/
   cloudflared/
   ```

9. **镜像必须真正支持 amd64 与 arm64**

   `platform=all` 只代表应用包不含架构相关二进制，容器镜像本身仍必须支持目标架构。CI 必须构建并推送：

   ```text
   linux/amd64
   linux/arm64
   ```

   否则 x86 测试通过但 ARM 设备仍会启动失败。

10. **版本和升级说明由 fnOS 管理**

    首发使用飞牛应用中心升级。`manifest.changelog` 应面向用户简洁表达本次变更；
    Havline 内部升级页面、`havline-updater` sidecar 和 Docker Socket 默认不进入 fnOS 包。

### 19.3 建议的首发实施顺序

阶段 1 只做“能安装、能打开、能保留数据”：

1. 用 `fnpack create havline --template docker` 生成标准骨架。
2. 改写 `manifest`，确定 `appname=havline`、`service_port` 和桌面入口。
3. 编写 Havline 专用 `app/docker/docker-compose.yaml`。
4. 编写 `cmd/main` 容器状态检查。
5. 编写 `wizard/install`，只收集端口、时区和初始管理员密码。
6. 编写 `cmd/install_callback`，创建数据目录、修正权限并完成首次初始化。
7. 构建 amd64 / arm64 镜像。
8. 用 `fnpack build` 打包并在飞牛测试设备安装验证。

阶段 1 暂不实现：

- Havline 自带 updater；
- Docker Socket；
- 统一网关 Unix Socket；
- 高级 host 网络模式；
- 自动迁移旧 Docker Compose 数据。

### 19.4 首发验收清单

```text
[ ] fnpack 模板生成的项目结构与官方一致
[ ] manifest 必填字段完整
[ ] service_port、入口 port、Compose 映射三处一致
[ ] cmd/main status 与 Compose 容器名一致
[ ] wizard 使用 password 类型收集初始密码
[ ] 安装回调创建并修正 TRIM_PKGVAR 权限
[ ] 数据目录持久化，升级后 session_secret 不变
[ ] 默认不挂载 Docker Socket
[ ] 默认关闭 Havline updater
[ ] amd64 与 arm64 镜像均可拉取并启动
[ ] 卸载默认保留数据，明确选择后才删除
[ ] TRIM_TEMP_LOGFILE 能输出可读错误
```

### 19.5 结论

现有方案不需要推倒重来，主要修正点是把手工设计的包结构收敛到官方
`fnpack create --template docker` 模板，并补齐向导密码、状态检查、
路径变量、生命周期错误输出和双架构镜像验证。
