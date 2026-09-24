# Changelog

All notable changes to this project will be documented in this file.

## [v1.2.4] - 2026-09-24

### 新增

- 公网 Agent 能力扩展（agent 0.10.0）：新增三个只读端点——`GET /api/v1/metrics`（VPS 主机资源：CPU / 内存 / 磁盘 / 负载 / 运行时长，CPU 用两次采样差值，1 秒内的重复请求直接复用上次结果）、`GET /api/v1/logs/nginx`（access / error 日志尾部，路径只从 `nginx -T` 实际生效配置、`FONU_AGENT_NGINX_LOG_DIR` 或默认目录取；按 32KB 块倒读，单行 4KB 截断，上限 2000 行）、`GET /api/v1/frps/config`（frps.toml 回读，256KB 上限，文件不存在返回 `exists: false`）
- 公网 Agent 页：新增「VPS 资源」横排带（CPU / 内存 / 磁盘三项并排 + 进度条 + 右侧平均负载与运行时长），占满「环境与依赖」整行，不做单独占格
- 日志中心：新增「公网 Nginx」页签（服务端 / 访问或错误日志 / 行数 200·500·2000 + 关键词搜索 + 分页），并显示 agent 解析出的实际日志路径与「仅显示尾部」标记
- 公网服务端「frps.toml 同源配置」弹窗：新增「比对 VPS 实际文件」——去注释与空行后逐行比对，一致给出绿色说明，不一致指出第一个差异行（下发值 / VPS 值）

### 改进

- `internal/sysinfo` 补 `LoadAvg()` 与 `UptimeSeconds()`：Linux 读 `/proc/loadavg`、`/proc/uptime`，Windows 用 `GetTickCount64`，其它平台返回空值
- 老版本 agent 的降级：三个新端点遇到 HTTP 404/405 时统一转成「VPS 上的 agent 版本过低，不支持该接口（需 ≥ 0.10.0），请在服务端详情里升级 Agent」的可操作提示，而不是原始 HTTP 错误

### 变更

- `AgentVersion` 0.9.2 → **0.10.0**；内嵌 `fonu-agent-linux-amd64`（9.9 MB）与 `-arm64`（9.2 MB）已用新代码重编，VPS 上需点「升级 Agent」才会生效
- 版本号提升到 1.2.4（版本单一来源仍是根目录 `VERSION`）

## [v1.2.3] - 2026-09-24

### 新增

- 仪表盘：新增「公网穿透」统计卡，并在「运行状态」面板增加「公网反代」行——数据来自新增的 `GET /api/frpmulti/routes/summary`（逐台服务端采集四段健康后聚合，单台失败只记名不中断）
- 仪表盘：新增「性能」面板（今日异常 / 平均响应，`/api/status` 早已返回但页面从未展示），「实时流量」面板新增「流量 Top 5」（直接复用已有的逐规则流量轮询）
- 仪表盘：新增一行三卡「系统资源 / 中国 IP 段 / 通知通道」；系统资源来自新增的跨平台采集包 `internal/sysinfo`（Linux `/proc`、Windows kernel32，无新增依赖；容器内为宿主机视角）
- 公网反代：新增部署漂移清单（未部署 / 孤儿配置 / 未生效 / 内容不一致）与「重新部署全部」——逐条下发、单条失败不中断；新增 `GET /api/frpmulti/servers/{id}/agent/routes/drift` 与 `POST .../agent/routes/redeploy`
- 公网 Agent：端口矩阵补端口含义提示；「frps 运行时」新增「启动一致性」核对（systemd 实际启动命令 vs 配置的二进制与配置文件，检出「Fonu 写入的 frps.toml 不是 frps 实际加载的那份」）
- 新增 `docs/agent-capability-plan.md`：fonu-agent 能力扩展方案（12 个端点现状盘点、P0–P2 分批设计、agent 侧与应用侧改动流程、验收与风险回滚）

### 改进

- 仪表盘「服务」卡：原先写死「正常 / 4 个」，现按 Fonu / Nginx / DDNS / 证书四项实际状态聚合，异常时点名并把徽标降级为 warning/error（此前 Nginx 停止、DDNS 异常时该卡仍显示「正常」，与右侧「运行状态」面板自相矛盾）
- 仪表盘轮询：`/api/status` 每 15 秒刷新一次（CPU 占用率需要两次采样才有意义），并在页面隐藏时暂停「实时流量」与「状态」两个轮询
- 公网反代健康列：证书徽标补「证书来源」——`Fonu 证书库：有（剩余 N 天）` 或 `无（需先在证书管理申请）`，用于区分「没有证书可推送」与「VPS 上证书过期或未覆盖」

### 修复

- FRP：frpc 版本读不出来——`RuntimeStatus.Version` 带 `omitempty`，`frpc --version` 探测失败时字段被整体省略，前端只能显示「未检测 / 未知」；现回落为按 `<FrpDir>/bin/<版本>/` 目录名推导（与「FRP 应用配置」弹窗的 `active_version` 同源），并改为逐台服务端探测版本（原先只探测默认那台）
- 仪表盘：删除死样式（`.dashboard` / `.stat-highlight` / `.proxy-metric*`）

