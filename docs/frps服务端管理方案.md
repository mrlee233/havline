# FRPS 服务端管理方案（havline-agent，部署于公网服务器）

> 需求：frpc 端（Havline/NAS）提交穿透规则后，**公网服务器端自动完成反向代理设置**，实现域名无端口访问（`http://tv.36800.cc`），并自动管理服务器侧证书与 frps。
> 实现形态：在公网 VPS 上部署一个常驻管理组件 **havline-agent**（单个 Go 二进制 + systemd），对外提供管理 API；Havline 在规则变化时调用该 API，agent 在 VPS 本地完成 Nginx 反代配置、frps 管理与证书部署。
> **安装与管理通道**：agent 的**首次安装、升级、卸载通过 SSH 由 Havline 远程完成**（用户在 Havline 界面填 SSH 信息点按钮即可，无需手工登 VPS）；装好后 agent 以 **API** 接受日常管理。管理面板全部在本地 Havline，VPS 上没有任何面板。
> 修订记录：v2 混合形态（SSH 装/卸 + API 管理）；v1 为纯 SSH 推送方案，v2 初稿为「agent 纯 API」方案，两版历史见文末对照。

## 1. 目标与效果

| 诉求 | 达成方式 |
| --- | --- |
| frpc 提交规则后 frps 端自动反代 | Havline 保存规则 → 调 agent API 注册域名路由 → agent 写 Nginx vhost 并 reload |
| 无端口访问 | agent 的 Nginx 监听 80（443），`proxy_pass` 到 frps vhost 端口，URL 不带端口号 |
| 管理证书 | Havline 证书页（DNS-01）签发后一键推送到 agent，自动挂 443 并续期重推 |
| frps 管理 | 配置下发（vhost 端口等）、进程重启/状态/版本、端口占用诊断，全部 API 化 |
| VPS 上已有 Nginx/站点 | 共存：agent 只写自己的 vhost 文件（`conf.d/havline-*.conf`），不动其他配置 |

最终链路（http 规则为例）：

```text
用户 → http://tv.36800.cc（80，无端口）
     → VPS Nginx（agent 生成的 vhost）→ 127.0.0.1:8080（frps vhostHTTPPort）
     → frps 按域名路由 → FRP 隧道 → NAS frpc → 192.168.31.152:7023
```

## 2. 总体架构

```text
Havline (NAS，管理面板所在地)                 公网服务器 (VPS)
┌──────────────────────────────┐          ┌────────────────────────────────┐
│ FRP 模块（现有）              │          │ frps   :7000 控制 / 8080 vhost  │
│  规则 CRUD / frpc / frps.toml │          │ nginx  :80/443                 │
│  证书页（lego DNS-01，已有）   │          │   └─ havline-*.conf（agent 产物）  │
│ ┌────────────────────┐       │          │ havline-agent :7700（常驻）        │
│ │ SSH 安装器（新增）   │──SSH──┼→ 首次安装/升级/卸载 agent            │
│ │ agent 客户端（新增） │──API──┼→ 日常管理：路由/配置/证书/状态        │
│ └────────────────────┘       │          │  ├─ 路由管理（nginx vhost 编排） │
│  管理面板 = Havline 网页          │          │  ├─ frps 管理（配置/进程/诊断）   │
│  （规则页/证书页/管理弹窗）     │          │  └─ 证书接收与挂载               │
└──────────────────────────────┘          └────────────────────────────────┘
```

两条通道各司其职：

- **SSH（低频）**：仅用于 agent 生命周期——安装、升级、卸载、agent 失联自救。用户在 Havline 填一次 SSH 信息（可选保存备用）；
- **API（高频）**：日常全部管理——规则同步、frps 配置下发、证书推送、状态查询。面板 100% 在 Havline 本地，VPS 上没有任何管理界面。

职责划分：Havline 是**控制面**（用户在哪配置就存哪），agent 是**执行面**（只接收指令并落实），frps/Nginx 是**数据面**。agent 无状态（配置即文件），可随时重装。

## 3. havline-agent 设计（VPS 侧）

单个 Go 二进制（复用主项目代码编译，`cmd/havline-agent`），systemd 常驻，管理 API 监听 `:7700`（可配置）。

