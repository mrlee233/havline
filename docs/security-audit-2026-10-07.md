# Havline 安全审查与代码审查报告

> 审查日期：2026-10-07
> 审查对象：Havline 主仓（`F:\Fonu-main\havline`），版本 `1.9.2`
> 审查方式：定向安全面扫描 + 关键路径人工复核 + 自动化检查

## 一、审查范围与方法

代码规模：

| 部分 | 文件数 | 约行数 |
| --- | --- | --- |
| Go 后端（`internal/`、`cmd/`） | 292 | 40,173 |
| 前端 Vue/TS（`web/src`） | 61 | 23,538 |

覆盖的安全面：

1. 命令执行（`os/exec` 全量枚举，回溯参数来源）
2. SQL 构造（拼接式查询扫描）
3. 路径处理（`filepath.Join` 与用户输入的交汇点）
4. 认证与授权（路由注册、中间件、会话、令牌）
5. 加密与密钥（TLS 配置、证书、令牌生成）
6. 出站请求（SSRF 面）
7. 文件解压与导入（tar/zip slip）
8. 前端注入与凭据存储
9. 日志与备份的信息泄漏链

自动化检查结果：

```
go vet ./...                通过（无输出）
npm run typecheck           通过（havline-web@1.9.2）
gosec / govulncheck / staticcheck   本机未安装，未执行
```

`gosec`、`govulncheck` 未安装，依赖漏洞与规则化静态扫描属于本次未覆盖范围，建议在 CI 中补上。

## 二、总体结论

整体安全基线扎实，不是"到处漏风"的状态。认证、路径安全、解压安全和 agent 暴露面这几处明显经过专门设计，多处还能看到针对具体攻击手法的注释（例如备份解压用 `filepath.Rel` 而非前缀比较，理由写在代码里）。

本次共确认 1 个高风险、4 个中风险、6 个低风险问题，均不属于"未授权访问"级别的直接可利用漏洞。高风险项是公网部署下的认证入口缺口，建议优先处理。

> 修订说明（2026-10-07）：初版把「Agent 远程监听为明文 HTTP」列为 P1-2。复核后确认该结论有误——项目已实现 SSH 隧道与 HTTPS 证书指纹固定两种安全传输模式，`transport_security` 字段只描述 Agent 本机监听地址的安全等级，与 Havline→Agent 的连接方式无关。该条已改写为 P3-6。

## 三、漏洞与风险清单

### P1 高风险

#### P1-1 登录接口无速率限制与失败锁定

位置：[auth_handler.go](F:/Fonu-main/havline/internal/api/auth_handler.go) 的 `Login`、[auth.go](F:/Fonu-main/havline/internal/auth/auth.go:120) 的 `Service.Login`

`Login` 对密码错误直接返回 `ErrUnauthorized`，没有失败计数、没有按 IP 或用户名限流、没有临时锁定，也没有写入安全审计日志。扫描 `internal/api/auth_handler.go` 与 `internal/auth/auth.go` 全文件，未发现任何 `rate` / `limit` / `attempt` / `lockout` / `backoff` 逻辑。

影响：Havline 面板常被端口映射到公网。攻击者可以对 `admin` 账号持续爆破。`bcrypt`（DefaultCost）把单次尝试成本抬到约几十毫秒，显著提高了爆破代价，但这只是延缓，不构成阻断；分布式来源的尝试更不受影响。

建议：按「用户名 + 客户端 IP」做令牌桶限流，失败累积到阈值后指数退避并短期锁定；成功登录后清零计数；失败事件写入系统日志便于发现。

#### P1-2（已降级为 P3-6）

初版结论「Agent 远程监听为明文 HTTP」经复核不成立，已改写为 P3-6，原因见该条。此处保留占位，便于与初版对照。

### P2 中风险

#### P2-1 证书域名校验过弱，与其它模块不一致

位置：[import.go](F:/Fonu-main/havline/internal/certificate/import.go:104) `validateDomainLabel`

