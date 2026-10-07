# 场景 A 可行性分析与操作步骤：多 Web 应用统一入口

> 日期：2026-10-07
> 场景：家庭宽带 NAS，多个 Web 应用统一 443 对外，VPS 做公网入口中转，TLS 在 NAS 本地终止
> 性质：只做分析与步骤说明，不含代码改动
> 结论：**整体可行**，但场景描述里有 3 处前提需要修正，否则其中一步会走不通

## 一、目标链路

```
用户 → https://app.example.com:443
     → DNS 解析到 VPS IP
     → VPS:443（frps 监听，由 tcp 规则提供）
     → FRP TCP 隧道（remotePort 443）
     → NAS frpc → 127.0.0.1:443
     → Havline 本地 Nginx（TLS 在此终止，按 server_name 分流）
     → 内网应用
```

这条链路的关键特征：**TLS 全程到 NAS 才解开**，VPS 只做 TCP 转发，看不到明文。相比「VPS 上终止 TLS」的方案，这是更安全的一种，值得保留。

## 二、先修正场景描述里的三处前提

### 前提 1：`http://` 跳转 `https://` 这一步走不通

原文第 6 步写「按域名建规则（HTTPS + 强制跳转 + 选证书）」。问题在于：浏览器访问 `http://app.example.com` 走的是 **80 端口**，而本场景只把 **443** 通过隧道引到 NAS，80 的流量根本到不了本地 Nginx。

再看 Havline 的实现，`http_redirect` 的语义是「在**同一个监听端口**上额外生成一个 301 server 块」：

```go
// internal/nginx/generate.go
if rule.HTTPRedirect && useHTTPS {
    b.WriteString("    return 301 https://$host:$server_port$request_uri;\n")
}
```

它服务于「同一端口既能收明文又能收 TLS」的场合，无法覆盖 80 端口。如果为了跳转把规则的监听端口设成 80 并开启 HTTPS，生成的会是 `listen 80 ssl;`，属于错误配置。

**处理方式（三选一）**：

1. **推荐**：在 VPS 层做跳转——VPS 上已有 Nginx 时，加一个 `listen 80; server_name app.example.com; return 301 https://$host$request_uri;`。这与隧道无关，最简单。
2. 额外开一条 tcp 隧道（remotePort 80 → 本地 80），再用自定义全局配置在本地 Nginx 手工加跳转。
3. 接受「只有 https 可访问」，不做跳转。

因此在第 6 步里，`HTTP 跳转 HTTPS` 开关不要打开。

### 前提 2：443 只能作为 TCP 规则的远程端口，不能填进 frps 的 vhost 端口

场景第 1 步写「放行 bindPort 与 443」，这是对的；但要避免一个常见混淆：Havline 在生成 frps.toml 时**明确禁止** `vhostHTTPPort` / `vhostHTTPSPort` 使用 80/443，会直接报错：

> frps vhost 回源端口不能使用 80/443（通常由 VPS 上的 Nginx 占用）；请在服务端表单改为 8080/8443 后重新生成配置

原因：vhost 端口是给「http/https 类型规则 + VPS Nginx 反代」用的，属于另一条技术路线。**本场景用的是 `tcp` 类型规则**，443 是规则的 remotePort，两者互不冲突。

### 前提 3：DDNS 解析到家庭宽带 IP 并不总是可行

原文最后一段写「域名也可解析到家庭宽带 IP（DDNS 留空自动检测），VPS 仅做隧道；两种都可行」。这条要加前提：

1. 家宽必须是**真正的公网 IPv4**（非 CGNAT 大内网），否则外部无法直连到家庭 IP；
2. 国内运营商普遍**封锁家宽的 80/443 入站**，即使有公网 IP，443 也可能不通。

只要这两条有一条不满足，「解析到家庭 IP」就等于解析到一个不可用的地址，链路会静默失败。相比之下，**解析到 VPS IP 是更稳的选择**，也是本场景的推荐形态。

## 三、项目里的功能支撑（已逐项核对）

