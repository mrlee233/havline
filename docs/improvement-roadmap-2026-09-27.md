# Fonu 改进路线实施方案（P0-P3）

> 制定日期：2026-09-27  
> 适用版本：v1.2.19 之后的迭代  
> 依据文档：`docs/project-analysis-2026-09-27.md`  
> 文档性质：实施方案，不代表本轮已经修改代码  
> 目标：把改进清单拆成可评审、可实施、可验证、可回滚的工程任务

---

## 0. 执行摘要

### 0.1 优先级与目标

| 优先级 | 目标 | 建议周期 | 预计工作量 | 产出 |
| --- | --- | --- | ---: | --- |
| P0 | 修复明确问题与安全边界 | 1 个短迭代 | 3-6 人日 | v1.2.x 修复版 |
| P1 | 降低 FRP、Agent、前端巨型模块风险 | 2-4 个迭代 | 15-25 人日 | 架构重构版 |
| P2 | 工程质量收口与测试补强 | 与 P1 并行，后续收尾 | 10-18 人日 | 稳定维护版 |
| P3 | 能力增强 | 独立产品迭代 | 40-70 人日 | v1.3/v1.4 功能版 |

预计工作量为熟悉本项目的一名工程师估算，不包含需求反复、真机环境等待和外部依赖调整。

### 0.2 推荐实施顺序

```text
M1：P0 修复与安全收口
  → M2：P1-4 测试基线与 P1-2 文件拆分
  → M3：P1-1 共享 Nginx 模板
  → M4：P1-3 前端拆分 + P1-5 轮询统一
  → M5：P2 工程收口
  → M6：P3 能力按单项排期
```

不推荐一次性同时推进 P1、P2、P3。FRP / Agent、前端大组件、数据库性能、迁移包和 2FA 都属于高回归风险区域，必须分批交付。

### 0.3 总体原则

1. **每一步都保持可运行**：先补测试和特征化断言，再做重构。
2. **优先移动而不是重写**：拆分大文件时先保持函数体不变，只改变文件归属。
3. **Agent 行为变更必须递增 `AgentVersion`**：并同步重编 amd64、arm64 内嵌二进制。
4. **数据库变更只向前迁移**：新增迁移从 `028` 开始，不修改已经发布的迁移。
5. **敏感数据默认不回传**：新增接口必须遵守现有的掩码与“只显示一次”约定。
6. **新增接口必须有测试**：至少覆盖无鉴权、正常路径、失败路径和兼容路径。
7. **重构不得顺手改业务语义**：同一 PR 只做一类事情，便于审查和回滚。
8. **删除前先确认引用**：P2 清理旧代码前，先用 `rg` 证明无运行时引用；删除归档内容需单独确认。

---

## 1. 基线与约束

### 1.1 当前代码基线

| 项目 | 当前值 |
| --- | --- |
| 版本 | `1.2.19` |
| Agent 版本 | `0.12.7` |
| Go 模块 | `github.com/fonu/fonu` |
| Go 声明版本 | 1.23 |
| Go 文件 | 235 个，其中 90 个测试文件 |
| 内部包 | 29 个 |
| 数据库迁移 | 27 个 |
| 受保护 API | 约 113 条 |
| Agent API | 20 条 `/api/v1/*` |
| 前端源文件 | 52 个 TypeScript / Vue 文件 |
| 前端测试 | 2 个 spec、13 条测试 |

### 1.2 当前验证门禁

每次合并前至少执行：

```bash
go test ./... -count=1
go vet ./...

cd web
npm run typecheck
npm test
npm run build
```

涉及 Agent、Docker、Nginx 或跨平台路径时，还需要补：

```bash
go build ./...
```

必要时使用 `scripts/build-linux.ps1` 验证 Linux 安装包。

### 1.3 当前已知工程状态

| 项目 | 当前状态 | 影响 |
| --- | --- | --- |
| `gofmt -l internal cmd` | 12 个文件未格式化 | 无法立即开启格式化 CI 卡口 |
| 前端硬编码颜色 | 约 287 处匹配 `#hex`、`rgb()`、`rgba()` | 主题一致性与维护成本 |
| FRP 主服务 | `internal/frp/service.go` 约 1805 行 | 修改风险集中 |
| Agent 主服务 | `internal/agent/server.go` 约 1551 行 | HTTP、Nginx、frps、配置写盘耦合 |
| 前端大页面 | `ProxyView.vue` 约 3150 行 | 组件复用、测试和样式隔离困难 |
| 工作区 | 有未提交的删除、二进制与品牌图片修改 | 开始迭代前需确认基线提交 |

### 1.4 开始实施前的仓库准备

1. 确认 `Fonu-1.0.2` 内当前未提交改动是否要保留。
2. 将品牌图片、内嵌 Agent 二进制更新拆成独立提交，避免和功能改动混在一起。
3. 把 `internal/frp-1.0.2` 与 `FrpView-1.0.2.vue` 的删除单独提交，不与其他重构同批。
4. 从最新 `main` 建分支，建议前缀 `codex/`。
5. 所有方案项都从当前 `origin/main` 的已提交状态开始，不基于未确认的工作区改动。

---

## 2. 全局完成定义

### 2.1 单个任务的完成定义

一个任务只有同时满足以下条件才算完成：

- 代码已实现，并保持现有接口兼容；
- 新增或更新了测试；
- `go test ./... -count=1` 通过；
- `go vet ./...` 通过；
- 前端改动执行 `npm run typecheck`、`npm test`、`npm run build`；
- Agent 行为变化已递增 `AgentVersion` 并重编双架构二进制；
- 数据库变化已新增迁移且验证旧库升级；
- 文档与 `CHANGELOG.md` 已更新；
- 明确记录回滚方式；
- 未把无关格式化、品牌资源、二进制替换混入同一提交。

### 2.2 单个 PR 的边界

| PR 类型 | 允许包含 | 不允许包含 |
| --- | --- | --- |
| P0 修复 | 单一缺陷、对应测试、必要文档 | 大范围重构、格式化全仓 |
| 拆分重构 | 文件移动、符号迁移、必要的包内适配 | 行为变化、顺手改接口 |
| 模板收敛 | 共享纯函数、两组调用方改造、golden 测试 | 顺手改变 Nginx 输出语义 |
| 前端拆分 | 组件搬迁、props/emits、样式同步搬迁 | 同时重做视觉样式 |
| 性能优化 | 基准、索引、确认后的连接策略 | 未量化就改 SQLite 配置 |
| P3 功能 | 迁移、接口、前端、测试、文档 | 多个 P3 功能混在一个 PR |

---

## 3. P0：先处理明确问题

P0 的目标是修复已经确认的缺陷与安全边界，不进行架构大改。

---

## 3.1 P0-1：修正 DDNS 页旧 `/api/frp` 调用

> 实施状态：2026-09-27 已完成。

### 3.1.1 现状

- `web/src/views/DdnsView.vue` 的 `init()` 调用 `api.getFRP()`。
- `web/src/api/client.ts` 仍保留旧方法：
  - `getFRP`
  - `saveFRP`
  - `getFRPStatus`
  - `syncFRPDomains`
  - `getFRPLogs`
- 后端 `internal/api/router.go` 当前不注册 `/api/frp`，只注册 `/api/frpmulti/*`。
- `api.getFRP()` 因此失败，进入 `catch`，`frpEnabled.value = false`。
- DDNS 页顶部“FRP 内网穿透已启用”提示不会正确出现。

### 3.1.2 目标

1. DDNS 页使用当前 FRP 多服务端接口。
2. 提示只在确实存在可用穿透配置时显示。
3. 不再依赖已退役的旧 API。

### 3.1.3 实施步骤

1. 将 `DdnsView.vue` 的调用改为：

   ```ts
   const status = await api.getFrpStatus()
   frpEnabled.value = status.configured && status.enabled_proxies > 0
   ```

2. 如产品语义希望“配置了服务端就提示”，可调整为：

   ```ts
   frpEnabled.value = status.configured
   ```

   推荐使用 `configured && enabled_proxies > 0`，避免用户刚建服务端但没有任何穿透规则时看到误导提示。

3. 将 `DdnsView.vue` 中 `api.getFRP` 的导入或调用彻底替换。
4. 暂时保留旧方法不做删除，删除工作纳入 P2-2，避免 P0 同时扩大改动范围。