### 3.1 管理 API（Bearer Token 鉴权）

| 方法/路径 | 作用 |
| --- | --- |
| `GET  /api/v1/ping` | 连通性/鉴权校验（Havline「测试连接」按钮） |
| `GET  /api/v1/status` | frps 版本/运行方式/配置路径、80/443/7000/8080 端口占用者、已注册路由、证书有效期 |
| `PUT  /api/v1/routes` | 注册/更新域名路由：`{domain, upstream(=frps vhost 端口), tls, cert_id}` → 写 Nginx vhost → `nginx -t` → reload（失败自动回滚文件） |
| `DELETE /api/v1/routes/{domain}` | 注销路由（规则删除/停用时调用） |
| `GET  /api/v1/routes` | 当前路由清单（对账用） |
| `PUT  /api/v1/frps/config` | 下发完整 frps.toml（Havline 同源生成）→ 备份旧文件 → 按运行方式重启/重载 → 回读验证 |
| `POST /api/v1/certs` | 部署证书：`{domain, fullchain_pem, key_pem}` → 写 `/var/lib/havline-agent/certs/<domain>/`（0600）→ 更新 443 vhost → reload |

### 3.2 Nginx 编排（反代核心）

- 每个域名一个文件：`/etc/nginx/conf.d/havline-tv.36800.cc.conf`，模板：

```nginx
server {
    listen 80;
    server_name tv.36800.cc;
    location / {
        proxy_pass http://127.0.0.1:8080;        # frps vhostHTTPPort
        proxy_set_header Host $host;             # frps 按域名分流，必须保留
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_http_version 1.1;
    }
}
# 启用证书后追加 443 server 块（ssl_certificate 指向 agent 证书目录）+ 80 跳转
```

- 写入流程：写 `.tmp` → `nginx -t` 校验 → 通过则替换并 `nginx -s reload`，失败保留原文件并把 `nginx -t` 输出回传给 Havline 显示；
- **只增删自己的 `havline-*.conf`**，绝不触碰其他配置 → 与宝塔/手工站点共存；
- 首次运行自检：nginx 是否安装、`conf.d`/`sites-enabled` 目录探测、80/443 监听者识别（nginx? frps? 其他?）。

### 3.3 frps 管理

- **配置下发**：Havline「frps.toml 同源生成」已有（含 vhost 端口、auth、webServer），改为推送给 agent 落盘：备份旧文件 → 原子替换 → 按运行方式生效（systemd 单元 `systemctl restart frps`；nohup 场景 `pkill frps` 后按原命令拉起，agent 从状态接口学习原启动命令）；
- **诊断**：`status` 聚合 frps 版本/存活/配置路径 + `ss -tlnp` 端口占用判定——把之前手工排查（`port unavailable`、80 被占）变成 API 一次调用；
- **边界**：不安装/升级 frps 二进制（检测报告提示版本即可）。

### 3.4 证书

- 签发仍在 **Havline 证书页**（lego DNS-01，复用 DDNS 凭证，NAS 侧完成，VPS 无需参与）；
- 部署：证书页「部署到服务器」→ `POST /certs` 推送 PEM → agent 落盘并生成 443 vhost → reload；
- 续期：Havline 定时任务续期成功后自动重推，VPS 侧无感；
- （可选演进）agent 本地 ACME HTTP-01：agent 已在 80 上，可自助签发，作为无 DNS 凭证场景的备选，本期不做。

## 4. 通信协议与安全

| 项 | 设计 |
| --- | --- |
| 鉴权 | Bearer Token（Havline 服务端表单生成/粘贴，agent 配置文件持有；两端一致即可用），`7700` 建议用防火墙限制仅 NAS IP 访问 |
| 传输 | 明文 HTTP 起步可接受（Token + 内网限制）；增强路径：agent 用 frps 证书或自签证书提供 HTTPS 管理口 |
| 命令安全 | agent 只执行固定动作（写文件/nginx -t/reload/重启 frps），**不执行 Havline 下发的任意命令**；域名/路径参数做白名单校验（域名格式、路径锁定 `conf.d/havline-*` 与 `/var/lib/havline-agent/`） |
| 幂等 | 路由/配置/证书接口全部幂等，重复调用无副作用；`routes` 对账接口支持 Havline 定时校准 |
| 审计 | agent 每个写操作记本地日志（who/when/what），Havline 侧同步状态入系统日志 |

