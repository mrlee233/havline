# Havline 飞牛统一网关应用改造方案

> 日期：2026-10-02
> 适用范围：`fnos/havline` 飞牛应用包与 Havline 主程序 1.9.x
> 官方依据：飞牛应用开放平台「统一网关」「应用入口」「应用资源」「环境变量」「Docker 应用案例」

## 一、结论先行

统一网关不是 Docker 应用模板的替代品，而是飞牛应用的一种**访问模型**。

官方文档明确说明：Docker 应用也可以使用统一网关。做法是把
`${TRIM_APPDEST}` 挂载到容器内，让容器中的服务在安装目录下创建
Unix Socket，再由 fnOS 网关转发请求。

因此 Havline 的正确改造方向是：

```text
Docker 运行形态（保留）
+ 统一网关访问模型（新增）
+ 端口服务回退（保留）
```

而不是把现有 Docker 应用整体推翻重做。

推荐首版边界：

```text
管理界面：
  通过 /app/havline 和 Unix Socket 暴露

公网反代、DDNS、证书、FRP、Cloudflare Tunnel：
  仍由 Havline 自己管理

端口 18080/9443：
  作为公网反代功能端口和回退入口保留
```

## 二、统一网关是什么

飞牛统一网关为应用提供飞牛 fnOS 访问域名下的稳定系统 URL。

请求路径：

```text
/app/{appname}
/app/{appname}/{customPath}
```

工作流程：

1. 用户在飞牛桌面打开应用。
2. fnOS 先校验用户会话。
3. fnOS 将请求转发到应用声明的 Unix Socket。
4. 应用读取网关注入的用户 Header。
5. 应用继续执行自己的业务权限校验。

统一网关与端口服务的对比：

| 能力 | 端口服务 | 统一网关 |
| --- | --- | --- |
| 访问地址 | 独立端口 | `/app/{appname}` |
| NAS 登录态 | 不接入 | 转发前校验 |
| 用户上下文 | 无 | `X-Trim-*` Header |
| WebSocket | 应用自行处理 | 支持 |
| 常驻服务 | 支持 | 支持 |
| 部署形态 | 任意 | 任意，Docker 也支持 |
| 核心适配 | 监听端口 | 监听 Unix Socket |

## 三、官方字段与约束

### 3.1 应用入口

统一网关入口写在 `app/ui/config`：

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

字段规则：

```text
gatewayPrefix
  必须使用 /app/{appname} 或 /app/{appname}/{customPath}
  发布后应保持稳定

gatewaySocket
  只填写 Socket 文件名，例如 app.sock
  实际路径为 ${TRIM_APPDEST}/app.sock

protocol / port
  在统一网关入口中被忽略，不参与路由
```

### 3.2 Docker 应用接入

官方 Docker 案例给出的结构：

```yaml
services:
  web:
    image: my-app:latest
    container_name: hello-docker-web
    restart: unless-stopped
    volumes:
      - "${TRIM_APPDEST}:/app/target:rw"
    environment:
      GATEWAY_SOCKET: /app/target/app.sock
```

容器内服务监听：

```text
/app/target/app.sock
```

宿主机对应路径：

```text
/var/apps/{appname}/target/app.sock
```

### 3.3 用户 Header

网关校验会话后注入：

| Header | 含义 | 示例 |
| --- | --- | --- |
| `X-Trim-Userid` | 当前用户 UID | `1000` |
| `X-Trim-Isadmin` | 是否管理员 | `true` / `false` |
| `X-Trim-Username` | 当前用户名 | `admin` |

这些 Header 只能作为网关可信上下文使用。应用仍然需要校验：

```text
接口权限
资源归属
文件路径
高风险操作
```

不能信任客户端自己传入的用户 ID。

## 四、当前 Havline 的差距

### 4.1 当前监听方式

```text
HAVLINE_LISTEN=:6893
```

主程序使用 TCP 端口监听：

```text
internal/config/config.go
  ListenAddr = HAVLINE_LISTEN，默认 :6893

internal/app/app.go
  http.Server{Addr: cfg.ListenAddr}
  server.ListenAndServe()
```

当前没有 Unix Socket 监听能力。

