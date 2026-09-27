# Fonu 品牌升级 / 改名方案

> 日期：2026-09-25　对象：Fonu 1.0.2（VERSION 1.2.12）
> 方法：把 "Fonu / fonu / FONU_" 在仓库里**实际出现的位置逐个查出来**（下文带 `文件:行` 的都是读代码得到的），再按「改动代价 × 破坏性」分层给方案。
> 结论速览：**能做，而且没有"改不动"的地方；但"全面改名"绝不等价于全局替换——有 3 处一动就会伤到存量用户（数据库文件名、环境变量前缀、远端 agent 凭据路径）。推荐策略 B：分层渐进 + 兼容层，一个版本后清理兼容代码。**

---

## 〇、一句话回答

| 问题 | 回答 |
| --- | --- |
| 能不能换名？ | 能。所有品牌落点都是可枚举的文本/路径/常量，没有第三方注册死锁（GitHub 仓库名可改、镜像名可切、CLI 名随二进制）。 |
| 能不能"全面"？ | 能，但要分 5 层做。**展示层（用户看见的字）改起来是半天的事，存储层与契约层（数据文件名、环境变量、远端 agent 路径）必须带兼容层**，否则老用户升级后会出现「数据像丢了」「容器起来了但库是空的」「VPS 上的 agent 失联」。 |
| 改名量级 | 实测 `Fonu|fonu|FONU` 至少 1912 处、120+ 个文件（分页上限，实际更多）；其中约 2/3 是 CSS 变量（`--fonu-*`）、Go import 前缀、文档正文这类**机械替换**，真正的决策点只有 5 个（见 §二）。 |

---

## 一、品牌落点盘点（按层，带实测证据）

### 第 1 层：展示层（用户能看见的字与图）

| 落点 | 位置 | 说明 |
| --- | --- | --- |
| 浏览器标题 | `web/index.html` `<title>Fonu</title>` | 建议与侧栏名统一 |
| 侧栏品牌区 | `web/src/layouts/AppLayout.vue:15,17,34` | logo `alt="Fonu"`、`brand__name` 文字、页脚品牌名；`:132` 引 `../assets/brand/logo.png` |
| 品牌标语 | `AppLayout.vue:18` 「让 NAS 访问更简单」 | 换名时建议一并重写（新名 + 新一句话定位） |
| 视觉资产 | `web/src/assets/brand/logo.png`、`web/public/favicon.png`、`web/assets/image/logo.png`、`web/assets/image/banner.png` | 图片需重做；`web/assets/image/*` 由 Dockerfile `COPY web/assets/image /app/web/assets/image` 进镜像 |
| 登录页 | `web/src/views/LoginView.vue` | 品牌露出最集中的页面之一 |
| 公共组件 | `web/src/components/{PageHeader,EmptyState,LoadError,StatusBadge,StatCard,FonuCard}.vue` | 前 5 个是文案/样式里的品牌残留，`FonuCard.vue` **文件名本身含品牌名**（含 import 该组件的多处 view） |
| 前端常量 | `web/src/constants/app.ts:1` `GITHUB_REPO = 'O96u/Fonu'` | 对外展示的仓库地址，改名必须同步（见 §六） |
| 通知消息 | `internal/notify/format.go`、`internal/notify/{service,store,push}.go` | 邮件/推送正文里的品牌前缀，用户直接在手机上看到 |
| 公开状态页 | `internal/api/status_page_handler.go`、`status_handler.go`（各 4~8 处） | `/status` 公开页标题、页脚品牌 |
| CSS 变量 | `web/src/styles/variables.css:2` 起 `--fonu-brand`、`--fonu-brand-soft`…（全仓 51 处） | **纯机械替换**，但波及所有 `.vue` 样式；建议一次性做且用 `replace_all` |
| 本地偏好 key | `web/src/composables/useTheme.ts:4` `themeKey = 'fonu-theme'` | 改名后老用户主题偏好重置（可一行兼容，见 §五.4） |

### 第 2 层：交付 / 部署层（用户拿到的东西叫什么）

| 落点 | 位置 | 风险 |
| --- | --- | --- |
| 主程序二进制名 | `Dockerfile`：构建产物 `/fonu` → `COPY --from=go-builder /fonu /app/fonu`、`CMD ["/app/fonu"]`；发布物 `fonu-1.1.0-linux-amd64` | 中：换名即换文件名，用户脚本里的路径要跟着改 |
| agent 交叉编译名 | `Dockerfile`：`go build -o internal/frp/agentbin/fonu-agent-linux-{amd64,arm64}` | **高**：与 `internal/frp/agentbin/embed.go` 的 `//go:embed fonu-agent-linux-amd64` 强耦合，改一处必须同批改另一处，否则编译期直接失败 |
| Docker 编排 | `docker-compose.yml`、`.host.yml`、`.hub.yml`：service `fonu`、`container_name: fonu`、`image: fonu:local`、volume `fonu-data` | **高**：volume 名一变，`docker compose up` 会建**新空 volume**，用户会认为「数据丢了」；必须写清迁移命令或显式 pin `name:` |
| 镜像仓库名 | `.github/workflows/docker-release.yml:63,94` `${DOCKERHUB_USERNAME}/fonu:<ver>-<arch>` | Docker Hub **不支持仓库改名**（只能新建），见 §六 |
| 前端 embed 路径 | 根 `.gitignore` 的 `cmd/fonu/web/dist/`、`.github/workflows/ci.yml:19,42,43`、`//go:embed all:web/dist` 所在 `cmd/fonu/main.go` | 中：目录改名会牵动 CI、构建脚本、embed 指令、Dockerfile |
| agent 安装布局 | `scripts/agent-install.sh`：`PREFIX=/usr/local/bin/fonu-agent`、`CONF_DIR=/etc/fonu-agent`、`UNIT=/etc/systemd/system/fonu-agent.service`、`/var/lib/fonu-agent/certs` | **高**：涉及已在用户 VPS 上运行的服务，升级路径必须兼容（见 §五.5） |
| 环境变量 | `internal/config/config.go:24,29,31~38`（`FONU_DATA_DIR`/`FONU_LISTEN`/`FONU_SESSION_SECRET`/`FONU_NGINX_*`/`FONU_FRPC_BIN`/`FONU_FRP_PID`）+ `Dockerfile` 的 `ENV` 块 + `docker-compose*.yml` + `docs/DEPLOYMENT.md`（大量） | **高**：前缀一改，老用户的 `.env` / compose / `-e` 全部静默失效 → 容器用默认值起来（例如数据目录跑回 `/data` 之外、会话密钥回到 `change-me-in-production`） |