### 3.1.4 测试

- 前端单元测试：
  - 新增 `DdnsView` 的辅助纯函数测试，或抽出 `shouldShowFrpHint(status)`。
  - 覆盖 `configured=false`、`enabled_proxies=0`、`configured=true && enabled_proxies>0`。
- 手工验证：
  - 无 FRP 服务端时无提示；
  - 有服务端但无启用规则时无提示；
  - 有启用规则时出现提示；
  - 接口失败时不阻塞 DDNS 页面。

### 3.1.5 验收标准

- `rg -n "api\\.getFRP" web/src/views/DdnsView.vue` 无结果。
- 页面不再向 `/api/frp` 发请求。
- DDNS 主要功能在接口异常时仍可加载。

### 3.1.6 风险

| 风险 | 处理 |
| --- | --- |
| `getFrpStatus` 返回状态与旧字段不同 | 使用明确映射函数，禁止直接把旧 `enabled` 字段改名为新字段 |
| 状态接口超时影响页面初始化 | 提示查询与主数据加载分离，失败只影响提示 |

### 3.1.7 实施记录

- `web/src/views/DdnsView.vue` 已改用 `api.getFrpStatus()`。
- 新增 `web/src/utils/frp.ts`，由 `shouldShowFrpHint` 统一判断“已配置且有启用规则”。
- 新增 `web/src/utils/frp.spec.ts`，覆盖配置、未配置、无启用规则和空状态。
- 旧 `/api/frp` 客户端方法尚未删除，按要求留到 P2-2 统一清理。
- 验证通过：`npm test` 17 条、`npm run typecheck`、`npm run build`。

---

## 3.2 P0-2：初始管理员密码不再写入持久应用日志

### 3.2.1 现状

- `internal/auth/auth.go` 的 `EnsureDefaultAdmin` 使用 `slog` 记录用户名和密码。
- `internal/app/app.go` 的日志同时写入 stdout 和 `<DataDir>/logs/app.log`。
- `internal/logstore/display.go` 的 `appendSystemAttrs` 专门把初始密码渲染进系统日志。
- `internal/api/logs_handler.go` 的 `/api/logs/system` 会读取 `app.log`。
- `scripts/fonu-install.sh` 当前从 `app.log` 中提取初始密码。

这意味着“初始密码只出现一次”的现实边界并不成立，密码会长期留在数据目录日志中。

### 3.2.2 目标

1. 初始密码只在首次启动时输出一次。
2. 默认输出到进程 stdout/stderr，不写入 `app.log`。
3. Docker 用户可通过 `docker logs fonu` 获取。
4. systemd 用户可通过 `journalctl -u fonu` 获取。
5. 安装脚本改为从 journal 或服务日志读取，不再读取 `app.log`。
6. 日志中心不再能读到初始密码。

### 3.2.3 推荐设计

修改 `auth.Service.EnsureDefaultAdmin`：

```go
type BootstrapAdmin struct {
    Username string
    Password string
    Created  bool
}

func (s *Service) EnsureDefaultAdmin(ctx context.Context) (BootstrapAdmin, error)
```

- `Created=false` 时不返回密码。
- 不在 `auth` 包内部写密码日志。
- `app.New` 收到 `Created=true` 后，使用独立的 `os.Stderr` 或标准输出直接打印，不经过 `slog` 的 `MultiWriter`。

示例输出：

```text
Fonu 首次启动已创建管理员账户
用户名：admin
初始密码：<一次性密码>
请登录后立即修改密码。
```

### 3.2.4 实施步骤

1. 重构 `EnsureDefaultAdmin` 返回值。
2. `app.New` 调整调用与一次性输出。
3. 删除 `logstore.appendSystemAttrs` 中初始密码的特殊拼接分支。
4. 更新 `logstore/display_test.go`，断言日志中出现初始密码构建消息时不再拼接 `password`。
5. 更新 `scripts/fonu-install.sh`：
   - 删除从 `app.log` grep 密码的逻辑；
   - 改为读取 `journalctl -u fonu` 最近日志；
   - 如果服务尚未写入日志，则提示用户执行 `journalctl` 或 `docker logs`。
6. 更新 Docker、原生安装和 FAQ 文档。

### 3.2.5 测试

- `internal/auth/auth_test.go`
  - 首次创建返回 `Created=true` 且有密码；
  - 第二次调用返回 `Created=false` 且无密码。
- `internal/app` 测试或日志测试：
  - `app.log` 不包含初始密码；
  - 标准输出可以包含一次初始密码。
- `internal/logstore/display_test.go`
  - 系统日志渲染不再输出 `password`。
- shell 验证：
  - 原生安装后 `journalctl -u fonu` 能读到初始密码；
  - `app.log` 中 grep 不到初始密码。

### 3.2.6 验收标准

- `<DataDir>/logs/app.log` 中不出现初始密码。
- `/api/logs/system` 响应中不出现初始密码。
- 安装脚本仍能提示首次密码。
- 已初始化实例不受影响。

### 3.2.7 兼容与回滚

- 已存在的日志不会被自动清理；可增加“安全清理旧日志”的手工建议，不在代码中自动删除。
- 回滚仅恢复日志输出方式，不影响管理员数据。
- 该修复不要求 Agent 升级。

---

## 3.3 P0-3：Agent Token 使用常量时间比较

> 实施状态：2026-09-27 已完成。

### 3.3.1 现状

`internal/agent/server.go` 的 `auth` 方法使用普通字符串比较：

```go
if r.Header.Get("Authorization") != "Bearer "+s.cfg.Token {
```

这不是直接可利用漏洞，但不符合安全敏感的 Token 比较约定。

### 3.3.2 目标

将 Bearer Token 解析成独立函数，并使用 `crypto/subtle.ConstantTimeCompare`。

### 3.3.3 推荐实现

```go
func bearerToken(r *http.Request) string {
    header := strings.TrimSpace(r.Header.Get("Authorization"))
    const prefix = "Bearer "
    if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
        return ""
    }
    return strings.TrimSpace(header[len(prefix):])
}

func secureEqual(got, want string) bool {
    return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
```

`auth` 方法：

```go
token := bearerToken(r)
if token == "" || !secureEqual(token, s.cfg.Token) {
    writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
    return
}
```

### 3.3.4 测试

新增 `internal/agent/auth_test.go`：

- 正确 Token 返回 200；
- 错误 Token 返回 401；
- 缺失 Authorization 返回 401；
- `bearer` 大小写兼容；
- 多空格处理；
- 长度不同时不 panic。

### 3.3.5 验收标准

- 所有 Agent 接口仍使用相同 Bearer Token。
- 正确 Token 通过，错误 Token 拒绝。
- Agent 不依赖应用侧 API 中间件代码。

### 3.3.6 实施记录

- `internal/agent/server.go` 新增 `bearerToken` 与 `secureTokenEqual`，认证路径改用 `crypto/subtle.ConstantTimeCompare`。
- `AgentVersion` 已由 `0.12.7` 提升到 `0.12.8`。
- 新增 `internal/agent/auth_test.go`，覆盖正确 Token、大小写 Bearer、多余空白、错误 Token、长度不同、缺失 Authorization 和错误认证方案。
- 内嵌 amd64 / arm64 二进制已重编；VPS 需要执行一次「升级 Agent」。
- 验证通过：`go test ./internal/agent -count=1`、`go test ./... -count=1`、`go vet ./...`、`npm test`（17 条）。

---

## 3.4 P0-4：限流状态解析失败时拒绝覆盖原状态

> 实施状态：2026-09-28 已完成。

### 3.4.1 现状

`internal/agent/server.go` 的 `readLimitState`：

```go
_ = json.Unmarshal(data, &state)
return state
```

如果 `route-limits.json` 损坏：

1. 返回空 map；
2. `applyLimitState` 只写入当前域名；
3. `writeLimitState` 用不完整状态覆盖原文件；
4. 其他域名的限流配置丢失。

### 3.4.2 目标

- 文件不存在：返回空状态，不报错。
- 文件存在但读取失败或 JSON 损坏：返回错误，不继续写。
- 调用方把错误回传到 HTTP 502/500，保留原文件和现有 Nginx zone。

### 3.4.3 推荐接口

