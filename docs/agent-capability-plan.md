# fonu-agent 能力扩展方案

> 本文盘点 VPS 侧 `fonu-agent` 现有的接口面与数据面，给出「能直接做」「需要新增 agent 端点」「不做」三类功能的详细设计。
>
> 依据：本仓库 `internal/agent/`、`internal/frp/agent_client.go`、`internal/frp/agent_installer.go`、`internal/frp/ssh_installer.go`、`internal/api/frpmulti_handler.go`、`web/src/views/AgentView.vue`、`web/src/views/ProxyRemoteView.vue`（均为读代码确认，非推测）。
> 当前 `AgentVersion = "0.9.2"`（`internal/agent/server.go:34`）。
> 更新日期：2026-09-23

---

## 一、现状盘点

### 1.1 agent 的接口面（12 个端点）

`internal/agent/server.go:151-162`：

| # | 方法与路径 | 作用 |
| --- | --- | --- |
| 1 | `GET /api/v1/ping` | 存活探测（安装/升级后确认 token 与网络） |
| 2 | `GET /api/v1/status` | 一次性返回 Nginx / frps / 端口 / 路由名的快照 |
| 3 | `GET /api/v1/routes` | 每条 `fonu-*.conf` 的真实状态：`file_exists` / `syntax_ok` / `loaded` |
| 4 | `PUT /api/v1/routes` | 写入（或覆盖）一条域名的 Nginx 片段 |
| 5 | `GET /api/v1/routes/{domain}/conf` | 回读某域名实际生效的配置文件内容 |
| 6 | `DELETE /api/v1/routes/{domain}` | 删除某域名的片段 |
| 7 | `POST /api/v1/nginx/install` | 一键安装 Nginx |
| 8 | `PUT /api/v1/china-cidr` | 下发国内地址库（供 `china_only` 规则引用） |
| 9 | `PUT /api/v1/frps/config` | 写入 frps.toml |
| 10 | `POST /api/v1/frps/install` | 下载并安装指定版本的 frps（幂等，重复调用=升级） |
| 11 | `POST /api/v1/frps/action` | frps 的 start / stop / restart |
| 12 | `POST /api/v1/certs` | 推送证书（fullchain + privatekey） |

鉴权：全部走 `s.auth(...)` 包装的 token 校验；agent **只监听 VPS 本地**，由 Fonu 主动发起请求，**不做任何主动上报**。

### 1.2 agent 的数据面（`status` 已返回、但应用侧只用了一部分）

`internal/agent/server.go:191` 的 `status` 返回字段：

| 字段 | 含义 | 应用侧现状 |
| --- | --- | --- |
| `agent_version` | agent 自身版本 | 已用（AgentView 版本核对） |
| `nginx_ok` / `nginx_missing` / `nginx_running` / `nginx_path` / `nginx_version` / `nginx_error` | Nginx 可用性探测（带缓存） | 已用 |
| `nginx_binary` / `nginx_conf_dir` | 配置的二进制与 vhost 目录 | 已用 |
| `ports` | 80 / 443 / 7000 / 8080 等端口是否监听 | **只用到了 8080 一项**（`AgentView.vue:359`） |
| `frps_active` / `frps_enabled` | systemd 单元是否运行 / 是否开机自启 | 已用 |
| `frps_binary` / `frps_config_path` | Fonu 侧配置的路径 | 已用 |
| `frps_unit_exec` | systemd 实际执行的启动命令 | 已展示，**未做一致性核对** |
| `frps_version` | 探测到的 frps 版本 | 已展示 |
| `frps_log_tail` | frps 日志尾部 6 行 | 已展示 |
| `dashboard_addr` | 从 frps.toml 解析出的 dashboard 地址 | 已用 |
| `routes` | 已部署的域名列表 | 已用（部署台账） |

### 1.3 应用侧的调用面

`internal/frp/agent_client.go` 暴露 15 个方法，与上面 12 个端点一一对应（`Ping` / `Status` / `SyncRoute` / `SyncRouteSpec` / `SyncRouteEx` / `RouteConf` / `RoutesDetail` / `DeleteRoute` / `InstallNginx` / `SyncChinaCIDR` / `FRPSAction` / `InstallFRPS` / `PushFRPSConfig` / `PushCertificate`）。