### 4.2 当前飞牛入口

```json
{
  ".url": {
    "havline.main": {
      "title": "Havline",
      "icon": "images/icon_{0}.png",
      "type": "iframe",
      "protocol": "http",
      "port": "${wizard_service_port}",
      "url": "/",
      "allUsers": false
    }
  }
}
```

这是端口服务入口，不是统一网关入口。

### 4.3 当前前端路径假设

```text
Vue Router：
  createWebHistory()，无 base

API 客户端：
  直接使用 /api/... 绝对路径

SSE / EventSource：
  /api/proxies/{id}/logs/stream

静态资源：
  由 Vite 默认根路径构建
```

统一网关路径是 `/app/havline`，这些绝对路径不能直接复用。

### 4.4 当前容器权限

Dockerfile 没有显式 `USER`，容器默认以 root 运行。

这对创建 Socket 文件是有利的：

```text
/app/target/app.sock
```

但如果后续改为非 root 用户运行，需要额外处理 Socket 的属主和权限。

### 4.5 当前端口映射

```yaml
ports:
  - "${wizard_service_port}:6893"
  - "${wizard_nginx_http_port}:80"
  - "${wizard_nginx_https_port}:443"
```

统一网关只替代管理界面的入口，不会替代 Havline 的反向代理端口。

## 五、目标架构

### 5.1 访问分层

```text
飞牛桌面
  -> https://fnOS域名/app/havline
  -> 飞牛统一网关校验登录态
  -> /var/apps/havline/target/app.sock
  -> Havline 网关监听器
  -> Havline API / Web UI

Havline 公网反代功能
  -> 18080/9443 端口
  -> Havline 内置 Nginx
  -> 用户配置的 NAS 服务
```

### 5.2 包结构目标

```text
fnos/havline/
├── manifest
├── app/
│   ├── docker/
│   │   └── docker-compose.yaml
│   └── ui/
│       └── config
├── config/
│   ├── privilege
│   └── resource
├── cmd/
│   ├── main
│   ├── install_callback
│   ├── upgrade_callback
│   ├── config_callback
│   └── uninstall_callback
└── wizard/
    ├── install
    ├── config
    └── uninstall
```

### 5.3 Docker Compose 目标

```yaml
services:
  havline:
    image: jayhub.top/mrlee/havline:${TRIM_APPVER}-amd64-docker
    container_name: havline
    restart: unless-stopped
    ports:
      - "${wizard_nginx_http_port}:80"
      - "${wizard_nginx_https_port}:443"
    volumes:
      - "${TRIM_PKGVAR}:/data"
      - "${TRIM_APPDEST}:/app/target:rw"
    environment:
      HAVLINE_DATA_DIR: /data
      HAVLINE_LISTEN: ":6893"
      HAVLINE_GATEWAY_SOCKET: "/app/target/app.sock"
      HAVLINE_GATEWAY_PREFIX: "/app/havline"
      HAVLINE_TRUST_GATEWAY_HEADERS: "false"
      HAVLINE_NGINX_HTTP_PORT: "80"
      HAVLINE_NGINX_HTTPS_PORT: "443"
      HAVLINE_UPDATER_SOCKET: ""
      HAVLINE_INITIAL_ADMIN_PASSWORD: "${wizard_initial_admin_password}"
      TZ: "${wizard_timezone}"
```

说明：

```text
HAVLINE_LISTEN 保留，用于端口回退和内部健康检查
HAVLINE_GATEWAY_SOCKET 指向统一网关 Socket
HAVLINE_GATEWAY_PREFIX 用于前端 base path 和路由
HAVLINE_TRUST_GATEWAY_HEADERS 第一阶段关闭
wizard_service_port 可以从端口映射中移除
```

### 5.4 manifest 目标

首版建议：

```ini
appname               = havline
version               = <版本>
display_name          = Havline
desc                  = NAS 公网访问管理：反向代理、DDNS、HTTPS 证书、FRP、Cloudflare Tunnel 与一键升级
source                = thirdparty
platform              = all
maintainer            = Havline
maintainer_url        = https://jayhub.top/mrlee/Havline
distributor           = Havline
desktop_uidir         = ui
desktop_applaunchname = havline.main
checkport             = false
ctl_stop              = true
service_port          = 6893
```

