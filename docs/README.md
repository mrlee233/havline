# Fonu 文档索引

> 维护约定：新增方案 / 审查文档时在下方登记；方案落地后在「状态」列标注实施结果，并给已被推翻的描述加时效说明。
> 最近更新：2026-09-28（v1.6.4）

## 方案与设计（本目录）

| 文件 | 主题 | 状态 |
| --- | --- | --- |
| `project-analysis-2026-09-27.md` | 全项目详细分析（v1.2.19）：架构、模块、数据、API、前端、链路、风险与改进路线 | 当前版本快照 |
| `improvement-roadmap-2026-09-27.md` | P0-P3 改进路线实施方案：逐项改法、测试、验收、依赖、发布与回滚 | P0-1、P0-3、P0-4 已完成，其余待实施 |
| `system-upgrade.md` | Docker Compose 一键升级：sidecar 架构、首次部署、安全边界、回滚、故障排查与验证页 | 已实施（v1.3.8） |
| `agent-listen-strategy-plan.md` | Agent 默认监听策略：回环监听、SSH 隧道优先、HTTPS pin 可选与自动迁移方案 | 阶段 1-4 已实施（v1.4.0）；2.0.0 兼容清理待后续 |
| `project-audit-2026-09-24.md` | 全项目审查：P0/P1/P2 分级，含精确行号与逐条修法 | **进行中**——P0 已随 v1.2.7 完成，P1 实施中 |
| `frp-server-list-merge-plan.md` | FRP 服务端列表并入「公网服务端」页（方案 A，四步） | 步 1/2 已完成（v1.2.6）；步 3 文案收尾、步 4 死样式清理未做 |
| `agent-capability-plan.md` | fonu-agent 能力扩展（P0/P1/P2/P3） | 已实施（agent 0.11.0）；P3 三项不做 |
| `agent-view-design.md` | 公网 Agent 页设计方案（Pass 1/2 流程） | 部分过期：顶部下拉已被左列表 + 右详情取代 |
| `agent-view-design-v2.md` | 公网 Agent 页设计方案 v2 | 部分过期：同上 |
| `agent-view-refactor-plan.md` | 公网 Agent 页四段式（状态 → 控制 → 生命周期 → 输出）整改 | 已实施 |
| `proxy-remote-design.md` | 公网反代页设计方案（以本地反代页为参照） | 已实施；列表已改为左服务端 + 右台账 |
| `agent-proxy-settings-plan.md` | 公网反代「部署设置」对齐本地反代的语义迁移 | 已实施 |
| `agent-proxy-roadmap.md` | 公网反代功能增强规划（含部署踩坑史） | 部分已实施（漂移检测、全量重部署），其余待排期 |
| `frps-features.md` | frps 服务端功能实现清单（对照官方 frps_full_example.toml） | 参考 |
| `frps服务端管理方案.md` | FRPS 服务端管理方案（fonu-agent 部署在公网服务器） | 已实施（Agent 管理面板） |
| `frps管理方案.md` | FRPS 管理方案（早期版本） | 疑似已被 `frps服务端管理方案.md` 取代，保留供追溯（待确认） |
| `upstream-sync-plan.md` | 上游 1.0.2 → 1.0.5 对照与可借鉴项 | 部分过期：对 `/frp` 现状的描述基于 2026-09-23 |
| `FRP公网暴露操作手册.md` | FRP 公网暴露操作手册（面向使用） | 参考 |
| `DEPLOYMENT.md` | 部署安装：裸机 / Docker / 飞牛 host 模式 | 参考；适用 `fonu-1.1.0` 安装包 |

## 其它位置

- 仓库根 `../docs/`（`111.md`、`FRP迁移方案-0.1.25到1.0.2.md`、`FRP版本对比-1.0.2-vs-0.1.25.md`）为**临时区**，不随本项目维护；正式文档一律放本目录。
- `screenshots/` 供 README 引用。
