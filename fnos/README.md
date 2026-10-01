# Havline fnOS 应用打包

本目录保存 Havline 的飞牛 fnOS 应用工程。

- 应用工程：[`havline/`](havline/)
- 官方工具：`fnpack`
- 当前工具版本：`1.2.3`

## 当前状态

已经完成：

- 使用 `fnpack create havline --template docker` 生成官方 Docker 应用骨架；
- 配置 `manifest`、应用入口、Docker Compose、权限和资源；
- 增加安装向导、配置向导和卸载向导；
- 增加安装、升级、配置和卸载生命周期脚本；
- 支持通过环境变量 `HAVLINE_INITIAL_ADMIN_PASSWORD` 初始化管理员。

仍需完成：

- 将 `app/docker/docker-compose.yaml` 中的 `ghcr.io/your-org/havline` 替换为实际多架构镜像地址；
- 将模板图标替换为 Havline 正式图标；
- 在飞牛测试设备验证安装、启动、入口、升级和卸载；
- 验证 amd64 与 arm64 镜像均可用。

## 打包

Windows PowerShell：

```powershell
cd F:\Fonu-main\havline
.\fnos\build.ps1
```

脚本会先复制一份不含 `.DS_Store` 的干净工程，再调用 `fnpack` 打包，输出到：

```text
fnos/dist/havline-<version>.fpk
```

如果 `fnpack` 已在 PATH 中，也可以传入路径：

```powershell
.\fnos\build.ps1 -FnpackPath fnpack
```

## 测试安装

```bash
appcenter-cli install-fpk havline.fpk
```

安装后检查：

1. 应用中心显示 Havline；
2. Docker 镜像可以拉取；
3. Havline 容器能够启动；
4. 桌面入口可以打开管理界面；
5. 数据写入 `${TRIM_PKGVAR}`；
6. 卸载默认保留数据。

## 当前默认端口

| 用途 | 安装向导默认值 | 容器端口 |
| --- | ---: | ---: |
| 管理界面 | `6893` | `6893` |
| HTTP 反向代理 | `18080` | `80` |
| HTTPS 反向代理 | `9443` | `443` |

## 首发边界

- 不挂载 Docker Socket。
- 不启动 Havline 自带 updater。
- 不使用 host 网络。
- 不共享数据库、证书、密钥、Nginx、FRP 和 cloudflared 目录。
- 应用中心负责安装、升级、启动、停止和卸载。