```go
func readLimitState() (map[string]routeLimit, error)

func (s *Server) applyLimitState(req routeRequest) error
func (s *Server) clearLimitState(domain string) error
```

建议错误文案：

```text
读取限流状态失败: /var/lib/fonu-agent/route-limits.json: <原因>
```

### 3.4.4 实施步骤

1. 修改 `readLimitState` 返回 `(map, error)`。
2. `os.IsNotExist` 返回空 map 和 nil。
3. `os.ReadFile` 其他错误返回错误。
4. `json.Unmarshal` 错误返回错误。
5. `applyLimitState`、`clearLimitState` 直接传播错误。
6. 新增测试：
   - 文件不存在；
   - 空对象；
   - 正常对象；
   - 截断 JSON；
   - 类型错误；
   - 解析失败时原文件内容不变。
7. 可选：在损坏时先生成 `.corrupt-<timestamp>` 快照，但不得自动删除原文件。

### 3.4.5 验收标准

- 损坏状态不会导致其他域名限流配置被清空。
- 错误能显示在“应用配置”或路由保存响应中。
- `nginx -t` 仍以真实生成结果为准。

### 3.4.6 实施记录

- `readLimitState` 改为返回 `(map[string]routeLimit, error)`，新增可测试的 `readLimitStateFile`。
- 文件不存在返回空状态；读取失败、空文件、`null` 或 JSON 损坏返回错误。
- `applyLimitState` / `clearLimitState` 传播错误并保留原文件，不再用空状态覆盖。
- 新增 `internal/agent/limits_test.go`，覆盖缺失、正常、空文件、截断 JSON、null 和原文件不变。
- `AgentVersion` 提升到 `0.12.9`，双架构内嵌二进制已重编。
- 验证通过：`go test ./internal/agent -count=1`、`go test ./... -count=1`、`go vet ./...`、`npm test`。

---

## 3.5 P0-5：明确 Agent 默认监听策略

> 方案文档：`agent-listen-strategy-plan.md`（2026-09-28，待实施）

> 实施状态：阶段 1-3 已随 v1.4.0 实施（默认回环、SSH 隧道、旧 HTTP 自动迁移）；HTTPS + pin 高级模式待后续。

### 3.5.1 现状

- `cmd/fonu-agent/main.go` 默认 `:7700`。
- `internal/agent.NewServer` 空配置时也回落到 `:7700`。
- `scripts/agent-install.sh` 的 systemd 单位只设置环境文件，不显式限制监听地址。
- Agent 当前为明文 HTTP，Bearer Token 在网络中裸传。
- 安装脚本打印 `http://<host-ip>:7700`，鼓励直接公网访问。

### 3.5.2 策略选择

推荐采用两阶段策略，避免一次性破坏现有部署：

#### 阶段 A：先警告与显式标记

适用于下一个修复版本：

1. Agent 状态接口增加：
   - `listen_addr`
   - `transport_scheme`
   - `transport_secure`
   - `security_warning`
2. 当监听非环回地址且未启用 TLS 时：
   - 启动日志打印醒目的中文警告；
   - 前端“公网服务端”页展示警告；
   - 安装脚本提示必须使用 Nginx HTTPS 或 SSH 隧道；
   - 不立即阻止启动。

#### 阶段 B：默认仅本机，显式允许远程

适用于一个明确的 minor 版本：

1. 默认监听改为 `127.0.0.1:7700`。
2. 新增 `FONU_AGENT_ALLOW_REMOTE=1` 或 `-allow-remote`：
   - 没有该显式选项时，拒绝监听非环回地址。
3. 新增可选直接 TLS：
   - `FONU_AGENT_TLS_CERT`
   - `FONU_AGENT_TLS_KEY`
4. 非环回监听满足以下任一条件才允许：
   - TLS 证书和私钥已配置；
   - 明确使用 `--insecure-allow-http` 并打印高危警告。
5. 安装脚本默认写入：

   ```dotenv
   FONU_AGENT_LISTEN=127.0.0.1:7700
   ```

6. 如用户选择远程模式，安装脚本引导配置 Nginx HTTPS 反向代理，或写入显式不安全开关。

### 3.5.3 中央 Fonu 的连接方式

推荐远程连接路径：

```text
Fonu 控制面
  → https://agent.example.com
  → VPS Nginx 443
  → 127.0.0.1:7700 fonu-agent
```

备选路径：

```text
Fonu 控制面
  → SSH 本地转发或 WireGuard
  → 127.0.0.1:7700 fonu-agent
```

不推荐：

```text
Fonu 控制面
  → http://公网IP:7700
```

### 3.5.4 前端与接口变化

1. `AgentView.vue` 状态卡增加“传输安全”字段。
2. 当 `transport_secure=false` 且监听地址非本机时，显示橙色警告。
3. 安装脚本输出改为：

   ```text
   Agent 默认仅监听 127.0.0.1:7700。
   如需远程连接，请先配置 HTTPS 反向代理，再设置 FONU_AGENT_ALLOW_REMOTE=1。
   ```

### 3.5.5 兼容迁移

升级已有 Agent 时：

1. 保留 `/etc/fonu-agent/agent.env`。
2. 旧配置未设置 `FONU_AGENT_LISTEN` 时：
   - 阶段 A 继续使用旧默认值；
   - 阶段 B 给出迁移提示，并在一个过渡版本内保持旧行为。
3. 新安装默认 loopback。
4. 如果旧部署使用公网 `:7700`，升级说明必须明确如何加 Nginx HTTPS。
5. `agent_url` 当前是 `http://` 时，前端显示不安全提示。

### 3.5.6 测试

- 默认启动监听 loopback。
- 非环回监听无显式允许时拒绝启动。
- `--insecure-allow-http` 时启动并输出警告。
- 配置 TLS 时启动 HTTPS。
- 安装脚本生成的环境文件符合所选模式。
- 旧环境文件升级不丢 Token。

### 3.5.7 验收标准

- 新安装不会默认暴露明文 Agent 到公网。
- 用户仍可通过明确配置保留远程管理能力。
- 文档给出 Nginx HTTPS 与 SSH 隧道两种方案。
- 安全状态在前端可见。

---

## 4. P1：降低核心模块风险

P1 的核心是控制复杂度，不改变可见功能。

---

## 4.1 P1-1：抽共享 Nginx 模板包

### 4.1.1 现状

本机与公网 Agent 各有一套 Nginx 生成逻辑：

| 位置 | 用途 |
| --- | --- |
| `internal/nginx/generate.go` | 本机反代主配置与规则块 |
| `internal/nginx/security.go` | 本机安全、限流、geo、TLS |
| `internal/agent/server.go` 的 `nginxRoute` | VPS vhost |
| `internal/agent/server.go` 的 `sslDirectives` | VPS TLS |
| `internal/agent/server.go` 的 `zoneKey` / `writeLimitState` | VPS 限流 zone |

同一类安全问题需要改两处，容易漂移。

### 4.1.2 共享边界

新建纯生成包，建议命名：

```text
internal/nginxtmpl
```

该包只负责把结构化参数渲染成字符串，不依赖：

- `internal/proxy`
- `internal/config`
- `internal/frp`
- `internal/agent`
- 文件系统
- 进程执行

这样 `internal/nginx` 和 `internal/agent` 都可以安全导入，不会循环依赖。

### 4.1.3 建议 API

```go
package nginxtmpl

type TLSConfig struct {
    CertPath  string
    KeyPath   string
    TLS13Only bool
}

type AccessControl struct {
    Allow     []string
    Deny      []string
    ChinaOnly bool
}

type LimitConfig struct {
    Zone  string
    Rate  int
    Burst int
    Conn  int
}

type SecurityHeaders struct {
    HTTPS          bool
    Enabled        bool
    HSTSMaxAgeDays int
}

func SSLDirectives(cfg TLSConfig) string
func ServerSecurityHeaders(cfg SecurityHeaders) string
func AccessDirectives(cfg AccessControl, indent string) string
func LimitDirectives(cfg LimitConfig, indent string) string
func RateLimitZone(zone string, rate int) string
func ConnLimitZone(zone string) string
func ZoneName(prefix string, parts ...string) string
```

### 4.1.4 先做特征化测试

在改动前，为两份输出增加或固定以下测试：