### 第 3 层：代码标识层（改了不影响运行语义，但波及面最大）

| 落点 | 位置 | 说明 |
| --- | --- | --- |
| Go module path | `go.mod:1` `module github.com/fonu/fonu` | **每个 .go 文件的 import 块**（`internal/api` 20 个文件、`internal/nginx`、`internal/frp`…）；同时 `Dockerfile` 的 `-ldflags "-X github.com/fonu/fonu/internal/version.Version=..."` 必须同步改，否则版本号注入静默失效（二进制显示 `dev`） |
| 前端包名 | `web/package.json:2` `"name": "fonu-web"` | 机械替换 |
| 内嵌 agent 资产名 | `internal/frp/agentbin/embed.go` | 见上，与 Dockerfile 成对出现 |
| 文档 | `README.md`、`CHANGELOG.md`、`docs/*.md`（20+ 篇，其中 `DEPLOYMENT.md` 56 处、`frps服务端管理方案.md` 52 处） | 量大但不危险；建议随代码同批改，避免文档与实现不一致 |

### 第 4 层：运行期契约层（跨进程/跨版本，最容易漏）

| 落点 | 位置 | 说明 |
| --- | --- | --- |
| 会话 Cookie 名 | `internal/api/middleware.go:18` `r.Cookie("fonu_session")` | 改名 = **所有已登录用户掉线**（可接受，但要在发布说明写明） |
| agent 凭据文件 | `internal/frp/agent_installer.go:35,36,168` + `:145`：服务端通过 SSH 在 VPS 上 `sed` 读 `/etc/fonu-agent/agent.env` 的 `FONU_AGENT_TOKEN` | **服务端与 agent 必须同批发布**：服务端换了路径/变量名，而用户 VPS 上还是旧 agent（或反之），「Agent 探测」会直接失败 |
| agent 自述标识 | `internal/agent/server.go:213,221` `{"ok":true,"service":"fonu-agent"}` | 属于对外可见的 API 响应体；若有任何前端/脚本按 `service` 字段判断，改名前要一起查 |
| agent 落盘目录 | `internal/agent/server.go:133,145`：`/var/lib/fonu-agent/certs`、`/var/lib/fonu-agent` | 与安装脚本一致 |
| 远端 systemd unit 描述 | `internal/agent/server.go:344` `Description=frps service (managed by fonu-agent)` | 远端残留可见文本 |
| 远端 nginx 探针/标记 | `internal/agent/server.go:674-675`：写 `fonu-probe.conf`、文件头 `# Managed by fonu-agent. Do not edit.`；`:692` 已有「清理历史残留 `fonu-*.conf`」逻辑 | **改名最容易踩的隐性坑**：老 VPS 上留着旧品牌标记的片段，新版本若只认新标记，会把它们判成「未知文件」→ 漂移检测误报或残留永不清理 |
| 远端 frp 安装与漂移 | `internal/frp/ssh_installer.go`、`internal/frp/route_drift.go`（各 4 / 2 处） | 同上：判定「这是我管理的文件」的依据里含品牌标记 |

### 第 5 层：数据层（最危险，改错就是"丢数据"）

| 落点 | 位置 | 说明 |
| --- | --- | --- |
| 数据库文件名 | `internal/config/config.go:49-51` `DBPath() = <DataDir>/fonu.db` | **一改，程序找不到库 → 建一个空库 → 用户以为数据没了**。必须带「探测旧文件 + 原子重命名」 |
| 数据目录默认值 | `config.go:126` `defaultDataDir()`：`/data` 或 `<cwd>/.data` | ✅ **好消息：不含品牌名，无需迁移** |
| 各子目录 | `config.go` 的 `NginxDir/LogsDir/CertsDir/FrpDir`…（`nginx`/`logs`/`certs`/`frp`/`ssh_known_hosts`） | ✅ 也都不含品牌名，只有 DB 文件名带品牌 |
| 迁移脚本 | `migrations/023_frp_server_agent.sql`（1 处品牌） | SQL 迁移一般不需要改（库文件名才是关键路径） |

> 盘点的意义：**品牌名"长"在 5 个不同性质的地方**——字、路径、常量、协议、文件名。前两类可以随便改，后三类改之前要先想好兼容。

---

## 二、三个必须先解决的暗礁

1. **数据库文件名 `fonu.db`（`internal/config/config.go:50`）**
   裸改 = 静默新建空库。做法：启动时若「新名库不存在」且「`fonu.db` 存在」→ 同目录 `os.Rename` 到新名（原子、可回滚），失败则**继续用旧路径并在日志/界面提示**，绝不新建第二份库。Windows 上文件被占用时 rename 会失败，所以「失败即回退到旧路径」这条兜底不是可选项。

