# 内网应用经 FRP 暴露公网：NAS 侧完整操作手册

> 场景：把 NAS 内网应用（本例 `192.168.31.152:7023`）通过一台公网 VPS（`38.95.74.87`）以域名（`tv.36800.cc`）暴露出去，URL 不带端口号，且后续域名 / HTTPS / 访问控制全部在 NAS 的 Havline 网页里管理，VPS 只做纯流量转发。
> 适用版本：Havline v1.1.0+（FRP 多服务端模块）。VPS 上 **不需要也不应该** 装 Nginx 反代。

## 0. 最终架构与访问地址

```text
用户浏览器
   ↓ http://tv.36800.cc（80 端口，URL 不带端口）
VPS frps（监听 80 —— 由 frpc 动态注册的 tcp 隧道，VPS 上无需 Nginx）
   ↓ FRP 隧道（bindPort 7000，TLS）
NAS frpc
   ↓ 127.0.0.1:18080
Havline Nginx（按域名分流，后续可加 HTTPS/黑白名单）
   ↓ http://192.168.31.152:7023
内网应用
```

**访问地址：`http://tv.36800.cc`**

| 角色 | 地址 | 说明 |
| --- | --- | --- |
| 公网 VPS | `38.95.74.87`（tv.36800.cc） | 只跑 frps，Nginx 停用 |
| NAS 内网应用 | `192.168.31.152:7023` | 真正的 Web 服务 |
| Havline 管理后台 | `http://<NAS-IP>:6893` | 全部配置入口 |

## 1. 前置条件

- VPS：已部署 frps（bindPort 7000、token 认证；参考 Havline 服务端详情「frps.toml」一键生成）；
- VPS：frps 的 Nginx **未**占用 80（占用见第 5 节排障）；
- 云厂商安全组已放行：7000/tcp（frps 控制）、80/tcp（对外）、7500/tcp（frps 管理接口，可选，用于流量统计）；
- NAS：Havline 已部署并能打开管理后台；
- 域名：`tv.36800.cc` 的 DNS A 记录指向 VPS IP（可用 Havline DDNS 页「自定义 IP」固定，或由 DNS 服务商直接管理）。

## 2. Havline 网页配置（三处）

### 2.1 FRP 服务端

路径：**FRP 穿透 → 添加服务端**

| 字段 | 值 |
| --- | --- |
| 主机地址 | VPS 的 IP 或域名 |
| 端口 | 7000（= frps bindPort） |
| 认证方式 | Token，与 VPS frps.toml 的 `auth.token` 一致 |
| TLS | 开启（frps 默认接受） |
| frps 管理接口（可选） | 填 frps `webServer` 的地址/端口/用户/密码，用于显示穿透流量 |

保存后状态应显示「运行中 · 已连接」。

### 2.2 穿透规则（核心）

路径：**FRP 穿透 → 添加规则**

| 字段 | 值 | 说明 |
| --- | --- | --- |
| 所属服务端 | 上一步的服务端 | |
| 名称 | `tv`（frps 全局唯一，出现在日志里） | |
| 类型 | **tcp** | ⚠️ 不选 http——80 直接由 frps 转发，域名分流交给 Havline 反代 |
| 远程端口 | **80** | 选 tcp 类型后出现此字段；frpc 登录时向 frps 动态注册 |
| 内网目标 | **127.0.0.1:18080** | 指向 Havline 自己的 Nginx HTTP 监听（fnOS host 模式默认 18080） |

保存 → 右上 **「应用配置」→「校验并应用配置」**，或对该服务端「停止→启动」。

### 2.3 反向代理（域名分流）

路径：**反向代理 → 添加规则**

| 字段 | 值 |
| --- | --- |
| 前端域名 | `tv.36800.cc` |
| 监听端口 | **18080**（与 2.2 的内网目标端口一致；fnOS 下不要用 80，系统 Nginx 已占用） |
| 目标地址 | `http://192.168.31.152:7023`（⚠️ 指向真正的内网应用，完整 URL 带协议与端口） |
| HTTPS | 按需；fnOS 下 Havline Nginx 的 HTTPS 监听默认 9443 |

> ⚠️ **回环警告**：目标地址千万不要填 `http://tv.36800.cc:xxxx`——那会让请求从 NAS 出公网再回 NAS（NAS→VPS→NAS 绕圈）。反代目标是**内网应用本身**。
> ⚠️ **「目标地址缺少主机名」报错**：目标地址格式必须完整 `http://IP:端口`（协议+主机+端口缺一不可，单斜杠 `http:/` 或中文冒号都会报 400）。

## 3. VPS 侧配置（一次性的两步）

### 3.1 frps.toml（Havline 一键生成）

Havline 服务端「详情」→「frps.toml」→ 复制内容到 VPS 保存为 `frps.toml`：

```toml
bindAddr = "0.0.0.0"
bindPort = 7000

auth.method = "token"
auth.token = "<与 Havline 一致>"

webServer.addr = "0.0.0.0"     # 可选：Havline 拉取穿透流量
webServer.port = 7500
webServer.user = "admin"
webServer.password = "<自定义>"

log.to = "console"
log.level = "info"
log.maxDays = 3
log.disablePrintColor = false
```

> 注意：**这个文件里没有任何远程端口**。tcp 类型规则的远程端口（80）由 frpc 登录时动态注册，frps 只管 bindPort 和认证——规则改了也不用改这个文件。

