# Cloudflare Tunnel（cloudflared）与 Fonu 的融合方案

> 日期：2026-09-25，v2 修订 2026-09-28　对象：Fonu 1.5.4（VERSION 1.5.4）
> 方法：先查 Cloudflare 官方文档与同类开源项目，再逐个核对本仓库现有可复用面（下文带 `文件:行` 的都是读过代码得出的）；v2 逐文件核对了 `F:\WWW\cf\Tunnel-Cloudflared-linux`（Tunnel-Cloudflared-Manager 1.0.1）源码。
> 结论速览：**值得做，但建议以「第二条出口」的形态并存，先做 locally-managed + 进程托管，不接 Cloudflare API**。v2 修订了部署位置、模式生效侧、metrics / 健康与二进制校验四处关键细节，详见 §五、§六、§十二；实施以 §十二 为准。

---

## 一、为什么值得做（对 Fonu 的战略意义）

Fonu 现在的穿透链路**必须有公网服务端**：本机 `frpc` → VPS 上的 `frps` → 域名。用户得有一台带公网 IP 的云主机，还得自己放行端口、签证书、配 DNS。

Cloudflare Tunnel 把「服务端」换成 Cloudflare 的边缘网络：本机 `cloudflared` **主动向外**建立到边缘的长连接（出站 only），公网侧不需要任何监听端口，也不需要公网 IP——顺带白拿 CF 的 WAF / DDoS 防护与自动 HTTPS。

对 Fonu 的用户画像（家庭/个人 NAS）来说，这正好补上一类人：**没有实体公网 IP、或不想为穿透长期租 VPS**的人。

但要说清楚它**替代不了什么**（这决定了它是「并存」而不是「替换」）：

| 场景 | frp | Cloudflare Tunnel |
| --- | --- | --- |
| 内网 HTTP(S) 网站/面板（域名访问） | ✅ | ✅ **更省事**（无需 VPS、无需放行端口） |
| 任意 TCP/UDP（游戏服、非 HTTP 协议） | ✅ | ⚠️ 需要访问端也装 `cloudflared`（`cloudflared access tcp`），不是通用 TCP 出口 |
| 客户端不需要额外软件的裸 TCP 端口 | ✅ | ❌ |
| 自持服务端 / 完全自主可控 | ✅ | ❌（流量必经 CF 边缘） |
| 大文件/视频流量 | ✅（自己带宽说话） | ⚠️ CF 条款限制非 Enterprise 的 CDN 视频/大文件用途，见 §七 |

所以定位是：**HTTP 类公网访问走 Cloudflare 更划算，TCP 类与自主性要求高的仍走 frp**。

---

## 二、同类项目（检索结果）