```go
func validateDomainLabel(domain string) error {
	if domain == "" {
		return fmt.Errorf("域名格式无效")
	}
	parts := strings.Split(domain, ".")
	for _, part := range parts {
		if part == "" || len(part) > 63 {
			return fmt.Errorf("域名格式无效")
		}
	}
	return nil
}
```

只检查「按 `.` 切分后每段非空且不超过 63 字符」，没有字符集白名单。对比另外两处的严格程度：

- Agent：[server.go](F:/Fonu-main/havline/internal/agent/server.go:112) 用正则 `domainPattern` 限定 `[a-z0-9-]`
- FRP：[validate.go](F:/Fonu-main/havline/internal/frp/validate.go) 的 `validDomain` 做了完整标签校验

具体后果：导入证书时域名取自证书的 `DNSNames` 与 `CommonName`（攻击者上传自签证书即可完全控制），`a/b` 这类含斜杠的值能通过校验，随后进入 `certStorageNames` → `filepath.Join(certsDir, name)` → [service.go](F:/Fonu-main/havline/internal/acme/service.go:624) 创建嵌套目录并写入 `fullchain.pem` / `privatekey.pem`。

好消息是 `..` 恰好会被「空段」检查挡住（`../..` 切分后产生空段），所以目前无法逃出 `certsDir`，实际危害限于目录污染。但这是靠巧合防护而非显式设计——`..` 与 `/` 的拦截不应该依赖同一条规则。而且这条宽松规则与另外两个模块的严格规则并存，属于典型的校验口径分裂。

建议：抽出统一的域名校验函数，三处共用；`certStorageNames` 侧再加一道 `filepath.Base` 与字符白名单兜底。

#### P2-2 TLS 私钥可经 API 明文导出

位置：[cert_handler.go](F:/Fonu-main/havline/internal/api/cert_handler.go:112) `Download`、[download.go](F:/Fonu-main/havline/internal/acme/download.go:23)

`GET /api/certificates/{domain}/download?part=key` 会直接返回 `privatekey.pem` 内容。

该接口有正确的鉴权（走 `RequireAuth`），也做了路径校验（`validateCertFilePath` 用 `filepath.Rel` 确认在 `CertsDir` 内，写得很好），且域名必须存在于数据库记录中，不能任意读文件。

风险点在于权限粒度：任何有效会话或具备 `write` scope 的 API Token 都能一次性导出全部站点私钥，没有二次确认、没有审计记录。结合 P1-1，一旦口令被爆破成功，攻击者可直接拖走所有证书私钥。

建议：私钥导出要求重新输入口令或二次确认，并记录审计日志；或对 `part=key` 单独要求更高权限。

#### P2-3 备份文件包含应用日志，日志里含初始管理员口令

位置：[auth.go](F:/Fonu-main/havline/internal/auth/auth.go:64) 首次启动分支、[service.go](F:/Fonu-main/havline/internal/backup/service.go:139) `addDir`、[config.go](F:/Fonu-main/havline/internal/config/config.go:102)

三处叠加形成一条泄漏链：

1. 首次启动自动生成管理员密码后，以明文写入系统日志（`"首次启动已创建管理员账户…"` 携带 `password` 字段）
2. `LogsDir()` 返回 `DataDir + "/logs"`，位于数据目录内
3. 备份打包 `addDir` 时只排除了 `backups` 子目录，其余目录整体入包

结果：导出的备份归档里带走了含明文初始口令的 `app.log`。

影响取决于用户是否已改密码——初始口令是一次性的，若未及时修改则等价于口令泄漏；即便已修改，日志中残留的其他敏感信息（DDNS 凭据、Agent Token 片段等）也可能随备份外传。

建议：首次口令只输出到标准输出或一次性文件，不落常规日志；备份打包时排除 `logs/`（或至少排除 `app.log`）。

#### P2-4 自更新按可变 tag 拉取镜像，不固定 digest