- HTTPS 证书指令；
- TLS 1.3 开关；
- HSTS 与安全响应头；
- allow / deny 顺序；
- 白名单末尾 `deny all`；
- WebSocket Upgrade 头；
- 限流与连接数指令；
- 中国 IP geo 变量；
- 错误页 location；
- 空参数不输出多余指令。

这些测试先锁定现有语义，再替换为共享函数。

### 4.1.5 实施步骤

1. 新增 `internal/nginxtmpl`。
2. 先迁出无业务依赖的函数：
   - TLS 指令；
   - 安全响应头；
   - allow / deny；
   - WebSocket 头；
   - zone 命名。
3. 本机 `internal/nginx` 改为调用共享函数，保持输出逐字节一致。
4. Agent `nginxRoute` 分阶段改为调用共享函数。
5. 为中国 IP 仅大陆规则保留差异适配层：
   - 本机使用 `geo $fonu_client_ip` 与规则 ID 变量；
   - Agent 使用 `$fonu_china_client`；
   - 两者共享 allow/deny 与返回 403 结构，不强行共享变量名。
6. 删除重复实现，但保留本机与 Agent 各自的上下文拼装函数。

### 4.1.6 验收标准

- 同一参数在两端的 TLS、安全头、WebSocket、限流输出一致。
- `internal/agent` 不再自行实现通用 TLS 指令。
- 两端测试均可独立覆盖共享模板。
- 本机 Nginx 与 Agent 生成内容没有未解释的语义变化。

### 4.1.7 风险

| 风险 | 处理 |
| --- | --- |
| 两端缩进与上下文不同 | 共享函数接受 `indent`，不强制服务块结构 |
| Agent 使用域名 zone，本机使用规则 ID | `ZoneName` 只负责合法化，不负责决定命名规则 |
| 修改模板导致现有 vhost 重写 | 先做 golden 对比，再在部署时保留版本回滚 |

---

## 4.2 P1-2：拆分 `internal/frp/service.go` 与 `internal/agent/server.go`

### 4.2.1 拆分原则

1. 同一 Go 包内拆文件，不改变外部接口。
2. 只移动函数、类型和常量，不重写逻辑。
3. 每个文件目标 300-500 行以内。
4. 每个函数继续遵守函数长度不超过 50 行的仓库约定。
5. 拆分后先跑全量测试，再做后续优化。

### 4.2.2 `internal/frp` 目标文件

| 新文件 | 责任 |
| --- | --- |
| `service.go` | Service 结构、构造、公共装配 |
| `service_servers.go` | 服务端 CRUD、配置保存、凭证与 TLS 资产 |
| `service_proxies.go` | 穿透规则 CRUD、列表、日志 |
| `service_runtime.go` | frpc 启动、停止、重载、状态、自动恢复 |
| `service_download.go` | 版本列表、下载任务、二进制激活与清理 |
| `service_diagnostics.go` | 服务端五项诊断与延迟记录 |
| `service_agent_lifecycle.go` | SSH 安装、升级、卸载、Probe、急救诊断 |
| `service_agent_routes.go` | vhost 部署、删除、漂移、版本与证书推送 |
| `service_health.go` | 规则健康、可用率、汇总 |
| `service_config.go` | frpc.toml / frps.toml 渲染与原子写 |
| `service_traffic.go` | frps dashboard 流量适配 |

### 4.2.3 `internal/agent` 目标文件

| 新文件 | 责任 |
| --- | --- |
| `server.go` | Config、Server、NewServer、路由注册、ListenAndServe |
| `auth.go` | Bearer Token 解析与常量时间比较 |
| `handlers_status.go` | ping、status、metrics 状态输出 |
| `handlers_routes.go` | route CRUD、conf、版本、回滚 |
| `handlers_nginx.go` | Nginx 探测、安装、校验、重载、日志 |
| `handlers_frps.go` | frps 配置、安装、启停、日志 |
| `handlers_certs.go` | 证书写入与列表 |
| `handlers_china.go` | 中国 IP 段写入与 geo 文件生成 |
| `limits.go` | route-limits.json、zone 文件、限流状态 |
| `nginx_template.go` | Agent vhost 模板拼装 |
| `path_detect.go` | Nginx 目录、可执行文件与版本探测 |
| `basic_auth.go` | htpasswd 生成与复用 |

### 4.2.4 拆分顺序

1. 先拆 `internal/frp` 中低耦合的下载、诊断、健康文件。
2. 再拆服务端 CRUD 与运行管理。
3. 最后拆 Agent 生命周期与路由部署。
4. `internal/agent` 先拆 auth、limits、handlers_nginx，再拆 routes。
5. 每拆一组文件执行：

   ```bash
   go test ./internal/frp ./internal/agent -count=1
   go test ./... -count=1
   ```

### 4.2.5 验收标准

- 包外 API 没有变化。
- 没有新增循环依赖。
- 测试结果与拆分前一致。
- 文件行数显著下降，主文件不再承担所有职责。
- 行为变更与文件移动没有混在同一 PR。

---

## 4.3 P1-3：拆分前端巨型页面

### 4.3.1 拆分原则

1. 组件只接收 props，通过 emits 通知父页面。
2. 网络请求留在页面或 composable 中，避免弹窗组件私自保存。
3. scoped 样式必须随模板一起搬迁。
4. 每次只拆一个页面，先拆纯展示区域，再拆表单。
5. 拆分后视觉应与原页面一致。

### 4.3.2 `ProxyView.vue` 拆分建议

| 新组件 / composable | 责任 |
| --- | --- |
| `ProxyRuleFormModal.vue` | 新建/编辑表单、基础/安全/Nginx 三个页签 |
| `ProxyRuleDetailModal.vue` | 规则详情、概览与日志 |
| `ProxyClientsPanel.vue` | 当前连接与客户端 IP |
| `ProxyNginxPanel.vue` | 自动/自定义配置、版本与回滚 |
| `useProxyRules.ts` | 列表、保存、删除、排序 |
| `useProxyTraffic.ts` | 流量与在线连接轮询 |

### 4.3.3 `SettingsView.vue` 拆分建议

| 新组件 / composable | 责任 |
| --- | --- |
| `SettingsGeneralPanel.vue` | 主题、时区、日志、DDNS、证书默认值 |
| `SettingsNotificationsPanel.vue` | 通知渠道与告警事件 |
| `SettingsSecurityPanel.vue` | 登录失败、全局 IP 策略、信任代理 |
| `SettingsAdvancedPanel.vue` | 中国 IP、Nginx 主配置、状态页、指标 |
| `BackupPanel.vue` | 立即备份、归档、恢复、删除 |
| `ApiTokensPanel.vue` | Token 创建、范围、有效期、撤销 |
| `useSettingsForm.ts` | 设置加载、脏状态、保存与掩码字段 |

### 4.3.4 `AgentView.vue` 拆分建议

| 新组件 / composable | 责任 |
| --- | --- |
| `AgentServerFormModal.vue` | 服务端、Agent、SSH、frps 配置表单 |
| `AgentServerDetailModal.vue` | 详情、frps 同源配置、控制入口 |
| `AgentServerLogsModal.vue` | Agent / frps 日志 |
| `AgentDiagnosticsPanel.vue` | 五项诊断与可用率 |
| `AgentMetricsPanel.vue` | 资源、延迟、日志和证书 |
| `useAgentServers.ts` | 列表、选择、加载、生命周期调用 |
| `useAgentPolling.ts` | 状态与诊断轮询 |

### 4.3.5 测试与验证

- 拆分后保留原有交互路径。
- 为 props/emits 增加 `@vue/test-utils` 测试。
- 使用 Playwright 对桌面与移动视口截图：
  - `/proxies`
  - `/settings`
  - `/frp-agent`
- 检查弹窗尺寸、滚动区域、按钮位置、移动端文本溢出。
- 检查 `vue-tsc` 与 `vite build`。

### 4.3.6 验收标准

- 单个页面文件不再超过约 1200 行。
- 新组件职责单一，父页面只负责协调。
- 原页面的交互和视觉不变。
- 拆分过程中未丢失 scoped 样式。

---

## 4.4 P1-4：补路由鉴权、配置回滚、Agent 版本兼容测试

### 4.4.1 路由鉴权覆盖测试

目标：防止新增 API 时漏掉 `protect()`。