其中 `RouteSpec`（`agent_client.go:83`）承载了完整部署语义：`Domain / Upstream / CertDir / TLS / WebSocket / RedirectHTTPS / HTTPSDisabled / SecurityHeaders / TLS13Only / VhostPort / AllowIPs / DenyIPs / BasicAuth* / ClientMaxBodySize / ProxyReadTimeout / RateLimit*`。

### 1.4 安装与升级链路（已闭环）

- agent 二进制**内嵌在 Fonu 里**（`internal/frp/agentbin`，按 arch 取），通过 **SSH** 上传到 VPS 并注册/重启 systemd 单元（`agent_installer.go` + `ssh_installer.go`）。SSH 连接信息就是迁移 023 加的 `ssh_host / ssh_port / ssh_user / ssh_auth / ssh_secret_enc`。
- 因此「升级 Agent」不需要 agent 自己有下载能力——**自升级不是必须项**。
- 版本约定：`AgentVersion` 上方按「每项行为变更一行」的注释累积，改动后手动递增（当前 0.9.2）。

### 1.5 能力边界（缺口所在）

1. **没有资源采集**：VPS 的 CPU / 内存 / 磁盘 / 负载完全不可见。
2. **没有日志读取**：只有 frps 的 6 行 tail，Nginx 的 access / error 日志读不到（排障必须 SSH）。
3. **frps 配置只写不读**：`PUT /api/v1/frps/config` 存在，没有对应的 GET；用户手改 VPS 上的 frps.toml 后 Fonu 无从知晓，下次下发直接覆盖。
4. **没有证书清单**：只能推送，无法列出 VPS 上现有的证书与到期时间（宝塔自己申请的证书看不到）。
5. **部署是逐条的**：`SyncRouteSpec` 按单条规则调用，没有「全量重部署」入口——agent 重装/换机后只能一条条点。
6. **写入结果不含 reload 细节**：片段写入成功但 Nginx reload 失败时，用户拿不到失败原因。

---

## 二、方案总览

| 批次 | 项 | 改动面 | 前置依赖 | 预估 |
| --- | --- | --- | --- | --- |
| **P0-1** | 端口矩阵 | 前端（AgentView） | 无，`status.ports` 已有 | 0.5h |
| **P0-2** | 部署漂移清单 + 一键全量重部署 | 后端 service + 前端 | 无，`GET /routes` 与 `RouteConf` 已有 | 2h |
| **P0-3** | frps 运行一致性核对 | 前端 | 无，`status` 已有 | 0.5h |
| **P0-4** | 证书推送核对 | 前端 | 无，`RouteHealth` 已有 | 0.5h |
| **P1-1** | VPS 资源采集 `GET /api/v1/metrics` | agent + 应用 + 前端 | 需递增 AgentVersion、重编二进制、VPS 上升级 | 2h |
| **P1-2** | 公网 Nginx 日志 `GET /api/v1/logs/nginx` | agent + 应用 + 前端 | 同上 | 2.5h |
| **P1-3** | frps.toml 回读 `GET /api/v1/frps/config` | agent + 应用 + 前端 | 同上 | 1.5h |
| **P2-1** | VPS 证书清单 `GET /api/v1/certs` | agent + 应用 + 前端 | 同上 | 1.5h |
| **P2-2** | reload 失败原因回传 | agent + 应用 | 同上 | 1h |
| **P3** | 不做：防火墙读写、任意命令执行、agent 自升级 | — | — | — |

**贯穿约束（所有新增 agent 端点都适用）**：老版本 agent 没有新端点，应用侧调用必须**按「端点不存在」优雅降级**（收到 404/405 时显示「VPS 上的 agent 版本过低，请升级 Agent」而不是报错），否则用户升级了 Fonu 但没升级 agent 时页面会直接报错。

---

## 三、P0 详细设计（不动 agent，纯应用侧）

### 3.1 端口矩阵

- **数据**：`status.ports`（布尔映射）。实现前先以 agent 代码为准列出全部键（当前已知 `80` / `443` / `7000` / `8080`）。
- **呈现**：AgentView「运行状态」段内加一张两列小表（端口 / 状态徽标），或把现有「8080 端口」那一项扩成完整列表。
- **降级**：agent 版本过旧、`ports` 缺失时整块显示「当前 agent 版本不返回端口信息」。
- **验收**：停掉监听 443 的服务后该行转异常色；旧 agent 上不报错。

### 3.2 部署漂移清单 + 一键全量重部署（价值最高）

三个集合：`A` = 数据库里启用中的反代规则域名；`B` = `GET /api/v1/routes` 返回的域名；`C` = 其中 `loaded = true` 的域名。