| 场景需要 | 项目实现 | 位置 |
| --- | --- | --- |
| 生成/下发 frps.toml | `FRPSServerConfig`（脱敏预览）+ `PushFRPSConfigToAgent`（下发） | [frps_template.go](F:/Fonu-main/havline/internal/frp/frps_template.go)、服务端「详情 → frps.toml」 |
| DDNS 自定义 IP（指向 VPS） | 自定义 IPv4/IPv6，留空则自动检测本机公网 IP | [DdnsView.vue](F:/Fonu-main/havline/web/src/views/DdnsView.vue:451) |
| 通配符证书 DNS-01 | lego `dns01`，支持 Cloudflare / DNSPod / 阿里云 / 腾讯云 / 火山 | [dns_provider.go](F:/Fonu-main/havline/internal/acme/dns_provider.go:16) |
| DNS-01 凭证复用 DDNS | **自动复用**，无需重填 | [service.go](F:/Fonu-main/havline/internal/acme/service.go:126) `ddnsSvc.ConfigForDNSZone` |
| FRP TCP 规则 | 类型 tcp + 远程端口 + 内网目标 | FrpView 规则表单 |
| 反代规则监听 443 + HTTPS + 出口 | 监听端口 / 出口多选 / 启用 HTTPS | [ProxyView.vue](F:/Fonu-main/havline/web/src/views/ProxyView.vue:459) |
| 证书自动绑定规则 | 按域名自动匹配，**通配符优先于精确域名** | [generate.go](F:/Fonu-main/havline/internal/nginx/generate.go:249) `findCertificate` |
| 校验并应用 | frpc `verify` 通过才落盘 | [manager.go](F:/Fonu-main/havline/internal/frp/manager.go:41) |

两点值得注意的细节：

1. **证书不需要在反代规则里逐个选**。生成器按域名找证书，顺序是先试 `*.example.com` 再试 `example.com`。所以只要申请了通配符证书，之后新增子域名规则会自动套用。
2. **DNS-01 凭证确实复用 DDNS**：`prepareApplyByZone` 先取 DDNS 配置，再由 `dnsCredentialsForDomain` 拿到 `ddns.Credentials` 交给 `newDNS01Provider`。前提是申请的域名必须落在 DDNS 已托管的 zone 内，否则会触发 `DomainsUnderZones` 校验失败。

## 四、操作步骤

### 步骤 1：VPS 侧准备

1. 确认 443 空闲：`ss -lntp | grep ':443 '`。若 VPS 上已有 Nginx 占用 443，需要先停掉或改端口，否则 frps 的 tcp 规则绑定失败。
2. 放行端口：`bindPort`（默认 7000）与 **443**。
3. 如果 frps.toml 配了 `allowPorts` 白名单，务必把 443 加进去。
4. 生成 frps 配置：Havline →「公网服务端」→ 选中该 VPS →「frps.toml」查看同源配置，用「下发 frps 配置」写入 VPS，或在 VPS 上手工放置。
5. 启动 frps，确认控制端口可连。

### 步骤 2：DDNS 指向 VPS

1. 「域名解析」页 → 新建任务 → 服务商选你的 DNS 服务商，填入凭证。
2. 记录填写完整域名：`example.com`、`*.example.com`（通配符便于后续新增子域）。
3. 点该任务的「自定义 IP」→ IPv4 填 **VPS 的 IP**（这一步很关键，不填的话 Havline 会去检测 NAS 自己的公网 IP）。
4. 立即同步一次，确认解析生效：`nslookup app.example.com` 返回 VPS IP。

### 步骤 3：申请通配符证书

1. 「证书」页 → 申请 → CA 选 Let's Encrypt → 校验方式为 **DNS-01**。
2. 域名填 `example.com` 与 `*.example.com`（一起提交，避免重复申请）。
3. 凭证无需填写，会自动复用步骤 2 里 DDNS 的凭证。
4. 申请成功后确认通配符证书落在 `wildcard.example.com` 目录下。

### 步骤 4：添加 FRP 服务端

1. 「公网服务端」→ 添加服务端。
2. 主机地址填 VPS IP，端口 7000，Token 与 frps 一致。
3. 若要用本页的「安装 Agent / 下发 frps 配置」能力，同时填好 Agent 地址与 SSH 信息。

### 步骤 5：添加 FRP TCP 规则

1. 「公网服务端」→ 选中该服务端 → 新建规则。
2. 类型 `tcp`；内网地址 `127.0.0.1`；内网端口 `443`；公网远程端口 `443`。
3. 命名建议用真实用途（例如 `nas-https-entry`），不要沿用 `ssh-tcp` 这类与实际不符的名字。
4. 启动 frpc，日志应出现该规则的 `start proxy success`。

### 步骤 6：建立反向代理规则

**入口更正**：这一步在「**反向代理**」页（路由 `/proxies`），不是「公网反代」页。「公网反代」（`/public-proxy`）管理的是部署到 **VPS 上的 Nginx 反代**，属于「VPS 出口」路线（TLS 在 VPS 终止）；本场景要的是 NAS 本地 Nginx，必须用「反向代理」页。

页面右上角「新建规则」→ 弹窗有三个页签：**基础配置 / 安全设置 / Nginx**。

#### 基础配置页签逐字段填写