启动：`./frps -c frps.toml`（建议配 systemd）。

### 3.2 释放 80 端口（关键，`port unavailable` 的唯一解法）

80 只能给一个进程。frps 要绑 80，VPS 上原有的 Nginx 必须停掉：

```bash
# 确认 80 被谁占用
ss -tlnp | grep ':80 '

# 停止并禁用 Nginx（宝塔面板安装的在宝塔「软件商店」里停）
systemctl stop nginx
systemctl disable nginx

# 确认已释放（应无输出）
ss -tlnp | grep ':80 '
```

frpc 约每 33 秒自动重试注册，释放后无需任何手动操作，注册成功后 frps 日志出现 proxy 启动记录、frpc 日志的 `start error: port unavailable` 消失。

> 如果 VPS 的 80/443 还要跑其他站点（Nginx 不能停），本方案不适用——改回「VPS Nginx 监听 80 → proxy_pass 127.0.0.1:8080（frps vhost）」的方案，FRP 规则用 http 类型，见第 6 节方案对比。

## 4. 验证清单

| 步骤 | 命令 / 操作 | 预期 |
| --- | --- | --- |
| VPS 80 已监听 | `ss -tlnp \| grep ':80 '` | 进程为 frps |
| frpc 注册成功 | Havline 服务端「日志」 | 无 `port unavailable`，有 proxy 启动记录 |
| 隧道通 | Havline 服务端「测速」/「详情」诊断 | TCP 可达、frpc 登录 ok、延迟正常 |
| 分流通 | 服务端「详情」→ 诊断 `proxy_reachable` | ok |
| 端到端 | 外网浏览器 `http://tv.36800.cc` | 直达内网应用页面 |

## 5. 故障排查（按链路顺序）

| 现象 | 断点 | 处理 |
| --- | --- | --- |
| `Connection refused`（外部 curl 80） | **VPS 侧**：frps 没绑上 80 | ① `ss -tlnp \| grep ':80 '` 看 80 被谁占（多为 Nginx 没停）→ 停用；② `systemctl status frps` 确认在跑且用的新配置（nohup 手动跑的必须 `pkill frps` 重启）；③ 重启后等 frpc 自动重试 |
| frpc 日志 `start error: port unavailable` | 同上 | VPS 80 被占；见 3.2 |
| 诊断「frpc 登录」失败 | Token/端口不一致 | 两端 token 对齐；端口=frps bindPort |
| 登录成功但 `proxy_reachable` 失败 | Havline 反代 | 反代规则监听端口与 FRP 内网目标端口一致（18080）；反代已启用 |
| 打开域名 404 | 域名分流失败 | 反代规则的前端域名与访问域名完全一致；frps 无需 vhost（tcp 方案） |
| 域名解析不对 | DNS | `nslookup tv.36800.cc` 应返回 VPS IP；用 DDNS 自定义 IP 固定 |
| 流量统计全 0 | frps webServer 未配 | 非必须；要在 Havline 看流量就配 webServer 并在服务端表单填管理接口 |

## 6. 两种方案对比（VPS 80 的归属）

| | 方案 A：NAS 侧全管（本文） | 方案 B：VPS Nginx 反代 |
| --- | --- | --- |
| FRP 规则类型 | tcp，远程端口 80 → 127.0.0.1:18080 | http 类型 + frps `vhostHTTPPort=8080` |
| http 类型去端口 | —（tcp 方案直接占 80） | `vhostHTTPPort` 从 8080 改 **80**：URL 变 `http://域名`（无端口）；前提同样是 VPS 停 Nginx 释放 80，改后 `pkill frps` 重启生效；NAS 侧规则无需动 |
| VPS Nginx | 不需要（停用） | 必须：listen 80 → proxy_pass 127.0.0.1:8080（**必须带** `proxy_set_header Host $host`） |
| 加新域名 | Havline 反代加一条 + FRP 加一条 tcp 规则（远程端口仍 80，内网目标同 18080） | VPS 上加 server 块 |
| 上 HTTPS | Havline 证书页（DNS-01）+ 反代开 HTTPS + FRP 加 tcp 443→127.0.0.1:9443 | VPS 上装证书 |
| 访问控制（IP/BasicAuth） | Havline 反代页直接配 | VPS 上手写 |
| 限制 | VPS 80/443 不能再跑别的站点 | VPS 可共存多站点 |
| URL | `http://tv.36800.cc`（无端口） | `http://tv.36800.cc`（80）或带 vhost 端口 |

选型：VPS 专用做穿透入口 → 方案 A；VPS 80/443 另有他用 → 方案 B。两方案不混用（80 归属唯一）。

## 7. HTTPS 升级路径（方案 A 延伸）

1. **Havline 证书页**：申请 `tv.36800.cc` 或 `*.36800.cc` 证书（ACME DNS-01，复用 DDNS 页服务商凭证）；
2. **反代规则**：开 HTTPS、绑定证书、（可选）HTTP 强制跳转；
3. **FRP 加一条 tcp 规则**：远程端口 443 → 内网目标 `127.0.0.1:9443`（Havline Nginx HTTPS 监听）；
4. 安全组放行 443；之后 `https://tv.36800.cc` 可用，TLS 全程在 NAS 侧终止。