2. **环境变量前缀 `FONU_*`（`internal/config/config.go:24,29,31-38`）**
   裸改 = 老部署的 `FONU_SESSION_SECRET` 失效 → 会话密钥回到公开默认值 `change-me-in-production`（**这是安全问题，不只是体验问题**：拿到默认密钥可伪造管理员会话、解密库里凭据，见 `docs/DEPLOYMENT.md` 对该密钥的说明）。
   做法：`envOr` 升级为「新前缀 → 旧前缀 → 默认值」三档读取，命中旧前缀时启动日志给一条明确的迁移提示（含新旧变量名对照），至少保留一个 stable 版本后再删旧前缀。**尤其 `FONU_SESSION_SECRET` 与 `FONU_DATA_DIR` 必须双读。**

3. **远端 agent 的凭据路径与品牌标记（`agent_installer.go:35,36,145,168`；`agent/server.go:674,692`）**
   服务端是**通过 SSH 去读用户 VPS 上的文件**来判断 agent 状态的。服务端与 agent 不同批发布时：
   - 探测读旧路径、agent 写新路径 → 「Agent 探测」永远显示未配置；
   - nginx 探针/片段标记改名 → 旧文件既不被清理也不被识别，漂移检测出现假阳性。
   做法：探测端**两个路径都试**（新路径优先，旧路径兜底）；`agent` 二进制**同时接受新旧 env 变量名**；生成器把新旧品牌标记**都视为"自己生成的文件"**（清理/漂移判定同时匹配），并在 agent 版本号里体现本次变更（本仓既有约定：agent 行为变更需递增 `AgentVersion` 并重编内嵌双架构二进制）。

---

## 三、三种策略与推荐

| | 策略 A：只换皮 | **策略 B：分层渐进 + 兼容层（推荐）** | 策略 C：一次性硬切 |
| --- | --- | --- | --- |
| 范围 | 只做第 1 层（文案/图/CSS 变量/通知） | 第 1→2→4→5→3 层按阶段推进，关键处留兼容 | 5 层全改，不留兼容 |
| 代码净度 | 低（`fonu.db`、`fonu-agent`、`FONU_*` 仍是旧名） | 中：过渡期有双读分支，1~2 个版本后清理 | 高 |
| 存量用户 | 无感 | 无感（或仅掉一次登录态） | **有损**：需手工迁移数据目录/重建 volume/重装 agent |
| 适合 | 只想换个门面、用户量很小 | 已有真实用户、有 Docker 与 VPS 两种部署形态（本仓现状） | 尚未发布 / 可强制清退老版本 |
| 代价 | 半天量级，零风险 | 机械替换 + 5 个兼容点 + 一轮双形态回归 | 改动最少但迁移成本转嫁给用户 |

**推荐 B**，理由：本仓已有 Docker（`docker-compose*.yml`）、host 直装、VPS agent 三种落地面，且 `docs/DEPLOYMENT.md` 里明确把 `FONU_SESSION_SECRET` 当作安全关键项——这些决定了「裸改」的代价远高于「留兼容层」。

---

## 四、推荐执行路线（五阶段）

> 每阶段的验证沿用本仓既有约定：`go build ./... && go vet ./... && go test ./...` + `cd web && npm run typecheck && npm run build`；涉及 agent 的改动递增 `AgentVersion` 并重编内嵌二进制；用户明确要求提交时，才同批更新 `CHANGELOG.md` 与 `VERSION`。

### 阶段 0：定名与冻结（不写代码）

- 定下新品牌名与英文小写标识（CLI/包名/目录用）、显示名（首字母大写形态）。
- 检查占用：域名、GitHub org/repo、Docker Hub 命名空间、唯一包名（如 npm/PyPI 若以后要用）、以及**中文语境下的商标检索**。
- 决定 GitHub 与 Docker Hub 的走向（§六），因为 `web/src/constants/app.ts:1` 与版本检查 `web/src/utils/version.ts:37` 直接依赖它。
- 产出：`旧名 → 新名` 对照表（含：显示名、小写标识、二进制名、镜像名、容器名、env 前缀、cookie 名、DB 文件名、agent 目录名）。

### 阶段 1：展示层（零风险，先拿到"看着像新品牌"的反馈）

- 改：`web/index.html` 标题、`AppLayout.vue:15,17,18,34`、`LoginView.vue`、公共组件（含 `FonuCard.vue` 重命名）、`web/src/constants/app.ts:1`、`internal/notify/format.go`、`internal/api/status_page_handler.go`/`status_handler.go` 的品牌文案。
- 换图：`web/src/assets/brand/logo.png`、`web/public/favicon.png`、`web/assets/image/{logo,banner}.png`。
- CSS 变量 `--fonu-*` → `--<new>-*`（`variables.css` 定义 + 全部 `.vue` 引用，用 `replace_all` 一次完成）。
- 验证：`npm run build` 通过 + 前端手点一遍侧栏/登录页/设置页/公开状态页。
- 回滚：纯文案与样式，`git revert` 即可。

### 阶段 2：交付层（二进制、镜像、compose、发布物）

- 二进制 `fonu` → 新名；`docker-compose*.yml` 的 service/容器名/image/volume；`.github/workflows/docker-release.yml:63,94` 的镜像路径。
- **volume 必须处理**：在 compose 里显式声明 `volumes: { <new>-data: { name: <new>-data } }` 并在 `docs/DEPLOYMENT.md` 写「老用户迁移三行命令」（旧 volume 挂到新名，或 `docker volume create` + 复制）。
- 老二进制名建议保留一个版本（同镜像内留符号链接，或发布说明里给 rename 命令），避免用户脚本直接报错。
- 验证：本地 `docker compose up`，用**旧 volume** 起新容器，确认库被正确打开（这是本阶段的验收核心，不是"能起"）。
- 回滚：compose 文件与镜像名回退即可；volume 命名要事先确定，避免中途再改一次。