### 变更

- 版本号提升到 1.2.3（版本单一来源仍是根目录 `VERSION`）

## [v1.2.2] - 2026-09-23

### 新增

- 日志中心：新增「FRP 日志」Tab——把 frpc 运行日志解析为 时间 / 级别 / 来源 / 消息 四列，支持关键词与级别筛选、分页、错误/警告行着色，与系统日志、访问日志、Nginx 日志同一套交互（对齐上游 1.0.3）
- 新增 `web/src/utils/clipboard.ts`：统一的剪贴板复制工具（异步剪贴板 API 优先，失败或非安全上下文时回退 `execCommand`）

### 改进

- 剪贴板复制在非 HTTPS 环境可用：Fonu 常以 `http://内网IP:6893` 访问，属非安全上下文，此时 `navigator.clipboard` 为 undefined，复制必然失败；公网反代（复制域名 / 复制配置）、公网 Agent（配置查看）、DDNS、本地反向代理（复制访问链接）共 5 处调用点统一改用上述工具（对齐上游 1.0.4）
- 容器时区：`Dockerfile` 新增 `TZ=Asia/Shanghai` 并在运行时阶段安装 `tzdata`（只设 `TZ` 而镜像内无 zoneinfo 时 Go 仍会回落 UTC），三个 compose 文件均支持 `TZ` 覆盖——通知消息时间按系统时区显示（对齐上游 1.0.4）
- 反代配置查看弹窗：补齐内容区内边距（与头部 24px 对齐），新增来源说明（由 VPS 上的 fonu-agent 生成、文件名 `fonu-<域名>.conf`、手工修改会被下次部署覆盖），并改为说明固定 + 代码块内部滚动
- 反向代理：复制规则不再自动追加「-复制」后缀，保留原名称与域名、由用户在表单里确认修改（原先一点复制就得到一个改过名的副本，容易误建同名规则）（对齐上游 1.0.5）
- FRP 穿透页：frps 同源配置弹窗新增「VPS 防火墙 / 安全组放行 TCP 远程端口」提示，端口列表由已启用的 TCP/UDP 规则实时推导（与 frps 的 `allowPorts` 同源；未放行时隧道连不上，但日志不会点明原因）（对齐上游 1.0.3）
- 通知：Telegram 代理地址只填 `host:port` 时自动补全 `http://`，并校验 scheme 仅支持 http / https / socks5，错误信息带上原始地址便于自查（对齐上游 1.0.3）

### 修复

- 通知：单个通道密文解密失败不再让整份配置加载失败——原先 SMTP / Webhook / Telegram 任一密文损坏（例如更换过加密密钥）都会直接返回错误，导致其它已配置通道一并无法告警；现改为仅把该通道标记为「未配置」并继续解析其它通道（对齐上游 1.0.5）
- 反向代理：监听占用冲突提示写明占用方——优先规则备注名，其次该规则首个域名，最后回落 `规则 #id`；原先只说「已被其他规则使用」，用户得自己翻列表找是哪一条
- 反向代理：手动（custom）Nginx 配置的监听占用纳入校验。custom 配置的正本在文件系统（数据库只记 `nginx_mode` 开关），其 `listen` / `server_name` 原先完全不参与校验，与其它规则「同域名 + 同端口」的冲突只会在 Nginx reload 时静默失效（第一个 server 块生效、后面的被忽略）；现于保存 / 回滚时解析并写入新表 `proxy_custom_endpoints`，与表单规则共用同一条判定（对齐上游 1.0.5）
- 反向代理：并发下修改监听端口可能绕过保存前校验（`UPDATE proxy_hosts` 的唯一约束冲突此前直接抛出原始 SQL 错误），现改为按域名复检并给出带规则名的提示

### 变更

- 新增 `docs/upstream-sync-plan.md`：上游 1.0.2 → 1.0.5 的逐版本对照方案，含 8 项待核对项的代码级核实结论、不照搬项及原因、以及「可增加项清单」（11 项，按价值排序）
- 新增迁移 `024_proxy_custom_endpoints.sql`（手动 Nginx 配置的占用表，`UNIQUE(rule_id, hostname, listen_port)` + `ON DELETE CASCADE`），解析器 `internal/nginx/endpoints.go`（`ParseServerEndpoints`，保守解析：`include` 片段 / 正则 `server_name` / `listen unix:` 一律忽略，宁可漏报不误报）与 `internal/proxy/custom_endpoints.go`
- 补测试：`internal/nginx/endpoints_test.go`（多 server 块、`[::]` 写法、通配与正则、无 `listen` 默认 80）、`internal/proxy/custom_endpoints_test.go`（走真实迁移建内存库：冲突含规则名、排除自身、同域名不同端口放行、custom 占用对表单规则可见、级联清理）
- `docs/upstream-sync-plan.md` 补 §八「第 5 项详细整理」：Nginx 端口占用语义（一条规则占用哪些端口）、现有校验链逐跳、三处真实缺口与方案 A/B/C 对比、实施记录与未覆盖边界