建议改造 `internal/api/router.go`：

```go
type routeSpec struct {
    Method   string
    Pattern  string
    Handler  http.HandlerFunc
    Public   bool
    TokenDeny bool
}
```

- 构造统一路由表；
- 注册循环只负责挂载与包中间件；
- 测试直接遍历路由表；
- 对受保护路由发匿名请求，期望 401；
- 对公开路由断言不因鉴权而 401。

测试文件：

```text
internal/api/router_auth_test.go
```

Agent 同样增加：

```text
internal/agent/router_auth_test.go
```

遍历 20 个 `/api/v1/*` 路径，无 Bearer Token 必须返回 401。

### 4.4.2 配置回滚测试

覆盖本机：

- `internal/nginx/versions.go` 的版本列表、读取、回滚；
- 非法版本名拒绝路径上跳；
- 回滚时先校验语法；
- 新配置校验失败时不覆盖当前配置；
- `fsutil.BackupVersioned` 去重逻辑。

覆盖 Agent：

- 写 vhost 前保存版本；
- `nginx -t` 失败后恢复上一可用版本；
- 首次部署失败时删除新文件而不破坏已有文件；
- 回滚接口调用后执行 `nginx -s reload`；
- 旧 Agent 无版本接口时返回升级提示。

建议测试文件：

```text
internal/nginx/versions_test.go
internal/agent/versions_test.go
internal/frp/agent_versions_compat_test.go
```

### 4.4.3 Agent 版本兼容测试

建立最低版本矩阵：

| 能力 | 最低 Agent 版本 |
| --- | --- |
| 基础状态 / 路由 | 现有最低版本 |
| metrics / logs / frps-config | 0.11.0 |
| route versions / rollback | 0.12.2 |
| 新版安全监听策略 | 新版本 |

测试内容：

- `AgentClient` 遇到 404 / 405 返回 `ErrAgentEndpointMissing`；
- 错误信息包含最低版本和升级提示；
- 其他 4xx / 5xx 不误判为版本过低；
- 状态接口返回 `agent_version` 后可正确展示；
- 新字段缺失时前端降级，不显示空白页。

### 4.4.4 验收标准

- 新增受保护路由漏鉴权时，测试会失败。
- 配置回滚失败不会覆盖当前可用配置。
- 旧 Agent 与新控制端组合时有明确提示，不崩溃。

---

## 4.5 P1-5：统一前端轮询组合式函数

### 4.5.1 现状

已有 `web/src/composables/useVisibilityPolling.ts`，并被：

- `ProxyRemoteView.vue`
- `FrpView.vue`

使用，但其他页面仍自行维护计时器：

- `DashboardView.vue`
- `ProxyView.vue`
- `LogsView.vue`
- `AgentView.vue`
- `SettingsView.vue` 的中国 IP 状态轮询

当前 composable 没有 in-flight 互斥，慢请求仍可能堆积。

### 4.5.2 目标

统一行为：

1. 页面隐藏时暂停；
2. 回到前台立即补一次；
3. 上一轮未结束时跳过下一轮；
4. 支持 `enabled` 开关；
5. 组件卸载时清理；
6. 可选 `immediate` 立即执行；
7. 返回 `busy` 状态，便于 UI 展示。

### 4.5.3 建议 API

```ts
export interface VisibilityPollingOptions {
  enabled?: Ref<boolean>
  immediate?: boolean
  onError?: (error: unknown) => void
}

export function useVisibilityPolling(
  fn: () => void | Promise<void>,
  intervalMs: number,
  options: VisibilityPollingOptions = {},
) {
  // return { start, stop, busy, lastError }
}
```

### 4.5.4 实施步骤

1. 更新 composable，增加 in-flight 锁。
2. `tick()` 中：

   ```ts
   if (busy.value) return
   busy.value = true
   Promise.resolve(fn())
     .catch(onError)
     .finally(() => { busy.value = false })
   ```

3. 将页面手动计时器替换为 composable。
4. 页面已有的“首次加载”和“轮询”分开：
   - 首次加载立即执行；
   - 轮询只刷新增量或状态。
5. SSE 日志不纳入轮询 composable。

### 4.5.5 测试

- 隐藏时不执行；
- 回到前台执行一次；
- 请求未结束时跳过下一轮；
- `enabled=false` 不执行；
- 卸载后无计时器；
- `onError` 被调用且不影响后续轮询。

### 4.5.6 验收标准

- 各页面不再重复实现 `setInterval + visibilitychange`。
- 慢接口不会产生请求堆积。
- 页面前后台切换行为一致。

---

## 5. P2：工程质量提升

P2 的重点是减少噪音、建立长期质量门禁，不改变大功能。

---

## 5.1 P2-1：全仓 `gofmt` 与 CI 卡口

### 5.1.1 当前结果

执行：

```bash
gofmt -l internal cmd
```

当前仍有未格式化文件，包括：

- `internal/acme/job.go`
- `internal/api/auth_handler_test.go`
- `internal/api/logs_handler_test.go`
- `internal/api/version_handler.go`
- `internal/ddns/store.go`
- `internal/ddns/tencentcloud_test.go`
- `internal/frp/diagnostics.go`
- `internal/frp/store.go`
- `internal/nginx/generate_multi_test.go`
- `internal/nginx/generate_seed_mix_test.go`
- `internal/nginx/realip.go`
- `internal/nginx/security.go`

### 5.1.2 实施步骤

1. 单独创建“格式化”提交。
2. 执行：

   ```bash
   gofmt -w ./cmd ./internal
   ```

3. 检查 diff，确认只有格式变化。
4. 运行：

   ```bash
   go test ./... -count=1
   go vet ./...
   ```

5. 在 `.github/workflows/ci.yml` 的 `go vet` 前增加：

   ```yaml
   - name: gofmt
     run: test -z "$(gofmt -l ./cmd ./internal)"
   ```

### 5.1.3 验收标准

- `gofmt -l internal cmd` 无输出。
- CI 对后续未格式化文件立即失败。
- 格式化 PR 不包含业务逻辑变化。

---

## 5.2 P2-2：清理旧 FRP 类型、旧 API 方法与未使用样式

### 5.2.1 旧 API 清理

目标：

- `web/src/api/client.ts` 删除：
  - `getFRP`
  - `saveFRP`
  - `getFRPStatus`
  - `syncFRPDomains`
  - `getFRPLogs`
- `web/src/api/types.ts` 删除不再使用的旧接口：
  - `FRPConfig`
  - `FRPResponse`
  - `FRPSaveResponse`
  - `FRPSavePayload`
  - 以及只服务旧 `/api/frp` 的 `FRPStatus`

注意：`FrpStatus` 这个名字与多服务端 `FrpStatus` 有潜在冲突，清理时先确认类型引用。

### 5.2.2 前端样式清理流程

1. 枚举模板中的 class；
2. 与 `<style scoped>` 中定义对比；
3. 标记只出现在样式、不出现在模板与脚本中的类；
4. 人工确认动态 class：
   - `:class="{ ... }"`
   - 字符串拼接；
   - `:deep()`；
5. 删除确认无用的样式块；
6. 每次只清理一个页面；
7. 截图对比删除前后。

重点页面：

- `FrpView.vue`
- `AgentView.vue`
- `ProxyView.vue`
- `ProxyRemoteView.vue`

### 5.2.3 验收标准

- `rg -n "/api/frp\\b" web/src internal` 不再包含旧路由调用。
- `vue-tsc` 无 unused 报错。
- 页面视觉无回归。
- 删除项有明确引用证明。

---

## 5.3 P2-3：收敛颜色令牌与重复弹窗外壳

### 5.3.1 颜色令牌

当前匹配 `#hex`、`rgb()`、`rgba()`、`color-mix()` 约 287 处。建议在 `web/src/styles/variables.css` 增加语义令牌：

```css
--fonu-tone-info
--fonu-tone-success
--fonu-tone-warning
--fonu-tone-error

--fonu-tone-info-soft
--fonu-tone-success-soft
--fonu-tone-warning-soft
--fonu-tone-error-soft

--fonu-log-bg
--fonu-log-text
--fonu-log-success
--fonu-log-warning
--fonu-log-error

--fonu-chart-1
--fonu-chart-2
--fonu-chart-3
--fonu-chart-4
--fonu-chart-5
--fonu-chart-6
```