`service_port` 是否保留需要实测：

```text
如果只注册统一网关入口：
  可以不依赖 service_port

如果同时保留端口回退入口：
  保留 service_port 便于调试
```

## 六、三种实现路线

### 方案 A：Go 主程序原生监听 Unix Socket

实现：

```text
internal/config
  增加 GatewaySocket、GatewayPrefix 配置

internal/app
  同时创建 TCP listener 和 Unix listener
  TCP 用于端口回退
  Unix 用于统一网关
```

伪代码：

```go
tcpServer := &http.Server{Addr: cfg.ListenAddr, Handler: handler}
unixServer := &http.Server{Handler: handler}

if cfg.GatewaySocket != "" {
    _ = os.Remove(cfg.GatewaySocket)
    listener, err := net.Listen("unix", cfg.GatewaySocket)
    if err != nil {
        return err
    }
    if err := os.Chmod(cfg.GatewaySocket, 0o660); err != nil {
        return err
    }
    go unixServer.Serve(listener)
}

go tcpServer.ListenAndServe()
```

优点：

```text
无额外进程
可以精确处理 base path
可以按来源区分 TCP 与网关请求
可以安全启用 X-Trim-* SSO
网络性能更好
```

缺点：

```text
需要修改 Go 主程序
需要补充 Unix Socket 生命周期测试
```

推荐度：高。

### 方案 B：容器内 Nginx 监听 Unix Socket

实现：

```text
容器内 Nginx 监听 /app/target/app.sock
反向代理到 127.0.0.1:6893
```

优点：

```text
不改 Go 主程序
可以快速做统一网关 PoC
```

缺点：

```text
多一层代理
base path 和 Header 处理复杂
错误排查链路更长
SSO 头部的可信边界不清晰
```

推荐度：中，适合过渡验证，不建议作为长期方案。

### 方案 C：单独 Sidecar 容器

实现：

```text
新增 sidecar 容器监听 Unix Socket
代理到 havline 容器
```

优点：

```text
主容器不增加进程
```

缺点：

```text
Compose 复杂度高
生命周期和升级耦合
收益不明显
```

推荐度：低。

## 七、前端 base path 改造

统一网关路径固定为：

```text
/app/havline
```

前端需要支持可配置 base path。

### 7.1 Vite

目标：

```ts
export default defineConfig({
  base: process.env.VITE_BASE_PATH || '/',
  // ...
})
```

构建飞牛统一网关版本时：

```text
VITE_BASE_PATH=/app/havline/
```

### 7.2 Vue Router

当前：

```ts
createWebHistory()
```

目标：

```ts
createWebHistory(import.meta.env.BASE_URL)
```

或者由运行时 `HAVLINE_GATEWAY_PREFIX` 注入。

### 7.3 API 客户端

当前：

```ts
fetch('/api/...')
```

目标：

```ts
const API_BASE = import.meta.env.BASE_URL.replace(/\/$/, '')
fetch(`${API_BASE}/api/...`)
```

或者由前端运行时配置读取：

```text
window.__HAVLINE_BASE_PATH__
```

### 7.4 SSE / EventSource

当前：

```ts
new EventSource('/api/proxies/{id}/logs/stream')
```

目标：

```ts
new EventSource(`${API_BASE}/api/proxies/{id}/logs/stream`)
```

### 7.5 WebSocket

目标路径：

```text
/app/havline/ws
```

前端必须使用当前页面主机和协议：

```ts
const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
const wsUrl = `${wsProtocol}//${location.host}/app/havline/ws`
```

### 7.6 Cookie

需要确认 Havline 的会话 Cookie 是否设置了 Path。

建议：

```text
端口模式：Path=/
网关模式：Path=/app/havline
```

或者统一使用 `Path=/`，减少改造量。

### 7.7 CSRF / Origin

如果 Havline 有 Origin 或 CSRF 校验，需要把以下来源加入白名单：

```text
https://<fnOS域名>
https://<fnOS域名>/app/havline
```

## 八、认证与权限设计

### 8.1 第一阶段：双登录，低风险

```text
飞牛网关校验 fnOS 登录态
Havline 仍然显示自己的登录页
用户登录 Havline 后正常使用
```

优点：

```text
改造最小
不影响端口模式
不引入 SSO 安全问题
```

缺点：

```text
用户需要登录两次
```

建议首版采用。

### 8.2 第二阶段：网关 SSO

启用条件：

```text
HAVLINE_TRUST_GATEWAY_HEADERS=true
只有 Unix Socket 请求才信任 X-Trim-* Header
```

映射规则建议：

```text
X-Trim-Isadmin=true
  -> 映射到 Havline admin