## [v1.2.1] - 2026-09-23

### 改进

- 公网 Agent 页：按「功能清单」重排为五段分区（运行状态 / 环境与依赖 / frps 控制 / Agent 生命周期 / 输出与日志）；4 张状态卡统一为标准 `.stats-row` + `.stat-card` 规格；补齐 5 项「数据已返回但页面未显示」的能力——Agent 凭据状态、8080 端口、已注册反代路由数、最后检查时间、Nginx 可用性
- 公网 Agent 页：「环境与依赖」改为三列并排（Fonu Agent / Nginx / frps 运行时），卡内短字段双列、长路径整行；「frps 控制」与「Agent 生命周期」两列并排；新增「配置查看」按钮（只读弹窗展示即将下发的 frps 配置，含复制），卸载（含数据）增加二次确认
- 公网反代页：规则台账改用 `n-data-table`（与本地反向代理页一致，表头与数据天然对齐），列为 规则/域名（含复制与打开按钮）、协议、上游/回源、健康（DNS·证书·隧道·服务）、连接、当前上传、当前下载、总上传、总下载、更新时间、操作
- 公网反代页：新增统计区（可部署规则 / 已生效 / 健康通过 / 当前连接）与环境与 Nginx 卡（4 列）；「使用说明」整卡改为页面级提示与入口行；部署设置弹窗对齐本地反向代理页规格（900px、三段式、页脚分隔线、开关行底色）并新增右侧「配置说明」栏
- 公网反代页：协议列改为按部署设置的「启用 HTTPS」自动识别（规则 type 恒为 http，不能代表入口协议），HTTPS 绿色 / HTTP 灰色并带悬停说明
- FRP 穿透页：4 张状态卡统一为标准统计区；公网服务端列表由自建网格改为 `n-data-table`（8 列，含启动/停止/查看/日志/测速/校验/Agent 管理/编辑/删除）
- FRP 穿透页：6 个弹窗统一到同一外壳（`.frp-card-modal` / `.frp-detail-card`），运行日志、导出配置、frps 同源配置、FRP 应用配置的头部/滚动区/页脚一致
- 统一设计令牌：清理硬编码色值（公网反代页 6 处、FRP 穿透页 13 处）与内联样式，删除死样式；统计区、弹窗、表格单元格全部走 `--fonu-*` 令牌与共享组件（`StatusBadge` / `EmptyState` / `LoadError` / `FonuCard`）
- 表格与列表交互：数字列右对齐、操作按钮右对齐并按行禁用、长文本换行、单元格内容垂直居中；窄屏按 1200 / 900 / 760 断点降列

### 修复

- FRP 穿透页「运行中」状态显示为灰色：`statusKind` 返回 `'success'` 却被当作 `value` 传给 `StatusBadge`（`utils/status.ts` 不识别该值，落到 `unknown`），且其中含非法的 `'default'`；现改为返回合法 `StatusKind` 并改用 `:kind`，服务端详情弹窗同类问题一并修复
- 表格单元格样式整体失效：`n-data-table` 的 render 函数用 `h()` 创建的节点不带 scoped 属性，导致 `table-muted` / `type-chip` / `mono` 与各 `cell-*` 样式未生效（表现为「上游/回源」两行变一行、域名与按钮错位、徽标无间距）；统一改用 `:deep()` 选择器
- 公网反代页刷新失败静默：`loadErrorMsg` 此前写入但从未渲染，现改为顶部可见提示，并让单个接口失败不再中断整轮数据加载
- 公网反代页切换服务端残留上一台数据、轮询在页面隐藏时仍请求、loading 粒度粗（任一操作全页按钮转圈）等问题修复

### 变更

- 新增设计文档：`docs/agent-view-design-v2.md`（公网 Agent 页）、`docs/proxy-remote-design.md`（公网反代页），以及被取代留档的 `docs/agent-view-design.md`、`docs/agent-view-refactor-plan.md`
- 新增页面 UI 标准 skill（工作区级 `.breezell/skills/fonu-page-ui`，不在仓库内）：页面骨架、共享组件契约、设计令牌、类名约定、弹窗标准结构、脚本约定与验收清单

## [v1.2.0] - 2026-09-23

### 新增