位置：[runner.go](F:/Fonu-main/havline/internal/updater/runner.go) `compose` / `currentImage` 相关逻辑

更新流程通过 `docker compose` 拉取 `havline:<version>` 镜像并以 tag 为主键，全文件搜索未见 `sha256` / `checksum` / 签名校验。

`docker pull` 自身会校验 manifest 与层 digest，所以这不是"完全不校验"。真正的风险是 tag 可变：同一个 `1.9.2` 在不同时间可能指向不同内容，镜像仓库一旦被入侵或凭据泄漏，可以覆盖 tag 下发任意镜像，且本地无法察觉版本内容被替换。

建议：发布时记录镜像 digest，更新流程固定 digest 或校验发布清单；至少在状态里展示 digest 便于比对。

#### P2-5 会话 Cookie 的 Secure 判定信任 `X-Forwarded-Proto`

位置：[auth_handler.go](F:/Fonu-main/havline/internal/api/auth_handler.go:181) `requestIsHTTPS`

```go
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
```

任何来源都可以设置该请求头，没有可信代理地址白名单。代码注释解释了为什么这样取舍（明文部署下若强制加 `Secure` 会导致无法登录），属于有意为之，但它把"是否启用 Secure"的控制权交给了客户端。

实际影响有限：伪造该头只会让服务器给 Cookie 加上 `Secure`，在明文场景下反而导致自己的登录态失效（自伤）。真正的隐患是它掩盖了部署形态——反代场景下若请求直达后端，判定的可信度无从保证。

建议：改为按配置项显式声明「对外协议」（部署时设定），或限定只信任来自已知反代地址的 `X-Forwarded-Proto`。

### P3 低风险

#### P3-1 备份恢复未防符号链接

位置：[service.go](F:/Fonu-main/havline/internal/backup/service.go:108) `Restore`

解压逻辑已经做对了最关键的部分：跳过非 `TypeReg` / `TypeDir` 条目，并用 `filepath.Rel` 拒绝逃出 `dataDir` 的路径（注释还专门说明了为什么不能用前缀比较）。剩余缺口是写入前没有 `Lstat` 检查目标是否已是指向目录外的符号链接——若 `dataDir` 内预先存在恶意符号链接，写入会跟随它。

触发前提较苛刻（需要先能在数据目录内造符号链接），定级 P3。建议写入前用 `os.Lstat` 检查并拒绝符号链接目标。

#### P3-2 随机口令生成存在取模偏置

位置：[auth.go](F:/Fonu-main/havline/internal/auth/auth.go:211) `generatePassword`

```go
out[i] = letters[int(randBytes[i])%len(letters)]
```

`len(letters)` 为 58，不能整除 256，因此字符分布轻微偏向字母表前部。熵损失极小（约 0.01 bit/字符），实际不构成风险，但属于应当消除的实现瑕疵。建议改用 `crypto/rand.Int` 或拒绝采样。

#### P3-3 会话生命周期固定，无滑动续期与绝对上限

位置：[auth.go](F:/Fonu-main/havline/internal/auth/auth.go:22) `sessionTTL`

会话固定 7 天过期，既不在活跃使用时续期，也没有"最长生命期"概念。两个方向都不理想：长期不用的会话仍保留 7 天有效窗口；活跃会话也不会因超期而强制重新认证。

#### P3-4 FRP 服务端布尔选项缺少默认值兜底（已修复）

位置：[validate.go](F:/Fonu-main/havline/internal/frp/validate.go:289)

本次审查中已修复：迁移 `016` 给已有服务端补的 `config_json` 默认值是 `{}`，而 `normalizeServerOptions` 只兜底整型字段，导致 `tcp_mux` / `login_fail_exit` 保持 Go 零值 `false`，生成 `transport.tcpMux = false`（每条新连接都要重建 TCP+TLS）与 `loginFailExit = false`。现已按「键缺省取默认、显式 false 保留」反序列化，并补了单元测试。

#### P3-5 Agent 路由内容可写入任意 Nginx 配置（设计如此）

