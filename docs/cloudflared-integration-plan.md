# Cloudflare Tunnel（cloudflared）与 Fonu 的融合方案

> 日期：2026-09-25　对象：Fonu 1.0.2（VERSION 1.2.12）
> 方法：先查 Cloudflare 官方文档与同类开源项目，再逐个核对本仓库现有可复用面（下文带 `文件:行` 的都是读过代码得出的）。
> 结论速览：**值得做，但建议以「第二条出口」的形态并存，先做 locally-managed + 进程托管，不接 Cloudflare API**。详见 §五、§六。

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

> 补充（参考 §九 后修正）：这个二分法不够细。参考实现证明**token 可以解码**（base64 的 JSON，含 `AccountTag` / `TunnelID` / `TunnelSecret`），用它自己生成 `creds.json` 就能按 locally-managed 跑本地 `config.yml`；再配合「云端 ↔ 本地 ingress 互相同步」，两种模式实际上是可切换的。所以 Fonu 的正确做法不是「二选一」，而是**显式区分并同步**：当前生效的是本地 `config.yml` 还是云端 ingress，以及「推送/拉取」两个动作。

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

> 说明：本文只做方案分析，未改动任何代码。实施时沿用本仓库既有约定：每步验证 `go build/vet/test` + `vue-tsc/vite build`、agent 行为变更递增 `AgentVersion`、提交同批更新 `CHANGELOG` 与 `VERSION`、密钥相关内容必须过一遍「预览脱敏 / 下发原文」两头检查。