### 阶段 3：存储层与契约层（暗礁集中区，必须带兼容）

- `config.go`：`DBPath()` 换新名 + **旧库自动接管**（§二.1）；`envOr` 支持旧前缀兜底 + 启动迁移提示（§二.2）。
- `middleware.go:18` cookie 名：可选做「新名 cookie 优先、旧名兜底」读一次，或直接换名并在发布说明写「需重新登录」。
- `useTheme.ts:4`：读旧 key 回退 + 写回新 key（三行代码，避免用户主题偏好被重置）。
- agent 侧：`agent-install.sh`（`/usr/local/bin`、`/etc`、`/var/lib`、systemd unit 名）、`internal/agent/server.go:133,145,213,221,344,674,692`、`internal/frp/agent_installer.go:35,36,145,168`、`ssh_installer.go`、`route_drift.go`。
  - 安装脚本要能**识别并接管已安装的旧 agent**（停旧 unit、装新 unit、保留/迁移 `/etc/fonu-agent` 的 env 文件），而不是留下两套服务互相抢。
  - 探测与漂移判定：新旧路径/标记**都算自己人**，至少一个版本。
- 验证（关键）：造一份**旧版数据目录**，用新二进制启动 → 确认打开的是旧库且数据完整；再用**旧版 agent** 对**新版服务端**跑一次「Agent 探测 / 安装 / 日志」；`go test ./...` 通过（新增的 DB 接管与 env 双读建议各补一个纯函数单测，参照本仓 `maskFRPSSecrets` 那类测试写法）。
- 回滚：DB rename 是单向的（必要时提供反向重命名说明）；env 双读让回滚无感。

### 阶段 4：代码标识层（机械但波及全仓）

- `go.mod:1` module path → 新路径；全仓 import 替换（`gofmt`/`goimports` 后 `go build ./...` 兜底）。
- **同步点**：`Dockerfile` 的 `-ldflags "-X github.com/fonu/fonu/internal/version.Version=..."`（漏改则版本号变 `dev`，且不报错）；`internal/frp/agentbin/embed.go` 与 Dockerfile 的 agent 输出名；`web/package.json:2`；`cmd/fonu/` → `cmd/<new>/`（连带 `.gitignore`、`ci.yml:19,42,43`、embed 指令）。
- 建议与阶段 2/3 分开发布（单独一次提交），便于「编译失败」时快速定位。
- 验证：`go build ./... && go vet ./... && go test ./...`；构建后确认 `GET /api/version`（或设置页）显示真实版本号而非 `dev`。

### 阶段 5：清理（1~2 个版本后）

- 删除旧 env 前缀兜底、旧 DB 文件名探测、旧 cookie/localStorage key、旧 agent 路径与标记、旧预先/镜像别名。
- 用 §九 的自查清单跑一次全仓 grep，确认只剩新名。

---

## 五、兼容层设计要点（可照做）

1. **环境变量**：`envOr` → `envOrLayered(newKey, oldKey, default)`；命中旧 key 时启动日志输出一次「检测到旧变量 `FONU_DATA_DIR`，已生效；请迁移到 `<NEW>_DATA_DIR`」。优先级：新 > 旧 > 默认，**不要**用「旧覆盖新」。
2. **数据库接管**：判断顺序 = 新库存在? → 用新库；否则旧库存在? → rename 后使用（失败则直接用旧路径 + 明确提示）；否则建新库。全程不改动用户其它文件。
3. **Docker volume**：`name:` 显式固定新名；给出一条从旧 volume 复制的命令；**不要**靠 compose 自动改名。
4. **前端本地状态**：`localStorage.getItem('fonu-theme') ?? localStorage.getItem('<new>-theme')`，读到旧值时写回新 key。
5. **agent 平滑升级**：
   - agent 二进制双读新旧 env 名（`FONU_AGENT_TOKEN` 与 `<NEW>_AGENT_TOKEN`）；
   - 安装脚本 `systemctl disable --now fonu-agent` → 装新 unit → 迁移 `/etc/fonu-agent/agent.env` → 启动新 unit；
   - 服务端探测命令**两个路径并联**（`test -s A || test -s B`），避免版本错配期间误报；
   - 远端文件识别同时匹配新旧标记；`server.go:692` 的「历史残留清理」逻辑扩成「新旧品牌前缀都清」。
6. **凭据与密钥**：品牌改名**不应触碰**加密数据本身（`secret.Box` 加密的 DDNS/FRP/SMTP 凭据与品牌无关），所以只要 `FONU_SESSION_SECRET` 双读做好，老用户的凭据继续可用——这也是「不要顺手改密钥派生」的理由。

---

## 六、平台与外部身份

| 项 | 现状 | 建议做法 |
| --- | --- | --- |
| GitHub 仓库 | `web/src/constants/app.ts:1` `O96u/Fonu`；`web/src/utils/version.ts:37` 用 `api.github.com/repos/${GITHUB_REPO}/releases/latest` 做版本检查 | GitHub **支持仓库改名并保留旧地址重定向（含 API 与 release 资产下载）**，但重定向会被「别人用旧名新建仓库」永久打断；对自用/小众项目风险低，仍建议：改名后**保留旧仓库名不被占用**（或在旧名新建一个空仓库做说明页——注意这恰好会打断重定向，二者只能选一）。改名后必须回归一次版本检查（`version.ts` 是前端直连 GitHub API）。 |
| Docker Hub | `.github/workflows/docker-release.yml:63,94` `${DOCKERHUB_USERNAME}/fonu` | Docker Hub **不支持仓库改名**，只能新建仓库并重推标签；老用户 `docker pull` 旧名会继续拿到旧标签（不会自动指向新镜像，也不会报错）→ 发布说明里给出新镜像地址 + 迁移命令。 |
| 域名 / 文档站 | 仓库内无硬编码站点域名 | 若已注册 `fonu.*` 域名，改名后建议保留旧域名 301 到新域名（同 GitHub 重定向的思路）。 |
| 文档 | 20+ 篇 md（`DEPLOYMENT.md` 56 处、`frps服务端管理方案.md` 52 处…） | 与代码同批改；`docs/brand-rename-plan.md`（本文）建议保留，作为改名决策记录。 |
| 发布物命名 | 根目录 `fonu-1.1.0-linux-amd64`、`fonu-1.1.0-src.zip`、`fonu-image-build4.log` 等 | 历史产物不必改名；新产物按新名。 |

