# FRPS 服务端自动管理方案（frps-manager）

> 目标：frpc 端在 Fonu 提交穿透规则后，frps 端**自动完成反向代理配置与生效**，实现域名无端口访问（`http://tv.36800.cc`），并自动管理 VPS 侧证书。本文是基于现有项目（Fonu v1.1.0 FRP 多服务端模块）的实施方案，不引入新服务、不改 frp 二进制。

## 1. 现状与问题

当前（v1.1.0）frpc 侧规则提交后：

| 环节 | 现状 | 问题 |
| --- | --- | --- |
| tcp 规则注册（远程端口） | ✅ 自动（frpc 登录时向 frps 动态注册） | 无 |
| http 规则域名路由 | ⚠️ 需 frps.toml 预先配置 `vhostHTTPPort` | 首次使用 http 类型时 VPS 要手工改配置+重启 |
| frps.toml 生成 | ✅ Fonu 一键生成，用户手动复制到 VPS | 每次变更都要 SSH 手工同步 |
| VPS 端口释放（80/443 被占） | ❌ 手动 SSH 停 Nginx | `port unavailable` 最常见根因 |
| 改配置后生效 | ❌ 手动 `pkill frps` 重启（frp 无热加载） | 容易忘、容易误 |
| VPS 侧证书 | ❌ 无 | HTTPS 要在 VPS 手工装证书 |
| 配置漂移检测 | ❌ 无 | VPS 上的文件与 Fonu 数据库可能不一致 |

## 2. 总体设计

在 Fonu（frpc 所在机器）新增 **frps-manager** 子系统：Fonu 通过 **SSH** 连接 VPS，执行「检测 → 生成 → 推送 → 生效 → 验证」闭环。VPS 侧**零安装**（只要求 frps + SSH）。

```text
Fonu (NAS)                                VPS (38.95.74.87)
┌─────────────────────────────┐           ┌──────────────────────────┐
│ FRP 模块（现有）             │           │ frps (7000/80/443/7500)  │
│  规则/服务端 CRUD            │           │  ↑ frpc 动态注册端口      │
│ frps-manager（新增）         │   SSH     │  frps.toml (推送产物)     │
│  检测→生成→推送→reload→验证  │ ────────→ │  /etc/fonu/frps/         │
│  证书申请→推送→挂载          │           │  /etc/fonu/frps/certs/   │
└─────────────────────────────┘           └──────────────────────────┘
```

核心思路：**frps 不认识"反代"，它只认 frps.toml**。因此"自动反代"= ① 规则变化时重算出应然的 frps.toml（含 vhost 端口、subDomainHost、TLS 配置）→ ② 与 VPS 现状 diff → ③ 无变化则跳过，有变化则原子推送 → ④ 通过 frps **admin API 热重载**（无需重启进程）→ ⑤ 回读验证。证书同理：Fonu 证书页现有 ACME 能力签好后把证书文件推到 VPS 并写入 frps.toml 的 `transport.tls`/`webServer.tls` 段。

## 3. 服务端模型扩展（frp_servers 表）

