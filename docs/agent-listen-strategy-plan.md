# Agent 默认监听策略与自动安全接入方案

> 方案编号：P0-5  
> 制定日期：2026-09-28  
> 当前 AgentVersion：0.13.0  
> 目标：默认不暴露 Agent 管理接口，安装和升级过程自动建立安全连接，不要求用户手工配置端口、隧道或证书

> 实施状态（2026-09-28）：阶段 1-4 已随 v1.4.0 实施；仅剩 2.0.0 的旧 HTTP 兼容清理和可选主机防火墙辅助。

---

## 一、结论

推荐采用**自动为主、安全优先、旧模式显式兼容**的三层策略：

1. **默认模式：SSH 隧道 + 回环监听**
   - `fonu-agent` 默认只监听 `127.0.0.1:7700`。
   - 中央 Fonu 使用现有 SSH 凭据建立本地转发：

     ```text
     中央 Fonu 127.0.0.1:<随机本地端口>
       → SSH 加密连接
       → VPS 127.0.0.1:7700
       → fonu-agent
     ```

   - 不开放公网 Agent 端口，不需要域名、证书、云安全组或防火墙修改。

2. **可选模式：自动 HTTPS 反代 + 证书固定**
   - Agent 仍只监听回环地址。
   - HTTPS 不作为首次安装选项；Agent 安全安装完成后，用户可在「公网 Agent」页切换到该高级模式。
   - 切换时自动配置 Nginx HTTPS 反代，并使用自签名证书。
   - 中央 Fonu 首次安装时记录证书指纹，后续按指纹校验。
   - 适用于希望免维护常驻 SSH 隧道、且愿意开放一个 HTTPS 端口的用户。

3. **兼容模式：显式公网明文 HTTP**
   - 保留旧部署能力，但必须显式开启。
   - UI 持续显示高危警告。
   - 不允许自动降级到该模式。

默认安装与升级过程不需要用户设置 `FONU_AGENT_LISTEN`、TLS 证书或防火墙规则。

---

## 二、现状

### 2.1 Agent 监听

当前入口：

```go
listenv := flag.String("listen", envOr("FONU_AGENT_LISTEN", ":7700"), ...)
```

`internal/agent.NewServer` 空配置时同样回落到：

```go
cfg.ListenAddr = ":7700"
```

`:7700` 等价于监听所有 IPv4 / IPv6 网卡。

### 2.2 传输安全

当前 Agent 使用明文 HTTP：

- Bearer Token 在网络上直接传输；
- Token 已采用常量时间比较；
- Agent 默认没有 TLS；
- 安装脚本打印：

  ```text
  API: http://<VPS-IP>:7700
  ```

### 2.3 中央 Fonu 连接

安装完成后，`AgentInstall` 写入：

```go
AgentURL = fmt.Sprintf("http://%s:%d", server.SSHHost, 7700)
```

中央 Fonu 后续直接访问 VPS 公网 IP 的 7700 端口。

### 2.4 当前问题

- 用户需要额外放行或确认 7700；
- 如果防火墙忘记放行，Agent 显示不可达；
- 如果放行，Token 暴露在明文网络中；
- 部署工具和容器环境可能改变监听行为；
- “默认安全”没有形成产品行为；
- 安全策略主要依赖用户理解和手工操作。

---

## 三、设计目标

### 3.1 必须实现

1. 新安装默认不监听公网网卡。
2. 安装过程自动建立中央 Fonu 到 Agent 的安全连接。
3. 不需要用户手工配置网络或证书。
4. Agent 升级时可自动迁移旧监听配置。
5. 旧部署仍可显式使用公网 HTTP。
6. 传输失败时给出可执行的诊断信息。
7. 中央 Fonu 重启后可自动恢复连接。
8. Agent 与中央 Fonu 的版本兼容策略保持清晰。

### 3.2 非目标

- 不尝试自动修改云厂商安全组；
- 不要求用户申请域名或公网 CA 证书；
- 不引入多租户权限模型；
- 不把 Agent 管理 API 暴露给家庭用户的普通浏览器；
- 不提供一个“自动回退到明文 HTTP”的危险路径。

---

## 四、方案对比