---

## 七、新名候选与自查

**命名约束（硬性）**：
- 小写标识：仅 `[a-z0-9-]`，2~8 字符，无连字符歧义（`<new>-agent`、`<new>.db`、`--<new>-brand`、`<NEW>_DATA_DIR` 都要好看）；
- 不与现有工具重名（`frp`、`nginx`、`caddy`、`tunnel` 等已占用词避免单独使用）；
- 域名 / GitHub org / Docker Hub / npm 包名四处可注册；
- 中文可读（本项目用户群中文语境，"念出来不别扭"很重要）；
- 商标自查（至少本国 + 主要市场）。

**候选清单**（`标识` 一列就是命令行、包名、`<标识>-agent`、`<标识>.db`、`<IDENT>_DATA_DIR` 要用的形态；每一项在落地前都要走 §七.3 的四处自查，下表「备注」只标注**已知**的同名存在，不代表未被占用）

#### A. 通路 / 门 / 桥（英文语义直白，技术用户第一眼就懂）

| 显示名 | 标识 | 中文名建议 | 寓意与适配 |
| --- | --- | --- | --- |
| Portway | `portway` | 通途 | port（端口）+ way（通路）——产品做的就是"把端口接出去"，语义与技术完全对齐；`portway-agent`、`portway.db` 读感都好 |
| Viaduct | `viaduct` | 虹桥 | 高架桥：在两个互不相通的网络之间架一条通路；画面感强，适合做几何型 logo |
| Causeway | `causeway` | 长堤 | 跨水而过的路——跨过 NAT/"内网之海"；8 字符是上限，仍可接受 |
| Underpass | `underpass` | 潜通 | 字面"从下面穿过去"，暗合 NAT 打洞的本意；比 Underpass 更贴的同类还有 Overpass，但后者已被 OSM 的查询 API 广泛占用 |
| Thruway | `thruway` | 直通 | thru 带"穿透"的俚语味，拼写短促；注意美式拼写，正式英文里少见 |
| Ponto | `ponto` | 蓬托 | 葡萄牙语/世界语"桥"，5 字符、短促好念；同名存在较多（银行、设计工具），必须自查 |

#### B. 渡 / 舟 / 引渡（中文意象最贴产品，拼音做标识）

| 显示名 | 标识 | 中文名 | 寓意与适配 |
| --- | --- | --- | --- |
| Dukou | `dukou` | 渡口 | **语义完全命中**：把内网的船渡到公网对岸；中文用户一看就懂、一说就记住；`dukou` 5 字符，命令行友好 |
| Zhandao | `zhandao` | 栈道 | 在无路可走的险处架出一条路——正是 NAT 之后的处境；名字冷门，检索不打架 |
| Yongdao | `yongdao` | 甬道 | 连接两端的地下通道；比"隧道"更雅，避开与 tunnel 这类通用词的碰撞 |
| Changdu | `changdu` | 长渡 | "长期摆渡"的意味，适合强调常驻服务；缺点是"长渡"作为词不常见 |
| Yinqiao | `yinqiao` | 引桥 | 从匝道引上主桥的那一段——很像"从内网引到出口"；亲切、口语化 |

> 说明：中文意象类在中文用户群里记忆点最强，代价是国际展示偏窄；如果以后要面向海外，建议「英文标识 + 中文名」成对使用（例如 `dukou` / 渡口），而不是只取其一。

#### C. 延续 Fonu 语音（保留 Fo 音，让改名读起来像"升级"而不是"换了个软件"）

| 显示名 | 标识 | 说明 |
| --- | --- | --- |
| Fonara | `fonara` | 保留 `Fo` 音 + 自造后缀，老用户不会觉得陌生；自造词在域名/商标上的可得性通常好于现成词 |
| Fonix | `fonix` | 与 Phoenix（凤凰）谐音，正好呼应"品牌涅槃升级"；注意存在 Fönix 等同名公司，需自查 |
| Fonia | `fonia` | 希腊语系词根常与"声音/传输"相关（telefonia），5 字符干净；要确认不与电信类品牌混淆 |
| Fonli | `fonli` | 最短的延续方案，`fonli-agent` / `FONLI_DATA_DIR` 都短；缺点是语义联想弱，需要靠视觉与标语补 |

#### D. 自造短词（域名与商标可得性最好，语义靠后续塑造）

| 显示名 | 标识 | 中文名建议 | 备注 |
| --- | --- | --- | --- |
| Nuvio | `nuvio` | 努维 | 发音圆润、无强语义包袱，适合做纯品牌名 |
| Tando | `tando` | 坦途 | 与"坦途"天然谐音——自造词的语义可由中文名反向赋义，这个配对意外地贴 |
| Kaimu | `kaimu` | 开目 | 意为"把门打开"，发音柔和 |
| Runa | `runa` | 鲁纳 | 短、好拼、国际化；注意与 Runa 相关的既有品牌 |
| Sairo | `sairo` | 赛罗 | 读音有速度感，适合强调链路性能 |
| Airlock | `airlock` | 气闸 | 不算自造词，但画面感极强：内网是舱内、公网是舱外，穿透就是"过气闸"；独特、好画 logo，且不与 frp/nginx/tunnel 语义重叠 |

