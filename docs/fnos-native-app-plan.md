# Havline 飞牛 Native 应用改造方案

> 日期：2026-10-02
> 依据：飞牛应用开放平台 Native 应用案例、统一网关、应用权限、环境变量、应用资源
> 目标：把 Havline 做成完全不依赖 Docker 的飞牛原生应用

## 一、结论

技术上可行，但不建议直接把当前离线 Docker 包替换掉。

推荐路线：

```text
Go 主程序原生运行
+ 统一网关 Unix Socket
+ 打包 Nginx 双架构二进制
+ frpc / cloudflared 按需打包或运行时下载
+ cmd/main 管理进程生命周期
```

最终形态：

```text
不依赖 Docker Engine
不依赖 Docker Compose
不依赖外部镜像仓库
管理界面通过 /app/havline 统一网关访问
公网反代功能通过高位端口运行
```

主要风险：

```text
Nginx 双架构打包与动态库依赖
非 root 用户无法绑定 80/443
进程启动、停止、状态和崩溃恢复
frpc / cloudflared 架构与版本管理
真机升级和回滚验证
```

## 二、官方 Native 应用模型

官方 Native 应用案例说明：

```text
Native 应用是在飞牛 fnOS 上运行的进程
可以提供 Web UI、后台服务、CLI 工具
推荐使用统一网关 /app/{appname}
服务监听 ${TRIM_APPDEST}/app.sock
cmd/main 负责 start、stop、status
应用文件放在 ${TRIM_APPDEST}
数据放在 ${TRIM_PKGVAR}
```

官方 Notepad 示例的关键结构：

```text
notepad/
├── app/
│   ├── server/
│   └── ui/config
├── cmd/
│   └── main
├── config/
│   ├── privilege
│   └── resource
├── manifest
├── ICON.PNG
└── ICON_256.PNG
```

启动脚本核心逻辑：

```bash
APP_DIR="${TRIM_APPDEST}/server"
SOCKET_PATH="${TRIM_APPDEST}/app.sock"

start)
  cd "$APP_DIR"
  SOCKET_PATH="$SOCKET_PATH" ./havline &
  echo "$!" > "$PID_FILE"
  ;;

stop)
  kill -TERM "$(cat "$PID_FILE")"
  rm -f "$SOCKET_PATH" "$PID_FILE"
  ;;

status)
  kill -0 "$(cat "$PID_FILE")"
  ;;
```

## 三、Havline 依赖清单

### 3.1 Go 主程序

可以直接编译为 Linux 静态二进制：

```text
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build
```

优点：

```text
SQLite 使用 modernc.org/sqlite，纯 Go，不依赖 CGO
前端已经通过 go:embed 内嵌
ACME、DDNS、通知、备份均为 Go 实现
不需要 Node.js、Python 或 Java 运行时
```

### 3.2 必需外部依赖

| 依赖 | 用途 | 原生方案 |
| --- | --- | --- |
| Nginx | 反向代理、HTTPS、证书加载 | 必须打包双架构二进制 |
| mime.types | Nginx MIME 配置 | 随包分发 |
| ca-certificates | HTTPS 客户端信任 | 使用系统证书或随包分发 |
| tzdata | 时区 | 使用系统时区或随包分发 |

### 3.3 可选外部依赖

| 依赖 | 用途 | 原生方案 |
| --- | --- | --- |
| frpc | FRP 内网穿透 | 打包双架构或运行时下载 |
| cloudflared | Cloudflare Tunnel | 打包双架构或运行时下载 |
| ssh | Agent 远程安装 | 使用系统 SSH |
| systemctl | 公网 Agent 管理 frps | 仅 Agent 场景需要 |
| ss | 端口诊断 | 使用系统工具 |

### 3.4 可以移除的依赖

原生应用不需要：

```text
Docker Engine
Docker Compose
Docker Socket
havline-updater sidecar
宿主机 Compose 文件
```

## 四、目标包结构

建议新增独立工程：

```text
fnos-native/
└── havline/
    ├── manifest
    ├── ICON.PNG
    ├── ICON_256.PNG
    ├── app/
    │   ├── server/
    │   │   ├── amd64/
    │   │   │   ├── havline
    │   │   │   └── nginx
    │   │   ├── arm64/
    │   │   │   ├── havline
    │   │   │   └── nginx
    │   │   ├── mime.types
    │   │   └── VERSION
    │   └── ui/
    │       ├── config
    │       └── images/
    ├── cmd/
    │   ├── main
    │   ├── install_callback
    │   ├── upgrade_callback
    │   ├── config_callback
    │   └── uninstall_callback
    ├── config/
    │   ├── privilege
    │   └── resource
    └── wizard/
        ├── install
        ├── config
        └── uninstall
```