| 类别 | 定义 | 含义 |
| --- | --- | --- |
| 未部署 | `A - B` | 库里要部署但 VPS 上没有（agent 重装、换机、手工删过） |
| 孤儿配置 | `B - A` | VPS 有但库里没有（手工加的 `fonu-*.conf`、已删规则的残留） |
| 未生效 | `A ∩ (B - C)` | 文件在但未被 Nginx 加载（`syntax_ok=false`，或 include 未覆盖） |
| 内容不一致 | `RouteConf(domain)` 与「按当前部署设置生成的内容」规范化比对 | 手工改过上游 / 证书路径 / 安全项 |

**一键全量重部署**：对 `A` 逐条 `SyncRouteSpec`（串行、逐条收集结果、失败不中断，沿用 `Service.Reload` 的隔离报错风格），结束后回读一次 `GET /routes` 复核，结果弹窗汇总「成功 N / 失败 M + 每条失败原因」。

**前端落点**：公网反代页顶部提示条（有差异时才出现）+「重新部署全部」按钮（二次确认，因为会覆盖手工改动）。

**验收**：手工删一个 `fonu-*.conf` → 清单出现「未部署」，重部署后消失；手工改一条 conf 的上游 → 出现「内容不一致」；语法错误时归入「未生效」并给出原因。

### 3.3 frps 运行一致性核对

- **比对三项**：`frps_unit_exec`（systemd 实际执行）vs Fonu 配置的 `frps_binary`；`frps_version` vs 期望版本；`frps_config_path` vs Fonu 生成路径。
- **呈现**：AgentView「frps 控制」段加一行「启动路径一致性」，不一致时并列显示两个路径。
- **验收**：手工改过 systemd unit 的 VPS 上显示不一致；一致的机器显示绿色。

### 3.4 证书推送核对

- **数据**：已有 `RouteHealth` 的证书段（`cert_ok` / `cert_detail`）+ 推送结果。
- **呈现**：公网反代页健康列已展示证书段；补一行「证书来源」（Fonu 推送 / 待推送 / VPS 本地已有）。
- **验收**：证书过期时健康列给出证书段失败与到期天数。

---

## 四、P1 详细设计（新增 agent 端点）

> 改动流程见 §7.1；以下端点响应都带 `checked_at`，与现有 `status` 保持一致。

### 4.1 `GET /api/v1/metrics` —— VPS 资源采集

响应：

```json
{
  "cpu_percent": 12.5,
  "mem_used_bytes": 1288490188,
  "mem_total_bytes": 4294967296,
  "disk_used_bytes": 21474836480,
  "disk_total_bytes": 53687091200,
  "disk_path": "/",
  "load_avg": [0.42, 0.51, 0.63],
  "uptime_seconds": 864000,
  "checked_at": "2026-09-23T19:00:00Z"
}
```

- **实现要点**：**直接复用仓库里已有的 `internal/sysinfo`**（同一 module，`cmd/fonu-agent` 可直接引用）——`/proc/stat`、`/proc/meminfo`、`Statfs` 以及 Windows 的 kernel32 路径都已写好；agent 跑在 VPS 上，采到的天然是 VPS 自身数值。
- **CPU 差值**：`sysinfo.Read(prev)` 需要上次采样；agent 进程内保存上次 `CPUSample`，并对间隔 < 1 秒的重复调用直接返回上次结果（避免连续刷新时数值抖动）。
- **`load_avg` / `uptime_seconds`**：Linux 读 `/proc/loadavg`、`/proc/uptime`；非 Linux 省略这两个字段（前端按缺失处理）。
- **应用侧链路**：`AgentClient.Metrics(ctx)` → `GET /api/frpmulti/servers/{id}/agent/metrics` → AgentView 新增「VPS 资源」卡（复用本地仪表盘「系统资源」的行样式：名称 / 用量明细 / 4px 进度条 / 百分比，≥75% 转橙、≥90% 转红）。
- **降级**：404/405 → 卡片显示「VPS 上的 agent 版本过低（需 ≥ 0.10.0），请升级 Agent」。
- **验收**：VPS 上跑 `dd` 或压测时 CPU / 磁盘数值随之上升；磁盘接近写满时进度条转红；旧 agent 上显示升级提示而不是报错。

### 4.2 `GET /api/v1/logs/nginx` —— 公网 Nginx 日志

请求：`GET /api/v1/logs/nginx?type=access|error&lines=200`（`lines` 上限 2000，默认 200）