## 5. Havline 侧改动（frpc 端）

| 模块 | 内容 |
| --- | --- |
| 数据模型 | `frp_servers` 加 `agent_url`、`agent_token_enc`（secret.Box 加密不回显）、`mgmt_enabled`（默认 0，行为与 v1.1.0 完全一致）；迁移 `023_frp_server_agent.sql` |
| 规则联动 | 规则保存/删除/启停且 `mgmt_enabled` 时，自动调 agent 路由接口（http/https 类型域名规则）；失败不阻塞保存，规则行显示同步状态（已同步/待同步/失败+原因） |
| frps.toml | 「下发到服务器」按钮替代「复制到 VPS」；一键含重启与验证 |
| 证书页 | 「部署到服务器」：选目标服务端 → 推送 PEM → 回显 agent 挂载结果；续期任务成功后自动重推 |
| 管理弹窗 | 服务端「管理」按钮：status 报告（frps 状态/端口占用者/路由清单/证书有效期）+ 一键操作（下发配置/全量对账） |
| API 客户端 | `internal/frp/agent_client.go`：Go HTTP 客户端封装上述 7 个接口（超时/重试/错误透传） |

## 6. agent 部署与生命周期管理（SSH，全程在 Havline 界面完成）

### 6.1 首次安装（用户不登 VPS）

1. Havline 服务端表单「远程管理」分组填 SSH 信息（地址/端口/用户/密码或私钥，可选保存）+ 点「安装 agent」；
2. Havline 经单次 SSH 会话自动完成：
   - 探测环境：`uname -m`（选二进制）、nginx 是否安装、`systemd` 可用性、端口占用快照；
   - SFTP 上传 `havline-agent` 二进制与 `install.sh`（从 Havline 自带资源解出对应架构）；
   - 远程执行 `install.sh`：落位 `/usr/local/bin/havline-agent`、写 `/etc/havline-agent/agent.toml`（**生成随机 Token 并回传给 Havline**，无需用户抄写）、注册 `systemd` 单元并启动；
   - 安装后立刻走 API `ping` 验证 + 防火墙提示（如 7700 未放行给出命令）；
3. UI 展示每步输出，失败可重试（幂等，重复安装=覆盖升级）。

### 6.2 升级 / 卸载 / 自救

| 动作 | 方式 | 说明 |
| --- | --- | --- |
| 升级 agent | SSH：上传新二进制 + `systemctl restart havline-agent` | 路由/证书/Token 均在磁盘，不丢 |
| 卸载 agent | SSH：`install.sh --uninstall`（停服务、可选保留数据） | nginx vhost 文件一并清理（可选项） |
| agent 失联 | API 不可达时，Havline 管理弹窗提示「走 SSH 通道诊断」：进程状态/端口/日志尾部 | SSH 是 API 故障时的备用通道 |

### 6.3 日常管理（API，面板在 Havline）

安装完成后的一切操作不再用 SSH：规则联动同步、frps.toml 下发、证书部署、status 查询、全量对账——全部是 Havline 界面按钮/自动触发，走 agent API。VPS 上没有任何管理面板，用户日常甚至不需要记住 VPS 密码。

## 6.5 Havline 侧新增「SSH 安装器」（与 agent 客户端并列）

| 模块 | 内容 |
| --- | --- |
| `internal/frp/ssh_installer.go`（新） | SSH 连接（x/crypto/ssh 已在 go.mod）、SFTP 上传二进制/install.sh、远程幂等安装流程、Token 回传 |
| agent 二进制内嵌 | 构建期把 `havline-agent` 各架构二进制 embed 进 Havline（构建矩阵：linux-amd64/arm64），安装时按 `uname -m` 选择，无需用户单独下载 |
| 数据模型 | 复用 023 迁移：`ssh_host/port/user/auth/secret_enc` 可选保存（仅启用「安装/升级/卸载」功能时需要），agent API Token 由安装器自动生成并写入 Havline |
| UI | 服务端表单「远程管理」：SSH 信息（可选）+「安装 agent / 升级 / 卸载 / 走 SSH 诊断」按钮，安装过程实时输出展示 |

