#!/usr/bin/env bash
# Havline 容器升级：在部署机（NAS）上直接构建，不需要再从别处传镜像包
#
#   sudo bash scripts/upgrade.sh                     # 拉取最新源码 → 构建 → 重建容器
#   sudo bash scripts/upgrade.sh --no-pull           # 不拉代码，用当前源码重建
#   sudo bash scripts/upgrade.sh --rollback 1.2.13   # 回滚到已存在的镜像 tag（不构建）
#
# 用 COMPOSE_FILE 指定编排文件（默认 docker-compose.yml）：
#   COMPOSE_FILE=docker-compose.host.yml sudo bash scripts/upgrade.sh
#
# 数据在 havline-data 卷里，重建容器不会丢。每次构建按仓库 VERSION 打一个新 tag
# （旧的 tag 会留着），所以出问题可以直接 --rollback 回上一个版本。
set -euo pipefail

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
APP_CONTAINER="${APP_CONTAINER:-havline}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:6893/api/version}"
NO_PULL=0
ROLLBACK_TAG=""

usage() {
  sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --no-pull) NO_PULL=1; shift ;;
    --rollback) ROLLBACK_TAG="${2:?--rollback 需要一个镜像 tag，例如 1.2.13}"; shift 2 ;;
    -h|--help) usage 0 ;;
    *) echo "未知参数：$1" >&2; usage 1 ;;
  esac
done

if ! command -v docker >/dev/null 2>&1; then
  echo "没有找到 docker 命令：这个脚本要在运行 Havline 容器的那台机器上执行" >&2
  exit 1
fi
if [ ! -f "$COMPOSE_FILE" ]; then
  echo "找不到 $COMPOSE_FILE：请在仓库根目录运行（或用 COMPOSE_FILE=... 指定编排文件）" >&2
  exit 1
fi

# 当前容器实际使用的镜像 tag，升级前后各打印一次，回滚时心里有数
current_image() {
  docker inspect --format '{{.Config.Image}}' "$APP_CONTAINER" 2>/dev/null || echo '（容器未运行）'
}

echo "==> 升级前：$(current_image)"

if [ -n "$ROLLBACK_TAG" ]; then
  if ! docker image inspect "havline:$ROLLBACK_TAG" >/dev/null 2>&1; then
    echo "本机没有 havline:$ROLLBACK_TAG；已有 tag：$(docker images --format '{{.Tag}}' havline | tr '\n' ' ')" >&2
    exit 1
  fi
  HAVLINE_VERSION="$ROLLBACK_TAG" docker compose -f "$COMPOSE_FILE" up -d
  echo "==> 已切回 havline:$ROLLBACK_TAG"
else
  if [ "$NO_PULL" -eq 0 ]; then
    if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      echo '==> git pull --ff-only'
      git pull --ff-only
    else
      echo '==> 当前目录不是 git 仓库，跳过拉取（请先自行更新源码）'
    fi
  fi
  if [ ! -f VERSION ]; then
    echo '找不到 VERSION 文件：请在仓库根目录运行' >&2
    exit 1
  fi
  HAVLINE_VERSION="$(tr -d ' \t\n\r' < VERSION)"
  export HAVLINE_VERSION
  echo "==> 构建并重建：havline:$HAVLINE_VERSION（首次构建较慢，之后有构建缓存）"
  docker compose -f "$COMPOSE_FILE" up -d --build
fi

echo '==> 等待服务就绪…'
for _ in $(seq 1 30); do
  if curl -fsS "$HEALTH_URL" >/dev/null 2>&1; then
    echo "==> 升级后：$(current_image)"
    echo "    版本接口：$(curl -fsS "$HEALTH_URL" 2>/dev/null)"
    echo "    日志：docker logs --tail 50 $APP_CONTAINER"
    exit 0
  fi
  sleep 2
done

echo "服务在 60 秒内没有响应 $HEALTH_URL，请看：docker logs --tail 100 $APP_CONTAINER" >&2
exit 1
