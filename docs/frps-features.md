# frps（服务端）功能实现清单

> 基于 frp v0.71.0 官方 `frps_full_example.toml` 全量对照。
> 状态图例：✅ 已实现　🔶 部分实现（有缺口或需手动步骤）　⬜ 未实现　🚫 有意不实现（有说明理由）
>
> 「Fonu 入口」= 用户在 Fonu 界面操作的位置；「生成器」= `internal/frp/frps_template.go` 的 frps.toml 同源生成器。
> 更新日期：2026-09-21

---

## 一、网络与监听（[common] 顶层键）

| 配置项 | 官方默认 | 状态 | 说明 |
| --- | --- | --- | --- |
| `bindAddr` | `0.0.0.0` | ✅ | 固定生成 `0.0.0.0` |
| `bindPort` | `7000` | ✅ | 取服务端表单「端口」，与 frpc 同源 |
| `kcpBindPort` | 0（禁用） | ✅ | 表单「协议」选 kcp 时生成 |
| `quicBindPort` | 0（禁用） | ✅ | 表单「协议」选 quic 时生成 |
| `proxyBindAddr` | 同 bindAddr | ⬜ | 未提供（UDP/HTTP 代理单独绑定地址的场景才需要） |
| `vhostHTTPPort` | 0（禁用） | ✅ | 有 http/https 规则时生成；**表单「远程管理 → frps vhost 反代端口」可自定义**，默认 8080 |
| `vhostHTTPSPort` | 0（禁用） | ✅ | 同上，默认 8443 |
| `vhostHTTPTimeout` | 60 | ⬜ | 未提供（vhost 响应超时，一般无需改） |
| `tcpmuxHTTPConnectPort` | 0（禁用） | ⬜ | tcpmux 需要它（存在 tcpmux 规则时生成器只提示 vhost，未生成此键） |
| `tcpmuxPassThrough` | false | ⬜ | 未提供 |

## 二、认证（auth.*）

| 配置项 | 官方默认 | 状态 | 说明 |
| --- | --- | --- | --- |
| `auth.method`（token） | `token` | ✅ | 与 frpc 同源；Token 加密存储，生成时解密写入 |
| `auth.token` | — | ✅ | 同上 |
| `auth.method`（none） | — | ✅ | 生成安全提示注释 |
| `auth.oidc.*`（issuer/clientID/clientSecret/audience/scope/tokenEndpointURL 等） | — | 🔶 | frpc 侧 OIDC 表单已支持；frps 侧校验参数依赖 IdP，生成器只输出注释指引手动配置 |

## 三、TLS（transport.tls.*）

| 配置项 | 官方默认 | 状态 | 说明 |
| --- | --- | --- | --- |
| `transport.tls.force` | false | 🔶 | frpc 侧未启用 TLS 时生成**注释**建议开启；无表单开关直接生成 |
| `transport.tls.certFile / keyFile` | — | ⬜ | frps 侧 TLS 证书未提供（frpc 侧证书表单已有，frps 反向 TLS 需另加） |
| `transport.tls.trustedCaFile` | — | ⬜ | 未提供 |

## 四、管理面板（webServer.*）

| 配置项 | 官方默认 | 状态 | 说明 |
| --- | --- | --- | --- |
| `webServer.addr / port` | `127.0.0.1:0` | ✅ | 表单「frps 管理接口」地址/端口；Fonu 借此拉流量；「frps 管理」按钮直达面板 |
| `webServer.user / password` | — | ✅ | 表单同源；密码加密存储（ConfiguredSecretField 掩码闭环） |
| `webServer.tls.certFile / keyFile` | — | ⬜ | dashboard 自身 HTTPS 未提供 |

## 五、HTTP / 子域名（vhost 语义）

| 配置项 | 官方默认 | 状态 | 说明 |
| --- | --- | --- | --- |
| `subDomainHost` | "" | 🔶 | **缺口检测已实现**：规则填了 subdomain 时生成器输出注释提醒必须配置；但表单无字段、不能直接生成 |
| `custom404Page` | — | ⬜ | 未提供 |
| `httpPlugins`（server 端） | — | ⬜ | frps 端 HTTP 插件未提供（frpc 端 8 种插件表单已支持，两者是不同层的功能） |

## 六、限流与配额

| 配置项 | 官方默认 | 状态 | 说明 |
| --- | --- | --- | --- |
| `allowPorts`（端口白名单/范围） | 不限 | ⬜ | 未提供 |
| `maxPortsPerClient` | 0（不限） | ⬜ | 未提供 |
| `userConnTimeout` | 10 | ⬜ | 未提供 |