安装后：

```text
/var/apps/havline/
├── target/
│   ├── server/
│   │   ├── amd64/
│   │   ├── arm64/
│   │   └── mime.types
│   └── app.sock
├── var/
│   ├── havline.db
│   ├── session_secret
│   ├── nginx/
│   ├── certs/
│   ├── frp/
│   └── cloudflared/
└── cmd/
```

## 五、运行架构

```text
cmd/main start
  -> 选择当前架构目录
  -> 设置 HAVLINE_DATA_DIR=${TRIM_PKGVAR}
  -> 设置 HAVLINE_NGINX_BIN=${TRIM_APPDEST}/server/<arch>/nginx
  -> 设置 HAVLINE_NGINX_MIME_TYPES=${TRIM_APPDEST}/server/mime.types
  -> 设置 HAVLINE_GATEWAY_SOCKET=${TRIM_APPDEST}/app.sock
  -> 启动 havline 主程序
  -> 写入 PID 文件

havline 主程序
  -> 监听 Unix Socket 供统一网关访问
  -> 启动 Nginx 子进程
  -> 按需启动 frpc / cloudflared
  -> 管理数据库、证书、DDNS、反代规则

cmd/main stop
  -> 发送 SIGTERM
  -> 等待主程序退出
  -> 主程序负责停止 Nginx 和可选子进程
  -> 清理 app.sock 和 PID 文件

cmd/main status
  -> 检查 PID 是否存活
  -> 存活返回 0，否则返回 3
```

## 六、统一网关接入

`app/ui/config`：

```json
{
  ".url": {
    "havline.main": {
      "title": "Havline",
      "icon": "images/icon_{0}.png",
      "type": "iframe",
      "protocol": "",
      "gatewayPrefix": "/app/havline",
      "gatewaySocket": "app.sock",
      "url": "/app/havline",
      "allUsers": false,
      "control": {
        "accessPerm": "readonly"
      }
    }
  }
}
```

主程序需要新增：

```text
HAVLINE_GATEWAY_SOCKET=${TRIM_APPDEST}/app.sock
HAVLINE_GATEWAY_PREFIX=/app/havline
```

监听方式：

```go
listener, err := net.Listen("unix", cfg.GatewaySocket)
if err != nil {
    return err
}
os.Chmod(cfg.GatewaySocket, 0o660)
server.Serve(listener)
```

前端需要支持：

```text
Vite base = /app/havline/
Vue Router base = /app/havline/
API base = /app/havline/api
SSE base = /app/havline/api/...
WebSocket base = /app/havline/ws
Cookie Path = /app/havline 或 /
```

## 七、Nginx 原生打包

这是整个改造最大的技术点。

### 7.1 可选方案

| 方案 | 优点 | 缺点 |
| --- | --- | --- |
| 打包静态 Nginx | 不依赖系统 Nginx | 需要自行构建双架构 |
| 打包动态 Nginx + 依赖库 | 接近发行版行为 | 需要处理 .so 和 LD_LIBRARY_PATH |
| 使用飞牛系统 Nginx | 包体小 | 版本、路径、配置目录不可控 |
| 改用 Go 内置反向代理 | 完全纯 Go | Havline 现有 Nginx 功能需要重写 |

推荐：

```text
打包静态 Nginx 或半静态 Nginx
amd64 与 arm64 各一份
随包提供 mime.types
```

### 7.2 Nginx 配置调整

原生应用以 package 用户运行，不能直接绑定 80/443。

建议：

```text
管理界面：统一网关 Unix Socket
Nginx HTTP：18080
Nginx HTTPS：9443
```

如果确实需要 80/443：

```text
使用 root 生命周期脚本启动 Nginx master
Nginx worker 降权到 package 用户
或者给二进制授予 CAP_NET_BIND_SERVICE
```

不推荐让整个 Havline 长期以 root 运行。

### 7.3 Nginx 目录

建议：

```text
${TRIM_APPDEST}/server/<arch>/nginx
${TRIM_APPDEST}/server/mime.types
${TRIM_PKGVAR}/nginx/
${TRIM_PKGVAR}/certs/
${TRIM_PKGVAR}/logs/
```

环境变量：