- FRP 公网反代：独立「公网反代」页（`/public-proxy`）——服务端选择、规则列表（名称 / 域名 / 内网目标 / 回源端口）、部署与移除、配置查看、15 秒自动刷新
- FRP 公网反代：**部署后回源自检**——agent 写入配置后模拟 Nginx 回源（`Host: 域名` → `127.0.0.1:<vhost 端口>`）并分级提示：0=连接不上 vhost、404=隧道未注册、5xx=内网服务异常，不再出现「部署成功但打开 502」
- FRP 公网反代：**状态徽章实时化**——基于 `nginx -T` 同时判定「文件存在 / 语法通过 / 实际被加载」，展示已生效 / 已写入未加载 / 语法错误 / 未部署
- FRP 公网反代：**配置查看**——`GET /api/v1/routes/{domain}/conf` 直读 VPS 上生成的 nginx 配置，排障无需 SSH
- FRP 公网反代：**五段健康检查**（逐规则）——DNS 指向、证书覆盖与剩余天数、隧道注册、内网服务探活、反代生效态；任一断点红标 + 悬停显示原因
- FRP 公网反代：**流量列**——当前连接 / 当前上传 / 当前下载 / 总上传 / 总下载（取自 frps dashboard 聚合）
- FRP 公网反代：「部署设置」弹窗（与本地反向代理弹窗同构，三 Tab）
  - 基础：启用 HTTPS（显式开关）、HTTP 强制跳转、WebSocket、上传大小限制、代理读超时
  - 安全：IP 白名单、IP 黑名单、Basic Auth（用户名 + 自定义密码，留空首次自动生成）、仅中国大陆 IP
  - 高级：安全响应头（HSTS / X-Frame-Options / X-Content-Type-Options / Referrer-Policy）、仅 TLS 1.3、请求限流（速率 / 突发）、每 IP 连接数上限
- FRP 公网反代：限流与连接数落地——agent 生成 `fonuagent-limits.conf`（http 上下文 zone 定义，由状态文件驱动增删），server 内输出 `limit_req_status 429` / `limit_conn_status 503`
- FRP 公网反代：仅中国大陆 IP——Fonu 把本地维护的中国 IP 段下发到 agent（`PUT /api/v1/china-cidr`），agent 生成 `geo $fonu_china_client`；IP 段未更新时部署显式报错，不产出半成品配置
- FRP 公网 Agent：新增独立「公网 Agent」页（`/frp-agent`）——agent 状态面板、frps 配置下发、frps 版本安装 / 升级、frps 启停与重启、frps 管理入口、日志、SSH 生命周期与诊断
- FRP 公网 Agent：**Nginx 状态卡片**——状态徽标（正常 / 未安装 / 配置校验未通过 / 已安装未运行）、版本、可执行路径、vhost 配置目录、`nginx -t` 结果、80/443 监听、反代配置文件数
- FRP 公网 Agent：**一键安装 Nginx**——agent 识别 apt-get / dnf / yum / apk 非交互安装并启动，回写探测结果（需 agent 以 root 运行）
- FRP 公网反代：兼容 frps v0.68 与 v0.71+——新版走 `GET /api/v2/proxies`，旧版走 `GET /api/proxies/{user.name}` 逐条查询

### 改进

- FRP 公网反代：部署选项（WebSocket / 强制跳转 / IP 白名单 / Basic Auth 等）从穿透规则弹窗迁移到公网反代页「部署设置」，语义归位（这些是公网反代属性，与隧道无关）
- FRP 公网反代：隧道匹配优先按域名（`spec.http.customDomains`），比按代理名猜测可靠；同时兼容 `user` 前缀（`admin.<规则名>`）与裸规则名
- FRP 公网反代：frps 管理接口调用失败时把具体原因透出到前端悬停（请求 URL、状态码、响应体、密码读取 / 解密环节），不再笼统提示「接口不可用」
- FRP 公网反代：部署预检——未安装 Nginx 时直接提示「VPS 未安装 Nginx，请先安装（apt install nginx / yum install nginx）」，替代 `nginx config rejected` 技术堆栈
- FRP：agent 部署接口由 10 个位置参数改为 `RouteSpec` 结构体（`SyncRouteSpec`），`SyncRouteEx` 保留为兼容包装
- FRP：agent 版本递增至 0.9.1，状态新增 `nginx_ok` / `nginx_missing` / `nginx_running` / `nginx_path` / `nginx_version` / `nginx_error`
- FRP：agent 0.9.2——Basic Auth 哈希改为**纯 Go apr1**（与 `openssl passwd -apr1` 输出逐字节一致，已用参考向量做单元测试），安装脚本 token 改用 `/dev/urandom`：agent 与安装流程彻底去除对 VPS 上 openssl 的依赖（原先缺 openssl 时会静默写入空 token、导致 Fonu 无法认证 agent）

### 修复

- FRP 公网反代：隧道状态误报 ✗——`RouteHealth` 静默忽略 dashboard 密码解密错误导致空密码 401，现逐环节记录并透出
- FRP 公网反代：隧道检测端点错配——旧代码请求 `/api/proxies`（405）或按类型子端点（404），现按 frps 版本自适应并解析 V2 的 `data.items`
- FRP 公网反代：流量恒为 0——V2 返回的键是 `admin.ssssss` 而代码只查裸规则名，现按「裸名 / user 前缀名 / 域名索引」三级匹配
- FRP 公网反代：列表健康徽标样式丢失（此前无样式渲染）
- 修复被外部进程清空的 `internal/frp/store.go`、`internal/api/ddns_handler.go`、`web/src/views/DdnsView.vue`