## 七、传输调优（transport.* 其余）

| 配置项 | 官方默认 | 状态 | 说明 |
| --- | --- | --- | --- |
| `transport.tcpMux` | true | ✅ | 与 frpc 同源；仅关闭时输出 |
| `transport.tcpMuxKeepaliveInterval` | 30 | ✅ | 表单「高级配置」同源（frpc 侧）；frps 生成器未输出该键，两端不一致时 TCPMux keepalive 各用默认值——**低风险缺口** |
| `transport.maxPoolCount` | 0（按 frpc） | ✅ | 有 pool_count 时输出 |
| `transport.heartbeatInterval / timeout` | 30/90 | ✅ | frpc 侧表单同源；frps 生成器不输出（服务端跟随客户端心跳，官方无需配置） |
| `transport.tls.force` | — | 🔶 | 见第三节 |

## 八、运行与日志

| 配置项 | 官方默认 | 状态 | 说明 |
| --- | --- | --- | --- |
| `log.to / level / maxDays` | console | ⬜ | frps 日志走 journalctl（agent 采集尾部显示），未提供文件日志配置生成 |
| `log.disablePrintColor` | false | ⬜ | 未提供 |
| `detailedErrorsToClient` | true | ⬜ | 未提供 |
| `printStats`（每 30s） | — | ⬜ | 未提供 |

## 九、用户隔离与资产

| 配置项 | 官方默认 | 状态 | 说明 |
| --- | --- | --- | --- |
| 多用户 `user` 前缀隔离 | — | ⬜ | 未提供（frps 端 `user` 是 frpc 登录名前缀隔离，与 Fonu 的服务端 user 概念不同） |
| `udpPacketSize` | 1500 | ⬜ | 未提供（需与 frpc 同源，frpc 侧也无此表单项） |

## 十、Fonu Agent 侧的 frps 运维能力（与配置生成配套）

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| 一键安装 / 升级（SSH） | ✅ | 原子替换 + `-version` 校验，失败不伪报成功 |
| systemd 托管（frps.service） | ✅ | 坏 unit 自动接管重写 |
| 状态可视（进程/端口/版本/日志尾/dashboard 地址） | ✅ | 「公网 Agent」页 |
| 启动 / 停止 / 重启 | ✅ | 含 `ok:false` 失败契约与 journal 透出 |
| 配置下发（同源 frps.toml） | ✅ | 「下发 frps 配置」按钮 |
| 官方版本安装 / 升级（下拉选择） | ✅ | 含下载代理 |
| 证书下发（HTTPS 规则） | ✅ | `/api/v1/certs` |
| **公网 Nginx 反代手动部署 / 移除** | ✅ | 试探-验证式目录自动发现（`-T` 加载验证），规则保存与反代解耦 |
| **vhost 端口自定义** | ✅ | 表单 → frps.toml → Nginx upstream 三方同源 |

---

## 优先级建议（下一步最值得补的）

1. **`subDomainHost` 表单字段**（🔶）：subdomain 功能目前只能靠手动改 frps.toml，是 vhost 体系里最后一块手动拼图；配合 `custom404Page` 一起做收益最高；
2. **`transport.tls.force` 开关**（🔶）：一条注释提示不如一个开关——公网 frps 的标准安全实践；
3. **`allowPorts` / `maxPortsPerClient`**（⬜）：多用户/防滥用场景需要；单用户自用优先级低；
4. **`tcpmuxHTTPConnectPort`**（⬜）：存在 tcpmux 规则时的硬依赖，用到 tcpmux 的话必须补；
5. **frps 日志配置**（⬜）：journalctl 尾部只能看 6 行，排障时不够；生成 `log.to = "/var/log/frps.log"` 即可接入现有日志查看。

## 附：架构口径

- **frps.toml 是生成器产物，不是用户手写文件**：所有已实现项均以 Fonu 服务端表单为唯一事实源，VPS 侧禁止手改（改了会被下次「下发 frps 配置」覆盖）；
- **端口同源链**：表单 `vhost_http_port/vhost_https_port` → frps.toml `vhostHTTPPort/vhostHTTPSPort` → Nginx `proxy_pass 127.0.0.1:<port>`，三方由 `agentUpstreamFor`/生成器共用同一值；
- **agent 与 frps 解耦**：agent 只管 Nginx 片段/证书/frps 生命周期，frps 进程由 systemd 托管；二者通过 Fonu 下发配置保持一致。
