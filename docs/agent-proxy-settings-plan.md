# 公网反代「部署设置」对齐本地反代 —— 方案设计

> 目标：把本地反向代理（`ProxyView.vue` 规则弹窗）中真正有价值的安全与运维设置，按公网反代的语义迁移到
> 「公网反代」页（`/public-proxy`，`ProxyRemoteView.vue`）的「部署设置」中，由 havline-agent 生成对应的 Nginx vhost 配置。
>
> 原则：**公网反代只管「公网入口」这一层**。凡是会破坏 frps vhost 路由、或本质属于「本机内网分流」的选项，
> 一律不迁移（见「明确不做」一节）。
>
> 更新日期：2026-09-22

---

## 建议
「部署设置」弹窗从现在的单页改为**三个 Tab（基础 / 安全 / 高级）**，与本地反代弹窗结构对齐：

- 
**基础**：启用 HTTPS、强制跳转、WebSocket、上传大小、代理超时

- 
**安全**：IP 白名单 / 黑名单、Basic Auth（用户名 + 可选密码）

- 
**高级**：仅 TLS 1.3、安全响应头、限流、连接数限制

第一梯队 7 项都是 agent 模板内改动 + Havline 参数透传，风险低、见效快；第二梯队 3 项需要 agent 多写一个 http 上下文文件。说一声我从第一梯队开始做，或者指定要哪几项。



## 一、现状对照

### 本地反代（ProxyView.vue）规则弹窗现有设置项

| 分组 | 设置项 |
| --- | --- |
| 基础 | 名称、前端域名、目标地址、**启用 HTTPS**、**HTTP 跳转 HTTPS**、启用规则 |
| 安全 · IP 访问控制 | 仅中国大陆 IP、黑名单模式 + 列表、白名单模式 + 列表 |
| 安全 · 认证 | Basic Auth（用户名 + **自定义密码**） |
| 安全 · 流量控制 | 请求限流（速率 / 突发）、连接数限制（每 IP 并发） |
| 安全 · 高级 | 忽略后端 TLS 证书、使用目标 Host 头、仅 TLS 1.3、安全响应头 |
| Nginx | auto / custom 自定义配置 |

### 公网反代（agent `nginxRoute` 模板）现有能力

`internal/agent/server.go` 的 `routeRequest` 与 `nginxRoute` 已支持：

- `domain` / `upstream`（`127.0.0.1:<frps vhost 端口>`）/ `tls` + `cert_dir`；
- `websocket`（Upgrade / Connection 头）；
- `redirect_https`（80 → 443 的 301 + 443 TLS server 两块）；
- `allow_ips`（location 内 allow + deny all）；
- `basic_auth_user`（密码由 agent 自动生成并落盘 htpasswd）；
- 默认头：Host / X-Real-IP / X-Forwarded-For。

Havline 侧对应链路：`ProxyOptions`（`internal/frp/model.go`）→ `DeployAgentRoute` → `AgentClient.SyncRouteEx`
→ `PUT /api/v1/routes`。

---

## 二、迁移清单

### 第一梯队：纯模板级改动，无 Nginx 上下文限制（建议先做）

| # | 功能 | 对应本地项 | 生成的 Nginx 指令 |
| --- | --- | --- | --- |
| 1 | **启用 HTTPS 开关** | 启用 HTTPS | 关闭时仅 `listen 80`；开启且证书已推送时 `listen 443 ssl` |
| 2 | **安全响应头** | 安全响应头 | `add_header Strict-Transport-Security / X-Frame-Options / X-Content-Type-Options / Referrer-Policy` |
| 3 | **仅 TLS 1.3** | 仅 TLS 1.3 | `ssl_protocols TLSv1.3;` |
| 4 | **客户端最大上传大小** | （本地为全局默认） | `client_max_body_size 50m;`（server 级，按规则可覆盖） |
| 5 | **代理超时** | （本地固定 3600s） | `proxy_read_timeout 3600s;`（长轮询 / 流媒体 / 大文件必需） |
| 6 | **IP 黑名单** | 黑名单模式 | location 内 `deny x.x.x.x;`（与现有白名单对称） |
| 7 | **Basic Auth 自定义密码** | Basic Auth 密码 | 用表单密码写 htpasswd；留空则保持「首次自动生成」 |

### 第二梯队：需要 Nginx `http` 上下文或额外数据下发（成本中等）