> SSH 信息**可选保存**：仅安装/升级时需要；日常管理走 API Token。安全上 SSH 凭据同 token 机制（secret.Box 加密、不回显），远程命令固定白名单模板（上传/安装/卸载三种），无注入面。

## 7. 实现拆解与工作量

| 模块 | 位置 | 内容 | 量级 |
| --- | --- | --- | --- |
| agent 主体 | `cmd/havline-agent/` + `internal/agent/`（新） | HTTP 服务、鉴权、路由/frps/证书三个执行器、nginx 模板与 reload、状态聚合 | 大（约 1200 行） |
| Havline SSH 安装器 | `internal/frp/ssh_installer.go`（新） | SSH/SFTP 上传、远程幂等安装/升级/卸载、agent 二进制 embed、Token 自动回传 | 中 |
| Havline agent 客户端 | `internal/frp/agent_client.go`（新） | 7 个接口封装 | 中 |
| 数据模型/迁移 | `023_frp_server_agent.sql` | SSH + agent 字段 + 规则同步状态 | 小 |
| 规则联动 | `internal/frp/service.go` | 保存/删除/启停后同步调用 + 状态落库 | 中 |
| API/UI | `frpmulti_handler.go`、`router.go`、`FrpView.vue`、`client.ts` | 5 个端点 + 表单分组 + 管理弹窗 + 同步状态列 | 中 |
| 测试 | `agent_test.go` 等 | nginx 模板渲染/回滚、接口幂等、鉴权 | 中 |

总计约 2500–3000 行（含测试）。`mgmt_enabled=0` 时 Havline 行为与 v1.1.0 完全一致；agent 未部署的服务端不受任何影响。

## 8. 里程碑

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| M1 | agent 主体（ping/status/routes）+ Havline SSH 安装器（一键安装） | Havline 填 SSH 信息点「安装」后自动装好 agent 并回连成功，status 能报端口占用者 |
| M2 | Havline 规则联动 + 路由自动注册 + frps 配置下发 | 保存 http 规则后 `http://域名` 无端口直达；frps.toml 一键下发生效 |
| M3 | 证书推送 + 续期联动 + 443 路由 | `https://域名` 可用，续期自动重推 |
| M4 | 管理弹窗 UI + 全量对账 + 升级/卸载/SSH 诊断 + 手册更新 | 手册第 6 节新增方案 C（agent 模式：SSH 装、API 管） |

## 9. 明确不做（本期）

- 不做 agent 侧自主 ACME（DNS-01 在 NAS 侧签发已覆盖；HTTP-01 作为后续可选）；
- 不做 frps 二进制远程安装/升级（status 报告版本，安装沿用现有手册）；
- 不做 agent 集群/多机编排（一个服务端一个 agent，模型对齐现有 frp_servers）；
- 不动 frp 二进制与协议（agent 只操作 frps 的配置文件、进程与 vhost 端口）。

## 10. 方案形态对照

| | v2 最终（agent + SSH 安装器） | v1（纯 SSH 推送） |
| --- | --- | --- |
| VPS 侧 | 常驻 agent，但**安装/升级由 Havline SSH 一键完成** | 零安装 |
| 日常管理 | API（面板全在 Havline，日常无需 SSH） | 每次操作都走 SSH 命令 |
| 能力上限 | 高：本地编排 nginx/frps/证书，可自愈可扩展 | 受限于 SSH 命令白名单 |
| 与已有站点共存 | 只写 `havline-*.conf`，共存 | 可实现但每步都要远程拼配置 |
| 故障定位 | status API 一次拿全；agent 失联时 SSH 急救 | 每次探测拼 SSH 命令 |
| 适用 | VPS 长期做穿透入口、多域名/HTTPS 全自动 | 不想在 VPS 装常驻组件 |

结论：**SSH 与 API 不是二选一**——SSH 只承担 agent 生命周期（装/升/卸/急救），API 承担日常管理；管理面板 100% 在 Havline 本地，VPS 上没有任何管理界面。