### 变更

- FRP 公网反代管理从「公网 Agent」页拆分为独立页面（`/public-proxy`），侧栏新增「公网反代」
- 新增 agent 端点：`GET /api/v1/routes/{domain}/conf`、`PUT /api/v1/china-cidr`、`POST /api/v1/nginx/install`
- 新增 Fonu 接口：`GET /api/frpmulti/servers/{id}/agent/routes/health`、`GET /api/frpmulti/servers/{id}/agent/routes/{proxyID}/conf`、`POST /api/frpmulti/servers/{id}/agent/nginx/install`
- 明确不做（已写入 `docs/agent-proxy-settings-plan.md`）：公网反代不使用「目标 Host 头」（会破坏 frps vhost 路由导致 404）、不做「忽略后端 TLS 校验」（回源为明文 HTTP）、不做公网侧自定义 upstream 覆盖（会绕过隧道）

## [v1.1.0] - 2026-09-19

### 新增

- FRP：新增公网 `fonu-agent` 服务端管理基础能力——VPS 侧提供 Bearer Token API，Fonu 本地可测试 Agent、读取状态、下发 frps.toml，并在 HTTP/HTTPS 规则保存后同步公网 Nginx 反代路由
- FRP：服务端新增「Agent 管理」弹窗——API 状态面板（连接/frps/Nginx/80-443 端口监听/已注册路由）与 SSH 生命周期操作（探测环境、一键安装/升级/卸载、SSH 诊断），安装后自动回填 Agent 地址与 Token；agent 二进制内嵌随主程序分发，无需单独下载
- FRP：模块整体升级为多服务端管理——支持添加多台 frps（独立启动 / 停止 / 日志 / 测速 / 校验），穿透规则支持全部 8 种类型（tcp / udp / http / https / tcpmux / stcp / sudp / xtcp）与插件（static_file / socks5 / https2http 等，按插件类型校验必填字段）
- FRP：服务端详情五项独立诊断（TCP 可达 / frpc 登录 / 代理可达 / DNS 解析 / 归属地），含最近 30 分钟可用率与延迟曲线
- FRP：frps.toml 同源生成——服务端「详情」一键生成与 frpc 侧配置同源的服务端配置（bindPort / 认证 / webServer / 传输参数 / vhost 端口 / 防火墙放行清单）；支持 token / 无认证 / OIDC 三种方式；格式对齐官方 `frps_full_example.toml`（v0.52.0+ TOML，v0.63.0+ 均适用）
- FRP：穿透规则实时流量与连接数展示（需在 frps 配置 webServer 管理接口）
- FRP：「应用配置」旁新增「使用说明」——配置流程、DDNS / 反向代理 / 证书组合说明与十个组合场景详解（双标签页 + 手风琴折叠展示）
- FRP：frpc 客户端多版本管理（「应用配置」弹窗内下载 / 启用 / 删除 / 配置下载代理）
- DDNS：新增「自定义 IP」——按任务设置固定 IPv4 / IPv6（如公网服务器 IP），更新时直接使用该 IP，不再检测本机出口 IP；须为公网地址，否则拒绝更新

### 改进

- FRP：脱敏令牌完整闭环——Token / OIDC 客户端密钥 / frps 管理密码掩码往返保护，回传掩码不会覆盖真实凭据；表单统一「已配置」标识
- FRP：服务端详情「配置」面板重新分组排版（描述 / 连接与认证 / 诊断）
- 前端：使用说明弹窗对齐「添加穿透规则」弹窗的自绘外壳样式

### 变更

- FRP：旧「Nginx 网关模式」退役——原 5 条 `/api/frp` 接口移除，由 `/api/frpmulti` 接管；旧实现归档为 `internal/frp-1.0.2`（不参与运行、可整目录删除）；旧网关配置（settings 键值）保留但不再读取，升级后需在「FRP 穿透」页重新配置服务端
- 数据库：新增迁移 015–021（FRP 多服务端核心表）与 022（DDNS 自定义 IP 字段），升级后首次启动自动应用

## [v1.0.2] - 2026-09-18

### 新增

- 证书：支持火山引擎 DNS 的 ACME DNS-01 验证

## [v1.0.1] - 2026-09-18

### 新增

- DDNS：支持火山引擎 DNS（AccessKey / Secret Access Key）
- DDNS：委托子域与多层子域名 Zone 自动匹配（最长后缀优先，如 `a.zzz.com`）

### 改进

- DDNS：解析记录统一展示完整域名；任务列表仅显示记录条数
- DDNS：保存时校验域名格式（非法字符、标签长度、连字符位置）
- 证书：火山引擎 DNS 任务申请证书时给出明确不支持 DNS-01 的提示

### 修复

- DDNS：多层子域名（如 `aaaa.vvv.bbb.com`、`www.a.zzz.com`）保存后被截断的问题
- DDNS：同一任务混用多根域/委托子域时 Zone 解析错误

## [v1.0.0] - 2026-09-18

### 新增