| # | 功能 | 关键约束 |
| --- | --- | --- |
| 8 | **请求限流** | `limit_req_zone` 只能出现在 `http` 块。agent 需额外生成 `havline-limits.conf`（宝塔将 vhost 目录 include 在 `http{}` 内，因此该文件顶层指令即 http 上下文），各域名 server 内 `limit_req zone=havline_<域名>_req burst=N nodelay;` |
| 9 | **连接数限制** | 同上，`limit_conn_zone` + `limit_conn havline_<域名>_conn N;` |
| 10 | **仅中国大陆 IP** | 需 `geo` 模块 + 中国 IP 段下发到 VPS（Havline 已有 china cidr 模块，7456 条 v4），下发体积最大、成本最高，建议最后做 |

### 明确不做（字段 / 语义不匹配）

| 功能 | 原因 |
| --- | --- |
| 使用目标 Host 头 | 公网回源目标是 frps 的 vhost 端口，**frps 靠 Host 头匹配隧道**。改成上游 Host 会让 vhost 无法路由，直接 404。必须保持 `$host` |
| 忽略后端 TLS 证书 | 公网 upstream 是 `http://127.0.0.1:<vhost端口>`，不涉及上游 TLS 校验，无作用 |
| 自定义 upstream 覆盖 | 技术上可行，但会让流量绕过 frps 隧道（健康检查「隧道」必然 ✗），语义属于「本机内网分流」，只在本地反代页做 |

---

## 三、字段契约

两侧 JSON key 必须严格一致（历史教训：字段未逐项对照导致多轮返工）。

| 显示名 | `ProxyOptions`（Go，`internal/frp/model.go`） | JSON key | TS（`web/src/api/types.ts`） | `routeRequest`（agent） | Nginx 指令 |
| --- | --- | --- | --- | --- | --- |
| 启用 HTTPS | `HTTPSDisabled bool` | `https_disabled` | `https_disabled?: boolean` | `HTTPSDisabled bool` | 关闭时不出 `listen 443` |
| 上传大小 | `ClientMaxBodySize string` | `client_max_body_size` | `client_max_body_size?: string` | `ClientMaxBodySize string` | `client_max_body_size 50m;` |
| 代理超时 | `ProxyReadTimeout string` | `proxy_read_timeout` | `proxy_read_timeout?: string` | `ProxyReadTimeout string` | `proxy_read_timeout 3600s;` |
| 安全响应头 | `SecurityHeaders bool` | `security_headers` | `security_headers?: boolean` | `SecurityHeaders bool` | `add_header ...` |
| 仅 TLS 1.3 | `TLS13Only bool` | `tls13_only` | `tls13_only?: boolean` | `TLS13Only bool` | `ssl_protocols TLSv1.3;` |
| IP 黑名单 | `DenyIPs []string` | `deny_ips` | `deny_ips?: string[]` | `DenyIPs []string` | `deny x;` |
| Basic Auth 密码 | `BasicAuthPassword string` | `basic_auth_password` | `basic_auth_password?: string` | `BasicAuthPassword string` | 写入 htpasswd |
| 请求限流（速率） | `RateLimitRate int` | `rate_limit_rate` | `rate_limit_rate?: number` | `RateLimitRate int` | `limit_req_zone ... rate=Nr/s;` |
| 请求限流（突发） | `RateLimitBurst int` | `rate_limit_burst` | `rate_limit_burst?: number` | `RateLimitBurst int` | `limit_req ... burst=N` |
| 连接数限制 | `ConnLimitMax int` | `conn_limit_max` | `conn_limit_max?: number` | `ConnLimitMax int` | `limit_conn_zone` / `limit_conn` |
| 仅中国大陆 | `ChinaOnly bool` | `china_only` | `china_only?: boolean` | `ChinaOnly bool` | `geo` + `allow`/`deny` |

约束：

- 这些字段**只作用于公网 Nginx 模板，绝不写入 frpc.toml**（与现有 `websocket` / `redirect_https` 等一致，在 `ProxyOptions` 上以注释标明）；
- 「启用 HTTPS」采用**反向字段 `https_disabled`**：字段缺省时保持现有行为（证书推送成功即启用 TLS），
  保证新 Havline + 旧 agent、旧数据不出现行为突变。

---

## 四、实施步骤

### 阶段 1：第一梯队后端（agent + Havline）

1. `internal/agent/server.go`
   - `routeRequest` 增加第一梯队字段；
   - `nginxRoute` 模板按字段输出指令：`listen 443` 受 `HTTPSDisabled` 控制；`redirect_https` 仅在 TLS 实际启用时生成；
   - `client_max_body_size` / `proxy_read_timeout` / 安全响应头 / `ssl_protocols` / `deny` 列表；
   - Basic Auth：传入密码时 `htpasswd -nbB` 写入，未传则沿用现有「首次自动生成 + 复用」。