```text
HAVLINE_NGINX_BIN=${TRIM_APPDEST}/server/<arch>/nginx
HAVLINE_NGINX_MIME_TYPES=${TRIM_APPDEST}/server/mime.types
HAVLINE_NGINX_PID=${TRIM_PKGVAR}/nginx/nginx.pid
HAVLINE_NGINX_HTTP_PORT=18080
HAVLINE_NGINX_HTTPS_PORT=9443
```

## 八、frpc 与 cloudflared

这两个是可选功能依赖。

### 方案 A：随包分发

```text
app/server/amd64/frpc
app/server/arm64/frpc
app/server/amd64/cloudflared
app/server/arm64/cloudflared
```

优点：

```text
完全离线
版本可控
```

缺点：

```text
包体变大
每次升级 Havline 都要同步升级二进制
```

### 方案 B：运行时按需下载

保持 Havline 现有下载逻辑，但把下载目录指向：

```text
${TRIM_PKGVAR}/frp/
${TRIM_PKGVAR}/cloudflared/
```

优点：

```text
FPK 更小
```

缺点：

```text
首次使用需要网络
下载源可能被墙
需要校验 SHA256
```

推荐首版：

```text
frpc：随包分发
cloudflared：随包分发或首次使用时下载
```

## 九、数据目录与迁移

原生应用数据目录：

```text
${TRIM_PKGVAR}
```

需要保留：

```text
havline.db
session_secret
nginx/
certs/
frp/
cloudflared/
logs/
ssh_known_hosts
```

从当前离线 Docker 包迁移：

```text
Docker 包数据目录：${TRIM_PKGVAR}
原生包数据目录：${TRIM_PKGVAR}
```

如果两个应用使用同一个 appname，路径可能相同。

迁移步骤：

```text
1. 停止 Docker 版 Havline
2. 备份数据目录
3. 安装原生版
4. 确认 session_secret 不变
5. 启动原生版
6. 验证数据库、证书、规则、隧道
```

## 十、生命周期脚本

### 10.1 cmd/main

职责：

```text
start：选择架构、设置环境变量、启动 havline、写 PID
stop：发送 SIGTERM、等待退出、清理 PID 和 Socket
status：检查 PID 是否存活
```

关键环境变量：

```bash
APP_DIR="${TRIM_APPDEST}/server"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH_DIR="amd64" ;;
  aarch64|arm64) ARCH_DIR="arm64" ;;
  *) exit 1 ;;
esac

export HAVLINE_DATA_DIR="$TRIM_PKGVAR"
export HAVLINE_NGINX_BIN="$APP_DIR/$ARCH_DIR/nginx"
export HAVLINE_NGINX_MIME_TYPES="$APP_DIR/mime.types"
export HAVLINE_NGINX_HTTP_PORT=18080
export HAVLINE_NGINX_HTTPS_PORT=9443
export HAVLINE_GATEWAY_SOCKET="$TRIM_APPDEST/app.sock"
export HAVLINE_GATEWAY_PREFIX="/app/havline"
export HAVLINE_UPDATER_SOCKET=""
```

### 10.2 install_callback

职责：

```text
创建 TRIM_PKGVAR
设置目录权限
复制默认配置
检查二进制架构
检查 Nginx 可执行
```

### 10.3 upgrade_callback

职责：

```text
检查数据库和 session_secret
保留数据目录
检查新二进制架构
必要时迁移 Nginx 配置
```

### 10.4 uninstall_callback

职责：

```text
默认保留 TRIM_PKGVAR
用户选择 purge 时删除数据
清理 app.sock 和 PID 文件
```

## 十一、多架构构建

构建脚本需要输出：

```text
amd64/havline
arm64/havline
amd64/nginx
arm64/nginx
```