- 通知：支持 SMTP 邮件、Webhook 预设（Bark / ntfy / Gotify / 自定义）与 Telegram 机器人（可选 HTTP 代理），全局仅可启用一种方式
- 设置页新增「测试通知」按钮，无需保存即可验证当前配置
- 告警事件：DDNS / 证书 / 安全 / 系统分组，含 IP 变更、续签成功/失败、登录异常、Nginx 重载失败等
- 设置页改为「常规 / 通知 / 安全 / 高级」分栏，减轻单页信息密度
- 日志中心：系统日志、访问日志、Nginx 错误日志三 Tab；移除实时日志 SSE
- 凭据类字段统一「已配置」标识与「留空表示不修改」占位（SMTP / Webhook / Telegram / DDNS / FRP / Basic Auth 等）

### 改进

- 旧版 `notify_webhook_url` 自动迁移为「自定义 Webhook」
- 通知密钥加密存储，接口不再返回明文；保存后表单状态与服务端同步
- 系统日志中文展示（历史英文日志兼容翻译）
- 仪表盘与反代详情「最近访问」展示反代服务名而非域名
- README 界面截图更新为原图，两列排版展示

### 修复

- 通知 / 系统设置保存后刷新丢失的问题

## [v0.1.31] - 2026-09-18

### 新增

- 反向代理 Nginx：支持单条规则自动/手动编辑、备份与回滚；详情页只读预览，编辑弹窗内配置
- 设置页 Nginx 全局片段：http 块自定义编辑，默认上传大小 50M 片段与框架说明
- Nginx 编辑器：CodeMirror 语法高亮；自动生成配置附带中文注释
- 保存 Nginx 时基础语法校验（无 nginx 可执行文件时亦可拦截明显错误）

### 改进

- 反代日志面板：移除「隐藏轮询」开关，简化日志工具栏

## [v0.1.30] - 2026-09-17

### 修复

- 反向代理 Basic Auth：htpasswd 文件补充 nginx 可读权限与 bcrypt 前缀兼容，修复认证后 500 错误
- 反代连接数：按「域名 + 监听端口」统计，修复同域名多规则互相串数的问题
- 仪表盘「今日请求」：排除错误页与 favicon 噪音，趋势改为较昨日对比

### 新增

- 反代详情日志：支持全屏查看实时访问日志

## [v0.1.29] - 2026-09-17

### 修复

- 反向代理 Basic Auth：修复密码哈希在 Nginx 重载前被剥离导致认证不生效的问题
- 反代上传：全局默认 `client_max_body_size 50m`，修复大文件上传 413 错误

### 改进

- README 更新界面截图（v0.1.28）并补充开源地址与协议说明

## [v0.1.28] - 2026-09-16

### 修复

- 仪表盘「今日请求」：改为今日 0–24 时固定坐标轴（00:00–22:00），未来时段为空

### 改进

- FRP Token：保存后显示「已保存」状态，说明不显示明文的原因
- FRP Web 穿透：移除重复域名输入框，统一在「已同步域名」弹窗中编辑保存

## [v0.1.27] - 2026-09-16

### 修复

- FRP：移除无效的 `pidFile` 配置项，修复 frpc 启动即退出（exit status 1）
- FRP：关闭穿透时保存按钮不可用，导致无法真正关闭
- FRP：frps 配置随连接设置实时生成，不再展示静态示例
- 仪表盘「今日请求」：改为滚动最近 24 小时统计，修复当前时段无数据

### 改进

- Docker 镜像内置 frpc 升级至 v0.71.0

## [v0.1.26] - 2026-09-16

### 新增

- FRP 内网穿透（Nginx 网关模式）：独立「内网穿透」页面配置 frps 连接，自动穿透本地 Nginx HTTP/HTTPS 端口
- Docker 镜像内置 frpc v0.66.x（amd64 / arm64）
- DDNS 页：FRP 启用时提示将域名解析到 VPS 公网 IP

## [v0.1.25] - 2026-09-15

### 新增

- DDNS 与证书 DNS-01：支持腾讯云 DNS（SecretId + SecretKey，区别于 DNSPod ID+Token）

## [v0.1.24] - 2026-09-14

### 修复

- 中国 IP 段更新后 nginx 重载失败：校验用临时文件，应用配置时改用正式的 `china_cidr.conf`
- 仪表盘「今日请求」图表：从完整 access.log 按小时统计，不再仅用最近 10 条访问记录

## [v0.1.23] - 2026-09-14

### 修复

- 自定义错误页插图无法显示：图片路径改为公开 location，浏览器可正常加载 `error.png` / `429.png`

## [v0.1.22] - 2026-09-14

### 改进

- 仅中国大陆：内网 IP（10/8、172.16/12、192.168/16 等）默认放行，无需手动加白名单
- 自定义错误页图片内嵌进二进制，Docker 镜像补充 `web/assets/image` 资源目录

### 修复

- 错误页目录权限：生成后自动 chmod/chown，避免 nginx（www-data）无法读取导致回退默认 403 页
- 中国 IP 段未就绪时，仅中国大陆规则改为只拦截非公网内网地址