2. `internal/frp/model.go`：`ProxyOptions` 增加同名 Go 字段（含注释）。
3. `internal/frp/agent_client.go`：`SyncRouteEx` 参数改为**传结构体**（当前 10 个位置参数已到上限，继续追加易错）：
   ```go
   type RouteSpec struct {
       Domain, Upstream, CertDir string
       TLS, WebSocket, RedirectHTTPS, HTTPSDisabled bool
       VhostPort int
       AllowIPs, DenyIPs []string
       BasicAuthUser, BasicAuthPassword string
       ClientMaxBodySize, ProxyReadTimeout string
       SecurityHeaders, TLS13Only bool
   }
   func (c *AgentClient) SyncRouteSpec(ctx, spec RouteSpec) (map[string]any, error)
   ```
   保留 `SyncRoute` / `SyncRouteEx` 作为兼容包装。
4. `internal/frp/service.go`：`DeployAgentRoute` 从 `proxy.Options` 组装 `RouteSpec` 透传。

### 阶段 2：第一梯队前端

5. `web/src/api/types.ts` 补 `FrpProxyOptions` 字段；
6. `ProxyRemoteView.vue`「部署设置」弹窗改为三个 Tab（与本地反代结构对齐）：
   - **基础**：启用 HTTPS、HTTP 强制跳转、WebSocket、上传大小、代理超时；
   - **安全**：IP 白名单、IP 黑名单、Basic Auth（用户名 + 可选密码）；
   - **高级**：仅 TLS 1.3、安全响应头（第二梯队上线后再加限流/连接数/中国 IP）。

### 阶段 3：第二梯队（按需）

7. agent 增加 `havline-limits.conf` 生成器（http 上下文 zone 定义）+ 每域名 server 引用；
8. 中国 IP：Havline 将 china cidr 列表推送到 agent（`/var/lib/havline-agent/geo/china.conf`），agent 生成 `geo` 块。

---

## 五、验证方案

每次改动后全量跑（项目既有闭环）：

```powershell
cd f:\Havline-main\Havline-1.0.2
gofmt -w internal/agent/server.go internal/frp/model.go internal/frp/agent_client.go internal/frp/service.go
go build ./... ; go vet ./internal/frp ./internal/agent ./internal/api
go test ./internal/frp ./internal/api
cd web ; npx vue-tsc --noEmit ; npx vite build
```

agent 二进制需按既有流程交叉编译（amd64 + arm64）并升级：

```powershell
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
go build -trimpath -o internal/frp/agentbin/havline-agent-linux-amd64.new ./cmd/havline-agent
# arm64 同理；Move-Item 覆盖后，在「公网 Agent」页执行「升级 Agent」
```

VPS 侧实测（部署 7023 规则后）：

```bash
nginx -t                                   # 语法
cat /www/server/panel/vhost/nginx/havline-7023.36800.cc.conf   # 配置内容（或页面「查看配置」）
curl -I http://7023.36800.cc               # 强制跳转 → 301
curl -sI -H 'Host: 7023.36800.cc' http://127.0.0.1:9080    # 回源自检
curl -k -sI https://7023.36800.cc          # 响应头（HSTS / X-Frame-Options）
openssl s_client -connect 7023.36800.cc:443 -tls1_2 </dev/null | head -3   # 仅 TLS1.3 时应握手失败
curl -sI -H 'Host: 7023.36800.cc' -d @bigfile http://127.0.0.1:9080        # 上传大小限制
for i in $(seq 1 50); do curl -s -o /dev/null -w '%{http_code} ' http://7023.36800.cc/; done   # 限流 → 429（第二梯队）
```

排查口径沿用现有约定：健康检查五段徽标（DNS / 证书 / 隧道 / 服务 / 反代生效态）任一 ✗，
悬停看详情定位断点，不再靠猜。

---

## 六、兼容性与回滚

| 场景 | 期望行为 |
| --- | --- |
| 新 Havline + 旧 agent | 旧 agent 忽略未知 JSON 字段（Go 默认行为），新选项静默不生效 → **部署前必须先「升级 Agent」**，页面提示 |
| 旧 Havline + 新 agent | 新字段缺省，行为与现状一致（`https_disabled` 缺省 = 启用 TLS） |
| frps v0.68.0 / v0.71.0 | 与本方案无关（作用于 Nginx vhost，不依赖 dashboard API） |
| 回滚 | 关闭对应开关后重新部署即可恢复；agent 侧每域名配置为整文件覆盖写，无残留 |

---

## 七、附：与既有文档的关系

- `docs/agent-proxy-roadmap.md`：功能梯队总览（本方案是其「第二批」在部署设置维度的细化）；
- `docs/frps-features.md`：frps 服务端配置能力对照（frps.toml 层面，与本文的 Nginx 层面互补）。