| 字段 | 填写值 | 说明 |
| --- | --- | --- |
| 名称 | `NAS 统一入口` | 选填，建议填，便于列表识别 |
| 前端域名 | `app.example.com` | 必填；每行一个；**不要带端口** |
| 监听端口 | `443` | 必填；**默认值是 80，必须改成 443** |
| 监听协议 | 只勾 `IPv4` | 默认即 IPv4；除非 VPS 侧走 IPv6 否则不必勾 IPv6 |
| 目标地址 | `http://127.0.0.1:8080` | 换成实际应用地址 |
| 出口 | 只勾「本机 Nginx」 | **不要勾 Cloudflare 隧道** |
| 启用 HTTPS | 打开 | 默认已开 |
| HTTP 跳转 HTTPS | **关闭** | **默认是打开的，必须手动关**，原因见前提 1 |
| 启用规则 | 打开 | 默认已开 |

证书不需要选，生成器按域名自动匹配步骤 3 的通配符证书（优先 `*.example.com`）。

#### 两个默认值陷阱

1. `监听端口` 默认是 **80**，不改的话会以 root 权限在 NAS 上监听 80，与预期不符。
2. `HTTP 跳转 HTTPS` 默认是 **开**。打开后会在 443 端口上额外生成一个明文 301 server 块——对公网访问没有实际作用，反而让配置语义混乱，因此要关掉。

#### 安全设置页签（可选）

1. 「一键推荐」会填入：HTTPS + 请求限流（10 r/s，burst 20）+ 安全响应头。**注意它会把 `HTTP 跳转 HTTPS` 重新打开**，点完要回基础配置再关掉。
2. **不要开启「仅中国大陆 IP」**：TCP 隧道下客户端真实 IP 已丢失（见 5.1），该策略会失效或误判。
3. 需要额外保护时可开 Basic Auth；「高级 → 仅 TLS 1.3」按需。

#### Nginx 页签

保持默认的生成模式即可，不需要手工编辑。

#### 保存与生效

1. 点保存。保存前会做「域名 + 端口」冲突检查，若提示 `域名 x:443 已被规则「y」使用`，说明该端口已被占用。
2. 保存成功后后端会自动同步 Nginx 配置并重载；若返回 `sync_warning`，表示规则已写入但 Nginx 同步失败，需要到日志页排查。
3. 若 NAS 本地 443 已被其他服务占用，重载会失败——先用 `ss -lntp | grep ':443 '` 确认。

### 步骤 7：验证

1. frpc 日志：`start proxy success`，无 `start error`。
2. VPS 上确认 443 已被 frps 监听：`ss -lntp | grep ':443 '`。
3. 外网探测 TCP：

```powershell
1..5 | % { (Measure-Command { $c = New-Object Net.Sockets.TcpClient; $c.Connect('VPS_IP', 443); $c.Close() }).TotalMilliseconds }
```

4. 端到端验证（这一步才说明链路通）：

```powershell
curl.exe -v --resolve app.example.com:443:VPS_IP https://app.example.com/
```

5. 检查返回的证书是否为通配符证书（`curl -v` 输出中 subject 应为 `*.example.com`）。
6. Havline「公网服务端 → 详情」看诊断与可用率；日志页看该服务端运行日志。

## 五、已知限制

### 5.1 客户端真实 IP 会丢失

TCP 隧道下，NAS 本地 Nginx 看到的 `$remote_addr` 是 frpc 来源地址（回环），不是访客真实 IP。影响两处：

1. Nginx 访问日志里的 IP 全部失真；
2. Havline 的「中国 IP 段」访问控制（按来源 IP 放行/拦截）会失效——所有请求看起来都来自内网。

若要保留真实 IP，需要 FRP 规则开启 `transport.proxyProtocolVersion = "v2"`，同时本地 Nginx 的监听加 `proxy_protocol` 并配合 realip 模块。这属于可选增强，需要两端同时改。

### 5.2 NAS 本地 443 的占用

Havline 要在 NAS 上监听 443。如果 fnOS 系统面板或其他容器已经占用 443，需要先让出端口。可用 `ss -lntp | grep ':443 '` 确认。

### 5.3 「本机出口」与「公网反代（VPS 出口）」是两条不同路线

1. 本场景用的是**本机出口**：配置生成在 NAS 本地 Nginx，TLS 在 NAS 终止。
2. 「公网反代」里另一种形态是 **VPS 出口**：frps 开 vhost 端口（8080/8443），VPS 上 Nginx 反代回源，TLS 在 VPS 终止。

两者不要混用同一条规则，否则会出现「证书在 NAS、TLS 却在 VPS 终止」的错配。判断方法：看规则表单里「出口」勾了哪些。

## 六、扩展性

这套结构的好处正如场景描述所说：**新增应用只改反代，不动 FRP**。

1. 新建反代规则，监听端口仍填 443，域名换 `app2.example.com`，上游指向新应用；
2. 生成器自动套用同一张通配符证书；
3. 不需要新增 FRP 规则、不需要改 frps、不需要动 DNS（通配符记录已覆盖）。

唯一需要留意的是：所有子域名共用同一条 443 隧道，因此 **NAS 本机 Nginx 是单点**——它挂了所有应用一起不可用。这是该架构的固有取舍，不是配置错误。