替换顺序：

1. `StatCard`、`StatusBadge`；
2. 仪表盘横幅；
3. 证书、DDNS、反代状态色；
4. 日志终端色；
5. 图表色板；
6. 零散页面。

### 5.3.2 弹窗外壳

新增 `web/src/components/ProxyModalShell.vue`：

| 插槽 | 内容 |
| --- | --- |
| `header` | 标题、状态、关闭按钮 |
| `tabs` | 可选页签 |
| `default` | 滚动内容 |
| `footer` | 保存、取消 |
| `help` | 右侧或底部说明面板 |

使用页面：

- `ProxyView.vue`
- `FrpView.vue`
- `ProxyRemoteView.vue`
- `AgentView.vue`

### 5.3.3 左侧列表

新增 `SidebarList.vue`：

- `items`
- `activeKey`
- `loading`
- `emptyText`
- `item` 插槽
- `header` 插槽

先替换同构的：

- `FrpView.vue`
- `AgentView.vue`
- `ProxyRemoteView.vue`
- `DdnsView.vue`

### 5.3.4 验收标准

- 新增颜色优先使用语义令牌。
- 弹窗外壳不再在四个页面各自复制。
- 暗色模式与亮色模式截图一致。

---

## 5.4 P2-4：前端 API 客户端测试与关键组件测试

### 5.4.1 测试环境

新增开发依赖：

```text
@vue/test-utils
happy-dom
```

Vitest 配置使用 `happy-dom`，避免引入完整浏览器。

### 5.4.2 API 客户端测试

测试 `web/src/api/client.ts`：

- 204 返回 `undefined`；
- 非 2xx 解析 JSON error；
- 非 JSON 错误回退；
- 30 秒超时；
- 证书 10 分钟超时；
- `credentials: include`；
- `asList` 对 null / undefined 的处理；
- Blob 下载解析文件名。

建议 mock `global.fetch`，使用 `vi.useFakeTimers()` 控制超时。

### 5.4.3 组件测试

优先测试纯组件和共用组件：

- `StatusBadge.vue`
- `StatCard.vue`
- `FonCard.vue`
- `EmptyState.vue`
- `ConfiguredSecretField.vue`
- `ConfigDiffView.vue`
- `ProxyModalShell.vue`
- `SidebarList.vue`

页面拆分后，再测试：

- `ProxyRuleFormModal`
- `SettingsNotificationsPanel`
- `AgentServerFormModal`

### 5.4.4 验收标准

- `npm test` 在 CI 中运行。
- 新增组件必须有测试或明确说明为何不测。
- 关键错误路径有前端断言，而不是只依赖手工点击。

---

## 5.5 P2-5：SQLite 高频查询基准与单连接评估

### 5.5.1 当前配置

`internal/db/db.go`：

```go
conn.SetMaxOpenConns(1)
```

优点是避免写锁冲突，缺点是所有读写串行。

### 5.5.2 基准目录

新增：

```text
internal/db/benchmark_test.go
internal/db/fixture_test.go
```

或放在对应包：

```text
internal/metrics/benchmark_test.go
internal/frp/health_benchmark_test.go
internal/proxy/benchmark_test.go
```

### 5.5.3 基准场景

| 场景 | 数据量 | 目标 |
| --- | ---: | --- |
| 24 小时指标查询 | 约 7,920 点 | < 100ms |
| 7 天指标查询、720 桶 | 约 55,440 点 | < 250ms |
| 100 条反代规则列表 | 100 规则 | < 50ms |
| 规则可用率推导 | 30 天状态变化 | < 100ms |
| 日志分页 | 10 万行 | < 200ms |
| 并发读 8 / 写 1 | 60 秒 | 无 `database is locked` |

目标值基于常见 NAS CPU，具体以项目基准环境为准。

### 5.5.4 评估顺序

1. 先建立基准，不在没有数据时改配置。
2. 检查索引是否被查询命中。
3. 评估 WAL：

   ```sql
   PRAGMA journal_mode=WAL;
   PRAGMA synchronous=NORMAL;
   ```

4. 评估单独读连接池：
   - 写仍保持单连接；
   - 读连接数量受控。
5. 仍不达标时：
   - 指标预聚合；
   - 分区式表或定期归档；
   - 避免在单次请求中加载全部历史。

### 5.5.5 验收标准

- 有可重复执行的基准命令。
- 有基准结果记录到文档。
- 任何连接策略变更都有并发测试与回滚方案。

---

## 6. P3：能力增强

P3 每项都建议独立立项、独立迁移、独立 UI 和测试。

---

## 6.1 P3-1：加密迁移包

### 6.1.1 目标

支持导出一个包含敏感凭据的完整实例包，并在另一台 Fonu 上导入，用于换机、重装和灾备。

### 6.1.2 与现有备份的区别

| 能力 | 现有脱敏导出 | 加密迁移包 |
| --- | --- | --- |
| 保存设置与规则 | 部分 | 完整 |
| 保存 DDNS / FRP 凭据 | 否 | 是，加密 |
| 保存证书与私钥 | 否 | 是，加密 |
| 保存会话 | 可选，不建议 | 默认不保存 |
| 导入到其他实例 | 否 | 是 |
| 需要口令 | 否 | 是 |

### 6.1.3 包结构

建议文件扩展名：

```text
.fonu-migrate
```

逻辑结构：

```text
manifest.json
db/fonu.db
certs/
nginx/
frp/
settings/
checksums.txt
```

不建议包含：

- `logs/`
- `backups/`
- 正在运行的 socket / pid 文件
- Agent 临时下载目录

### 6.1.4 加密设计

- KDF：Argon2id，使用随机 salt；
- 对称加密：AES-256-GCM；
- 文件按块加密，避免整包读入内存；
- 每个块使用独立 nonce；
- 包尾包含校验；
- manifest 在解密后校验版本和 schema。

如果选择引入成熟库，可评估 `age` 或其他流式加密库；否则复用现有 `golang.org/x/crypto/argon2`。

### 6.1.5 导出流程

1. 校验当前数据库可读。
2. 使用 SQLite `VACUUM INTO` 生成一致快照。
3. 收集证书、Nginx 配置、frp 配置。
4. 生成 manifest 和 checksums。
5. 打 tar。
6. 流式加密到目标文件。
7. 验证可解密并读取 manifest。
8. 返回一次性下载。

### 6.1.6 导入流程

1. 只允许导入空实例，或要求显式“覆盖现有数据”。
2. 上传后先写临时目录。
3. 校验文件头、版本、schema、checksums。
4. 解密到临时目录。
5. 验证 SQLite 完整性：

   ```sql
   PRAGMA integrity_check;
   ```

6. 自动创建导入前备份。
7. 原子替换数据目录关键文件。
8. 要求重启 Fonu。

### 6.1.7 接口与前端

接口建议：

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| POST | `/api/migration/export` | 生成加密包 |
| POST | `/api/migration/inspect` | 解密前只读元数据 |
| POST | `/api/migration/import` | 导入 |

前端放在设置页“数据”或“高级”中：

- 口令与确认口令；
- 导出按钮；
- 上传与预检；
- 显示版本、schema、文件数、创建时间；
- 导入二次确认；
- 结果提示与重启提醒。

### 6.1.8 测试

- 正确口令导出并导入；
- 错误口令失败且不落盘；
- 文件截断失败；
- checksum 篡改失败；
- 高版本 schema 拒绝；
- 已存在数据默认拒绝覆盖；
- 导入前自动备份生成。

---

## 6.2 P3-2：审计日志

### 6.2.1 目标

记录“谁、何时、从哪里、修改了什么、修改前是什么、修改后是什么”。

### 6.2.2 数据表

新增迁移：

```sql
CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    actor_name TEXT NOT NULL,
    source_ip TEXT NOT NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    summary TEXT NOT NULL,
    before_json TEXT,
    after_json TEXT,
    created_at TEXT NOT NULL
);
```

建议索引：

```sql
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);
CREATE INDEX idx_audit_logs_target ON audit_logs(target_type, target_id, created_at);
```

### 6.2.3 记录范围

必须审计：