## [v0.1.21] - 2026-09-14

### 新增

- 反代安全设置：IP 黑/白名单、仅中国大陆、Basic Auth、速率/连接数限制
- 设置页：信任代理（CDN Real IP）、全局 IP 黑名单、中国 IP 段定时更新
- 自定义 Nginx 错误页（403 / 404 / 429 / 500 / 503）
- 高级选项：上游 TLS 校验跳过、目标 Host 头、TLS 1.3 仅、HTTPS 安全响应头

### 改进

- 中国 IP 段：同步更新 API、ghfast 镜像回退、多源下载与状态展示
- 反代编辑/详情弹窗 UI 重构；列表域名访问链接与复制
- 仅中国大陆 + 白名单：白名单 IP 正确豁免大陆限制（geo + 变量组合）

### 修复

- 白名单模式开启但未填 IP 时的校验
- API 测试依赖 settings 包导入

## [v0.1.20] - 2026-09-13

### 改进

- 反代详情改为弹窗展示，概览布局重排（信息条 + 图表/指标 + 最近访问）
- 标题栏显示当前连接数；最近访问列表支持纵向滚动
- 反代详情日志结构化展示（时间、方法、路径、状态码等分列）

### 文档

- README 默认反代端口更新为 `18080` / `9443`

## [v0.1.19] - 2026-09-13

### 修复

- 反代详情日志：修复无法滚动的问题，悬停显示滚动条，默认跟随最新日志

## [v0.1.18] - 2026-09-13

### 改进

- 反代详情：概览 / 日志 Tab 切换，默认概览；日志区域撑满侧栏
- 反代列表：状态列改为开关，可直接启用/停用
- 反代「复制」改为填入新建表单（名称加 `-复制` 后缀）
- README Docker 徽章按 semver 排序，避免显示 `-amd64` 标签

### 移除

- 反代详情「设置」Tab 与拖动排序提示文案

## [v0.1.17] - 2026-09-13

### 新增

- 反代规则「备注」改为「名称」，列表以名称优先展示
- 反代规则复制当前信息到剪贴板
- 反代列表拖动排序（无筛选时）

## [v0.1.16] - 2026-09-13

### 改进

- 证书申请：首次未配置 ACME 邮箱时提示填写，前后端校验邮箱格式
- 证书列表空状态与反代/DDNS 页风格统一

## [v0.1.15] - 2026-09-13

### 修复

- Docker 发版：amd64/arm64 均构建完成后再推送 `latest` 与版本多架构 manifest
- 补全 `web/assets/image` 品牌图片资源，同步仪表盘 banner

### 改进

- arm64 镜像构建：前端与 Go 在 BUILDPLATFORM 编译，缩短 QEMU 构建时间

## [v0.1.14] - 2026-09-13

### 新增

- ZeroSSL 证书申请支持
- 反代规则备注字段；域名+端口唯一约束
- 可配置默认反代监听端口（`FONU_NGINX_HTTP_PORT` / `FONU_NGINX_HTTPS_PORT`），适配飞牛 host 部署
- `docker-compose.host.yml` 飞牛 host 模式部署示例
- 全新 Web UI：品牌视觉、仪表盘、证书/DDNS/反代/设置页重构

### 改进

- README 重写：飞牛 fnOS 部署说明、界面截图、Docker Hub 徽章
- `/api/version` 返回默认反代端口配置
- 移除仓库内 `scripts/`、`cmd/seed` 本地开发辅助

## [v0.1.13] - 2026-09-13

### 修复

- 修复证书页 TypeScript 类型错误导致 Docker 构建失败

## [v0.1.12] - 2026-09-12

### 新增

- DDNS：单任务支持多根域名，列表按域名展示解析 IP 与同步状态
- 证书：申请进度弹窗与实时日志（SSE 异步任务）；列表支持下载证书/私钥/ZIP
- 反代：按规则实时访问日志、流量统计（累计/速率）与当前连接 IP 查看
- 日志：访问/错误/运行日志分页；实时日志预载最近 100 条 Nginx 记录

### 修复

- 实时日志 SSE 因中间件未转发 Flush 导致始终为空
- Vite 开发代理对 SSE 流的缓冲问题
- 证书申请：同一 DDNS 凭证管理多个根域（如 roven.cc + chiak.cc）时校验失败
- 首次启动自动创建管理员并在运行日志输出初始密码（移除独立初始化页）
- 证书申请失败写入 `last_error` 并在列表展示
- 系统日志过滤无意义 API 轮询噪音

### 改进

- 仪表盘/证书/DDNS 列表加载优化（轻量接口与缓存刷新）
- Nginx 访问日志增加 `request_length` / `bytes_sent` 供流量统计
- 反代与证书页操作按钮横排布局

## [v0.1.11] - 2026-09-12

### 修复

- Vite 开发服务器允许经 Nginx 反代用自定义域名访问（`server.allowedHosts`）