### 4.1 方案 A：公网 HTTP + Bearer Token（现状）

| 项目 | 评价 |
| --- | --- |
| 自动化 | 高 |
| 安全性 | 低 |
| 防火墙 | 需要放行 7700 |
| 证书 | 不需要 |
| 复杂度 | 低 |
| 主要风险 | Token 明文、端口暴露、被动扫描 |

结论：只保留为显式兼容模式，不再作为默认。

### 4.2 方案 B：SSH 隧道 + Agent 回环监听（推荐默认）

| 项目 | 评价 |
| --- | --- |
| 自动化 | 很高 |
| 安全性 | 高 |
| 防火墙 | 不需要新增端口 |
| 证书 | 不需要 |
| 复杂度 | 中高，需要隧道生命周期管理 |
| 依赖 | SSH 可达；本功能安装 Agent 时已经要求 SSH |
| 主要优势 | 不暴露端口、不依赖域名和证书 |

结论：作为默认模式。

### 4.3 方案 C：Nginx HTTPS + 自签名证书固定

| 项目 | 评价 |
| --- | --- |
| 自动化 | 高 |
| 安全性 | 高 |
| 防火墙 | 需要放行一个 HTTPS 端口 |
| 证书 | 自动生成和固定，不需要公网 CA |
| 复杂度 | 中 |
| 主要优势 | 中央 Fonu 可直接连接，不依赖常驻 SSH |
| 主要限制 | 云安全组仍需用户放行端口 |

结论：作为可选直连模式。

### 4.4 方案 D：公网域名 + ACME 证书

| 项目 | 评价 |
| --- | --- |
| 自动化 | 中 |
| 安全性 | 高 |
| 防火墙 | 需要 443 或 DNS-01 |
| 证书 | 需要域名、DNS 与 CA |
| 复杂度 | 高 |
| 用户成本 | 高 |

结论：不作为默认。可作为未来面向高级用户的能力。

---

## 五、推荐架构

### 5.1 默认数据流

```mermaid
flowchart LR
    Web[Fonu Web] --> API[Fonu 后端]
    API --> Tunnel[SSH 隧道管理器]
    Tunnel -->|127.0.0.1:自动分配端口| SSH[SSH 加密连接]
    SSH --> VPSLoopback[VPS 127.0.0.1:7700]
    VPSLoopback --> Agent[fonu-agent]
```

### 5.2 角色职责

| 组件 | 职责 |
| --- | --- |
| Fonu 后端 | 决定 transport、启动隧道、调用 AgentClient |
| 隧道管理器 | 建立 SSH 本地转发、保持连接、重连、健康检查 |
| fonu-agent | 默认只监听回环，不再负责公网暴露 |
| 安装脚本 | 写入 `FONU_AGENT_LISTEN=127.0.0.1:7700`，禁止默认可公开 |
| UI | 显示安全模式、连接状态、可选迁移和显式旧模式警告 |

### 5.3 连接抽象

中央 Fonu 不再把“公网 IP + 7700”当作 Agent 的唯一定义。新增 transport 概念：

```text
tunnel     SSH 本地转发，默认
https-pin  Nginx HTTPS + 证书指纹固定，可选
http      公网明文，仅显式兼容
```

`AgentClient` 仍使用普通 HTTP 客户端接口，但 baseURL 由 transport 管理：

- tunnel：`http://127.0.0.1:<localPort>`
- https-pin：`https://<host>:<port>`
- http：`http://<host>:7700`

---

## 六、默认模式：SSH 隧道

### 6.1 为什么默认走 SSH

安装 Agent 已经必须使用 SSH，因此：

- 不新增用户操作；
- 不需要公网开放 7700；
- 不需要证书和域名；
- 不依赖云安全组修改；
- SSH 主机密钥已经使用 TOFU 校验；
- Token 继续保留，形成双层认证。

### 6.2 本地端口分配

每个 VPS 服务端分配一个稳定的本地回环端口：

```text
127.0.0.1:<agent_local_port>
```

分配方式建议：

1. 首次安装时在 `frp_servers.agent_local_port` 中保存；
2. 可在 `17700-18700` 范围内分配；
3. 分配时检查：
   - 未被其他服务端占用；
   - 未被本机其他 SSH 隧道占用；
   - 可以正常绑定。