Go 二进制：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o amd64/havline ./cmd/havline
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o arm64/havline ./cmd/havline
```

Nginx：

```text
从静态构建产物复制
或从容器镜像中提取
并检查动态库依赖
```

检查：

```bash
file amd64/havline
file arm64/havline
ldd amd64/nginx
ldd arm64/nginx
```

## 十二、分阶段实施

### 阶段 1：Native PoC

目标：不打包 Nginx，先验证 Go 主程序原生运行。

内容：

```text
编译 amd64/arm64 Go 二进制
统一网关 Unix Socket
cmd/main start/stop/status
TRIM_PKGVAR 数据目录
```

验收：

```text
[ ] 应用可安装
[ ] /app/havline 可打开
[ ] 进程可启停
[ ] 数据可持久化
```

### 阶段 2：Nginx 打包

目标：让反向代理功能原生可用。

内容：

```text
静态 Nginx 双架构
mime.types
18080/9443 端口
证书目录
nginx -t 校验
```

验收：

```text
[ ] Nginx 可启动
[ ] 配置可校验
[ ] HTTPS 可加载
[ ] 反向代理规则生效
```

### 阶段 3：frpc / cloudflared

目标：恢复 FRP 和 Cloudflare Tunnel 功能。

内容：

```text
双架构二进制
或按需下载
SHA256 校验
进程管理
日志
```

### 阶段 4：升级与迁移

目标：从 Docker 包迁移到原生包。

内容：

```text
数据迁移
session_secret 保留
版本兼容
回滚方案
```

### 阶段 5：真机验证

目标：x86_64 与 ARM 设备安装验证。

内容：

```text
安装、启动、停止
统一网关入口
反向代理
证书
FRP
Cloudflare
升级
卸载
```

## 十三、验收清单

```text
[ ] 不依赖 Docker Engine
[ ] 不依赖 Docker Compose
[ ] 不依赖外部镜像仓库
[ ] Go 二进制支持 amd64 与 arm64
[ ] Nginx 支持 amd64 与 arm64
[ ] 管理界面通过 /app/havline 打开
[ ] Unix Socket 权限正确
[ ] 主程序可启动、停止、检查状态
[ ] 数据写入 TRIM_PKGVAR
[ ] session_secret 升级后不变
[ ] Nginx 反向代理正常
[ ] HTTPS 证书正常
[ ] FRP 功能正常
[ ] Cloudflare Tunnel 功能正常
[ ] 升级不丢数据
[ ] 卸载默认保留数据
```

## 十四、风险与工作量

| 风险 | 影响 | 处理 |
| --- | --- | --- |
| Nginx 双架构打包 | 原生版无法启动 | 阶段 2 专项验证 |
| Nginx 动态库依赖 | 目标设备缺库 | 静态编译或随包分发 .so |
| 非 root 绑定 80/443 | 端口权限失败 | 使用 18080/9443 |
| 进程崩溃 | 服务中断 | PID 检查、重启策略、日志 |
| Socket 权限 | 网关无法访问 | 启动时 chmod |
| 前端绝对路径 | 网关下 404 | 统一 base path |
| frpc/cloudflared 版本 | 功能不可用 | 随包分发并校验 SHA256 |
| 数据迁移 | 凭据无法解密 | 保留 session_secret |
| 升级替换二进制 | 运行中进程被覆盖 | stop 后再替换 |
| ARM 真机差异 | 测试不充分 | 真机验证 |

工作量估算：

```text
阶段 1：2 到 3 天
阶段 2：3 到 5 天
阶段 3：2 到 4 天
阶段 4：2 到 3 天
阶段 5：3 到 5 天
合计：2 到 4 周
```

## 十五、与离线 Docker 包对比

| 项目 | 离线 Docker 包 | 原生应用 |
| --- | --- | --- |
| 依赖 Docker | 是 | 否 |
| 依赖仓库 | 否 | 否 |
| 包体 | 约 150 MB | 预计 40 到 80 MB |
| 改造量 | 已完成 | 2 到 4 周 |
| Nginx | 容器内已有 | 需要自行打包 |
| 升级 | Docker load | 替换二进制 |
| 进程管理 | Docker Compose | cmd/main |
| 风险 | 低 | 高 |
| 推荐度 | 当前优先 | 长期演进 |

## 十六、建议

当前阶段建议：

```text
先发布并验证离线 Docker 包
保留统一网关改造方案
原生应用作为独立分支推进
```

原生应用的推荐顺序：

```text
1. 先做 Go 主程序 + 统一网关 PoC
2. 再做 Nginx 双架构打包
3. 再恢复 FRP 和 Cloudflare
4. 最后做数据迁移和真机升级
```

只有在以下条件满足时，才建议把原生应用作为正式发布形态：

```text
Nginx 双架构在真实设备稳定运行
统一网关 Socket 权限稳定
升级和回滚验证通过
frpc / cloudflared 架构验证通过
```

## 十七、官方参考

- Native 应用案例：<https://developer.fnnas.com/docs/examples/native/>
- 统一网关：<https://developer.fnnas.com/docs/core-concepts/gateway-registration/>
- 应用权限：<https://developer.fnnas.com/docs/core-concepts/privilege/>
- 环境变量：<https://developer.fnnas.com/docs/core-concepts/environment-variables/>
- 应用资源：<https://developer.fnnas.com/docs/core-concepts/resource/>
- 应用入口：<https://developer.fnnas.com/docs/core-concepts/app-entry/>