#### D 组补充：再给 10 个自造短词

**D-1 音节谐音型** —— 读音贴合一个中文词，中文用户不需要解释就记住，中文名与标识互相印证：

| 显示名 | 标识 | 中文名 | 说明 |
| --- | --- | --- | --- |
| Kaitu | `kaitu` | 开通 | 读音近似「开通」，含义直接落在"把内网服务开通到公网"；5 字符，`kaitu-agent`、`KAITU_DATA_DIR` 都短 |
| Kailu | `kailu` | 开路 | 与「开路」完全同音——在 NAT 后面开出一条路，痛点隐喻最强；代价是内蒙古开鲁县同名，检索会混入地名 |
| Toulu | `toulu` | 透路 | 「透」（穿透）+「路」（通路），两个核心动作各取一字；`toulu` 生僻，检索最干净 |
| Chuantu | `chuantu` | 穿途 | 「穿」+「途」，气质偏文学；8 字符正好卡在硬上限，`chuantu-agent` 偏长，适合只在品牌层使用、标识再截短 |

**D-2 纯自造型** —— 无既有语义包袱，靠 logo 与标语赋义，域名与商标的可得性通常更好：

| 显示名 | 标识 | 中文名建议 | 说明 |
| --- | --- | --- | --- |
| Kuvo | `kuvo` | 库沃 | 4 字符、K/V 音干脆，读起来就像产品名；注意先锋 DJ 有同名社区平台 KUVO，必须自查 |
| Kelo | `kelo` | 松洛 | 芬兰语 kelo 指北极圈的站桩枯松（在恶劣环境里站住的意思），冷僻、科技圈少见 |
| Veko | `veko` | 维克 | 纯自造，音形短促，适合做"快"的联想 |
| Yomo | `yomo` | 悠陌 | 发音柔和，中文名可往"悠然可达"方向塑造 |
| Tovo | `tovo` | 途沃 | 首音节与「途」谐形，自带一点通路感，便于往中文语义靠 |
| Kivo | `kivo` | 启沃 | 首音节与「启」（开启）谐形，在图标上可以做"开门"的暗示 |

> 两条路线的取舍：**D-1 在中文本地传播最省力**（不用解释就懂），**D-2 在域名与商标上更干净**（生僻意味着少撞）。用户群以中文 NAS 玩家为主时 D-1 沟通成本最低；若之后要同时面向海外，D-2 + 一个中文名是更稳的组合。

#### E. 「本地 / 自家」语义（名字自带"服务跑在本地"的意思）

**E-1 拉丁语与概念型**（"就地、内部、家宅"的直译，气质克制，适合技术向调性）

| 显示名 | 标识 | 中文名建议 | 说明 |
| --- | --- | --- | --- |
| Insitu | `insitu` | 就地 | 拉丁语 in situ = "在原位、就地"——服务不往外搬，就在它原本那台机器上运行。6 字符，冷僻，检索最干净 |
| Intra | `intra` | 内域 | 拉丁语"内部"，直接指向内网/局域网；注意 Google Jigsaw 有同名的反审查 DNS 应用，需自查 |
| Domus | `domus` | 宅邸 | 拉丁语"家宅"，数据与服务都收在自己的房子里；Domus 是欧美建筑媒体的常用名 |
| Localis | `localis` | 本地 | local 的拉丁语源，语义最直白；缺点是偏长（7 字符），且与英国一家同名智库撞车 |

**E-2 跨语言的"家"**（一个字就说清"自己的地方"，且大多冷僻）

| 显示名 | 标识 | 中文名建议 | 说明 |
| --- | --- | --- | --- |
| Kodu | `kodu` | 科杜 | 爱沙尼亚语"家"；4 字符、极冷僻，念起来干净利落 |
| Sumika | `sumika` | 住屋 | 日语「住処」（すみか）= 栖息之所；6 字符，画面温和，科技圈少见 |
| Jitaku | `jitaku` | 自宅 | 日语「自宅」，含义精确到"自己的房子"，与 self-hosted 一一对应 |
| Uchi | `uchi` | 屋内 | 日语「内 / うち」同时是"内部"和"家"；缺点是与美国连锁日料品牌 Uchi 同名 |
| Hearth | `hearth` | 炉边 | 英语"炉边"，是"家"最温暖的意象；缺点是撞《炉石传说》的简称 Hearth |

**E-3 中文拼音直觉型**（中文用户零解释成本，代价是通用词难注册）

| 显示名 | 标识 | 中文名 | 说明 |
| --- | --- | --- | --- |
| Benji | `benji` | 本机 | 拼音「本机」完全对应，语义最准；缺点是与英文人名 Benji 撞车，检索会被淹没 |
| Bendi | `bendi` | 本地 | 最直白的一个；正因为通用，作为商品名与商标的注册难度最大 |
| Jiudi | `jiudi` | 就地 | 拼音「就地」，是 E-1 里 `insitu` 的中文同义形态，两者可以互为备选 |
| Wojia | `wojia` | 我家 | 亲切、有归属感，一句"服务就在我家跑着"把定位说完；注意与房产类品牌谐音 |
| Yuannei | `yuannei` | 院内 | "自家院子里那台机器"——NAS 用户共情很强；7 字符偏长 |
| Anwo | `anwo` | 安窝 | "给服务安个窝"，读音短、意象活泼，适合偏年轻的产品调性 |

> 两点提醒：
> 1. **`homelab` / `self-hosted` / `on-prem` 这几个词恰好描述你的产品，但它们是行业通用术语，不能当商品名**——建议放进标语（例如「为 homelab 而生」），品牌名另取。
> 2. "本地"语义与定位最贴，也最容易撞现成词：`bendi`、`benji`、`localis` 这类直白形态被占的概率，明显高于 `insitu`、`kodu`、`sumika` 这类冷僻形态。走这条语义线时，优先在冷僻形态里挑。