## [v0.1.10] - 2026-09-12

### 修复

- Windows 本地 Nginx：证书/日志使用绝对路径，HTTPS 块先于 HTTP 重定向，修复 `ssl_certificate` 校验失败
- Windows 进程检测与启动：修复 `isPIDAlive` 误判、启动阻塞及重复拉起 Nginx 导致端口冲突
- 系统日志写入 `app.log`，API 请求记录状态码；实时日志连接时预载最近条目
- 未设置 `FONU_DATA_DIR` 时默认使用当前目录 `.data`

### 改进

- 新增 `scripts/dev.ps1`、`scripts/clean-data.ps1` 便于 Windows 本地开发
- 测试与文档示例域名统一为 `example.com`，移除个人域名硬编码

## [v0.1.9] - 2026-09-12

### 修复

- 反代保存/删除不再因 Nginx 重载卡住 30 秒：优先 SIGHUP 信号重载，失败时快速强制重启
- 保存/更新规则先落库再重载 Nginx，避免超时后规则未写入
- 部分成功（规则已保存但 Nginx 失败）时前端仍关闭弹窗并刷新列表

## [v0.1.8] - 2026-09-12

### 修复

- 修复反代规则删除无反应：删除失败时显示错误提示；Nginx 重载失败时不再回滚已删除的规则

## [v0.1.7] - 2026-09-12

### 改进

- 界面左下角版本号随发版标签注入，与 Docker 镜像版本一致（不再固定显示 v0.1.0）
- 新增 `GET /api/version` 接口；发版时更新根目录 `VERSION` 文件

## [v0.1.6] - 2026-09-12

### 修复

- 为 Nginx 校验/重载/启动命令增加 15 秒超时，避免反代保存时无限转圈
- 前端 API 请求增加 30 秒超时，超时后显示明确错误提示
- Nginx 重载失败时先尝试优雅停止再重启，降低端口占用导致的卡死

## [v0.1.5] - 2026-09-12

### 修复

- 反代证书匹配改为读取证书库记录与磁盘文件，导入证书可正确用于 Nginx HTTPS
- 启用 HTTPS 但域名未匹配证书时，返回明确错误（含当前证书覆盖范围）而非 Nginx 校验失败
- 修复 HTTPS 配置生成时可能遗漏 HTTP 回退的问题

## [v0.1.4] - 2026-09-12

### 修复

- 修复启用 HTTPS 但证书文件不可用时，Nginx 生成 `listen ... ssl` 却缺少 `ssl_certificate` 导致反代保存失败
- 仅在证书文件完整存在时生成 HTTPS 配置，否则自动降级为 HTTP

## [v0.1.3] - 2026-09-12

### 新增

- **反代多域名**：一条规则支持多个前端域名、自定义监听端口，以及 IPv4/IPv6 监听开关
- **单域名独立端口**：前端地址可写 `example.com:6893` 覆盖规则默认端口

### 改进

- 本地开发未安装 Nginx 时，保存反代规则仍写入配置并跳过校验，便于调试 UI

### 数据库迁移

- `006_proxy_multi_host.sql` — 反代规则多域名与监听配置

## [v0.1.2] - 2026-09-12

### 修复

- 修复 Nginx PID 文件为空或进程已退出时仍尝试重载，导致反代规则保存失败

## [v0.1.1] - 2026-09-12

### 修复

- 修复全新部署时 API 返回 `null` 导致仪表盘/日志等页面空白、`map` 报错
- 修复 Nginx 重载使用错误 PID 路径（`/run/nginx.pid`）导致反代保存失败
- 公网 IP 检测增加多个备用服务，降低 Docker 环境下单点失败概率
- 修复 SPA 路由刷新时未正确回退到 `index.html`

## [v0.1.0] - 2026-09-12

### 新增

- **DDNS 多域名**：支持多条 DDNS 配置，独立管理根域名与记录
- **证书多域名**：申请/续签支持多域名与通配符（`*.example.com`）
- **ACME 颁发机构**：申请与续签时可选择 Let's Encrypt 正式/测试环境
- **证书导入**：支持粘贴 PEM 或指定文件路径导入自签/已有证书
- **Docker 发版**：推送 `v*` 标签自动分架构推送镜像（`latest` / `0.1.0` / `0.1.0-amd64` / `0.1.0-arm64`）
- **表格操作规范**：所有列表操作按钮平铺展示（`renderTableRowActions`）

### 改进

- **仪表盘**：全新布局，ECharts 图表，状态卡片与日志预览
- **日志页**：分页（20 条/页），支持 `?tab=` 跳转，统一时间格式
- **证书页**：概览统计、申请弹窗优化、表格自适应宽度
- **布局**：顶栏时间靠左、操作靠右，内容区铺满主区域
- **主题**：深色模式与组件样式统一

### 数据库迁移

- `003_ddns_multi.sql` — DDNS 多配置
- `004_cert_acme_ca.sql` — 证书 ACME CA 字段
- `005_cert_domains.sql` — 证书多域名