响应：

```json
{
  "type": "access",
  "path": "/var/log/nginx/access.log",
  "lines": ["127.0.0.1 - - [23/Sep/2026:19:00:00 +0800] \"GET / HTTP/1.1\" 200 1234"],
  "truncated": false,
  "checked_at": "2026-09-23T19:00:00Z"
}
```

- **路径来源（白名单，不接受任意 path 参数）**：
  1. 优先从 `nginx -T` 的 dump 里解析 `access_log` / `error_log` 指令（agent 已有 `nginx -T` 解析与缓存经验，见 `resolveNginxConfDir`）；
  2. 解析不到时回落默认路径 `/var/log/nginx/{access,error}.log`；
  3. 支持环境变量 `FONU_AGENT_NGINX_LOG_DIR` 覆盖（与现有 `FONU_AGENT_NGINX_CONF_DIR` 的回退风格一致）。
- **只读尾部**：实现 `tailFile(path, n)`（与现有 `journalTail` 同思路），从文件末尾按块倒读，避免把整个日志读进内存；单行超长（如 4KB）截断。
- **错误处理**：文件不存在 → 200 + `lines: []` + 原样返回 `path`（前端显示「未找到日志文件」）；无权限 → 403 并给出明确原因。
- **应用侧链路**：`AgentClient.NginxLogs(ctx, serverID, type, lines)` → `GET /api/frpmulti/servers/{id}/agent/logs/nginx?...`；日志中心新增「公网 Nginx」页签，复用现有 `LogToolbar` / `LogPagination` / `EmptyState` 与 `logTimeCell`，与「FRP 日志」页签同一套呈现。
- **验收**：VPS 上产生一次访问后页签能看到该条 access 记录；agent 无读权限时显示明确错误；`lines` 超过 2000 被夹到上限。

### 4.3 `GET /api/v1/frps/config` —— frps.toml 回读

响应：`{ "path": "/etc/frp/frps.toml", "exists": true, "content": "...", "mod_time": "2026-09-23T18:00:00Z", "checked_at": "..." }`

- **实现要点**：读 `s.cfg.FrpsConfigPath`，限制文件大小（如 256KB），超限返回 `truncated: true`；文件不存在返回 `exists: false` 而不是错误。
- **应用侧**：在「公网服务端」的 frps 同源配置弹窗里加「与 VPS 实际文件比对」——把「Fonu 即将下发的内容」与回读内容做规范化 diff（去注释与空白），不一致时提示「VPS 上的 frps.toml 已被手工修改」，并给「重新下发」按钮（复用已有 `PushFRPSConfig`）。
- **验收**：在 VPS 上手工改一行 frps.toml → 弹窗提示不一致并高亮差异行；重新下发后恢复一致。

---

## 五、P2 详细设计

### 5.1 `GET /api/v1/certs` —— VPS 证书清单

响应：`{ "certs": [{ "domain": "7023.36800.cc", "cert_path": "...", "key_path": "...", "not_after": "2026-12-01T00:00:00Z", "days_left": 69 }], "warnings": [], "checked_at": "..." }`

- **实现要点**：遍历 `s.cfg.CertsDir` 下的域名目录，用标准库 `crypto/x509` 解析 `fullchain.pem` 的 `NotAfter`；解析失败的单条跳过并记入 `warnings`。
- **应用侧**：与 Fonu 的证书列表交叉核对，输出「Fonu 有 / VPS 有 / 两边都有但到期时间不同」三类；宝塔自行申请的证书也能被看到。
- **验收**：过期证书 `days_left < 0` 并标红；Fonu 未推送过的域名出现在「VPS 本地已有」分组。

### 5.2 reload 失败原因回传

- **做法（择一，倾向 A）**：
  - **A**：`PUT /api/v1/routes` 与 `DELETE /api/v1/routes/{domain}` 的响应体增加 `reload_ok` / `reload_error` 字段——写入成功但 reload 失败时不再只回 200。
  - **B**：新增 `POST /api/v1/nginx/reload`，返回 `nginx -t` 与 reload 的 stderr。
- **应用侧**：部署结果提示里带上 `reload_error`；漂移清单的「未生效」类别直接引用它。
- **验收**：故意写一条语法错误片段 → 部署结果明确显示 reload 失败原因，而不是「部署成功」。

---

## 六、明确不做的