4. 保存后不再变化，便于重启恢复。

### 6.3 隧道管理器

新增：

```text
internal/frp/agent_tunnel.go
```

核心能力：

- `Ensure(serverID)`：确保隧道已启动；
- `Stop(serverID)`：停止隧道；
- `Restart(serverID)`：重建；
- `State(serverID)`：返回 connected / reconnecting / failed；
- 应用退出时统一关闭。

实现建议：

- 使用现有 `golang.org/x/crypto/ssh`；
- 复用 `SSHInstaller` 的 known_hosts 校验；
- SSH 连接使用 keepalive；
- 本地监听使用 `127.0.0.1`，不绑定 `0.0.0.0`；
- 连接断开后指数退避重连；
- 每个服务端最多一个隧道实例；
- 同一 `AgentClient` 复用 HTTP 连接。

### 6.4 健康检查

隧道建立后执行：

1. 本地端口可以连接；
2. `GET /api/v1/ping` 返回 200；
3. Bearer Token 验证通过；
4. `GET /api/v1/status` 返回 Agent 版本。

只有四项都通过，才把 transport 标记为 connected。

### 6.5 连接状态

前端需要显示：

```text
传输方式：SSH 隧道
本地端口：127.0.0.1:177xx
Agent 地址：127.0.0.1:7700（VPS 回环）
隧道状态：已连接 / 重连中 / 失败
最后错误
```

---

## 七、Agent 默认监听与防误开

### 7.1 默认值

修改：

```go
const defaultListenAddr = "127.0.0.1:7700"
```

以下位置同步：

- `cmd/fonu-agent/main.go`
- `internal/agent.NewServer`
- `scripts/agent-install.sh`
- `internal/frp/agent_installer.go` 内嵌安装脚本

### 7.2 远程监听保护

如果监听地址不是回环：

```text
0.0.0.0:7700
:7700
[::]:7700
VPS公网IP:7700
```

则必须显式设置：

```dotenv
FONU_AGENT_ALLOW_REMOTE=1
```

否则 Agent 拒绝启动并输出：

```text
检测到 Agent 将监听非回环地址，但未显式允许远程访问。
默认安全模式只监听 127.0.0.1:7700。
如确认需要公网明文访问，请设置 FONU_AGENT_ALLOW_REMOTE=1。
```

### 7.3 旧配置迁移

旧版本 `agent.env` 可能没有 `FONU_AGENT_LISTEN`，此时：

1. 新 Agent 默认监听回环；
2. 安装脚本显式写入：

   ```dotenv
   FONU_AGENT_LISTEN=127.0.0.1:7700
   ```

3. 中央 Fonu 在执行「升级 Agent」时同步建立隧道；
4. 升级完成后切换 transport；
5. 旧公网地址从 `agent_url` 中移除；
6. 不停留手工维护窗口。

### 7.4 状态接口

Agent `/api/v1/status` 新增：

```json
{
  "listen_addr": "127.0.0.1:7700",
  "listen_scope": "loopback",
  "remote_access_enabled": false,
  "transport_security": "loopback"
}
```

---

## 八、自动安装流程

### 8.1 新安装

1. 用户填写 VPS SSH 信息。
2. 中央 Fonu 通过 SSH 探测系统与 Nginx。
3. 分配 `agent_local_port`。
4. 上传 Agent 二进制。
5. 安装脚本写入：
   - Token；
   - `FONU_AGENT_LISTEN=127.0.0.1:7700`；
   - systemd 服务。
6. 启动 Agent。
7. 通过 SSH 本地转发建立隧道。
8. 调用：
   - `GET /api/v1/ping`
   - `GET /api/v1/status`
9. 保存：
   - token 密文；
   - transport=`tunnel`；
   - local port；
   - AgentVersion。
10. UI 显示“安全模式已启用”。

### 8.2 自动迁移旧安装

1. 检测现有 `agent_url` 是否为 `http://公网IP:7700`。
2. 显示“可升级到安全模式”，但不强制立即迁移。
3. 用户点击「升级 Agent」后：
   - 安装新二进制；
   - 重写 `agent.env` 为回环监听；
   - 重启服务；
   - 建立隧道；
   - 验证连通；
   - 切换 central transport。