X-Trim-Isadmin=false
  -> 默认拒绝管理界面
  -> 或后续映射为只读用户

缺少 X-Trim-Userid
  -> 视为非网关请求，回退到本地登录
```

安全要求：

```text
Unix Socket 文件权限限制为网关和 Havline 进程可访问
TCP 端口请求绝对不能信任 X-Trim-* Header
网关 Header 只用于身份上下文，不替代 Havline 业务权限
```

## 九、Havline 功能边界

统一网关只解决飞牛系统域名下的应用入口，不替代 Havline 的核心功能。

| Havline 功能 | 统一网关是否替代 | 说明 |
| --- | --- | --- |
| 管理界面入口 | 是 | 改为 `/app/havline` |
| fnOS 登录态 | 部分 | 网关校验登录，Havline 仍需业务鉴权 |
| 内置 Nginx 反向代理 | 否 | 仍由 Havline 管理 |
| DDNS | 否 | 仍由 Havline 管理 |
| HTTPS 证书申请 | 否 | 仍由 Havline 管理 |
| FRP 内网穿透 | 否 | 仍由 Havline 管理 |
| Cloudflare Tunnel | 否 | 仍由 Havline 管理 |
| 公网域名反代 | 否 | 仍由 Havline 管理 |
| 端口 18080/9443 | 否 | 作为 Havline 功能端口保留 |

不要把 Havline 的公网反代、证书和隧道功能迁入飞牛统一网关，否则会丢失
Havline 作为独立公网访问管理工具的能力。

## 十、生命周期与打包变更

### 10.1 安装回调

需要确保：

```text
${TRIM_APPDEST} 存在
${TRIM_APPDEST} 权限允许容器写入 app.sock
${TRIM_PKGVAR} 数据目录权限正确
```

### 10.2 启动

容器启动时：

```text
删除 ${TRIM_APPDEST}/app.sock 残留文件
创建新的 Unix Socket
设置 Socket 权限
```

### 10.3 升级

升级时：

```text
gatewayPrefix 保持不变
gatewaySocket 文件名保持不变
数据目录保持不变
Socket 文件重新创建
```

### 10.4 卸载

卸载时：

```text
删除 ${TRIM_APPDEST}/app.sock
默认保留 ${TRIM_PKGVAR}
```

## 十一、分阶段实施计划

### 阶段 0：官方行为 PoC

目标：用最小 Docker 示例验证统一网关。

必须验证：

```text
[ ] ${TRIM_APPDEST} 挂载路径
[ ] Socket 文件权限
[ ] /app/havline 请求是否保留前缀
[ ] Header 是否原样注入
[ ] WebSocket 是否可用
[ ] 应用重启后 Socket 是否自动重建
```

产出：

```text
docs/fnos-unified-gateway-poc.md
```

### 阶段 1：Havline 支持 Unix Socket

目标：主程序可以同时监听 TCP 和 Unix Socket。

改造：

```text
internal/config
internal/app
启动日志
停止清理
```

验收：

```text
[ ] TCP 6893 正常
[ ] Unix Socket 正常
[ ] Socket 权限正确
[ ] 容器重启后 Socket 重建
```

### 阶段 2：前端 base path

目标：Havline 可以在 `/app/havline` 下完整运行。

改造：

```text
Vite base
Vue Router base
API client base
EventSource base
WebSocket base
Cookie Path
CSRF / Origin
```

验收：

```text
[ ] 页面打开
[ ] 静态资源加载
[ ] 登录
[ ] API
[ ] SSE
[ ] WebSocket
[ ] 刷新深层路由
```

### 阶段 3：飞牛包切换统一网关入口

目标：桌面入口走 `/app/havline`。

改造：

```text
app/ui/config
docker-compose.yaml
```

保留：

```text
18080/9443 端口
端口回退入口
```

验收：

```text
[ ] 飞牛桌面入口打开正确
[ ] 不需要端口入口即可访问管理界面
[ ] 18080/9443 功能不受影响
```

### 阶段 4：网关 SSO

目标：使用 `X-Trim-*` 自动识别飞牛管理员。

改造：

```text
HAVLINE_TRUST_GATEWAY_HEADERS=true
只对 Unix Socket 请求生效
管理员映射
非管理员处理策略
```

验收：

```text
[ ] 管理员免登录
[ ] 非管理员被正确拒绝或只读
[ ] TCP 请求伪造 Header 无效
```

### 阶段 5：体验收敛

目标：统一网关成为默认入口，端口仅保留给 Havline 自身功能。

验收：

```text
[ ] 飞牛桌面入口只显示统一网关入口
[ ] 端口入口不再出现在应用卡片
[ ] 升级、卸载、重装正常
```

## 十二、验收清单

```text
[ ] gatewayPrefix 稳定为 /app/havline
[ ] gatewaySocket 稳定为 app.sock
[ ] ${TRIM_APPDEST} 正确挂载
[ ] Havline 监听 Unix Socket
[ ] Socket 启动前清理残留文件
[ ] Socket 权限满足网关访问
[ ] 前端使用 /app/havline base
[ ] API、SSE、WebSocket 路径正确
[ ] Cookie 和 CSRF 正常
[ ] 端口回退仍可用
[ ] 飞牛管理员可通过网关访问
[ ] 非管理员访问策略明确
[ ] 升级后数据不丢
[ ] 卸载默认保留数据
[ ] amd64 与 arm64 镜像可启动
```

## 十三、风险与未决问题

| 风险 | 影响 | 处理 |
| --- | --- | --- |
| 网关是否剥离 `/app/havline` 前缀未知 | 前端 base path 可能不一致 | 阶段 0 PoC 实测 |
| Socket 属主和权限未知 | 网关无法访问 Socket | PoC 验证并在启动时 chmod |
| 前端绝对路径较多 | 网关下页面或 API 404 | 统一引入 base path |
| SSE / WebSocket 路径 | 实时日志和状态页不可用 | 阶段 2 专项测试 |
| 双登录体验 | 用户需要登录两次 | 阶段 4 再做 SSO |
| 伪造 `X-Trim-*` Header | 越权风险 | 只信任 Unix Socket 请求 |
| 端口与网关并存 | 入口混乱 | 桌面只注册网关入口 |
| Havline 公网反代功能边界 | 用户误解为被替代 | 文档明确说明 |
| 飞牛审核要求 | 上架被拒 | 提交前复核官方审核项 |
| 镜像多架构兼容 | 飞牛 Docker 28.5.2 对 OCI index 兼容性有限 | 使用 Docker schema2 兼容格式 |

## 十四、推荐落地顺序

```text
1. 做最小统一网关 Docker PoC
2. 确认前缀、Socket 权限、Header、WebSocket
3. 主程序增加 Unix Socket 监听
4. 前端增加可配置 base path
5. 飞牛包注册统一网关入口
6. 保留端口回退
7. 实测通过后再做网关 SSO
8. 最后收敛桌面入口
```

## 十五、结论

Havline 不需要从 Docker 应用模板重写成另一种应用，而应在现有 Docker
应用上叠加统一网关访问模型。

推荐的最终形态：

```text
运行形态：飞牛 Docker 应用
访问模型：统一网关 /app/havline
服务监听：TCP 6893 + Unix Socket app.sock
用户体系：首版保留 Havline 登录，后续再做网关 SSO
功能边界：Havline 继续管理公网反代、DDNS、证书、FRP 和 Cloudflare Tunnel
回退能力：保留 18080/9443 端口
```

这样既能获得飞牛系统统一的 HTTPS 域名、登录态和桌面入口体验，又不会
破坏 Havline 作为独立 NAS 公网访问管理工具的核心能力。