**这条语义线上的首选三个**

1. **`insitu` / 就地** —— 语义精确（就地运行、不外迁）、拉丁出处让名字有质感、且检索干净，是"本地服务"这层意思里最不像拼凑的一个。
2. **`jitaku` / 自宅** —— self-hosted 的日文对应词，含义零歧义，短而冷门，写进 `<标识>-agent`、`<标识>.db` 也好看。
3. **`kodu` / 家** —— 4 字符、跨语言都能念，留白最多，便于日后把它讲成"你自己的服务空间"。

#### F. 「NAS 存储 + 穿透」双关（一个名字同时指向"存着"和"能到"）

**F-1 英文双关型**（同一个词里同时有"盘/存储"和"通路"）

| 显示名 | 标识 | 中文名建议 | 说明 |
| --- | --- | --- | --- |
| Driveway | `driveway` | 盘路 | drive（盘/硬盘）+ way（通路）——字面是"车库前的车道"，放在存储语境里刚好是"盘往外走的路"，双关最巧；缺点是常见英文词，域名与商标被占概率高，必须先查 |
| Baylink | `baylink` | 盘链 | bay（NAS 的"盘位"）+ link（连接）；7 字符、自造度高，检索干净 |
| Bayway | `bayway` | 盘途 | bay + way，5 字符，短而奇；英文里有同名小地名，软件领域少见 |
| Portage | `portage` | 搬运 | 指在两条不通的水道之间把船与货搬过去，与"把 NAS 里的东西搬到公网"完全同构；**但 Gentoo 的包管理器就叫 Portage，技术圈认知已被占满，不建议** |

**F-2 存储器物型**（"存"义明确，通路那层交给标语）

| 显示名 | 标识 | 中文名建议 | 说明 |
| --- | --- | --- | --- |
| Tansu | `tansu` | 箪笥 | 日语「箪笥」＝分层抽屉的日式储物柜；冷僻、有质感，画面也很适合做图标 |
| Coffer | `coffer` | 藏柜 | 英语"保险箱"，5 字符，画面是"锁着的箱子"；冷僻、少撞 |
| Attic | `attic` | 阁楼 | "东西放阁楼，要用时上去取"——正是家庭 NAS 的真实使用方式，亲切易懂 |
| Larder | `larder` | 储藏室 | 家里存食物的小间，冷僻且带生活气 |
| Granary | `granary` | 粮仓 | 囤粮之所，画面感最强；7 字符偏长 |
| Hako | `hako` | 箱 | 日语「箱」，4 字符、极简，适合走冷淡克制的产品调性 |

**F-3 中文拼音双关型**（仓／库 + 门／路／口）

| 显示名 | 标识 | 中文名 | 说明 |
| --- | --- | --- | --- |
| Canglu | `canglu` | 仓路 | cāng lù 完全对应：「仓」是 NAS 里存的东西、「路」是穿透出去的通路——一个名字两层意思，5 字符，`canglu-agent`、`canglu.db` 都顺 |
| Cangmen | `cangmen` | 仓门 | 「把自家仓门打开」，直指"在外面访问家里的东西"这个诉求；6 字符 |
| Chuku | `chuku` | 出库 | 「出库」＝把仓库里的东西送出去，动作感最强，中文用户零解释成本；4 字符 |
| Kumen | `kumen` | 库门 | 与 cangmen 同义，更短、更有古意 |
| Cangdao | `cangdao` | 仓道 | 「仓中之道」，把存储与通路并列写出来；6 字符 |
| Kufang | `kufang` | 库房 | 最直白好懂；正因为通用，商标注册难度最大 |

**F-4 直白组合型**（NAS + 词：`nasway` / `nasport` / `nasbay`）

含义最准确、最不用解释，但都含通用缩写 `NAS`，商标几乎不可能注册，看起来也像内部工具名——**建议只用作副标题或仓库简介（例如「nasway — 为 NAS 而生的穿透面板」），不要当品牌名**。

#### 这条线上的首选三个

1. **`driveway` / 盘路** —— 双关最巧：drive 是硬盘、way 是通路，英文用户会心一笑，中文名「盘路」也说得通；代价是常见词，必须先核域名与商标。
2. **`canglu` / 仓路** —— 中文双关，拼音与标识完全对应，`canglu-agent`、`canglu.db`、`CANGLU_DATA_DIR` 都好念，且不与前几组任何候选重叠。
3. **`tansu` / 箪笥** —— 想要"存储器物"的气质又要冷僻好检索时的选择；分层抽屉的柜子本身就是个不错的图标母题。

> ⚠️ 一个诚实的提醒：让**一个名字同时说清"NAS 里存着"和"外面能到"**，很容易变成拼装词（nasway、cangdao 这类就在这个边缘）。如果只能保住一层意思，建议保"通路"，把"NAS／存储"交给标语去说——例如「让家里的 NAS 走出内网」，名字本身留出生长空间。

#### 建议优先考虑的组合（按这个项目的实际处境排序）

1. **`portway` / 通途** —— 语义、标识、双语读感三项都最均衡；技术与中文用户都不需要解释。
2. **`dukou` / 渡口** —— 中文语境里最容易被记住的一个，产品定位讲一遍就懂；国际化时用英文标识兜住。
3. **`zhandao` / 栈道** —— 冷门、好检索、意象有情绪（"在无路处开出一条路"），适合做社区故事。
4. **`fonara`** —— 想保住 Fo 音、把这次改动讲成"品牌升级"而非"另起炉灶"时的首选。
5. **`airlock` / 气闸** —— 想要强识别度的视觉符号（圆环阀门 + 状态灯，天然适合做状态图标）时的首选。