- 反代规则创建、修改、删除、排序；
- Nginx 配置保存与回滚；
- DDNS 创建、修改、删除、凭证更新；
- 证书申请、导入、续期、删除；
- FRP 服务端与规则创建、修改、删除；
- Agent 安装、升级、卸载；
- frps 配置下发与启停；
- 设置修改；
- API Token 创建与撤销；
- 管理员改密、2FA 变更；
- 备份恢复与迁移导入。

不记录：

- 已脱敏字段的真实值；
- 密码、Token、私钥明文；
- 每次 GET 和无副作用的轮询。

### 6.2.4 实现方式

新增：

```text
internal/audit
```

建议接口：

```go
type Entry struct {
    ActorType string
    ActorID   string
    ActorName string
    SourceIP  string
    Action    string
    TargetType string
    TargetID  string
    Summary   string
    Before    any
    After     any
}

func (s *Service) Record(ctx context.Context, entry Entry) error
```

在业务成功写入后记录，不用全局中间件猜测副作用。

### 6.2.5 前端

新增审计页或在日志中心增加“审计”页签：

- 时间范围；
- 操作者；
- 动作类型；
- 目标；
- 来源 IP；
- 详情弹窗显示脱敏前后差异。

### 6.2.6 保留策略

- 默认保留 180 天；
- 设置页可配置 30/90/180/365/永久；
- 清理任务加入 scheduler。

### 6.2.7 测试

- 每个写入口成功时记录；
- 失败操作不误记成功；
- 敏感字段被掩码；
- API Token 与会话都能识别 actor；
- 分页、筛选和保留策略。

---

## 6.3 P3-3：2FA / TOTP 与登录设备管理

### 6.3.1 数据模型

`admins` 扩展：

```text
totp_secret_enc
totp_enabled
totp_confirmed_at
recovery_codes_json
```

`sessions` 扩展：

```text
user_agent
source_ip
last_seen_at
label
created_at
```

新增登录挑战表：

```sql
CREATE TABLE login_challenges (
    id TEXT PRIMARY KEY,
    admin_id INTEGER NOT NULL,
    expires_at TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
```

### 6.3.2 登录流程

```text
用户名密码校验
  → 未开 2FA：直接创建 session
  → 已开 2FA：返回 challenge
  → 用户提交 TOTP 或恢复码
  → 校验成功创建 session
```

### 6.3.3 接口

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/api/auth/2fa/status` | 当前账号 2FA 状态 |
| POST | `/api/auth/2fa/setup` | 生成 secret 与 otpauth URI |
| POST | `/api/auth/2fa/enable` | 校验首个 TOTP 并启用 |
| POST | `/api/auth/2fa/disable` | 密码 + TOTP 二次确认后关闭 |
| POST | `/api/auth/2fa/verify` | 登录挑战校验 |
| POST | `/api/auth/2fa/recovery` | 使用恢复码登录 |
| GET | `/api/settings/sessions` | 登录设备列表 |
| DELETE | `/api/settings/sessions/{id}` | 踢下线 |

### 6.3.4 安全要求

- TOTP secret 使用现有 secret box 加密；
- 恢复码只显示一次，库中存哈希；
- challenge 短有效期，5 分钟；
- 最大尝试次数，超过后作废；
- TOTP 窗口限制 ±1 step；
- 记录最近成功 step，拒绝重放；
- 修改 2FA 需登录会话，不允许 API Token；
- 禁用 2FA 需密码与 TOTP 双确认。

### 6.3.5 前端

设置页“安全”增加：

- 2FA 状态卡；
- 二维码绑定；
- 恢复码一次性展示；
- 关闭确认；
- 登录设备列表；
- “退出其他设备”和单设备撤销。

### 6.3.6 测试

- TOTP 正确 / 错误 / 过期；
- 恢复码一次性；
- 挑战过期与尝试上限；
- 禁用 2FA 需要双确认；
- 会话列表与撤销；
- API Token 不能管理 2FA 和会话。

---

## 6.4 P3-4：SMART 磁盘健康与 UPS 状态

### 6.4.1 目标

在仪表盘增加 NAS 硬件健康：

- 磁盘 SMART 温度、通电时长、重分配扇区、待处理扇区、总体健康；
- UPS 电量、剩余时间、市电状态。

### 6.4.2 后端设计

新增：

```text
internal/hardware
```

接口：

```go
type DiskHealth struct {
    Device           string
    Model            string
    Serial           string
    Health           string
    TemperatureC     int
    PowerOnHours     uint64
    Reallocated      uint64
    PendingSectors   uint64
    Uncorrectable    uint64
    Available        bool
    Error            string
}

type UPSStatus struct {
    Name             string
    Online           bool
    BatteryChargePct int
    RuntimeSeconds   int
    LoadPct          int
    Available        bool
    Error            string
}
```

### 6.4.3 SMART 采集

1. 从 `/sys/block` 枚举物理块设备。
2. 排除：
   - loop；
   - ram；
   - dm；
   - md 虚拟设备。
3. 检测 `smartctl` 是否存在。
4. 固定 argv 执行：

   ```bash
   smartctl -H -A -j /dev/sdX
   ```

5. 解析 JSON。
6. 结果缓存 5 分钟，避免频繁唤醒磁盘。
7. 无权限时返回 `Available=false` 和明确错误，不影响仪表盘。

### 6.4.4 UPS 采集

优先 NUT：

```bash
upsc <ups>@localhost
```

解析：

- `ups.status`
- `battery.charge`
- `battery.runtime`
- `ups.load`

如果没有 NUT，尝试只读 `/sys/class/power_supply`。

### 6.4.5 接口与告警

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/api/hardware/disks` | 磁盘健康 |
| GET | `/api/hardware/ups` | UPS 状态 |
| GET | `/api/status` | 合并一条硬件摘要 |

告警事件：

- SMART 健康非 PASSED；
- 温度超过阈值；
- 重分配 / 待处理扇区增加；
- UPS 切换到电池；
- 电量低于阈值；
- 剩余时间低于阈值。

### 6.4.6 前端

仪表盘新增“硬件健康”卡：

- 磁盘数量；
- 异常磁盘数；
- 最高温度；
- UPS 电量；
- 剩余时间；
- 点击展开详情。

### 6.4.7 测试

- smartctl JSON fixture；
- 无 smartctl；
- 权限不足；
- 磁盘休眠或读取失败；
- NUT 存在 / 不存在；
- 多个 UPS；
- 阈值告警与恢复。

---

## 6.5 P3-5：Prometheus `/metrics` 导出

### 6.5.1 目标

让 Grafana、VictoriaMetrics、HomeAssistant 等系统可以直接抓取 Fonu 指标。

### 6.5.2 鉴权策略

推荐：

1. 新增设置项 `prometheus_enabled`，默认关闭。
2. 新增设置项 `prometheus_token`：
   - 只显示一次；
   - 库中保存哈希。
3. 支持：

   ```http
   Authorization: Bearer <prometheus_token>
   ```

4. 也允许使用已有 `read` API Token。
5. 未启用时返回 404，避免暴露端点存在。

### 6.5.3 指标设计

```text
fonu_build_info{version,agent_version}
fonu_host_cpu_percent
fonu_host_memory_bytes{state="used|total"}
fonu_host_disk_bytes{state="used|total"}
fonu_proxy_traffic_bytes_total{rule_id,name,direction}
fonu_proxy_connections{rule_id,name}
fonu_route_up{server_id,domain}
fonu_route_uptime_ratio{server_id,domain,window}
fonu_frp_server_up{server_id,name}
fonu_frp_server_latency_ms{server_id,name}
fonu_certificate_days_left{domain}
fonu_notify_events_total{event,result}
```

### 6.5.4 采集策略

- 使用 `prometheus/client_golang`；
- 抓取时读取内存缓存或当前快照；
- 数据库查询设置 2 秒超时；
- 不因一个数据源失败让整个抓取失败；
- 控制 label 数量，避免规则名称无限增长；
- 不暴露凭据、路径、内网上游。

### 6.5.5 前端

设置页增加：

- 开关；
- Token 创建与复制；
- 抓取地址示例；
- 当前状态。

### 6.5.6 测试

- 未启用 404；
- 无 Token 401；
- 正确 Token 200；
- 输出格式可被 Prometheus parser 解析；
- 标签不包含敏感路径；
- 数据源失败时仍能导出部分指标。

---