位置：[server.go](F:/Fonu-main/havline/internal/agent/server.go:1370) `routePath` / `applyRouteContent`

`PUT /api/v1/routes/{domain}` 的内容由管理端提供，Agent 落盘后执行 `nginx -t` 并 reload。域名已严格校验，且改前留版本、校验失败自动回滚，工程上做得很细。

需要记录的是权限含义：持有 Agent Token 即可写入任意 Nginx 指令。若 Nginx 以 root 运行且存在可用的危险指令组合，等于把文件写入能力提升为更高权限。这属于"Agent 本就拥有主机管理权"的设计前提，建议在文档中明确这一信任边界，而不是在代码层修补。

#### P3-6 Agent 传输模式的默认值是明文 http

位置：[028_agent_transport.sql](F:/Fonu-main/havline/migrations/028_agent_transport.sql)、[AgentView.vue](F:/Fonu-main/havline/web/src/views/AgentView.vue:255)

初版把这里误判为 P1。复核后的准确情况是：Agent 本机监听 `127.0.0.1:7700` 明文 HTTP 属于设计（回环流量不出主机，`validateListenAddr` 也会拦截非回环地址），而 Havline→Agent 的连接有三种模式，其中两种已经是安全的。

| 模式 | 路径 | 安全性 |
| --- | --- | --- |
| `tunnel` | Havline 侧建立 SSH 隧道，转发到 VPS 的 `127.0.0.1:7700` | 安全，Agent 无需对外开放 |
| `https-pin` | Agent 侧 Nginx 监听 TLS 端口（默认 7443），证书由 Havline 生成下发，客户端做 SHA-256 指纹固定 | 安全，证书 365 天且到期前自动轮换 |
| `http` | 明文直连 Agent 的 7700 | 明文，Token 可被嗅探 |

实现依据：[service.go](F:/Fonu-main/havline/internal/frp/service.go:831) 的 tunnel 分支、[service.go](F:/Fonu-main/havline/internal/frp/service.go:885) 的 https-pin 分支、[agent_https.go](F:/Fonu-main/havline/internal/frp/agent_https.go) 的证书生成、[agent_client.go](F:/Fonu-main/havline/internal/frp/agent_client.go:79) 的指纹固定。

前端也已经处理到位：`agent_transport === 'http'` 时显示告警「当前仍是公网明文 HTTP。点击「升级 Agent」可自动迁移到 SSH 隧道安全模式。」，并提供一键切换。

残留风险只在默认值：迁移 `028` 把 `agent_transport` 默认设为 `'http'`，已有记录与新建记录都落在明文模式，需要用户主动迁移。若部署时开了 `HAVLINE_AGENT_ALLOW_REMOTE=1` 且未切换，就会形成长期明文暴露。

建议：把默认值改为 `tunnel`（或在新装流程中默认选择安全模式），并在「Agent 处于远程监听且 transport 仍为 `http`」时于服务端列表持续标红，而不是只弹一次告警。

## 四、代码质量审查

### 结构与规模

- 分层清晰：`cmd`（入口）/ `internal`（领域服务）/ `web`（前端）/ `migrations`（增量迁移），跨模块调用基本走接口。
- 单文件过大是主要结构问题：[agent/server.go](F:/Fonu-main/havline/internal/agent/server.go) 达 71 KB，把路由、Nginx 管理、证书、frps 控制、命令执行混在一处。按领域拆分（route / nginx / frps / certs）会显著降低审查难度。
- `internal/frp/service.go` 同样偏大，本次修 bug 时也要在多处来回定位。

### 规范一致性