新增「服务端连接信息」（加密存储，复用 secret.Box）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ssh_host` | TEXT | VPS SSH 地址（可与 server_addr 不同） |
| `ssh_port` | INTEGER | 默认 22 |
| `ssh_user` | TEXT | 建议 root 或具备 sudo 的用户 |
| `ssh_auth` | TEXT | `password` / `key` |
| `ssh_secret_enc` | TEXT | 密码或私钥（加密） |
| `frps_dir` | TEXT | VPS 上 frps 目录，默认 `/etc/fonu/frps` |
| `frps_bin` | TEXT | frps 二进制路径，默认 `/usr/local/bin/frps` |
| `mgmt_enabled` | INTEGER | 是否启用自动管理（0/1，默认 0=纯手动，向后兼容） |

迁移 `023_frp_server_ssh.sql`。SSH 凭据用 secret.Box 加密，接口不回显（同 Token 处理）。

## 4. 核心流程

### 4.1 检测（Probe）——服务端详情新增「检测服务端」按钮

一次 SSH 会话内批量执行（只读，不改任何东西）：

```bash
frps --version                          # 版本
ss -tlnp | grep -E ':(80|443|7000) '    # 端口占用者（frps? nginx? 其他?）
systemctl is-active nginx frps          # 服务状态
cat /proc/$(pidof frps)/cmdline         # 实际使用的配置路径
cat <配置路径>                           # 现有 frps.toml 内容（做 diff）
```

返回结构化报告：frps 版本/运行状态/配置路径、80/443 占用者判定（frps/nginx/其他/空闲）、现有 frps.toml 与 Fonu 期望配置的 **diff 摘要**、待办建议（如「80 被 nginx 占用，需释放」）。这就是用户之前反复手工排查的自动化。

### 4.2 下发（Apply）——「自动反代」的主流程

触发时机：用户点「下发配置」，或规则保存后若 `mgmt_enabled` 且检测到配置将变化（规则含 http 类型 / 证书变更）自动触发。

1. **生成期望配置**：扩展现有 `buildFRPSServerConfig`：
   - 存在 http/https/tcpmux 规则 → 输出 `vhostHTTPPort = 80`、`vhostHTTPSPort = 443`（去端口访问的关键）；
   - 存在 subdomain 规则 → 输出 `subDomainHost = <用户在服务端表单新填的主域>`；
   - 证书启用时输出 `transport.tls.force` / `webServer.tls` 段；
2. **SSH 推送（原子）**：新内容写 `/etc/fonu/frps/frps.toml.new` → `mv` 原子替换 → 旧文件留 `frps.toml.bak`（回滚点）；
3. **热重载**：调 frps admin API `PUT /api/reload`（frps webServer 7500 已有 Basic Auth 凭据）——**不杀进程**，连接不断；API 不可用时降级 `systemctl restart frps`（nohup 场景提示用户）；
4. **验证**：重读 VPS 上的 frps.toml 确认与期望一致 + `ss` 确认 80/443 已由 frps 监听 + 通过公网侧发起一次探测请求；
5. 全程动作与输出写入系统日志（复用 logstore），UI 弹窗展示每步结果。

### 4.3 释放端口（安全加固提示，不偷偷杀人）

检测发现 80/443 被 nginx 等占用时，**不自动杀进程**（危险动作），而是：
- 检测报告中标红 + 给出一键命令；
- UI 提供「释放端口」按钮：二次确认后执行 `systemctl stop nginx && systemctl disable nginx`（仅当占用者确认为 nginx 时可点，其他占用者只提示不动手）；
- 释放后自动重跑 Apply 验证闭环。

### 4.4 证书管理（VPS 侧 HTTPS）

复用 Fonu 证书页现有 ACME（lego）能力，证书签发在 NAS 侧完成（DNS-01，无需 VPS 参与），然后：

1. 证书页选「部署到 VPS」→ 选目标服务端；
2. SSH 推送 `fullchain.pem` / `privkey.pem` 到 `/etc/fonu/frps/certs/<域名>/`（0600）；
3. frps.toml 写入 `webServer.tls.certFile/keyFile`（管理面板 HTTPS）；对外流量 TLS 若需要也在 frps 侧终止时，走 `transport.tls` 相关段（v0.71 支持 `transport.tls.force` + 证书）；
4. 续期：Fonu 证书定时任务续期成功后自动重推 + reload（证书 60 天滚动，VPS 侧无感）。

> 设计取舍：对外业务的 HTTPS 终止仍推荐在 **Fonu Nginx（NAS 侧）**（本手册方案 A 架构），frps 侧证书用于 webServer 管理面板与 `tls.force` 场景。若用户想让 VPS 终止 TLS，同一套推送机制也覆盖。

## 5. API 与 UI

### 5.1 新增 API（挂在 `/api/frpmulti` 下，鉴权与现有中间件一致）

| 方法/路径 | 作用 |
| --- | --- |
| `PUT /servers/{id}/ssh` | 保存 SSH 连接信息（加密存储） |
| `GET /servers/{id}/probe` | 检测：frps 状态/端口占用/配置 diff |
| `POST /servers/{id}/apply` | 下发期望配置 + reload + 验证 |
| `POST /servers/{id}/release-port` | 释放被占端口（body: `{port, confirm: true}`） |
| `POST /servers/{id}/push-cert` | 推送证书到 VPS 并挂载 |

### 5.2 前端（FrpView.vue）

- 服务端表单「高级配置」折叠区新增「远程管理」分组：SSH 地址/端口/用户/认证方式/密钥（ConfiguredSecretField，v-model）/frps 目录；
- 服务端列表操作列新增「管理」按钮 → 弹窗（复用 `proxy-modal` 自绘骨架）三块：检测报告（分色状态行）、diff 视图（`<pre>` 左右对照期望/现状）、操作按钮（下发配置 / 释放端口 / 推送证书）；
- 规则保存时的自动下发提示：非阻塞 toast「检测到 vhost 配置变化，点击下发」。

## 6. 实现拆解与工作量

| 模块 | 文件 | 内容 | 量级 |
| --- | --- | --- | --- |
| 迁移 | `migrations/023_frp_server_ssh.sql` | SSH 字段 | 小 |
| SSH 客户端 | `internal/frp/ssh.go`（新） | 连接池/单会话命令执行/SFTP 推送/超时 | 中（x/crypto/ssh 已在 go.mod） |
| 检测 | `internal/frp/probe.go`（新） | 探测命令编排 + 结果结构体 + 80 占用者判定 | 中 |
| 配置 diff/应用 | `internal/frp/frps_apply.go`（新） | 期望配置生成（扩展现有 frps_template）、diff、原子推送、admin API reload、验证 | 中 |
| 证书推送 | `internal/frp/cert_push.go`（新） | 证书文件推送 + frps.toml 证书段写入 | 小 |
| Service/API | `frpmulti_handler.go` + `router.go` | 5 个端点 | 小 |
| 前端 | `FrpView.vue` + `client.ts` | 管理弹窗 + 表单分组 + 按钮 | 中 |
| 测试 | `probe_test.go` `frps_apply_test.go` | 命令输出解析、diff、原子写逻辑 | 中 |

总计约 1500–2000 行（含测试），纯增量、不动现有链路（`mgmt_enabled=0` 时行为与 v1.1.0 完全一致）。

## 7. 安全设计

1. SSH 凭据 secret.Box 加密落库，API 永不回显（同 token 掩码机制）；
2. 推荐文档引导用户用**密钥登录**（Fonu 可一键生成 ed25519 密钥对、展示公钥让用户贴到 VPS `authorized_keys`）；
3. 所有远程命令为**固定白名单模板**（版本/端口/状态/替换文件/reload），不拼接用户输入，杜绝命令注入；SFTP 只写 `/etc/fonu/frps/` 前缀内路径；
4. 危险动作（释放端口、重启 frps）二次确认 + 动作日志；
5. Apply 幂等：重复执行无副作用（diff 无变化即跳过）；
6. frps admin API 调用走本机 127.0.0.1:7500（SSH 远端执行 curl），不暴露公网。

## 8. 里程碑

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| M1 | 迁移 + SSH 客户端 + 检测（Probe） | 服务端「检测」按钮能报告 frps 状态与 80 占用者 |
| M2 | 期望配置生成 + diff + 原子推送 + reload + 验证 | 改规则后一键下发，`http://域名` 直达，`port unavailable` 类问题由系统自愈 |
| M3 | 证书推送 + 续期联动 | 证书页一键部署到 VPS，续期自动重推 |
| M4 | UI 完善 + 文档（手册第 6 节追加方案 A'） | 手册更新，`port unavailable` 类问题由系统自愈 |

## 9. 明确不做（本期）

- 不实现 VPS 伴生 agent（能力更强但需安装，作为后续可选演进）；
- 不做 frps 二进制的远程安装/升级（用户自装，检测报告给出版本即可）；
- 不做多 VPS 并行下发编排（现有逐服务端模型已覆盖）；
- 不改 frp 二进制、不依赖 frp 私有接口（只用 admin API 与文件系统）。