## 6.6 P3-6：状态页事故记录与服务入口聚合

### 6.6.1 状态页事故记录

现有 `route_health_events` 已经记录状态变化，可用于推导事故。

新增：

- 当前未恢复事故；
- 最近 24 小时 / 7 天事故列表；
- 开始时间、结束时间、持续时长、原因；
- 每条规则的可用率。

接口：

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/api/status-page` | 增加 active incidents |
| GET | `/api/status-page/incidents` | 查询事故窗口 |

公开状态页仍然只显示：

- 域名；
- 状态；
- 可用率；
- 事故时间；
- 不含内网地址、端口、证书路径和上游地址。

### 6.6.2 服务入口聚合

目标：在仪表盘把启用中的服务聚合为可点击入口。

数据源：

- 本机 `proxy_rules` 与 `proxy_hosts`；
- 公网 FRP 规则与域名；
- 健康状态；
- 证书状态。

接口建议：

```http
GET /api/services
```

返回：

```json
{
  "items": [
    {
      "name": "示例服务",
      "domain": "app.example.com",
      "source": "proxy",
      "scheme": "https",
      "health": "up",
      "cert_days_left": 72
    }
  ]
}
```

前端：

- 卡片或紧凑列表；
- 按来源筛选；
- 健康点；
- 点击打开新窗口；
- 复制域名；
- 无启用规则时显示空态。

### 6.6.3 性能要求

- 服务入口读取缓存，不逐条探测；
- 健康结果复用 monitor 最近一轮；
- 100 条规则时接口目标小于 100ms；
- 公开状态页不能查询敏感表。

### 6.6.4 测试

- 事故从状态变化推导正确；
- 未恢复事故持续增长；
- unknown 不计入 down；
- 服务入口不含重复域名；
- 无健康数据时显示 unknown；
- 公开状态页不泄露内网信息。

---

## 7. 跨模块测试矩阵

| 场景 | 本地 Go | 前端 | Docker | fnOS host | VPS Debian | 宝塔 Nginx |
| --- | --- | --- | --- | --- | --- | --- |
| P0 修复 | 必须 | 必须 | 可选 | 建议 | 可选 | 可选 |
| Agent Token / 监听 | 必须 | 建议 | 必须 | 可选 | 必须 | 必须 |
| Nginx 模板收敛 | 必须 | 必须 | 必须 | 必须 | 必须 | 必须 |
| FRP / Agent 拆分 | 必须 | 可选 | 建议 | 可选 | 必须 | 必须 |
| 前端拆分 | 可选 | 必须 | 建议 | 必须 | 可选 | 可选 |
| SQLite 基准 | 必须 | 可选 | 必须 | 必须 | 可选 | 可选 |
| 迁移包 | 必须 | 必须 | 可选 | 必须 | 可选 | 可选 |
| 2FA | 必须 | 必须 | 可选 | 建议 | 可选 | 可选 |
| SMART / UPS | 必须 | 必须 | 建议 | 必须 | 可选 | 可选 |
| Prometheus | 必须 | 必须 | 必须 | 建议 | 可选 | 可选 |

### 7.1 平台注意事项

- Windows 仅用于开发，不作为生产验证依据。
- fnOS host 模式必须验证 80/443 被系统 Nginx 占用时的行为。
- Debian / Ubuntu 与宝塔环境的 Nginx 路径不同，Agent 必须都能探测。
- 旧 Agent 必须与新控制端兼容。
- 新 Agent 必须兼容旧控制端的基础状态接口。

---

## 8. 发布与回滚策略

### 8.1 版本建议

| 里程碑 | 版本类型 | 内容 |
| --- | --- | --- |
| M1 | patch | P0 修复与安全边界 |
| M2 | minor | P1 重构与测试基线 |
| M3 | patch/minor | P2 工程收口 |
| M4 | minor | 迁移包、审计、2FA |
| M5 | minor | SMART / UPS、Prometheus、状态页增强 |

最终版本号仍以实际兼容性判断为准。

### 8.2 Agent 版本

以下改动必须递增 `AgentVersion`：

- 常量时间 Token；
- 监听策略；
- 限流状态错误处理；
- Nginx 模板变化；
- vhost 版本、证书、frps 与日志接口变化。

发布时：

1. 更新 `AgentVersion`；
2. 重编 amd64 / arm64；
3. 替换 `internal/frp/agentbin`；
4. 更新 CHANGELOG；
5. 在 VPS 上升级 Agent 后验证。

### 8.3 数据库

每个 P3 功能各自新增迁移：

- 迁移编号按现有文件顺序递增；
- 不修改已发布迁移；
- 迁移前自动备份；
- 升级失败时恢复数据库。

### 8.4 回滚

| 改动 | 回滚方式 |
| --- | --- |
| 前端 P0 修复 | 退回旧前端构建 |
| 初始密码日志策略 | 恢复旧输出代码，不涉及数据 |
| Agent 安全策略 | 回滚 Agent 二进制，保留旧配置 |
| Nginx 模板 | 使用已有配置版本回滚 |
| 文件拆分 | 纯代码回滚，不影响运行数据 |
| 数据库迁移 | 使用升级前备份恢复 |
| 迁移包导入 | 使用自动创建的导入前备份恢复 |

---

## 9. 实施检查清单

### 9.1 每个 P0 任务

- [ ] 复现问题；
- [ ] 写失败测试；
- [ ] 最小修复；
- [ ] 验证正常路径；
- [ ] 验证失败路径；
- [ ] 更新文档；
- [ ] 记录是否需要 Agent 升级。

### 9.2 每个 P1 重构

- [ ] 先确认当前测试全绿；
- [ ] 只做文件移动或纯函数抽取；
- [ ] 保持包外 API 不变；
- [ ] 每步跑全量 Go 测试；
- [ ] 前端改动跑 typecheck、test、build；
- [ ] 使用截图验证关键页面；
- [ ] 不混入业务功能。

### 9.3 每个 P2 清理

- [ ] 有引用证明；
- [ ] diff 可审查；
- [ ] 不含未确认删除；
- [ ] CI 卡口通过；
- [ ] 视觉和主题无回归。

### 9.4 每个 P3 功能

- [ ] 独立迁移；
- [ ] 独立接口；
- [ ] 独立 UI；
- [ ] 安全评审；
- [ ] 降级路径；
- [ ] 文档和示例；
- [ ] 性能与并发测试；
- [ ] 备份和回滚验证。

---

## 10. 需要用户提前决策的事项

1. **Agent 监听默认值**：
   - 是否接受新安装默认 `127.0.0.1:7700`；
   - 是否接受远程模式必须显式配置 HTTPS 或 `--insecure-allow-http`。
2. **迁移包加密实现**：
   - 使用自研 Argon2id + AES-GCM 分块格式，还是引入成熟加密库；
   - 导入是否只允许空实例，还是也支持显式覆盖。
3. **2FA 依赖**：
   - 是否允许引入 TOTP / QR 相关第三方库；
   - 恢复码数量与格式。
4. **Prometheus 鉴权**：
   - 使用独立 Token，还是允许现有 read API Token；
   - 端点使用 `/metrics` 还是 `/api/metrics/prometheus`。
5. **审计保留期**：
   - 默认 180 天是否合适；
   - 是否允许永久保留。
6. **SMART 权限边界**：
   - Fonu 容器是否默认具有访问宿主磁盘设备的权限；
   - 只读失败时是否接受只显示“不可用”。

---

## 11. 最终建议

最合理的实施方式不是按 P0→P3 一次性做完，而是先完成 M1，把明确缺陷和安全性问题清掉；随后用 M2 的测试基线保护 FRP / Agent 重构；等核心模块稳定后，再做前端拆分和工程清理；最后按产品价值逐项推进 P3。

如果只能先做三项，建议顺序是：

1. **P0-1 + P0-2 + P0-4**：直接修复用户可见缺陷、日志泄密和配置丢失风险；
2. **P1-4 + P1-2**：先建立保护网，再拆 FRP / Agent 巨型文件；
3. **P1-1**：收敛 Nginx 模板，消除本机与公网两套安全逻辑漂移的根因。

这样可以在不破坏现有部署的前提下，逐步把 Fonu 从“功能完整但复杂度集中”推进到“模块清晰、测试可靠、升级可控”的状态。