#### 已知撞名、不建议使用的名字

| 名字 | 为什么别用 |
| --- | --- |
| Pangolin / 穿山甲 | 是**同类开源项目本身**（`fosrl/pangolin`，见 `docs/feature-benchmark-2026-09-24.md` 的同类项目盘点），会被直接误认成它 |
| Harbor | CNCF 的镜像仓库项目，技术圈认知已被占满 |
| Warp / Warpgate / Skyway / Overpass / Lumen | 分别撞 Cloudflare WARP 与 Warp.dev、知名开源堡垒机、NTT 的 P2P 服务、OSM 查询 API、电信运营商与多个产品线 |
| Nexo / Zeta / Orbi | 本文初稿曾把它们当作"简洁短词"举例，实际分别撞加密借贷平台、多家同名公司、Netgear 的 Mesh 产品线；写进 `web/src/constants/app.ts` 之前先排除 |
| Portalis / Wayfare | 游戏/出行类同名产品较多，检索会被淹没 |

> ⚠️ 现名 `Fonu` 与罗曼语系词 `fono`（电话）高度接近，换名正好可以顺手绕开这类"念起来像另一个词"的问题——建议把「读起来/拼起来是否与既有词重合」列入上面的硬性约束。

**改名后自查清单（供阶段 5 使用）**：

```
# 全仓（排除 node_modules / dist / .data）
rg -in "fonu" --glob '!web/node_modules/**' --glob '!**/dist/**' --glob '!.data/**'
# 分类核查
rg -n "FONU_"                                   # 环境变量：是否只剩新前缀
rg -n "fonu_session|fonu-theme"                 # 契约/本地状态
rg -n "fonu\\.db|/fonu-agent|fonu-probe"        # 存储与远端残留
rg -n "O96u/Fonu|/fonu:"                        # 外部身份与镜像
rg -n "github.com/fonu/fonu"                    # module path（含 Dockerfile ldflags）
```

**正式定名前的四处占用自查（逐个到站内查，别指望搜索引擎）**

| 查什么 | 怎么查 | 判定标准 |
| --- | --- | --- |
| GitHub 组织 / 仓库 | `https://github.com/search?q=<标识>&type=repositories` 与组织页直连 | 组织名被占 → 无法建同名 org；同名仓库若是活跃的同类工具 → 直接换 |
| Docker Hub | 站内搜 `<标识>`，并确认 `<你的用户名>/<标识>` 可创建 | 镜像名一旦发布，改名只能新建仓库（见 §六） |
| 域名 | `.com` / `.net` / `.io` / `.cn` / `.dev` 的 whois | 核心后缀被竞品或商标方持有 → 减分；仅冷门后缀被占可接受 |
| 商标 | 中国商标网 + 目标市场（EUIPO / USPTO），查第 9 类（软件）、第 42 类（技术服务） | 存在同类同名的在先注册 → 直接排除，不改 |

> 本次实测：泛化的网页检索对"这个名字有没有被占用"几乎无效（只会返回无关内容），所以这四处必须站内查。另外把派生形态也念一遍——`<标识>-agent`、`<标识>.db`、`--<标识>-brand`、`<IDENT>_DATA_DIR`——它们是每天都要看见的东西。

---

## 八、风险与回滚

| 风险 | 触发条件 | 缓解 |
| --- | --- | --- |
| 老用户"数据丢失" | 只改 `DBPath()` 不做接管 | 阶段 3 的 rename + 失败回退；发布说明写明「首次启动会接管旧库」 |
| 会话密钥静默回落默认值 | 只改 env 前缀 | `FONU_SESSION_SECRET` 双读 + 启动告警；保留一个版本的旧前缀 |
| VPS agent 失联/漂移误报 | 服务端与 agent 版本错配 | 新旧路径/变量/标记双支持；`AgentVersion` 递增；安装脚本接管旧 unit |
| Docker 起来是空库 | volume 名变更 | compose 显式 `name:` + 迁移命令 + 用旧 volume 起新容器做验收 |
| 版本号显示 `dev` | 漏改 Dockerfile 的 `-ldflags` module path | 构建后必查版本接口；CI 加一条断言 |
| 客户端全部掉登录态 | cookie 改名 | 双读或接受一次重登（发布说明写明） |
| 前端版本检查失效 | GitHub 仓库改名后旧名被占/新常量未同步 | 改名后回归一次 `version.ts` 路径 |

**回滚策略**：阶段 1~3 的回滚以「保留旧名兼容分支」为主（一个 `git revert` 即可，因为兼容层是新增而非替换）；数据库重命名是不可逆动作，但方向单一（旧→新），回退版本时提供一条反向重命名说明即可。

---

## 九、结论

- **可以做全面品牌升级，落点是有限且已全部找清的**（见 §一，含 5 层证据）。
- **不要全局替换**：`internal/config/config.go:50`（`fonu.db`）、`config.go:24,29,31-38`（`FONU_*`）、`internal/frp/agent_installer.go:35,36`（VPS 上 `/etc/fonu-agent/agent.env`）这三处是"改了会伤用户"的位置，必须先设计兼容再动手。
- **建议按 §四 的五阶段推进**，阶段 1 就能拿到"看着焕然一新"的结果，阶段 3 才是真正需要谨慎的工程部分；阶段 5 用 §七 的清单收尾。
- 本方案只做分析，未改动任何代码。

---

> 说明：本文基于 2026-09-25 的仓库状态（VERSION 1.2.12）实测盘点；外部平台（GitHub 重定向、Docker Hub 不支持改名）的结论来自当日检索，实施前建议再核实一次平台当前行为。