| 项目 | 做什么 | 对 Fonu 的借鉴 |
| --- | --- | --- |
| [WisdomSky/Cloudflared-web](https://github.com/WisdomSky/Cloudflared-web)（≈451★，Vue） | Docker 镜像里打包 `cloudflared` + 简单 Web UI，用来 start/stop **remotely-managed** 隧道 | **最接近的同位产品**：证明「面板管 cloudflared 进程」这个形态成立；但它的能力止步于开关，规则仍在 CF 面板里——这正是 Fonu 可以做得更好的地方（规则同源生成） |
| [adyanth/cloudflare-operator](https://github.com/adyanth/cloudflare-operator)（≈691★） | Kubernetes Operator，用 **CF API** 创建/管理 Tunnel 与 DNS | 演示了「API 驱动」的完整闭环；但需要 API Token + Account/Zone ID，是第二个凭据面（Fonu 第一轮不建议走，见 §六） |
| [qingchencloud/cftunnel](https://github.com/qingchencloud/cftunnel)（≈338★） | 轻量隧道控制 | 佐证需求存在 |
| [brenner-tobias/addon-cloudflared](https://github.com/brenner-tobias/addon-cloudflared) | Home Assistant 插件，locally-managed 与 remotely-managed 两种模式都支持 | HA 社区的做法可参考：**两种模式并存**，文档把差异讲透 |
| [cloudflare/cloudflared](https://github.com/cloudflare/cloudflared) | 官方客户端（Apache-2.0） | 我们的被托管对象；GitHub Release 提供多平台二进制 |
| 对照项：[Pangolin](https://github.com/fosrl/pangolin) / Tailscale Funnel / ngrok | 同类「免公网 IP」方案 | Tailscale Funnel 只开 HTTPS 且端口数量有限、ngrok 免费档收紧；CF Tunnel 免费档最宽松，这也是它值得优先支持的原因 |

`docs/feature-benchmark-2026-09-24.md` 里已经把 Pangolin 列为架构最近的同类项目；Cloudflare Tunnel 的差别在于：**它把「服务端」外包给了 Cloudflare，Fonu 只需管好本机客户端**。

---

## 三、cloudflared 的两种托管模型（选型的关键）

| | locally-managed（本地管理） | remotely-managed（远程管理） |
| --- | --- | --- |
| 凭据 | `cert.pem`（`cloudflared tunnel login` 浏览器授权）+ `<UUID>.json` credentials | 一个 **token**（`eyJ...`，CF 面板生成） |
| 规则来源 | 本机 `config.yml` 的 `ingress:` | **CF 面板 / API**（本地 `config.yml` 会被**忽略**） |
| 建隧道 | `cloudflared tunnel create <name>` | CF 面板，或 `POST /accounts/{id}/cfd_tunnel` |
| DNS | `cloudflared tunnel route dns <tunnel> <hostname>` | CF 面板自动建 CNAME |
| 与 Fonu 的价值取向 | ✅ **契合**：反代规则由 Fonu 同源生成（和现在生成 `frps.toml` / nginx vhost 一个路子） | ⚠️ 冲突：规则跑到 CF 面板去了，Fonu 沦为「进程开关」 |

官方文档确认了这个分界（[配置参考](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/do-more-with-tunnels/local-management/configuration-file)），社区也踩过坑：给 remotely-managed 隧道传 `--config` 时配置会被云端覆盖，日志里出现 `No ingress rules were defined ... cloudflared will return 503`（[cloudflared#1029](https://github.com/cloudflare/cloudflared/issues/1029)）。

**结论：第一轮只支持 locally-managed。** 若用户只想粘贴 token，提供「仅托管进程」的降级模式并**在界面上明确写清「规则由 Cloudflare 面板管理」**，不要让用户以为 Fonu 生成的规则生效了。

> 补充（参考 §九 后修正）：这个二分法不够细。参考实现证明**token 可以解码**（base64 的 JSON，含 `AccountTag` / `TunnelID` / `TunnelSecret`），用它自己生成 `creds.json` 就能按 locally-managed 跑本地 `config.yml`；再配合「云端 ↔ 本地 ingress 互相同步」，两种模式实际上是可切换的。所以 Fonu 的正确做法不是「二选一」，而是**显式区分并同步**：当前生效的是本地 `config.yml` 还是云端 ingress，以及「推送/拉取」两个动作。**v2 已把模式拆成三种并明确生效侧，见 §12.3。**

### 健康探测（这点比 frp 更省事）

`cloudflared tunnel --metrics 127.0.0.1:<port> run --config ...` 会暴露 HTTP 端点，其中 **`/ready` 在有到边缘的活跃连接时返回 200**（CF 官方在 k8s livenessProbe 里就是这么用的）。Fonu 现在为了判断「隧道通不通」要绕 frps dashboard，还专门加过 `tunnel_known` 来区分「判不出来」和「不通」；CF Tunnel 则有明确的就绪端点，可以直接喂给现有健康链路。

---

## 四、本仓库可复用的面（已核对代码）

| 能力 | Fonu 现有实现 | 复用方式 |
| --- | --- | --- |
| 二进制下载 / 启用 | `Service.ListReleases`（`internal/frp/service.go:112`）、`ListBinaries`（`:120`）、`ActivateBinary`（`:258`）；`internal/frp/runtime.go` 的 `fetchOfficialReleases` / `listBinaries` / `readActiveBinary` / `writeActiveBinary` / `versionFromBinaryPath` | cloudflared 同样发 GitHub Release，**同一套框架泛化成「可指定仓库」即可**（现在 URL 拼的是 frp 的 release 页） |
| 客户端进程托管 | `internal/frp/manager.go`：`os.OpenFile(logPath, O_CREATE\|O_APPEND\|O_WRONLY, 0600)` + `cmd.Stdout/Stderr` 重定向 + 状态/退出原因 + `tailFile` 读日志尾 | 与 `cloudflared tunnel run` 完全同构，可直接复用（日志追加、不截断的语义也一致） |
| 凭据加密与回显 | `secret.Box` + `maskedToken = "********"`（`internal/frp/service.go:23`）+ 「预览脱敏 / 下发原文」两分支约定（v1.2.12 刚修） | token / credentials JSON 存加密字段；**预览一律掩码** |
| 配置生成 | `internal/frp/frps_template.go`、`internal/proxy` 的 nginx vhost 模板 | `config.yml` 的 `ingress:` 用同样思路生成，注释与安全提示的风格照搬 |
| 健康与可用率 | `internal/monitor`（`Source` 接口 + `HealthRecorder`）、`migrations/026` 的健康事件表、`metrics_samples` | 给 `Source` 加一个 provider（frp / cloudflared），`/ready` 作为判活依据 |
| 日志界面 / 公开状态页 | 日志页各页签、`/status` 公开状态页（默认关闭 + 访问码 + 限流） | 新隧道类型直接进现有页面，不用另造一套 |
| 规则模型 | `frp.Proxy{ServerID, Type, LocalIP, LocalPort, CustomDomains, Options}`（`internal/frp/model.go:186`） | 不再需要 `ServerID` 时用一个 provider 字段代替（见方案 B） |

---

## 五、三个方案

### 方案 A：独立模块「Cloudflare 隧道」（推荐第一轮）

新增一个与「公网服务端（FRP）」并列的模块，**不碰现有 frp 规则模型**：

- 管理一份 cloudflared 二进制（复用 §四 的下载/启用框架）。
- 管理 N 个隧道实例，每个实例 = 一份 `config.yml` + credentials + 日志文件 + 子进程。
- `ingress` 规则**由 Fonu 生成**：从本机已有的反代规则里挑「要暴露的域名」，映射到 `http://127.0.0.1:<本地端口>`，模板与 nginx vhost 同源风格；末尾固定给 catch-all `service: http_status:404`（CF 要求最后一条兜底）。
- 健康：`--metrics` + `/ready`，写入现有健康与可用率链路；通知复用（隧道断开/恢复）。
- 前端：与「公网服务端」同构的列表 + 详情（状态 / 日志 / 配置预览 / 下发）。

**优点**：改动封闭、不动现有模型、可独立上线、风险低。
**代价**：规则要在两处维护（frp 规则 vs CF 隧道规则）——这是方案 B 要解决的。

### 方案 B：把「隧道出口」抽象成 provider（v2 方向）

在规则上加 `provider: frp | cloudflare`（`frp.Proxy` 增字段，配套迁移），规则本身只说「哪个域名 → 内网哪个端口」，出口由 provider 决定：

- 好处：一条规则可在 FRP / CF 之间切换，健康、可用率、状态页、漂移检测天然统一。
- 成本：动模型 + 迁移 + 前端多处（规则表单、健康四段、公网反代页），且 `ServerID` 的语义要重新定义（CF 侧没有服务端）。
- 建议：**等方案 A 稳定、用户真的需要在两种出口间迁移规则时再做**。

### 方案 C：只做引导（不接管进程）

只生成 `config.yml` 与 DNS 指引，让用户自己跑 `cloudflared`。

**不建议**：不接管进程就没有状态、日志、健康、可用率，等于放弃 Fonu 的全部价值。

---

## 六、推荐落地路径（分阶段）

> 本节阶段划分是 v1 口径；v2 修订见 §12.10，实施以 §12.10 为准。

### 阶段 1：只读与引导（最低风险，先验证需求）

1. 新增「Cloudflare 隧道」入口：粘贴 **token**（remotely-managed）或上传 **credentials JSON + tunnel UUID**（locally-managed），凭据加密存储。
2. 生成 `config.yml` 预览（**脱敏**，沿用 v1.2.12 的预览/下发两分支约定）并把文件写到 `<DataDir>/cloudflared/<name>/config.yml`。
3. 界面给出 CF 侧操作指引（建隧道、CNAME、`cloudflared tunnel route dns`），**不接管进程**。
4. 明确提示模式差异：locally-managed 才由 Fonu 生成规则。

### 阶段 2：托管进程与健康（核心价值）

5. 把 §四 的 release/binaries 框架泛化，支持下载 `cloudflared` 并启用指定版本；进程托管复用 `Manager`（日志、状态、退出原因、`tailFile`）。
6. `--metrics 127.0.0.1:<port>` + `/ready` 接入 `internal/monitor`：新增 provider 判活，接入现有通知与 `metrics_samples` 可用率。
7. 子命令支持：启动 / 停止 / 重启 / 重载配置 / 查看日志（与「公网服务端」的操作面板一致）。

### 阶段 3：规则同源与界面完善

8. 从本机反代规则推导 `ingress`（勾选要暴露的域名），生成后一键下发并重启；末尾自动补 catch-all。
9. 前端补齐：状态卡、日志页签、配置预览（脱敏）、与 VPS 实际文件的漂移检测（沿用「配置漂移」的交互）。
10. Access（Zero Trust）相关只做**诊断提示**（例如「此域名未启用 Access 保护」），不代配置策略。

### 阶段 4（可选，需单独评估）

11. 接 Cloudflare API 自动建隧道与 DNS：需要 API Token（权限 `Account: Cloudflare Tunnel: Edit` + `Zone: DNS: Edit`）+ Account ID + Zone ID。**这是第二个凭据面**，收益是省掉几步手工操作，代价是多一份高权限凭据要保管——建议等前面稳定后再评估，并且只在用户显式开启时启用。

---

## 七、风险与边界（务必写进功能说明）

**条款与合规**
- Tunnel 本身免费、不限条数/带宽，但 Cloudflare 的 CDN 条款限制「用 CDN 提供视频与其他大文件」（非 Enterprise 需购买 Stream/Images 等付费服务）。社区里这条措辞有过调整，但仍应视为风险：**用 CF Tunnel 做影音/大文件分发不在我们的推荐用法内**，界面上要提示。
- TryCloudflare（快速隧道：`cloudflared tunnel --url http://localhost:PORT`）官方明确「不保证 SLA 与可用性，仅供测试与开发」，域名随机、每次重启变化——**不作为正式功能**，最多做成「临时演示」按钮。

**网络可用性**
- 中国大陆访问 CF 边缘的延迟与稳定性不稳定，这对 Fonu 的用户群是**现实问题**：建议在文档与界面上对比说明「CF 适合海外访问优先的场景，国内链路以 frp + 自持 VPS 更可控」。
- 出站 only 意味着 VPS 上不需要任何放行端口，但也就**读不到**「CF 侧有哪些域名指向了这条隧道」——相关校验只能靠 `cloudflared` 自身命令，不能靠 agent。

**实现边界**
- **不做 remotely-managed 的规则生成**（会被云端覆盖，见 §三）。
- **不代配 Access 策略**（只提示状态）。
- **不改现有 frp 链路**：CF Tunnel 是并存出口；`internal/frp` 的规则、健康、状态页语义保持不变。
- **不做通用 TCP 出口**：需要访问端装 `cloudflared` 的场景超出「NAS 面板」的定位，只做说明。
- 凭据全部走 `secret.Box` 加密 + 掩码回显；日志与 API 响应绝不出现明文 token（沿用 `maskedToken` 与「预览脱敏」约定）。

---

## 九、参考实现分析：`cf-tunnel-manager`（用户自有项目，Gitee `li-hua-88/cf-tunnel-manager`）

> 本节是 2026-09-25 的 v1 分析。2026-09-28 已对照 1.0.1 Linux 源码逐文件复核，补充与修正见 §十二。

> 项目：**Tunnel-Cloudflared-Manager** v1.0.1，Rust + Actix-web + Tokio + Serde，前端原生 JS + Tailwind（`static/`），JSON 文件存储，MIT；Linux systemd（`tcm`/`cftm` 运维脚本）+ Docker 两套交付。
> 本节结论来自逐个读到源码的模块：`src/services/{tunnel,binary,cloudflare_api}.rs`、`src/models/tunnel.rs`、`src/utils/{config_helper,paths}.rs`、`Cargo.toml`。

### 9.1 它的定位与能力

| 维度 | 它做了什么 |
| --- | --- |
| 隧道类型 | **命名隧道**（token 或 config-file 启动）+ **快速隧道**（`--url`） |
| 进程管理 | `TunnelProcessManager`：`HashMap<名称, Child>` + 独立的 `login_process` + 快速隧道 URL 缓存 |
| 日志 | `tokio::process` 逐行读 stdout/stderr → **WebSocket 广播**（`tunnel-log` 事件），前端实时滚动 |
| 指标 | 每 2 秒抓 `cloudflared` 的 Prometheus 端点 → 同样用 WebSocket 广播（`tunnel-metrics`） |
| 二进制 | `src/services/binary.rs` 自管下载/替换/检测，不要求用户预装 |
| 账号模式 | `--origincert` + `tunnel create` + `tunnel route dns`，把生成的 `creds.json` 搬进自己的 `data/tunnels/<名>/` |
| 云端同步 | `src/services/cloudflare_api.rs` 能按 tunnel **拉取/推送** ingress 配置（`fetch_tunnel_config` / `push_tunnel_config`） |
| 登录与安全 | JWT + bcrypt 管理员账号，默认 `admin`/`admin666`（首次登录强制改密） |
| 网络调优 | `protocol`（auto/http2/quic，Windows 或检测到代理时回落 http2）、`ha-connections`、`edge-ip-version`、`originRequest`（`caPool`/`noTLSVerify`/`httpHostHeader`）、代理环境变量开关 |

### 9.2 值得直接借鉴的六个做法

**① token 可以解码成凭据，于是 token 模式也能本地管规则**
`initialize_token_tunnel` 把 CF 隧道 token 解成 `{AccountTag, TunnelID, TunnelSecret}` 并写出 `creds.json` + `config.yml`——即「用户只给一个 token，但规则仍由面板本地生成」。这直接提升了 Fonu 方案的上限（§三 的补充已按此修正）。

**② metrics 端口不去猜，从日志里读出来**
命名隧道启动时**故意不传 `--metrics`**，让 cloudflared 用默认端口段（20241-20245），然后从 stderr 解析 `INF starting metrics server on 127.0.0.1:65432/metrics` 拿到真实端口——天然避开端口冲突（Fonu 现在给 frps dashboard 固定端口，多实例时会撞）。

**③ `localhost` 必须归一化成 `127.0.0.1`**
`add_tunnel_endpoint` 里把 `localhost` 换成 `127.0.0.1`，注释写明「避免 Cloudflare 将其转换为本地网络 IP」。这是 CF 侧的真实坑，Fonu 生成 `ingress` 时要照做。

**④ 始终保留最后一条 catch-all**
新增端点插在「数组倒数第一位之前」（`insert_pos = len - 1`），末条永远是 `service: http_status:404`。Fonu 改 `ingress` 时必须沿用这个不变式，否则配置校验不过。

**⑤ 带宽指标：官方 metrics 没有 HTTP/2 字节数，它做了系统级兜底**
- QUIC 下直接读 `quic_client_receive_bytes` / `quic_client_sent_bytes` / `quic_client_smoothed_rtt`。
- HTTP/2 下官方无字节数，改用**按进程采 TCP 连接字节**：Linux 走 netlink `SOCK_DIAG_BY_FAMILY` + `/proc/<pid>/fd` 取 socket inode；Windows 走 `GetExtendedTcpTable` + `GetPerTcpConnectionEStats`（需要管理员，失败时给 `windows_estats_requires_admin` 降级原因）。
- 并且**只统计公网远端 IP**（过滤私有/回环/链路本地/`100.64/10` CGNAT），延迟兜底用**主动 HEAD 探测** ingress 里的公网 hostname（带 `?__cf_probe=<nonce>` + no-cache）。
> 对 Fonu 的意义：这一块可以**先不做**（我们已有规则级流量与可用率），但「降级要给出明确原因而不是空白」这个原则要学。

**⑥ Windows 下起子进程要隐藏控制台窗口**
`creation_flags(CREATE_NO_WINDOW)`（`0x08000000`）。Fonu 本机在 Windows 上跑，托管 `cloudflared.exe` 时必须带上，否则会闪黑窗。

### 9.3 与 Fonu 的差异（决定「借鉴什么、不借鉴什么」）

| 维度 | cf-tunnel-manager | Fonu 该怎么做 |
| --- | --- | --- |
| 职责 | **专做** Cloudflare Tunnel 的一个独立面板 | 把 CF 作为**第二条出口**并入现有规则/健康/通知/状态页体系 |
| 存储 | JSON 文件（`data/*.json`）+ `data/tunnels/<名>/` | 继续用 SQLite + 迁移；凭据进 `secret.Box` 加密字段 |
| 规则来源 | 面板自己的 ingress 列表（+ 云端同步） | 从**本机已有反代规则**推导（同源生成），避免第二份手工维护 |
| 健康 | metrics 端点 scraping + 主动探测（仅前端展示） | 接入 `internal/monitor` 的 `Source`/`HealthRecorder`，落库可用率 + 复用告警与通知 |
| 实时日志 | WebSocket 广播（专为「看着它滚」设计） | 沿用现有「读日志文件 + 日志页」模型（与 frpc 一致，无需引入 WS） |
| 多实例 | 多隧道并行（每隧道一个子进程） | 同模；但用 SQLite 建 `tunnels` 表而不是内存 HashMap |
| 认证 | 自带 JWT + 账号体系 | 复用 Fonu 现有会话 + API Token |

### 9.4 对方案本体的修订建议

1. **方案 A 的第一阶段就加「凭据导入」两条路**：`token`（解码出 creds.json）与 `cert.pem + credentials JSON`（账号模式），二者最终都落到「本地 `config.yml` 由 Fonu 生成」。
2. **阶段 2 把「metrics 端口从日志发现」写成实现约定**，不要固定端口。
3. **阶段 3 增加「云端 ↔ 本地同步」两个动作**（拉取 / 推送），并在界面上标明当前生效侧——这是参考项目比原方案更完整的地方。
4. **`ingress` 生成器锁定三条不变式**：末条 catch-all、`localhost` → `127.0.0.1`、插入位置在 catch-all 之前；并补一个「生成后自检」的纯函数测试（参照本仓 `maskFRPSSecrets` 的测试写法）。

---

## 十、IP 优选：能不能做、能做到哪一步

### 结论一句话

**隧道侧（本机 `cloudflared` → CF 边缘）可以优选，访问侧（浏览器 → CF 边缘）不行。** 而国内用户最在意的那一段延迟往往在访问侧——所以不要把它宣传成「提速神器」，它的收益是**可控但有限**的。

### 隧道侧：三条可用手段（已核对官方文档与变更日志）

| 手段 | 具体做法 | 适用与代价 |
| --- | --- | --- |
| **选协议** | `--protocol quic` / `http2` | 国内 QUIC（UDP 7844）常被 QoS 掉，`http2`（TCP 7844）反而稳；这是**成本最低、收益最直接**的一项 |
| **选地址族** | `--edge-ip-version 4` / `6` / `auto`，或环境变量 `TUNNEL_EDGE_IP_VERSION` | ⚠️ **2026.4.0 起默认值由 `4` 改成 `auto`**（官方 CHANGES 里的 Breaking Change）——`auto` 会跟着系统解析器走，国内环境若 IPv6 路由差就可能反而变慢。**建议 Fonu 显式下发 `--edge-ip-version 4`**（除非用户自己有 IPv6 好线路） |
| **优选边缘 IP** | 对 `region1.v2.argotunnel.com` / `region2.v2.argotunnel.com` 做延迟测速，选中最快的官方边缘 IP，写 hosts 覆盖解析 | 官方文档给出固定端点与端口：这类域名指向 CF 的 IPv4 段（`198.41.192.7`~`198.41.192.127`、`162.159.234.1`~`162.159.234.10`）与 **端口 7844（TCP/UDP）**。这是社区「Tunnel 优选 IP」的通行做法 |

**优选时两条不能错的约束**：

1. **两个 region 都要覆盖**。官方预检文档要求健康部署有**四个活跃 replica、同时连上 `region1` 与 `region2`**（或对应地区域名）——只改其中一个域名，另一半连接会跑到未优选（乃至不通）的路径上，净效果可能是变差。
2. **IP 列表不能硬编码**。CF 改过端点与 hostname（例：`proxy-dns` 整条功能已在 2026.2.0 被移除），优选表要么给默认值 + 可配置，要么每次从官方 DNS 拉候选集再测。

关于实例数：`--ha-connections` 决定每个 region 开几条连接，也是「多路择优」的天然手段（参考实现 §9.1 已把它做成设置项）。

### 访问侧：Tunnel 模式下做不到

用 CF Tunnel 时，公域域名必须 CNAME 到 `<tunnel-uuid>.cfargotunnel.com` 并且**开启代理（橙云）**——解析结果由 CF 决定，**用户无法自选边缘 IP**。社区那套「测速 + 自选 IP」之所以常见，是因为他们用的是「CNAME 到优选 IP + 关闭橙云 + 自己回源」的方案；那**恰好是 Tunnel 不能走的路**（关掉代理就没有 Tunnel 了）。

也就是说：换域名解析 / 让访客改 hosts 都不现实，**访问侧这一段在 Tunnel 架构下就是交给 CF 的**。

### 对 Fonu 方案的落点

| 阶段 | 做什么 | 依据 |
| --- | --- | --- |
| 阶段 2（建议直接做） | 「网络调优」三项做成设置项：`protocol`（默认 http2）、`edge-ip-version`（**默认 4**）、`ha-connections`；以命令行参数下发 | 参考实现已验证可行，且改一项就能治国内 QUIC 被打压的场景 |
| 阶段 3（可选） | 「边缘优选」工具：并发探测官方 IPv4 段的 **TCP 7844 握手延迟** → 生成「两个 region 各 1~2 个优选 IP」的 hosts 建议；**页面展示而不是代写 hosts**（Windows 改 hosts 需管理员，Fonu 不应越权），并提供「复制这段内容」+ 生效检测 | 沿用本仓「不越权、降级要说明原因」的口径 |
| 阶段 3（可选） | 优选前后对比：用**已有的** `metrics_samples` 延迟/可用率序列展示效果，而不是另造一套测速图 | 复用 §四 的健康与指标链路 |

> metrics 端口发现、健康判定与网络调优默认值见 §12.5 / §12.7；本节只讨论「本机 → CF 边缘」的优选。

### 必须写进功能说明的三句实话

1. 优选只影响「本机 → CF 边缘」这一段；访客到自己那侧 CF 边缘的那段，Fonu 与用户都控制不了。
2. 所以它**不是**「翻墙提速」类工具：国内访问 CF 的稳定性受链路与运营商影响，优选只能把「本机出口到边缘」这一段挑好。
3. 官方端点会变（见上），因此把所有 IP/域名做成**可配置 + 有默认值 + 可一键恢复默认**。

---

## 十一、给用户的一句话结论（可直接对外）

> Fonu 现在支持两类公网出口：**frp**（自持 VPS，任意 TCP/UDP，完全自主）与 **Cloudflare 隧道**（无需公网 IP 与 VPS，HTTP 类最省事，自带 CF 防护）。
> 要裸 TCP 端口、要完全自主，用 frp；要让内网 Web 服务零端口暴露地上公网，用 Cloudflare 隧道。
> Cloudflare 隧道还提供网络调优（协议 / 地址族 / 连接数）与**边缘 IP 优选**（仅作用于本机到 CF 边缘这一段，访客侧无法自选）。

---

## 十二、方案优化 v2（结合 Tunnel-Cloudflared-Manager 1.0.1 与 Fonu 1.5.4）

> 本节是 2026-09-28 的优化版落地口径。§一～§十一 保留为背景与论证；出现冲突时以本节为准。

> 实施状态（2026-09-28，v1.6.0）：阶段 1–2 已实施，包含 token-local / token-remote 两种模式、token 解码与 creds.json / config.yml 生成、ingress 生成器、SQLite 存储、cloudflared 二进制下载、进程托管、日志、metrics 端口发现与健康抓取、`/api/cloudflare/*` 接口与前端页面。阶段 3 的账号模式（cert.pem）与云端 fetch / push、阶段 4 的边缘优选与快速隧道尚未实施。

### 12.1 源码核对新增的关键事实

逐文件核对 `F:\WWW\cf\Tunnel-Cloudflared-linux` 后，以下事实会直接改变实施细节：

| 事实 | 源码位置 | 对方案的影响 |
| --- | --- | --- |
| token 解码后的 JSON 字段是 `a` / `t` / `s`，兼容 `AccountTag` / `TunnelID` / `TunnelSecret`；使用标准 base64 并自动补 `=` | `src/services/cloudflare_api.rs` 的 `decode_tunnel_token` / `base64_decode` | token 模式也能本地生成 `creds.json`，从而按 locally-managed 跑本地规则 |
| `cert.pem` 内含 `-----BEGIN ARGO TUNNEL TOKEN-----` 块，解出 `accountID` + `apiToken` / `serviceKey` | 同文件 `parse_cert_pem` | 只有账号模式持有云端 API 凭据，才能 push / fetch 云端 ingress |
| `start_tunnel` 在 token 非空时优先用 `--token` 启动，即使已经生成了本地 `config.yml` | `src/services/tunnel.rs` 的 `start_tunnel` | **本地规则会被云端覆盖**；Fonu 必须显式选择「本地生效」还是「云端生效」 |
| 命名隧道**不传** `--metrics`，从 stderr 解析 `starting metrics server on 127.0.0.1:<port>/metrics`；快速隧道传 `--metrics 127.0.0.1:0` | `start_tunnel` / `start_quick_tunnel` | metrics 端口必须从日志发现，不能固定，否则多实例会撞端口 |
| 健康用 `cloudflared_tunnel_ha_connections > 0` 判定已连接；QUIC 有字节指标，HTTP/2 没有 | `start_metrics_scraper` | `/ready` 只适合做就绪判断，连接数与带宽要走 metrics；HTTP/2 带宽要明确降级 |
| 二进制下载只校验「文件 >= 10MB」，没有 SHA-256；`get_available_mirrors` 返回的镜像 ID 与实际支持的镜像不一致 | `src/services/binary.rs` | Fonu 必须做哈希 / 版本校验，并统一镜像表 |
| ingress 三条不变式：末条固定 `http_status:404`、`localhost` 归一化为 `127.0.0.1`、新规则插在 catch-all 之前 | `add_tunnel_endpoint` / `add_tunnel_endpoint_local_only` | 生成器必须锁定这三条并加纯函数测试 |
| Windows 子进程设置 `CREATE_NO_WINDOW` | `start_tunnel` | Fonu 在 Windows 托管 `cloudflared.exe` 时必须带，否则闪黑窗 |
| 快速隧道从 stderr 解析 `*.trycloudflare.com` URL 并缓存 | `start_quick_tunnel` | 只能做「临时演示」；URL 每次重启都会变 |
| 存储是 JSON 文件 + 内存 `HashMap<String, Child>` | `src/storage/config.rs`、`TunnelProcessManager` | Fonu 用 SQLite + 迁移，多实例与并发状态更稳 |

### 12.2 部署位置：原方案最大的空白

原方案默认「本机跑 cloudflared」，但没有回答一个关键问题：**Fonu 是容器化部署，容器里的 `127.0.0.1` 不是宿主机。**

Fonu 的运行时（`Dockerfile`）在同一个容器里装了主程序与 nginx，`docker-compose.yml` 只映射 `6893` / `80` / `443`。因此 v2 的默认拓扑是：

> **cloudflared 跑在 Fonu 容器内，与 nginx 同命名空间；CF ingress 统一指向 `http://127.0.0.1:<nginx_http_port>`（默认 80），由 nginx 再转发到上游。**

这样做有四个直接收益：

1. 绕开容器网络命名空间的坑：`127.0.0.1` 在容器内确定指向 Fonu nginx，而不是宿主机或上游。
2. 复用现有 nginx 的安全、限流、geo、TLS、错误页与 Basic Auth，不在 CF 侧重复一套策略。
3. 规则同源：CF 只负责「域名 → Fonu nginx」，nginx 负责「域名 → 上游」；本机反代规则仍是唯一事实来源。
4. 只有 HTTP / HTTPS 规则能进 CF；TCP / UDP 规则明确排除，避免用户误解。

三种拓扑的取舍：

| 拓扑 | cloudflared 位置 | ingress 目标 | 适用 |
| --- | --- | --- | --- |
| **容器内（默认）** | Fonu 容器 | `http://127.0.0.1:80`（容器内 nginx） | 绝大多数部署；规则先接入本机 nginx |
| host 网络 | Fonu 容器 `network_mode: host` | `http://127.0.0.1:80`（宿主机 nginx） | 需要直接访问宿主机端口、或已有宿主机 nginx |
| VPS Agent（可选） | VPS 上的 `fonu-agent` 托管 | `http://127.0.0.1:<frps vhost 端口>` | 已有 VPS、只想让部分域名走 CF 的高级用户 |

部署注意事项：

- cloudflared 是**出站 only**，不要为它额外映射任何入站端口。
- 二进制、`creds.json`、`cert.pem`、`config.yml`、日志都必须落在 `/data` 持久卷下，否则容器重建即丢失。
- 如果要暴露宿主机或内网服务，推荐先把该服务接入 Fonu 的本机 nginx 反代，再让 CF 指向 nginx；不要在 CF ingress 里直接写宿主机 IP。
- host 网络模式只作为显式选项，不默认开启；开启后 `127.0.0.1` 的语义会变化，UI 必须提示。

### 12.3 凭据与模式：显式区分「谁在生效」

参考实现证明了 token 可以解码，但也暴露了一个陷阱：**token 启动与本地 `config.yml` 是互斥的**。v2 把模式拆成三种，UI 必须让用户明确选择并显示当前生效侧：

| 模式 | 凭据 | 启动方式 | 规则来源 | 云端 fetch / push |
| --- | --- | --- | --- | --- |
| **A. token + 本地规则** | token | 解码 token → 写 `creds.json` → `tunnel --config config.yml run` | Fonu 生成的 `config.yml` | ❌（token 不含 API 凭据） |
| **B. token + 云端规则** | token | `tunnel run --token <token>` | Cloudflare Dashboard / 云端 | ❌ |
| **C. 账号 + 本地规则 + 云端同步** | `cert.pem` + credentials JSON | `tunnel --config config.yml run` | 本地，可与云端互相同步 | ✅ |

实施口径：

- 模式 A 绝不传 `--token`，只用 `--config`，否则本地规则不生效。
- 模式 B 必须在 UI 明确写「规则由 Cloudflare 面板管理，Fonu 的本地预览不生效」，不要让用户以为 Fonu 生成的规则已经下发。
- 模式 C 才提供「从云端拉取」「推送到云端」两个动作；token 模式不提供，只给 Dashboard 指引。
- 所有凭据走 `secret.Box` 加密字段；预览一律掩码，下发时用原文。`creds.json` 权限 `0600`，`config.yml` 权限 `0640`，目录 `0750`。
- 状态接口返回 `effective_side: local | cloud`，前端常驻显示，切换模式时要求二次确认。

### 12.4 数据模型与目录布局（Fonu 版）

参考实现用 JSON 文件 + 内存 HashMap，适合单进程小工具；Fonu 已有 SQLite 与迁移体系，v2 建议落成表：

`cf_tunnels`：

| 字段 | 说明 |
| --- | --- |
| `id` / `name` | 主键与用户可见名称 |
| `mode` | `token-local` / `token-remote` / `account-local` |
| `tunnel_id` / `account_tag` | 从 token 或 credentials 解析 |
| `credentials_enc` | token 或 credentials JSON 密文 |
| `cert_pem_enc` | 账号模式 `cert.pem` 密文 |
| `api_token_enc` | 可选的云端同步 API Token 密文 |
| `config_path` / `creds_path` / `log_path` | 运行文件路径 |
| `binary_version` | 当前使用的 cloudflared 版本 |
| `network_json` | protocol / edge-ip-version / ha-connections / proxy_mode / originRequest |
| `status` / `last_error` | `stopped` / `starting` / `connected` / `reconnecting` / `failed` |
| `metrics_port` | 从日志发现的端口，仅运行时缓存 |
| `auto_start` | Fonu 启动时是否自动拉起 |

目录布局：

```text
<DataDir>/cloudflared/
  bin/<version>/cloudflared
  tunnels/<id>/
    config.yml
    creds.json
    cert.pem
    cloudflared.log
```

多实例 = 多目录 + 多子进程 + 多日志文件；快速隧道单独用 `mode=quick` 记录，`public_url` 只在运行期有效。

### 12.5 进程与二进制管理

建议新建 `internal/cloudflared` 包，沿用 `internal/frp` 的 runtime / manager 模式，但不直接耦合 frp 模型：

**二进制**

- 统一镜像表：`official` / `gh-proxy-org` / `gh-proxy-hk` / `gh-proxy-cdn` / `gh-proxy-edgeone` + 自定义 URL 模板；修正参考项目「镜像列表与实现不一致」的问题。
- 下载校验：优先官方 checksums（若提供）；否则至少校验文件大小、可执行性、`cloudflared --version`；镜像源与官方源使用同一份哈希。
- 版本目录 + 原子替换 + 激活指针，复用 `internal/frp/runtime.go` 的 `readActiveBinary` / `writeActiveBinary` 思路。
- 支持 linux amd64 / arm64、windows amd64、darwin amd64 / arm64；Fonu 侧用 `runtime.GOOS/GOARCH` 选择。

**进程**

- `exec.Command` + stdout / stderr 逐行读取，同时追加写入 `<tunnel>/cloudflared.log`。
- Windows 设置 `CREATE_NO_WINDOW`（`0x08000000`）。
- 启动参数：
  - 模式 A / C：`tunnel --config <config.yml> run`
  - 模式 B：`tunnel run --token <token>`
  - 快速隧道：`tunnel --metrics 127.0.0.1:0 --url <target>`
- 网络参数：`--ha-connections`、`--origin-ca-pool`、`--no-tls-verify`、`--http-host-header`；环境变量 `TUNNEL_TRANSPORT_PROTOCOL`、`TUNNEL_EDGE_IP_VERSION`。

**metrics 端口发现**

- 命名隧道不传 `--metrics`；快速隧道传 `--metrics 127.0.0.1:0`。
- 从 stderr 用正则解析：`starting metrics server on 127\.0\.0\.1:(\d+)/metrics`。
- 解析到端口后轮询 `http://127.0.0.1:<port>/metrics`，最多等待 60 秒；超时给出明确原因，不留下空白状态。

**健康判定**

- `/ready` 返回 200：cloudflared 已就绪。
- `cloudflared_tunnel_ha_connections > 0`：已连上边缘。
- 延迟：QUIC 用 `quic_client_smoothed_rtt`；HTTP/2 用 `cloudflared_rpc_client_latency_secs` 或主动 HEAD 探测。
- 带宽：QUIC 直接用 `quic_client_receive_bytes` / `quic_client_sent_bytes` 差值；HTTP/2 官方没有字节指标，Fonu 第一轮明确降级为「不展示带宽」并给出原因，不做参考项目那套进程 socket 统计。

---

### 12.6 ingress 生成器（与 nginx 规则同源）

输入：

- 本机反代规则中启用且属于 HTTP / HTTPS 的域名；
- Fonu nginx 的容器内入口端口（默认 `80`，可用配置覆盖）；
- 用户勾选要暴露的域名集合。

输出：`config.yml` 的 `ingress` 数组。三条不变式必须写进生成器并用纯函数测试锁定：

1. 末条永远是 `- service: http_status:404`；
2. `localhost` 一律归一化为 `127.0.0.1`；
3. 新规则插在 catch-all 之前，且同域名去重、排序稳定。

与 nginx 的关系：

```text
访客 → Cloudflare 边缘 → cloudflared（Fonu 容器）→ 127.0.0.1:80（Fonu nginx）→ 上游
```

- CF 侧不重复配置限流、geo、TLS、Basic Auth；这些仍由本机 nginx 负责。
- 只允许 HTTP / HTTPS 规则进入 CF ingress；TCP / UDP 规则在界面上明确标为「Cloudflare 隧道不支持」。
- 生成后提供「配置预览（脱敏）」与「生成后自检」，自检至少覆盖三条不变式与空 ingress 场景。

### 12.7 网络调优（默认值与下发方式）

| 设置 | Fonu 默认 | 下发方式 | 说明 |
| --- | --- | --- | --- |
| `transport_protocol` | `http2` | 环境变量 `TUNNEL_TRANSPORT_PROTOCOL` + `config.yml` 的 `protocol` | 国内 QUIC（UDP 7844）常被 QoS，TCP 更稳；提供 `auto` / `quic` |
| `edge_ip_version` | `4` | 环境变量 `TUNNEL_EDGE_IP_VERSION` + `config.yml` 的 `edge-ip-version` | 2026.4.0 起官方默认改为 `auto`，国内 IPv6 路由差时反而变慢；提供 `auto` / `6` |
| `ha_connections` | `2` | `--ha-connections` + `config.yml` 的 `ha-connections` | 每个 region 的连接数，1–4 可调 |
| `proxy_mode` | `system` | 启动时按需移除 / 保留 `HTTP_PROXY` 等环境变量 | `disabled` 时清空代理变量；检测到代理或 Windows 时协议回落 `http2` |
| `originRequest` | 空 | `--origin-ca-pool` / `--no-tls-verify` / `--http-host-header` | 仅高级用户；默认不回源 HTTPS，避免自签证书问题 |

诊断接口返回：平台、代理环境探测结果、实际协议、实际代理模式、建议。降级必须给出原因，例如 `windows_estats_requires_admin`、`http2_official_metrics_unavailable`。

边缘优选（阶段 3 可选）：

- 只做「本机 → CF 边缘」的 TCP 7844 延迟探测与展示，不代写 hosts；
- 两个 region 都要覆盖，IP 列表可配置、有默认值、可一键恢复默认；
- 访客到自己那侧 CF 边缘的一段无法优选，功能说明必须写清楚。

### 12.8 云端同步（仅账号模式）

账号模式（`cert.pem`）才提供云端同步，API 细节：

- 拉取：`GET https://api.cloudflare.com/client/v4/accounts/{account_id}/cfd_tunnel/{tunnel_id}/configurations`
- 推送：`PUT` 同一路径，body 为 `{"config":{"ingress":[...]}}`
- 推送的 ingress 必须包含末条 catch-all，否则 Cloudflare 可能拒绝或行为不确定。

冲突处理：

- UI 常驻显示 `effective_side: local | cloud`；
- 「拉取到本地」会覆盖本地 `config.yml`，操作前备份当前版本（复用本仓 `fsutil.BackupVersioned`）；
- 「推送到云端」会覆盖云端 ingress，操作前展示 diff 并二次确认；
- 两个动作都写审计日志（操作者、隧道、方向、结果）。

token 模式不提供 push / fetch，只给 Dashboard 指引；这是凭据能力的硬边界，不要用「同步失败」这类模糊提示掩盖。

### 12.9 与 Fonu 现有模块的集成点

| Fonu 模块 | 复用还是新增 | 具体落点 |
| --- | --- | --- |
| 二进制下载 / 启用 | 新增 `internal/cloudflared`，参考 `internal/frp/runtime.go` | 镜像表、哈希校验、版本目录、激活指针 |
| 进程托管 | 新增 `internal/cloudflared`，参考 `internal/frp/manager.go` | 启停、日志追加、退出原因、`tailFile` |
| 健康与可用率 | 复用 `internal/monitor` 的 `Source` / `HealthRecorder` | 新增 CF Tunnel Source；`/ready` + `ha_connections` |
| 通知 | 复用 `internal/notify` | 隧道断开 / 恢复事件 |
| 状态页 | 复用现有公开状态页 | 新增隧道条目与可用率 |
| 规则来源 | 复用 `internal/proxy` 的本机反代规则 | 只取 HTTP / HTTPS 域名，生成 ingress |
| 日志界面 | 复用现有日志页模型 | 读 `<tunnel>/cloudflared.log`，不引入 WebSocket |
| 凭据加密 | 复用 `internal/secret` | token / cert.pem / credentials JSON 全部加密 |
| frp 链路 | **不改** | CF Tunnel 是并存出口，不碰现有 frp 规则、健康与状态语义 |

### 12.10 修订后的阶段计划

| 阶段 | 交付物 | 验收标准 |
| --- | --- | --- |
| 阶段 1：凭据与配置 | 凭据导入（token / cert.pem）、模式选择、`creds.json` + `config.yml` 生成、脱敏预览 | 三种模式都能生成文件；模式 B 明确提示本地规则不生效；凭据不明文回显 |
| 阶段 2：进程与健康 | 二进制下载 / 启用、进程启停、日志、metrics 端口发现、`/ready` + 连接数健康 | 启动后能连上边缘；停止后进程与端口释放；健康状态落库并进通知 |
| 阶段 3：规则同源与云端同步 | 从本机反代规则生成 ingress、漂移检测、账号模式 fetch / push、网络调优设置 | ingress 三条不变式测试通过；云端同步有 diff 与审计；网络参数可配置 |
| 阶段 4（可选） | 边缘优选、快速隧道、高级 originRequest | 优选只展示不越权；快速隧道明确标注「临时演示」 |

### 12.11 测试与验收

单元测试：

- token 解码：标准 base64、缺填充、非法输入、`a/t/s` 与 `AccountTag/TunnelID/TunnelSecret` 两种字段名；
- ingress 生成：catch-all、`localhost` 归一化、插入位置、去重、空规则；
- metrics 解析：端口行、QUIC / HTTP/2 指标、连接数、降级原因；
- 网络参数构建：protocol / edge-ip-version / ha-connections / proxy_mode；
- 镜像 URL 与自定义模板；
- 凭据脱敏：token、cert.pem、credentials JSON 不出现在预览与日志。

集成测试：

- 启动 / 停止 / 重启 / 自动启动；
- metrics 端口发现与健康状态流转；
- 日志追加与 tail；
- 云端 fetch / push 用 mock API 验证请求体、catch-all 与错误处理；
- 容器内 `127.0.0.1:80` 回源链路。

前端：

- 隧道列表 / 详情 / 状态 / 日志 / 配置预览；
- 模式选择与「当前生效侧」提示；
- 脱敏预览与二次确认。

真机矩阵：Linux amd64 / arm64、Windows、Docker 容器内、host 网络、代理环境、国内与海外链路。

### 12.12 风险与边界（补充）

- **容器网络**：默认拓扑是容器内 cloudflared → 容器内 nginx；不要假设 `127.0.0.1` 是宿主机。
- **token 覆盖**：模式 B 的本地 `config.yml` 不生效；UI 必须明确，模式 A 才能本地管规则。
- **HTTP/2 带宽**：官方 metrics 没有字节指标；第一轮明确降级并说明原因，不做进程 socket 统计。
- **Windows**：`CREATE_NO_WINDOW` 必带；若做进程级字节统计还需要管理员权限，Fonu 第一轮不做。
- **CF 条款**：视频 / 大文件分发不在推荐用法内；TryCloudflare 不保证 SLA。
- **国内链路**：CF 边缘延迟受运营商影响；优选只影响本机到边缘一段。
- **API 凭据**：账号模式的 `cert.pem` 与可选 API Token 都是高权限凭据，必须最小化权限并加密存储。
- **Access**：只做诊断提示，不代配策略。
- **TCP / UDP**：CF Tunnel 不是通用 TCP 出口；需要访问端装 cloudflared 的场景不在首轮范围。
- **不改 frp**：现有 frp 链路、健康与状态页语义保持不变。

### 12.13 结论

Cloudflare Tunnel 对 Fonu 是**第二条出口**，不是 frp 的替代：它补上「没有公网 IP、也不想长期租 VPS」的 HTTP 类场景。v2 的核心修订是四件事：

1. cloudflared 跑在 Fonu 容器内，ingress 统一指向容器内 nginx，绕开容器网络坑；
2. 显式区分「本地规则生效」与「云端规则生效」，不让 token 模式悄悄覆盖本地配置；
3. metrics 端口从日志发现，健康用 `/ready` + `ha_connections`，HTTP/2 带宽明确降级；
4. 二进制下载补哈希 / 版本校验，ingress 生成器锁定三条不变式。

按 §12.10 的阶段 1–3 实施即可形成可用闭环；阶段 4 的边缘优选与快速隧道作为可选增强，不阻塞主线。

---

> 说明：本文只做方案分析，未改动任何代码。实施时沿用本仓库既有约定：每步验证 `go build/vet/test` + `vue-tsc/vite build`、agent 行为变更递增 `AgentVersion`、提交同批更新 `CHANGELOG` 与 `VERSION`、密钥相关内容必须过一遍「预览脱敏 / 下发原文」两头检查。
