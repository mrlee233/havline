# Gitea Registry 镜像拉取问题与修复方案

> 记录时间：2026-10-02
> 适用范围：`jayhub.top/mrlee/havline` 与飞牛应用 `Fnos-Havline`

## 一、当前状态

Havline `1.9.0` 的多架构镜像已经通过 Registry HTTP API 上传成功：

| 平台 | 子 manifest |
| --- | --- |
| linux/amd64 | `sha256:83b31003aaa84435bdfc02e5e436d7e23f71e184326002ff8cdc6bba84e4c77c` |
| linux/arm64 | `sha256:5f30e660bd8ec4563bfe225c69bc5a1cb80b1766655494c2d7bd863f43aa4b06` |
| 多架构索引 | `sha256:6eed17af17349bea312b5e1fd8d6fbbde074d157a9e8687a734d9371c59a8db2` |

Registry 中当前 tag：

```text
mrlee/havline:1.9.0
```

使用带认证的 Registry API 可以正常读取 OCI index。Docker CLI 的
`push` 与 `pull` 仍然失败：

```text
failed to authorize: failed to fetch oauth token:
authorization server did not include a token in the response
```

## 二、根因

Gitea 当前生成的 Registry 认证地址是 HTTP：

```text
WWW-Authenticate: Bearer realm="http://jayhub.top/v2/token",
service="container_registry",scope="*"
```

实际 token 接口已经通过 HTTPS 提供服务。Docker 遇到 HTTP 到 HTTPS 的
跨协议跳转时，会丢掉用于换取 token 的 Basic 认证信息，最终拿不到带权限
的 token。

同时，匿名请求得到的 token 没有 `pull` scope：

```text
scope=[]
user=-1
```

这意味着飞牛设备在没有 Registry 登录凭据时，即使服务器修好 realm，也
仍然会被公开包的认证策略拦截。仓库和包本身虽然是 public，但当前 Gitea
Registry 仍要求客户端登录。

## 三、服务器端修复

### 1. 修正 Gitea 的 ROOT_URL

编辑 Gitea 的 `app.ini`，把 `ROOT_URL` 从 HTTP 改为 HTTPS：

```ini
[server]
ROOT_URL = https://jayhub.top/
```

### 2. 确保 Nginx 传递正确协议

在 Gitea 的反向代理配置中加入：

```nginx
proxy_set_header Host $host;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
```

如果你使用的是宝塔、1Panel 或飞牛自带的反代，请在对应站点配置中确认
`X-Forwarded-Proto` 没有被覆盖为 `http`。

### 3. 重启 Gitea

```bash
docker restart gitea
```

如果容器名不同，先执行：

```bash
docker ps --format '{{.Names}} {{.Image}}'
```

### 4. 验证 realm

```bash
curl -sSI https://jayhub.top/v2/ | grep -i www-authenticate
```

期望结果：

```text
WWW-Authenticate: Bearer realm="https://jayhub.top/v2/token",
service="container_registry",scope="*"
```

### 5. 验证 Docker 拉取

```bash
docker pull jayhub.top/mrlee/havline:1.9.0
```

如果本机已经登录过 `jayhub.top`，先执行一次：

```bash
docker logout jayhub.top
docker login jayhub.top
```

## 四、飞牛端仍被 401 拦截时的处理

### 方案 A：在飞牛设备登录 Registry（推荐用于继续使用 Gitea）

在飞牛终端中执行：

```bash
docker login jayhub.top -u mrlee
```

密码建议使用 Gitea Personal Access Token，而不是账号密码。Token 权限
至少需要：

```text
read:package
```

登录后再安装 `havline-1.9.0.fpk`。这种方式需要每台安装设备执行一次登录，
不适合完全没有终端操作经验的使用者。

### 方案 B：改为离线镜像包（推荐给最终用户）

如果希望完全绕开 Gitea Registry 的认证和 realm 问题，可以把 `docker save`
生成的镜像归档放进飞牛 `.fpk`，并在安装回调中执行 `docker load`。这样
安装过程不依赖外网 Registry，也不需要在飞牛设备上保存 Registry 凭据。

这个方案的代价是 `.fpk` 体积会显著增加，需要同时评估飞牛应用中心的上传
限制和升级包体积。

### 方案 C：改用允许匿名拉取的公共 Registry

如果继续发布给普通用户使用，建议把镜像推送到 Docker Hub、GHCR 或其它
允许匿名拉取多架构镜像的 Registry，再把飞牛 Compose 中的镜像地址改成
对应的公共地址。这个方案的用户体验最好，但需要额外的 Registry 账号和
发布流程。

## 五、应急手动推送工具

在 Gitea realm 修好之前，可以使用下面的脚本绕过 Docker 客户端直接推送：

```powershell
docker buildx build `
  --platform linux/amd64 `
  --build-arg VERSION_FILE=VERSION `
  --output type=oci,dest=build\havline-amd64.oci.tar .

tar -xf build\havline-amd64.oci.tar -C build\havline-amd64

pwsh scripts/push-oci-manual.ps1 `
  -LayoutDir build\havline-amd64 `
  -Platform linux/amd64 `
  -Tag 1.9.0
```

脚本会从 Git 凭据存储中读取 `jayhub.top` 的登录信息，通过 HTTPS 请求
Bearer token，再直接调用 Registry HTTP API 上传缺失 blob 和 manifest。

这个脚本只是应急工具，不能替代服务器端的 `ROOT_URL` 修复。服务器修好
之后，应恢复使用标准的 `docker buildx build --push`。

## 六、发布前检查清单

```text
[ ] Gitea ROOT_URL 已改为 https://jayhub.top/
[ ] Nginx 已传递 X-Forwarded-Proto https
[ ] curl /v2/ 返回的 realm 是 https
[ ] docker pull 在干净客户端上成功
[ ] 飞牛设备无需额外操作即可安装
[ ] amd64 与 arm64 镜像都能被设备拉取
```