4. 只有隧道验证成功后，才关闭旧公网连接。
5. 失败时保留旧 Agent 配置和旧连接，自动回滚。

### 8.3 应用重启恢复

1. 读取启用管理且 transport=tunnel 的服务端。
2. 逐个调用 `Tunnel.Ensure`。
3. 启动失败不阻塞整个 Fonu 启动。
4. UI 显示重连状态。
5. 后台按指数退避继续重试。

---

## 九、可选模式：HTTPS + 证书固定

### 9.1 适用场景

- 不希望长期维护 SSH 隧道；
- 可以接受开放一个 HTTPS 端口；
- 没有域名或不想申请公网证书；
- 希望 Agent API 可被中央 Fonu直接访问。

### 9.2 安装后切换流程

首次安装默认仍使用 SSH 隧道。只有用户在「公网 Agent」页明确切换到 HTTPS 直连时，才执行以下步骤：

1. Agent 仍监听 `127.0.0.1:7700`。
2. 切换流程生成自签名证书：

   ```text
   /etc/fonu-agent/tls/agent.crt
   /etc/fonu-agent/tls/agent.key
   ```

3. 自动寻找可用端口：
   - 优先 7443；
   - 其次 8443、9443；
   - 若 443 空闲且未占用可提供选项，但不默认抢占。
4. 生成独立 Nginx server block：

   ```nginx
   server {
       listen 7443 ssl;
       ssl_certificate /etc/fonu-agent/tls/agent.crt;
       ssl_certificate_key /etc/fonu-agent/tls/agent.key;

       location /api/v1/ {
           proxy_pass http://127.0.0.1:7700;
       }
   }
   ```

5. 返回证书 SHA-256 指纹。
6. 中央 Fonu 保存 pin，并使用自定义校验。

### 9.3 TLS 固定

不能使用：

```go
InsecureSkipVerify: true
```

应实现：

```go
VerifyPeerCertificate(rawCerts [][]byte, _ [][]*x509.Certificate) error {
    // 计算证书 DER 的 SHA-256
    // 与数据库中保存的 pin 做常量时间比较
    // 同时校验有效期与 SAN（如可校验）
}
```

首次安装必须通过 SSH 获取指纹，避免首次连接被中间人截获。

### 9.4 防火墙

首版不自动修改主机防火墙。阶段 4 之后，在用户明确确认时可以辅助生成并执行主机防火墙命令：

- ufw；
- firewalld；
- nftables / iptables。

云厂商安全组始终不能自动修改。UI 必须提示：

```text
请在云控制台放行 TCP 7443。主机防火墙需要你确认后由 Fonu 执行，Fonu 不会修改云安全组。
```

---

## 十、兼容模式：显式公网 HTTP

仅用于旧部署迁移或用户明确要求：

```dotenv
FONU_AGENT_LISTEN=:7700
FONU_AGENT_ALLOW_REMOTE=1
```

中央 Fonu UI 必须：

- 显示“公网明文传输”红色状态；
- 不在新安装中默认选择；
- 不支持由自动模式静默回退；
- 显示迁移到 SSH 隧道或 HTTPS 的按钮；
- 在变更时记录审计事件。

旧 transport 允许读取和继续连接，但所有新增安装默认使用 tunnel。

---

## 十一、数据模型变更

建议新增迁移：