1. **VPS 防火墙规则读写**（ufw / firewalld / 宝塔面板差异大、需要 root，误操作会直接断网）——最多做只读展示。
2. **任意命令执行端点**——agent 目前只有 token 鉴权，开放 shell 等于把 VPS 交出去。
3. **agent 自升级**——现有链路（Fonu 内嵌二进制 + SSH 上传 + 重启 systemd）已经闭环，自升级只省掉存 SSH 凭据，收益有限，还要额外处理下载源与签名校验。

---

## 七、改动流程与验收

### 7.1 agent 侧（每新增一个端点都要走完）

1. `internal/agent/server.go`：加 handler + `mux.HandleFunc(...)`，并在 `AgentVersion` 上方按现有约定补一行版本说明（例如 `// 0.10.0：新增 /api/v1/metrics 资源采集与 /api/v1/logs/nginx 日志读取。`），把常量递增到 **`0.10.0`**。
2. 重编两个架构的内嵌二进制（Fonu 编译期嵌入，必须是真实二进制而不是占位文件）：

```powershell
$env:GOOS='linux'; $env:CGO_ENABLED='0'; $env:GOARCH='amd64'
go build -trimpath -o internal/frp/agentbin/fonu-agent-linux-amd64.new ./cmd/fonu-agent
$env:GOARCH='arm64'
go build -trimpath -o internal/frp/agentbin/fonu-agent-linux-arm64.new ./cmd/fonu-agent
Move-Item -Force internal/frp/agentbin/fonu-agent-linux-amd64.new internal/frp/agentbin/fonu-agent-linux-amd64
Move-Item -Force internal/frp/agentbin/fonu-agent-linux-arm64.new internal/frp/agentbin/fonu-agent-linux-arm64
```

3. 重启本机 Fonu（加载新内嵌二进制）→ VPS 上「升级 Agent」→ 用 `GET /api/v1/status` 的 `agent_version` 确认已是 `0.10.0`。

### 7.2 应用侧（每个端点对应一条链路）

`internal/frp/agent_client.go` 加方法 → `internal/frp/service.go` 加业务方法（含**端点不存在时的降级**：区分 404/405 与真实错误）→ `internal/api/frpmulti_handler.go` 加 handler → `internal/api/router.go` 注册（沿用现有 `agent/*` 路径风格）→ `web/src/api/{client,types}.ts` → 前端页面。

### 7.3 验收（每批的检查项）

- **P0**：手工制造四类漂移各一次，清单分类正确；全量重部署在单条失败时其余仍继续并汇总报错；旧 agent 上端口矩阵降级不报错。
- **P1**：`metrics` 数值随 VPS 负载变化且 CPU 不抖动；`logs/nginx` 尾部读取正确且超限被夹到 2000；`frps/config` 回读能检出手工改动。
- **P2**：证书清单含宝塔自签证书与到期天数；语法错误片段部署时回传 reload 失败原因。
- **通用**：新端点全部走 `s.auth`；`go vet` / `go test ./internal/...` / `vue-tsc` 全绿；agent 版本不匹配时前端一律显示升级提示而不是报错。

---

## 八、风险与回滚

| 风险 | 说明 | 应对 |
| --- | --- | --- |
| **新老版本错配** | Fonu 已升级、VPS 上的 agent 仍是旧版 | 应用侧对新端点统一做 404/405 降级，前端显示「请升级 Agent」；不改动已有端点的行为 |
| **只读端点的权限** | `/var/log/nginx` 与证书目录可能不允许 agent 运行用户读取 | 端点返回明确的 403 + 路径，前端提示「agent 无读取权限」；安装脚本里补充读权限说明 |
| **`metrics` 采样抖动** | 高频调用时 CPU 差值失真 | 间隔 < 1 秒直接返回上次结果；前端按 15 秒轮询（与本地仪表盘一致） |
| **全量重部署覆盖手工改动** | 会覆盖 VPS 上手工编辑过的 `fonu-*.conf` | 按钮带二次确认 + 差异预览；`RouteConf` 回读内容先展示再覆盖 |
| **回滚** | agent 行为变更后需要退回旧版本 | agent 二进制随 Fonu 版本走：用旧版 Fonu 的「升级 Agent」即可把旧二进制重新上传；新增端点均为**纯增量**，不改变已有端点语义，应用侧回滚也无需数据迁移 |

**总原则**：新增端点一律只读或纯增量、不改变已有端点语义；所有 agent 交互都保持「Fonu 主动拉取、agent 被动应答」的现有模型，不引入 agent 主动上报。
