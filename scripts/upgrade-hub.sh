#!/usr/bin/env bash
# Havline 一键升级（适用于「只有编排文件」的部署目录，即 docker-compose.hub.yml 那种）
#
#   sudo bash upgrade-hub.sh
#
# 可选环境变量：
#   COMPOSE_FILE=docker-compose.hub.yml   # 要用的编排文件
#   REPO=http://.../mrlee/Havline.git       # 源码仓库
#   BRANCH=main                           # 分支
#
# 它做四件事：
#   1) 尽量取到仓库里的 VERSION，作为本次镜像 tag（有 tag 才能回滚）；
#      取不到就用时间戳当 tag，同样是唯一的、可回滚的
#   2) docker compose up -d --build —— 构建上下文是 Git 地址，会重新拉最新分支
#   3) 轮询 /api/version 确认服务起来
#   4) 打印「回滚到上一个 tag」的命令
set -euo pipefail

REPO="${REPO:-https://jayhub.top/mrlee/Havline.git}"
BRANCH="${BRANCH:-main}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.hub.yml}"
APP_CONTAINER="${APP_CONTAINER:-havline}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:6893/api/version}"

if ! command -v docker >/dev/null 2>&1; then
  echo "没有找到 docker：这个脚本要在运行 Havline 容器的那台机器上执行" >&2
  exit 1
fi
if [ ! -f "$COMPOSE_FILE" ]; then
  echo "找不到 $COMPOSE_FILE：请把它放在本脚本同目录（或用 COMPOSE_FILE=... 指定）" >&2
  exit 1
fi

# 升级前容器用的镜像 tag，升级失败时可以切回去
previous_image=$(docker inspect --format '{{.Config.Image}}' "$APP_CONTAINER" 2>/dev/null || echo '')
echo "==> 当前：${previous_image:-（容器未运行）}"

# 取版本号：优先仓库里的 VERSION；没 git 就退回时间戳（同样唯一，可回滚）
version=""
if command -v git >/dev/null 2>&1; then
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' EXIT
  if git clone --depth 1 --branch "$BRANCH" "$REPO" "$work/src" >/dev/null 2>&1 && [ -f "$work/src/VERSION" ]; then
    version="$(tr -d ' \t\n\r' < "$work/src/VERSION")"
    echo "==> 仓库版本：$version"
  fi
fi
if [ -z "$version" ]; then
  version="$(date +%Y%m%d-%H%M%S)"
  echo "==> 未能从仓库读到 VERSION（可能没装 git），改用时间戳 tag：$version"
fi

export HAVLINE_VERSION="$version"
echo "==> 构建并重建：havline:$version（构建上下文是 Git 地址，会自动拉最新 $BRANCH；有缓存时只重编改动的部分）"
docker compose -f "$COMPOSE_FILE" up -d --build

echo '==> 等待服务就绪…'
for _ in $(seq 1 30); do
  if curl -fsS "$HEALTH_URL" >/dev/null 2>&1; then
    echo "==> 升级完成"
    echo "    当前版本：$(curl -fsS "$HEALTH_URL" 2>/dev/null)"
    if [ -n "$previous_image" ]; then
      echo "    回滚：HAVLINE_VERSION=${previous_image#havline:} docker compose -f $COMPOSE_FILE up -d"
    fi
    echo "    日志：docker logs --tail 50 $APP_CONTAINER"
    exit 0
  fi
  sleep 2
done

echo "服务在 60 秒内没有响应 $HEALTH_URL，请看：docker logs --tail 100 $APP_CONTAINER" >&2
if [ -n "$previous_image" ]; then
  echo "如需回滚：HAVLINE_VERSION=${previous_image#havline:} docker compose -f $COMPOSE_FILE up -d" >&2
fi
exit 1