```sql
ALTER TABLE frp_servers ADD COLUMN agent_transport TEXT NOT NULL DEFAULT 'http';
ALTER TABLE frp_servers ADD COLUMN agent_local_port INTEGER NOT NULL DEFAULT 0;
ALTER TABLE frp_servers ADD COLUMN agent_tls_pin TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN agent_listen_addr TEXT NOT NULL DEFAULT '';
ALTER TABLE frp_servers ADD COLUMN agent_transport_state TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE frp_servers ADD COLUMN agent_transport_error TEXT NOT NULL DEFAULT '';
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `agent_transport` | `tunnel` / `https-pin` / `http` |
| `agent_local_port` | 隧道本地回环端口 |
| `agent_tls_pin` | HTTPS 证书 DER SHA-256 |
| `agent_listen_addr` | Agent 实际监听地址缓存 |
| `agent_transport_state` | `connected` / `reconnecting` / `failed` / `unknown` |
| `agent_transport_error` | 最近一次连接错误 |

### 11.1 兼容迁移

旧数据：

```text
agent_transport = http
```

新安装：

```text
agent_transport = tunnel
```

---

## 十二、API 与前端变更

### 12.1 API

`FrpServer` JSON 增加：

```json
{
  "agent_transport": "tunnel",
  "agent_transport_state": "connected",
  "agent_local_port": 17701,
  "agent_listen_addr": "127.0.0.1:7700",
  "agent_transport_error": ""
}
```

新增管理端点：

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| POST | `/api/frpmulti/servers/{id}/agent/transport` | 切换 tunnel / https-pin / http |
| POST | `/api/frpmulti/servers/{id}/agent/tunnel/restart` | 重启隧道 |
| GET | `/api/frpmulti/servers/{id}/agent/transport` | 读取详细状态 |

### 12.2 前端

「公网 Agent」页新增传输模式卡：

- SSH 隧道，默认；
- HTTPS + 证书固定，可选；
- 公网 HTTP 明文，兼容。

显示：

```text
传输方式：SSH 隧道
隧道状态：已连接
本地入口：127.0.0.1:17701
Agent 监听：127.0.0.1:7700
安全等级：安全
```

旧模式显示：

```text
公网明文 HTTP
风险：Token 会通过网络明文传输
建议迁移到 SSH 隧道
```

---

## 十三、自动回滚

迁移或切换 transport 失败时：

1. 保留旧 transport 与旧 Agent 配置；
2. 不覆盖旧 Token；
3. 不删除旧公网端口配置；
4. 如果已经重写 `agent.env`，恢复旧文件；
5. 如果 Agent 已重启但新连接失败，重新启动旧服务；
6. 向 UI 返回完整错误和回滚结果。

任何自动迁移都必须满足：

```text
新连接验证成功
→ 才提交数据库 transport
→ 才停止旧监听
```

---

## 十四、测试计划

### 14.1 单元测试

- 默认监听地址是回环；
- 非回环监听缺少 `FONU_AGENT_ALLOW_REMOTE` 时拒绝启动；
- 回环监听无需额外变量；
- `agent_transport` 序列化与旧数据兼容；
- 隧道本地端口分配、冲突与释放；
- 隧道重连、退避和关闭；
- 证书 pin 正确、证书变化、pin 不匹配时拒绝；
- 旧 HTTP transport 继续可用并产生警告。

### 14.2 集成测试

- 在测试 SSH 服务上建立本地转发；
- Agent 只监听 127.0.0.1；
- 应用重启后隧道自动恢复；
- SSH 断开后重连；
- 多服务端并发隧道；
- 迁移前失败回滚；
- 迁移成功后旧公网端口关闭。

### 14.3 真机验证

至少覆盖：

- Debian / Ubuntu / CentOS；
- fnOS 中的 Fonu；
- 宝塔 Nginx；
- 已存在 80 / 443 / 7443 占用的 VPS；
- 云安全组只开放 SSH 的 VPS；
- 云安全组开放 7443 的可选 HTTPS 模式。

### 14.4 验收条件

- 新安装后 `ss -tlnp` 只能看到 Agent 监听 `127.0.0.1:7700`；
- 公网扫描不到 7700；
- 无需新增云安全组规则；
- UI 显示隧道已连接；
- Fonu 重启后自动恢复；
- 「升级 Agent」可以自动迁移旧配置；
- 失败时不会丢失旧连接。

---

## 十五、实施阶段

### 阶段 1：默认回环与防误开

- Agent 默认监听改为 `127.0.0.1:7700`；
- 非回环监听要求 `FONU_AGENT_ALLOW_REMOTE=1`；
- 安装脚本显式写入监听地址；
- UI 显示监听范围。

该阶段暂不自动切换中央连接，避免破坏旧部署。

### 阶段 2：SSH 隧道管理器

- 新增数据库字段；
- 新增隧道管理器和 AgentClient transport；
- 安装后自动建立隧道；
- 应用重启恢复；
- UI 显示 transport 状态。

### 阶段 3：自动迁移旧 Agent

- 检测旧公网 HTTP；
- 「升级 Agent」时执行安全迁移；
- 成功切换后关闭旧入口；
- 失败自动回滚。

### 阶段 4：可选 HTTPS + pin

- 自动生成证书；
- Nginx server block；
- TLS pin；
- 防火墙辅助；
- UI 可选切换。

### 阶段 5：收紧默认

- 新安装不再接受公网 HTTP；
- 兼容模式需要明确的高风险确认；
- 文档和通知引导迁移。

---

## 十六、决策结论

以下决策已于 2026-09-28 确定，后续实施按此执行。

| 编号 | 决策 | 执行设定 | 理由 |
| --- | --- | --- | --- |
| 1 | SSH 隧道作为唯一默认模式 | 首次安装不提供模式选择，统一安装为回环监听 + SSH 隧道；HTTPS 直连只作为安装后的高级切换项 | 现有安装流程已经依赖 SSH；默认路径必须最少输入、最少网络要求 |
| 2 | 隧道本地端口使用固定范围 | 固定为 `17700-18700`，绑定 `127.0.0.1`；分配结果持久化；启动时先复用持久化端口，再在范围内顺序查找空闲端口；范围耗尽时明确报错，不使用临时端口 | 固定范围便于排查和审计；持久化避免每次重启变化；不回退临时端口避免地址漂移 |
| 3 | 隧道跟随主应用生命周期 | 使用应用内 `agent_tunnel` 管理器，在 Fonu 启动后按服务端恢复；应用退出时统一关闭；断线后指数退避重连；首版不新增独立 systemd 服务 | 当前项目是单进程控制面，调度器、FRP、监控均在同一生命周期内，新增守护进程会扩大部署复杂度 |
| 4 | 公网 HTTP 保留整个 1.x 兼容期 | 自 `1.4.0` 起不再允许新安装选择 HTTP；存量连接继续可用但显示高危警告并提供迁移；计划在 `2.0.0` 移除 | 需要给旧 Agent 和环境留出迁移窗口，同时从版本边界上明确技术债的截止点 |
| 5 | HTTPS 自签名证书使用短期自动轮换 | ECDSA P-256，有效期 365 天；剩余 30 天时自动轮换；轮换期间同时接受旧 pin 和新 pin，双 pin 窗口 7 天；优先通过现有 SSH 通道获取新指纹后再切换 | 避免长期证书失管；双 pin 防止轮换瞬间把所有中央连接锁死 |
| 6 | 首版不自动改主机防火墙 | 首先只检测、展示和提示；SSH 隧道模式本身不需要放行新端口；HTTPS 高级模式后续再提供显式的“一键放行主机防火墙”操作，并始终不修改云厂商安全组 | 防火墙和云安全组属于高风险外部状态；首版先把安全默认值落地，避免自动变更导致 SSH 或现有服务失联 |

### 16.1 决策对实施阶段的影响

- 阶段 1 只做 Agent 默认回环和防误开。
- 阶段 2 实现 `17700-18700` 的持久化端口分配与应用内隧道管理器。
- 阶段 3 完成旧 Agent 自动迁移；迁移失败必须回到旧 transport。
- 阶段 4 增加 HTTPS + pin 的高级模式、证书轮换和双 pin 窗口。
- `1.4.0` 起前端不再允许新建 HTTP transport。
- `2.0.0` 删除 `http` transport 和旧 Agent 兼容分支。
- 防火墙变更仅在阶段 4 之后按用户明确操作执行，首版不实现自动写规则。

---

## 十七、最终建议

优先实现前三个阶段：

1. Agent 默认回环；
2. SSH 隧道自动建立与恢复；
3. 旧 Agent 自动迁移并支持回滚。

这三步完成后，默认部署不再暴露公网 7700，也不需要用户修改 Composer 或防火墙。HTTPS + pin 与防火墙辅助放在后续阶段，作为需要直连用户的可选增强。这样可以在不扩大用户操作成本的前提下，完成 P0-5 的安全目标。