- 中文注释质量高，多处注释解释"为什么这么写"而非"做了什么"（例如备份解压用 `filepath.Rel` 的原因、`natTraversal` 只在显式禁用时输出），这类注释对后续维护价值很大。
- 主要不一致是同一语义三种实现：域名校验存在 Agent 正则、FRP `validDomain`、证书 `validateDomainLabel` 三套口径，宽松程度不同。建议统一。
- 文档与实现存在偏差：[FrpView.vue](F:/Fonu-main/havline/web/src/views/FrpView.vue:385) 描述诊断有「TCP 可达 / frpc 登录 / 代理可达 / DNS 解析 / 归属地」五项，而 [diagnostics.go](F:/Fonu-main/havline/internal/frp/diagnostics.go:88) 实际只产出 `tcp_reachable`、`frpc_login`、`dns_resolution`、`location` 四项，「代理可达」并不存在；且诊断只探测控制端口，不覆盖规则的公网远程端口。

### 测试

测试密度在同类项目中属于良好水平：认证、令牌、备份、Agent 授权、路由版本、frp 配置渲染等关键路径均有单元测试，`agent` 包甚至有遍历全部路由、断言无 Token 必须返回 401 的测试（`router_auth_test.go`），这是很有效的防回归手段。

本次验证命令均通过：

```
go test ./internal/frp/     通过
go vet ./...                通过
npm run typecheck           通过
```

未覆盖：`-race` 竞态检测、依赖漏洞扫描（`govulncheck`）、SAST（`gosec`）本次均未执行。

## 五、修复优先级建议

按投入产出排序：

1. 登录限流与失败锁定（P1-1）——改动集中在认证层，收益最大
2. 统一域名校验函数（P2-1）——顺手消除三处口径分裂
3. 私钥导出加二次确认与审计（P2-2）
4. 备份排除 `logs/`，初始口令不落常规日志（P2-3）
5. Agent 传输默认值改为 `tunnel`，并对明文模式持续标红（P3-6）——迁移里改一个默认值即可改善默认安全姿态

## 六、已确认的安全设计

以下设计经代码复核确认正确，无需修改：

1. 口令使用 `bcrypt`（DefaultCost），会话 ID 由 `crypto/rand` 生成 32 字节
2. API Token 仅存 SHA-256、区分读写 scope，且 `password` / `logout` / `tokens` / `update` 路径禁止 Token 访问，避免令牌自提权（[middleware.go](F:/Fonu-main/havline/internal/api/middleware.go)）
3. Cookie 设置 `HttpOnly` + `SameSite=Lax`，条件启用 `Secure`
4. 路由默认全部经过 `SessionMiddleware` + `RequireAuth`，`public()` 仅用于登录与登出
5. 备份 tar 解压拒绝目录逃逸，归档名走正则白名单，解压不照搬权限位
6. 版本文件读取拒绝 `.` / `..` / 含路径分隔符（[fsutil.go](F:/Fonu-main/havline/internal/fsutil/fsutil.go:185)）
7. Agent 全部路由统一鉴权、常量时间比较 Token、默认仅监听回环、`upstream` 与域名过滤防 Nginx 配置注入
8. TLS 指纹固定实现正确：pin 为空时 fail-closed（[agent_client.go](F:/Fonu-main/havline/internal/frp/agent_client.go:79)）；Agent 自检的 `InsecureSkipVerify` 只用于回环地址并随后比对指纹
9. 未放宽 CORS；前端不把 Token 存入 `localStorage`（仅主题偏好）
10. SQL 全量使用参数占位符，未发现拼接式查询
11. Agent 远程传输已提供 SSH 隧道与「HTTPS + 证书指纹固定」两种安全模式，证书自动轮换，前端对明文模式给出告警和一键迁移入口

## 七、结论

本次未发现未授权访问、远程代码执行或 SQL 注入级别的直接可利用漏洞。代码在这几类问题上防守到位，甚至优于多数同类自托管项目的平均水平。

真正需要处理的是公网暴露面下的认证入口缺口：登录没有限流（P1-1）。Agent 的远程传输已经具备 SSH 隧道与 HTTPS 指纹固定两种安全模式，遗留问题只是默认值仍落在明文 `http`（P3-6）。其余问题集中在校验口径不统一、敏感数据出口缺少二次确认与审计这两条主线上。
